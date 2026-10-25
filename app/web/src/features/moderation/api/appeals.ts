// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the review of appeals (FR-707): the appeals that local users made of the
// moderation actions on their accounts, awaiting a decision or decided, newest first; one appeal with its action;
// and upholding or overturning it with an explanation shown to the user. The lists follow the API's keyset
// pagination as infinite queries whose keys end with "pages" (web-client/10-api-client-and-auth.md §10). A decision answers the appeal
// after it, with its action (lifted by an overturn), which replaces the cached appeal once the lists of appeals
// it changes are fetched again.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";
import { refreshAdminAccount, refreshPublicAccount } from "./accounts";
import { pageQuery, refreshAuditLog } from "./staff";

type Schemas = components["schemas"];

export type Appeal = Schemas["AdminAppeal"];
export type AppealState = Schemas["ModerationAppealState"];
export type AppealPage = Schemas["AdminAppealPage"];

/** The tabs of the appeals screen, in their order: the appeals that await a decision, and those decided. */
export const appealFilters = ["pending", "decided"] as const;

export type AppealFilter = (typeof appealFilters)[number];

/** What a moderator decides on an appeal; each is the last segment of its operation's path. */
export type AppealDecision = "uphold" | "overturn";

/** The most characters of the explanation of a decision (DecideAppealRequest). */
export const appealReasonLimit = 2000;

/** The problem type of deciding an appeal that another moderator decided the other way first. */
const appealDecidedType = "/api/v1/problems/appeal-decided";

/** Whether `error` is the server's refusal of a decision because the appeal was decided the other way first. */
export function isAppealDecided(error: unknown): boolean {
  return error instanceof ApiError && error.type === appealDecidedType;
}

/**
 * The appeals of a tab, newest first (GET /admin/appeals). The API filters by one state, so the decided tab
 * reads every state and keeps the upheld and overturned ones: a page of pending appeals only is followed by the
 * next until one has a decided appeal or the list ends, so that a page is never empty while more follow.
 */
export function appealsQueryOptions(api: Api, filter: AppealFilter) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/appeals", { filter }, "pages"] as const,
    queryFn: async ({ pageParam, signal }): Promise<AppealPage> => {
      let cursor = pageParam;
      for (;;) {
        const query = filter === "pending" ? { ...pageQuery(cursor), state: "pending" as const } : pageQuery(cursor);
        const { data, error } = await api.fetch.GET("/admin/appeals", { params: { query }, signal });
        // The middleware throws an ApiError for every error status; this narrows the type.
        if (data === undefined) throw new ApiError(error);
        if (filter === "pending") return data;
        const items = data.items.filter((appeal) => appeal.state !== "pending");
        if (items.length > 0 || data.next_cursor === undefined) return { ...data, items };
        cursor = data.next_cursor;
      }
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** One appeal with its action (GET /admin/appeals/{id}). */
export function appealQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/admin/appeals/{id}", { params: { path: { id } } });
}

/** One decision: on which appeal, which, and the explanation to the user ("" for none). */
export type DecideAppeal = { id: string; decision: AppealDecision; reason: string };

/**
 * Upholds or overturns an appeal (POST /admin/appeals/{id}/uphold | overturn), as the options of TanStack
 * Query's useMutation (web-client/10-api-client-and-auth.md §10). Returns the appeal after it, with its action.
 */
export function decideAppealMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/appeals/{id}"] as const,
    mutationFn: async ({ id, decision, reason }: DecideAppeal): Promise<Appeal> => {
      const init = { params: { path: { id } }, body: reason === "" ? {} : { public_reason: reason } };
      const { data, error } =
        decision === "uphold" ? await api.fetch.POST("/admin/appeals/{id}/uphold", init) : await api.fetch.POST("/admin/appeals/{id}/overturn", init);
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Marks stale what a decision on an appeal of `accountId` changes (web-client/README.md S1): the lists of appeals (it
 * moves between the tabs), the moderators' counts, that account with its actions and in the searches (an
 * overturn lifts an action), and the audit log, which records the decision. Those on screen are fetched again at
 * once; a list opened later is fetched again before it shows (S7), so it never shows a decided appeal among those
 * awaiting review.
 */
async function refreshAppealLists(queryClient: QueryClient, api: Api, accountId: string): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ predicate: (query) => query.queryKey[1] === "/admin/appeals" || query.queryKey[1] === "/admin/counts" }),
    refreshAdminAccount(queryClient, api, accountId, { actions: true }),
    refreshAuditLog(queryClient),
  ]);
}

/**
 * Marks stale what a decision changes (refreshAppealLists, and what everyone sees of the account when an
 * overturn lifts its action, refreshPublicAccount), then puts `appeal`, as the decision answered it, into the
 * cache: the appeal's page shows the decision once the list has it too.
 */
export async function storeAppeal(queryClient: QueryClient, api: Api, appeal: Appeal): Promise<void> {
  await Promise.all([
    refreshAppealLists(queryClient, api, appeal.account_id),
    appeal.state === "overturned" ? refreshPublicAccount(queryClient, appeal.account_id, appeal.action.kind) : undefined,
  ]);
  queryClient.setQueryData(appealQueryOptions(api, appeal.id).queryKey, appeal);
}

/**
 * After a decision refused because another moderator decided appeal `id` first: that decision changed the appeal
 * and the lists as well, so they are marked stale; resolves once those on screen are fetched again.
 */
export async function refreshDecidedAppeal(queryClient: QueryClient, api: Api, id: string): Promise<void> {
  const accountId = queryClient.getQueryData<Appeal>(appealQueryOptions(api, id).queryKey)?.account_id;
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: appealQueryOptions(api, id).queryKey }),
    accountId === undefined ? undefined : refreshAppealLists(queryClient, api, accountId),
  ]);
}
