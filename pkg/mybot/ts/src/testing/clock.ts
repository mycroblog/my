// SPDX-License-Identifier: MIT

/** A sleeper on a Clock. */
type Waiter = { readonly until: number; readonly resolve: () => void };

/**
 * A clock that moves only when the test moves it, for the scheduled jobs (Bot.every), the send budget, and the reply
 * limits. Its now and sleep are the options of the same names of a bot, which Server.options sets. Times are
 * milliseconds since the Unix epoch.
 */
export class Clock {
  #now: number;
  #waiters: Waiter[] = [];
  #listeners = new Set<() => void>();

  /** A clock at the time at. */
  constructor(at: number) {
    this.#now = at;
  }

  /** The time of the clock: the option now of a bot. */
  readonly now = (): number => this.#now;

  /**
   * Waits until the clock has moved on by ms, or rejects with the reason of signal when it aborts first: the option
   * sleep of a bot. It resolves at once for ms <= 0.
   */
  readonly sleep = (ms: number, signal: AbortSignal): Promise<void> => {
    if (signal.aborted) return Promise.reject(signal.reason as Error);
    if (ms <= 0) return Promise.resolve();
    return new Promise<void>((resolve, reject) => {
      const onAbort = () => {
        this.#waiters = this.#waiters.filter((w) => w !== waiter);
        this.#notify();
        reject(signal.reason as Error);
      };
      const waiter: Waiter = {
        until: this.#now + ms,
        resolve: () => {
          signal.removeEventListener("abort", onAbort);
          resolve();
        },
      };
      signal.addEventListener("abort", onAbort, { once: true });
      this.#waiters.push(waiter);
      this.#notify();
    });
  };

  /** Moves the clock on by ms and wakes the sleepers whose time came. */
  advance(ms: number): void {
    this.#set(this.#now + ms);
  }

  /** Moves the clock to at, which must not be before now, and wakes the sleepers whose time came. */
  set(at: number): void {
    if (at < this.#now) throw new Error("mybot/testing: Clock.set to a time before now");
    this.#set(at);
  }

  #set(at: number): void {
    this.#now = at;
    const due = this.#waiters.filter((w) => w.until <= at);
    this.#waiters = this.#waiters.filter((w) => w.until > at);
    for (const w of due) w.resolve();
    this.#notify();
  }

  #notify(): void {
    for (const listener of this.#listeners) listener();
  }

  /** How many sleep on the clock. */
  get sleepers(): number {
    return this.#waiters.length;
  }

  /**
   * Waits until at least n sleep on the clock, such as a bot's scheduled jobs waiting for their next time, so that a
   * following advance wakes them. It rejects when signal aborts first, after 10 seconds by default.
   */
  waitForSleepers(n: number, { signal = AbortSignal.timeout(10_000) }: { signal?: AbortSignal } = {}): Promise<void> {
    return waitFor(this.#listeners, () => this.#waiters.length >= n, signal, `${String(n)} sleepers on the clock`);
  }
}

/** Resolves when cond holds, checked whenever a listener of listeners is called, or rejects when signal aborts. */
export function waitFor(
  listeners: Set<() => void>,
  cond: () => boolean,
  signal: AbortSignal,
  what: string,
): Promise<void> {
  if (cond()) return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const done = () => {
      listeners.delete(check);
      signal.removeEventListener("abort", onAbort);
    };
    const check = () => {
      if (!cond()) return;
      done();
      resolve();
    };
    const onAbort = () => {
      done();
      reject(new Error(`mybot/testing: gave up waiting for ${what}`, { cause: signal.reason }));
    };
    if (signal.aborted) {
      onAbort();
      return;
    }
    listeners.add(check);
    signal.addEventListener("abort", onAbort, { once: true });
  });
}
