// SPDX-License-Identifier: AGPL-3.0-only
// The query and mutation of the user's automatic deletion of their own old posts (FR-118): the policy, and replacing
// it as a whole.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";

export type AutoDeletePolicy = components["schemas"]["AutoDeletePolicy"];

/** The user's automatic deletion of old posts (GET /preferences/auto_delete); the default, disabled one before any is saved. */
export function autoDeletePolicyQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/preferences/auto_delete");
}

/**
 * What a draft policy, saved or not, would delete now (POST /preferences/auto_delete/preview): the count, capped by
 * the server (`more`), and the oldest posts. The key holds the whole policy, so each combination is counted once.
 */
export function autoDeletePreviewQueryOptions(api: Api, policy: AutoDeletePolicy) {
  return api.query.queryOptions("post", "/preferences/auto_delete/preview", { body: policy });
}

/** Replaces the policy (PUT /preferences/auto_delete), as the options of useMutation. */
export function updateAutoDeletePolicyMutation(api: Api) {
  return {
    mutationKey: ["put", "/preferences/auto_delete"] as const,
    mutationFn: async (policy: AutoDeletePolicy): Promise<AutoDeletePolicy> => {
      const { data, error } = await api.fetch.PUT("/preferences/auto_delete", { body: policy });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Puts the policy as the server saved it into the cache (web-client/README.md S1, ADR-0045). Saving it deletes nothing at
 * once (the server's hourly job does), so no list of posts changes.
 */
export function cacheSavedAutoDeletePolicy(api: Api, queryClient: QueryClient, saved: AutoDeletePolicy): void {
  queryClient.setQueryData(autoDeletePolicyQueryOptions(api).queryKey, saved);
}
