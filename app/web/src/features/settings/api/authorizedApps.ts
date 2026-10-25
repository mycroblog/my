// SPDX-License-Identifier: AGPL-3.0-only
// The apps the user authorized (ADR-0090 §5): the list and revoking one. Revoking the server's own web client signs the
// user out of every browser, so the caller signs this one out instead of fetching the list again.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

export type AuthorizedApp = components["schemas"]["AuthorizedApp"];

/** The apps the user authorized, the most recently used first (GET /authorized_apps). */
export function authorizedAppsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/authorized_apps");
}

/** Revokes every grant of an app (DELETE /authorized_apps/{id}), as the options of useMutation. */
export function revokeAuthorizedAppMutation(api: Api) {
  return {
    mutationKey: ["delete", "/authorized_apps/{id}"] as const,
    mutationFn: async (id: string): Promise<string> => {
      const { error } = await api.fetch.DELETE("/authorized_apps/{id}", { params: { path: { id } } });
      if (error !== undefined) throw new ApiError(error);
      return id;
    },
  };
}

/** What revoking an app changes (web-client/README.md S1, ADR-0045): the list. Resolves once it is fetched again. */
export async function refreshAuthorizedApps(api: Api, queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: authorizedAppsQueryOptions(api).queryKey });
}
