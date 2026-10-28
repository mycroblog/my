// SPDX-License-Identifier: AGPL-3.0-only
// The announcements feature (FR-726): the server's announcements to every local user above the home timeline,
// with marking them read, and what the event stream says about them. Administrators write them on the admin
// screen (features/admin). Routes and other features use it through this file only (web-client/README.md A3).
export {
  catchUpAnnouncements,
  cacheAnnouncement as receiveAnnouncement,
  removeAnnouncement,
} from "./api/announcements";
export { Announcements } from "./components/Announcements";
