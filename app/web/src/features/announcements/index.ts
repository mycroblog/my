// SPDX-License-Identifier: AGPL-3.0-only
// The announcements feature (FR-726, ADR-0125): the server's announcements to every local user, above the home
// timeline below xl and in the panel of the third column from xl, the list page with the ended ones, marking them
// read, and what the event stream says about them. Administrators write them on the admin screen (features/admin).
// Routes and other features use it through this file only (web-client/README.md A3).
export {
  announcementHistoryQueryOptions,
  catchUpAnnouncements,
  cacheAnnouncement as receiveAnnouncement,
  removeAnnouncement,
} from "./api/announcements";
export { AnnouncementHistory, AnnouncementHistorySkeleton } from "./components/AnnouncementHistory";
export { Announcements } from "./components/Announcements";
export { AnnouncementsPanel } from "./components/AnnouncementsPanel";
export { announcementPath } from "./components/AnnouncementTitle";
export { useUnreadAnnouncementCount } from "./hooks/useAnnouncements";
