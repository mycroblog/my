// SPDX-License-Identifier: AGPL-3.0-only
// Public surface of @my/ui, the shared UI kit (ADR-0026). The package ships TypeScript
// source with no build step: apps compile it with their own Vite and React Compiler setup,
// and their Tailwind entry scans it with @source.
export { Center, type CenterProps } from "./Center";

// The app-wide settings of the kit's components, which an app renders once at its root, so that it needs
// no direct dependency on react-aria-components:
// - I18nProvider: the locale (built-in strings, number and date formats, and the direction of arrow keys);
// - RouterProvider: the app router's navigate for every Link, Menu item or Tab with an href.
export { I18nProvider, type I18nProviderProps, RouterProvider } from "react-aria-components";

// Form controls.
export { Button, ButtonLink, type ButtonLinkProps, type ButtonProps } from "./Button";
export { Checkbox, type CheckboxProps } from "./Checkbox";
export { DurationSelect, type DurationSelectProps } from "./DurationSelect";
export type { FieldErrorMessage, FieldTextProps } from "./Field";
// The hidden file input behind a Button child (React Aria FileTrigger): it has no look of its own.
export { FileTrigger, type FileTriggerProps } from "react-aria-components";
export { ExternalLink, ExternalLinkContext, type ExternalLinkProps, type ExternalLinkSettings } from "./ExternalLink";
export { Link, type LinkProps } from "./Link";
export { Radio, RadioGroup, type RadioGroupProps, type RadioProps } from "./RadioGroup";
export { ReasonField, type ReasonFieldProps } from "./ReasonField";
export { Select, SelectItem, type SelectItemProps, type SelectProps } from "./Select";
export { Switch, type SwitchProps } from "./Switch";
export { TextArea, type TextAreaProps } from "./TextArea";
export {
  TextAreaAutocomplete,
  type TextAreaAutocompleteProps,
  type TextAutocompleteSuggestion,
  type TextAutocompleteWord,
  TextFieldAutocomplete,
  type TextFieldAutocompleteProps,
} from "./TextAreaAutocomplete";
export type { TextHighlight, TextHighlightKind } from "./TextAreaMirror";
export { TextField, type TextFieldProps } from "./TextField";
export { ToggleButton, type ToggleButtonProps } from "./ToggleButton";
export { Segmented, SegmentedItem, type SegmentedItemProps, type SegmentedProps } from "./Segmented";

// Overlays, tabs and announcements.
export { ConfirmDialog, type ConfirmDialogProps } from "./ConfirmDialog";
export { AnnouncerProvider, useAnnouncer, type Announce, type AnnouncerProviderProps, type Politeness } from "./Announcer";
export { Dialog, DialogTrigger, Sheet, type DialogProps, type DialogTriggerProps, type SheetProps } from "./Dialog";
export {
  Menu,
  MenuItem,
  MenuSeparator,
  MenuTrigger,
  SubmenuTrigger,
  type MenuItemProps,
  type MenuProps,
  type MenuTriggerProps,
  type SubmenuTriggerProps,
} from "./Menu";
export { Pagination, type PaginationProps, type PaginationTarget } from "./Pagination";
export { Popover, PopoverTrigger, type PopoverProps } from "./Popover";
export { Tab, TabList, TabPanel, Tabs, type TabListProps, type TabPanelProps, type TabProps, type TabsProps } from "./Tabs";
export { ToastProvider, useToast, type ToastApi, type ToastOptions, type ToastProviderProps, type ToastTone } from "./Toast";
export { Tooltip, TooltipTrigger, type TooltipProps, type TooltipTriggerProps } from "./Tooltip";

// Display.
export { Avatar, type AvatarProps } from "./Avatar";
export { Badge, type BadgeProps } from "./Badge";
export { RailItem, type RailItemBadge, type RailItemProps } from "./RailItem";
export { RoleBadge, RoleBadgeList, type RoleBadgeListProps, type RoleBadgeProps } from "./RoleBadge";
export { DescriptionItem, DescriptionList, type DescriptionItemProps, type DescriptionListProps } from "./DescriptionList";
export { EmptyState, type EmptyStateProps, type HeadingLevel } from "./EmptyState";
export { OfflineBanner, type OfflineBannerProps } from "./OfflineBanner";
export { Blurhash, type BlurhashProps } from "./Blurhash";
export { AutoplayContext, type ImageCandidate, type ImageSources, Picture, type PictureProps } from "./Picture";
export { QrCode, type QrCodeProps } from "./QrCode";
export { RegionError, type RegionErrorProps } from "./RegionError";
export { Skeleton, type SkeletonProps } from "./Skeleton";
export { AccountRowSkeleton, NotificationRowSkeleton, PostSkeleton, type SkeletonRowProps } from "./SkeletonRows";
export { Spinner, type SpinnerProps } from "./Spinner";
export { BrandMark, type BrandMarkProps } from "./BrandMark";
export { Wordmark, type WordmarkProps } from "./Wordmark";
export { StatusMessage, type StatusMessageProps } from "./StatusMessage";
export { VisuallyHidden, type VisuallyHiddenProps } from "./VisuallyHidden";

// Icons (web-client/16-look-states-and-wording.md §16.3): Phosphor through react-icons/pi (ADR-0043), decorative, sized by the control around them.
export {
  AboutIcon,
  CheckIcon,
  AdminIcon,
  AltTextIcon,
  BackIcon,
  BookmarkIcon,
  BoostIcon,
  CloseIcon,
  EmojiIcon,
  ExternalLinkIcon,
  EyeIcon,
  FederatedIcon,
  FinanceIcon,
  HomeIcon,
  ImageIcon,
  LanguageIcon,
  LocalIcon,
  MenuIcon,
  ModerationIcon,
  MoreIcon,
  MoveDownIcon,
  MoveUpIcon,
  NotificationsIcon,
  PlusIcon,
  PollIcon,
  ProfileIcon,
  QuoteIcon,
  ReplyIcon,
  SearchIcon,
  SettingsIcon,
  SignInIcon,
  SignUpIcon,
  StarIcon,
  SubmenuIcon,
  type ToggleIconProps,
  WarningIcon,
} from "./Icons";

// Timeline.
export { ContentWarning, type ContentWarningProps } from "./ContentWarning";
export { CustomEmoji, EmojiText, type CustomEmojiImage, type CustomEmojiProps, type EmojiTextProps } from "./CustomEmoji";
export {
  EmojiPicker,
  emojiMatchRank,
  normalizeEmojiSearch,
  searchEmojis,
  skinTones,
  skinToneVariant,
  type EmojiPickerLabels,
  type EmojiPickerProps,
  type EmojiPickerSection,
  type EmojiPickerSelection,
  type EmojiSectionIcon,
  type PickerCustomEmoji,
  type PickerEmoji,
  type PickerSkinVariant,
  type PickerUnicodeEmoji,
  type SkinTone,
} from "./EmojiPicker";
export { Feed, FeedArticle, type FeedArticleProps, type FeedProps } from "./Feed";
export { LinkCard, type LinkCardImage, type LinkCardProps } from "./LinkCard";
export { AudioPlayer, type AudioPlayerLabels, type AudioPlayerProps } from "./AudioPlayer";
export { MediaGrid, type MediaGridProps, type MediaItem, type MediaPlayback } from "./MediaGrid";
export type { MediaViewerLabels } from "./MediaViewer";
export { NewItemsButton, type NewItemsButtonProps } from "./NewItemsButton";
export { PostCard, PostCardAuthor, type PostCardAuthorProps, type PostCardProps } from "./PostCard";
export { PostContent, sanitizedHtmlText, serverSanitizedHtml, type PostContentProps, type SanitizedHtml } from "./PostContent";
export { QuoteCard, type QuoteCardProps } from "./QuoteCard";

// The design tokens a server theme may override (ADR-0059), and the contrast of their colors under a theme.
export type { TokenKind } from "./tokens";
// The colors of role badges (ADR-0071), in the order the staff choose from.
export { badgeColors, type BadgeColor } from "./tokens";
export { themeTokens, type ThemeToken } from "./themeTokens.gen";
export { themeContrast, type ThemeColorValue, type ThemeContrast } from "./themeContrast";

// Layout and text (web-client/16-look-states-and-wording.md §16.1, §16.2): screens lay themselves out with these instead of classes of their own.
export { Box, type BoxProps } from "./Box";
export { Card, type CardElement, type CardProps } from "./Card";
export { SettingsLayout, type SettingsLayoutProps } from "./SettingsLayout";
// The frame of the admin and moderation mode and the parts its list screens share.
export { AdminNavBack, AdminNavSection, AdminShell, type AdminNavBackProps, type AdminNavSectionProps, type AdminShellProps } from "./AdminShell";
export { ListToolbar, type ListToolbarProps } from "./ListToolbar";
export { FeatureCard, type FeatureCardProps } from "./FeatureCard";
export { Hero, type HeroProps } from "./Hero";
export { Cluster, type ClusterProps } from "./Cluster";
export { InfoPage, type InfoPageProps } from "./InfoPage";
export { Grid, type GridProps } from "./Grid";
export type { LayoutElement, Space } from "./layout";
export { Stack, type StackProps } from "./Stack";
export { Heading, type HeadingProps } from "./Heading";
export { SectionHeader, type SectionHeaderProps } from "./SectionHeader";
export { Hint, InlineError, Text, type HintProps, type InlineErrorProps, type TextElement, type TextProps } from "./Text";
export { DialogActions, type DialogActionsProps } from "./DialogActions";
export { Tag, type TagProps } from "./Tag";
export { Form, type FormProps } from "./Form";
export {
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TableRow,
  TableRowHeader,
  type TableBodyProps,
  type TableCellProps,
  type TableColumnHeaderProps,
  type TableHeadProps,
  type TableProps,
  type TableRowHeaderProps,
  type TableRowProps,
} from "./Table";
export {
  ProfileField,
  ProfileFields,
  ProfileHeader,
  ProfileHeaderImage,
  type ProfileFieldProps,
  type ProfileHeaderCount,
  type ProfileHeaderImageProps,
  type ProfileHeaderProps,
} from "./ProfileHeader";
export { AccountItem, type AccountItemProps } from "./AccountItem";
export {
  PostCardAuthorLink,
  PostCardAvatarLink,
  PostCardBoostContext,
  PostCardNameLink,
  PostCardOrigin,
  PostCardTimeLink,
  type PostCardAuthorLinkProps,
  type PostCardAvatarLinkProps,
  type PostCardNameLinkProps,
  type PostCardOriginProps,
  type PostCardTimeLinkProps,
} from "./PostCardParts";
export { NotificationLinks, NotificationOthers, NotificationSentence, type NotificationOthersProps, type NotificationSentenceProps } from "./NotificationRow";
export { PostExcerpt, PostExcerptText, QuoteBlock, type PostExcerptProps, type PostExcerptTextProps, type QuoteBlockProps } from "./Quote";

// The app shell (web-client/09-app-shell.md §9): the frame, the navigation, the page heading and the routes' states.
export { AppShell, AsidePanel, type AppShellProps, type AsidePanelProps } from "./AppShell";
export {
  AppNav,
  AppNavAccount,
  AppNavAction,
  type AppNavActionProps,
  AppNavBrand,
  type AppNavBrandProps,
  AppNavItem,
  AppNavMenu,
  AppNavMenuLink,
  AppNavMenuList,
  AppNavMenuSection,
  type AppNavAccountProps,
  type AppNavBadge,
  type AppNavItemProps,
  type AppNavMenuLinkProps,
  type AppNavMenuListProps,
  type AppNavMenuProps,
  type AppNavMenuSectionProps,
  type AppNavProps,
} from "./AppNavigation";
export { BackLinkHeading, PageHeading, type BackLinkHeadingProps, type PageHeadingProps } from "./PageHeading";
export { RouteLoading, StandalonePage, type RouteLoadingProps, type StandalonePageProps } from "./RouteStates";
export { ServerNameLink, type ServerNameLinkProps } from "./ServerNameLink";

// The display of a poll (FR-112).
export {
  Poll,
  PollChoices,
  PollHint,
  PollResults,
  PollSummary,
  type PollChoicesProps,
  type PollHintProps,
  type PollOptionProps,
  type PollProps,
  type PollResultsProps,
  type PollSummaryProps,
} from "./Poll";
export { EmojiTile, type EmojiTileProps } from "./EmojiTile";
export { CodeBlock, type CodeBlockProps } from "./CodeBlock";
export { PreviewImage, type PreviewImageProps } from "./PreviewImage";
export { ThemePreview, type ThemePreviewProps } from "./ThemePreview";
export { ReactionChip, ReactionEmoji, ReactionRow, type ReactionChipProps, type ReactionEmojiProps, type ReactionRowProps } from "./ReactionChip";
export { DetailSection, type DetailSectionProps } from "./DetailSection";
export { BannerImage, type BannerImageProps } from "./BannerImage";
export { IconLabel, type IconLabelProps } from "./IconLabel";
export { Callout, type CalloutProps } from "./Callout";
export { CodeList, type CodeListProps } from "./CodeList";

// The parts of the post form (FR-101): an image of a draft, a framed group of controls, the floating compose button.
export { AttachmentItem, type AttachmentItemProps } from "./AttachmentItem";
export { Fieldset, type FieldsetProps } from "./Fieldset";
export { FloatingButton, type FloatingButtonProps } from "./FloatingButton";
export { OutboxItem, type OutboxItemProps } from "./OutboxItem";
