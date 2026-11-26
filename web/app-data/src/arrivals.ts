// SPDX-License-Identifier: AGPL-3.0-only
// The statuses that reach a streamed timeline after its first page (ADR-0013 §3–§8, §13, §17, ADR-0028 §7): the home
// timeline, each of the reader's lists, and the local and federated timelines. The streams (FR-207) hand what they
// receive to receiveHomeStatuses, receiveListStatuses and receivePublicStatuses, and the break they report when they
// connect again to receiveHomeGap, receiveListsGap and receivePublicGap. While no
// screen shows a timeline, a status goes at once to the top of its cached first page, and a break marks it to be read
// again from its first page. While one shows it, the screen's policy decides at each arrival whether it goes in now;
// what it does not take is held here, with a break when the held statuses no longer join those loaded (after a
// reconnection, or past 200), until the policy takes them: inserted at the top, or, after a break, the first page read
// again in place of every page loaded. A reader who pulls the shown timeline at its top has its first page read again
// as after a break, which brings what is held with it.
import { useEffect, useEffectEvent, useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { readFirstPageAgain } from "./paged";
import { useAppData } from "./provider";
import { receiveGone, shadowed } from "./shadow";
import { retryStream } from "./stream-retry";
import type { Status } from "./statuses";
import { homeTimelineQuery, listTimelineQuery, publicTimelineQuery, type PublicTimelineKind } from "./timeline";

/** What reached the shown timeline: statuses, as many as would go in now, or a break. */
export type Arrival = { readonly kind: "statuses"; readonly count: number } | { readonly kind: "gap" };

/**
 * A timeline a stream brings new statuses to: the home timeline, a public timeline of the instance, or the reader's list
 * of `list` (its ID).
 */
export type StreamedTimeline = "home" | PublicTimelineKind | { readonly list: string };

/** What is held for the shown timeline: the statuses, newest first, and whether they no longer join those loaded. */
type HeldArrivals = { readonly statuses: readonly Status[]; readonly gap: boolean };

/** How many held statuses still join those loaded; past it, inserting them could leave posts out between. */
export const maxHeld = 200;

type Store = {
  readonly timeline: StreamedTimeline;
  held: HeldArrivals;
  /** The policy of the screen that shows the timeline: whether what reached it goes in now. */
  policy: ((arrival: Arrival) => boolean) | undefined;
  readonly listeners: Set<() => void>;
};

const none: HeldArrivals = { statuses: [], gap: false };
const stores = new WeakMap<AppData, Map<string, Store>>();

const nameOf = (timeline: StreamedTimeline) => (typeof timeline === "string" ? timeline : `list:${timeline.list}`);

function storesOf(data: AppData): Map<string, Store> {
  let all = stores.get(data);
  if (!all) {
    all = new Map();
    stores.set(data, all);
  }
  return all;
}

function storeOf(data: AppData, timeline: StreamedTimeline): Store {
  const all = storesOf(data);
  const name = nameOf(timeline);
  let store = all.get(name);
  if (!store) {
    store = { timeline, held: none, policy: undefined, listeners: new Set() };
    all.set(name, store);
  }
  return store;
}

/** The cache key of a streamed timeline. */
const queryKeyOf = (data: AppData, timeline: StreamedTimeline) =>
  timeline === "home"
    ? homeTimelineQuery(data).queryKey
    : typeof timeline === "string"
      ? publicTimelineQuery(data, timeline).queryKey
      : listTimelineQuery(data, timeline.list).queryKey;

function hold(store: Store, held: HeldArrivals): void {
  store.held = held;
  for (const listener of store.listeners) listener();
}

/** Puts `statuses` (newest first) at the top of the timeline's cached first page, leaving out those it holds. */
function prependStatuses(data: AppData, timeline: StreamedTimeline, statuses: readonly Status[]): void {
  const queryKey = queryKeyOf(data, timeline);
  // A fetch of a page writes the pages as they were when it started, over what went in since: it waits for the fetch.
  const query = data.queryClient.getQueryCache().find({ queryKey, exact: true });
  if (query?.state.fetchStatus === "fetching" && query.promise) {
    const after = () => {
      prependStatuses(data, timeline, statuses);
    };
    query.promise.then(after, after);
    return;
  }
  data.queryClient.setQueryData(queryKey, (cached) => {
    const [first, ...rest] = cached?.pages ?? [];
    if (!cached || !first) return cached;
    const loaded = new Set(cached.pages.flatMap((page) => page.items.map((s) => s.id)));
    const fresh = statuses.filter((s) => !loaded.has(s.id));
    if (fresh.length === 0) return cached;
    return { ...cached, pages: [{ ...first, items: [...fresh, ...first.items] }, ...rest] };
  });
}

/**
 * Puts `statuses` (newest first) at the top of the home timeline's cached first page, leaving out those it holds; the
 * reader's own new post goes in this way at once (compose.ts).
 */
export function prependHomeStatuses(data: AppData, statuses: readonly Status[]): void {
  prependStatuses(data, "home", statuses);
}

/**
 * Keeps the timeline's first page alone and reads it again: at once while a screen shows it, else when one next opens
 * it.
 */
const reloadFirstPage = (data: AppData, timeline: StreamedTimeline): Promise<void> =>
  readFirstPageAgain(data.queryClient, queryKeyOf(data, timeline));

/**
 * Puts in nothing that is held and reads the timeline's first page again, which brings what was held with the rest;
 * and starts the stream again where the server refused it.
 */
function refresh(data: AppData, store: Store): Promise<void> {
  hold(store, none);
  retryStream(data);
  return reloadFirstPage(data, store.timeline);
}

/** Puts what is held into the timeline: the statuses at its top, or after a break its first page read again. */
function take(data: AppData, store: Store): Promise<void> {
  const { statuses, gap } = store.held;
  if (!gap && statuses.length === 0) return Promise.resolve();
  hold(store, none);
  if (gap) return reloadFirstPage(data, store.timeline);
  prependStatuses(data, store.timeline, statuses);
  return Promise.resolve();
}

/** Asks the shown timeline's policy about what it holds, and puts it in when the policy takes it. */
function offer(data: AppData, store: Store): void {
  const { statuses, gap } = store.held;
  const arrival: Arrival = gap ? { kind: "gap" } : { kind: "statuses", count: statuses.length };
  if (store.policy?.(arrival)) void take(data, store);
}

/** Hands new statuses of a streamed timeline (newest first), from the stream, to the timeline. */
function receiveStatuses(data: AppData, timeline: StreamedTimeline, statuses: readonly Status[]): void {
  if (statuses.length === 0) return;
  const store = storeOf(data, timeline);
  if (!store.policy) {
    prependStatuses(data, timeline, statuses);
    return;
  }
  const known = new Set(statuses.map((s) => s.id));
  const held = [...statuses, ...store.held.statuses.filter((s) => !known.has(s.id))];
  hold(store, { statuses: held, gap: store.held.gap || held.length > maxHeld });
  offer(data, store);
}

/** Hands new statuses of the home timeline (newest first), from the stream, to the timeline. */
export function receiveHomeStatuses(data: AppData, statuses: readonly Status[]): void {
  receiveStatuses(data, "home", statuses);
}

/** Hands new statuses of the reader's list of `list` (newest first), from the stream's `list_update`, to its timeline. */
export function receiveListStatuses(data: AppData, list: string, statuses: readonly Status[]): void {
  receiveStatuses(data, { list }, statuses);
}

/** Hands new statuses of the public timeline of `kind` (newest first), from its stream, to the timeline. */
export function receivePublicStatuses(data: AppData, kind: PublicTimelineKind, statuses: readonly Status[]): void {
  receiveStatuses(data, kind, statuses);
}

/**
 * Takes a status that was deleted, from the stream, out of every list and view through the shadow, and out of what
 * every streamed timeline holds: the status and the boosts of it go, and a quote of it shows it deleted.
 */
export function receiveDeletion(data: AppData, id: string): void {
  receiveGone(data, id);
  for (const store of storesOf(data).values()) {
    const statuses = store.held.statuses.filter((s) => shadowed(data, s) !== undefined);
    if (statuses.length !== store.held.statuses.length) hold(store, { ...store.held, statuses });
  }
}

/** Tells a streamed timeline that statuses may have been missed. */
function receiveGap(data: AppData, timeline: StreamedTimeline): void {
  const store = storeOf(data, timeline);
  if (!store.policy) {
    void reloadFirstPage(data, timeline);
    return;
  }
  hold(store, { ...store.held, gap: true });
  offer(data, store);
}

/** Tells the home timeline that statuses may have been missed: the stream connected again. */
export function receiveHomeGap(data: AppData): void {
  receiveGap(data, "home");
}

/**
 * Tells the reader's lists that statuses may have been missed: the stream connected again. Each list whose timeline
 * the cache holds, or a screen shows, is read again as the home timeline is.
 */
export function receiveListsGap(data: AppData): void {
  const lists = new Set<string>();
  for (const query of data.queryClient.getQueryCache().getAll()) {
    const [method, path, init] = query.queryKey as readonly [
      unknown,
      unknown,
      { params?: { path?: { id?: string } } }?,
    ];
    const id = init?.params?.path?.id;
    if (method === "get" && path === "/timelines/list/{id}" && id !== undefined) lists.add(id);
  }
  for (const store of storesOf(data).values())
    if (typeof store.timeline !== "string" && store.policy) lists.add(store.timeline.list);
  for (const list of lists) receiveGap(data, { list });
}

/** Tells the public timeline of `kind` that statuses may have been missed: its stream opened, or connected again. */
export function receivePublicGap(data: AppData, kind: PublicTimelineKind): void {
  receiveGap(data, kind);
}

/** What a screen that shows a streamed timeline holds of what reached it, and how it puts that in. */
type TimelineArrivals = {
  readonly held: HeldArrivals;
  /** Puts what is held into the timeline; it resolves once a first page read again after a break has arrived. */
  readonly take: () => Promise<void>;
  /**
   * Reads the timeline's first page again, which the reader at its top asked for (a pull), holding nothing from then
   * on; it resolves once the page arrived or failed.
   */
  readonly refresh: () => Promise<void>;
};

/**
 * Shows a streamed timeline under `policy` until the function it returns is called, which puts in what is still held:
 * from then on, nothing is held.
 */
function showTimeline(data: AppData, timeline: StreamedTimeline, policy: (arrival: Arrival) => boolean): () => void {
  const store = storeOf(data, timeline);
  const mine = (arrival: Arrival) => policy(arrival);
  store.policy = mine;
  return () => {
    if (store.policy === mine) store.policy = undefined;
    void take(data, store);
  };
}

/** showTimeline of the home timeline. */
export function showHomeTimeline(data: AppData, policy: (arrival: Arrival) => boolean): () => void {
  return showTimeline(data, "home", policy);
}

/**
 * The arrivals of a streamed timeline for the screen that shows it, which `policy` decides at each one: true puts it
 * in now, false holds it. What is still held when the screen goes is put in.
 */
export function useTimelineArrivals(
  timeline: StreamedTimeline,
  policy: (arrival: Arrival) => boolean,
): TimelineArrivals {
  const data = useAppData();
  // The store stands for the timeline, which a new object naming the same list does not change.
  const store = storeOf(data, timeline);
  const decide = useEffectEvent((arrival: Arrival) => policy(arrival));
  useEffect(() => showTimeline(data, store.timeline, (arrival) => decide(arrival)), [data, store]);
  const held = useSyncExternalStore(
    (listener) => {
      store.listeners.add(listener);
      return () => {
        store.listeners.delete(listener);
      };
    },
    () => store.held,
    () => store.held,
  );
  return { held, take: () => take(data, store), refresh: () => refresh(data, store) };
}
