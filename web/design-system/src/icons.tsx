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
  IconBell,
  IconBellFilled,
  IconBookmark,
  IconBookmarkFilled,
  IconBuildingCommunity,
  IconDots,
  IconExternalLink,
  IconEyeOff,
  IconFlag,
  IconHome,
  IconHomeFilled,
  IconInfoCircle,
  IconLanguage,
  IconLink,
  IconLock,
  IconMessageCircle,
  IconMicrophone,
  IconMoodPlus,
  IconMoodSmile,
  IconMoon,
  IconPaperclip,
  IconPencil,
  IconPencilPlus,
  IconQuote,
  IconRepeat,
  IconSearch,
  IconSearchFilled,
  IconSettings,
  IconSettingsFilled,
  IconShare2,
  IconSpeakerphone,
  IconStar,
  IconStarFilled,
  IconTrash,
  IconUser,
  IconUserFilled,
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
  // Seen on this server alone: its community, not the world.
  local: { off: IconBuildingCommunity },
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
  // The places of the navigation, on where the reader is, and the entry to write a post.
  home: { off: IconHome, on: IconHomeFilled },
  notifications: { off: IconBell, on: IconBellFilled },
  search: { off: IconSearch, on: IconSearchFilled },
  profile: { off: IconUser, on: IconUserFilled },
  settings: { off: IconSettings, on: IconSettingsFilled },
  compose: { off: IconPencilPlus },
  quote: { off: IconQuote },
  react: { off: IconMoodPlus },
  translate: { off: IconLanguage },
  // Places with no item in the navigation: a voice chat room, the server's announcements, and the server's own page.
  room: { off: IconMicrophone },
  announcements: { off: IconSpeakerphone },
  about: { off: IconInfoCircle },
  // What a reader chose not to see yet: a content warning's fold. An eye crossed out, not a flag, which reads as report.
  hidden: { off: IconEyeOff },
} as const satisfies Record<string, Entry>;

export type IconName = keyof typeof icons;

/** Where an icon sits, which gives its size: in an action of a timeline, beside a caption, or in the navigation. */
export type IconPlace = "action" | "caption" | "navigation";

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
      navigation: { glyph: "size-icon-navigation", tile: "size-icon-navigation" },
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
