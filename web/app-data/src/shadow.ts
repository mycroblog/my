// SPDX-License-Identifier: AGPL-3.0-only
// The shadow over the cached statuses (ADR-0028 §7, §18), as Bluesky's social-app keeps one: a write to a status (a
// mark, an edit, a vote, a deletion) never rewrites the copies the queries hold; it lays a change over the status's
// ID, and every hook that hands statuses to a screen shows each copy through it, in every list and view at once.
//
// A change shows on every copy while its write is in flight, and is taken off when the write fails: that is the
// rollback, with nothing to put back. Once the server took it, the change (or the one its answer gives) stays on the
// copies the cache held then, and on those of fetches begun before, which may not know of the write yet; a copy of a
// fetch begun after it shows what the server says. Changes apply in the order they were made; a change with a key
// replaces the earlier ones of that key that the server took, so the answer to an older write never shows over a newer
// one. A status gone (deleted) is gone for the session: its copies leave every list, a boost of it too, and a quote of
// it shows it deleted.
import { replaceEqualDeep, type Query, type QueryCacheNotifyEvent, type QueryClient } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { cachedCopies, copiesIn, copiesOf, findCachedCopy, type Status } from "./statuses";

/** A change of a status: the status as it shows after it. */
export type StatusChange = (status: Status) => Status;

type Layer = {
  readonly key: string | undefined;
  /** The change, or "gone" for a deletion in flight. */
  change: StatusChange | "gone";
  /** The step at which the server took the write; undefined while it is in flight. */
  settled: number | undefined;
  /** The copies a change the server took shows on. */
  readonly stale: WeakSet<Status>;
};

type Shadow = {
  /** The changes of each status's ID, in the order they were made. */
  readonly layers: Map<string, Layer[]>;
  /** The IDs of the statuses gone. */
  readonly gone: Set<string>;
  /** A counter of the writes settled and the fetches begun, to tell which came first without the wall clock. */
  step: number;
  /** The step at which the last fetch of each query began, and whether it read only one more page. */
  readonly fetches: WeakMap<Query, { readonly step: number; readonly more: boolean }>;
  /** The data each query held at its last update: what a write by hand (setQueryData) replaced. */
  readonly held: WeakMap<Query, unknown>;
  /** Changes with each change of the shadow. */
  version: number;
  /**
   * How a copy shows, a new function with each change of the shadow: what a hook subscribes to and returns, so that a
   * memo of what it shows (React Compiler's, on the function's identity) is worked out again when the shadow changes.
   */
  view: ShowStatus;
  readonly listeners: Set<() => void>;
  /** Each copy as it shows, for the version it was worked out at: a copy keeps its identity while nothing changes. */
  readonly shown: WeakMap<Status, { readonly version: number; readonly status: Status | undefined }>;
};

const shadows = new WeakMap<QueryClient, Shadow>();

function shadowOf(queryClient: QueryClient): Shadow {
  let shadow = shadows.get(queryClient);
  if (shadow === undefined) {
    const created: Shadow = {
      layers: new Map(),
      gone: new Set(),
      step: 0,
      fetches: new WeakMap(),
      held: new WeakMap(),
      version: 0,
      view: (copy) => show(created, copy),
      listeners: new Set(),
      shown: new WeakMap(),
    };
    for (const query of queryClient.getQueryCache().getAll()) created.held.set(query, query.state.data);
    queryClient.getQueryCache().subscribe((event) => {
      watch(created, event);
    });
    shadows.set(queryClient, created);
    shadow = created;
  }
  return shadow;
}

function changed(shadow: Shadow): void {
  shadow.version += 1;
  shadow.view = (copy) => show(shadow, copy);
  for (const listener of shadow.listeners) listener();
}

/**
 * Follows the fetches of the cache: a fetch begun before a write was settled may answer without it, so the change
 * stays on the copies it brings; one begun after shows what the server says, even on a copy whose data did not change.
 * A write by hand brings nothing of the server's: what the change showed on before shows it still.
 */
function watch(shadow: Shadow, event: QueryCacheNotifyEvent): void {
  if (event.type !== "updated") return;
  const { action } = event;
  // The cache types its queries' data as any; a query's data is only read through copiesIn.
  const query = event.query as Query;
  const before = shadow.held.get(query);
  shadow.held.set(query, query.state.data);
  if (action.type === "fetch") {
    shadow.step += 1;
    shadow.fetches.set(query, { step: shadow.step, more: action.meta?.fetchMore !== undefined });
    return;
  }
  if (action.type !== "success" || shadow.layers.size === 0) return;
  if (action.manual === true) {
    if (carry(shadow, query, before)) changed(shadow);
    return;
  }
  const fetch = shadow.fetches.get(query);
  let freed = false;
  for (const copy of copiesIn(query))
    for (const layer of shadow.layers.get(copy.id) ?? []) {
      if (layer.settled === undefined) continue;
      if (fetch === undefined || fetch.step < layer.settled) layer.stale.add(copy);
      // A page read after the others leaves those as they were: only a fetch of every page frees them.
      else if (!fetch.more && layer.stale.delete(copy)) freed = true;
    }
  if (freed) changed(shadow);
}

/**
 * After a write by hand to `query`, carries each change the server took from a copy of `before` it showed on to the
 * copy of the same status now held that equals it. The cache shares what did not change by position, so a status the
 * write moved (one put above it) is a new copy with the same data. True when a copy took a change.
 */
function carry(shadow: Shadow, query: Query, before: unknown): boolean {
  const old = new Map<string, Status[]>();
  for (const copy of copiesOf(query, before))
    if (shadow.layers.has(copy.id)) old.set(copy.id, [...(old.get(copy.id) ?? []), copy]);
  if (old.size === 0) return false;
  let carried = false;
  for (const copy of copiesIn(query)) {
    const was = old.get(copy.id);
    if (was === undefined) continue;
    for (const layer of shadow.layers.get(copy.id) ?? []) {
      if (layer.settled === undefined || layer.stale.has(copy)) continue;
      if (was.some((o) => layer.stale.has(o) && replaceEqualDeep(o, copy) === o)) {
        layer.stale.add(copy);
        carried = true;
      }
    }
  }
  return carried;
}

/** The copy as it shows through the shadow; undefined for a status gone, or a boost of one. */
function show(shadow: Shadow, copy: Status): Status | undefined {
  if (shadow.layers.size === 0 && shadow.gone.size === 0) return copy;
  const memo = shadow.shown.get(copy);
  if (memo?.version === shadow.version) return memo.status;
  const status = work(shadow, copy);
  shadow.shown.set(copy, { version: shadow.version, status });
  return status;
}

function work(shadow: Shadow, copy: Status): Status | undefined {
  if (shadow.gone.has(copy.id)) return undefined;
  let next = copy;
  for (const layer of shadow.layers.get(copy.id) ?? []) {
    if (layer.change === "gone") return undefined;
    if (layer.settled === undefined || layer.stale.has(copy)) next = layer.change(next);
  }
  if (next.boost !== undefined) {
    const boost = show(shadow, next.boost);
    if (boost === undefined) return undefined;
    if (boost !== next.boost) next = { ...next, boost };
  }
  const quoted = next.quote?.quoted_status;
  if (next.quote !== undefined && quoted !== undefined) {
    const shown = show(shadow, quoted);
    if (shown === undefined) next = { ...next, quote: { state: "deleted" } };
    else if (shown !== quoted) next = { ...next, quote: { ...next.quote, quoted_status: shown } };
  }
  return next;
}

// ---- Writing -------------------------------------------------------------------------------------------------------

/** A change laid over a status while its write is in flight, until the write settles. */
export type StatusShadow = {
  /**
   * The server took the write: the change stays on the copies cached now and on those of the fetches begun before.
   * `change`, when given, replaces it (the server's answer, written over the fields the write changes).
   */
  readonly confirm: (change?: StatusChange) => void;
  /** The write failed: the change is taken off, and every copy shows again what it showed before. */
  readonly discard: () => void;
};

function lay(queryClient: QueryClient, id: string, change: StatusChange | "gone", key?: string): StatusShadow {
  const shadow = shadowOf(queryClient);
  const layer: Layer = { key, change, settled: undefined, stale: new WeakSet() };
  shadow.layers.set(id, [...(shadow.layers.get(id) ?? []), layer]);
  changed(shadow);
  let done = false;
  /** Keeps the changes of the status that `keep` keeps. */
  const keepOnly = (keep: (l: Layer, i: number, all: readonly Layer[]) => boolean) => {
    const layers = (shadow.layers.get(id) ?? []).filter(keep);
    if (layers.length === 0) shadow.layers.delete(id);
    else shadow.layers.set(id, layers);
  };
  return {
    confirm: (answered) => {
      if (done) return;
      done = true;
      if (layer.change === "gone") gone(queryClient, shadow, id);
      else {
        if (answered !== undefined) layer.change = answered;
        shadow.step += 1;
        layer.settled = shadow.step;
        for (const copy of cachedCopies(queryClient)) if (copy.id === id) layer.stale.add(copy);
        // The earlier changes of the key that the server took show nothing this one does not replace.
        keepOnly(
          (l, i, all) => key === undefined || l.key !== key || l.settled === undefined || i >= all.indexOf(layer),
        );
      }
      changed(shadow);
    },
    discard: () => {
      if (done) return;
      done = true;
      keepOnly((l) => l !== layer);
      changed(shadow);
    },
  };
}

/** Marks the status of `id` gone, and reads the status alone again, which answers that it is gone. */
function gone(queryClient: QueryClient, shadow: Shadow, id: string): void {
  shadow.gone.add(id);
  shadow.layers.delete(id);
  for (const query of queryClient.getQueryCache().getAll()) {
    const [method, path] = query.queryKey;
    if (method === "get" && path === "/statuses/{id}" && (query.state.data as Status | undefined)?.id === id)
      void queryClient.invalidateQueries({ queryKey: query.queryKey, exact: true });
  }
}

/**
 * Lays `change` over every copy of the status of `id` at once, while its write is in flight; the caller confirms or
 * discards it when the write settles. Changes with the same `key` (the fields a kind of write changes) leave the copies
 * to the latest one.
 */
export function shadowStatus(data: AppData, id: string, change: StatusChange, key?: string): StatusShadow {
  return lay(data.queryClient, id, change, key);
}

/** Takes the status of `id` out of every list and view at once, while its deletion is in flight. */
export function shadowGone(data: AppData, id: string): StatusShadow {
  return lay(data.queryClient, id, "gone");
}

/**
 * Writes a change of the status of `id` through the shadow: `change` shows at once on every copy, then, once `send`
 * resolves, the change `answered` gives of its answer (else `change` itself); if `send` rejects, the copies show again
 * what they showed before, and this rejects with its failure.
 */
export async function changeStatus<T>(
  data: AppData,
  id: string,
  write: {
    readonly change: StatusChange;
    readonly send: () => Promise<T>;
    readonly answered?: (answer: T) => StatusChange | undefined;
    readonly key?: string;
  },
): Promise<T> {
  const shadow = shadowStatus(data, id, write.change, write.key);
  try {
    const answer = await write.send();
    shadow.confirm(write.answered?.(answer));
    return answer;
  } catch (e) {
    shadow.discard();
    throw e;
  }
}

/**
 * Deletes the status of `id` through the shadow: it leaves every list and view at once, and comes back if `send`
 * rejects, with its failure.
 */
export async function removeStatus<T>(data: AppData, id: string, send: () => Promise<T>): Promise<T> {
  const shadow = shadowGone(data, id);
  try {
    const answer = await send();
    shadow.confirm();
    return answer;
  } catch (e) {
    shadow.discard();
    throw e;
  }
}

/** The status of `id` is gone, as the server said (the stream): every copy leaves at once. */
export function receiveGone(data: AppData, id: string): void {
  shadowGone(data, id).confirm();
}

// ---- Reading -------------------------------------------------------------------------------------------------------

/** How a copy of a status shows: through the shadow, or undefined when it is gone (or boosts one gone). */
export type ShowStatus = (copy: Status) => Status | undefined;

/** How a copy of a status shows now, outside React. */
export function shadowed(data: AppData, copy: Status): Status | undefined {
  return show(shadowOf(data.queryClient), copy);
}

/** The statuses of `copies` as they show, those gone left out; `copies` itself when none changes. */
export function showList(show: ShowStatus, copies: readonly Status[]): readonly Status[] {
  const shown = copies.flatMap((copy) => {
    const status = show(copy);
    return status === undefined ? [] : [status];
  });
  return shown.length === copies.length && shown.every((s, i) => s === copies[i]) ? copies : shown;
}

/** The status of `id` as it shows, from a copy some cached query holds; undefined when none does, or it is gone. */
export function cachedStatus(data: AppData, id: string): Status | undefined {
  const copy = findCachedCopy(data.queryClient, id);
  return copy === undefined ? undefined : shadowed(data, copy);
}

/**
 * How a copy of a status shows through the shadow of the nearest AppDataProvider; the component renders again when
 * the shadow changes, and the function returned is a new one then.
 */
export function useShadow(): ShowStatus {
  const { queryClient } = useAppData();
  const shadow = shadowOf(queryClient);
  return useSyncExternalStore(
    (listener) => {
      shadow.listeners.add(listener);
      return () => {
        shadow.listeners.delete(listener);
      };
    },
    () => shadow.view,
    () => shadow.view,
  );
}
