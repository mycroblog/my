// SPDX-License-Identifier: AGPL-3.0-only
// The browsers signed in to the user's account (ADR-0090): the list, signing out one, and signing out the others.
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { authorizedAppsQueryOptions } from "./authorizedApps";

export type BrowserSession = components["schemas"]["BrowserSession"];

/** The signed-in browsers, this one first and then the most recently used first (GET /sessions). */
export function browserSessionsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/sessions");
}

/** Signs out one browser (DELETE /sessions/{id}), as the options of useMutation. */
export function revokeBrowserSessionMutation(api: Api) {
  return {
    mutationKey: ["delete", "/sessions/{id}"] as const,
    mutationFn: async (id: string): Promise<void> => {
      const { error } = await api.fetch.DELETE("/sessions/{id}", { params: { path: { id } } });
      if (error !== undefined) throw new ApiError(error);
    },
  };
}

/** Signs out every browser but this one (POST /sessions/revoke_others), as the options of useMutation. */
export function revokeOtherBrowserSessionsMutation(api: Api) {
  return {
    mutationKey: ["post", "/sessions/revoke_others"] as const,
    mutationFn: async (): Promise<void> => {
      const { error } = await api.fetch.POST("/sessions/revoke_others");
      if (error !== undefined) throw new ApiError(error);
    },
  };
}

/**
 * What signing out a browser changes (web-client/README.md S1, ADR-0028): this list and the last use of the web client
 * among the authorized apps. Resolves once the list is fetched again.
 */
export async function refreshBrowserSessions(api: Api, queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: browserSessionsQueryOptions(api).queryKey }),
    queryClient.invalidateQueries({ queryKey: authorizedAppsQueryOptions(api).queryKey }),
  ]);
}
