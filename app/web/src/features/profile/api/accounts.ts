// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the profile feature (web-client/README.md S1, FR-204, FR-301 to FR-303): an
// account looked up by its handle, the viewer's relationship with it, the accounts it follows and that
// follow it, and the changes of the relationship. Keys follow openapi-react-query's `[method, path, init]`.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { isPostListQuery } from "../../../lib/query";
import type { components } from "@mycroblog/mykit";

/** An account, as the client API returns it; its own endpoints add the follow counts. */
export type Account = components["schemas"]["Account"];

/** The viewer's relationship with an account. */
export type Relationship = components["schemas"]["Relationship"];

/** A page of accounts. */
export type AccountPage = components["schemas"]["AccountPage"];

/** The account whose handle (`username`, or `username@domain`) is `acct` (GET /accounts/lookup). */
export function accountQueryOptions(api: Api, acct: string) {
  return api.query.queryOptions("get", "/accounts/lookup", { params: { query: { acct } } });
}

/**
 * The viewer's relationship with the account `id` (GET /accounts/relationships), or null when the server
 * left it out (the account is gone or blocks the viewer).
 */
export function relationshipQueryOptions(api: Api, id: string) {
  return {
    ...api.query.queryOptions("get", "/accounts/relationships", { params: { query: { id: [id] } } }),
    select: (relationships: Relationship[]): Relationship | null =>
      relationships.find((relationship) => relationship.id === id) ?? null,
  };
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

/** What the viewer can do to their relationship with an account. */
export type RelationshipAction = "follow" | "unfollow" | "block" | "unblock" | "mute" | "unmute";

/** Changes the viewer's relationship with the account `id`; returns the relationship after the change. */
export async function changeRelationship(api: Api, id: string, action: RelationshipAction): Promise<Relationship> {
  const init = { params: { path: { id } } };
  const { data, error } = await (() => {
    switch (action) {
      case "follow":
        return api.fetch.POST("/accounts/{id}/follow", init);
      case "unfollow":
        return api.fetch.POST("/accounts/{id}/unfollow", init);
      case "block":
        return api.fetch.POST("/accounts/{id}/block", init);
      case "unblock":
        return api.fetch.POST("/accounts/{id}/unblock", init);
      case "mute":
        return api.fetch.POST("/accounts/{id}/mute", init);
      case "unmute":
        return api.fetch.POST("/accounts/{id}/unmute", init);
    }
  })();
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * A change of a relationship between the viewer and another account: one of the viewer's own, or the viewer
 * accepting the other account's follow request.
 */
export type RelationshipChange = RelationshipAction | "accept";

/** Whether `change` starts or ends a follow, either way: a block ends the follows both ways. */
function changesFollows(change: RelationshipChange): boolean {
  return change === "follow" || change === "unfollow" || change === "accept" || change === "block";
}

/**
 * Marks stale every cached query that a change of the viewer's relationship with the account `accountId`
 * changes (web-client/README.md S1), and resolves once those on screen are fetched again; the others are fetched again
 * when their page opens (S7).
 * A change of a follow changes the follow counts on both profiles and the viewer's own account, every follow
 * list (any may list either account), and, for the viewer's own follows, the home timeline; a change of a block
 * or a mute changes every list of posts, which leave out the accounts the viewer blocks or mutes. The lists of
 * the blocked and muted accounts in the settings are left to the caller (storeRelationship).
 */
export function refreshRelationship(
  queryClient: QueryClient,
  change: RelationshipChange,
  accountId: string,
  viewerId: string,
): Promise<void> {
  const follows = changesFollows(change);
  const posts = change !== "accept";
  return queryClient.invalidateQueries({
    predicate: (query) => {
      const path = query.queryKey[1];
      if (path === "/accounts/lookup") {
        const id = (query.state.data as Account | undefined)?.id;
        return follows && (id === accountId || id === viewerId);
      }
      if (
        path === "/accounts/verify_credentials" ||
        path === "/accounts/{id}/followers" ||
        path === "/accounts/{id}/following"
      )
        return follows;
      if (change === "follow" || change === "unfollow") return path === "/timelines/home";
      return posts && isPostListQuery(query.queryKey);
    },
  });
}

/**
 * Puts a changed relationship into the cache and marks stale what it changes (refreshRelationship), and the
 * settings' lists of the accounts that the viewer blocks and mutes; resolves once those on screen are fetched again.
 */
export async function storeRelationship(
  queryClient: QueryClient,
  api: Api,
  relationship: Relationship,
  action: RelationshipAction,
  viewerId: string,
): Promise<void> {
  queryClient.setQueryData(relationshipQueryOptions(api, relationship.id).queryKey, [relationship]);
  await Promise.all([
    refreshRelationship(queryClient, action, relationship.id, viewerId),
    queryClient.invalidateQueries({ queryKey: ["get", "/blocks"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/mutes"] }),
  ]);
}
