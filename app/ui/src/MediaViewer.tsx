// SPDX-License-Identifier: AGPL-3.0-only
// The image viewer of a post's pictures (a lightbox): a modal dialog of React Aria (WAI-ARIA APG, dialog pattern), so it
// traps focus, closes on Esc, by its × button and by a press on the scrim, and returns focus to the picture that
// opened it. Unlike the other dialogs it has no header: the picture fills the frame, scaled down to fit, and the
// buttons sit over it; its description is laid over it on demand, and the buttons, the arrow keys and a swipe step to the
// picture before and after it. The position under it counts the post's media as its grid does (videos too), and a
// picture that loads shows its blur and a spinner, one that failed "Try again".
import { type RefObject, useEffect, useEffectEvent, useId, useRef, useState } from "react";
import {
  Button as AriaButton,
  Dialog as AriaDialog,
  Heading,
  Modal,
  ModalOverlay,
  useLocale,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { Blurhash } from "./Blurhash";
import { IconButton } from "./IconButton";
import { AltTextIcon, CloseIcon, NextIcon, PreviousIcon } from "./Icons";
import { MediaLoadFailure, type MediaLoadLabels } from "./MediaLoadFailure";
import { type ImageSources, Picture } from "./Picture";
import { Spinner } from "./Spinner";
import { Tooltip, TooltipTrigger } from "./Tooltip";

// The picture is the frame: no header and no padding, so it fills the panel. The dialog is named by a heading that
// only screen readers meet, the × sits over the picture's corner and the pager under it, outside the frame.
const mediaViewer = tv({
  slots: {
    overlay: [
      "fixed inset-x-0 top-0 z-50 flex h-full max-h-(--visual-viewport-height) items-center justify-center bg-overlay p-4",
      "motion-safe:transition-opacity motion-safe:duration-normal data-entering:opacity-0 data-exiting:opacity-0",
      "motion-safe:data-exiting:ease-exit",
    ],
    // Wider than any other dialog: a picture uses the room of the screen, up to its own size.
    panel: "flex max-h-full w-full max-w-[min(100%,80rem)] justify-center outline-none",
    dialog: "flex min-h-0 max-w-full flex-col items-center gap-3 outline-none",
    frame: "relative max-w-full overflow-hidden rounded-xl bg-surface-sunken shadow-lg",
    figure: "m-0",
    // At most its own size, and scaled down to the panel's width and to the height the screen leaves it.
    image: "block size-auto max-h-[calc(85svh-4rem)] max-w-full object-contain",
    // The two round buttons over the picture's corners.
    corner: [
      "absolute z-20 grid size-10 place-items-center rounded-full shadow-md outline-none",
      "data-focus-visible:focus-ring pointer-coarse:size-11 [&>svg]:size-5",
    ],
    close: "inset-e-2.5 top-2.5 bg-toast text-toast-fg",
    // Turns light while the description is shown, so that it stands out from the dark ground it sits on.
    altToggle:
      "inset-e-2.5 bottom-2.5 bg-toast text-toast-fg data-[expanded=true]:bg-toast-fg data-[expanded=true]:text-toast",
    // Over the whole picture; the room at the end is the toggle's.
    alt: "absolute inset-0 z-10 m-0 overflow-y-auto overscroll-contain bg-alt-overlay px-6 py-10 pe-10 text-base/8 wrap-break-word whitespace-pre-line text-toast-fg",
    // While the picture loads: its blur, or the frame's surface, with a spinner in the middle.
    loading: "pointer-events-none absolute inset-0 z-10 grid place-items-center",
    blur: "absolute inset-0 size-full",
    nav: "flex items-center gap-3",
    navButton: "w-10 bg-surface-raised px-0 shadow-md pointer-coarse:w-11",
    count: "rounded-full bg-toast px-3 py-1 text-sm font-extrabold text-toast-fg tabular-nums",
  },
});

/** One picture of the viewer. */
type MediaViewerItem = {
  id: string;
  /** The renditions of the picture at its original size (still, and moving for an animation). */
  sources: ImageSources;
  /** The author's description, or `null` when there is none. */
  alt: string | null;
  lang?: string | undefined;
  /** The size of the original picture, when known, so that the layout does not move while it loads. */
  width?: number | undefined;
  height?: number | undefined;
  /** The blurhash of the picture, drawn while it loads, as in the grid. */
  blurhash?: string | undefined;
  /**
   * The picture's place (1-based) among all the media of the post, videos and audio included, so that the position
   * matches the grid ("3 / 3" for the third cell); without it, its place among the pictures of the viewer.
   */
  position?: number | undefined;
};

/** The texts of the viewer, which the app translates; the load texts are the picture's loading and failure. */
export type MediaViewerLabels = MediaLoadLabels & {
  /** The heading that names the dialog, for the picture at `position` (1-based) of `count`: "Image 2 of 3". */
  title: (position: number, count: number) => string;
  /** The name of the × button. */
  close: string;
  /** The position shown between the buttons that step, for the picture at `position` (1-based) of `count`: "2 / 3". */
  position: (position: number, count: number) => string;
  /** The name of the button that shows or hides the picture's description. */
  altText: string;
  /** The names of the buttons that show the picture before and after. */
  previous: string;
  next: string;
};

export type MediaViewerProps = {
  items: readonly MediaViewerItem[];
  /** The picture shown; it stays the last one while the viewer closes, so the closing panel keeps its picture. */
  index: number;
  isOpen: boolean;
  onIndexChange: (index: number) => void;
  onOpenChange: (isOpen: boolean) => void;
  labels: MediaViewerLabels;
  /** The name of a picture without a description. */
  noDescriptionLabel: string;
  /** How many media the post has, videos and audio included, for the position; without it, the pictures' count. */
  total?: number | undefined;
  /** Shows the description of the picture the viewer opens at: it opened from the picture's description mark. */
  altOpenAtStart?: boolean;
};

type ViewerContentProps = Pick<
  MediaViewerProps,
  "items" | "index" | "onIndexChange" | "labels" | "noDescriptionLabel" | "total" | "altOpenAtStart"
>;

/**
 * The picture `by` places after `index` (before it, when negative), going round from the last picture to the first and
 * back. The buttons stay enabled at either end, so that the focus never sits on a button that the step disabled.
 */
function step(index: number, by: 1 | -1, count: number): number {
  return (index + by + count) % count;
}

type FrameProps = {
  item: MediaViewerItem;
  labels: MediaViewerLabels;
  noDescriptionLabel: string;
  /** Shows the description at first: the viewer opened from the picture's description mark. */
  defaultAltOpen: boolean;
  /** Steps to the picture after (`1`) or before (`-1`) on a horizontal swipe; undefined for a single picture. */
  onSwipe: ((by: 1 | -1) => void) | undefined;
};

/** How far a finger moves along the line before the move is a swipe, in CSS pixels. */
const swipeDistance = 48;

/**
 * Calls `onSwipe` when a finger or a pen moves along the line over the element of `ref` by at least `swipeDistance`
 * and more than across it: `1` (the next picture) towards the start of the line, as a page turns, `-1` the other way.
 * A mouse does not swipe; it has the buttons and the arrow keys. The element only listens: it is not a control.
 */
function useSwipe(ref: RefObject<HTMLElement | null>, onSwipe: ((by: 1 | -1) => void) | undefined) {
  const { direction } = useLocale();
  const onSwipeEvent = useEffectEvent((by: 1 | -1) => {
    onSwipe?.(by);
  });
  const enabled = onSwipe !== undefined;
  useEffect(() => {
    const element = ref.current;
    if (element === null || !enabled) return;
    let start: { x: number; y: number } | null = null;
    const down = (event: PointerEvent) => {
      start = event.pointerType === "mouse" ? null : { x: event.clientX, y: event.clientY };
    };
    const up = (event: PointerEvent) => {
      if (start === null) return;
      const dx = event.clientX - start.x;
      const dy = event.clientY - start.y;
      start = null;
      if (Math.abs(dx) < swipeDistance || Math.abs(dx) < Math.abs(dy)) return;
      onSwipeEvent((direction === "ltr" ? dx < 0 : dx > 0) ? 1 : -1);
    };
    const cancel = () => {
      start = null;
    };
    element.addEventListener("pointerdown", down);
    element.addEventListener("pointerup", up);
    element.addEventListener("pointercancel", cancel);
    return () => {
      element.removeEventListener("pointerdown", down);
      element.removeEventListener("pointerup", up);
      element.removeEventListener("pointercancel", cancel);
    };
  }, [ref, enabled, direction]);
}

/**
 * One picture with the buttons over it. It is mounted anew for each picture, so the description starts hidden. While
 * the picture loads, its blur (or the frame's surface) shows with a spinner; a picture that could not load shows
 * `MediaLoadFailure`, whose "Try again" loads it again. A finger swiped along the line steps to the next or the
 * previous picture (the start of the line is the previous one, mirrored in right-to-left text).
 */
function Frame({ item, labels, noDescriptionLabel, defaultAltOpen, onSwipe }: FrameProps) {
  const styles = mediaViewer();
  const [altOpen, setAltOpen] = useState(defaultAltOpen);
  const [load, setLoad] = useState<{ status: "loading" | "loaded" | "failed"; attempt: number }>({
    status: "loading",
    attempt: 0,
  });
  const frameRef = useRef<HTMLDivElement>(null);
  useSwipe(frameRef, onSwipe);
  const altId = useId();
  const language = item.alt !== null ? { dir: "auto" as const, lang: item.lang } : {};
  const sized = item.width !== undefined && item.height !== undefined;
  return (
    <div ref={frameRef} className={styles.frame()} data-load={load.status}>
      {load.status === "loading" && (
        <div className={styles.loading()}>
          {item.blurhash !== undefined && <Blurhash hash={item.blurhash} className={styles.blur()} />}
          <Spinner label={labels.loading} className="relative" />
        </div>
      )}
      {load.status === "failed" && (
        <MediaLoadFailure
          labels={labels}
          onRetry={() => {
            setLoad((current) => ({ status: "loading", attempt: current.attempt + 1 }));
          }}
        />
      )}
      {/* A new picture is announced by its name, as a carousel announces its slide (WAI-ARIA APG). */}
      <figure
        aria-live="polite"
        className={styles.figure({ class: !sized && load.status !== "loaded" ? "min-h-48 min-w-64" : undefined })}
      >
        <Picture
          // A new attempt is a new element, so the browser asks for the picture again.
          key={load.attempt}
          sources={item.sources}
          alt={item.alt ?? noDescriptionLabel}
          {...language}
          {...(sized && { width: item.width, height: item.height })}
          decoding="async"
          onLoad={() => {
            setLoad((current) => ({ ...current, status: "loaded" }));
          }}
          onError={() => {
            setLoad((current) => ({ ...current, status: "failed" }));
          }}
          className={styles.image()}
        />
      </figure>
      {/* The picture's name already says it to screen readers; this shows it to everyone else, on demand. */}
      {item.alt !== null && (
        <>
          <p id={altId} hidden={!altOpen} aria-hidden="true" className={styles.alt()} {...language}>
            {item.alt}
          </p>
          <TooltipTrigger>
            <AriaButton
              aria-label={labels.altText}
              aria-expanded={altOpen}
              aria-controls={altId}
              data-expanded={altOpen}
              onPress={() => {
                setAltOpen((open) => !open);
              }}
              className={styles.corner({ class: styles.altToggle() })}
            >
              <AltTextIcon />
            </AriaButton>
            <Tooltip>{labels.altText}</Tooltip>
          </TooltipTrigger>
        </>
      )}
      <TooltipTrigger>
        <AriaButton slot="close" aria-label={labels.close} className={styles.corner({ class: styles.close() })}>
          <CloseIcon />
        </AriaButton>
        <Tooltip>{labels.close}</Tooltip>
      </TooltipTrigger>
    </div>
  );
}

/**
 * The picture and its controls, mounted only while the dialog is open, so that its listener for the arrow keys
 * lives as long as the dialog. The dialog itself, and not only one of its controls, has the focus at first, so the
 * keys are heard on the window; the modal dialog keeps the rest of the page out of reach meanwhile.
 */
function ViewerContent({
  items,
  index,
  onIndexChange,
  labels,
  noDescriptionLabel,
  total,
  altOpenAtStart = false,
}: ViewerContentProps) {
  const { direction } = useLocale();
  const styles = mediaViewer();
  const item = items[index];
  const count = items.length;
  // The picture the viewer opened at, which shows its description at once when it opened from the description mark.
  const [startId] = useState(item?.id);

  // Left and right follow the screen: in right-to-left text the picture before is on the right (as its button is).
  const onKeyDown = useEffectEvent((event: KeyboardEvent) => {
    if (count < 2 || event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    const forward = (event.key === "ArrowRight") === (direction === "ltr");
    event.preventDefault();
    onIndexChange(step(index, forward ? 1 : -1, count));
  });
  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      onKeyDown(event);
    };
    window.addEventListener("keydown", listener);
    return () => {
      window.removeEventListener("keydown", listener);
    };
  }, []);

  if (item === undefined) return null;
  return (
    <>
      {/* A new picture is a new frame: the old one does not stay on screen while the next one loads. */}
      <Frame
        key={item.id}
        item={item}
        labels={labels}
        noDescriptionLabel={noDescriptionLabel}
        defaultAltOpen={altOpenAtStart && item.id === startId}
        onSwipe={
          count < 2
            ? undefined
            : (by) => {
                onIndexChange(step(index, by, count));
              }
        }
      />
      {count > 1 && (
        <div className={styles.nav()}>
          <IconButton
            size="md"
            label={labels.previous}
            className={styles.navButton()}
            onPress={() => {
              onIndexChange(step(index, -1, count));
            }}
          >
            <PreviousIcon />
          </IconButton>
          <span aria-hidden="true" className={styles.count()}>
            {labels.position(item.position ?? index + 1, total ?? count)}
          </span>
          <IconButton
            size="md"
            label={labels.next}
            className={styles.navButton()}
            onPress={() => {
              onIndexChange(step(index, 1, count));
            }}
          >
            <NextIcon />
          </IconButton>
        </div>
      )}
    </>
  );
}

/** A modal viewer of pictures at their original size (controlled: `index`, `isOpen`). */
export function MediaViewer({
  items,
  index,
  isOpen,
  onIndexChange,
  onOpenChange,
  labels,
  noDescriptionLabel,
  total,
  altOpenAtStart = false,
}: MediaViewerProps) {
  const styles = mediaViewer();
  return (
    <ModalOverlay isOpen={isOpen} onOpenChange={onOpenChange} isDismissable className={styles.overlay()}>
      <Modal className={styles.panel()}>
        <AriaDialog className={styles.dialog()}>
          <Heading slot="title" className="sr-only">
            {labels.title(index + 1, items.length)}
          </Heading>
          <ViewerContent
            items={items}
            index={index}
            onIndexChange={onIndexChange}
            labels={labels}
            noDescriptionLabel={noDescriptionLabel}
            total={total}
            altOpenAtStart={altOpenAtStart}
          />
        </AriaDialog>
      </Modal>
    </ModalOverlay>
  );
}
