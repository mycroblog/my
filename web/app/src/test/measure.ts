// SPDX-License-Identifier: AGPL-3.0-only

// What the app's measurement test reads in place of the browser's: a performance timeline the test fills, a router
// whose screens it changes by hand, and a call whose quality it gives. @my/platform's and @my/rtc's tests have fakes
// of the same contracts of their own, which the app cannot import.

import type { RoomKind, RoomSession } from "@my/rtc";
import type { TimedEntry } from "@my/platform/measure";

/**
 * A performance timeline the test fills: `give` delivers entries of a type to the observers of it. As a browser does,
 * it gives an observer of `event` only the entries as long as its `durationThreshold` (104 ms when it names none).
 */
export function fakeTimeline() {
  const observers: FakeObserver[] = [];
  class FakeObserver {
    static readonly supportedEntryTypes = ["largest-contentful-paint", "event", "long-animation-frame"];
    readonly types: string[] = [];
    eventThreshold = 104;
    disconnected = false;
    readonly callback: (list: { getEntries(): readonly TimedEntry[] }) => void;
    constructor(callback: (list: { getEntries(): readonly TimedEntry[] }) => void) {
      this.callback = callback;
      observers.push(this);
    }
    observe({ type, durationThreshold }: { type: string; durationThreshold?: number }): void {
      this.types.push(type);
      if (type === "event" && durationThreshold !== undefined) this.eventThreshold = durationThreshold;
    }
    takeRecords(): readonly TimedEntry[] {
      return [];
    }
    disconnect(): void {
      this.disconnected = true;
    }
  }
  return {
    Observer: FakeObserver,
    give: (type: string, entries: readonly TimedEntry[]) => {
      for (const o of observers) {
        if (o.disconnected || !o.types.includes(type)) continue;
        const kept = type === "event" ? entries.filter((e) => e.duration >= o.eventThreshold) : [...entries];
        if (kept.length > 0) o.callback({ getEntries: () => kept });
      }
    },
  };
}

type Listener = (change: {
  fromLocation?: { pathname: string; state: { __TSR_index: number } };
  toLocation: { pathname: string; state: { __TSR_index: number } };
  pathChanged: boolean;
}) => void;

/** A router on the screen of the route `routeId` at `pathname`; `go` moves it to another, as a navigation does. */
export function fakeRouter(routeId: string, pathname: string) {
  const listeners = { onBeforeNavigate: new Set<Listener>(), onRendered: new Set<Listener>() };
  let at = { pathname, index: 0, routeId };
  const router = {
    state: { matches: [{ routeId: "__root__" }, { routeId: "/_app" }, { routeId }] },
    subscribe: (event: keyof typeof listeners, fn: Listener) => {
      listeners[event].add(fn);
      return () => {
        listeners[event].delete(fn);
      };
    },
  };
  const go = (to: { routeId: string; pathname: string }) => {
    const from = at;
    at = { ...to, index: from.index + 1 };
    const change = {
      fromLocation: { pathname: from.pathname, state: { __TSR_index: from.index } },
      toLocation: { pathname: at.pathname, state: { __TSR_index: at.index } },
      pathChanged: true,
    };
    for (const fn of listeners.onBeforeNavigate) fn(change);
    router.state = { matches: [{ routeId: "__root__" }, { routeId: "/_app" }, { routeId: to.routeId }] };
    for (const fn of listeners.onRendered) fn(change);
  };
  return { router, go };
}

type Quality = { rtt: number | null; jitter: number | null; packetLoss: number | null };

/** A call of `kind` made by hand, whose quality the test gives with `say`. */
export function handMadeCall(kind: RoomKind): { session: RoomSession; say: (quality: Quality) => void } {
  const heard = new Set<(quality: Quality) => void>();
  const session: RoomSession = {
    kind,
    session: "s1",
    capabilities: { outputSelection: false },
    update: () => undefined,
    setMicrophone: () => Promise.resolve(),
    setOutputDevice: () => Promise.resolve(),
    renew: () => undefined,
    reconnect: () => undefined,
    receiveSignal: () => undefined,
    resumePlayback: () => Promise.resolve(),
    snapshot: () => ({
      connection: { state: "connected", participants: [] },
      talking: new Set(),
      playbackBlocked: false,
      microphone: false,
      quality: null,
      failure: null,
    }),
    on: (event, listener) => {
      if (event !== "quality") return () => undefined;
      const one = listener as (quality: Quality) => void;
      heard.add(one);
      return () => heard.delete(one);
    },
    close: () => undefined,
  };
  return {
    session,
    say: (quality) => {
      for (const listener of heard) listener(quality);
    },
  };
}
