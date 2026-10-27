// SPDX-License-Identifier: AGPL-3.0-only
// The server administration feature (ADR-0058): the server's settings (its name and descriptions in each language
// and its policies), its rules, its terms of service and privacy policy (#489), the custom emoji (ADR-0051), the announcements (FR-726), the webhooks (ADR-0060), the federation relays (ADR-0061), and the queue dashboard of the job queue with running failed jobs again (ADR-0064). Each screen
// is for a viewer whose roles have its permission (manage_settings, manage_rules, manage_custom_emojis, manage_announcements, manage_webhooks, manage_jobs); the server checks it on every
// request. The parts and the permissions that the screens share with the moderation screens come from the
// moderation feature.
// Routes and other features use it through this file only (web-client/README.md A3).
export { adminPages, AdminPages, useAdminEntry } from "./components/AdminMenu";
// The queue dashboard: the jobs of the job queue by state, their attempts, and running failed ones again (ADR-0064), for manage_jobs.
export { type JobFilter, jobFilters, jobQueueQueryOptions, jobsQueryOptions } from "./api/jobs";
export { JobDashboard } from "./components/JobDashboard";
// The terms of service and the privacy policy (#489), for manage_settings.
export { instanceTextsQueryOptions } from "./api/documents";
export { DocumentSettings } from "./components/DocumentSettings";
// The server's settings: its name and descriptions in each language, and its policies, for manage_settings.
export { instanceQueryOptions as ownInstanceQueryOptions } from "./api/documents";
export { instanceSettingsQueryOptions } from "./api/settings";
export { ServerSettings } from "./components/ServerSettings";
// The server's rules (FR-705), for manage_rules.
export { rulesQueryOptions } from "./api/rules";
export { RuleSettings } from "./components/RuleSettings";
// The server's theme: overrides of the design tokens and a custom CSS (ADR-0059), for manage_settings.
export { themeQueryOptions } from "./api/theme";
export { ThemeSettings } from "./components/ThemeSettings";
// The custom emoji: registering this server's and managing those of other servers (ADR-0051), and deciding the
// requests of the holders of create_custom_emojis (ADR-0070), for manage_custom_emojis.
export { adminCustomEmojisQueryOptions, type CustomEmojiFilter } from "./api/customEmojis";
export { adminCustomEmojiRequestsQueryOptions, type CustomEmojiRequestState } from "./api/customEmojiRequests";
export { CustomEmojiSettings, type CustomEmojiTab } from "./components/CustomEmojiSettings";
// What the user's page of their own custom emoji (features/custom-emojis) shares with the screen: the rules of an
// emoji, how a sentence names one, and the names of the states of a request.
export { acceptedEmojiTypes, categoryLimit, federatedEmojiBytes, maxAliases, shortcodePattern } from "./api/customEmojis";
export { requestStateLabels } from "./components/CustomEmojiRequests";
export { parseAliases } from "./hooks/useCustomEmojis";
// The OAuth apps: every client by registration route, disabling them, and denying the hosts of Client ID Metadata
// Documents (ADR-0090 §1), for manage_oauth_apps.
export { hostDenialsQueryOptions, type OAuthClientFilter, oauthClientsQueryOptions } from "./api/oauthApps";
export { OAuthAppSettings, type OAuthAppTab } from "./components/OAuthAppSettings";
// The server's announcements to every local user (FR-726), for manage_announcements.
export { adminAnnouncementsQueryOptions } from "./api/announcements";
export { AnnouncementSettings } from "./components/AnnouncementSettings";
// The external login providers: setting them up, changing, disabling and deleting them (ADR-0106), for manage_login_providers.
export { loginProvidersQueryOptions } from "./api/loginProviders";
export { LoginProviderSettings } from "./components/LoginProviderSettings";
// The webhooks: setting them up, their deliveries and sending one again (ADR-0060), for manage_webhooks.
export { webhookDeliveriesQueryOptions, webhookQueryOptions, webhooksQueryOptions } from "./api/webhooks";
export { WebhookDeliveries } from "./components/WebhookDeliveries";
export { WebhookSettings } from "./components/WebhookSettings";
// The federation relays: registering, joining, leaving and deleting them (ADR-0061), for manage_settings.
export { relaysQueryOptions } from "./api/relays";
export { RelaySettings } from "./components/RelaySettings";
// The roles: creating them and changing their positions and permissions (FR-720), for manage_roles. Their queries
// are those of the moderation feature, which shows an account's roles.
export { RoleSettings } from "./components/RoleSettings";
// The instance's costs: recording them, their totals by week, month and year, and the state of the billing imports
// (ADR-0067, ADR-0068), for manage_finance.
export {
  adminIncomeTotalsQueryOptions,
  billingImportsQueryOptions,
  costItemsQueryOptions,
  costTotalsQueryOptions,
  type IncomeMovementFilter,
  incomeKinds,
  incomeMovementsQueryOptions,
} from "./api/finance";
export { FinanceSettings } from "./components/FinanceSettings";
// The supporter program: enabling it, its plans and their subscriptions (ADR-0072), for manage_supporters.
export {
  type SupporterSubscriptionFilter,
  supporterPlansQueryOptions,
  supporterProgramQueryOptions,
  supporterSubscriptionStates,
  supporterSubscriptionsQueryOptions,
} from "./api/supporters";
export { SupporterSettings } from "./components/SupporterSettings";
// The service closure: announcing it, changing and withdrawing the announcement, checking and confirming the closing,
// and the progress of its deliveries (ADR-0088), for manage_service_closure.
export { adminClosureCheckQueryOptions, adminClosureQueryOptions } from "./api/closure";
export { ServiceClosureSettings } from "./components/ServiceClosureSettings";
// The groups of the errors that the server and the web client report: the list, one group with its latest occurrence,
// and resolving, muting and unmuting it (ADR-0103 §8), for manage_errors.
export { type ErrorFilter, errorGroupQueryOptions, errorGroupsQueryOptions, errorSourceFilters, errorStateFilters } from "./api/errors";
export { ErrorGroupDetail } from "./components/ErrorGroupDetail";
export { ErrorGroups } from "./components/ErrorGroups";
