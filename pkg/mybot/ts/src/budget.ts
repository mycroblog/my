// SPDX-License-Identifier: MIT

/** A number of events in a window of time, in milliseconds, such as 60 posts an hour. */
export type Rate = {
  readonly n: number;
  readonly per: number;
};

const minute = 60_000;
const hour = 60 * minute;

/** The budget of posts, replies included: 60 an hour, six tenths of the server's 300 in three hours (ADR-0094 §6). */
export const defaultPostRate: Rate = { n: 60, per: hour };
/** The budget of the other writes (reactions, favourites, follows): 60 a minute. */
export const defaultWriteRate: Rate = { n: 60, per: minute };
/** The most replies to one sender: 5 in 10 minutes and 15 in an hour (ADR-0094 §8). */
export const defaultReplyLimits: readonly Rate[] = [
  { n: 5, per: 10 * minute },
  { n: 15, per: hour },
];
/** The longest a write waits for the send budget before it is dropped: 10 minutes. */
export const defaultMaxSendWait = 10 * minute;

/** The budget a write draws on. */
export type SendKind = "post" | "write";

/** A token bucket that holds up to rate.n tokens and gains rate.n every rate.per. */
class Bucket {
  readonly #rate: Rate;
  #tokens: number;
  #last: number | null = null;

  constructor(rate: Rate) {
    this.#rate = rate;
    this.#tokens = rate.n;
  }

  /** When the bucket was last refilled, null before the first wait. */
  get last(): number | null {
    return this.#last;
  }

  /** Refills the bucket at now and returns how long until it holds one token. */
  wait(now: number): number {
    const { n, per } = this.#rate;
    if (n <= 0 || per <= 0) return 0;
    if (this.#last !== null) this.#tokens = Math.min(n, this.#tokens + ((now - this.#last) * n) / per);
    this.#last = now;
    if (this.#tokens >= 1) return 0;
    return ((1 - this.#tokens) * per) / n;
  }

  /** Uses one token, which wait made sure of or reserved. */
  take(): void {
    if (this.#rate.n > 0 && this.#rate.per > 0) this.#tokens--;
  }
}

/**
 * The client-side send budget of a bot: a token bucket for posts and one for the other writes, and the pause that the
 * server's RateLimit fields ask for when less than a tenth of the limit remains.
 */
export class Budget {
  readonly #buckets: Record<SendKind, Bucket>;
  #pause = 0;

  constructor(post: Rate, write: Rate) {
    this.#buckets = { post: new Bucket(post), write: new Bucket(write) };
  }

  /**
   * Reserves a send of kind at now and returns how long to wait before sending it; null, reserving nothing, when the
   * wait would be longer than maxWait. During a pause of the server the bucket counts from the end of the pause, so
   * that the writes waiting for it go out spaced by the budget, not all at its end.
   */
  reserve(kind: SendKind, now: number, maxWait: number): number | null {
    const bucket = this.#buckets[kind];
    let from = Math.max(now, this.#pause);
    // Never count back from the bucket's last refill, which would drain it.
    if (bucket.last !== null && bucket.last > from) from = bucket.last;
    const wait = from - now + bucket.wait(from);
    if (wait > maxWait) return null;
    bucket.take();
    return wait;
  }

  /**
   * Reads the RateLimit and RateLimit-Policy fields of an answer (draft-ietf-httpapi-ratelimit-headers,
   * `"name";r=<remaining>;t=<seconds>` and `"name";q=<quota>;w=<window>`): when less than a tenth of a policy's quota
   * remains, the writes wait until its window resets.
   */
  observe(headers: Headers, now: number): void {
    const limits = headers.get("RateLimit");
    if (limits === null) return;
    const quotas = new Map<string, number>();
    for (const item of structuredItems(headers.get("RateLimit-Policy") ?? "")) {
      const q = item.params.get("q");
      if (q !== undefined) quotas.set(item.name, q);
    }
    for (const item of structuredItems(limits)) {
      const r = item.params.get("r");
      const t = item.params.get("t");
      const q = quotas.get(item.name);
      if (r === undefined || t === undefined || q === undefined || q <= 0 || r * 10 >= q) continue;
      this.#pause = Math.max(this.#pause, now + t * 1000);
    }
  }
}

/** A member of a structured field list: a string or token and its integer parameters. */
type StructuredItem = { name: string; params: Map<string, number> };

/** Reads the members of a list field such as `"api";r=12;t=30, "posts";r=1;t=600`, skipping the parameters that are not integers. */
function structuredItems(value: string): StructuredItem[] {
  if (value.trim() === "") return [];
  return value.split(",").map((member) => {
    const [name = "", ...params] = member.split(";");
    const item: StructuredItem = { name: name.trim().replaceAll(/^"|"$/g, ""), params: new Map() };
    for (const p of params) {
      const eq = p.indexOf("=");
      if (eq < 0) continue;
      const v = p.slice(eq + 1).trim();
      if (/^-?\d+$/.test(v)) item.params.set(p.slice(0, eq).trim(), Number(v));
    }
    return item;
  });
}

/** Counts the replies to each sender, for the reply limits of ADR-0094 §8. */
export class ReplyLimiter {
  readonly #limits: readonly Rate[];
  readonly #sent = new Map<string, number[]>();

  constructor(limits: readonly Rate[]) {
    this.#limits = limits;
  }

  /** Whether one more reply to sender at now would go over a limit. */
  full(sender: string, now: number): boolean {
    const longest = Math.max(0, ...this.#limits.map((r) => r.per));
    // Drop what no window reaches any more.
    const keep = (this.#sent.get(sender) ?? []).filter((at) => now - at < longest);
    if (keep.length === 0) this.#sent.delete(sender);
    else this.#sent.set(sender, keep);
    return this.#limits.some((r) => r.n > 0 && r.per > 0 && keep.filter((at) => now - at < r.per).length >= r.n);
  }

  /** Records a reply to sender at now; false, recording nothing, when it would go over a limit. */
  take(sender: string, now: number): boolean {
    if (this.full(sender, now)) return false;
    this.#sent.set(sender, [...(this.#sent.get(sender) ?? []), now]);
    return true;
  }

  /** Forgets the reply to sender recorded at at, which was not sent. */
  untake(sender: string, at: number): void {
    const times = this.#sent.get(sender);
    if (times === undefined) return;
    const i = times.lastIndexOf(at);
    if (i >= 0) times.splice(i, 1);
  }
}
