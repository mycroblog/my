// SPDX-License-Identifier: AGPL-3.0-only
// The notifications and their read state (ADR-0028 §7, §18, ADR-0158 §7). The server holds it for every client: a
// marker, which only moves forward and reads every notification up to it, and the notifications read on their own past
// it, which each notification's `read` tells. A notification the stream receives goes to the top of each cached list it
// belongs to and counts as unread; after a break of the stream, the lists and the counts are read again.
//
// The reader reads a notification by clicking it or by scrolling it out above the view, newest first, so the newest
// are read before the older ones under them. The marker moves up to the newest notification below which every one is
// read: once the reader read the oldest unread one, or all of them. A notification read above an unread one is sent to
// the server on its own. From the read until the server's answer, it counts as read here, in the list's marks and in
// the unread count.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { receiveInteractionRequests } from "./approvals";
import { receiveFollowRequests } from "./follow-requests";
import { pagedQuery, usePagedList, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";
import { useShadow } from "./shadow";
import type { Status } from "./statuses";

/** A notification of the client API (named apart from the browser's Notification). */
export type NotificationItem = components["schemas"]["Notification"];

/** The kind of a notification. */
export type NotificationType = components["schemas"]["NotificationType"];

type NotificationMarker = components["schemas"]["NotificationMarker"];

/** What the stream says the reader read on a device: the marker, the count, and the one read on its own. */
export type NotificationsRead = components["schemas"]["NotificationsRead"];

/** How many notification requests wait, by account and by notification (ADR-0158 §7). */
export type NotificationRequestSummary = components["schemas"]["NotificationRequestSummary"];

/**
 * Which notifications a list holds: all of them, or those that speak to the reader (a mention, a quote, a new post of
 * an account they subscribe to, ADR-0086 §17).
 */
export type NotificationFilter = "all" | "mentions";

const filters: Readonly<Record<NotificationFilter, readonly NotificationType[] | undefined>> = {
  all: undefined,
  mentions: ["mention", "quote", "status"],
};

/** The notifications that tell of a reply, boost or quote waiting for the reader's approval (approvals.ts). */
const requestTypes: ReadonlySet<NotificationType> = new Set(["reply_request", "boost_request", "quote_request"]);

/** The read marker of the notifications and the number of notifications newer than it. */
export function notificationMarkerQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/notifications/marker");
}

/** How many notification requests wait. */
export function notificationRequestSummaryQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/notifications/requests/summary");
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

// ---- What the reader read, until the server answers ----------------------------------------------------------------

type Store = {
  /** The notifications the reader read past the marker that the cache does not yet tell are read. */
  seen: ReadonlySet<string>;
  readonly listeners: Set<() => void>;
  moving: Promise<void>;
  /** Who is told of each notification the stream brings. */
  readonly arrivals: Set<(notification: NotificationItem) => void>;
};

const stores = new WeakMap<AppData, Store>();

function storeOf(data: AppData): Store {
  let store = stores.get(data);
  if (!store) {
    store = { seen: new Set(), listeners: new Set(), moving: Promise.resolve(), arrivals: new Set() };
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

/** Whether the cache holds the notification of `id` as the server answered it read. */
function cachedRead(data: AppData, id: string): boolean {
  const one = data.queryClient.getQueryData<NotificationItem>(notificationQuery(data, id).queryKey);
  if (one?.read === true) return true;
  return lists(data).some(({ queryKey }) =>
    data.queryClient
      .getQueryData<Pages<NotificationItem>>(queryKey)
      ?.pages.some((page) => page.items.some((n) => n.id === id && n.read)),
  );
}

/** Tells the cached lists and the notification of `id` that it is read. */
function cacheRead(data: AppData, id: string): void {
  const { queryClient } = data;
  for (const { queryKey } of lists(data)) {
    const cached = queryClient.getQueryData<Pages<NotificationItem>>(queryKey);
    if (cached === undefined) continue;
    queryClient.setQueryData<Pages<NotificationItem>>(queryKey, {
      ...cached,
      pages: cached.pages.map((page) => ({
        ...page,
        items: page.items.map((n) => (n.id === id && !n.read ? { ...n, read: true } : n)),
      })),
    });
  }
  queryClient.setQueryData<NotificationItem>(notificationQuery(data, id).queryKey, (n) => n && { ...n, read: true });
}

/**
 * Reads the notification of `id` on the server on its own, above the marker: the cache tells it read, and the count
 * the server answered goes down by it until the marker is read again.
 */
async function postRead(data: AppData, id: string): Promise<void> {
  await data.api.POST("/notifications/{id}/read", { params: { path: { id } } });
  const store = storeOf(data);
  cacheRead(data, id);
  data.queryClient.setQueryData<NotificationMarker>(
    notificationMarkerQuery(data).queryKey,
    (marker) => marker && { ...marker, unread_count: Math.max(0, marker.unread_count - 1) },
  );
  if (store.seen.has(id)) setSeen(store, new Set([...store.seen].filter((seen) => seen !== id)));
  void data.queryClient.invalidateQueries({ queryKey: notificationMarkerQuery(data).queryKey, exact: true });
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
    if (!seen.has(n.id) && !n.read) break;
    upTo = n.id;
  }
  return upTo;
}

/**
 * Reads from the server what a read goes by and the cache does not hold: the marker, which says whether a notification
 * is unread, and the first page of the full list, by which the marker moves. The address a push opens starts the app
 * with neither. A failure leaves them unknown.
 */
async function learn(data: AppData): Promise<void> {
  const { queryClient } = data;
  const list = notificationsQuery(data, "all");
  await Promise.all([
    markerOf(data) === undefined ? queryClient.query({ ...notificationMarkerQuery(data), staleTime: Infinity }) : null,
    queryClient.getQueryData(list.queryKey) === undefined
      ? queryClient.infiniteQuery({ ...list, staleTime: Infinity })
      : null,
  ]).catch(() => undefined);
}

/**
 * Tells the server that the reader read the notification of `id`, one write after another: the marker moves as far as
 * what the reader read allows when that reaches it, and it is read on its own otherwise.
 */
function advance(data: AppData, id: string): Promise<void> {
  const store = storeOf(data);
  store.moving = store.moving
    .then(async () => {
      await learn(data);
      if (!newer(id, markerOf(data)?.last_read_id)) return;
      const upTo = readUpTo(data, store.seen);
      if (upTo !== undefined && !newer(id, upTo)) await putMarker(data, upTo);
      else await postRead(data, id);
    })
    .catch(() => {
      // The server keeps it unread; it stays read here for the session, and the next notification read moves the
      // marker again.
    });
  return store.moving;
}

/** Counts the notification of `id` as read (acted on, or scrolled out above the view), and tells the server. */
function readNotification(data: AppData, id: string): Promise<void> {
  const store = storeOf(data);
  const marker = markerOf(data);
  // Without the marker nothing says whether it is unread: it is read once the server said where the marker is.
  if (marker === undefined)
    return learn(data).then(() => (markerOf(data) === undefined ? undefined : readNotification(data, id)));
  if (store.seen.has(id) || !newer(id, marker.last_read_id) || cachedRead(data, id)) return Promise.resolve();
  setSeen(store, new Set([...store.seen, id]));
  return advance(data, id);
}

/**
 * Reads the notifications of the announcement of `id`, which the reader marked read (on its page, or its line above
 * the home timeline): their cached copies say it is read, and each counts as read as if the reader read it here.
 */
export function readAnnouncementNotifications(data: AppData, id: string): void {
  const told = new Set<string>();
  for (const { queryKey } of lists(data))
    data.queryClient.setQueryData<Pages<NotificationItem>>(queryKey, (cached) => {
      if (!cached?.pages.some((page) => page.items.some((n) => n.announcement?.id === id))) return cached;
      return {
        ...cached,
        pages: cached.pages.map((page) => ({
          ...page,
          items: page.items.map((n) => {
            if (n.announcement?.id !== id) return n;
            told.add(n.id);
            return { ...n, announcement: { ...n.announcement, read: true } };
          }),
        })),
      };
    });
  for (const notification of told) void readNotification(data, notification);
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

// ---- Dismissing ----------------------------------------------------------------------------------------------------

/** The cached lists with the notification of `id` taken out, and puts back what they held before. */
function takeOut(data: AppData, id: string): () => void {
  const before = lists(data).map(({ queryKey }) => ({
    queryKey,
    cached: data.queryClient.getQueryData<Pages<NotificationItem>>(queryKey),
  }));
  for (const { queryKey, cached } of before) {
    if (cached === undefined) continue;
    data.queryClient.setQueryData<Pages<NotificationItem>>(queryKey, {
      ...cached,
      pages: cached.pages.map((page) => ({ ...page, items: page.items.filter((n) => n.id !== id) })),
    });
  }
  return () => {
    for (const { queryKey, cached } of before)
      if (cached !== undefined) data.queryClient.setQueryData(queryKey, cached);
  };
}

/**
 * Dismisses the notification of `id`: it leaves every list at once, and the server takes it out of the list and the
 * unread count, which is read again. A failure puts it back and rejects with the problem.
 */
async function dismissNotification(data: AppData, id: string): Promise<void> {
  const putBack = takeOut(data, id);
  try {
    await data.api.POST("/notifications/{id}/dismiss", { params: { path: { id } } });
  } catch (e) {
    putBack();
    throw e;
  }
  // The server counts it no more, so it is no longer one the reader read that the server has yet to answer: the press
  // that dismissed it read it, and it would be taken off the unread count twice.
  const store = storeOf(data);
  if (store.seen.has(id)) setSeen(store, new Set([...store.seen].filter((seen) => seen !== id)));
  void data.queryClient.invalidateQueries({ queryKey: notificationMarkerQuery(data).queryKey, exact: true });
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
  if (notification !== undefined && requestTypes.has(notification.type)) receiveInteractionRequests(data);
  if (notification?.type === "follow_request") receiveFollowRequests(data);
  if (notification !== undefined) {
    for (const told of storeOf(data).arrivals) told(notification);
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

/**
 * Tells `listener` of each notification the stream brings, as it arrives, until the function it returns is called. A
 * notification the reader's policy holds back is none the stream brings (ADR-0158 §7).
 */
export function watchNotifications(data: AppData, listener: (notification: NotificationItem) => void): () => void {
  const { arrivals } = storeOf(data);
  arrivals.add(listener);
  return () => {
    arrivals.delete(listener);
  };
}

/**
 * Puts what the stream says the reader read on a device, this one or another, into the cache: the marker and the
 * count, unless the cache holds a marker further on, and the notification read on its own as read.
 */
export function receiveNotificationsRead(data: AppData, read: NotificationsRead): void {
  const store = storeOf(data);
  data.queryClient.setQueryData<NotificationMarker>(notificationMarkerQuery(data).queryKey, (cached) =>
    cached?.last_read_id !== undefined && newer(cached.last_read_id, read.last_read_id)
      ? cached
      : {
          ...(read.last_read_id === undefined ? {} : { last_read_id: read.last_read_id }),
          unread_count: read.unread_count,
        },
  );
  if (read.id !== undefined) cacheRead(data, read.id);
  const last = markerOf(data)?.last_read_id;
  const seen = new Set([...store.seen].filter((id) => newer(id, last) && id !== read.id));
  if (seen.size !== store.seen.size) setSeen(store, seen);
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
  receiveInteractionRequests(data);
  receiveFollowRequests(data);
  for (const { queryKey } of lists(data)) void queryClient.invalidateQueries({ queryKey, exact: true });
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
 * The number of unread notifications of the viewer, for the shell: the server's, less those the reader read that the
 * server has yet to answer; undefined until the server answered.
 */
export function useUnreadNotifications(): number | undefined {
  const data = useAppData();
  const seen = useSeen(data);
  const marker = useQuery(notificationMarkerQuery(data)).data;
  if (marker === undefined) return undefined;
  const read = [...seen].filter((id) => newer(id, marker.last_read_id)).length;
  return Math.max(0, marker.unread_count - read);
}

/** Each notification with the status it is about as it shows, one kept as it is while that does not change. */
const shownNotifications = new WeakMap<NotificationItem, { status: Status; shown: NotificationItem }>();

/**
 * The notifications of a filter, for the list, each with the status it is about as it shows (shadow.ts); one about a
 * status gone leaves the list, as the server leaves it out.
 */
export function useNotifications(filter: NotificationFilter): PagedList<NotificationItem> {
  const list = usePagedList(notificationsQuery(useAppData(), filter));
  const show = useShadow();
  const items = list.items.flatMap((n) => {
    if (n.status === undefined) return [n];
    const status = show(n.status);
    if (status === undefined) return [];
    if (status === n.status) return [n];
    const memo = shownNotifications.get(n);
    if (memo?.status === status) return [memo.shown];
    const shown = { ...n, status };
    shownNotifications.set(n, { status, shown });
    return [shown];
  });
  return { ...list, items };
}

/** What the list reads of the marker, and how the reader reads notifications. */
export type NotificationReads = {
  /** Whether the notification of `id` is unread; false until the marker is known. */
  readonly unread: (id: string) => boolean;
  /** Reads the notification of `id`: the reader acted on it, or it scrolled out above the view. */
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
    unread: (id) => marker !== undefined && newer(id, marker.last_read_id) && !seen.has(id) && !cachedRead(data, id),
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

/** Dismisses a notification for the session of the nearest AppDataProvider (dismissNotification). */
export function useDismissNotification(): (id: string) => Promise<void> {
  const data = useAppData();
  return (id) => dismissNotification(data, id);
}

// ---- One notification --------------------------------------------------------------------------------------------

/** One of the reader's notifications, by its ID (where a push or an email about it leads). */
export function notificationQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/notifications/{id}", { params: { path: { id } } });
}

/** One notification as a screen reads it, and how it came. */
export type OneNotification = {
  readonly notification: NotificationItem | undefined;
  /** `gone` once the server says the reader has no such notification (404): dismissed, another's, or none. */
  readonly status: "pending" | "success" | "error" | "gone";
  readonly refetch: () => void;
};

/** The reader's notification of `id`. */
export function useNotification(id: string): OneNotification {
  const query = useQuery(notificationQuery(useAppData(), id));
  const { error } = query;
  return {
    notification: query.data,
    status:
      query.status === "error"
        ? error instanceof ProblemError && error.status === 404
          ? "gone"
          : "error"
        : query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}
