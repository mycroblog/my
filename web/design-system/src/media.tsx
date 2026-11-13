// SPDX-License-Identifier: AGPL-3.0-only

// The images of an entry (a post's attachments), with no border and the corners of a control. At the full width they
// share one box 16:9 of its width up to a height, so that the next entry shows on the same screen at PC: one alone,
// two side by side, three or four in a grid of two by two. As a preview (in a quoted post), each is a small tile in a
// row, so that what is embedded never outweighs what carries it. Each image's alternative text is its description;
// an image without one is decorative.

import { tv } from "tailwind-variants/lite";

export type MediaItem = {
  /** Where the image is served; a preview's own rendition when there is one. */
  src: string;
  /** The alternative text; empty for none. */
  alt: string;
  /** What tells the item apart from the others (its id). */
  id: string;
};

const grid = tv({
  slots: {
    frame: "@container",
    box: [
      "grid gap-media overflow-hidden rounded-control",
      "h-[min(calc(100cqw*var(--media-ratio)),var(--media-max-height))]",
    ],
    image: "block size-full min-h-0 object-cover",
    row: "flex flex-wrap gap-media",
    tile: "block h-media-preview-height w-media-preview-width rounded-control object-cover",
  },
  variants: {
    columns: { 1: { box: "grid-cols-[minmax(0,1fr)]" }, 2: { box: "grid-cols-[minmax(0,1fr)_minmax(0,1fr)]" } },
    rows: { 1: { box: "grid-rows-[minmax(0,1fr)]" }, 2: { box: "grid-rows-[minmax(0,1fr)_minmax(0,1fr)]" } },
  },
});

export type MediaGridProps = {
  items: readonly MediaItem[];
  /** At the full width (a post's), or as small previews (a quoted post's). */
  size?: "full" | "preview";
};

export function MediaGrid({ items, size = "full" }: MediaGridProps) {
  if (items.length === 0) return null;
  const n = items.length;
  const s = grid({ columns: n === 1 ? 1 : 2, rows: n > 2 ? 2 : 1 });
  if (size === "preview")
    return (
      <div className={s.row()}>
        {items.map((m) => (
          <img key={m.id} className={s.tile()} src={m.src} alt={m.alt} />
        ))}
      </div>
    );
  return (
    <div className={s.frame()}>
      <div className={s.box()}>
        {items.map((m) => (
          <img key={m.id} className={s.image()} src={m.src} alt={m.alt} />
        ))}
      </div>
    </div>
  );
}
