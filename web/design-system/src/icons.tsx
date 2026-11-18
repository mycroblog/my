// SPDX-License-Identifier: AGPL-3.0-only

// The icons (ADR-0025 §6): a thin layer from a meaning name to a glyph of Tabler (@tabler/icons-react), the only file
// that imports it. One table gives each meaning its off glyph, its on glyph and whether it bears a direction. Every
// icon is decorative to assistive technology (the control's label or the text beside it names it), a glyph that
// bears a direction mirrors in right-to-left text, and the place an icon sits gives its size. An on state differs from
// the off state by shape: a closed glyph takes Tabler's filled glyph; an open one, which Tabler cannot fill (boost),
// is the glyph knocked out of a solid tile.

import {
  IconActivityHeartbeat,
  IconAdjustmentsHorizontal,
  IconAlarm,
  IconAlertCircle,
  IconAlertTriangle,
  IconApps,
  IconArchive,
  IconArrowLeft,
  IconArrowMoveRight,
  IconArticle,
  IconAt,
  IconBallFootball,
  IconBan,
  IconBell,
  IconBellFilled,
  IconBellRinging,
  IconBookmark,
  IconBookmarkFilled,
  IconBrandDiscord,
  IconBrandGithub,
  IconBroadcast,
  IconBug,
  IconBuildingCommunity,
  IconBulb,
  IconCalendarBolt,
  IconCalendarCheck,
  IconCalendarCog,
  IconCalendarEvent,
  IconCalendarEventFilled,
  IconCalendarExclamation,
  IconCalendarTime,
  IconCalendarX,
  IconChartBar,
  IconCheck,
  IconChecks,
  IconChevronDown,
  IconChevronLeft,
  IconChevronRight,
  IconCircleCheck,
  IconCircleX,
  IconClock,
  IconCloudOff,
  IconCoin,
  IconCoinFilled,
  IconCreditCardOff,
  IconDatabase,
  IconDeviceFloppy,
  IconDoorExit,
  IconDots,
  IconExternalLink,
  IconEyeOff,
  IconFile,
  IconFilePencil,
  IconFilter,
  IconFlag,
  IconGavel,
  IconHandStop,
  IconHash,
  IconHeart,
  IconHeartBroken,
  IconHeartHandshake,
  IconHistory,
  IconHome,
  IconHomeFilled,
  IconHourglassLow,
  IconInfoCircle,
  IconInfoCircleFilled,
  IconKey,
  IconLanguage,
  IconLink,
  IconLinkOff,
  IconListDetails,
  IconLock,
  IconLogin2,
  IconLogout2,
  IconMail,
  IconMailFilled,
  IconMaximize,
  IconMenu2,
  IconMessageCircle,
  IconMicrophone,
  IconMicrophoneOff,
  IconMoodHappy,
  IconMoodPlus,
  IconMoodSmile,
  IconMoon,
  IconMusic,
  IconNote,
  IconNotes,
  IconPaperclip,
  IconPaw,
  IconPencil,
  IconPencilPlus,
  IconPin,
  IconPlane,
  IconPlayerPauseFilled,
  IconPlayerPlayFilled,
  IconPlugConnectedX,
  IconPlus,
  IconQuote,
  IconRefresh,
  IconRepeat,
  IconSearch,
  IconSearchFilled,
  IconServer2,
  IconServerOff,
  IconSettings,
  IconSettingsFilled,
  IconShare2,
  IconShield,
  IconShieldCheck,
  IconShieldFilled,
  IconSparkles,
  IconSpeakerphone,
  IconSquare,
  IconSquareCheckFilled,
  IconStar,
  IconStarFilled,
  IconTicket,
  IconTool,
  IconToolsKitchen2,
  IconTrash,
  IconUser,
  IconUserFilled,
  IconUserMinus,
  IconUserPlus,
  IconUserQuestion,
  IconUsers,
  IconVolumeOff,
  IconWebhookOff,
  IconWorld,
  IconWorldFilled,
  IconWorldOff,
  IconX,
  IconZoomIn,
  IconZoomOut,
  type TablerIcon,
} from "@tabler/icons-react";
import { tv } from "tailwind-variants/lite";
import { Box, Row } from "./layout";

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
  // Deleting a post to write it again: a page being written, the draft it goes back to.
  redraft: { off: IconFilePencil },
  // The versions of an edited post: a clock turning back.
  history: { off: IconHistory },
  // The places of the navigation, on where the reader is, and the entry to write a post.
  home: { off: IconHome, on: IconHomeFilled },
  notifications: { off: IconBell, on: IconBellFilled },
  // The direct messages, by conversation: a letter, apart from reply's bubble and from the visibility's @.
  directMessages: { off: IconMail, on: IconMailFilled },
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
  // The sections of the emoji picker, by its row of categories (ADR-0091 §3): the reader's frequent ones (the clock,
  // as `time`), the server's own, and the nine groups of Unicode emoji. The glyphs stand for the emoji, which change
  // shape and color by platform and vanish in forced colors.
  emojiCustom: { off: IconSparkles },
  emojiSmileys: { off: IconMoodHappy },
  emojiPeople: { off: IconHandStop },
  emojiNature: { off: IconPaw },
  emojiFood: { off: IconToolsKitchen2 },
  emojiTravel: { off: IconPlane },
  emojiActivities: { off: IconBallFootball },
  emojiObjects: { off: IconBulb },
  emojiSymbols: { off: IconHeart },
  emojiFlags: { off: IconFlag },
  translate: { off: IconLanguage },
  // Places with no item in the navigation: a voice chat room, the server's announcements, and the server's own page.
  room: { off: IconMicrophone },
  announcements: { off: IconSpeakerphone },
  // On, while what it tells is shown (the media viewer's description).
  about: { off: IconInfoCircle, on: IconInfoCircleFilled },
  // What a reader chose not to see yet: a content warning's fold. An eye crossed out, not a flag, which reads as report.
  // On, while the composer holds a content warning.
  hidden: { off: IconEyeOff, on: "tile" },
  // The kinds of a notification with no action of their own (the rough of #3047): a follow, an ended poll (poll,
  // above), the moderators' notice; and a post its author pinned to their profile.
  follow: { off: IconUserPlus, directional: true },
  // Taking an account off the reader's followers: the follow glyph's person, a minus for its plus.
  removeFollower: { off: IconUserMinus, directional: true },
  // The reader's own note on an account, which only they see: a sticky note, apart from the drafts' page of notes.
  note: { off: IconNote },
  // An account that moved to another: an arrow leaving a bar, toward the end of the line, apart from open's box.
  moved: { off: IconArrowMoveRight, directional: true },
  moderation: { off: IconShield, on: IconShieldFilled },
  pin: { off: IconPin },
  // What a timeline is of, off the navigation: a hashtag, and a list of accounts the reader made.
  hashtag: { off: IconHash },
  list: { off: IconListDetails },
  // Whether something is in a set the reader keeps (an account in one of their lists): a box, filled and checked when
  // it is, so that on differs from off by shape.
  included: { off: IconSquare, on: IconSquareCheckFilled },
  // The value chosen among a menu's (a visibility), at the item's end.
  chosen: { off: IconCheck },
  // The accounts in a list: people, apart from followers' lock.
  members: { off: IconUsers },
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
  // Media: playing a video or a sound, pausing it, a file of a kind the client cannot show, a sound, opening a
  // picture whole, zooming in it, and moving to the previous one in a row of them (`onward` moves to the next).
  play: { off: IconPlayerPlayFilled },
  pause: { off: IconPlayerPauseFilled },
  file: { off: IconFile },
  audio: { off: IconMusic },
  whole: { off: IconMaximize },
  zoomIn: { off: IconZoomIn },
  zoomOut: { off: IconZoomOut },
  previous: { off: IconChevronLeft, directional: true },
  // Drafts and scheduled posts (ADR-0156): a post set to go out at a time (on, while the composer holds one); the
  // reader's drafts, a page of notes; and keeping what is written as a draft.
  schedule: { off: IconCalendarTime, on: "tile" },
  drafts: { off: IconNotes },
  // An article (ADR-0160): a page with a title over its lines.
  article: { off: IconArticle },
  saveDraft: { off: IconDeviceFloppy },
  // The kinds of a notification, each by what it tells (FR-205); a kind the client does not know yet keeps the bell.
  // A new post of an account the reader subscribes to: the bell ringing, apart from the place's bell.
  subscribed: { off: IconBellRinging },
  // What the reader asked of another account answered: their reply, boost or quote approved, or refused.
  accepted: { off: IconCircleCheck },
  rejected: { off: IconCircleX },
  // A follow the reader lost when the moderators suspended a server or an account: a link broken.
  severed: { off: IconLinkOff },
  // The moderators' work: a registration that waits for approval, an appeal of an action, a webhook the server turned
  // off, an external review whose calls fail, a subscribed block list that cannot be fetched, a group of errors.
  signUp: { off: IconUserQuestion },
  appeal: { off: IconGavel },
  webhookOff: { off: IconWebhookOff },
  reviewFailing: { off: IconPlugConnectedX },
  listFailing: { off: IconWorldOff },
  errors: { off: IconBug },
  // The reader's supporter subscription: a payment that failed, the subscription ended, its plan retired.
  paymentFailed: { off: IconCreditCardOff },
  supportEnded: { off: IconHeartBroken },
  planRetired: { off: IconArchive },
  // The service closing, its dates, or its announcement withdrawn.
  closure: { off: IconServerOff },
  // The server's limit on how long the reader's posts stay before they are deleted.
  autoDelete: { off: IconHourglassLow },
  // A scheduled post that could not go out at its time: the calendar with a mark of what went wrong.
  scheduleFailed: { off: IconCalendarExclamation },
  // An event (events, above, for an invitation): its date fixed, its date or places changed, cancelled, a reminder
  // before it, its start, and its voice chat room that cannot open.
  eventConfirmed: { off: IconCalendarCheck },
  eventChanged: { off: IconCalendarCog },
  eventCancelled: { off: IconCalendarX },
  eventReminder: { off: IconAlarm },
  eventStarted: { off: IconCalendarBolt },
  roomUnavailable: { off: IconMicrophoneOff },
  // The screens of the settings with no glyph of their own elsewhere (ADR-0110 §5): how the app shows and posts
  // (sliders), the keyword filters (a funnel), signing in and who is signed in (a lock, as what only the reader
  // opens), the apps the account lets in (a grid of them), the invitations (a ticket), the account's data (a store of
  // it), and signing out: an arrow out of a door, as signing in is one into it. Within signing in, the second check
  // after the password: a shield that passed one.
  display: { off: IconAdjustmentsHorizontal },
  filter: { off: IconFilter },
  security: { off: IconLock },
  twoFactor: { off: IconShieldCheck },
  apps: { off: IconApps },
  invite: { off: IconTicket },
  accountData: { off: IconDatabase },
  signOut: { off: IconLogout2, directional: true },
} as const satisfies Record<string, Entry>;

export type IconName = keyof typeof icons;

/** Where an icon sits, which gives its size: in an action of a timeline, beside a caption, or in the navigation. */
export type IconPlace = "action" | "caption" | "navigation";

const icon = tv({
  slots: {
    glyph: "",
    tile: "rounded-icon-tile bg-current",
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
  return (
    <Box
      rigid
      as={<Glyph aria-hidden="true" focusable="false" {...(stroke ? { stroke } : {})} />}
      className={className}
    />
  );
}

/** An icon by its meaning, at the size of its place; `on` draws its on state. */
export function Icon({ name, place, on = false }: { name: IconName; place: IconPlace; on?: boolean }) {
  const entry: Entry = icons[name];
  const s = icon({ place, directional: entry.directional ?? false });
  const shape = on ? entry.on : undefined;
  if (shape === "tile")
    return (
      <Row as="span" inline rigid align="center" justify="center" aria-hidden="true" className={s.tile()}>
        {draw(entry.off, s.inner(), tileStroke)}
      </Row>
    );
  return draw(shape ?? entry.off, s.glyph());
}
