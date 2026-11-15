// SPDX-License-Identifier: AGPL-3.0-only

// The icons (ADR-0025 §6): a thin layer from a meaning name to a glyph of Tabler (@tabler/icons-react), the only file
// that imports it. One table gives each meaning its off glyph, its on glyph and whether it bears a direction. Every
// icon is decorative to assistive technology (the control's label or the text beside it names it), a glyph that
// bears a direction mirrors in right-to-left text, and the place an icon sits gives its size. An on state differs from
// the off state by shape: a closed glyph takes Tabler's filled glyph; an open one, which Tabler cannot fill (boost),
// is the glyph knocked out of a solid tile.

import {
  IconActivityHeartbeat,
  IconAlertCircle,
  IconAlertTriangle,
  IconArrowLeft,
  IconAt,
  IconBan,
  IconBell,
  IconBellFilled,
  IconBookmark,
  IconBookmarkFilled,
  IconBrandDiscord,
  IconBrandGithub,
  IconBroadcast,
  IconBuildingCommunity,
  IconCalendarEvent,
  IconCalendarEventFilled,
  IconChartBar,
  IconChecks,
  IconChevronDown,
  IconChevronRight,
  IconClock,
  IconCloudOff,
  IconCoin,
  IconCoinFilled,
  IconDoorExit,
  IconDots,
  IconExternalLink,
  IconEyeOff,
  IconFlag,
  IconHeartHandshake,
  IconHome,
  IconHomeFilled,
  IconInfoCircle,
  IconKey,
  IconLanguage,
  IconLink,
  IconLock,
  IconLogin2,
  IconMenu2,
  IconMessageCircle,
  IconMicrophone,
  IconMicrophoneOff,
  IconMoodPlus,
  IconMoodSmile,
  IconMoon,
  IconPaperclip,
  IconPencil,
  IconPencilPlus,
  IconPin,
  IconPlus,
  IconQuote,
  IconRefresh,
  IconRepeat,
  IconSearch,
  IconSearchFilled,
  IconServer2,
  IconSettings,
  IconSettingsFilled,
  IconShare2,
  IconShield,
  IconShieldFilled,
  IconSpeakerphone,
  IconStar,
  IconStarFilled,
  IconTool,
  IconTrash,
  IconUser,
  IconUserFilled,
  IconUserPlus,
  IconVolumeOff,
  IconWorld,
  IconWorldFilled,
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
  // Seen on this server alone: its community, not the world; on, where the reader is in the navigation.
  local: { off: IconBuildingCommunity, on: "tile" },
  bookmark: { off: IconBookmark, on: IconBookmarkFilled },
  share: { off: IconShare2 },
  attach: { off: IconPaperclip },
  emoji: { off: IconMoodSmile },
  close: { off: IconX },
  back: { off: IconArrowLeft, directional: true },
  // What a row opens, toward the end of the line.
  onward: { off: IconChevronRight, directional: true },
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
  // The timeline of every server this one knows: the world, as the public glyph is, filled where the reader is.
  federated: { off: IconWorld, on: IconWorldFilled },
  // The places only some readers have: the administrators' screens and the server's money (moderation, below).
  admin: { off: IconServer2, on: "tile" },
  finance: { off: IconCoin, on: IconCoinFilled },
  // The tab bar's entry to the sheet of the places it does not hold.
  menu: { off: IconMenu2 },
  // The readers' support of the server (its funding meter), apart from the staff's finance.
  funding: { off: IconHeartHandshake },
  // What is coming on the server: its events.
  events: { off: IconCalendarEvent, on: IconCalendarEventFilled },
  quote: { off: IconQuote },
  // A poll a post carries: bars of its counts; on, while the composer holds one.
  poll: { off: IconChartBar, on: "tile" },
  // One more of a list (a poll's option).
  add: { off: IconPlus },
  // How long something runs (a poll's duration).
  time: { off: IconClock },
  // A control that opens a list of choices (a poll's duration).
  expand: { off: IconChevronDown },
  react: { off: IconMoodPlus },
  translate: { off: IconLanguage },
  // Places with no item in the navigation: a voice chat room, the server's announcements, and the server's own page.
  room: { off: IconMicrophone },
  announcements: { off: IconSpeakerphone },
  about: { off: IconInfoCircle },
  // What a reader chose not to see yet: a content warning's fold. An eye crossed out, not a flag, which reads as report.
  // On, while the composer holds a content warning.
  hidden: { off: IconEyeOff, on: "tile" },
  // The kinds of a notification with no action of their own (the rough of #3047): a follow, an ended poll (poll,
  // above), the moderators' notice; and a post its author pinned to their profile.
  follow: { off: IconUserPlus, directional: true },
  moderation: { off: IconShield, on: IconShieldFilled },
  pin: { off: IconPin },
  // Marking every notification read: two checks.
  read: { off: IconChecks },
  // The auth screens: what went wrong (a round mark), what needs care before agreeing (a triangle), a passkey, and the
  // marks of the login providers, in the text's color like every glyph.
  error: { off: IconAlertCircle },
  caution: { off: IconAlertTriangle },
  passkey: { off: IconKey },
  // The frame's notices: the server out of reach, the server down for maintenance, the server's status page, and a
  // newer version of the app.
  offline: { off: IconCloudOff },
  maintenance: { off: IconTool },
  status: { off: IconActivityHeartbeat },
  update: { off: IconRefresh },
  // The call's bar: the call going on (waves from a point, apart from the microphone), the reader's microphone (on,
  // muted: crossed out), and leaving the room by its door.
  live: { off: IconBroadcast },
  microphone: { off: IconMicrophone, on: IconMicrophoneOff },
  leave: { off: IconDoorExit, directional: true },
  // Signing in: an arrow into a door.
  signIn: { off: IconLogin2, directional: true },
  github: { off: IconBrandGithub },
  discord: { off: IconBrandDiscord },
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
