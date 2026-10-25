// SPDX-License-Identifier: AGPL-3.0-only
// The user's aliases (FR-009, ADR-0089): the accounts the user moves from. Both changes answer the whole list, which
// replaces the cached one (web-client/README.md S1).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

export type AccountAlias = components["schemas"]["AccountAlias"];
type AccountAliasList = components["schemas"]["AccountAliasList"];

/** The most aliases a user has (api/openapi.yaml, addAccountAlias). */
export const maxAliases = 5;

/** The longest handle or address the server takes (AddAccountAliasRequest.acct). */
export const aliasInputLimit = 2048;

/** The user's aliases, oldest first (GET /accounts/aliases). */
export function aliasesQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/aliases");
}

/** Adds an alias by handle or address (POST /accounts/aliases), as the options of useMutation. Returns the whole list. */
export function addAliasMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/aliases"] as const,
    mutationFn: async (acct: string): Promise<AccountAliasList> => {
      const { data, error } = await api.fetch.POST("/accounts/aliases", { body: { acct } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Removes an alias (POST /accounts/aliases/remove), as the options of useMutation. Returns the whole list. */
export function removeAliasMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/aliases/remove"] as const,
    mutationFn: async (uri: string): Promise<AccountAliasList> => {
      const { data, error } = await api.fetch.POST("/accounts/aliases/remove", { body: { uri } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Replaces the cached list with the one the server answered. */
export function cacheAliases(api: Api, queryClient: QueryClient, list: AccountAliasList): void {
  queryClient.setQueryData(aliasesQueryOptions(api).queryKey, list);
}
