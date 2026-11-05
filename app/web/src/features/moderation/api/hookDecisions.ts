// SPDX-License-Identifier: AGPL-3.0-only
// The queries and the mutation of the decisions of the external reviews (ADR-0147 §9): what each hook said, the newest
// first, their counts and latency, and the cancellation of a decision by a viewer with manage_reports. The list follows
// the API's keyset pagination as a TanStack Query infinite query whose key ends with "pages"
// (web-client/10-api-client-and-auth.md §10).
import type { components } from "@mycroblog/mykit";
import { infiniteQueryOptions, type QueryClient, queryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { pageQuery } from "./staff";

type Schemas = components["schemas"];

export type HookDecision = Schemas["AdminHookDecision"];
export type HookName = Schemas["HookName"];
export type HookSummary = Schemas["AdminHookSummary"];
export type HookOutcome = Schemas["HookOutcome"];
export type HookMode = Schemas["HookMode"];
export type HookDecisionMode = Schemas["HookDecisionMode"];

/** The hooks, in the order of the screen and the API. */
const hookNames = [
  "inbound.status",
  "inbound.follow",
  "signup.check",
  "media.check",
  "local.status",
] as const satisfies readonly HookName[];

/** The filters of the list: a hook, or every one. */
export const hookDecisionFilters = ["all", ...hookNames] as const;
export type HookDecisionFilter = (typeof hookDecisionFilters)[number];

export function hookDecisionsQueryOptions(api: Api, filter: HookDecisionFilter) {
  const hook = filter === "all" ? undefined : filter;
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/hook_decisions", { hook }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = { ...pageQuery(pageParam), ...(hook === undefined ? {} : { hook }) };
      const { data, error } = await api.fetch.GET("/admin/hook_decisions", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

export function hookSummaryQueryOptions(api: Api) {
  return queryOptions({
    queryKey: ["get", "/admin/hook_decisions/summary"] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/admin/hook_decisions/summary", { signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/** Reads the lists and the summary again after a cancellation, which changes a row and nothing else. */
export function refreshHookDecisions(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[0] === "get" && String(query.queryKey[1]).startsWith("/admin/hook_decisions"),
  });
}

export function overrideHookDecisionMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/hook_decisions/{id}/override"] as const,
    mutationFn: async (id: string): Promise<HookDecision> => {
      const { data, error } = await api.fetch.POST("/admin/hook_decisions/{id}/override", {
        params: { path: { id } },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}
