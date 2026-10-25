// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the moderators' account screens (FR-714, FR-703, FR-713): searching the local and
// remote accounts, one account as the moderators see it, showing the email address and the IP addresses of a
// local account (recorded in the audit log, so only on the moderator's explicit action: a mutation, never a
// query that runs by itself or again), removing its two-factor authentication, issuing a password reset link for its
// user (shown once), and lifting a moderation action. The search follows the API's keyset
// pagination as an infinite query whose key ends with "pages" (web-client/10-api-client-and-auth.md §10).
import { type InfiniteData, infiniteQueryOptions, type Query, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "../../../lib/api/schema.gen";
import { isPostListQuery } from "../../../lib/query";
import { pageQuery, refreshAuditLog } from "./staff";
import { type AccountAction, accountActionsQueryOptions, refreshReportLists } from "./reports";

type Schemas = components["schemas"];

export type AdminAccount = Schemas["AdminAccount"];
export type AdminAccountDetail = Schemas["AdminAccountDetail"];
export type AdminAccountState = Schemas["AdminAccountState"];
export type AdminAccountRestriction = Schemas["AdminAccountRestriction"];
export type AdminAccountRegistrationState = Schemas["AdminAccountRegistrationState"];
export type AdminAccountPrivateData = Schemas["AdminAccountPrivateData"];
export type EmailSuppression = Schemas["EmailSuppression"];
export type AdminPasswordResetLink = Schemas["AdminPasswordResetLink"];

/** The states of an account, in the order of the search's select. */
export const adminAccountStates = ["active", "pending", "rejected", "limited", "disabled", "suspended", "deleted"] as const satisfies readonly AdminAccountState[];

/** Where the accounts of a search are: on this server or on others. */
export const adminAccountOrigins = ["local", "remote"] as const;

export type AdminAccountOrigin = (typeof adminAccountOrigins)[number];

/** The search fields that look for a local account by what only the moderators may see; at most one is given. */
export const adminAccountContactFields = ["email", "email_domain", "ip"] as const;

export type AdminAccountContactField = (typeof adminAccountContactFields)[number];

/** The most characters of a search field (GET /admin/accounts). */
export const adminAccountSearchLimit = 320;

/** A search of the accounts, with the API's own names; an omitted field keeps every account. */
export type AdminAccountSearch = {
  username?: string | undefined;
  domain?: string | undefined;
  origin?: AdminAccountOrigin | undefined;
  state?: AdminAccountState | undefined;
  email?: string | undefined;
  email_domain?: string | undefined;
  ip?: string | undefined;
};

/** A search without its omitted fields. */
type SearchQuery = { [K in keyof AdminAccountSearch]?: Exclude<AdminAccountSearch[K], undefined> };

/** The search without its omitted fields, as the key and the query of the request. */
function searchQuery(search: AdminAccountSearch): SearchQuery {
  return Object.fromEntries(Object.entries(search).filter(([, value]) => value !== undefined));
}

/** The accounts of `search`, newest first (GET /admin/accounts). A page may be short, or empty, and still continue. */
export function adminAccountsQueryOptions(api: Api, search: AdminAccountSearch) {
  const query = searchQuery(search);
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/accounts", query, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/accounts", { params: { query: { ...query, ...pageQuery(pageParam) } }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** One account as the moderators see it, without its email and IP addresses (GET /admin/accounts/{id}). */
export function adminAccountQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/admin/accounts/{id}", { params: { path: { id } } });
}

/** Whether a query key is a search of the accounts, of any fields. */
function isAccountSearchQuery(queryKey: readonly unknown[]): boolean {
  return queryKey[1] === "/admin/accounts";
}

/**
 * Marks stale what a change of the state, the restrictions or the roles of account `id` changes: the searches
 * that are cached (each shows those of its accounts) and the account itself, and with `actions` its moderation
 * actions too. Resolves once those on screen are fetched again; the others are fetched when their page opens
 * (web-client/README.md S1, S7).
 */
export async function refreshAdminAccount(queryClient: QueryClient, api: Api, id: string, { actions }: { actions: boolean }): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ predicate: (query) => isAccountSearchQuery(query.queryKey) }),
    queryClient.invalidateQueries({ queryKey: adminAccountQueryOptions(api, id).queryKey }),
    ...(actions ? [queryClient.invalidateQueries({ queryKey: accountActionsQueryOptions(api, id).queryKey })] : []),
  ]);
}

/**
 * Shows the email address and the IP addresses of a local account (POST /admin/accounts/{id}/reveal), as the
 * options of useMutation. The server records every call in the audit log before it answers.
 */
export function revealAccountMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/accounts/{id}/reveal"] as const,
    mutationFn: async (id: string): Promise<AdminAccountPrivateData> => {
      const { data, error } = await api.fetch.POST("/admin/accounts/{id}/reveal", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Removes the two-factor authentication of a local account (POST /admin/accounts/{id}/two_factor/disable), as the
 * options of useMutation. The server records it in the audit log.
 */
export function disableAccountTwoFactorMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/accounts/{id}/two_factor/disable"] as const,
    mutationFn: async (id: string): Promise<void> => {
      const { error } = await api.fetch.POST("/admin/accounts/{id}/two_factor/disable", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this is only for the types.
      if (error !== undefined) throw new ApiError(error);
    },
  };
}

/**
 * Lifts the suppression of the primary email address of a local account (POST /admin/accounts/{id}/email_suppression/lift),
 * as the options of useMutation. The server records it in the audit log; an address that is not suppressed is a 409.
 */
export function liftAccountEmailSuppressionMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/accounts/{id}/email_suppression/lift"] as const,
    mutationFn: async (id: string): Promise<void> => {
      const { error } = await api.fetch.POST("/admin/accounts/{id}/email_suppression/lift", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this is only for the types.
      if (error !== undefined) throw new ApiError(error);
    },
  };
}

/**
 * Issues a password reset link for the user of a local account (POST /admin/accounts/{id}/password_reset_link), as
 * the options of useMutation. The server stores only the link's hash, so the answer is the only place it is shown;
 * the server records the issuance in the audit log.
 */
export function issuePasswordResetLinkMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/accounts/{id}/password_reset_link"] as const,
    mutationFn: async (id: string): Promise<AdminPasswordResetLink> => {
      const { data, error } = await api.fetch.POST("/admin/accounts/{id}/password_reset_link", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * What removing the two-factor authentication of account `id` changes (web-client/README.md S1, ADR-0045): its page, which
 * shows whether it has one, and the audit log, which records the removal. Both are marked stale; resolves once those
 * on screen are fetched again.
 */
export async function refreshAccountTwoFactor(queryClient: QueryClient, api: Api, id: string): Promise<void> {
  await Promise.all([queryClient.invalidateQueries({ queryKey: adminAccountQueryOptions(api, id).queryKey }), refreshAuditLog(queryClient)]);
}

/**
 * What lifting the email suppression of account `id` changes: its page, which shows the suppression, and the audit log,
 * which records the lift. Both are marked stale; resolves once those on screen are fetched again.
 */
export async function refreshAccountEmailSuppression(queryClient: QueryClient, api: Api, id: string): Promise<void> {
  await Promise.all([queryClient.invalidateQueries({ queryKey: adminAccountQueryOptions(api, id).queryKey }), refreshAuditLog(queryClient)]);
}

/** Lifts a moderation action on an account (POST /admin/account_actions/{id}/lift), as the options of useMutation. */
export function liftAccountActionMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/account_actions/{id}/lift"] as const,
    mutationFn: async ({ id, reason }: { id: string; reason: string }): Promise<AccountAction> => {
      const { data, error } = await api.fetch.POST("/admin/account_actions/{id}/lift", { params: { path: { id } }, body: reason === "" ? {} : { reason } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Whether `query` is the public profile of account `id` (GET /accounts/lookup, whose key names the handle only). */
function isProfileOf(query: Query, id: string): boolean {
  return query.queryKey[1] === "/accounts/lookup" && (query.state.data as { id?: unknown } | undefined)?.id === id;
}

/** The kinds of moderation action that change what everyone sees of the account: its profile or its posts. */
const publicKinds: ReadonlySet<AccountAction["kind"]> = new Set(["mark_statuses_sensitive", "delete_statuses", "force_sensitive", "limit", "suspend", "ban"]);

/**
 * Marks stale what everyone sees of account `id` when a moderation action of `kind` starts or ends (publicKinds):
 * its profile and the lists of posts. A warning or a restriction of what the account itself may do changes neither.
 */
export function refreshPublicAccount(queryClient: QueryClient, id: string, kind: AccountAction["kind"]): Promise<void> | undefined {
  if (!publicKinds.has(kind)) return undefined;
  return queryClient.invalidateQueries({ predicate: (query) => isProfileOf(query, id) || isPostListQuery(query.queryKey) });
}

/**
 * What a new moderation action on account `accountId` changes (web-client/README.md S1, ADR-0045): the account's page,
 * its actions and the searches (refreshAdminAccount), the audit log, the lists of reports when it resolves report
 * `reportId`, and what everyone sees of the account. They are marked stale; resolves once those on screen are fetched again.
 */
export async function cacheIssuedAccountAction(queryClient: QueryClient, api: Api, accountId: string, action: AccountAction, reportId: string | undefined): Promise<void> {
  await Promise.all([
    refreshAdminAccount(queryClient, api, accountId, { actions: true }),
    reportId === undefined ? undefined : refreshReportLists(queryClient),
    refreshAuditLog(queryClient),
    refreshPublicAccount(queryClient, accountId, action.kind),
  ]);
}

/**
 * What a new moderation action changes once the screen has shown it: every cached report, which tells of the
 * account's actions (the one it resolved too), and the posts it acted on are marked stale.
 */
export function refreshActedReports(queryClient: QueryClient, action: AccountAction): void {
  void queryClient.invalidateQueries({ predicate: (query) => query.queryKey[1] === "/admin/reports/{id}" });
  if (action.status_ids !== undefined) void queryClient.invalidateQueries({ predicate: (query) => query.queryKey[1] === "/statuses/{id}" });
}

/**
 * Marks stale what lifting `lifted`, an action on account `accountId`, changes beside its row: the account's
 * state and restrictions (its page and the searches), the audit log, and what everyone sees of the account.
 */
export async function refreshLiftedAccountAction(queryClient: QueryClient, api: Api, accountId: string, lifted: AccountAction): Promise<void> {
  await Promise.all([
    refreshAdminAccount(queryClient, api, accountId, { actions: false }),
    refreshAuditLog(queryClient),
    refreshPublicAccount(queryClient, accountId, lifted.kind),
  ]);
}

/** Writes the lifted action from the answer into the cached actions of account `accountId` (ADR-0045). */
export function cacheLiftedAccountAction(api: Api, queryClient: QueryClient, accountId: string, lifted: AccountAction): void {
  queryClient.setQueryData<InfiniteData<{ items: AccountAction[] }>>(
    accountActionsQueryOptions(api, accountId).queryKey,
    (data) => data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === lifted.id ? lifted : item)) })) },
  );
}
