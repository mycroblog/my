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
export { useConfirmation, useProviderReturn, type ConfirmationOptions, type Confirmed } from "./settings/confirmation";
export {
  loadSettingsEntry,
  settingsEntries,
  type SettingsAddress,
  type SettingsEntry,
  type SettingsEntryProps,
  type SettingsFacts,
  type SettingsGroupId,
  type SettingsSubProps,
} from "./settings/entries";
export { securityPath, securityPaths, useSecurityTitle } from "./settings/security/parts";
export { SecurityPasskey } from "./settings/security/passkey";
export { SecurityPassword } from "./settings/security/password";
export { loadSecuritySettings, SecuritySettings } from "./settings/security/security";
export { SecurityTwoFactor } from "./settings/security/two-factor";
export { SettingsBeside, settingsEntryPath, SettingsIndex } from "./settings/menu";
export { SettingsEntryScreen, useSettingsTitle, type SettingsEntryScreenProps } from "./settings/screen";
export { SignOutDialog, type SignOutDialogProps } from "./settings/sign-out";
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
