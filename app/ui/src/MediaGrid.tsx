// SPDX-License-Identifier: AGPL-3.0-only
import { type ComponentPropsWithRef, useState, useSyncExternalStore } from "react";
import { Button, ToggleButton, useLocale } from "react-aria-components";
import { tv } from "tailwind-variants";
import { AudioPlayer, type AudioPlayerLabels } from "./AudioPlayer";
import { Blurhash } from "./Blurhash";
import { formatClock } from "./clock";
import { ExternalLink } from "./ExternalLink";
import { AltTextIcon, HideMediaIcon, PlayIcon } from "./Icons";
import { MediaViewer, type MediaViewerLabels } from "./MediaViewer";
import { type ImageSources, Picture, useAnimationsPlay } from "./Picture";
import { Tooltip, TooltipTrigger } from "./Tooltip";
import { VisuallyHidden } from "./VisuallyHidden";

const mediaGrid = tv({
  slots: {
    base: "relative grid aspect-video w-full gap-1 overflow-hidden rounded-lg",
    cell: "relative min-h-0 min-w-0 overflow-hidden bg-surface-sunken",
    // Above the blur, which is positioned: the image covers it as it loads.
    image: "relative size-full object-cover",
    // A picture that opens the viewer: the whole cell is its button, with the focus ring inside the cell's clip.
    open: [
      "relative block size-full cursor-zoom-in outline-none",
      "data-focus-visible:outline-2 data-focus-visible:-outline-offset-2 data-focus-visible:outline-focus",
    ],
    // A video keeps its whole frame in the cell, letterboxed on the cell's surface.
    video: "relative size-full object-contain",
    audio: "absolute inset-x-2 bottom-2",
    // The play disc and the length over a video's poster only show what the native controls do; pointer events pass through to them.
    disc: "pointer-events-none absolute inset-0 m-auto grid size-14 place-items-center rounded-full bg-accent-fill text-accent-fill-fg [&>svg]:size-6",
    // At the top, clear of the native controls along the bottom edge.
    length:
      "pointer-events-none absolute inset-e-2 top-2 rounded-md bg-media-chip px-2 py-0.5 text-xs font-bold text-toast-fg tabular-nums",
    blur: "absolute inset-0",
    // Marks a picture that has a description; decorative, and clear of pointer events so the cell's button still gets them.
    badge:
      "pointer-events-none absolute inset-e-1.5 bottom-1.5 grid size-6 place-items-center rounded-full bg-media-chip text-toast-fg shadow-sm [&>svg]:size-4",
    // The mark of the attachments that the grid leaves out, a link to the post that has them, in the top-end corner.
    more: [
      "absolute inset-e-1.5 top-1.5 min-h-6 gap-1 rounded-md bg-media-chip px-2 text-xs font-bold text-toast-fg tabular-nums no-underline shadow-sm",
      "data-hovered:text-toast-fg data-hovered:underline pointer-coarse:min-h-11 [&>svg]:size-3.5",
    ],
    toggle:
      "absolute cursor-default rounded-full bg-surface-raised font-extrabold text-fg select-none data-focus-visible:focus-ring data-hovered:bg-surface-sunken motion-safe:transition-colors",
  },
  variants: {
    count: {
      1: {},
      2: { base: "grid-cols-2" },
      // The first item fills the start column; the other two stack in the end column.
      3: { base: "grid-cols-2 grid-rows-2 [&>*:first-child]:row-span-2" },
      4: { base: "grid-cols-2 grid-rows-2" },
    },
    revealed: {
      // Concealed: one button over the middle of the grid, so it is the obvious thing to press.
      false: {
        toggle:
          "inset-0 m-auto size-fit min-h-10 max-w-[calc(100%-1rem)] px-4 py-2 text-sm shadow-md pointer-coarse:min-h-11",
      },
      // Shown: a small icon button in the corner that conceals the media again.
      true: {
        toggle: "inset-s-1.5 top-1.5 grid size-8 place-items-center shadow-sm pointer-coarse:size-11 [&>svg]:size-5",
      },
    },
  },
});

/**
 * A video or audio file that a cell plays (ADR-0054 §6). A `video` never plays by itself and has the browser's controls, and an `audio` file never plays by itself and has an `AudioPlayer`; a `gifv` (a short silent video in place of an animated image) loops muted without controls while animations
 * play by themselves (`AutoplayContext`) or the post plays them (`AnimationsPlayContext`) and the system does not ask for reduced motion, and has controls otherwise.
 */
export type MediaPlayback = { kind: "video" | "audio" | "gifv"; url: string };

/** One picture, video, or audio file of a post. */
export type MediaItem = {
  /** A stable ID of the attachment, used as the React key. */
  id: string;
  /** The image URL (the preview size); for a video or audio file, its poster, or undefined without one. */
  src: string | undefined;
  /** The video or audio file to play in the cell, with `src` as its poster; undefined for a picture. */
  playback?: MediaPlayback | undefined;
  /** The renditions of the image, used instead of `src`: the one that fits the cell, still unless `Picture` lets it move. */
  sources?: ImageSources | undefined;
  /**
   * The author's description of the image, or `null` when there is none. It is required so that a missing
   * description is always a decision the caller made; a picture with one gets the note badge.
   */
  alt: string | null;
  /** The language of the description, when known. */
  lang?: string | undefined;
  /**
   * The blurhash of the image, drawn while it is concealed or loading. Without one, the cell's plain surface
   * stands in for it.
   */
  blurhash?: string | undefined;
  /**
   * The picture at its original size, which the viewer shows when the picture is pressed; without it, or without
   * `viewerLabels`, the picture does not open.
   */
  original?: { sources: ImageSources; width?: number | undefined; height?: number | undefined } | undefined;
};

export type MediaGridProps = Omit<ComponentPropsWithRef<"div">, "children"> & {
  /** One to four pictures; items after the fourth are not shown. */
  items: readonly MediaItem[];
  /**
   * The name of a picture that has no description ("No description"). Nothing shows it; screen readers read it
   * as the picture's name, so that the picture is not skipped silently.
   */
  noDescriptionLabel: string;
  /** Whether the author marked the media sensitive. Sensitive media are concealed until the reader shows them. */
  sensitive?: boolean;
  /**
   * The name of the toggle button that shows sensitive media ("Show sensitive media"). It stays the same in both
   * states, as a toggle button's name must: `aria-pressed` says whether the media are shown. While they are shown,
   * the button draws an eye icon instead of the text. Only used when `sensitive`.
   */
  revealLabel: string;
  /** Whether sensitive media are shown (controlled). */
  isRevealed?: boolean;
  /** Whether sensitive media are shown at first (uncontrolled). */
  defaultRevealed?: boolean;
  onRevealedChange?: (isRevealed: boolean) => void;
  /**
   * The texts of the viewer that opens a picture at its original size when it is pressed. Without them the
   * pictures do not open.
   */
  viewerLabels?: MediaViewerLabels | undefined;
  /** The texts of the player of an audio file. */
  audioLabels: AudioPlayerLabels;
  /**
   * The attachments that the grid does not have, such as those of a remote post after the four that the server
   * keeps: a mark in the top-end corner shows `text` ("+2") and links to `href`, the post that has them, in a new
   * tab. `label` names the link and must contain `text` ("+2 more — see the original post").
   */
  more?: { text: string; label: string; href: string } | undefined;
};

type MediaCellProps = {
  item: MediaItem;
  revealed: boolean;
  count: 1 | 2 | 3 | 4;
  noDescriptionLabel: string;
  audioLabels: AudioPlayerLabels;
  /** Opens the viewer at this picture; undefined when the picture does not open. */
  onOpen?: (() => void) | undefined;
  /** Whether the mark of the left-out attachments covers the cell's top-end corner, where a video shows its length. */
  underMore?: boolean;
};

const reducedMotionQuery = "(prefers-reduced-motion: reduce)";

function subscribeReducedMotion(onChange: () => void): () => void {
  if (typeof window.matchMedia !== "function") return () => undefined;
  const query = window.matchMedia(reducedMotionQuery);
  query.addEventListener("change", onChange);
  return () => {
    query.removeEventListener("change", onChange);
  };
}

/** Whether the system asks for reduced motion, rendering again when that changes. */
function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(
    subscribeReducedMotion,
    () => typeof window.matchMedia === "function" && window.matchMedia(reducedMotionQuery).matches,
    () => false,
  );
}

/**
 * A cell that plays a video or audio file once shown: the blur of its poster while it is concealed or loading, then a
 * `<video>` with its poster, or the poster with an `<audio>` over its bottom edge. The player is named by the
 * description, or by `noDescriptionLabel` when there is none.
 */
function PlaybackCell({
  item,
  playback,
  revealed,
  count,
  noDescriptionLabel,
  audioLabels,
  underMore = false,
}: MediaCellProps & { playback: MediaPlayback }) {
  const [loaded, setLoaded] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [length, setLength] = useState(0);
  const { locale } = useLocale();
  const autoplay = useAnimationsPlay();
  const reducedMotion = usePrefersReducedMotion();
  const slots = mediaGrid({ count, revealed });
  const name = item.alt ?? noDescriptionLabel;
  const loops = playback.kind === "gifv";
  const playsByItself = loops && autoplay && !reducedMotion;
  const showsPoster = playback.kind === "audio" && item.src !== undefined;
  return (
    <div className={slots.cell()}>
      {item.blurhash !== undefined && !(revealed && (loaded || playback.kind === "audio")) && (
        <Blurhash hash={item.blurhash} className={slots.blur()} />
      )}
      {revealed && (
        <>
          {playback.kind === "audio" ? (
            <>
              {showsPoster && (
                <Picture
                  sources={item.sources ?? { still: [{ url: item.src ?? "" }] }}
                  sizes={count === 1 ? "(min-width: 40rem) 40rem, 100vw" : "(min-width: 40rem) 20rem, 50vw"}
                  // The audio player carries the description; the poster adds nothing for a screen reader.
                  alt=""
                  loading="lazy"
                  decoding="async"
                  className={slots.image()}
                />
              )}
              <AudioPlayer
                src={playback.url}
                name={name}
                lang={item.alt === null ? undefined : item.lang}
                labels={audioLabels}
                className={slots.audio()}
              />
            </>
          ) : (
            <>
              <video
                // A new source or a change of autoplay starts the element again.
                key={playsByItself ? "loop" : "still"}
                src={playback.url}
                poster={item.src}
                controls={!playsByItself}
                autoPlay={playsByItself}
                muted={loops}
                loop={loops}
                playsInline
                preload={playsByItself ? "auto" : "metadata"}
                aria-label={name}
                onLoadedData={() => {
                  setLoaded(true);
                }}
                onLoadedMetadata={(event) => {
                  setLength(event.currentTarget.duration);
                }}
                onPlay={() => {
                  setPlaying(true);
                }}
                onPause={() => {
                  setPlaying(false);
                }}
                className={slots.video()}
              />
              {!playing && !playsByItself && (
                <span aria-hidden="true" className={slots.disc()}>
                  <PlayIcon />
                </span>
              )}
              {!playing && !loops && !underMore && Number.isFinite(length) && length > 0 && (
                <span aria-hidden="true" className={slots.length()}>
                  {formatClock(locale, length)}
                </span>
              )}
            </>
          )}
          {item.alt !== null && (
            <span aria-hidden="true" className={slots.badge()}>
              <AltTextIcon />
            </span>
          )}
        </>
      )}
    </div>
  );
}

/**
 * One cell of the grid: the blur of the picture (its blurhash) while it is concealed or loading, and once shown,
 * the picture over it. The blur goes once the picture has loaded, so that a transparent picture shows the surface
 * behind it, and stays should the picture fail to load.
 */
function MediaCell({ item, revealed, count, noDescriptionLabel, onOpen }: MediaCellProps) {
  const [loaded, setLoaded] = useState(false);
  const slots = mediaGrid({ count, revealed });
  const picture = (
    <Picture
      sources={item.sources ?? { still: [{ url: item.src ?? "" }] }}
      // A post is at most about 40rem wide; two or more pictures share it in two columns.
      sizes={count === 1 ? "(min-width: 40rem) 40rem, 100vw" : "(min-width: 40rem) 20rem, 50vw"}
      alt={item.alt ?? noDescriptionLabel}
      // The description is user content (web-client/README.md Y10); the fallback is the app's own text.
      {...(item.alt !== null && { dir: "auto", lang: item.lang })}
      loading="lazy"
      decoding="async"
      onLoad={() => {
        setLoaded(true);
      }}
      className={slots.image()}
    />
  );
  return (
    <div className={slots.cell()}>
      {item.blurhash !== undefined && !(revealed && loaded) && (
        <Blurhash hash={item.blurhash} className={slots.blur()} />
      )}
      {revealed && (
        <>
          {onOpen === undefined ? (
            picture
          ) : (
            // Named by the picture inside it; `aria-haspopup` says that it opens a dialog.
            <Button aria-haspopup="dialog" onPress={onOpen} className={slots.open()}>
              {picture}
            </Button>
          )}
          {item.alt !== null && (
            <span aria-hidden="true" className={slots.badge()}>
              <AltTextIcon />
            </span>
          )}
        </>
      )}
    </div>
  );
}

/**
 * The pictures, videos, and audio files of a post in a 16:9 grid of one to four cells. A video plays in
 * its cell with the browser's controls under a play disc and its length, an audio file in an `AudioPlayer`, and neither starts by itself (`MediaPlayback`). An item without a description is
 * named by `noDescriptionLabel` for screen readers only, and one with a description carries a small note badge (web-client/README.md Y9). Sensitive media are
 * concealed behind a toggle button (`aria-pressed`): while concealed, the images are neither loaded nor
 * exposed and each cell shows the blur of its picture, and the button keeps its place in the tab order and its focus when it is pressed.
 * With `viewerLabels`, each shown picture that has an `original` is a button that opens it at its original size in a
 * modal viewer (`MediaViewer`), which steps through the post's other pictures; a concealed picture does not open.
 * With `more`, a mark in the top-end corner says how many attachments the grid leaves out and links to the post that has them.
 */
export function MediaGrid({
  items,
  noDescriptionLabel,
  sensitive = false,
  revealLabel,
  isRevealed,
  defaultRevealed = false,
  onRevealedChange,
  viewerLabels,
  audioLabels,
  more,
  className,
  ...props
}: MediaGridProps) {
  const [uncontrolled, setUncontrolled] = useState(defaultRevealed);
  // The picture the viewer shows; it stays set while the viewer closes, so the closing panel keeps its picture.
  const [viewer, setViewer] = useState<{ isOpen: boolean; index: number }>({ isOpen: false, index: 0 });
  const revealed = !sensitive || (isRevealed ?? uncontrolled);
  const shown = items.slice(0, 4);
  const count = Math.min(4, Math.max(1, shown.length)) as 1 | 2 | 3 | 4;
  const slots = mediaGrid({ count, revealed });
  // The viewer steps through the shown pictures that have an original.
  const viewable =
    viewerLabels === undefined
      ? []
      : shown.flatMap((item) =>
          item.original === undefined || item.playback !== undefined
            ? []
            : [{ ...item.original, id: item.id, alt: item.alt, lang: item.lang }],
        );
  const openerOf = (index: number) =>
    index < 0
      ? undefined
      : () => {
          setViewer({ isOpen: true, index });
        };

  return (
    <div {...props} className={slots.base({ className })}>
      {shown.map((item, index) =>
        item.playback === undefined ? (
          <MediaCell
            key={item.id}
            item={item}
            revealed={revealed}
            count={count}
            noDescriptionLabel={noDescriptionLabel}
            audioLabels={audioLabels}
            onOpen={openerOf(viewable.findIndex((viewed) => viewed.id === item.id))}
          />
        ) : (
          <PlaybackCell
            key={item.id}
            item={item}
            playback={item.playback}
            revealed={revealed}
            count={count}
            noDescriptionLabel={noDescriptionLabel}
            audioLabels={audioLabels}
            // The top-end cell: the second of two or more, or the only one.
            underMore={more !== undefined && index === Math.min(1, shown.length - 1)}
          />
        ),
      )}
      {more !== undefined && (
        <ExternalLink href={more.href} aria-label={more.label} variant="standalone" className={slots.more()}>
          {/* Isolated, so that "+2" keeps its sign before the number in right-to-left text. */}
          <bdi>{more.text}</bdi>
        </ExternalLink>
      )}
      {sensitive && (
        // Once the media are shown it is only an icon, which shows its name as a tooltip (G12).
        <TooltipTrigger isDisabled={!revealed}>
          <ToggleButton
            isSelected={revealed}
            onChange={(next) => {
              setUncontrolled(next);
              onRevealedChange?.(next);
            }}
            className={slots.toggle()}
          >
            {revealed ? (
              <>
                <HideMediaIcon />
                <VisuallyHidden>{revealLabel}</VisuallyHidden>
              </>
            ) : (
              revealLabel
            )}
          </ToggleButton>
          <Tooltip>{revealLabel}</Tooltip>
        </TooltipTrigger>
      )}
      {viewerLabels !== undefined && viewable.length > 0 && (
        <MediaViewer
          items={viewable}
          index={Math.min(viewer.index, viewable.length - 1)}
          isOpen={viewer.isOpen && revealed}
          onIndexChange={(index) => {
            setViewer({ isOpen: true, index });
          }}
          onOpenChange={(isOpen) => {
            setViewer((current) => ({ ...current, isOpen }));
          }}
          labels={viewerLabels}
          noDescriptionLabel={noDescriptionLabel}
        />
      )}
    </div>
  );
}
