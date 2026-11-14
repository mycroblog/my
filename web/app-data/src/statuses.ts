// SPDX-License-Identifier: AGPL-3.0-only
// The statuses of the cache. A status appears in many queries (each timeline, a thread, a notification, a boost, a
// quote); there is no store of
// its own: a change that the answer of a write makes certain is written into every copy (ADR-0028 §6, §7).
import type { components } from "@mycroblog/mykit";
import type { InfiniteData, QueryClient } from "@tanstack/react-query";

/** A status of the client API, as a timeline carries it. */
export type Status = components["schemas"]["Status"];

/** A page of statuses with the cursor of the next one. */
export type StatusPage = components["schemas"]["StatusPage"];

type Notification = components["schemas"]["Notification"];
type StatusContext = components["schemas"]["StatusContext"];

/** The cached pages of a list, each keyed by the cursor it was fetched with (undefined for the first). */
type Pages<T> = InfiniteData<{ items: T[] }, string | undefined>;

type MapStatus = (status: Status) => Status;

/** The items of each page mapped; the pages, and the data, kept as they were where nothing changed. */
function mapPages<T>(data: Pages<T>, item: (t: T) => T): Pages<T> {
  const pages = data.pages.map((page) => {
    const items = page.items.map(item);
    return items.every((t, i) => t === page.items[i]) ? page : { ...page, items };
  });
  return pages.every((page, i) => page === data.pages[i]) ? data : { ...data, pages };
}

/** The items of each page that `keep` keeps, mapped; the pages, and the data, kept as they were where nothing changed. */
function filterPages<T>(data: Pages<T>, keep: (t: T) => boolean, item: (t: T) => T): Pages<T> {
  const pages = data.pages.map((page) => {
    const items = page.items.filter(keep).map(item);
    return items.length === page.items.length && items.every((t, i) => t === page.items[i]) ? page : { ...page, items };
  });
  return pages.every((page, i) => page === data.pages[i]) ? data : { ...data, pages };
}

function mapList(list: Status[], map: MapStatus): Status[] {
  const mapped = list.map(map);
  return mapped.every((s, i) => s === list[i]) ? list : mapped;
}

/**
 * How a query that holds statuses changes them, by the path of its operation (the timelines' and an account's pages,
 * the notifications' pages by the status each is about, a status alone, and a thread): `map` writes a change into each
 * status, and `remove` takes a deleted status out, the boosts of it too, a quote of it shown deleted. A notification
 * about the deleted status leaves the list, as the server leaves it out; the deleted status alone is read again (a 404).
 */
type Mapper = {
  readonly map: (data: unknown, map: MapStatus) => unknown;
  readonly remove: (data: unknown, id: string) => unknown;
};

function mapperOf(path: string): Mapper | undefined {
  if (path.startsWith("/timelines/") || path === "/accounts/{id}/statuses")
    return {
      map: (data, map) => mapPages(data as Pages<Status>, map),
      remove: (data, id) =>
        filterPages(
          data as Pages<Status>,
          (s) => s.id !== id && s.boost?.id !== id,
          (s) => unquote(s, id),
        ),
    };
  if (path === "/notifications") {
    const about = (map: MapStatus) => (n: Notification) => {
      if (n.status === undefined) return n;
      const status = map(n.status);
      return status === n.status ? n : { ...n, status };
    };
    return {
      map: (data, map) => mapPages(data as Pages<Notification>, about(map)),
      remove: (data, id) =>
        filterPages(
          data as Pages<Notification>,
          (n) => n.status?.id !== id && n.status?.boost?.id !== id,
          about((s) => unquote(s, id)),
        ),
    };
  }
  if (path === "/statuses/{id}")
    return { map: (data, map) => map(data as Status), remove: (data, id) => unquote(data as Status, id) };
  if (path === "/statuses/{id}/context") {
    const each = (change: (list: Status[]) => Status[]) => (data: unknown) => {
      const context = data as StatusContext;
      const ancestors = change(context.ancestors);
      const descendants = change(context.descendants);
      return ancestors === context.ancestors && descendants === context.descendants
        ? context
        : { ancestors, descendants };
    };
    return {
      map: (data, map) => each((list) => mapList(list, map))(data),
      remove: (data, id) => each((list) => [...withoutStatus(list, id)])(data),
    };
  }
  return undefined;
}

/** Each cached query that holds statuses, with how it changes them. */
function* statusQueries(queryClient: QueryClient) {
  for (const query of queryClient.getQueryCache().getAll()) {
    const [method, path] = query.queryKey;
    if (method !== "get" || typeof path !== "string") continue;
    const mapper = mapperOf(path);
    if (mapper !== undefined) yield { query, mapper };
  }
}

/**
 * Writes update into every cached copy of the status with the ID given: the items of each timeline and of an
 * account's posts, the notifications about it, the status alone and the threads it is in, and a boost or quote of it.
 * A copy that update returns unchanged keeps its query's data as it was.
 */
export function updateCachedStatuses(queryClient: QueryClient, id: string, update: (status: Status) => Status): void {
  const map: MapStatus = (status) => mapStatus(status, id, update);
  for (const { query, mapper } of statusQueries(queryClient)) {
    if (query.state.data === undefined) continue;
    const next = mapper.map(query.state.data, map);
    if (next !== query.state.data) queryClient.setQueryData(query.queryKey, next);
  }
}

/**
 * Takes the deleted status with the ID given out of every cached query that holds it: the timelines, an account's
 * posts, the threads and the notifications about it lose it and the boosts of it, a quote of it shows it deleted, as
 * the server answers once it is gone, and the status alone is read again. A query that a fetch is reading loses it
 * again when the fetch ends, since the fetch writes what it read over the change.
 */
export function removeCachedStatus(queryClient: QueryClient, id: string): void {
  for (const { query, mapper } of statusQueries(queryClient)) {
    const remove = (data: unknown) => (data === undefined ? data : mapper.remove(data, id));
    if (query.queryKey[1] === "/statuses/{id}" && (query.state.data as Status | undefined)?.id === id) {
      void queryClient.invalidateQueries({ queryKey: query.queryKey, exact: true });
      continue;
    }
    const next = remove(query.state.data);
    if (next !== query.state.data) queryClient.setQueryData(query.queryKey, next);
    if (query.state.fetchStatus !== "fetching" || !query.promise) continue;
    const again = () => {
      queryClient.setQueryData(query.queryKey, remove);
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
