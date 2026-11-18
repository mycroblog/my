// SPDX-License-Identifier: AGPL-3.0-only

// The features of the web client (ADR-0025 §2–§4), a directory each: what a screen of the app places, built from
// @my/app-components and fed by @my/app-data. A feature imports no other feature; the app wires them together.

export { parseAuthPage, type AuthPage, type ScreenName } from "./auth/data";
export {
  AnnouncementList,
  AnnouncementPage,
  loadAnnouncement,
  loadAnnouncements,
  useAnnouncementTitle,
} from "./announcements/announcements";
export { AuthScreen } from "./auth/screen";
export { enterCall, leaveCall, muteCall, updateCall, useCall, type Call, type CallControls } from "./call/store";
export { ClosedScreen, type ClosedScreenProps } from "./closure/closed";
export {
  ArticleEditor,
  ArticleView,
  type ArticleEditorProps,
  type ArticleLeaving,
  type ArticleOpening,
  type ArticleSpecimen,
  type ArticleViewProps,
} from "./compose/article";
export { emptyArticle, type ArticleDraft } from "./compose/article-model";
export {
  conversationPath,
  ConversationList,
  ConversationView,
  loadConversation,
  loadConversations,
  type ConversationReplyTo,
} from "./conversations/conversations";
export { DraftsView, type DraftsState, type DraftsViewProps } from "./compose/drafts";
export { InPlaceReply, type InPlaceReplyProps } from "./compose/in-place";
export { ComposeLayer, type ComposeLayerProps, type ComposeSpecimen } from "./compose/layer";
export {
  emptyDraft as emptyComposeDraft,
  type Draft as ComposeDraft,
  type Opening as ComposeOpening,
  type Target as ComposeTarget,
} from "./compose/model";
export { LatestNotifications } from "./notifications/latest";
export { loadNotifications, Notifications } from "./notifications/notifications";
export { Held, HeldOf, loadHeld, loadHeldOf } from "./notifications/held";
export { loadWaitingRequests, WaitingRequests } from "./notifications/waiting";
export { FollowRequests, loadFollowRequests } from "./notifications/follow-requests";
export { PostActions, type ComposeRequest } from "./post-actions";
export { inAnotherLanguage, TranslatablePost, usePostTranslation, type PostTranslationState } from "./post-translation";
export { Follows, loadFollows } from "./profile/follows";
export { loadOwnProfile, loadProfile, OwnProfile, Profile, useProfileTitle } from "./profile/profile";
export { loadOwnApp, OwnApp, useOwnAppTitle } from "./settings/apps/app";
export { loadAppsSettings } from "./settings/apps/apps";
export { AuthorizedAppScreen, useAuthorizedAppTitle } from "./settings/apps/authorized";
export { appsPath, appsPaths, useAppsTitle, type AppScreenProps } from "./settings/apps/parts";
export { AppRegister, loadAppRegister } from "./settings/apps/register";
export { AppToken } from "./settings/apps/token";
export { useConfirmation, useProviderReturn, type ConfirmationOptions, type Confirmed } from "./settings/confirmation";
export {
  ProfileAvatarField,
  profileEditPath,
  ProfileNameField,
  usePickedPicture,
  useProfileIdentity,
  type PickedPicture,
  type ProfileIdentity,
} from "./profile-identity";
export {
  loadSettingsEntry,
  loadSettingsFacts,
  settingsEntries,
  type SettingsAddress,
  type SettingsEntry,
  type SettingsEntryProps,
  type SettingsFacts,
  type SettingsGroupId,
  type SettingsSessionProps,
  type SettingsSubProps,
} from "./settings/entries";
export { InviteCreate } from "./settings/invites/create";
export { OwnInviteScreen, useOwnInviteTitle, type InviteScreenProps } from "./settings/invites/invite";
export { loadInvitesSettings } from "./settings/invites/invites";
export { invitesPath, invitesPaths, useInviteCreateTitle } from "./settings/invites/parts";
export { securityPath, securityPaths, useSecurityTitle } from "./settings/security/parts";
export { SecurityPasskey } from "./settings/security/passkey";
export { SecurityPassword } from "./settings/security/password";
export { loadSecuritySettings, SecuritySettings } from "./settings/security/security";
export { SecurityTwoFactor } from "./settings/security/two-factor";
export {
  loadModerationAction,
  ModerationActionScreen,
  useModerationActionTitle,
  type ModerationActionProps,
} from "./settings/account-action";
export { AccountStatusScreen, loadAccountStatus, useAccountStatusTitle } from "./settings/account-status";
export { AccountExportScreen, loadAccountExport, useAccountExportTitle } from "./settings/account-export";
export { AccountImportScreen, loadAccountImport, useAccountImportTitle } from "./settings/account-import";
export {
  AccountAutoDelete,
  loadAccountAutoDelete,
  useAccountAutoDeleteTitle,
  type AccountAutoDeleteProps,
} from "./settings/account-auto-delete";
export { AccountAliasesScreen, loadAccountAliases, useAccountAliasesTitle } from "./settings/account-aliases";
export { loadProfileEdit, ProfileEdit, useProfileEditTitle, type ProfileEditProps } from "./settings/profile-edit";
export { FilterEdit, loadFilterEdit, useFilterEditTitle, type FilterEditProps } from "./settings/filter-edit";
export { loadRelationshipSettings, RelationshipListScreen, useRelationshipListTitle } from "./settings/relationships";
export { SettingsBeside, settingsEntryPath, settingsFactsWaitMs, SettingsIndex } from "./settings/menu";
export {
  SettingsEntryScreen,
  SettingsMissing,
  useSettingsTitle,
  type SettingsEntryScreenProps,
} from "./settings/screen";
export { SignOutDialog, type SignOutDialogProps } from "./settings/sign-out";
export { UnsavedSettings } from "./settings/unsaved";
export { loadReactors, ReactorList } from "./thread/reactors";
export { loadThread, Thread } from "./thread/thread";
export { HomeTimeline, loadHomeTimeline } from "./timeline/home";
export { loadPublicTimeline, PublicTimeline } from "./timeline/public";
export { Bookmarks, loadBookmarks } from "./timeline/bookmarks";
export { ListIndex, ListTimeline, loadLists, loadListTimeline } from "./timeline/lists";
export { FindPeople, loadPeople } from "./people/people";
export { Search } from "./search/search";
export { ListMembers, loadListMembers } from "./timeline/list-members";
export { loadTagTimeline, TagTimeline } from "./timeline/tag";
export { useWelcomeTitle, Welcome, type WelcomeProps, type WelcomeSpecimen } from "./welcome/welcome";
