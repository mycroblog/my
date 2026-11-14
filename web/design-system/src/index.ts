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
export { Disclosure, type DisclosureProps } from "./disclosure";
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
  Fold,
  GroupHeading,
  NoticeCaption,
  NoticeLine,
  OpenRow,
  ScreenStack,
  SentenceName,
  StateText,
  TabRow,
  UnreadMark,
  type NoticeLineProps,
  type TabRowProps,
} from "./list";
export { MenuButton, MenuGroup, MenuItem, MenuPanel } from "./menu";
export { Badge, FieldList, MetaCount, MetaLine, ProfileHead, TagList, type ProfileHeadProps } from "./profile";
export { DetailCount, DetailLines, GlyphText, ThreadFocus, ThreadReply } from "./thread";
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
  Frame,
  FrameAction,
  FrameAside,
  FrameFloat,
  FrameHeading,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  FrameTabBar,
  focusFrameHeading,
  NavItem,
  NavList,
} from "./shell";
export { useScrollRestoration, type ScrollMode } from "./scroll";
export { ActionScope, helpAction, ShortcutProvider, useAction } from "./shortcuts";
export { RelativeTime, type RelativeTimeProps } from "./time";
// The app's router reaches the links of the parts through React Aria's RouterProvider, which the app places.
export { I18nProvider, RouterProvider as NavigationProvider, useLocale } from "react-aria-components";
