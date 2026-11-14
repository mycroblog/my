// SPDX-License-Identifier: AGPL-3.0-only
// The notifications and their read marker (ADR-0028 §7, §18, ADR-0158 §7). The server holds the marker, shared by
// every client: the notifications up to it are read, those after it unread, and it only moves forward. A notification
// the stream receives goes to the top of each cached list it belongs to and counts as unread; after a break of the
// stream, the lists and the counts are read again.
//
// The reader reads a notification by clicking it or by scrolling it out above the view, newest first, so the newest
// are read before the older ones under them. The marker cannot say that, so what the reader read after the marker is
// kept here, for the session, and the marker moves up to the newest notification below which every one is read: once
// the reader read the oldest unread one, or all of them. Until it moves, those count as read here, in the list's marks
// and in the unread count.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import { cacheRelationship } from "./accounts";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";

/** A notification of the client API (named apart from the browser's Notification). */
export type NotificationItem = components["schemas"]["Notification"];

/** The kind of a notification. */
export type NotificationType = components["schemas"]["NotificationType"];

/** A reply, boost or quote of the viewer's post that waits for the viewer's approval. */
export type InteractionRequest = components["schemas"]["InteractionRequest"];

type NotificationMarker = components["schemas"]["NotificationMarker"];

/** How many notification requests wait, by account and by notification (ADR-0158 §7). */
export type NotificationRequestSummary = components["schemas"]["NotificationRequestSummary"];

/** Which notifications a list holds: all of them, or those that speak to the reader (a mention, a quote). */
export type NotificationFilter = "all" | "mentions";

const filters: Readonly<Record<NotificationFilter, readonly NotificationType[] | undefined>> = {
  all: undefined,
  mentions: ["mention", "quote"],
};

/** The read marker of the notifications and the number of notifications newer than it. */
export function notificationMarkerQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/notifications/marker");
}

/** How many notification requests wait. */
export function notificationRequestSummaryQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/notifications/requests/summary");
}

/** The interaction requests that wait for the viewer: the first page, in which a notification's request is looked up. */
function interactionRequestsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/interaction_requests");
}

/** The notifications of a filter, newest first. */
export function notificationsQuery({ api, queries }: AppData, filter: NotificationFilter) {
  const types = filters[filter];
  const query = types === undefined ? {} : { types: [...types] };
  return pagedQuery<NotificationItem>(
    queries.queryOptions("get", "/notifications", { params: { query } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/notifications", {
        params: { query: cursor === undefined ? query : { ...query, cursor } },
        signal,
      });
      return data;
    },
  );
}

const lists = (data: AppData) =>
  (Object.keys(filters) as NotificationFilter[]).map((filter) => ({
    filter,
    queryKey: notificationsQuery(data, filter).queryKey,
  }));

/** IDs are UUIDv7: in their canonical form, a later one sorts after an earlier one. */
const newer = (a: string, b: string | undefined) => b === undefined || a > b;

// ---- What the reader read after the marker -------------------------------------------------------------------------

type Store = { seen: ReadonlySet<string>; readonly listeners: Set<() => void>; moving: Promise<void> };

const stores = new WeakMap<AppData, Store>();

function storeOf(data: AppData): Store {
  let store = stores.get(data);
  if (!store) {
    store = { seen: new Set(), listeners: new Set(), moving: Promise.resolve() };
    stores.set(data, store);
  }
  return store;
}

function setSeen(store: Store, seen: ReadonlySet<string>): void {
  store.seen = seen;
  for (const listener of store.listeners) listener();
}

const markerOf = (data: AppData) =>
  data.queryClient.getQueryData<NotificationMarker>(notificationMarkerQuery(data).queryKey);

/** Puts a marker the server answered into the cache, unless one further on is there already. */
function cacheMarker(data: AppData, marker: NotificationMarker): void {
  const store = storeOf(data);
  data.queryClient.setQueryData<NotificationMarker>(notificationMarkerQuery(data).queryKey, (cached) =>
    cached?.last_read_id !== undefined && !newer(marker.last_read_id ?? "", cached.last_read_id) ? cached : marker,
  );
  const last = markerOf(data)?.last_read_id;
  const seen = new Set([...store.seen].filter((id) => newer(id, last)));
  if (seen.size !== store.seen.size) setSeen(store, seen);
}

/** Moves the marker on the server to `id`, and puts what it answered into the cache. */
async function putMarker(data: AppData, id: string): Promise<void> {
  const { data: marker } = await data.api.PUT("/notifications/marker", { body: { last_read_id: id } });
  if (marker !== undefined) cacheMarker(data, marker);
}

/**
 * The newest notification of the full list up to which every unread one is read, or undefined. The list must reach
 * the marker, or its own end, so that no unread notification between them is left out.
 */
function readUpTo(data: AppData, seen: ReadonlySet<string>): string | undefined {
  const last = markerOf(data)?.last_read_id;
  const cached = data.queryClient.getQueryData<Pages<NotificationItem>>(notificationsQuery(data, "all").queryKey);
  if (cached === undefined) return undefined;
  const items = cached.pages.flatMap((page) => page.items);
  const end = cached.pages.at(-1)?.next_cursor;
  const reaches = items.some((n) => !newer(n.id, last)) || end === undefined || end === "";
  if (!reaches) return undefined;
  let upTo: string | undefined;
  for (const n of items.filter((n) => newer(n.id, last)).reverse()) {
    if (!seen.has(n.id)) break;
    upTo = n.id;
  }
  return upTo;
}

/** Moves the marker as far as what the reader read allows, one move after another. */
function advance(data: AppData): Promise<void> {
  const store = storeOf(data);
  store.moving = store.moving
    .then(async () => {
      const upTo = readUpTo(data, store.seen);
      if (upTo !== undefined) await putMarker(data, upTo);
    })
    .catch(() => {
      // The marker stays where it was; the next notification read moves it again.
    });
  return store.moving;
}

/** Counts the notification of `id` as read (clicked, or scrolled out above the view), and moves the marker. */
function readNotification(data: AppData, id: string): Promise<void> {
  const store = storeOf(data);
  const marker = markerOf(data);
  if (marker === undefined || store.seen.has(id) || !newer(id, marker.last_read_id)) return Promise.resolve();
  setSeen(store, new Set([...store.seen, id]));
  return advance(data);
}

/** Moves the marker to the newest notification the cache holds: every one is read. It rejects with the problem. */
async function readAllNotifications(data: AppData): Promise<void> {
  let newest: string | undefined;
  for (const { queryKey } of lists(data)) {
    const first = data.queryClient.getQueryData<Pages<NotificationItem>>(queryKey)?.pages[0]?.items[0];
    if (first !== undefined && newer(first.id, newest)) newest = first.id;
  }
  if (newest === undefined || !newer(newest, markerOf(data)?.last_read_id)) return;
  await putMarker(data, newest);
}

// ---- The stream ----------------------------------------------------------------------------------------------------

/**
 * Puts a new notification that the stream received at the top of each cached list it belongs to, and counts it as
 * unread. While a list or the count is being read, the answer may or may not hold it, so it is read again instead.
 */
export function receiveNotification(data: AppData, notification?: NotificationItem): void {
  const { queryClient } = data;
  const fetching = (queryKey: readonly unknown[]) =>
    queryClient.getQueryCache().find({ queryKey, exact: true })?.state.fetchStatus === "fetching";
  if (notification !== undefined) {
    for (const { filter, queryKey } of lists(data)) {
      const types = filters[filter];
      if (types !== undefined && !types.includes(notification.type)) continue;
      if (fetching(queryKey)) {
        void queryClient.invalidateQueries({ queryKey, exact: true });
        continue;
      }
      queryClient.setQueryData<Pages<NotificationItem>>(queryKey, (cached) => {
        const [first, ...rest] = cached?.pages ?? [];
        if (!cached || !first || first.items.some((n) => n.id === notification.id)) return cached;
        return { ...cached, pages: [{ ...first, items: [notification, ...first.items] }, ...rest] };
      });
    }
  }
  const { queryKey } = notificationMarkerQuery(data);
  if (fetching(queryKey)) {
    void queryClient.invalidateQueries({ queryKey, exact: true });
    return;
  }
  queryClient.setQueryData<NotificationMarker>(
    queryKey,
    (marker) => marker && { ...marker, unread_count: marker.unread_count + 1 },
  );
}

/** Puts the count of the notification requests that the stream received into the cache. */
export function receiveNotificationRequests(data: AppData, summary: NotificationRequestSummary): void {
  data.queryClient.setQueryData(notificationRequestSummaryQuery(data).queryKey, summary);
}

/** Reads the counts and the lists again: notifications may have arrived while the stream was closed. */
export function receiveNotificationGap(data: AppData): void {
  const { queryClient } = data;
  void queryClient.invalidateQueries({ queryKey: notificationMarkerQuery(data).queryKey, exact: true });
  void queryClient.invalidateQueries({ queryKey: notificationRequestSummaryQuery(data).queryKey, exact: true });
  for (const { queryKey } of lists(data)) void queryClient.invalidateQueries({ queryKey, exact: true });
}

// ---- The writes of the requests ------------------------------------------------------------------------------------

/** Answers an interaction request, and resolves once the cache knows it. It rejects with the server's problem. */
async function answerInteractionRequest(data: AppData, id: string, accept: boolean): Promise<void> {
  const path = { params: { path: { id } } };
  if (accept) await data.api.POST("/interaction_requests/{id}/accept", path);
  else await data.api.POST("/interaction_requests/{id}/reject", path);
  await data.queryClient.invalidateQueries({ queryKey: interactionRequestsQuery(data).queryKey, exact: true });
}

/**
 * Answers the follow request of the account of `id`, and resolves once the cache holds the relationship the server
 * answered. It rejects with the server's problem.
 */
async function answerFollowRequest(data: AppData, id: string, accept: boolean): Promise<void> {
  const path = { params: { path: { id } } };
  const { data: relationship } = accept
    ? await data.api.POST("/follow_requests/{id}/accept", path)
    : await data.api.POST("/follow_requests/{id}/reject", path);
  if (relationship !== undefined) await cacheRelationship(data, relationship);
}

// ---- The hooks -----------------------------------------------------------------------------------------------------

function useSeen(data: AppData): ReadonlySet<string> {
  const store = storeOf(data);
  return useSyncExternalStore(
    (listener) => {
      store.listeners.add(listener);
      return () => {
        store.listeners.delete(listener);
      };
    },
    () => store.seen,
    () => store.seen,
  );
}

/**
 * The number of unread notifications of the viewer, for the shell: the server's, less those the reader read after the
 * marker; undefined until the server answered.
 */
export function useUnreadNotifications(): number | undefined {
  const data = useAppData();
  const seen = useSeen(data);
  const marker = useQuery(notificationMarkerQuery(data)).data;
  if (marker === undefined) return undefined;
  const read = [...seen].filter((id) => newer(id, marker.last_read_id)).length;
  return Math.max(0, marker.unread_count - read);
}

/** The notifications of a filter, for the list. */
export function useNotifications(filter: NotificationFilter): PagedList<NotificationItem> {
  return usePagedList(notificationsQuery(useAppData(), filter));
}

/** What the list reads of the marker, and how the reader reads notifications. */
export type NotificationReads = {
  /** Whether the notification of `id` is unread; false until the marker is known. */
  readonly unread: (id: string) => boolean;
  /** Reads the notification of `id`: it was clicked, or scrolled out above the view. */
  readonly read: (id: string) => void;
  /** Reads every notification; it resolves once the server moved the marker, and rejects with its problem. */
  readonly readAll: () => Promise<void>;
};

/** The read marker as the list shows it. */
export function useNotificationReads(): NotificationReads {
  const data = useAppData();
  const seen = useSeen(data);
  const marker = useQuery(notificationMarkerQuery(data)).data;
  return {
    unread: (id) => marker !== undefined && newer(id, marker.last_read_id) && !seen.has(id),
    read: (id) => {
      void readNotification(data, id);
    },
    readAll: () => readAllNotifications(data),
  };
}

/** How many notification requests wait; undefined until the server answered. */
export function useNotificationRequestSummary(): NotificationRequestSummary | undefined {
  return useQuery(notificationRequestSummaryQuery(useAppData())).data;
}

const requestKinds: Partial<Record<NotificationType, InteractionRequest["kind"]>> = {
  reply_request: "reply",
  boost_request: "announce",
  quote_request: "quote",
};

/**
 * The waiting interaction request of a notification that tells of one (a `reply_request`, `boost_request` or
 * `quote_request`): the request of its account about its post, among those the server lists first. Undefined while
 * unknown, for another kind, and for a request answered already.
 */
export function useInteractionRequest(notification: NotificationItem): InteractionRequest | undefined {
  const kind = requestKinds[notification.type];
  const requests = useQuery({ ...interactionRequestsQuery(useAppData()), enabled: kind !== undefined }).data;
  if (kind === undefined) return undefined;
  return requests?.items.find(
    (r) =>
      r.state === "pending" &&
      r.kind === kind &&
      r.account.id === notification.account?.id &&
      r.status.id === notification.status?.id,
  );
}

/** The answers of the requests a notification carries, from a screen. */
export function useAnswerRequest(): {
  readonly follow: (accountId: string, accept: boolean) => Promise<void>;
  readonly interaction: (requestId: string, accept: boolean) => Promise<void>;
} {
  const data = useAppData();
  return {
    follow: (accountId, accept) => answerFollowRequest(data, accountId, accept),
    interaction: (requestId, accept) => answerInteractionRequest(data, requestId, accept),
  };
}
