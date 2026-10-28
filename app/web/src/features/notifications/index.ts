// SPDX-License-Identifier: AGPL-3.0-only
// The notifications feature (FR-205): the notifications page, its read marker, and the unread badge of the
// navigation. Other features and routes use it through this file only (web-client/README.md A3).
export {
  notificationFilters,
  notificationMarkerQueryOptions,
  notificationsQueryOptions,
  type NotificationFilter,
} from "./api/notifications";
export { catchUpNotifications, receiveNotification } from "./api/live";
export { NotificationsSkeleton } from "./components/NotificationsSkeleton";
export { NotificationsView } from "./components/NotificationsView";
export { useUnreadNotificationCount } from "./hooks/useUnreadNotifications";
