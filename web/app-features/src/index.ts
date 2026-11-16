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
export { loadWaitingRequests, WaitingRequests } from "./notifications/waiting";
export { FollowRequests, loadFollowRequests } from "./notifications/follow-requests";
export { PostActions, type ComposeRequest } from "./post-actions";
export { inAnotherLanguage, TranslatablePost, usePostTranslation, type PostTranslationState } from "./post-translation";
export { Follows, loadFollows } from "./profile/follows";
/** The features' strings in a locale: what a story's play presses and reads by the words a screen shows. */
export { stringsIn as featureStringsIn } from "./locale";
export type { MessageId as FeatureMessageId } from "./messages.gen";
export { loadOwnProfile, loadProfile, OwnProfile, Profile, useProfileTitle } from "./profile/profile";
export { loadReactors, ReactorList } from "./thread/reactors";
export { loadThread, Thread } from "./thread/thread";
export { HomeTimeline, loadHomeTimeline } from "./timeline/home";
export { loadPublicTimeline, PublicTimeline } from "./timeline/public";
export { Bookmarks, loadBookmarks } from "./timeline/bookmarks";
export { ListIndex, ListTimeline, loadLists, loadListTimeline } from "./timeline/lists";
export {
  DiscoveryLayoutProvider,
  FindPeople,
  loadPeople,
  useDiscoveryLayout,
  type DiscoveryLayout,
} from "./people/people";
export { Search } from "./search/search";
export { ListMembers, loadListMembers } from "./timeline/list-members";
export { loadTagTimeline, TagTimeline } from "./timeline/tag";
