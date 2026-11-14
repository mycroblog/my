// SPDX-License-Identifier: AGPL-3.0-only
// The statuses of the cache. A status appears in many queries (each timeline, a boost, a quote); there is no store of
// its own: a change that the answer of a write makes certain is written into every copy (ADR-0028 §6, §7).
import type { components } from "@mycroblog/mykit";
import type { InfiniteData, QueryClient } from "@tanstack/react-query";

/** A status of the client API, as a timeline carries it. */
export type Status = components["schemas"]["Status"];

/** A page of statuses with the cursor of the next one. */
export type StatusPage = components["schemas"]["StatusPage"];

/** The cached pages of a timeline, each keyed by the cursor it was fetched with (undefined for the first). */
type StatusPages = InfiniteData<StatusPage, string | undefined>;

/** The start of the path of every timeline operation, the second element of its query's key. */
const timelinePathPrefix = "/timelines/";

/**
 * Writes update into every cached copy of the status with the ID given: the items of each timeline, and a boost or
 * quote of it. A copy that update returns unchanged keeps its query's data as it was.
 */
export function updateCachedStatuses(queryClient: QueryClient, id: string, update: (status: Status) => Status): void {
  queryClient.setQueriesData<StatusPages>({ predicate: (query) => isTimelineKey(query.queryKey) }, (data) => {
    if (data === undefined) return data;
    const pages = data.pages.map((page) => {
      const items = page.items.map((status) => mapStatus(status, id, update));
      return items.every((status, i) => status === page.items[i]) ? page : { ...page, items };
    });
    return pages.every((page, i) => page === data.pages[i]) ? data : { ...data, pages };
  });
}

/**
 * Takes the deleted status with the ID given out of every cached timeline: the status, and the boosts of it, leave the
 * items, and a quote of it shows it deleted, as the server answers once it is gone. A timeline that a fetch is reading
 * loses it again when the fetch ends, since the fetch writes what it read over the change.
 */
export function removeCachedStatus(queryClient: QueryClient, id: string): void {
  const remove = (data: StatusPages | undefined) => {
    if (data === undefined) return data;
    const pages = data.pages.map((page) => {
      const items = withoutStatus(page.items, id);
      return items === page.items ? page : { ...page, items: [...items] };
    });
    return pages.every((page, i) => page === data.pages[i]) ? data : { ...data, pages };
  };
  const filters = { predicate: (query: { queryKey: readonly unknown[] }) => isTimelineKey(query.queryKey) };
  queryClient.setQueriesData<StatusPages>(filters, remove);
  for (const query of queryClient.getQueryCache().findAll(filters)) {
    if (query.state.fetchStatus !== "fetching" || !query.promise) continue;
    const again = () => {
      queryClient.setQueryData<StatusPages>(query.queryKey, remove);
    };
    query.promise.then(again, again);
  }
}

/**
 * `statuses` without the deleted status with the ID given and the boosts of it, and with a quote of it shown deleted;
 * `statuses` itself when none of them has it.
 */
export function withoutStatus(statuses: readonly Status[], id: string): readonly Status[] {
  const kept = statuses.filter((status) => status.id !== id && status.boost?.id !== id).map((s) => unquote(s, id));
  return kept.length === statuses.length && kept.every((status, i) => status === statuses[i]) ? statuses : kept;
}

function isTimelineKey(key: readonly unknown[]): boolean {
  const path = key[1];
  return key[0] === "get" && typeof path === "string" && path.startsWith(timelinePathPrefix);
}

/** Applies update to status if it has the ID, and to the status it boosts or quotes if that one has it. */
function mapStatus(status: Status, id: string, update: (status: Status) => Status): Status {
  let next = status.id === id ? update(status) : status;
  if (next.boost !== undefined) {
    const boost = mapStatus(next.boost, id, update);
    if (boost !== next.boost) next = { ...next, boost };
  }
  const quoted = next.quote?.quoted_status;
  if (next.quote !== undefined && quoted !== undefined) {
    const mapped = mapStatus(quoted, id, update);
    if (mapped !== quoted) next = { ...next, quote: { ...next.quote, quoted_status: mapped } };
  }
  return next;
}

/** Shows a quote of the deleted status with the ID given deleted, in status and in the status it boosts. */
function unquote(status: Status, id: string): Status {
  if (status.quote?.quoted_status?.id === id) return { ...status, quote: { state: "deleted" } };
  if (status.boost === undefined) return status;
  const boost = unquote(status.boost, id);
  return boost === status.boost ? status : { ...status, boost };
}
