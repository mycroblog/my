// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer: a post's pictures and videos, one at a time and whole, over the page (a modal, modal.tsx), always in
// the dark scheme on a near-black backing, so that a picture is seen as it is in either scheme. Its bar holds where the
// reader is among the post's media at its start, and at its end zoom and close (close last, as every modal's). The
// picture fits the room left and keeps its shape; it zooms by the zoom button, a double click or tap, the wheel, a
// pinch, or + and -, up to `viewer.maxZoom`, and pans by dragging while zoomed. The reader moves between the media by
// the arrow keys, which follow the reading direction, by a swipe while not zoomed, or by the buttons at the stage's
// sides from `tablet` (a phone swipes). A video plays with the browser's controls; a GIF loops, muted. Escape, close,
// a press on the backing around the picture, or a swipe down while not zoomed (swipe-close.tsx) closes it, and focus
// goes back to what opened it (the caller's, as with every modal). A swipe down moves the picture and its description
// together, and the backing fades by the share it gives up, its bar and its side buttons with it. A move is announced to a screen reader: where the reader is, and the start of the description, or the
// kind without one.
//
// The description (alternative text) of what is shown sits under the picture at all times, as plain text, and is read
// there alone, the picture carrying no name of its own beside it; the two one
// group in the middle of the room: in a column of a reading width, which scrolls in its own area past a third of the
// screen, so that a long one never leaves the picture a sliver. The words are the design system's own, in the reader's
// locale.

import { swipeClose, viewer as tokens } from "@my/design-tokens";
import { m, useReducedMotion } from "motion/react";
import {
  useEffect,
  useRef,
  useState,
  type HTMLAttributes,
  type KeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type WheelEvent as ReactWheelEvent,
} from "react";
import { Dialog } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { IconButton } from "./icon-button";
import { Box, Grid, Pin, Row } from "./layout";
import { stringsIn, useReader } from "./locale";
import type { MediaItem } from "./media";
import { modalMotion, ModalLayer, swipeExit } from "./modal";
import {
  remOf,
  swipeAtRest,
  swipeBackingStyle,
  swipeGesture,
  swipeSurfaceStyle,
  type SwipeState,
  type SwipeStyle,
} from "./swipe-close";

const view = tv({
  slots: {
    panel: "box-border font-sans text-body text-text outline-hidden [color-scheme:dark]",
    // The backing, a layer of its own under the bar and the room, which a swipe fades.
    scrim: "bg-viewer-scrim",
    bar: "h-viewer-bar px-viewer-inset",
    position: "text-caption text-muted-text tabular-nums",
    stage: "box-border px-viewer-inset select-none",
    picture: "object-contain will-change-transform",
    // Plain text at a reading width; a long one scrolls in its own area, at most a third of the screen, so that the
    // picture keeps the rest.
    caption: [
      "box-border px-viewer-inset text-body whitespace-pre-line rounded-control outline-hidden",
      "focus-visible:shadow-focus",
    ],
  },
  variants: {
    zoomed: { true: { picture: "cursor-grab active:cursor-grabbing" }, false: { picture: "cursor-zoom-in" } },
  },
});

/** What a swipe moves or fades in the viewer: its style, and how it leaves when let go to close. */
type Swiped = {
  style: SwipeStyle;
  // Variants from the start, so that the modal's leave reaches them (Motion adds a node to the tree of variants as it
  // mounts): nothing to do unless let go to close.
  variants: { hidden: ReturnType<typeof swipeExit> | typeof modalMotion.backdrop.hidden | Record<string, never> };
};

/**
 * The room under the bar, where the picture and its description stand as one group in the middle (`surface`, which a
 * swipe moves): the picture's row is as tall as the picture at the full width, and gives up height first; the room
 * left goes above and below the pair. Its foot keeps the space the bar keeps above its buttons. The whole room takes a
 * swipe, a pinch and a drag, the room around a small picture as well as the picture. It holds what is placed against
 * its sides (`sides`, `ViewerSide`).
 */
function ViewerRoom({
  described,
  surface,
  sides,
  children,
  ...rest
}: { described: boolean; surface: Swiped; sides: ReactNode; children: ReactNode } & HTMLAttributes<HTMLDivElement>) {
  const rows = described ? "grid-rows-[minmax(0,auto)_auto]" : "grid-rows-[minmax(0,auto)]";
  return (
    <div className="relative min-h-0 overflow-hidden touch-none" {...rest}>
      <m.div
        data-viewer-backing=""
        className={`box-border grid size-full grid-cols-[minmax(0,1fr)] content-center justify-items-center gap-viewer-inset pb-viewer-foot ${rows}`}
        {...surface}
      >
        {children}
      </m.div>
      {sides}
    </div>
  );
}

/**
 * A button held against a side of the room, halfway down it, on a backing: from `tablet` alone, where the viewport has
 * the room for it (a phone swipes). One of the frames of a page: it reads the viewport.
 */
function ViewerSide({ place, backing, children }: { place: "start" | "end"; backing: Swiped; children: ReactNode }) {
  const side = place === "start" ? "start-viewer-inset" : "end-viewer-inset";
  return (
    <m.span
      className={`absolute top-1/2 -translate-y-1/2 rounded-full bg-media-badge max-tablet:hidden [&>button]:h-icon-button-touch [&>button]:px-viewer-side-inline [&>button]:text-on-media ${side}`}
      {...backing}
    >
      {children}
    </m.span>
  );
}

export type MediaViewerProps = {
  /** The post's pictures and videos, in its order. */
  items: readonly MediaItem[];
  /** The one shown. */
  index: number;
  /** The reader moved to another. */
  onIndexChange: (index: number) => void;
  /** Close, Escape, a press on the backing, or a swipe down. */
  onClose: () => void;
  /** Over the page, as the app opens it, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  /** The reader's locale; the I18nProvider's by default. */
  locale?: string | undefined;
};

const clamp = (z: number) => Math.min(tokens.maxZoom, Math.max(1, z));

// How far a pointer may move between down and up and still be a press (on the backing, which closes), in rem: as far
// as a swipe goes before it is told from a press.
const pressSlop = swipeClose.slop;

// How much of a description a move announces: its start, so that a long one does not hold the reader up.
const announcedLength = 100;

/** The start of a description, whole characters, cut short with … when long. */
function startOf(description: string): string {
  const chars = [...new Intl.Segmenter().segment(description)].map((g) => g.segment);
  return chars.length <= announcedLength ? description : `${chars.slice(0, announcedLength).join("").trimEnd()}…`;
}

type Point = { x: number; y: number };

/** The longest side of the box a picture of unknown size takes from its preview's shape: past any screen, so it fits. */
const unknownSide = 4096;

/** A transparent picture of `width` × `height` px, which takes the box the original will take before it comes. */
const sizer = (width: number, height: number) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="${String(width)}" height="${String(height)}"/>`)}`;

/**
 * A picture, whole in the room from the start: its original once it has come; until then, and when it fails, its
 * preview (which the post showed, so cached) in the box the original takes, by the original's size, else by the
 * preview's shape at a size past any screen. Its box never waits on the original to know its size.
 */
function Picture({
  item,
  className,
  style,
  onDoubleClick,
}: {
  item: MediaItem;
  /** The box's classes, which the layout part that holds it gives. */
  className?: string | undefined;
  style: { transform: string };
  onDoubleClick: () => void;
}) {
  const original = item.href ?? item.src;
  const [come, setCome] = useState(false);
  const [shape, setShape] = useState<{ width: number; height: number } | undefined>(undefined);
  useEffect(() => {
    let live = true;
    const image = new Image();
    image.src = original;
    image
      .decode()
      .then(() => {
        if (live) setCome(true);
      })
      // A failed original leaves the preview in its place.
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [original]);
  const common = { alt: "", draggable: false, className, style, onDoubleClick };
  if (come || original === item.src) return <img src={original} {...common} />;
  const size =
    item.width !== undefined && item.height !== undefined && item.width > 0 && item.height > 0
      ? { width: item.width, height: item.height }
      : shape;
  if (size === undefined)
    // The preview's own shape, read once it loads, sizes the box the original takes.
    return (
      <img
        src={item.src}
        {...common}
        onLoad={(e) => {
          const { naturalWidth: w, naturalHeight: h } = e.currentTarget;
          if (w > 0 && h > 0) {
            const k = unknownSide / Math.max(w, h);
            setShape({ width: Math.round(w * k), height: Math.round(h * k) });
          }
        }}
      />
    );
  return (
    <img
      src={sizer(size.width, size.height)}
      {...common}
      style={{
        ...style,
        backgroundImage: `url(${JSON.stringify(item.src)})`,
        backgroundSize: "contain",
        backgroundRepeat: "no-repeat",
        backgroundPosition: "center",
      }}
    />
  );
}

/** What is shown, at its zoom and pan; a double click or tap zooms it in or out. */
function Stage({
  item,
  zoom,
  pan,
  onToggleZoom,
}: {
  item: MediaItem;
  zoom: number;
  pan: Point;
  onToggleZoom: () => void;
}) {
  const s = view({ zoomed: zoom > 1 });
  const kind = item.kind ?? "image";
  // The description is the caption under the picture, read there: the picture itself is not named by it again.
  const element =
    kind === "video" ? (
      <video src={item.href} poster={item.src} controls autoPlay playsInline />
    ) : kind === "gifv" ? (
      <video src={item.href} poster={item.src} muted loop autoPlay playsInline />
    ) : undefined;
  return (
    <Row full="both" shrink="block" align="center" justify="center" className={s.stage()} data-viewer-stage="">
      <Box
        as={
          element ?? (
            <Picture
              item={item}
              style={{ transform: `scale(${String(zoom)}) translate(${String(pan.x)}px, ${String(pan.y)}px)` }}
              onDoubleClick={onToggleZoom}
            />
          )
        }
        display="block"
        max="contain"
        className={s.picture()}
      />
    </Row>
  );
}

export function MediaViewer({ items, index, onIndexChange, onClose, layer = true, locale }: MediaViewerProps) {
  const { lang, direction } = useReader(locale);
  const { t } = stringsIn(lang);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState<Point>({ x: 0, y: 0 });
  // The pointers down on the room, and where the gesture started; `pinch` once two pointers were down in it, which
  // makes it never a swipe nor a press; `backing` while the one pointer down is on the backing around what is shown.
  const pointers = useRef(new Map<number, Point>());
  // A press on the backing ended: the click it makes closes the viewer, so that no click lands on the page under it.
  const pressedBacking = useRef(false);
  const start = useRef<{
    x: number;
    y: number;
    pan: Point;
    spread: number;
    zoom: number;
    pinch: boolean;
    backing: boolean;
  } | null>(null);
  // What a screen reader hears on a move: empty until the reader moves, so that opening announces nothing twice.
  const [moved, setMoved] = useState(false);
  // A swipe down that closes, over the page alone (`layer`), told from the one pointer down while not zoomed.
  const [swipe] = useState(swipeGesture);
  const [at, setAt] = useState<SwipeState>(swipeAtRest);
  const reduced = useReducedMotion() ?? false;
  const item = items[index];
  if (item === undefined) return null;
  const description = item.alt;
  const s = view();
  const many = items.length > 1;
  const kind = item.kind ?? "image";
  const isImage = kind === "image";
  const zoomTo = (z: number) => {
    const next = clamp(z);
    setZoom(next);
    if (next === 1) setPan({ x: 0, y: 0 });
  };
  const go = (to: number) => {
    if (to < 0 || to >= items.length) return;
    zoomTo(1);
    setMoved(true);
    onIndexChange(to);
  };
  // A gesture starts on the room or the picture, never on a button, the description (which scrolls and selects as
  // text), or a video (whose controls take the pointer).
  const gesture = (e: ReactPointerEvent | ReactWheelEvent) => {
    const target = e.target as Element;
    if (target instanceof HTMLVideoElement && kind === "video") return false;
    return (
      target === e.currentTarget ||
      target.hasAttribute("data-viewer-backing") ||
      target.closest("[data-viewer-stage]") !== null
    );
  };
  const spread = () => {
    const [a, b] = [...pointers.current.values()];
    return a && b ? Math.hypot(a.x - b.x, a.y - b.y) : 0;
  };
  // The backing: the room itself, or the stage around the picture, not the picture or the video.
  const onBacking = (e: ReactPointerEvent) => {
    const target = e.target as Element;
    return (
      target === e.currentTarget ||
      target.hasAttribute("data-viewer-stage") ||
      target.hasAttribute("data-viewer-backing")
    );
  };
  const springBack = () => {
    if (at.phase === "drag") setAt({ phase: "back", down: at.down });
  };
  const down = (e: ReactPointerEvent) => {
    pressedBacking.current = false;
    if (!gesture(e)) return;
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    e.currentTarget.setPointerCapture(e.pointerId);
    const apart = spread();
    const backing = apart === 0 && onBacking(e);
    start.current = { x: e.clientX, y: e.clientY, pan, spread: apart, zoom, pinch: apart > 0, backing };
    // One pointer while not zoomed may swipe; a second ends the swipe, which springs back.
    const rem = remOf(e.currentTarget);
    if (pointers.current.size === 1 && zoom === 1 && layer)
      swipe.start(e.clientX / rem, e.clientY / rem, e.timeStamp, 1);
    else {
      swipe.cancel();
      springBack();
    }
  };
  // A pointer left: from two to one, the one left goes on from where it is, as a drag of the pinch's end, never a swipe.
  const lift = (pointerId: number) => {
    if (!pointers.current.delete(pointerId)) return false;
    const [rest, ...others] = [...pointers.current.values()];
    if (rest !== undefined && others.length === 0 && start.current !== null)
      start.current = { x: rest.x, y: rest.y, pan, spread: 0, zoom, pinch: true, backing: false };
    return true;
  };
  const move = (e: ReactPointerEvent) => {
    if (!pointers.current.has(e.pointerId) || start.current === null) return;
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (pointers.current.size === 2 && start.current.spread > 0) {
      zoomTo(start.current.zoom * (spread() / start.current.spread));
      return;
    }
    if (zoom === 1 && pointers.current.size === 1 && !start.current.pinch) {
      const rem = remOf(e.currentTarget);
      const now = swipe.move(e.clientX / rem, e.clientY / rem, e.timeStamp, 1);
      if (now !== undefined) setAt({ phase: "drag", down: now });
      return;
    }
    if (zoom > 1)
      setPan({
        x: start.current.pan.x + (e.clientX - start.current.x) / zoom,
        y: start.current.pan.y + (e.clientY - start.current.y) / zoom,
      });
  };
  const up = (e: ReactPointerEvent) => {
    if (!lift(e.pointerId) || pointers.current.size > 0) return;
    const from = start.current;
    if (from === null) return;
    start.current = null;
    const closes = swipe.end(e.timeStamp);
    if (at.phase === "drag") {
      // A swipe down let go: it closes as close does, or springs back; it is no press and no move.
      if (closes) {
        setAt({ phase: "leave", down: at.down });
        onClose();
      } else springBack();
      return;
    }
    const dx = e.clientX - from.x;
    // A press on the backing, as a press outside any modal, closes the viewer, by the click it makes; a swipe, a drag
    // or a pinch does not.
    pressedBacking.current =
      from.backing && !from.pinch && Math.hypot(dx, e.clientY - from.y) <= pressSlop * remOf(e.currentTarget);
    if (pressedBacking.current) return;
    if (zoom === 1 && !from.pinch && Math.abs(dx) > 48 && Math.abs(dx) > Math.abs(e.clientY - from.y)) {
      // A swipe toward the start of the line brings the next, as a page turns.
      go(index + ((direction === "rtl" ? dx > 0 : dx < 0) ? 1 : -1));
    }
  };
  // A gesture the browser took over moves nothing: its pointer reports no place to measure a swipe from.
  const cancel = (e: ReactPointerEvent) => {
    swipe.cancel();
    springBack();
    lift(e.pointerId);
    if (pointers.current.size === 0) start.current = null;
  };
  const where = { index: index + 1, count: items.length };
  const announced = !moved
    ? ""
    : description === ""
      ? t("viewer.shown_kind", { ...where, kind })
      : t("viewer.shown", { ...where, description: startOf(description) });
  // What the swipe moves (the picture and its description) and what fades with the backing (the scrim, the bar, the
  // side buttons): let go to close, the one goes on down and the others fade out, as the modal leaves.
  const leaving = at.phase === "leave" && !reduced;
  const surface: Swiped = {
    style: swipeSurfaceStyle(at, reduced),
    variants: { hidden: leaving ? swipeExit() : {} },
  };
  const backing: Swiped = {
    style: swipeBackingStyle(at, reduced),
    variants: { hidden: leaving ? modalMotion.backdrop.hidden : {} },
  };
  const panel = (
    <Grid rows="head" anchor full="both" className={s.panel()} dir={direction} lang={lang}>
      <Pin as={<m.div aria-hidden="true" {...backing} />} cover className={s.scrim()} />
      <Row as={<m.div {...backing} />} anchor align="center" gap="entry-caption" className={s.bar()}>
        <Box as="span" grow className={s.position()}>
          {many ? t("viewer.position", where) : ""}
        </Box>
        <span className="sr-only" aria-live="polite">
          {announced}
        </span>
        <Row as="span" align="center" gap="entry-action">
          {isImage && (
            <IconButton
              icon={zoom > 1 ? "zoomOut" : "zoomIn"}
              label={t(zoom > 1 ? "viewer.zoom_out" : "viewer.zoom_in")}
              onPress={() => {
                zoomTo(zoom > 1 ? 1 : 2);
              }}
            />
          )}
          <IconButton icon="close" label={t("viewer.close")} onPress={onClose} />
        </Row>
      </Row>
      <ViewerRoom
        described={description !== ""}
        surface={surface}
        sides={
          <>
            {many && index > 0 && (
              <ViewerSide place="start" backing={backing}>
                <IconButton
                  icon="previous"
                  label={t("viewer.previous")}
                  onPress={() => {
                    go(index - 1);
                  }}
                />
              </ViewerSide>
            )}
            {many && index < items.length - 1 && (
              <ViewerSide place="end" backing={backing}>
                <IconButton
                  icon="onward"
                  label={t("viewer.next")}
                  onPress={() => {
                    go(index + 1);
                  }}
                />
              </ViewerSide>
            )}
          </>
        }
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={cancel}
        onClick={() => {
          if (!pressedBacking.current) return;
          pressedBacking.current = false;
          onClose();
        }}
        onWheel={(e) => {
          if (!isImage || !gesture(e)) return;
          zoomTo(zoom + (e.deltaY < 0 ? tokens.zoomStep : -tokens.zoomStep));
        }}
      >
        <Stage
          key={item.id}
          item={item}
          zoom={zoom}
          pan={pan}
          onToggleZoom={() => {
            zoomTo(zoom > 1 ? 1 : 2);
          }}
        />
        {description !== "" && (
          // A long description scrolls, so it takes the focus that scrolling by keys needs. Keyed apart from the stage,
          // its sibling, so that a move replaces each.
          <Box
            as="p"
            key={`caption:${item.id}`}
            full
            max="reading"
            scroll="y"
            className={s.caption()}
            dir="auto"
            tabIndex={0}
          >
            {description}
          </Box>
        )}
      </ViewerRoom>
    </Grid>
  );
  // The keys act wherever the focus is in the viewer, the dialog itself included (where it starts), except in a video,
  // whose controls take the arrows for seeking.
  const keys = (e: KeyboardEvent) => {
    if (e.target instanceof HTMLMediaElement) return;
    const next = direction === "rtl" ? "ArrowLeft" : "ArrowRight";
    const previous = direction === "rtl" ? "ArrowRight" : "ArrowLeft";
    if (e.key === next) go(index + 1);
    else if (e.key === previous) go(index - 1);
    else if ((e.key === "+" || e.key === "=") && isImage) zoomTo(zoom + tokens.zoomStep);
    else if (e.key === "-" && isImage) zoomTo(zoom - tokens.zoomStep);
    else return;
    e.preventDefault();
  };
  if (!layer)
    return (
      <Box anchor role="dialog" aria-label={t("viewer.label")} className="h-viewer-specimen" onKeyDown={keys}>
        {panel}
      </Box>
    );
  return (
    <ModalLayer onDismiss={onClose} place="cover" swipe={{ at, moves: "inside" }}>
      <div className="contents" onKeyDown={keys}>
        <Box as={<Dialog aria-label={t("viewer.label")} className="outline-hidden" />} full="both">
          {panel}
        </Box>
      </div>
    </ModalLayer>
  );
}
