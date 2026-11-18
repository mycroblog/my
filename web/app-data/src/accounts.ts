// SPDX-License-Identifier: AGPL-3.0-only
// Accounts and the viewer's relationships with them (ADR-0028 §7): the account a profile shows, found by its handle,
// at once from the copy its posts carry and fetched again by its ID in the background, its posts, its featured hashtags, the viewer's own account, and the relationship that the viewer's writes change. A
// write puts the relationship the server answered into the cache, and marks what the change may reorder or recount:
// the account's counts, the home timeline, and the account's posts.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery, type Query, type QueryClient } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";
import { showList, useShadow } from "./shadow";
import { copiesIn, type Status } from "./statuses";

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

/**
 * The account of `id`. Without data, its first data is the newest copy of the account the cache holds (cachedAccount),
 * as fetched when that copy was: it shows at once, and is fetched again in the background once it is stale (ADR-0028
 * §7). A copy never replaces data the query holds.
 */
export function accountByIdQuery(data: AppData, id: string) {
  let copy: AccountCopy | undefined | null = null;
  const seed = () => (copy ??= newestCopy(data.queryClient, (a) => a.id === id));
  return {
    ...data.queries.queryOptions("get", "/accounts/{id}", { params: { path: { id } } }),
    initialData: () => seed()?.account,
    initialDataUpdatedAt: () => seed()?.at,
  };
}

/** A copy of an account that a cached query holds, and when that query's data was fetched. */
type AccountCopy = { readonly account: Account; readonly at: number };

/** The accounts a query holds: an account's own data, and the author of each status it holds, boosted or quoted too. */
function* accountsIn(query: Query): Generator<Account> {
  const [method, path] = query.queryKey;
  if (method === "get" && (path === "/accounts/{id}" || path === "/accounts/lookup")) {
    const account = query.state.data as Account | undefined;
    if (account !== undefined) yield account;
    return;
  }
  for (const status of copiesIn(query)) yield status.account;
}

/** The copy of an account that `match` picks, from the query fetched last among those that hold one. */
function newestCopy(queryClient: QueryClient, match: (account: Account) => boolean): AccountCopy | undefined {
  let newest: AccountCopy | undefined;
  for (const query of queryClient.getQueryCache().getAll()) {
    const at = query.state.dataUpdatedAt;
    if (newest !== undefined && at <= newest.at) continue;
    for (const account of accountsIn(query))
      if (match(account)) {
        newest = { account, at };
        break;
      }
  }
  return newest;
}

/** Whether `account` is the account of a handle: `user` for a local account, `user@domain` for any. */
const hasHandle = (account: Account, acct: string) => {
  const handle = acct.toLowerCase();
  return account.acct.toLowerCase() === handle || `${account.username}@${account.domain}`.toLowerCase() === handle;
};

/** The newest copy of the account of a handle that the cache holds, from a post, its lookup, or itself; else undefined. */
export function cachedAccount(data: AppData, acct: string): Account | undefined {
  return newestCopy(data.queryClient, (a) => hasHandle(a, acct))?.account;
}

/**
 * What a route's loader waits for of the account of a handle: nothing when the cache holds a copy of it, which the
 * account's query (accountByIdQuery) starts from, else its lookup; either again when a write marked it stale. It
 * rejects with the server's problem.
 */
export async function loadAccount(data: AppData, acct: string): Promise<Account> {
  const id = cachedAccount(data, acct)?.id;
  return id === undefined
    ? data.queryClient.query({ ...accountQuery(data, acct), staleTime: Infinity })
    : data.queryClient.query({ ...accountByIdQuery(data, id), staleTime: Infinity });
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
 * has no filter of media, so its pages are read without replies and the posts without media left out), its articles
 * (ADR-0160 §11), and its pinned posts.
 */
export type AccountPosts = "posts" | "replies" | "media" | "articles" | "pinned";

const postsQuery = (kind: AccountPosts) =>
  kind === "pinned"
    ? { pinned: true }
    : kind === "articles"
      ? { articles: true }
      : kind === "replies"
        ? {}
        : { exclude_replies: true };

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
 * The account of a handle, and its failure (a ProblemError of mykit): at once from a copy the cache holds, fetched
 * again by its ID in the background once stale (accountByIdQuery), else from its lookup; undefined until the server
 * answered, or without a handle. `missing` once the server says the account is not there (404 or 410: never was,
 * deleted, or deactivated, which it does not tell apart), even over a copy the cache held.
 */
export function useAccount(acct: string | undefined): {
  account: Account | undefined;
  error: Error | null;
  missing: boolean;
} {
  const data = useAppData();
  const id = acct === undefined ? undefined : cachedAccount(data, acct)?.id;
  const byId = useQuery({ ...accountByIdQuery(data, id ?? ""), enabled: id !== undefined });
  const lookup = useQuery({ ...accountQuery(data, acct ?? ""), enabled: acct !== undefined && id === undefined });
  const query = id === undefined ? lookup : byId;
  // The client rejects with a ProblemError or a NetworkError, which openapi-react-query types as the problem's body.
  const error = query.error as Error | null;
  const missing = error instanceof ProblemError && (error.status === 404 || error.status === 410);
  return { account: query.data, error, missing };
}

/** The viewer's own account, undefined until the server answered; not read while `enabled` is false (no session). */
export function useViewer(enabled = true): Account | undefined {
  return useQuery({ ...viewerQuery(useAppData()), enabled }).data;
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

/** The posts of the account of `id` of the kind given, as they show; nothing is fetched while `enabled` is false. */
export function useAccountStatuses(id: string, kind: AccountPosts, enabled = true): PagedList<Status> {
  const paged = usePagedList(accountStatusesQuery(useAppData(), id, kind), enabled);
  const list = { ...paged, items: showList(useShadow(), paged.items) };
  if (kind !== "media") return list;
  return { ...list, items: list.items.filter((s) => (s.boost ?? s).media_attachments.length > 0) };
}

/**
 * What the viewer may do to their relationship with an account: follow it, block it, mute it, be notified of its new
 * posts (subscribe), each or its undoing, and remove it from their followers.
 */
export type RelationshipChange =
  "follow" | "unfollow" | "block" | "unblock" | "mute" | "unmute" | "subscribe" | "unsubscribe" | "remove-follower";

async function send({ api }: AppData, id: string, change: RelationshipChange): Promise<Relationship | undefined> {
  const path = { params: { path: { id } } };
  switch (change) {
    case "subscribe":
      return (await api.POST("/accounts/{id}/subscribe", path)).data;
    case "unsubscribe":
      return (await api.DELETE("/accounts/{id}/subscribe", path)).data;
    case "remove-follower":
      return (await api.DELETE("/accounts/{id}/follower", path)).data;
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

/**
 * How a write marks what it may change: `shown` reads the shown screens' lists now; `later` leaves them as they are
 * until they open again, so that what the reader pressed stays where it is (a post they muted the author of).
 */
export type Marking = "shown" | "later";

/** Puts a relationship the server answered into the cache, and marks what it may change. */
export async function cacheRelationship(
  data: AppData,
  relationship: Relationship,
  marking: Marking = "shown",
): Promise<void> {
  const { queryClient } = data;
  queryClient.setQueryData(relationshipQuery(data, relationship.id).queryKey, { items: [relationship] });
  await markAccountLists(queryClient, relationship.id, marking);
}

/**
 * Marks what a change of a relationship may change: the accounts with their counts and their lists of follows, the
 * timelines and the notifications, the lists of the accounts the viewer blocks and mutes, and the posts of the
 * account. The marks of screens not shown are read when one
 * opens; the shown ones are read now, or with `later` when they open again.
 */
export async function markAccountLists(queryClient: QueryClient, id: string, marking: Marking): Promise<void> {
  const refetchType = marking === "shown" ? "active" : "none";
  const lists = ["/accounts/{id}/followers", "/accounts/{id}/following", "/notifications", "/blocks", "/mutes"];
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/accounts/lookup"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/accounts/{id}"] }),
    queryClient.invalidateQueries({
      predicate: ({ queryKey }) =>
        typeof queryKey[1] === "string" && (queryKey[1].startsWith("/timelines/") || lists.includes(queryKey[1])),
      refetchType,
    }),
    queryClient.invalidateQueries({
      predicate: ({ queryKey }) =>
        queryKey[1] === "/accounts/{id}/statuses" && JSON.stringify(queryKey[2]).includes(`"${id}"`),
      refetchType,
    }),
  ]);
}

/**
 * Changes the viewer's relationship with the account of `id`, and resolves once the cache holds what the server
 * answered and the shown screens read what it marked (`marking`). It rejects with the server's problem.
 */
export async function changeRelationship(
  data: AppData,
  id: string,
  change: RelationshipChange,
  marking: Marking = "shown",
): Promise<void> {
  const relationship = await send(data, id, change);
  if (relationship !== undefined) await cacheRelationship(data, relationship, marking);
}

/** Changes the viewer's relationship with an account, from a screen (`changeRelationship`). */
export function useChangeRelationship(): (id: string, change: RelationshipChange, marking?: Marking) => Promise<void> {
  const data = useAppData();
  return (id, change, marking) => changeRelationship(data, id, change, marking);
}

/** The most characters of the viewer's private note on an account (Unicode code points). */
export const accountNoteLimit = 2000;

/**
 * Writes the viewer's private note on the account of `id`; an empty note removes it. Resolves once the cache holds
 * the relationship the server answered, which carries the note; rejects with the server's problem.
 */
export async function writeAccountNote(data: AppData, id: string, note: string): Promise<void> {
  const path = { params: { path: { id } } };
  const relationship =
    note === ""
      ? (await data.api.DELETE("/accounts/{id}/note", path)).data
      : (await data.api.PUT("/accounts/{id}/note", { ...path, body: { note } })).data;
  if (relationship !== undefined)
    data.queryClient.setQueryData(relationshipQuery(data, relationship.id).queryKey, { items: [relationship] });
}

/** Writes the viewer's private note on an account, from a screen (`writeAccountNote`). */
export function useWriteAccountNote(): (id: string, note: string) => Promise<void> {
  const data = useAppData();
  return (id, note) => writeAccountNote(data, id, note);
}
