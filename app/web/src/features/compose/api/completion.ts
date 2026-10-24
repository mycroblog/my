// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the completion of mentions and hashtags (ADR-0092 §1, §3): the prefix searches of accounts
// (GET /accounts/search) and hashtags (GET /tags/search), and the lookup of a full `user@domain` that the search does
// not know. Each search is kept for 60 seconds, so deleting letters and typing them again asks nothing; a request that a
// newer word replaces is cancelled.
import type { QueryClient, QueryKey } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "../../../lib/api/schema.gen";
import { lookupAccountQueryOptions } from "./compose";

export type Account = components["schemas"]["Account"];
export type TagSuggestion = components["schemas"]["TagSuggestion"];

/** How long the answer for one query stays fresh (ADR-0092 §1). */
const completionStaleTime = 60_000;

/** The accounts whose username, `username@domain` or display name word starts with `q`. */
function accountSearchQueryOptions(api: Api, q: string) {
  return api.query.queryOptions("get", "/accounts/search", { params: { query: { q } } }, { staleTime: completionStaleTime, retry: false });
}

/** The hashtags that start with `q`, the most used first. */
function tagSearchQueryOptions(api: Api, q: string) {
  return api.query.queryOptions("get", "/tags/search", { params: { query: { q } } }, { staleTime: completionStaleTime, retry: false });
}

/** Runs `read`; when `signal` aborts first, cancels the query of `queryKey`, which goes back to what it held. */
async function cancellable<T>(queryClient: QueryClient, queryKey: QueryKey, signal: AbortSignal, read: () => Promise<T>): Promise<T> {
  const cancel = () => {
    void queryClient.cancelQueries({ queryKey, exact: true });
  };
  signal.addEventListener("abort", cancel, { once: true });
  try {
    return await read();
  } finally {
    signal.removeEventListener("abort", cancel);
  }
}

/** The accounts that GET /accounts/search finds for `q`, from the cache while it is fresh. */
export function searchAccounts(api: Api, queryClient: QueryClient, q: string, signal: AbortSignal): Promise<Account[]> {
  const options = accountSearchQueryOptions(api, q);
  return cancellable(queryClient, options.queryKey, signal, () => queryClient.query(options));
}

/** The hashtags that GET /tags/search finds for `q`, from the cache while it is fresh. */
export function searchTags(api: Api, queryClient: QueryClient, q: string, signal: AbortSignal): Promise<TagSuggestion[]> {
  const options = tagSearchQueryOptions(api, q);
  return cancellable(queryClient, options.queryKey, signal, () => queryClient.query(options));
}

/**
 * The account of the handle `acct` (`user@domain`) from GET /accounts/lookup, resolved from its server with `resolve`
 * (not while the instance takes nothing in from other servers, ADR-0076); undefined when it is not found (404), the
 * fetch budget is spent (429) or the request fails.
 */
export function lookUpAccount(api: Api, queryClient: QueryClient, acct: string, resolve: boolean, signal: AbortSignal): Promise<Account | undefined> {
  const options = lookupAccountQueryOptions(api, acct, resolve);
  return cancellable(queryClient, options.queryKey, signal, () => queryClient.query(options)).catch(() => undefined);
}
