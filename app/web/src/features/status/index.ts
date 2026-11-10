// SPDX-License-Identifier: AGPL-3.0-only
// The status feature: a post's card with the viewer's actions on it (boost, favourite, bookmark, edit, pin,
// delete, copy the link, reply), a post's page with its thread, and its edit history (FR-106 to FR-109,
// FR-113, FR-403). Routes and other features use it through this file only (web-client/README.md A3).
export {
  cacheAnsweredRequest,
  type InteractionRequest,
  pinnedStatusesQueryOptions,
  type Status,
  statusContextQueryOptions,
  statusHistoryQueryOptions,
  statusPath,
  statusQueryOptions,
} from "./api/status";
export { CompactStatus } from "./components/QuotedStatus";
export { StatusCard } from "./components/StatusCard";
// What a post that the viewer's keyword filters fold says in its place (ADR-0157 §2).
export { useFilteredLabel } from "./hooks/useFilteredLabel";
export { ReportCommentText } from "./components/ReportCommentText";
export { StatusHistory } from "./components/StatusHistory";
export { StatusThread } from "./components/StatusThread";
