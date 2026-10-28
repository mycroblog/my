// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the badges on the user's profile (web-client/README.md S1, ADR-0071): the roles with a
// badge that the user holds and whether each badge is shown (GET /preferences/badges), and showing or hiding one
// (PUT /preferences/badges/{role_id}). A badge is hidden until the user shows it, and shows on the profile only.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { credentialsQueryOptions } from "./me";

export type BadgeChoice = components["schemas"]["BadgeChoice"];
type BadgeChoiceList = components["schemas"]["BadgeChoiceList"];

/** The roles with a badge that the user holds, highest first, with whether each badge is shown (GET /preferences/badges). */
export function badgeChoicesQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/preferences/badges");
}

/** Shows or hides the badge of a role on the user's profile (PUT /preferences/badges/{role_id}), as the options of useMutation. */
export function updateBadgeChoiceMutation(api: Api) {
  return {
    mutationKey: ["put", "/preferences/badges/{role_id}"] as const,
    mutationFn: async ({ roleId, displayed }: { roleId: string; displayed: boolean }): Promise<BadgeChoice> => {
      const { data, error } = await api.fetch.PUT("/preferences/badges/{role_id}", {
        params: { path: { role_id: roleId } },
        body: { displayed },
      });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Puts the choice as the server answered it into the list (web-client/README.md S1, ADR-0045), and marks the viewer's account
 * and the profiles, whose badges it changes; those shown now are fetched again and awaited.
 */
export async function cacheBadgeChoice(api: Api, queryClient: QueryClient, saved: BadgeChoice): Promise<void> {
  queryClient.setQueryData(badgeChoicesQueryOptions(api).queryKey, (list: BadgeChoiceList | undefined) =>
    list === undefined ? list : { ...list, items: list.items.map((c) => (c.role_id === saved.role_id ? saved : c)) },
  );
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: credentialsQueryOptions(api).queryKey }),
    queryClient.invalidateQueries({ queryKey: ["get", "/accounts/lookup"] }),
  ]);
}
