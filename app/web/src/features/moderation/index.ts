// SPDX-License-Identifier: AGPL-3.0-only
// The moderation feature (ADR-0058).
//
// On the user's side: the account status page, with the moderation actions on the user's own account and
// appealing them (FR-715, FR-707).
//
// On the staff's side, the moderation screens: the reports (FR-701, FR-710), the review of appeals (FR-707),
// the accounts and the actions on them (FR-714, FR-703), the review of registrations awaiting approval
// (FR-721), the blocks on sign-ups (FR-719), the moderation actions on domains (FR-702), the health of remote
// servers with resuming deliveries to them (federation.md §7.3), and the audit log (FR-704). Each screen is for
// a viewer whose roles have its permission (manage_reports, manage_accounts, manage_domains, view_audit); the
// server checks it on every request. The navigation's Moderation entry and the menu show what awaits the
// moderators (FR-712).
//
// It also exports what the server administration screens (the admin feature) share with these: the
// permissions, the paging of the lists, and the parts of the screens.
// Routes and other features use it through this file only (web-client/README.md A3).
export { ownActionsQueryOptions } from "./api/actions";
export { AccountStatus } from "./components/AccountStatus";
// The menus and the navigation's entry.
export { refreshAdminCounts } from "./api/counts";
export { ModerationPages, useModerationBadges } from "./components/ModerationMenu";
export { moderationPages } from "./components/moderationPages";
export { useModerationEntry } from "./hooks/useModerationCounts";
// What the server administration screens share.
export { isMissingPermission, pageQuery, type Permission, refreshAuditLog } from "./api/staff";
export {
  DateTime,
  DetailSection,
  DomainName,
  MissingPermission,
  RecordList,
  RecordTable,
  RequirePermission,
  useFocusAfterChange,
} from "./components/parts";
export { type StaffPage, type StaffPageBadge, StaffPages, useShownStaffPages } from "./components/StaffPages";
export { type PagedList, type PagedPages, toPagedList, wholeList } from "./hooks/pagedList";
export { useHasPermission, useViewerPermissions } from "./hooks/usePermissions";
// The roles of the instance and of an account, which the roles screen (manage_roles) shares.
export { accountRolesQueryOptions, rolesQueryOptions } from "./api/roles";
// Remote servers, domain actions and the audit log, for manage_domains and view_audit.
export {
  auditActions,
  type AuditLogFilter,
  auditLogMaxPages,
  auditLogQueryOptions,
  auditTargetKinds,
  availabilityEventsQueryOptions,
  type DomainActionFilter,
  domainActionFilters,
  domainActionsQueryOptions,
  type InstanceFilter,
  instanceFilters,
  instanceQueryOptions,
  instancesQueryOptions,
} from "./api/staff";
export { AuditLog } from "./components/AuditLog";
export { DomainActions } from "./components/DomainActions";
export { InstanceDetail } from "./components/InstanceDetail";
export { InstanceList } from "./components/InstanceList";
// The review of registrations and the blocks on sign-ups, for manage_accounts.
export { registrationsQueryOptions } from "./api/registrations";
export { RegistrationList } from "./components/RegistrationList";
export { type SignupBlockFilter, signupBlockFilters, signupBlocksQueryOptions } from "./api/signupBlocks";
export { SignupBlocks } from "./components/SignupBlocks";
// The invites that let people sign up in the invite registration mode (FR-001), for manage_invites.
export { invitesQueryOptions } from "./api/invites";
export { instanceInfoQueryOptions as moderationInstanceQueryOptions } from "./api/reports";
export { Invites } from "./components/Invites";
// The report queue and a report's page (FR-701, FR-710, NFR-MOD-4), for manage_reports.
export { type ReportFilter, reportQueryOptions, reportsQueryOptions } from "./api/reports";
export { ReportDetailView } from "./components/ReportDetail";
export { ReportQueue } from "./components/ReportQueue";
// The account search and an account's page with its moderation actions (FR-714, FR-703, FR-713), for manage_accounts.
export {
  adminAccountContactFields,
  adminAccountOrigins,
  adminAccountQueryOptions,
  type AdminAccountSearch,
  adminAccountSearchLimit,
  adminAccountsQueryOptions,
  adminAccountStates,
} from "./api/accounts";
export { AccountDetailView } from "./components/AccountDetail";
export { AccountSearch } from "./components/AccountSearch";
// The review of appeals of moderation actions (FR-707), for manage_accounts.
export { type AppealFilter, appealQueryOptions, appealsQueryOptions } from "./api/appeals";
export { AppealDetailView } from "./components/AppealDetail";
export { AppealQueue } from "./components/AppealQueue";
