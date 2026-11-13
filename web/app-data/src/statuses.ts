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
