// SPDX-License-Identifier: AGPL-3.0-only

// A browser of several tabs for the app's tests: its tabs join one group, which grants the lead to one tab at a time
// in the order asked, as Web Locks grant an exclusive lock, and delivers a message to the other tabs that listen, as
// a BroadcastChannel does. The app's data has the same browser for its own tests (web/app-data/src/test/tabs.ts).

import type { TabGroup } from "@my/platform";

/** A browser whose tabs join one group with `join`. */
export function fakeBrowser(): { join: () => TabGroup } {
  const listeners = new Set<(message: unknown) => void>();
  let led: Promise<void> = Promise.resolve();
  const join = (): TabGroup => {
    const mine = new Set<(message: unknown) => void>();
    return {
      lead: (signal, lead) => {
        const turn = led.then(() => (signal.aborted ? undefined : lead(false)));
        // The next tab leads once this one is done, however it ended.
        led = turn.then(
          () => undefined,
          () => undefined,
        );
        return turn;
      },
      post: (message) => {
        const copy = structuredClone(message);
        for (const listener of listeners) if (!mine.has(listener)) listener(copy);
      },
      listen: (listener) => {
        mine.add(listener);
        listeners.add(listener);
        return () => {
          mine.delete(listener);
          listeners.delete(listener);
        };
      },
      close: () => {
        for (const listener of mine) listeners.delete(listener);
        mine.clear();
      },
    };
  };
  return { join };
}
