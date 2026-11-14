// SPDX-License-Identifier: AGPL-3.0-only
// The statuses that reach the home timeline after its first page (ADR-0013 §3–§8, ADR-0028 §7): what the stream
// (FR-207) receives, which it hands to receiveHomeStatuses, and the break it reports when it connects again
// (receiveHomeGap). While no screen shows the home timeline, a status goes at once to the top of its cached first page,
// and a break marks it to be read again from its first page. While one shows it, the screen's policy decides at each
// arrival whether it goes in now; what it does not take is held here, with a break when the held statuses no longer
// join those loaded (after a reconnection, or past 200), until the policy takes them: inserted at the top, or, after a
// break, the first page read again in place of every page loaded.
import { useEffect, useEffectEvent, useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import type { Status } from "./statuses";
import { homeTimelineQuery } from "./timeline";

/** What reached the shown timeline: statuses, as many as would go in now, or a break. */
export type Arrival = { readonly kind: "statuses"; readonly count: number } | { readonly kind: "gap" };

/** What is held for the shown timeline: the statuses, newest first, and whether they no longer join those loaded. */
type HeldArrivals = { readonly statuses: readonly Status[]; readonly gap: boolean };

/** How many held statuses still join those loaded; past it, inserting them could leave posts out between. */
export const maxHeld = 200;

type Store = {
  held: HeldArrivals;
  /** The policy of the screen that shows the timeline: whether what reached it goes in now. */
  policy: ((arrival: Arrival) => boolean) | undefined;
  readonly listeners: Set<() => void>;
};

const none: HeldArrivals = { statuses: [], gap: false };
const stores = new WeakMap<AppData, Store>();

function storeOf(data: AppData): Store {
  let store = stores.get(data);
  if (!store) {
    store = { held: none, policy: undefined, listeners: new Set() };
    stores.set(data, store);
  }
  return store;
}

function hold(store: Store, held: HeldArrivals): void {
  store.held = held;
  for (const listener of store.listeners) listener();
}

/** Puts `statuses` (newest first) at the top of the home timeline's cached first page, leaving out those it holds. */
function prependHomeStatuses(data: AppData, statuses: readonly Status[]): void {
  const { queryKey } = homeTimelineQuery(data);
  // A fetch of a page writes the pages as they were when it started, over what went in since: it waits for the fetch.
  const query = data.queryClient.getQueryCache().find({ queryKey, exact: true });
  if (query?.state.fetchStatus === "fetching" && query.promise) {
    const after = () => {
      prependHomeStatuses(data, statuses);
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
 * Keeps the home timeline's first page alone and reads it again: at once while a screen shows it, else when one next
 * opens it.
 */
async function reloadHomeFirstPage(data: AppData): Promise<void> {
  const { queryKey } = homeTimelineQuery(data);
  data.queryClient.setQueryData(
    queryKey,
    (cached) => cached && { pages: cached.pages.slice(0, 1), pageParams: cached.pageParams.slice(0, 1) },
  );
  await data.queryClient.invalidateQueries({ queryKey, exact: true });
}

/** Puts what is held into the timeline: the statuses at its top, or after a break its first page read again. */
function take(data: AppData, store: Store): Promise<void> {
  const { statuses, gap } = store.held;
  if (!gap && statuses.length === 0) return Promise.resolve();
  hold(store, none);
  if (gap) return reloadHomeFirstPage(data);
  prependHomeStatuses(data, statuses);
  return Promise.resolve();
}

/** Asks the shown timeline's policy about what it holds, and puts it in when the policy takes it. */
function offer(data: AppData, store: Store): void {
  const { statuses, gap } = store.held;
  const arrival: Arrival = gap ? { kind: "gap" } : { kind: "statuses", count: statuses.length };
  if (store.policy?.(arrival)) void take(data, store);
}

/** Hands new statuses of the home timeline (newest first), from the stream, to the timeline. */
export function receiveHomeStatuses(data: AppData, statuses: readonly Status[]): void {
  if (statuses.length === 0) return;
  const store = storeOf(data);
  if (!store.policy) {
    prependHomeStatuses(data, statuses);
    return;
  }
  const known = new Set(statuses.map((s) => s.id));
  const held = [...statuses, ...store.held.statuses.filter((s) => !known.has(s.id))];
  hold(store, { statuses: held, gap: store.held.gap || held.length > maxHeld });
  offer(data, store);
}

/** Tells the home timeline that statuses may have been missed: the stream connected again. */
export function receiveHomeGap(data: AppData): void {
  const store = storeOf(data);
  if (!store.policy) {
    void reloadHomeFirstPage(data);
    return;
  }
  hold(store, { ...store.held, gap: true });
  offer(data, store);
}

/** What a screen that shows the home timeline holds of what reached it, and how it puts that in. */
type HomeArrivals = {
  readonly held: HeldArrivals;
  /** Puts what is held into the timeline; it resolves once a first page read again after a break has arrived. */
  readonly take: () => Promise<void>;
};

/**
 * Shows the home timeline under `policy` until the function it returns is called, which puts in what is still held:
 * from then on, nothing is held.
 */
export function showHomeTimeline(data: AppData, policy: (arrival: Arrival) => boolean): () => void {
  const store = storeOf(data);
  const mine = (arrival: Arrival) => policy(arrival);
  store.policy = mine;
  return () => {
    if (store.policy === mine) store.policy = undefined;
    void take(data, store);
  };
}

/**
 * The arrivals of the home timeline for the screen that shows it, which `policy` decides at each one: true puts it
 * in now, false holds it. What is still held when the screen goes is put in.
 */
export function useHomeArrivals(policy: (arrival: Arrival) => boolean): HomeArrivals {
  const data = useAppData();
  const store = storeOf(data);
  const decide = useEffectEvent((arrival: Arrival) => policy(arrival));
  useEffect(() => showHomeTimeline(data, (arrival) => decide(arrival)), [data]);
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
  return { held, take: () => take(data, store) };
}
