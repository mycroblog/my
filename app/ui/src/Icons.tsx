// SPDX-License-Identifier: AGPL-3.0-only
// The icon set of the kit (web-client/16-look-states-and-wording.md §16.3, G10–G13; ADR-0043). The shapes are Phosphor's, through
// react-icons/pi; this file is the only one that imports them. `iconTable` maps each meaningful name
// (`ReplyIcon`) to its Phosphor icon off and on and to whether it has a direction, and every export below
// renders one row of it. The Regular weight is the base; a toggle that is on uses the Fill weight when its
// shape closes (star, bookmark, warning) and the Bold weight when it does not (boost), so the state shows by
// shape and not by color alone (WCAG 1.4.1).
//
// Every icon is decorative (`aria-hidden`, `focusable="false"`): the visible label or the accessible name of
// the control carries the meaning. An icon sets no size of its own; the control around it does (`Button`,
// `ToggleButton` and `Link` size a child `svg`), and `currentColor` gives it the control's color. Icons with
// a direction mirror in right-to-left text with `rtl:-scale-x-100` (Phosphor's `mirrored` ignores the
// writing direction, so it is not used).
import {
  PiAirplaneTilt,
  PiClockCounterClockwise,
  PiFlag,
  PiForkKnife,
  PiHandWaving,
  PiHeadset,
  PiHeart,
  PiLightbulb,
  PiPawPrint,
  PiSoccerBall,
  PiSticker,
  PiSealCheck,
  PiHandHeart,
  PiMonitor,
  PiClockCountdown,
  PiUserCheck,
  PiProhibit,
  PiSpeakerSimpleSlash,
  PiGlobeX,
  PiPassword,
  PiLockKey,
  PiFingerprint,
  PiLinkSimple,
  PiPlugsConnected,
  PiCode,
  PiArrowsDownUp,
  PiAddressBook,
  PiSuitcaseRolling,
  PiClipboardText,
  PiArchive,
  PiSquaresFour,
  PiScales,
  PiListMagnifyingGlass,
  PiUsers,
  PiUserCirclePlus,
  PiUserCircleMinus,
  PiGlobeSimpleX,
  PiGear,
  PiListChecks,
  PiPalette,
  PiFileText,
  PiMegaphone,
  PiBroadcast,
  PiWebhooksLogo,
  PiKey,
  PiIdentificationBadge,
  PiQueue,
  PiBug,
  PiPower,
  PiArrowBendUpLeft,
  PiAt,
  PiBuildings,
  PiGlobeHemisphereWest,
  PiLockSimple,
  PiLockSimpleOpen,
  PiArrowLeft,
  PiArrowSquareOut,
  PiArrowDown,
  PiArrowUp,
  PiBell,
  PiBookmarkSimple,
  PiBookmarkSimpleFill,
  PiCaretLeft,
  PiCheck,
  PiCaretRight,
  PiChartBarHorizontal,
  PiChartBarHorizontalFill,
  PiCoins,
  PiExport,
  PiHandPalm,
  PiHandPalmFill,
  PiMicrophone,
  PiMicrophoneFill,
  PiMicrophoneSlash,
  PiSignOut,
  PiDotsThreeBold,
  PiEyeSlash,
  PiGlobe,
  PiHouse,
  PiImage,
  PiInfo,
  PiList,
  PiMagnifyingGlass,
  PiNote,
  PiPauseFill,
  PiPlayFill,
  PiPlus,
  PiQuotes,
  PiRepeat,
  PiRepeatBold,
  PiShieldCheck,
  PiSignIn,
  PiSmiley,
  PiSlidersHorizontal,
  PiStar,
  PiStarFill,
  PiTextAlignLeft,
  PiTranslate,
  PiUser,
  PiUserPlus,
  PiWarning,
  PiWarningFill,
  PiWrench,
  PiX,
} from "react-icons/pi";

/** The state of a toggle's icon: filled while on, so the state shows by shape and not by color alone (WCAG 1.4.1). */
export type ToggleIconProps = { filled?: boolean };

/**
 * One Phosphor icon and its name in the Phosphor catalog (https://phosphoricons.com/), which the Storybook
 * gallery shows (a minified build renames the components, so the name is written out).
 */
type Glyph = { readonly icon: typeof PiX; readonly phosphor: string };

type IconEntry = {
  readonly off: Glyph;
  /** The shape of a toggle that is on. */
  readonly on?: Glyph;
  /** Mirrors the icon in right-to-left text, for an icon whose direction follows the line (reply, back). */
  readonly directional?: boolean;
};

const glyph = (icon: typeof PiX, phosphor: string): Glyph => ({ icon, phosphor });

/** Every icon of the kit: its meaningful name, and the Phosphor icons that draw it. */
export const iconTable = {
  // Actions.
  CloseIcon: { off: glyph(PiX, "PiX") },
  // Done and succeeded, for a success message and toast; a check does not point anywhere, so it does not mirror.
  CheckIcon: { off: glyph(PiCheck, "PiCheck") },
  PlusIcon: { off: glyph(PiPlus, "PiPlus") },
  // Regular's dots are too small to read at size-5, so the overflow menu uses Bold's.
  MoreIcon: { off: glyph(PiDotsThreeBold, "PiDotsThreeBold") },
  BackIcon: { off: glyph(PiArrowLeft, "PiArrowLeft"), directional: true },
  ExternalLinkIcon: { off: glyph(PiArrowSquareOut, "PiArrowSquareOut"), directional: true },
  SubmenuIcon: { off: glyph(PiCaretRight, "PiCaretRight"), directional: true },
  // Up and down are the order of a list in any writing direction, so they do not mirror.
  MoveUpIcon: { off: glyph(PiArrowUp, "PiArrowUp") },
  MoveDownIcon: { off: glyph(PiArrowDown, "PiArrowDown") },
  // Posts.
  ReplyIcon: { off: glyph(PiArrowBendUpLeft, "PiArrowBendUpLeft"), directional: true },
  // Up is where the new items wait (above the feed) in any writing direction, so it does not mirror.
  NewItemsIcon: { off: glyph(PiArrowUp, "PiArrowUp") },
  HideMediaIcon: { off: glyph(PiEyeSlash, "PiEyeSlash") },
  // The image viewer steps through a post's pictures in the order of the line, so they mirror.
  PreviousIcon: { off: glyph(PiCaretLeft, "PiCaretLeft"), directional: true },
  NextIcon: { off: glyph(PiCaretRight, "PiCaretRight"), directional: true },
  // A note: the button that lays a picture's description over it. The page has no direction, so it does not mirror.
  AltTextIcon: { off: glyph(PiNote, "PiNote") },
  // A loop means a loop and not a direction, so it does not mirror; it has nothing to fill, so on is Bold.
  BoostIcon: { off: glyph(PiRepeat, "PiRepeat"), on: glyph(PiRepeatBold, "PiRepeatBold") },
  StarIcon: { off: glyph(PiStar, "PiStar"), on: glyph(PiStarFill, "PiStarFill") },
  BookmarkIcon: {
    off: glyph(PiBookmarkSimple, "PiBookmarkSimple"),
    on: glyph(PiBookmarkSimpleFill, "PiBookmarkSimpleFill"),
  },
  // Quotation marks are not a direction, so they do not mirror.
  QuoteIcon: { off: glyph(PiQuotes, "PiQuotes") },
  // Playback of audio and video: a triangle pointing along the line would flip, but a play button points to the
  // end of the line in left-to-right text and the media's time still runs that way, so it does not mirror.
  PlayIcon: { off: glyph(PiPlayFill, "PiPlayFill") },
  PauseIcon: { off: glyph(PiPauseFill, "PiPauseFill") },
  // The new-post form.
  ImageIcon: { off: glyph(PiImage, "PiImage") },
  // Filled, the exclamation mark stays cut out of the triangle.
  WarningIcon: { off: glyph(PiWarning, "PiWarning"), on: glyph(PiWarningFill, "PiWarningFill") },
  // The visibilities of a post (ADR-0017), one shape each from the widest to the narrowest: the world, this
  // server's buildings, an open lock, a closed lock, and the at sign of a mention. None has a direction.
  PublicVisibilityIcon: { off: glyph(PiGlobeHemisphereWest, "PiGlobeHemisphereWest") },
  LocalVisibilityIcon: { off: glyph(PiBuildings, "PiBuildings") },
  UnlistedVisibilityIcon: { off: glyph(PiLockSimpleOpen, "PiLockSimpleOpen") },
  PrivateVisibilityIcon: { off: glyph(PiLockSimple, "PiLockSimple") },
  DirectVisibilityIcon: { off: glyph(PiAt, "PiAt") },
  LanguageIcon: { off: glyph(PiTranslate, "PiTranslate") },
  EmojiIcon: { off: glyph(PiSmiley, "PiSmiley") },
  // The category buttons of the emoji picker (ADR-0091 §3): drawn with Phosphor and not with an emoji, whose shape and
  // color differ between systems and which forced colors flatten. The smiling face of EmojiIcon stands for the smileys.
  FrequentEmojiIcon: { off: glyph(PiClockCounterClockwise, "PiClockCounterClockwise") },
  CustomEmojiIcon: { off: glyph(PiSticker, "PiSticker") },
  PeopleEmojiIcon: { off: glyph(PiHandWaving, "PiHandWaving") },
  NatureEmojiIcon: { off: glyph(PiPawPrint, "PiPawPrint") },
  FoodEmojiIcon: { off: glyph(PiForkKnife, "PiForkKnife") },
  // A picture of a plane and not a direction along the line, so it does not mirror.
  TravelEmojiIcon: { off: glyph(PiAirplaneTilt, "PiAirplaneTilt") },
  ActivityEmojiIcon: { off: glyph(PiSoccerBall, "PiSoccerBall") },
  ObjectEmojiIcon: { off: glyph(PiLightbulb, "PiLightbulb") },
  SymbolEmojiIcon: { off: glyph(PiHeart, "PiHeart") },
  FlagEmojiIcon: { off: glyph(PiFlag, "PiFlag") },
  // The bars of the results grow from the start of the line, so they mirror; filled while the post has a poll.
  PollIcon: {
    off: glyph(PiChartBarHorizontal, "PiChartBarHorizontal"),
    on: glyph(PiChartBarHorizontalFill, "PiChartBarHorizontalFill"),
    directional: true,
  },
  // Navigation.
  HomeIcon: { off: glyph(PiHouse, "PiHouse") },
  LocalIcon: { off: glyph(PiTextAlignLeft, "PiTextAlignLeft") },
  FederatedIcon: { off: glyph(PiGlobe, "PiGlobe") },
  SearchIcon: { off: glyph(PiMagnifyingGlass, "PiMagnifyingGlass") },
  NotificationsIcon: { off: glyph(PiBell, "PiBell") },
  ProfileIcon: { off: glyph(PiUser, "PiUser") },
  SettingsIcon: { off: glyph(PiSlidersHorizontal, "PiSlidersHorizontal") },
  ModerationIcon: { off: glyph(PiShieldCheck, "PiShieldCheck") },
  AdminIcon: { off: glyph(PiWrench, "PiWrench") },
  FinanceIcon: { off: glyph(PiCoins, "PiCoins") },
  // Voice chat (ADR-0100): a headset, which has no direction.
  VoiceIcon: { off: glyph(PiHeadset, "PiHeadset") },
  // The grouped menus of the settings and of the admin and moderation mode (ADR-0110 §5): one icon per page.
  BadgeIcon: { off: glyph(PiSealCheck, "PiSealCheck") },
  SupporterIcon: { off: glyph(PiHandHeart, "PiHandHeart") },
  DisplayIcon: { off: glyph(PiMonitor, "PiMonitor") },
  AutoDeleteIcon: { off: glyph(PiClockCountdown, "PiClockCountdown") },
  FollowRequestIcon: { off: glyph(PiUserCheck, "PiUserCheck") },
  BlockIcon: { off: glyph(PiProhibit, "PiProhibit") },
  MuteIcon: { off: glyph(PiSpeakerSimpleSlash, "PiSpeakerSimpleSlash") },
  DomainMuteIcon: { off: glyph(PiGlobeX, "PiGlobeX") },
  PasswordIcon: { off: glyph(PiPassword, "PiPassword") },
  TwoFactorIcon: { off: glyph(PiLockKey, "PiLockKey") },
  PasskeyIcon: { off: glyph(PiFingerprint, "PiFingerprint") },
  LinkedAccountIcon: { off: glyph(PiLinkSimple, "PiLinkSimple") },
  AuthorizedAppIcon: { off: glyph(PiPlugsConnected, "PiPlugsConnected") },
  DeveloperIcon: { off: glyph(PiCode, "PiCode") },
  ImportExportIcon: { off: glyph(PiArrowsDownUp, "PiArrowsDownUp") },
  AliasIcon: { off: glyph(PiAddressBook, "PiAddressBook") },
  MoveAccountIcon: { off: glyph(PiSuitcaseRolling, "PiSuitcaseRolling") },
  AccountStatusIcon: { off: glyph(PiClipboardText, "PiClipboardText") },
  ArchiveIcon: { off: glyph(PiArchive, "PiArchive") },
  OverviewIcon: { off: glyph(PiSquaresFour, "PiSquaresFour") },
  ReportIcon: { off: glyph(PiFlag, "PiFlag") },
  AppealIcon: { off: glyph(PiScales, "PiScales") },
  AuditLogIcon: { off: glyph(PiListMagnifyingGlass, "PiListMagnifyingGlass") },
  AccountsIcon: { off: glyph(PiUsers, "PiUsers") },
  RegistrationIcon: { off: glyph(PiUserCirclePlus, "PiUserCirclePlus") },
  SignUpBlockIcon: { off: glyph(PiUserCircleMinus, "PiUserCircleMinus") },
  DomainActionIcon: { off: glyph(PiGlobeSimpleX, "PiGlobeSimpleX") },
  ServerSettingsIcon: { off: glyph(PiGear, "PiGear") },
  RulesIcon: { off: glyph(PiListChecks, "PiListChecks") },
  ThemeIcon: { off: glyph(PiPalette, "PiPalette") },
  DocumentIcon: { off: glyph(PiFileText, "PiFileText") },
  AnnouncementIcon: { off: glyph(PiMegaphone, "PiMegaphone") },
  RelayIcon: { off: glyph(PiBroadcast, "PiBroadcast") },
  WebhookIcon: { off: glyph(PiWebhooksLogo, "PiWebhooksLogo") },
  LoginProviderIcon: { off: glyph(PiKey, "PiKey") },
  RoleIcon: { off: glyph(PiIdentificationBadge, "PiIdentificationBadge") },
  JobIcon: { off: glyph(PiQueue, "PiQueue") },
  ErrorIcon: { off: glyph(PiBug, "PiBug") },
  ClosureIcon: { off: glyph(PiPower, "PiPower") },
  // The controls of a call. The microphone and the raised hand close, so they fill while on.
  MicrophoneIcon: { off: glyph(PiMicrophone, "PiMicrophone"), on: glyph(PiMicrophoneFill, "PiMicrophoneFill") },
  // The slash is the mark of a microphone that is off (a badge), not a direction, so it does not mirror.
  MicrophoneOffIcon: { off: glyph(PiMicrophoneSlash, "PiMicrophoneSlash") },
  RaiseHandIcon: { off: glyph(PiHandPalm, "PiHandPalm"), on: glyph(PiHandPalmFill, "PiHandPalmFill") },
  // An arrow up out of a tray: up is not a direction along the line, so it does not mirror.
  ShareIcon: { off: glyph(PiExport, "PiExport") },
  InviteIcon: { off: glyph(PiUserPlus, "PiUserPlus") },
  // The arrow points out of the door along the line, so it mirrors in right-to-left text.
  LeaveIcon: { off: glyph(PiSignOut, "PiSignOut"), directional: true },
  MenuIcon: { off: glyph(PiList, "PiList") },
  AboutIcon: { off: glyph(PiInfo, "PiInfo") },
  // The arrow points into the door along the line, so it mirrors in right-to-left text.
  SignInIcon: { off: glyph(PiSignIn, "PiSignIn"), directional: true },
  SignUpIcon: { off: glyph(PiUserPlus, "PiUserPlus") },
  // Data (ADR-0113): the change of a number from the period before. Up and down are the value's direction in any
  // writing direction, so they do not mirror.
  IncreaseIcon: { off: glyph(PiArrowUp, "PiArrowUp") },
  DecreaseIcon: { off: glyph(PiArrowDown, "PiArrowDown") },
} as const satisfies Record<string, IconEntry>;

function renderIcon(entry: IconEntry, on = false) {
  const { icon: Glyph } = (on && entry.on) || entry.off;
  return <Glyph aria-hidden="true" focusable="false" className={entry.directional ? "rtl:-scale-x-100" : undefined} />;
}

// Actions.

/** A check mark, for something that succeeded. */
export function CheckIcon() {
  return renderIcon(iconTable.CheckIcon);
}

/** The × of close and dismiss buttons. */
export function CloseIcon() {
  return renderIcon(iconTable.CloseIcon);
}

/** A plus, for a button that creates something (a post). */
export function PlusIcon() {
  return renderIcon(iconTable.PlusIcon);
}

/** Three dots, for the button of an overflow menu. */
export function MoreIcon() {
  return renderIcon(iconTable.MoreIcon);
}

/** An arrow to the start of the line, for a link back to the page before. It flips in right-to-left text. */
export function BackIcon() {
  return renderIcon(iconTable.BackIcon);
}

/** A box with an arrow leaving it, after the text of a link to another site. It flips in right-to-left text. */
export function ExternalLinkIcon() {
  return renderIcon(iconTable.ExternalLinkIcon);
}

/** A caret to the end of the line, after the label of a menu item that opens a submenu. It flips in right-to-left text. */
export function SubmenuIcon() {
  return renderIcon(iconTable.SubmenuIcon);
}

/** An arrow up, for a button that moves an item one place up in a list. */
export function MoveUpIcon() {
  return renderIcon(iconTable.MoveUpIcon);
}

/** An arrow down, for a button that moves an item one place down in a list. */
export function MoveDownIcon() {
  return renderIcon(iconTable.MoveDownIcon);
}

// Posts.

/** An arrow turning back to the start of the line, for replying. It flips in right-to-left text. */
export function ReplyIcon() {
  return renderIcon(iconTable.ReplyIcon);
}

/** An arrow up, for the button that shows the new posts waiting above a feed. */
export function NewItemsIcon() {
  return renderIcon(iconTable.NewItemsIcon);
}

/** A filled triangle, for the button that plays audio or video. */
export function PlayIcon() {
  return renderIcon(iconTable.PlayIcon);
}

/** Two filled bars, for the button that pauses audio or video. */
export function PauseIcon() {
  return renderIcon(iconTable.PauseIcon);
}

/** An eye struck through, for the button that conceals sensitive media again. */
export function HideMediaIcon() {
  return renderIcon(iconTable.HideMediaIcon);
}

/** A caret to the start of the line, for the button that shows the picture before. It flips in right-to-left text. */
export function PreviousIcon() {
  return renderIcon(iconTable.PreviousIcon);
}

/** A note, for the button of the image viewer that shows or hides the picture's description. */
export function AltTextIcon() {
  return renderIcon(iconTable.AltTextIcon);
}

/** A caret to the end of the line, for the button that shows the next picture. It flips in right-to-left text. */
export function NextIcon() {
  return renderIcon(iconTable.NextIcon);
}

/** Two arrows in a loop, for boosting; bolder while on. */
export function BoostIcon({ filled = false }: ToggleIconProps) {
  return renderIcon(iconTable.BoostIcon, filled);
}

/** A star, for favouriting; filled while on. */
export function StarIcon({ filled = false }: ToggleIconProps) {
  return renderIcon(iconTable.StarIcon, filled);
}

/** A bookmark ribbon, for bookmarking; filled while on. */
export function BookmarkIcon({ filled = false }: ToggleIconProps) {
  return renderIcon(iconTable.BookmarkIcon, filled);
}

/** Quotation marks, for quoting a post (FR-114). */
export function QuoteIcon() {
  return renderIcon(iconTable.QuoteIcon);
}

// The new-post form.

/** A framed picture, for attaching images. */
export function ImageIcon() {
  return renderIcon(iconTable.ImageIcon);
}

/** A triangle with an exclamation mark, for a content warning; filled while on. */
export function WarningIcon({ filled = false }: ToggleIconProps) {
  return renderIcon(iconTable.WarningIcon, filled);
}

/** The visibilities of a post from the widest to the narrowest (ADR-0017). */
export type PostVisibility = "public" | "local" | "unlisted" | "private" | "direct";

/** A globe, for a public post: anyone, listed on the public timelines. */
export function PublicVisibilityIcon() {
  return renderIcon(iconTable.PublicVisibilityIcon);
}

/** Buildings, for a post to this server: its signed-in people and the author's followers. */
export function LocalVisibilityIcon() {
  return renderIcon(iconTable.LocalVisibilityIcon);
}

/** An open lock, for an unlisted post: anyone, kept off the public timelines. */
export function UnlistedVisibilityIcon() {
  return renderIcon(iconTable.UnlistedVisibilityIcon);
}

/** A closed lock, for a post to the followers. */
export function PrivateVisibilityIcon() {
  return renderIcon(iconTable.PrivateVisibilityIcon);
}

/** An at sign, for a post to the mentioned people only. */
export function DirectVisibilityIcon() {
  return renderIcon(iconTable.DirectVisibilityIcon);
}

const visibilityIcons = {
  public: iconTable.PublicVisibilityIcon,
  local: iconTable.LocalVisibilityIcon,
  unlisted: iconTable.UnlistedVisibilityIcon,
  private: iconTable.PrivateVisibilityIcon,
  direct: iconTable.DirectVisibilityIcon,
} as const satisfies Record<PostVisibility, IconEntry>;

export type VisibilityIconProps = {
  /** The level whose icon to draw. */
  visibility: PostVisibility;
};

/** The icon of a visibility: a different shape for each level, so that they are told apart without color. */
export function VisibilityIcon({ visibility }: VisibilityIconProps) {
  return renderIcon(visibilityIcons[visibility]);
}

/** Two scripts side by side, for the language of a post. */
export function LanguageIcon() {
  return renderIcon(iconTable.LanguageIcon);
}

/** A smiling face, for the button that opens the picker of custom emoji. */
export function EmojiIcon() {
  return renderIcon(iconTable.EmojiIcon);
}

/** A clock turning back, for the emoji used often, first in the emoji picker. */
export function FrequentEmojiIcon() {
  return renderIcon(iconTable.FrequentEmojiIcon);
}

/** A sticker, for the custom emoji of the server in the emoji picker. */
export function CustomEmojiIcon() {
  return renderIcon(iconTable.CustomEmojiIcon);
}

/** A waving hand, for the people and body emoji in the emoji picker. */
export function PeopleEmojiIcon() {
  return renderIcon(iconTable.PeopleEmojiIcon);
}

/** A paw print, for the animal and nature emoji in the emoji picker. */
export function NatureEmojiIcon() {
  return renderIcon(iconTable.NatureEmojiIcon);
}

/** A fork and a knife, for the food and drink emoji in the emoji picker. */
export function FoodEmojiIcon() {
  return renderIcon(iconTable.FoodEmojiIcon);
}

/** A plane, for the travel and places emoji in the emoji picker. */
export function TravelEmojiIcon() {
  return renderIcon(iconTable.TravelEmojiIcon);
}

/** A ball, for the activity emoji in the emoji picker. */
export function ActivityEmojiIcon() {
  return renderIcon(iconTable.ActivityEmojiIcon);
}

/** A light bulb, for the object emoji in the emoji picker. */
export function ObjectEmojiIcon() {
  return renderIcon(iconTable.ObjectEmojiIcon);
}

/** A heart, for the symbol emoji in the emoji picker. */
export function SymbolEmojiIcon() {
  return renderIcon(iconTable.SymbolEmojiIcon);
}

/** A flag, for the flag emoji in the emoji picker. */
export function FlagEmojiIcon() {
  return renderIcon(iconTable.FlagEmojiIcon);
}

/** Bars of results, for adding a poll to a post; filled while on. */
export function PollIcon({ filled = false }: ToggleIconProps) {
  return renderIcon(iconTable.PollIcon, filled);
}

// Navigation.

/** A house, for the home timeline. */
export function HomeIcon() {
  return renderIcon(iconTable.HomeIcon);
}

/** Lines of text, for the local timeline. */
export function LocalIcon() {
  return renderIcon(iconTable.LocalIcon);
}

/** A globe, for the federated timeline. */
export function FederatedIcon() {
  return renderIcon(iconTable.FederatedIcon);
}

/** A magnifying glass, for search. */
export function SearchIcon() {
  return renderIcon(iconTable.SearchIcon);
}

/** A bell, for notifications. */
export function NotificationsIcon() {
  return renderIcon(iconTable.NotificationsIcon);
}

/** A head and shoulders, for a profile. */
export function ProfileIcon() {
  return renderIcon(iconTable.ProfileIcon);
}

/** Sliders, for settings. */
export function SettingsIcon() {
  return renderIcon(iconTable.SettingsIcon);
}

/** A shield with a check, for the moderation. */
export function ModerationIcon() {
  return renderIcon(iconTable.ModerationIcon);
}

/** A wrench, for the server administration. */
export function AdminIcon() {
  return renderIcon(iconTable.AdminIcon);
}

/** A stack of coins, for the finances of the server. */
export function FinanceIcon() {
  return renderIcon(iconTable.FinanceIcon);
}

/** A headset, for the voice chat rooms. */
export function VoiceIcon() {
  return renderIcon(iconTable.VoiceIcon);
}

/** A seal with a check, for the badges shown on the user's profile. */
export function BadgeIcon() {
  return renderIcon(iconTable.BadgeIcon);
}

/** A hand holding a heart, for supporting the server (the supporter plans and the supporters). */
export function SupporterIcon() {
  return renderIcon(iconTable.SupporterIcon);
}

/** A screen, for how the app shows things and posts (the user's display and posting preferences). */
export function DisplayIcon() {
  return renderIcon(iconTable.DisplayIcon);
}

/** A clock counting down, for deleting old posts automatically. */
export function AutoDeleteIcon() {
  return renderIcon(iconTable.AutoDeleteIcon);
}

/** A person with a check, for the follow requests that await the user. */
export function FollowRequestIcon() {
  return renderIcon(iconTable.FollowRequestIcon);
}

/** A circle struck through, for the accounts the user blocks. */
export function BlockIcon() {
  return renderIcon(iconTable.BlockIcon);
}

/** A speaker struck through, for the accounts the user mutes. */
export function MuteIcon() {
  return renderIcon(iconTable.MuteIcon);
}

/** A globe with a cross, for the servers the user mutes. */
export function DomainMuteIcon() {
  return renderIcon(iconTable.DomainMuteIcon);
}

/** A row of hidden characters, for the password. */
export function PasswordIcon() {
  return renderIcon(iconTable.PasswordIcon);
}

/** A padlock with a keyhole, for two-factor authentication. */
export function TwoFactorIcon() {
  return renderIcon(iconTable.TwoFactorIcon);
}

/** A fingerprint, for passkeys. */
export function PasskeyIcon() {
  return renderIcon(iconTable.PasskeyIcon);
}

/** Two links of a chain, for the accounts of other services linked to the user's. */
export function LinkedAccountIcon() {
  return renderIcon(iconTable.LinkedAccountIcon);
}

/** Two plugs joined, for the apps the user has let use the account. */
export function AuthorizedAppIcon() {
  return renderIcon(iconTable.AuthorizedAppIcon);
}

/** Angle brackets, for the apps registered for the API (the user's and the server's). */
export function DeveloperIcon() {
  return renderIcon(iconTable.DeveloperIcon);
}

/** Arrows down and up, for importing and exporting the user's data. */
export function ImportExportIcon() {
  return renderIcon(iconTable.ImportExportIcon);
}

/** An address book, for the other addresses of the account (its aliases). */
export function AliasIcon() {
  return renderIcon(iconTable.AliasIcon);
}

/** A suitcase, for moving the account to another server. */
export function MoveAccountIcon() {
  return renderIcon(iconTable.MoveAccountIcon);
}

/** A clipboard, for the account's status and the actions taken on it. */
export function AccountStatusIcon() {
  return renderIcon(iconTable.AccountStatusIcon);
}

/** A box of files, for the archive of the account and its deletion. */
export function ArchiveIcon() {
  return renderIcon(iconTable.ArchiveIcon);
}

/** Four squares, for the overview page of an area. */
export function OverviewIcon() {
  return renderIcon(iconTable.OverviewIcon);
}

/** A flag, for the reports of posts and accounts. */
export function ReportIcon() {
  return renderIcon(iconTable.ReportIcon);
}

/** Scales, for the appeals against moderation actions. */
export function AppealIcon() {
  return renderIcon(iconTable.AppealIcon);
}

/** A list under a magnifying glass, for the audit log. */
export function AuditLogIcon() {
  return renderIcon(iconTable.AuditLogIcon);
}

/** Two people, for the accounts of the server. */
export function AccountsIcon() {
  return renderIcon(iconTable.AccountsIcon);
}

/** A person with a plus in a circle, for the registrations that await approval. */
export function RegistrationIcon() {
  return renderIcon(iconTable.RegistrationIcon);
}

/** A person with a minus in a circle, for the addresses that may not sign up. */
export function SignUpBlockIcon() {
  return renderIcon(iconTable.SignUpBlockIcon);
}

/** A globe with a cross, for the actions taken on other servers. */
export function DomainActionIcon() {
  return renderIcon(iconTable.DomainActionIcon);
}

/** A gear, for the settings of the server. */
export function ServerSettingsIcon() {
  return renderIcon(iconTable.ServerSettingsIcon);
}

/** A list with checks, for the rules of the server. */
export function RulesIcon() {
  return renderIcon(iconTable.RulesIcon);
}

/** A palette, for the theme of the server. */
export function ThemeIcon() {
  return renderIcon(iconTable.ThemeIcon);
}

/** A page of text, for the server's documents (the terms of service and the privacy policy). */
export function DocumentIcon() {
  return renderIcon(iconTable.DocumentIcon);
}

/** A megaphone, for the server's announcements. */
export function AnnouncementIcon() {
  return renderIcon(iconTable.AnnouncementIcon);
}

/** A broadcasting antenna, for the relays. */
export function RelayIcon() {
  return renderIcon(iconTable.RelayIcon);
}

/** The webhook mark, for the webhooks. */
export function WebhookIcon() {
  return renderIcon(iconTable.WebhookIcon);
}

/** A key, for the external providers people sign in with. */
export function LoginProviderIcon() {
  return renderIcon(iconTable.LoginProviderIcon);
}

/** A name badge, for the roles. */
export function RoleIcon() {
  return renderIcon(iconTable.RoleIcon);
}

/** A queue, for the background jobs. */
export function JobIcon() {
  return renderIcon(iconTable.JobIcon);
}

/** A bug, for the errors the server recorded. */
export function ErrorIcon() {
  return renderIcon(iconTable.ErrorIcon);
}

/** A power symbol, for closing the service. */
export function ClosureIcon() {
  return renderIcon(iconTable.ClosureIcon);
}

/** A microphone, for the switch of the user's microphone in a call; filled while it is on. */
export function MicrophoneIcon({ filled = false }: ToggleIconProps) {
  return renderIcon(iconTable.MicrophoneIcon, filled);
}

/** A microphone with a slash, for a participant whose microphone is off. */
export function MicrophoneOffIcon() {
  return renderIcon(iconTable.MicrophoneOffIcon);
}

/** An open hand, for raising a hand to ask to speak; filled while it is raised. */
export function RaiseHandIcon({ filled = false }: ToggleIconProps) {
  return renderIcon(iconTable.RaiseHandIcon, filled);
}

/** An arrow out of a tray, for sharing a link. */
export function ShareIcon() {
  return renderIcon(iconTable.ShareIcon);
}

/** A person with a plus, for inviting someone. */
export function InviteIcon() {
  return renderIcon(iconTable.InviteIcon);
}

/** An arrow out of a door, for leaving a call. */
export function LeaveIcon() {
  return renderIcon(iconTable.LeaveIcon);
}

/** Three lines, for the button that opens the menu with the rest of the navigation. */
export function MenuIcon() {
  return renderIcon(iconTable.MenuIcon);
}

/** An "i" in a circle, for the server's information page. */
export function AboutIcon() {
  return renderIcon(iconTable.AboutIcon);
}

/** An arrow into a door, for signing in. */
export function SignInIcon() {
  return renderIcon(iconTable.SignInIcon);
}

/** A person with a plus, for creating an account. */
export function SignUpIcon() {
  return renderIcon(iconTable.SignUpIcon);
}

// Data.

/** An arrow up, beside the text of a number that rose from the period before (`StatTile`). */
export function IncreaseIcon() {
  return renderIcon(iconTable.IncreaseIcon);
}

/** An arrow down, beside the text of a number that fell from the period before (`StatTile`). */
export function DecreaseIcon() {
  return renderIcon(iconTable.DecreaseIcon);
}
