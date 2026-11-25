// SPDX-License-Identifier: AGPL-3.0-only
// A browser of several tabs and a fake stream for the tests of the stream. The tabs' group grants the lead as Web
// Locks grant an exclusive lock, one tab at a time in the order asked, and delivers a message as a BroadcastChannel
// does, to the other tabs that listen. The fake stream stands in for mykit's runStream: the test opens it and sends its
// events by hand, breaks it or has the server refuse it, and it ends when its signal aborts.
import type { StreamEvent, StreamOptions } from "@mycroblog/mykit";
import type { TabGroup } from "@my/platform";

type Waiter = { readonly start: () => void };

/** A browser whose tabs join one group with `join`. */
export function fakeBrowser(): { join: () => TabGroup } {
  const listeners = new Set<(message: unknown) => void>();
  const waiting: Waiter[] = [];
  let held = false;
  const join = (): TabGroup => {
    const mine = new Set<(message: unknown) => void>();
    let waited = false;
    return {
      lead: (signal, lead) =>
        new Promise<void>((resolve, reject) => {
          const waiter: Waiter = {
            start: () => {
              signal.removeEventListener("abort", abort);
              held = true;
              lead(waited)
                .then(resolve, reject)
                .finally(() => {
                  held = false;
                  waiting.shift()?.start();
                });
            },
          };
          const abort = () => {
            waiting.splice(waiting.indexOf(waiter), 1);
            resolve();
          };
          if (!held) {
            waiter.start();
            return;
          }
          waited = true;
          signal.addEventListener("abort", abort, { once: true });
          waiting.push(waiter);
        }),
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

/** One stream that the fake runStream opened: the options it was given, and how the test drives it. */
export type FakeStream = {
  readonly options: StreamOptions;
  /** Opens it, as the first connection (false) or a later one (true). */
  open: (reconnected: boolean) => void;
  send: (event: StreamEvent) => void;
  /**
   * Breaks it, as runStream does after a failure it recovers from or a stream the server ended: it reports a failure
   * when one is given, and waits with the options' `sleep` before the next connection, which the test opens.
   */
  drop: (error?: unknown) => void;
  /** Ends it as runStream does when the server refuses the stream for good: it rejects with `error`. */
  refuse: (error?: Error) => void;
  /** Whether its signal aborted, which ends it. */
  readonly ended: () => boolean;
};

/** A fake runStream and the streams it ran, in order. */
export function fakeStreams(): { run: (options: StreamOptions) => Promise<void>; streams: FakeStream[] } {
  const streams: FakeStream[] = [];
  const run = (options: StreamOptions) =>
    new Promise<void>((resolve, reject) => {
      streams.push({
        options,
        open: (reconnected) => options.onOpen?.(reconnected),
        send: (event) => {
          options.onEvent(event);
        },
        drop: (error) => {
          if (error !== undefined) options.onError?.(error);
          void options.sleep?.(0, options.signal).catch(() => undefined);
        },
        refuse: (error = new Error("test: the server refused the stream")) => {
          reject(error);
        },
        ended: () => options.signal.aborted,
      });
      options.signal.addEventListener("abort", () => {
        resolve();
      });
    });
  return { run, streams };
}
