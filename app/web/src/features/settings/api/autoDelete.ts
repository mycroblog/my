// SPDX-License-Identifier: AGPL-3.0-only
// The query and mutation of the user's automatic deletion of their own old posts (FR-118): the policy with the server
// maximum when it applies to the user (ADR-0151 §8), and replacing the policy as a whole.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";

export type AutoDeletePolicy = components["schemas"]["AutoDeletePolicy"];
/** The saved policy with the server maximum when it applies to the user (`server_limit`). */
export type AutoDeleteSettings = components["schemas"]["AutoDeleteSettings"];
/** The server maximum as it applies to the user: its days and allowed exceptions, the effective policy, and the grace. */
export type AutoDeleteServerLimit = components["schemas"]["AutoDeleteServerLimit"];

/** The properties of a policy, which a 422 names in its detail. */
export const autoDeletePolicyProperties = [
  "enabled",
  "min_age_days",
  "keep_pinned",
  "keep_bookmarked",
  "keep_favourited",
  "keep_media",
  "keep_polls",
  "keep_visibilities",
  "keep_min_boosts",
  "keep_min_favourites",
] as const satisfies readonly (keyof AutoDeletePolicy)[];

export type AutoDeletePolicyProperty = (typeof autoDeletePolicyProperties)[number];

/** The policy alone of the settings, as PUT and the preview take it: without `server_limit`. */
export function policyOf(settings: AutoDeletePolicy): AutoDeletePolicy {
  return {
    enabled: settings.enabled,
    min_age_days: settings.min_age_days,
    keep_pinned: settings.keep_pinned,
    keep_bookmarked: settings.keep_bookmarked,
    keep_favourited: settings.keep_favourited,
    keep_media: settings.keep_media,
    keep_polls: settings.keep_polls,
    keep_visibilities: settings.keep_visibilities,
    keep_min_boosts: settings.keep_min_boosts,
    keep_min_favourites: settings.keep_min_favourites,
  };
}

/**
 * The user's automatic deletion of old posts (GET /preferences/auto_delete), with the server maximum when it applies
 * to them; the default, disabled one before any is saved.
 */
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
    mutationFn: async (policy: AutoDeletePolicy): Promise<AutoDeleteSettings> => {
      const { data, error } = await api.fetch.PUT("/preferences/auto_delete", { body: policy });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Puts the policy as the server saved it, with what the maximum makes of it, into the cache (web-client/README.md S1,
 * ADR-0028). Saving it deletes nothing at once (the server's hourly job does), so no list of posts changes.
 */
export function cacheSavedAutoDeletePolicy(api: Api, queryClient: QueryClient, saved: AutoDeleteSettings): void {
  queryClient.setQueryData(autoDeletePolicyQueryOptions(api).queryKey, saved);
}
