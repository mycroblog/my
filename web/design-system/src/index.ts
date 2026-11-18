// SPDX-License-Identifier: AGPL-3.0-only

// The design system of the web client (ADR-0025 §2–§6): its components, on React Aria Components, styled with
// Tailwind from the tokens of @my/design-tokens, and its icons. An app loads @my/design-system/styles.css once, with
// @my/design-system/emoji.css, the self-hosted Noto Color Emoji (ADR-0091 §9), and places React Aria's I18nProvider,
// from which every part reads the locale and the direction (locale.ts), and the TimeZoneProvider, from which it reads
// the zone of the dates and times it shows (zone.tsx).

export { Avatar, type AvatarProps, type AvatarSize } from "./avatar";
export {
  Button,
  LinkButton,
  markedSurface,
  raisedSurface,
  TextButton,
  TextLink,
  type ButtonKind,
  type ButtonProps,
  type ButtonSize,
  type LinkButtonProps,
  type TextButtonProps,
} from "./button";
export { Badge, Chip, ChipLink, ChipRow, type ChipProps } from "./chip";
export { BannerHead } from "./banner-head";
export { QuietLink } from "./quiet-link";
export {
  AttachButton,
  AttachmentList,
  AttachmentRow,
  CharCount,
  ChoiceSelect,
  ComposerFrame,
  ComposerText,
  PollSettings,
  ToolText,
  UndoNotice,
  WarningField,
  type AttachmentRowProps,
  type Choice,
  type ComposerFrameProps,
} from "./composer";
export { SuggestionList, type Suggestion, type SuggestionListProps } from "./suggestions";
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
export {
  ConfirmDialog,
  PanelDialog,
  ReauthDialog,
  type ConfirmDialogProps,
  type PanelDialogProps,
  type ReauthDialogProps,
} from "./dialog";
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
  HeardActions,
  LivePart,
  Stamp,
  type BylineProps,
  type CaptionLineProps,
  type EntryProps,
  type StampProps,
} from "./entry";
export { Feed, FeedItem, FeedNotice, moveFeedFocus, type FeedProps, type NewItems } from "./feed";
export { Faces, type FacesProps } from "./faces";
export {
  BareTextArea,
  CheckField,
  DateTimeField,
  FieldError,
  FieldNote,
  focusOnLoadAttribute,
  focusRefusedField,
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
export { PairField, PairList, type PairFieldProps } from "./pair-field";
export { PictureField, type PictureFieldProps } from "./picture-field";
export { FileField, type FileFieldProps } from "./file-field";
export { Icon, icons, type IconName, type IconPlace } from "./icons";
export { whenIdle } from "./idle";
export { catalogReader, type Strings } from "./locale";
export {
  ArticleCard,
  ArticleCover,
  ArticleParts,
  ArticleTable,
  ArticleText,
  ArticleTitle,
  type ArticleCardProps,
  type ArticleCoverImage,
} from "./article";
export {
  ArticleDetails,
  ArticleField,
  ArticleFrame,
  ArticleLimits,
  ArticleMode,
  ArticlePane,
  ArticlePreview,
  ArticleTextArea,
  ArticleTitleInput,
  CoverAdd,
  CoverRemove,
  CoverRow,
  TagInput,
  type ArticleFrameProps,
} from "./article-editor";
export { LinkCard, type LinkCardProps } from "./card";
export { AudioRow, FileLink, MediaGrid, playTime, type MediaGridProps, type MediaItem, type MediaKind } from "./media";
export { MediaViewer, type MediaViewerProps } from "./viewer";
export { InlinePrompt, type InlinePromptProps } from "./prompt";
export { Switch, type SwitchProps } from "./switch";
export {
  AccountRow,
  FoundRows,
  RowItem,
  RowList,
  ResultGroup,
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
  PressRow,
  ScreenHead,
  ScreenStack,
  ScreenTitle,
  SentenceName,
  StateText,
  TabRow,
  UnreadMark,
  type FacesRowProps,
  type NoticeLineProps,
  type ScreenTitleProps,
  type TabRowProps,
} from "./list";
export {
  MenuButton,
  MenuGroup,
  MenuHead,
  MenuItem,
  MenuPanel,
  FormPopup,
  MenuSheet,
  MenuSurface,
  useHeaded,
  type MenuPlacement,
  type MenuSurfaceProps,
} from "./menu";
export { ChoiceField, ChoiceList, ChoiceMark, ShareBar, type ChoiceItem } from "./choice";
export { Box, Pin, Row, Spoken, Stack } from "./layout";
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
  FrameActions,
  FrameAside,
  FrameBanner,
  FrameBanners,
  FrameDock,
  FrameFold,
  FrameFoot,
  FrameFooter,
  FrameHead,
  FrameHeading,
  FrameMain,
  FrameMark,
  FramePanel,
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
export { SearchField, type SearchFieldProps } from "./search-field";
export { SaveState, type SaveStateProps } from "./save-state";
export {
  NumberField,
  Select,
  type NumberFieldProps,
  type SelectGroup,
  type SelectOption,
  type SelectProps,
} from "./field";
export {
  SettingRow,
  SettingsEmpty,
  SettingsFacts,
  SettingsGroup,
  SettingsItem,
  SettingsList,
  SettingsAccountItem,
  SettingsFieldRow,
  SettingsListRow,
  SettingsMenu,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
  type SettingRowProps,
  type SettingsFactLine,
  type SettingsRowResult,
} from "./settings";
export {
  ChannelCell,
  ChannelGap,
  ChannelRow,
  ChannelTable,
  type ChannelCellProps,
  type ChannelColumn,
} from "./channels";
export { useScrollRestoration, type ScrollMode } from "./scroll";
export { ActionScope, helpAction, ShortcutProvider, useAction } from "./shortcuts";
export { RelativeTime, type RelativeTimeProps } from "./time";
export { dateFormat, instantAt, TimeZoneProvider, timeZoneName, useTimeZone, wallTime, type WallTime } from "./zone";
// The app's router reaches the links of the parts through React Aria's RouterProvider, which the app places.
export { I18nProvider, RouterProvider as NavigationProvider, useLocale } from "react-aria-components";
