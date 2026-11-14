// SPDX-License-Identifier: AGPL-3.0-only

// The server data of the web client (ADR-0025 §2, ADR-0028 §6, §7, §18): the client API through
// @mycroblog/mykit, and TanStack Query as the one cache of what it answers. Features read and write the server only
// through the hooks and queries here, and never copy what they return into state or context.

export { createAppData, createQueryClient, type AppData } from "./app-data";
export { receiveHomeGap, receiveHomeStatuses, useHomeArrivals, type Arrival } from "./arrivals";
export {
  accountQuery,
  accountStatusesQuery,
  changeRelationship,
  featuredHashtagsQuery,
  relationshipQuery,
  useAccount,
  useAccountStatuses,
  useChangeRelationship,
  useFeaturedHashtags,
  useRelationship,
  useViewer,
  viewerQuery,
  type Account,
  type AccountPosts,
  type FeaturedHashtag,
  type Relationship,
  type RelationshipChange,
} from "./accounts";
export {
  cachePostedStatus,
  defaultComposeSettings,
  deleteDraft,
  findCachedStatus,
  MediaFailedError,
  postStatus,
  refusal,
  saveDraft,
  uploadAttachment,
  useComposeSettings,
  useComposeWrites,
  useStatus,
  type Audience,
  type ComposeSettings,
  type ComposeWrites,
  type CreateStatus,
  type DraftRequest,
  type Interaction,
  type MediaAttachment,
  type SavedDraft,
  type Visibility,
} from "./compose";
export {
  notificationMarkerQuery,
  notificationRequestSummaryQuery,
  notificationsQuery,
  receiveNotification,
  useAnswerRequest,
  useInteractionRequest,
  useNotificationReads,
  useNotificationRequestSummary,
  useNotifications,
  useUnreadNotifications,
  type InteractionRequest,
  type NotificationFilter,
  type NotificationItem,
  type NotificationReads,
  type NotificationRequestSummary,
  type NotificationType,
} from "./notifications";
export type { PagedList } from "./paged";
export { AppDataProvider } from "./provider";
export { openSession } from "./session";
export {
  appPath,
  finishSignIn,
  SignInError,
  startSignIn,
  type SignInFailure,
  type SignInOptions,
  type SignInStorage,
} from "./sign-in";
export { updateCachedStatuses, type Status, type StatusPage } from "./statuses";
export { startStream } from "./stream";
export { statusContextQuery, statusQuery, useThread, type StatusContext, type Thread } from "./thread";
export { homeTimelineQuery, useHomeTimeline, type HomeTimeline } from "./timeline";
