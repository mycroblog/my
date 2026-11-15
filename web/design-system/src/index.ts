// SPDX-License-Identifier: AGPL-3.0-only

// The design system of the web client (ADR-0025 §2–§6): its components, on React Aria Components, styled with
// Tailwind from the tokens of @my/design-tokens, and its icons. An app loads @my/design-system/styles.css once, and
// places React Aria's I18nProvider, from which every part reads the locale and the direction (locale.ts).

export { Avatar, type AvatarProps, type AvatarSize } from "./avatar";
export {
  Button,
  LinkButton,
  markedSurface,
  TextButton,
  TextLink,
  type ButtonKind,
  type ButtonProps,
  type ButtonSize,
  type LinkButtonProps,
  type TextButtonProps,
} from "./button";
export { Chip, ChipRow, type ChipProps } from "./chip";
export {
  AttachButton,
  AttachmentList,
  AttachmentRow,
  CharCount,
  ChoiceBack,
  ChoiceGroup,
  ChoiceOpens,
  ChoicePopup,
  ChoiceSection,
  ChoiceSelect,
  ComposerFrame,
  ComposerQuote,
  ComposerReply,
  ComposerText,
  PollCard,
  PollOption,
  PollOptions,
  PollSettings,
  ToolText,
  UndoNotice,
  WarningField,
  type AttachmentRowProps,
  type Choice,
  type ComposerFrameProps,
} from "./composer";
export {
  ContentBody,
  ContentLink,
  CustomEmoji,
  UrlPart,
  type ContentBodyProps,
  type ContentLinkKind,
  type ContentLinkProps,
  type CustomEmojiProps,
  type UrlPartProps,
} from "./content";
export { ConfirmDialog, PanelDialog, type ConfirmDialogProps, type PanelDialogProps } from "./dialog";
export { Disclosure, type DisclosureProps } from "./disclosure";
export {
  DiffMedia,
  DiffText,
  Version,
  VersionList,
  type DiffImage,
  type DiffKind,
  type DiffMarks,
  type DiffPart,
  type VersionProps,
} from "./history";
// The picker itself is a chunk of its own, loaded when first wanted: the entry holds its types and where it opens.
export type { EmojiPickerLabels, EmojiPickerProps, EmojiSection, PickerEmoji } from "./emoji-picker";
export {
  LazyEmojiPicker as EmojiPicker,
  EmojiPickerPopover,
  EmojiPickerSheet,
  EmojiPickerSurface,
  preloadEmojiPicker,
  preloadEmojiPickerWhenIdle,
} from "./emoji-picker-overlay";
export { ModalPresence } from "./modal";
export {
  ActionRow,
  Byline,
  CaptionLine,
  CaptionName,
  Embed,
  Entry,
  Stack,
  Stamp,
  type BylineProps,
  type CaptionLineProps,
  type EntryProps,
  type StampProps,
} from "./entry";
export { Feed, FeedItem, FeedNotice, moveFeedFocus, type FeedProps, type NewItems } from "./feed";
export {
  CheckField,
  FieldError,
  focusOnLoadAttribute,
  Form,
  FormAccount,
  FormActions,
  FormAlert,
  FormBadge,
  FormCaution,
  FormChoice,
  FormHead,
  FormLink,
  FormLinks,
  FormList,
  FormOthers,
  FormPage,
  FormPageFooter,
  FormSection,
  FormText,
  GlyphLabel,
  TextField,
  WidgetSlot,
  type FormPageProps,
  type FormPageServer,
  type TextFieldProps,
} from "./form-page";
export { IconButton, type IconButtonProps } from "./icon-button";
export { Icon, icons, type IconName, type IconPlace } from "./icons";
export { catalogReader, type Strings } from "./locale";
export { MediaGrid, type MediaGridProps, type MediaItem } from "./media";
export {
  AccountRow,
  BackButton,
  ButtonPair,
  Excerpt,
  FacesRow,
  Fold,
  GroupHeading,
  ListTitle,
  NoticeCaption,
  NoticeLine,
  OpenRow,
  ScreenStack,
  SentenceName,
  StateText,
  TabRow,
  UnreadMark,
  type FacesRowProps,
  type NoticeLineProps,
  type TabRowProps,
} from "./list";
export { MenuButton, MenuGroup, MenuItem, MenuPanel } from "./menu";
export {
  PollBox,
  PollChoices,
  PollFoot,
  PollMark,
  PollResults,
  type PollChoiceItem,
  type PollResultItem,
} from "./poll";
export { Badge, FieldList, MetaCount, MetaLine, ProfileHead, TagList, type ProfileHeadProps } from "./profile";
export { DetailLines, GlyphText, ThreadFocus, ThreadReply } from "./thread";
export {
  routerTransitions,
  runTransition,
  Shared,
  transitionKinds,
  transitionTypes,
  type TransitionKind,
} from "./motion";
export type { Binding, Keymap, Scope } from "./keys";
export {
  CallBar,
  Frame,
  FrameAction,
  FrameAside,
  FrameBanner,
  FrameBanners,
  FrameDock,
  FrameFold,
  FrameFooter,
  FrameHeading,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  focusFrameHeading,
  Meter,
  NavItem,
  NavList,
  NavMenu,
  SectionLine,
  SectionList,
  SectionNote,
  Toast,
} from "./shell";
export { useScrollRestoration, type ScrollMode } from "./scroll";
export { ActionScope, helpAction, ShortcutProvider, useAction } from "./shortcuts";
export { RelativeTime, type RelativeTimeProps } from "./time";
// The app's router reaches the links of the parts through React Aria's RouterProvider, which the app places.
export { I18nProvider, RouterProvider as NavigationProvider, useLocale } from "react-aria-components";
