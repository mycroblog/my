// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer: a post's pictures and videos, one at a time and whole, over the page (a modal, modal.tsx), always in
// the dark scheme on a near-black backing, so that a picture is seen as it is in either scheme. Its bar holds where the
// reader is among the post's media at its start, and at its end zoom and close (close last, as every modal's). The
// picture fits the room left and keeps its shape; it zooms by the zoom button, a double click or tap, the wheel, a
// pinch, or + and -, up to `viewer.maxZoom`, and pans by dragging while zoomed. The reader moves between the media by
// the arrow keys, which follow the reading direction, by a swipe while not zoomed, or by the buttons at the stage's
// sides from `tablet` (a phone swipes). A video plays with the browser's controls; a GIF loops, muted. Escape or close
// closes it, and focus goes back to what opened it (the caller's, as with every modal). A move is announced to a screen
// reader: where the reader is, and the start of the description, or the kind without one.
//
// The description (alternative text) of what is shown sits under the picture at all times, as plain text, and is read
// there alone, the picture carrying no name of its own beside it; the two one
// group in the middle of the room: in a column of a reading width, which scrolls in its own area past a third of the
// screen, so that a long one never leaves the picture a sliver. The words are the design system's own, in the reader's
// locale.

import { viewer as tokens } from "@my/design-tokens";
import {
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
import { Box, Grid, Row } from "./layout";
import { stringsIn, useReader } from "./locale";
import type { MediaItem } from "./media";
import { ModalLayer } from "./modal";

const view = tv({
  slots: {
    panel: "box-border bg-viewer-scrim font-sans text-body text-text outline-hidden [color-scheme:dark]",
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

/**
 * The room under the bar, where the picture and its description stand as one group in the middle: the picture's row is
 * as tall as the picture at the full width, and gives up height first; the room left goes above and below the pair.
 * Its foot keeps the space the bar keeps above its buttons. The whole room takes a swipe, a pinch and a drag, the room
 * around a small picture as well as the picture. It holds what is placed against its sides (`ViewerSide`).
 */
function ViewerRoom({
  described,
  children,
  ...rest
}: { described: boolean; children: ReactNode } & HTMLAttributes<HTMLDivElement>) {
  const rows = described ? "grid-rows-[minmax(0,auto)_auto]" : "grid-rows-[minmax(0,auto)]";
  return (
    <div
      className={`relative grid min-h-0 grid-cols-[minmax(0,1fr)] content-center justify-items-center gap-viewer-inset overflow-hidden touch-none pb-viewer-foot ${rows}`}
      {...rest}
    >
      {children}
    </div>
  );
}

/**
 * A button held against a side of the room, halfway down it, on a backing: from `tablet` alone, where the viewport has
 * the room for it (a phone swipes). One of the frames of a page: it reads the viewport.
 */
function ViewerSide({ place, children }: { place: "start" | "end"; children: ReactNode }) {
  const side = place === "start" ? "start-viewer-inset" : "end-viewer-inset";
  return (
    <span
      className={`absolute top-1/2 -translate-y-1/2 rounded-full bg-media-badge max-tablet:hidden [&>button]:h-icon-button-touch [&>button]:px-viewer-side-inline [&>button]:text-on-media ${side}`}
    >
      {children}
    </span>
  );
}

export type MediaViewerProps = {
  /** The post's pictures and videos, in its order. */
  items: readonly MediaItem[];
  /** The one shown. */
  index: number;
  /** The reader moved to another. */
  onIndexChange: (index: number) => void;
  /** Close, Escape, or a press on the backing. */
  onClose: () => void;
  /** Over the page, as the app opens it, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  /** The reader's locale; the I18nProvider's by default. */
  locale?: string | undefined;
};

const clamp = (z: number) => Math.min(tokens.maxZoom, Math.max(1, z));

// How much of a description a move announces: its start, so that a long one does not hold the reader up.
const announcedLength = 100;

/** The start of a description, whole characters, cut short with … when long. */
function startOf(description: string): string {
  const chars = [...new Intl.Segmenter().segment(description)].map((g) => g.segment);
  return chars.length <= announcedLength ? description : `${chars.slice(0, announcedLength).join("").trimEnd()}…`;
}

type Point = { x: number; y: number };

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
    ) : (
      <img
        src={item.href ?? item.src}
        alt=""
        draggable={false}
        style={{ transform: `scale(${zoom}) translate(${pan.x}px, ${pan.y}px)` }}
        onDoubleClick={onToggleZoom}
      />
    );
  return (
    <Row full="both" shrink="block" align="center" justify="center" className={s.stage()} data-viewer-stage="">
      <Box as={element} display="block" max="contain" className={s.picture()} />
    </Row>
  );
}

export function MediaViewer({ items, index, onIndexChange, onClose, layer = true, locale }: MediaViewerProps) {
  const { lang, direction } = useReader(locale);
  const { t } = stringsIn(lang);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState<Point>({ x: 0, y: 0 });
  // The pointers down on the room, and where the gesture started; `pinch` once two pointers were down in it, which
  // makes it never a swipe.
  const pointers = useRef(new Map<number, Point>());
  const start = useRef<{ x: number; y: number; pan: Point; spread: number; zoom: number; pinch: boolean } | null>(null);
  // What a screen reader hears on a move: empty until the reader moves, so that opening announces nothing twice.
  const [moved, setMoved] = useState(false);
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
    return target === e.currentTarget || target.closest("[data-viewer-stage]") !== null;
  };
  const spread = () => {
    const [a, b] = [...pointers.current.values()];
    return a && b ? Math.hypot(a.x - b.x, a.y - b.y) : 0;
  };
  const down = (e: ReactPointerEvent) => {
    if (!gesture(e)) return;
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    e.currentTarget.setPointerCapture(e.pointerId);
    const apart = spread();
    start.current = { x: e.clientX, y: e.clientY, pan, spread: apart, zoom, pinch: apart > 0 };
  };
  // A pointer left: from two to one, the one left goes on from where it is, as a drag of the pinch's end, never a swipe.
  const lift = (pointerId: number) => {
    if (!pointers.current.delete(pointerId)) return false;
    const [rest, ...others] = [...pointers.current.values()];
    if (rest !== undefined && others.length === 0 && start.current !== null)
      start.current = { x: rest.x, y: rest.y, pan, spread: 0, zoom, pinch: true };
    return true;
  };
  const move = (e: ReactPointerEvent) => {
    if (!pointers.current.has(e.pointerId) || start.current === null) return;
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (pointers.current.size === 2 && start.current.spread > 0) {
      zoomTo(start.current.zoom * (spread() / start.current.spread));
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
    const dx = e.clientX - from.x;
    if (zoom === 1 && !from.pinch && Math.abs(dx) > 48 && Math.abs(dx) > Math.abs(e.clientY - from.y)) {
      // A swipe toward the start of the line brings the next, as a page turns.
      go(index + ((direction === "rtl" ? dx > 0 : dx < 0) ? 1 : -1));
    }
  };
  // A gesture the browser took over moves nothing: its pointer reports no place to measure a swipe from.
  const cancel = (e: ReactPointerEvent) => {
    lift(e.pointerId);
    if (pointers.current.size === 0) start.current = null;
  };
  const where = { index: index + 1, count: items.length };
  const announced = !moved
    ? ""
    : description === ""
      ? t("viewer.shown_kind", { ...where, kind })
      : t("viewer.shown", { ...where, description: startOf(description) });
  const panel = (
    <Grid rows="head" anchor full="both" className={s.panel()} dir={direction} lang={lang}>
      <Row align="center" gap="entry-caption" className={s.bar()}>
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
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={cancel}
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
        {many && index > 0 && (
          <ViewerSide place="start">
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
          <ViewerSide place="end">
            <IconButton
              icon="onward"
              label={t("viewer.next")}
              onPress={() => {
                go(index + 1);
              }}
            />
          </ViewerSide>
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
    <ModalLayer onDismiss={onClose} place="cover">
      <div className="contents" onKeyDown={keys}>
        <Box as={<Dialog aria-label={t("viewer.label")} className="outline-hidden" />} full="both">
          {panel}
        </Box>
      </div>
    </ModalLayer>
  );
}
