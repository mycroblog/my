// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's relationship with an account (web-client/README.md S1, FR-301 to FR-303): reading it, batched, and the
// changes of it. Keys follow openapi-react-query's `[method, path, init]`.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { isPostListQuery } from "../../../lib/query";
import type { components } from "@mycroblog/mykit";

type Account = components["schemas"]["Account"];

/** The viewer's relationship with an account. */
export type Relationship = components["schemas"]["Relationship"];

/** The most accounts one GET /accounts/relationships takes (its `maxItems`). */
const relationshipBatchLimit = 40;

type RelationshipRequest = {
  id: string;
  resolve: (relationships: Relationship[]) => void;
  reject: (reason: unknown) => void;
};

/** The relationships each client was asked for in this task, sent together after it. */
const pendingRelationships = new WeakMap<Api, RelationshipRequest[]>();

/**
 * Asks for the viewer's relationship with the account `id` together with every other one asked for in the same task:
 * a list of accounts (the follow lists, the notifications) mounts a follow button per row, and their queries become
 * one request per 40 accounts instead of one each.
 */
function loadRelationship(api: Api, id: string): Promise<Relationship[]> {
  return new Promise((resolve, reject) => {
    let queue = pendingRelationships.get(api);
    if (queue === undefined) {
      queue = [];
      pendingRelationships.set(api, queue);
      setTimeout(() => {
        sendRelationships(api);
      }, 0);
    }
    queue.push({ id, resolve, reject });
  });
}

function sendRelationships(api: Api) {
  const queue = pendingRelationships.get(api) ?? [];
  pendingRelationships.delete(api);
  const ids = [...new Set(queue.map((request) => request.id))];
  for (let start = 0; start < ids.length; start += relationshipBatchLimit) {
    const batch = ids.slice(start, start + relationshipBatchLimit);
    const waiting = queue.filter((request) => batch.includes(request.id));
    api.fetch
      .GET("/accounts/relationships", { params: { query: { id: batch } } })
      .then(({ data, error }) => {
        // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
        if (data === undefined) throw new ApiError(error);
        for (const request of waiting)
          request.resolve(data.items.filter((relationship) => relationship.id === request.id));
      })
      .catch((reason: unknown) => {
        for (const request of waiting) request.reject(reason);
      });
  }
}

/**
 * The viewer's relationship with the account `id` (GET /accounts/relationships), or null when the server
 * left it out (the account is gone or blocks the viewer). The relationships asked for at once go in one request.
 */
export function relationshipQueryOptions(api: Api, id: string) {
  return {
    ...api.query.queryOptions("get", "/accounts/relationships", { params: { query: { id: [id] } } }),
    queryFn: async () => ({ items: await loadRelationship(api, id) }),
    select: (relationships: { items: Relationship[] }): Relationship | null =>
      relationships.items.find((relationship) => relationship.id === id) ?? null,
  };
}

/**
 * What the viewer can do to their relationship with an account. `subscribe` and `unsubscribe` turn on and off the
 * notifications of the account's new posts (ADR-0123), which the account is not told of.
 */
export type RelationshipAction =
  "follow" | "unfollow" | "block" | "unblock" | "mute" | "unmute" | "subscribe" | "unsubscribe";

/** Changes the viewer's relationship with the account `id`; returns the relationship after the change. */
export async function changeRelationship(api: Api, id: string, action: RelationshipAction): Promise<Relationship> {
  const init = { params: { path: { id } } };
  const { data, error } = await (() => {
    switch (action) {
      case "follow":
        return api.fetch.POST("/accounts/{id}/follow", init);
      case "unfollow":
        return api.fetch.DELETE("/accounts/{id}/follow", init);
      case "block":
        return api.fetch.POST("/accounts/{id}/block", init);
      case "unblock":
        return api.fetch.DELETE("/accounts/{id}/block", init);
      case "mute":
        return api.fetch.POST("/accounts/{id}/mute", init);
      case "unmute":
        return api.fetch.DELETE("/accounts/{id}/mute", init);
      case "subscribe":
        return api.fetch.POST("/accounts/{id}/subscribe", init);
      case "unsubscribe":
        return api.fetch.DELETE("/accounts/{id}/subscribe", init);
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
 * or a mute changes every list of posts, which leave out the accounts the viewer blocks or mutes. A subscription to
 * the account's new posts changes only the relationship itself, which the caller stores. The lists of
 * the blocked and muted accounts in the settings are left to the caller (storeRelationship).
 */
export function refreshRelationship(
  queryClient: QueryClient,
  change: RelationshipChange,
  accountId: string,
  viewerId: string,
): Promise<void> {
  if (change === "subscribe" || change === "unsubscribe") return Promise.resolve();
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
  queryClient.setQueryData(relationshipQueryOptions(api, relationship.id).queryKey, { items: [relationship] });
  await Promise.all([
    refreshRelationship(queryClient, action, relationship.id, viewerId),
    queryClient.invalidateQueries({ queryKey: ["get", "/blocks"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/mutes"] }),
  ]);
}
