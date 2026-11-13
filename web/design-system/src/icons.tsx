// SPDX-License-Identifier: AGPL-3.0-only

// The icons (ADR-0025 §6): a thin layer from a meaning name to a glyph of Tabler (@tabler/icons-react), the only file
// that imports it. One table gives each meaning its off glyph, its on glyph and whether it bears a direction. Every
// icon is decorative to assistive technology (the control's label or the text beside it names it), a glyph that
// bears a direction mirrors in right-to-left text, and the place an icon sits gives its size. An on state differs from
// the off state by shape: a closed glyph takes Tabler's filled glyph; an open one, which Tabler cannot fill (boost),
// is the glyph knocked out of a solid tile.

import {
  IconArrowLeft,
  IconAt,
  IconBan,
  IconBookmark,
  IconBookmarkFilled,
  IconDots,
  IconExternalLink,
  IconFlag,
  IconLink,
  IconLock,
  IconMessageCircle,
  IconMoodSmile,
  IconMoon,
  IconPaperclip,
  IconPencil,
  IconRepeat,
  IconShare2,
  IconStar,
  IconStarFilled,
  IconTrash,
  IconVolumeOff,
  IconWorld,
  IconX,
  type TablerIcon,
} from "@tabler/icons-react";
import { tv } from "tailwind-variants/lite";

/** How an icon's on state is drawn: another glyph, or the off glyph knocked out of a solid tile. */
type On = TablerIcon | "tile";

type Entry = { readonly off: TablerIcon; readonly on?: On; readonly directional?: boolean };

/** The icons by meaning: the off glyph, the on glyph, and whether the glyph bears a direction. */
export const icons = {
  // A speech bubble, not a U-turn arrow, which reads as undo; its tail points to a side.
  reply: { off: IconMessageCircle, directional: true },
  boost: { off: IconRepeat, on: "tile" },
  favourite: { off: IconStar, on: IconStarFilled },
  more: { off: IconDots },
  public: { off: IconWorld },
  unlisted: { off: IconMoon },
  followers: { off: IconLock },
  direct: { off: IconAt },
  bookmark: { off: IconBookmark, on: IconBookmarkFilled },
  share: { off: IconShare2 },
  attach: { off: IconPaperclip },
  emoji: { off: IconMoodSmile },
  close: { off: IconX },
  back: { off: IconArrowLeft, directional: true },
  link: { off: IconLink },
  // An arrow out of a box, toward the end of the line.
  open: { off: IconExternalLink, directional: true },
  mute: { off: IconVolumeOff },
  block: { off: IconBan },
  report: { off: IconFlag },
  edit: { off: IconPencil },
  delete: { off: IconTrash },
} as const satisfies Record<string, Entry>;

export type IconName = keyof typeof icons;

/** Where an icon sits, which gives its size: in an action of a timeline, or beside a caption. */
export type IconPlace = "action" | "caption";

const icon = tv({
  slots: {
    glyph: "shrink-0",
    tile: "inline-flex shrink-0 items-center justify-center rounded-icon-tile bg-current",
    inner: "size-icon-tile-glyph text-surface",
  },
  variants: {
    place: {
      action: { glyph: "size-icon-action", tile: "size-icon-action" },
      caption: { glyph: "size-icon-caption", tile: "size-icon-caption" },
    },
    directional: { true: { glyph: "rtl:-scale-x-100", inner: "rtl:-scale-x-100" } },
  },
});

// The glyph inside a tile draws a little heavier, so that it keeps its weight at the smaller size.
const tileStroke = 2.5;

function draw(Glyph: TablerIcon, className: string, stroke?: number) {
  return <Glyph aria-hidden="true" focusable="false" className={className} {...(stroke ? { stroke } : {})} />;
}

/** An icon by its meaning, at the size of its place; `on` draws its on state. */
export function Icon({ name, place, on = false }: { name: IconName; place: IconPlace; on?: boolean }) {
  const entry: Entry = icons[name];
  const s = icon({ place, directional: entry.directional ?? false });
  const shape = on ? entry.on : undefined;
  if (shape === "tile")
    return (
      <span aria-hidden="true" className={s.tile()}>
        {draw(entry.off, s.inner(), tileStroke)}
      </span>
    );
  return draw(shape ?? entry.off, s.glyph());
}
