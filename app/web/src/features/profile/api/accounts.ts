// SPDX-License-Identifier: AGPL-3.0-only
// The queries of the profile feature (web-client/README.md S1, FR-204): an account looked up by its handle, and the
// accounts it follows and that follow it. Keys follow openapi-react-query's `[method, path, init]`.
import { infiniteQueryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";

/** An account, as the client API returns it; its own endpoints add the follow counts. */
export type Account = components["schemas"]["Account"];

/** A page of accounts. */
export type AccountPage = components["schemas"]["AccountPage"];

/** The account whose handle (`username`, or `username@domain`) is `acct` (GET /accounts/lookup). */
export function accountQueryOptions(api: Api, acct: string) {
  return api.query.queryOptions("get", "/accounts/lookup", { params: { query: { acct } } });
}

/** The two lists of a profile's follows. */
export type FollowList = "followers" | "following";

/** The query key of a follow list; `["get", "/accounts/{id}/followers"]` is the prefix of every account's list. */
function followListKey(id: string, list: FollowList) {
  return list === "followers"
    ? (["get", "/accounts/{id}/followers", { params: { path: { id } } }, "pages"] as const)
    : (["get", "/accounts/{id}/following", { params: { path: { id } } }, "pages"] as const);
}

/** The accounts that follow the account `id`, or that it follows, newest follow first, a page at a time. */
export function followListQueryOptions(api: Api, id: string, list: FollowList) {
  return infiniteQueryOptions({
    queryKey: followListKey(id, list),
    queryFn: async ({ pageParam, signal }): Promise<AccountPage> => {
      const params = { path: { id }, query: pageParam === undefined ? {} : { cursor: pageParam } };
      const { data, error } =
        list === "followers"
          ? await api.fetch.GET("/accounts/{id}/followers", { params, signal })
          : await api.fetch.GET("/accounts/{id}/following", { params, signal });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
    // A failed page is shown at once with a button to try again, as on the timelines.
    retry: false,
  });
}
