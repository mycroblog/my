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
export { ComposeLayer, type ComposeLayerProps } from "./compose/layer";
export {
  emptyDraft as emptyComposeDraft,
  type Draft as ComposeDraft,
  type Target as ComposeTarget,
} from "./compose/model";
export { LatestNotifications } from "./notifications/latest";
export { loadNotifications, Notifications } from "./notifications/notifications";
export { PostActions, type ComposeRequest } from "./post-actions";
export { loadOwnProfile, loadProfile, OwnProfile, Profile, useProfileTitle } from "./profile/profile";
export { loadThread, Thread } from "./thread/thread";
export { HomeTimeline, loadHomeTimeline } from "./timeline/home";
export { loadPublicTimeline, PublicTimeline } from "./timeline/public";
