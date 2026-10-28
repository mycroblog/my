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
export { celebrationClasses, celebrationMs, useCelebration, type Celebration } from "./celebration";
export {
  IconButton,
  IconButtonLink,
  IconToggleButton,
  type IconButtonLinkProps,
  type IconButtonProps,
  type IconToggleButtonProps,
} from "./IconButton";
export { ActionCount, type ActionCountProps } from "./ActionCount";
export { Segmented, SegmentedItem, type SegmentedItemProps, type SegmentedProps } from "./Segmented";

// Overlays, tabs and announcements.
export { ConfirmDialog, type ConfirmDialogProps } from "./ConfirmDialog";
export {
  AnnouncerProvider,
  useAnnouncer,
  type Announce,
  type AnnouncerProviderProps,
  type Politeness,
} from "./Announcer";
export { useNotice, type Notice } from "./Notice";
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
export {
  Tab,
  TabList,
  TabPanel,
  Tabs,
  type TabListProps,
  type TabPanelProps,
  type TabProps,
  type TabsProps,
} from "./Tabs";
export {
  ToastProvider,
  useToast,
  type ToastApi,
  type ToastOptions,
  type ToastProviderProps,
  type ToastTone,
} from "./Toast";
export { Tooltip, TooltipTrigger, type TooltipProps, type TooltipTriggerProps } from "./Tooltip";

// Display.
export { Avatar, type AvatarProps } from "./Avatar";
export { Badge, type BadgeProps } from "./Badge";
export { RailItem, type RailItemBadge, type RailItemProps } from "./RailItem";
export { NavMenu, navMenuGroupLimit, type NavMenuGroup, type NavMenuItem, type NavMenuProps } from "./NavMenu";
export { RoleBadge, RoleBadgeList, type RoleBadgeListProps, type RoleBadgeProps } from "./RoleBadge";
export {
  DescriptionItem,
  DescriptionList,
  type DescriptionItemProps,
  type DescriptionListProps,
} from "./DescriptionList";
export { EmptyState, type EmptyStateProps, type HeadingLevel } from "./EmptyState";
export { NoticeBanner, type NoticeBannerProps } from "./NoticeBanner";
export { OfflineBanner, type OfflineBannerProps } from "./OfflineBanner";
export { Blurhash, type BlurhashProps } from "./Blurhash";
export { AutoplayContext, type ImageCandidate, type ImageSources, Picture, type PictureProps } from "./Picture";
export { QrCode, type QrCodeProps } from "./QrCode";
export { RegionError, type RegionErrorProps } from "./RegionError";
export { SignInState, type SignInStateProps } from "./SignInState";
export { Skeleton, type SkeletonProps } from "./Skeleton";
export {
  AccountRowSkeleton,
  FactsCardSkeleton,
  NotificationRowSkeleton,
  PostSkeleton,
  type SkeletonRowProps,
  StageSkeleton,
  VoiceRoomCardSkeleton,
} from "./SkeletonRows";
export { Spinner, type SpinnerProps } from "./Spinner";
export { BrandIcon, type BrandIconProps, type BrandLogo } from "./BrandImage";
export { BrandMark, type BrandMarkProps } from "./BrandMark";
export { BrandPreview, BrandPreviewList, type BrandPreviewProps } from "./BrandPreview";
export { Wordmark, type WordmarkProps } from "./Wordmark";
export { StatusMessage, type StatusMessageProps } from "./StatusMessage";
export {
  AutosaveControl,
  SaveIndicator,
  type AutosaveControlProps,
  type SaveIndicatorLabels,
  type SaveIndicatorProps,
  type SaveState,
} from "./SaveIndicator";
export { ChangeNotice, type ChangeNoticeProps } from "./ChangeNotice";
export { FormActionBar, type FormActionBarProps, type FormActionResult } from "./FormActionBar";
export { VisuallyHidden, type VisuallyHiddenProps } from "./VisuallyHidden";

// Data visualisation (ADR-0113): numbers, trends, shares and amounts, each with its values in text.
export { BreakdownBar, type BreakdownBarProps, type BreakdownItem } from "./BreakdownBar";
export { ProgressMeter, type ProgressMeterProps } from "./ProgressMeter";
export { Sparkline, type SparklineProps } from "./Sparkline";
export { AgeBadge, type AgeBadgeProps, type AgeLevel } from "./AgeBadge";
export { TriageLayout, type TriageLayoutProps } from "./TriageLayout";
export { SeverityMark, type SeverityMarkProps } from "./SeverityMark";
export { StatTile, type StatTileChange, StatTileGrid, type StatTileGridProps, type StatTileProps } from "./StatTile";
export {
  TrendChart,
  type TrendChartLabels,
  type TrendChartProps,
  type TrendChartView,
  type TrendPeriod,
  type TrendSeries,
} from "./TrendChart";

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
  CallConnectedIcon,
  CallConnectingIcon,
  CallReconnectingIcon,
  ExternalLinkIcon,
  FederatedIcon,
  FinanceIcon,
  HomeIcon,
  ImageIcon,
  InviteIcon,
  LanguageIcon,
  LeaveIcon,
  ListenIcon,
  LocalIcon,
  MenuIcon,
  MicrophoneIcon,
  MediaFailedIcon,
  MenuIndicatorIcon,
  MicrophoneOffIcon,
  ModerationIcon,
  MoreIcon,
  MoveDownIcon,
  MoveUpIcon,
  NotificationsIcon,
  PinIcon,
  PlusIcon,
  PollIcon,
  ProfileIcon,
  QuoteIcon,
  RaiseHandIcon,
  RepliesIcon,
  ReplyIcon,
  SearchIcon,
  SettingsIcon,
  ShareIcon,
  SignInIcon,
  SignUpIcon,
  StarIcon,
  SubmenuIcon,
  type ToggleIconProps,
  VoiceIcon,
  WarningIcon,
} from "./Icons";
// The icons of the pages of the grouped menus (NavMenu).
export {
  BadgeIcon,
  SupporterIcon,
  DisplayIcon,
  AutoDeleteIcon,
  FollowRequestIcon,
  BlockIcon,
  MuteIcon,
  DomainMuteIcon,
  PasswordIcon,
  TwoFactorIcon,
  PasskeyIcon,
  LinkedAccountIcon,
  AuthorizedAppIcon,
  DeveloperIcon,
  ImportExportIcon,
  AliasIcon,
  MoveAccountIcon,
  AccountStatusIcon,
  ArchiveIcon,
  OverviewIcon,
  ReportIcon,
  AppealIcon,
  AuditLogIcon,
  AccountsIcon,
  RegistrationIcon,
  SignUpBlockIcon,
  DomainActionIcon,
  ServerSettingsIcon,
  RulesIcon,
  ThemeIcon,
  DocumentIcon,
  AnnouncementIcon,
  RelayIcon,
  WebhookIcon,
  LoginProviderIcon,
  RoleIcon,
  JobIcon,
  ErrorIcon,
  ClosureIcon,
  CustomEmojiIcon,
} from "./Icons";
// The icons of the states of a queue's items (StatusBadge, AgeBadge).
export { AssignedIcon, LegalIcon, SpamIcon, UnassignedIcon, WaitingIcon } from "./Icons";
export { type PostVisibility, VisibilityIcon, type VisibilityIconProps } from "./Icons";
export { StatusBadge, type StatusBadgeProps, VisibilityMark, type VisibilityMarkProps } from "./StatusBadge";
// The follow button of a profile and of a row of accounts.
export { FollowButton, type FollowButtonProps, type FollowStatus } from "./FollowButton";

// Timeline.
export { ContentWarning, type ContentWarningProps } from "./ContentWarning";
export {
  CustomEmoji,
  EmojiText,
  type CustomEmojiImage,
  type CustomEmojiProps,
  type EmojiTextProps,
} from "./CustomEmoji";
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
export { MediaLoadFailure, type MediaLoadFailureProps, type MediaLoadLabels } from "./MediaLoadFailure";
export { NewItemsButton, type NewItemsButtonProps } from "./NewItemsButton";
export { PostCard, PostCardAuthor, type PostCardAuthorProps, type PostCardProps } from "./PostCard";
export {
  PostContent,
  sanitizedHtmlText,
  serverSanitizedHtml,
  type PostContentProps,
  type SanitizedHtml,
} from "./PostContent";
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
export { AdminNavBack, AdminShell, type AdminNavBackProps, type AdminShellProps } from "./AdminShell";
export { ListToolbar, type ListToolbarProps } from "./ListToolbar";
// An open voice chat room in a list (ADR-0100 §8).
export { VoiceRoomCard, type VoiceRoomCardProps, type VoiceRoomCardTag } from "./VoiceRoomCard";
// The stage of a voice chat room and the controls of its call.
export { LivePill, type LivePillProps } from "./LivePill";
export {
  StageAvatar,
  StageGrid,
  type StageAvatarLabels,
  type StageAvatarProps,
  type StageAvatarReaction,
  type StageGridProps,
} from "./StageAvatar";
export {
  CallDock,
  CallDockButton,
  type CallDockButtonProps,
  type CallDockProps,
  CallMicrophoneButton,
  type CallMicrophoneButtonProps,
} from "./CallDock";
export { CallConnectionBadge, type CallConnectionBadgeProps, type CallConnectionState } from "./CallConnectionBadge";
export { Cluster, type ClusterProps } from "./Cluster";
export { InfoPage, type InfoPageProps } from "./InfoPage";
// The landing page's parts read in order, and the steps that follow an action (web-client/screens/landing.md).
export { SectionProgress, type SectionProgressPart, type SectionProgressProps } from "./SectionProgress";
export { Steps, type Step, type StepsProps } from "./Steps";
// Where a flow of steps stands (the report dialog).
export { Stepper, type StepperProps } from "./Stepper";
// Details folded under their summary (the about page).
export { Disclosure, type DisclosureProps } from "./Disclosure";
export { Grid, type GridProps } from "./Grid";
export type { LayoutElement, Space } from "./layout";
export { Stack, type StackProps } from "./Stack";
export { Heading, type HeadingProps } from "./Heading";
export { SectionHeader, type SectionHeaderProps } from "./SectionHeader";
export {
  Hint,
  InlineError,
  Text,
  type HintProps,
  type InlineErrorProps,
  type TextElement,
  type TextProps,
} from "./Text";
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
  PostCardContext,
  type PostCardContextProps,
  PostCardNameLink,
  PostCardOrigin,
  PostCardTimeLink,
  type PostCardAuthorLinkProps,
  type PostCardAvatarLinkProps,
  type PostCardNameLinkProps,
  type PostCardOriginProps,
  type PostCardTimeLinkProps,
} from "./PostCardParts";
export {
  NotificationLinks,
  NotificationOthers,
  NotificationSentence,
  type NotificationOthersProps,
  type NotificationSentenceProps,
} from "./NotificationRow";
export {
  PostExcerpt,
  PostExcerptText,
  QuoteBlock,
  type PostExcerptProps,
  type PostExcerptTextProps,
  type QuoteBlockProps,
} from "./Quote";

// The app shell (web-client/09-app-shell.md §9): the frame, the navigation, the page heading and the routes' states.
export {
  AppShell,
  AsidePanel,
  ShellDock,
  type AppShellProps,
  type AsidePanelProps,
  type ShellDockProps,
} from "./AppShell";
export { CallBar, type CallBarProps } from "./CallBar";
export { AvatarStack, type AvatarStackItem, type AvatarStackProps } from "./AvatarStack";
export { MiniPlayer, type MiniPlayerAvatar, type MiniPlayerProps } from "./MiniPlayer";
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
  AppNavTopAction,
  type AppNavTopActionProps,
  type AppNavTopMenu,
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
export {
  ReactionChip,
  ReactionEmoji,
  ReactionRow,
  type ReactionChipProps,
  type ReactionEmojiProps,
  type ReactionRowProps,
} from "./ReactionChip";
export { Section, type SectionProps } from "./Section";
export { BannerImage, type BannerImageProps } from "./BannerImage";
export { IconLabel, type IconLabelProps } from "./IconLabel";
export { Callout, type CalloutProps } from "./Callout";
export { CodeList, type CodeListProps } from "./CodeList";

// The parts of the post form (FR-101): an image of a draft, a framed group of controls, the floating compose button.
export { AttachmentItem, type AttachmentItemProps } from "./AttachmentItem";
export { Fieldset, type FieldsetProps } from "./Fieldset";
export { FloatingButton, type FloatingButtonProps } from "./FloatingButton";
export { OutboxItem, type OutboxItemProps } from "./OutboxItem";
