// SPDX-License-Identifier: MIT
import type { components } from "@mycroblog/mykit";

type Notification = components["schemas"]["Notification"];
type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];

/** One event to handle: a notification, or a post of the home timeline. */
export type Job = { notification: Notification; status?: undefined; sender: string } | { notification?: undefined; status: Status; sender: string };

/** The key of the account that acted, "" for an event without one. */
export function senderOf(account: Account | undefined): string {
  return account?.id ?? "";
}

/**
 * Runs the jobs: up to its limit at once, those of one sender one after the other in the order they came (ADR-0094
 * §4). A notification that is queued or running is not queued again, so that the catch-up and the event stream may
 * both bring it. run must not reject.
 */
export class Dispatcher {
  readonly #run: (job: Job) => Promise<void>;
  readonly #limit: number;
  #active = 0;
  readonly #waiting: (() => void)[] = [];
  readonly #queues = new Map<string, Job[]>();
  readonly #queued = new Set<string>();
  readonly #drains = new Set<Promise<void>>();
  #stopping = false;

  constructor(concurrency: number, run: (job: Job) => Promise<void>) {
    this.#limit = Math.max(1, concurrency);
    this.#run = run;
  }

  /** Queues job and reports whether it did: not after stop, and not for a notification already queued. */
  submit(job: Job): boolean {
    if (this.#stopping) return false;
    if (job.notification !== undefined) {
      if (this.#queued.has(job.notification.id)) return false;
      this.#queued.add(job.notification.id);
    }
    const queue = this.#queues.get(job.sender);
    if (queue !== undefined) {
      queue.push(job);
      return true;
    }
    this.#queues.set(job.sender, [job]);
    const drain = this.#drain(job.sender).finally(() => this.#drains.delete(drain));
    this.#drains.add(drain);
    return true;
  }

  /** Runs the queue of sender until it is empty, or the dispatcher stops. */
  async #drain(sender: string): Promise<void> {
    for (;;) {
      const queue = this.#queues.get(sender);
      const job = queue?.[0];
      if (this.#stopping || queue === undefined || job === undefined) {
        // The jobs that never started are left to the next catch-up.
        this.#forgetQueue(sender);
        return;
      }
      await this.#acquire();
      // stop may have been called while the job waited for its turn.
      const stopping = this.#isStopping();
      try {
        if (!stopping) await this.#run(job);
      } finally {
        this.#release();
      }
      if (job.notification !== undefined) this.#queued.delete(job.notification.id);
      if (queue.length <= 1 || stopping) {
        this.#forgetQueue(sender);
        return;
      }
      queue.shift();
    }
  }

  #forgetQueue(sender: string): void {
    for (const job of this.#queues.get(sender) ?? []) if (job.notification !== undefined) this.#queued.delete(job.notification.id);
    this.#queues.delete(sender);
  }

  async #acquire(): Promise<void> {
    if (this.#active < this.#limit) {
      this.#active++;
      return;
    }
    await new Promise<void>((resolve) => this.#waiting.push(resolve));
  }

  #release(): void {
    const next = this.#waiting.shift();
    if (next === undefined) this.#active--;
    else next();
  }

  #isStopping(): boolean {
    return this.#stopping;
  }

  /** Makes the dispatcher take no more jobs and start none of those queued. */
  stop(): void {
    this.#stopping = true;
  }

  /** Resolves when every running job has returned. */
  async idle(): Promise<void> {
    while (this.#drains.size > 0) await Promise.all(this.#drains);
  }
}

/**
 * Computes the position of the catch-up (ADR-0094 §3): the newest notification up to which every notification handed
 * to the bot was processed. Handlers end out of order, so a processed notification moves the position only when no
 * older one is still pending; while a catch-up hands out notifications, the position holds still, so that a restart
 * in the middle of it reads them again. Notification IDs are UUIDv7 strings, whose order is the order of time.
 */
export class Tracker {
  readonly #pending = new Set<string>();
  /** The processed notifications above the position, in order. */
  #done: string[] = [];
  #holds = 0;

  /** Marks the notification id pending. */
  add(id: string): void {
    this.#pending.add(id);
  }

  /** Drops a pending notification that was not handed out after all. */
  forget(id: string): void {
    this.#pending.delete(id);
  }

  /** Marks the notification id processed and returns the new position, null when it does not move. */
  complete(id: string): string | null {
    this.#pending.delete(id);
    const i = lowerBound(this.#done, id);
    if (this.#done[i] !== id) this.#done.splice(i, 0, id);
    return this.#advance();
  }

  /** Keeps the position where it is until release. */
  hold(): void {
    this.#holds++;
  }

  /** Ends a hold and returns the new position, null when it does not move. */
  release(): string | null {
    this.#holds--;
    return this.#advance();
  }

  /** Moves the position past the processed notifications older than every pending one. */
  #advance(): string | null {
    if (this.#holds > 0 || this.#done.length === 0) return null;
    let oldest: string | null = null;
    for (const id of this.#pending) if (oldest === null || id < oldest) oldest = id;
    const n = oldest === null ? this.#done.length : lowerBound(this.#done, oldest);
    if (n === 0) return null;
    const position = this.#done[n - 1] ?? null;
    this.#done = this.#done.slice(n);
    return position;
  }
}

/** The index of the first element of the sorted xs that is not less than x. */
function lowerBound(xs: readonly string[], x: string): number {
  let lo = 0;
  let hi = xs.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if ((xs[mid] ?? "") < x) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}
