// SPDX-License-Identifier: AGPL-3.0-only
// The queries and the mutation of the user's own account status (FR-715, FR-707): the moderation actions on
// the user's account, newest first, and appealing one. The lists follow the API's keyset pagination as
// infinite queries whose keys end with "pages" (web-client/10-api-client-and-auth.md §10).
import { type InfiniteData, infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

type Schemas = components["schemas"];

export type ModerationAction = Schemas["ModerationAction"];
export type ModerationActionKind = ModerationAction["kind"];
export type ModerationActionState = ModerationAction["state"];
export type ModerationAppeal = Schemas["ModerationAppeal"];
export type ModerationAppealState = ModerationAppeal["state"];

/** Which actions a list holds: those in effect now (`active=true`), or all of them. */
export type ActionScope = "active" | "all";

/** The most characters of an appeal's text (AppealModerationActionRequest). */
export const appealTextLimit = 2000;

/** The page size of the lists. */
const pageSize = 40;

/** The actions on the user's own account, newest first (GET /moderation/actions). */
export function ownActionsQueryOptions(api: Api, scope: ActionScope) {
  return infiniteQueryOptions({
    queryKey: ["get", "/moderation/actions", "pages", scope] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = { limit: pageSize, ...(pageParam === undefined ? {} : { cursor: pageParam }), ...(scope === "active" ? { active: true } : {}) };
      const { data, error } = await api.fetch.GET("/moderation/actions", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Whether a query is one of the lists of the user's own actions, which an appeal changes. */
function isOwnActionsQuery(queryKey: readonly unknown[]): boolean {
  return queryKey[0] === "get" && queryKey[1] === "/moderation/actions";
}

/** One appeal: of which action, and why the moderators should reconsider it. */
export type AppealAction = { actionId: string; text: string };

/**
 * Appeals an action on the user's own account (POST /moderation/actions/{id}/appeal), as the options of
 * TanStack Query's useMutation (web-client/10-api-client-and-auth.md §10). Returns the action with the appeal.
 */
export function appealActionMutation(api: Api) {
  return {
    mutationKey: ["post", "/moderation/actions/{id}/appeal"] as const,
    mutationFn: async ({ actionId, text }: AppealAction): Promise<ModerationAction> => {
      const { data, error } = await api.fetch.POST("/moderation/actions/{id}/appeal", { params: { path: { id: actionId } }, body: { text } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Replaces the action of `action.id` in the pages of a list of actions. */
function replaceAction(data: InfiniteData<{ items: ModerationAction[] }> | undefined, action: ModerationAction) {
  if (data === undefined) return data;
  return { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === action.id ? action : item)) })) };
}

/**
 * Puts the action as the server answered its appeal, with the appeal, into both cached lists of the user's own
 * actions (ADR-0045): the form gives way to the appeal's state on the page at once.
 */
export function cacheAppealedAction(api: Api, queryClient: QueryClient, action: ModerationAction): void {
  for (const scope of ["active", "all"] as const) {
    queryClient.setQueryData(ownActionsQueryOptions(api, scope).queryKey, (data) => replaceAction(data, action));
  }
}

/**
 * Marks stale both lists of the user's own actions (active and all) after an appeal that the action's state
 * refused (isStaleAppealError): the one on screen is fetched again at once, the other when the page shows it
 * (web-client/README.md S1, S7).
 */
export function refreshOwnActions(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({ predicate: (query) => isOwnActionsQuery(query.queryKey) });
}

/**
 * The problem types of an appeal that the action's own state explains (it has an appeal already, its window
 * closed, or its account is gone): after one, the list is stale, and reading it again takes the form away.
 */
const staleAppealProblems = new Set(["/api/v1/problems/already-appealed", "/api/v1/problems/appeal-window-closed", "/api/v1/problems/not-appealable"]);

/** Whether an appeal failed because the action can no longer be appealed. */
export function isStaleAppealError(error: unknown): boolean {
  return error instanceof ApiError && staleAppealProblems.has(error.type);
}
