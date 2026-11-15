// SPDX-License-Identifier: AGPL-3.0-only
// The statuses of the cache. A status appears in many queries (each timeline, a thread, a notification, a boost, a
// quote) as the server answered each; the cache is never rewritten to change them, the shadow (shadow.ts) shows a
// write over every copy (ADR-0028 §7, §18). This file only finds the copies.
import type { components } from "@mycroblog/mykit";
import type { Query, QueryClient } from "@tanstack/react-query";

/** A status of the client API, as a timeline carries it. */
export type Status = components["schemas"]["Status"];

/** A page of statuses with the cursor of the next one. */
export type StatusPage = components["schemas"]["StatusPage"];

type Notification = components["schemas"]["Notification"];
type StatusContext = components["schemas"]["StatusContext"];

/** The cached pages of a list. */
type Pages<T> = { readonly pages: readonly { readonly items: readonly T[] }[] };

/**
 * The statuses the data of a query holds, by the path of its operation: the timelines' and an account's pages, the
 * notifications' pages by the status each is about, a status alone, and a thread; none for another query.
 */
function* statusesIn(path: unknown, data: unknown): Generator<Status> {
  if (data === undefined || typeof path !== "string") return;
  if (path.startsWith("/timelines/") || path === "/accounts/{id}/statuses") {
    for (const page of (data as Pages<Status>).pages) yield* page.items;
  } else if (path === "/notifications") {
    for (const page of (data as Pages<Notification>).pages)
      for (const n of page.items) if (n.status !== undefined) yield n.status;
  } else if (path === "/statuses/{id}") {
    yield data as Status;
  } else if (path === "/statuses/{id}/context") {
    const context = data as StatusContext;
    yield* context.ancestors;
    yield* context.descendants;
  }
}

/** `status`, then the status it boosts and the one it quotes, each with those it boosts and quotes. */
function* withInner(status: Status): Generator<Status> {
  yield status;
  if (status.boost !== undefined) yield* withInner(status.boost);
  const quoted = status.quote?.quoted_status;
  if (quoted !== undefined) yield* withInner(quoted);
}

/** Each copy of a status that the query holds, with those its statuses boost or quote. */
export function copiesIn(query: Query): Generator<Status> {
  return copiesOf(query, query.state.data);
}

/** Each copy of a status that `data`, as the data of the query, holds, with those its statuses boost or quote. */
export function* copiesOf(query: Query, data: unknown): Generator<Status> {
  const [method, path] = query.queryKey;
  if (method !== "get") return;
  for (const status of statusesIn(path, data)) yield* withInner(status);
}

/** Each copy of a status that some cached query holds. */
export function* cachedCopies(queryClient: QueryClient): Generator<Status> {
  for (const query of queryClient.getQueryCache().getAll()) yield* copiesIn(query);
}

/** A copy of the status with the ID given as some cached query holds it (or boosts or quotes it), else undefined. */
export function findCachedCopy(queryClient: QueryClient, id: string): Status | undefined {
  for (const status of cachedCopies(queryClient)) if (status.id === id) return status;
  return undefined;
}
