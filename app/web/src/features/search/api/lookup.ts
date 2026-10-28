// SPDX-License-Identifier: AGPL-3.0-only
// The account search (web-client/README.md S1, FR-305): a handle (`@username@domain`) or the URL of an actor or
// a profile, looked up among the accounts the instance knows and, for a signed-in viewer, fetched from its
// server when the instance does not know it yet (GET /accounts/lookup?resolve=true). Resolving reaches
// another server and is rate limited, so it is a mutation the viewer sends, not a query a page load runs.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { type Account, accountQueryOptions } from "../../profile";

/** The key of the search's mutation. */
export const accountSearchMutationKey = ["get", "/accounts/lookup", "resolve"] as const;

/** The longest search the server accepts (api/openapi.yaml, lookupAccount `acct`). */
export const maxSearchLength = 2048;

/**
 * Finds the account that `query` names. With `resolve`, an account the instance does not know is fetched
 * from its server; that needs the viewer's token.
 */
export async function lookupAccount(api: Api, query: string, resolve: boolean): Promise<Account> {
  const { data, error } = await api.fetch.GET("/accounts/lookup", {
    params: { query: resolve ? { acct: query, resolve: true } : { acct: query } },
  });
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Puts a found account into the cache of the profile that shows it, so that the profile opens without fetching it again. */
export function storeFoundAccount(queryClient: QueryClient, api: Api, account: Account): void {
  queryClient.setQueryData(accountQueryOptions(api, account.acct).queryKey, account);
}
