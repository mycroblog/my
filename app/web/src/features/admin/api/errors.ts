// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the errors screens (ADR-0103 §8, web-client/README.md S1, manage_errors): the error
// groups under a state and a source filter, the most recently seen first (GET /admin/error_groups), one group with its
// latest occurrence, its hourly counts and the changes of its state (GET /admin/error_groups/{id}), and resolving,
// muting and unmuting a group. The list follows the API's keyset pagination as a TanStack Query infinite query, keyed
// like the client's own `[method, path]` with "pages" at the end (web-client/10-api-client-and-auth.md §10).
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { pageQuery } from "../../moderation";

type Schemas = components["schemas"];

export type ErrorGroup = Schemas["ErrorGroup"];
export type ErrorGroupDetail = Schemas["ErrorGroupDetail"];
export type ErrorGroupState = Schemas["ErrorGroupState"];
export type ErrorGroupSource = Schemas["ErrorGroupSource"];
export type ErrorGroupKind = Schemas["ErrorGroupKind"];
export type ErrorGroupSample = Schemas["ErrorGroupSample"];
export type ErrorGroupHourCount = Schemas["ErrorGroupHourCount"];
export type ErrorGroupEvent = Schemas["ErrorGroupEvent"];

/** The state filters of the list, in the order of their menu: the open groups first, then every group. */
export const errorStateFilters = ["open", "resolved", "muted", "all"] as const satisfies readonly (ErrorGroupState | "all")[];
export type ErrorStateFilter = (typeof errorStateFilters)[number];

/** The source filters of the list: both sources, the server's errors, or the web client's. */
export const errorSourceFilters = ["all", "server", "web"] as const satisfies readonly (ErrorGroupSource | "all")[];
export type ErrorSourceFilter = (typeof errorSourceFilters)[number];

export type ErrorFilter = { state: ErrorStateFilter; source: ErrorSourceFilter };

/** What an administrator can do to a group. */
export type ErrorGroupAction = "resolve" | "mute" | "unmute";

/** The error groups under `filter`, the most recently seen first. */
export function errorGroupsQueryOptions(api: Api, filter: ErrorFilter) {
  const state = filter.state === "all" ? undefined : filter.state;
  const source = filter.source === "all" ? undefined : filter.source;
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/error_groups", { state, source }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = { ...pageQuery(pageParam), ...(state === undefined ? {} : { state }), ...(source === undefined ? {} : { source }) };
      const { data, error } = await api.fetch.GET("/admin/error_groups", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** One error group with its latest occurrence, its hourly counts, and the changes of its state. */
export function errorGroupQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/admin/error_groups/{id}", { params: { path: { id } } });
}

/** Resolves, mutes or unmutes a group, as the options of useMutation. Returns the group as it is now. */
export function errorGroupActionMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/error_groups/{id}/action"] as const,
    mutationFn: async ({ id, action }: { id: string; action: ErrorGroupAction }): Promise<ErrorGroup> => {
      const params = { params: { path: { id } } };
      const { data, error } =
        action === "resolve"
          ? await api.fetch.POST("/admin/error_groups/{id}/resolve", params)
          : action === "mute"
            ? await api.fetch.POST("/admin/error_groups/{id}/mute", params)
            : await api.fetch.POST("/admin/error_groups/{id}/unmute", params);
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Puts the group's new state into the cached detail at once, then fetches again what the change changes (ADR-0045):
 * the detail, whose history gains the change, and every list, which the group may enter or leave. The queries shown
 * now are awaited.
 */
export async function cacheErrorGroupChange(api: Api, queryClient: QueryClient, group: ErrorGroup): Promise<void> {
  const detailKey = errorGroupQueryOptions(api, group.id).queryKey;
  queryClient.setQueryData(detailKey, (detail) => detail && { ...detail, ...group });
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: detailKey }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/error_groups"] }),
  ]);
}
