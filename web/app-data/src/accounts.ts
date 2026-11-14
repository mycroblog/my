// SPDX-License-Identifier: AGPL-3.0-only
// Accounts and the viewer's relationships with them (ADR-0028 §7): the account a profile shows, found by its handle,
// its posts, its featured hashtags, the viewer's own account, and the relationship that the viewer's writes change. A
// write puts the relationship the server answered into the cache, and marks what the change may reorder or recount:
// the account's counts, the home timeline, and the account's posts.
import type { components } from "@mycroblog/mykit";
import { useQuery, type QueryClient } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";
import type { Status } from "./statuses";

/** An account of the client API. */
export type Account = components["schemas"]["Account"];

/** The viewer's relationship with an account. */
export type Relationship = components["schemas"]["Relationship"];

/** A hashtag an account features on its profile. */
export type FeaturedHashtag = components["schemas"]["FeaturedHashtag"];

/** The account of a handle (`user` or `user@domain`), fetched from its server when this one does not know it yet. */
export function accountQuery({ queries }: AppData, acct: string) {
  return queries.queryOptions("get", "/accounts/lookup", { params: { query: { acct, resolve: true } } });
}

/** The viewer's own account. */
export function viewerQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/accounts/verify_credentials");
}

/** The viewer's relationship with the account of `id`. */
export function relationshipQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/accounts/relationships", { params: { query: { id: [id] } } });
}

/** The hashtags the account of `id` features. */
export function featuredHashtagsQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/accounts/{id}/featured_tags", { params: { path: { id } } });
}

/**
 * Which of an account's posts a list holds: its posts without its replies, all of them, or those with media (the API
 * has no filter of media, so its pages are read without replies and the posts without media left out), and its pinned
 * posts.
 */
export type AccountPosts = "posts" | "replies" | "media" | "pinned";

const postsQuery = (kind: AccountPosts) =>
  kind === "pinned" ? { pinned: true } : kind === "replies" ? {} : { exclude_replies: true };

/** The posts of the account of `id`, newest first, of the kind given. */
export function accountStatusesQuery({ api, queries }: AppData, id: string, kind: AccountPosts) {
  const init = { params: { path: { id }, query: postsQuery(kind) } };
  return pagedQuery<Status>(
    [...queries.queryOptions("get", "/accounts/{id}/statuses", init).queryKey, kind],
    async (cursor, signal) => {
      const query = cursor === undefined ? init.params.query : { ...init.params.query, cursor };
      const { data } = await api.GET("/accounts/{id}/statuses", { params: { path: { id }, query }, signal });
      return data;
    },
  );
}

/**
 * The account of a handle; undefined until the server answered, or without a handle, and its failure (a ProblemError
 * of mykit).
 */
export function useAccount(acct: string | undefined): { account: Account | undefined; error: Error | null } {
  const query = useQuery({ ...accountQuery(useAppData(), acct ?? ""), enabled: acct !== undefined });
  // The client rejects with a ProblemError or a NetworkError, which openapi-react-query types as the problem's body.
  return { account: query.data, error: query.error as Error | null };
}

/** The viewer's own account, undefined until the server answered. */
export function useViewer(): Account | undefined {
  return useQuery(viewerQuery(useAppData())).data;
}

/** The viewer's relationship with the account of `id`; undefined until the server answered, or without an id. */
export function useRelationship(id: string | undefined): Relationship | undefined {
  const data = useAppData();
  return useQuery({ ...relationshipQuery(data, id ?? ""), enabled: id !== undefined }).data?.items[0];
}

/** The hashtags the account of `id` features; empty until the server answered. */
export function useFeaturedHashtags(id: string): readonly FeaturedHashtag[] {
  return useQuery(featuredHashtagsQuery(useAppData(), id)).data?.items ?? [];
}

/** The posts of the account of `id` of the kind given; nothing is fetched while `enabled` is false. */
export function useAccountStatuses(id: string, kind: AccountPosts, enabled = true): PagedList<Status> {
  const list = usePagedList(accountStatusesQuery(useAppData(), id, kind), enabled);
  if (kind !== "media") return list;
  return { ...list, items: list.items.filter((s) => (s.boost ?? s).media_attachments.length > 0) };
}

/** What the viewer may do to their relationship with an account. */
export type RelationshipChange = "follow" | "unfollow" | "block" | "unblock" | "mute" | "unmute";

async function send({ api }: AppData, id: string, change: RelationshipChange): Promise<Relationship | undefined> {
  const path = { params: { path: { id } } };
  switch (change) {
    case "follow":
      return (await api.POST("/accounts/{id}/follow", path)).data;
    case "unfollow":
      return (await api.DELETE("/accounts/{id}/follow", path)).data;
    case "block":
      return (await api.POST("/accounts/{id}/block", path)).data;
    case "unblock":
      return (await api.DELETE("/accounts/{id}/block", path)).data;
    case "mute":
      return (await api.POST("/accounts/{id}/mute", path)).data;
    case "unmute":
      return (await api.DELETE("/accounts/{id}/mute", path)).data;
  }
}

/** Puts a relationship the server answered into the cache, and marks what it may change. */
export async function cacheRelationship(data: AppData, relationship: Relationship): Promise<void> {
  const { queryClient } = data;
  queryClient.setQueryData(relationshipQuery(data, relationship.id).queryKey, { items: [relationship] });
  await markAccountLists(queryClient, relationship.id);
}

/**
 * Marks what a change of a relationship may change: the accounts with their counts, the home timeline, and the posts
 * of the account. The marks of screens not shown are read when one opens; the shown ones are read now.
 */
async function markAccountLists(queryClient: QueryClient, id: string): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/accounts/lookup"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/timelines/home"] }),
    queryClient.invalidateQueries({
      predicate: ({ queryKey }) =>
        queryKey[1] === "/accounts/{id}/statuses" && JSON.stringify(queryKey[2]).includes(`"${id}"`),
    }),
  ]);
}

/**
 * Changes the viewer's relationship with the account of `id`, and resolves once the cache holds what the server
 * answered and the shown screens read what it marked. It rejects with the server's problem.
 */
export async function changeRelationship(data: AppData, id: string, change: RelationshipChange): Promise<void> {
  const relationship = await send(data, id, change);
  if (relationship !== undefined) await cacheRelationship(data, relationship);
}

/** Changes the viewer's relationship with an account, from a screen (`changeRelationship`). */
export function useChangeRelationship(): (id: string, change: RelationshipChange) => Promise<void> {
  const data = useAppData();
  return (id, change) => changeRelationship(data, id, change);
}
