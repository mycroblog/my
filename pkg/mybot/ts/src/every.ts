// SPDX-License-Identifier: MIT
import type { components } from "@mycroblog/mykit";
import type { Handler } from "./context.js";
import type { Schedule } from "./schedule.js";

type Visibility = components["schemas"]["Visibility"];

/**
 * What a scheduled job does about the times it missed while the bot was stopped: "skip", the default, leaves them out,
 * and the job runs from its next time after the start; "latest" runs, when the bot starts, the newest of the times
 * after the job's last run in the store, once. The older ones are left out, so that a long stop never posts a pile at
 * once. A job that never ran runs its newest past time.
 */
export type CatchUp = "skip" | "latest";

/** The visibilities of the posts of a scheduled job. */
export type EveryVisibility = Extract<Visibility, "unlisted" | "public" | "local">;

/** The options of a scheduled job (Bot.every). */
export type EveryOptions = {
  /**
   * Names the job in the store (its last run) and in the Idempotency-Key of its posts: keep it when the expression
   * changes. Absent or "" means the expression. Two jobs of one bot cannot share a name.
   */
  name?: string;
  /** The IANA time zone of the expression, such as "Asia/Tokyo"; absent or "" means UTC. */
  timeZone?: string;
  /** "skip" (the default) or "latest". */
  catchUp?: CatchUp;
  /**
   * The visibility of the posts of Context.post: "unlisted" (the default), "public", or "local", which reaches the
   * server of the bot and its followers alone (ADR-0017).
   */
  visibility?: EveryVisibility;
};

/** A job that Bot.every registered. */
export type EveryJob = {
  readonly name: string;
  readonly schedule: Schedule;
  readonly handler: Handler;
  readonly catchUp: CatchUp;
  readonly visibility: EveryVisibility;
};

/**
 * The Idempotency-Key of the post of the run at (milliseconds since the Unix epoch) of the scheduled job name
 * (ADR-0094 §5): mybot:every:<name>:<the time in RFC 3339, UTC>. Two processes of one bot that run the same time post
 * once. Use it when posting with mykit directly.
 */
export function everyIdempotencyKey(name: string, at: number): string {
  return `mybot:every:${name}:${new Date(at).toISOString().replace(/\.\d+Z$/, "Z")}`;
}

/** The error of Context.post outside a scheduled job. */
export class NotScheduledError extends Error {
  override readonly name = "NotScheduledError";

  constructor() {
    super("mybot: post is for the handler of a scheduled job");
  }
}

/** The error of a second Context.post in one run of a scheduled job, whose Idempotency-Key would answer with the first post. */
export class PostedAlreadyError extends Error {
  override readonly name = "PostedAlreadyError";

  constructor() {
    super("mybot: the run of the scheduled job posted already");
  }
}
