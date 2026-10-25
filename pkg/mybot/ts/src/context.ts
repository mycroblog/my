// SPDX-License-Identifier: MIT
import type { Api, components, Sleep } from "@mycroblog/mykit";
import type { Budget, Rate, ReplyLimiter, SendKind } from "./budget.js";
import type { Job } from "./dispatch.js";
import { type EveryJob, everyIdempotencyKey, NotScheduledError, PostedAlreadyError } from "./every.js";
import { type Logger, withAttributes } from "./logger.js";
import { plainText } from "./text.js";

type Notification = components["schemas"]["Notification"];
type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];
type CredentialAccount = components["schemas"]["CredentialAccount"];
type Visibility = components["schemas"]["Visibility"];

/**
 * The Idempotency-Key of the n-th reply (from 1) that a handler posts for the event source: a notification ID, or for
 * onStatus the ID of the post (ADR-0094 §6). The server keeps the keys of each account, so the same event processed
 * twice (after a restart in the middle of its handler) posts each reply once. Use it when posting with mykit directly:
 *
 * ```ts
 * await api.POST("/statuses", { params: { header: { "Idempotency-Key": replyIdempotencyKey(id, 1) } }, body });
 * ```
 */
export function replyIdempotencyKey(source: string, n: number): string {
  return `mybot:reply:${source}:${String(n)}`;
}

/**
 * The visibility of a reply to a post of visibility v: never wider than v (ADR-0094 §8), and unlisted for a public
 * post, so that a bot's replies stay out of the public timelines.
 */
export function replyVisibility(v: Visibility): Visibility {
  switch (v) {
    case "direct":
    case "private":
    case "local":
    case "unlisted":
      return v;
    default:
      return "unlisted";
  }
}

/** The error of reply, react, and favourite for an event without a post, such as a follow. */
export class NoStatusError extends Error {
  override readonly name = "NoStatusError";

  constructor() {
    super("mybot: the event has no post");
  }
}

/** The error of follow for an event without an account. */
export class NoAccountError extends Error {
  override readonly name = "NoAccountError";

  constructor() {
    super("mybot: the event has no account");
  }
}

/** The error of reply when the replies to the sender reached a limit of the replyLimits option. The reply was not sent. */
export class ReplyLimitError extends Error {
  override readonly name = "ReplyLimitError";

  constructor() {
    super("mybot: the replies to this sender reached their limit");
  }
}

/** The error of a write that would have waited for the send budget longer than the maxSendWait option. It was not sent. */
export class BudgetExceededError extends Error {
  override readonly name = "BudgetExceededError";

  constructor() {
    super("mybot: the send budget is used up for longer than the longest wait");
  }
}

/** Handles one event. What it throws is logged; the event counts as processed all the same, so that a failing event is not retried for ever. */
export type Handler = (ctx: Context) => void | Promise<void>;

/**
 * Wraps the handlers, such as to check who may run a command, or to log in a format of one's own: it calls next to
 * run the rest of the chain and the handler, or returns without calling it to skip them.
 */
export type Middleware = (ctx: Context, next: () => Promise<void>) => Promise<void>;

/** The data of an answer, which mykit sets whenever the call resolves; an error when the answer has none. */
export function answered<T>(data: T | undefined, operation: string): T {
  if (data === undefined) throw new Error(`mybot: ${operation} answered no data`);
  return data;
}

/** What the contexts of a running bot share. */
export type Runtime = {
  readonly me: CredentialAccount;
  readonly api: Api;
  readonly logger: Logger;
  readonly now: () => number;
  readonly sleep: Sleep;
  readonly budget: Budget;
  readonly replies: ReplyLimiter;
  /** The reply limits, null when they are lifted. */
  readonly replyLimits: readonly Rate[] | null;
  readonly maxSendWait: number;
  readonly respondToBots: boolean;
};

/** The key of the library's state of a Context, which the default middleware reads. */
export const internal: unique symbol = Symbol("mybot.internal");

/** The library's state of a Context. */
export type ContextState = {
  readonly runtime: Runtime;
  /** The ID that the idempotency keys of the replies derive from. */
  readonly source: string;
  /** The key of the account for the reply limits and the order of processing. */
  readonly sender: string;
  replies: number;
  /** The scheduled job of the run, undefined for an event. */
  readonly every: EveryJob | undefined;
  /** Whether the run of the scheduled job posted. */
  posted: boolean;
  /** Makes the helpers wait for the send budget (the sendBudget middleware). */
  budgeted: boolean;
};

/**
 * What a handler receives: the event, its post and its sender, the plain text and the command of a mention, and
 * helpers that answer within the defaults of the library; or for a scheduled job its name and time, and post. Its
 * signal aborts when the bot stops and the grace period is over.
 */
export class Context {
  /** The notification of the event; undefined for onStatus. */
  readonly notification: Notification | undefined;
  /**
   * The post of the event: the post that mentions the bot, the bot's post that was reacted to, favourited, boosted, or
   * quoted, the quote, or the post of onStatus. Undefined for an event without one.
   */
  readonly status: Status | undefined;
  /** The sender: the account that acted. Undefined for a notification without one. */
  readonly account: Account | undefined;
  /** The content of status as plain text (plainText), "" without a status. */
  readonly text: string;
  /** Whether status replies to a post. */
  readonly isReply: boolean;
  /** Whether status is a direct post. */
  readonly isDirect: boolean;
  /** The lowercase name of the command of onCommand, "" otherwise. */
  command = "";
  /** The text after the command's name, with the spaces around it trimmed. */
  args = "";
  /** The name of the scheduled job (Bot.every) that the context runs, "" for an event. */
  readonly job: string;
  /**
   * The time of the run of the scheduled job, in milliseconds since the Unix epoch, which may be earlier than now for
   * a run that catches up; undefined for an event.
   */
  readonly scheduledAt: number | undefined;
  /** Aborts when the bot stops and the grace period is over; pass it to the requests of the handler. */
  readonly signal: AbortSignal;
  /** The library's state; not for handlers. */
  readonly [internal]: ContextState;

  /** A context is made by the bot for each event. */
  constructor(runtime: Runtime, job: Job, signal: AbortSignal) {
    this.signal = signal;
    this.job = job.every?.name ?? "";
    this.scheduledAt = job.every === undefined ? undefined : job.at;
    if (job.every !== undefined) {
      this.notification = undefined;
      this.status = undefined;
      this.account = undefined;
    } else if (job.notification !== undefined) {
      this.notification = job.notification;
      this.status = job.notification.status;
      this.account = job.notification.account;
    } else {
      this.notification = undefined;
      this.status = job.status;
      this.account = job.status.account;
    }
    this.text = this.status === undefined ? "" : plainText(this.status.content);
    this.isReply = this.status?.in_reply_to_id !== undefined;
    this.isDirect = this.status?.visibility === "direct";
    this[internal] = {
      runtime,
      source: job.notification?.id ?? job.status?.id ?? "",
      sender: job.sender,
      replies: 0,
      every: job.every,
      posted: false,
      budgeted: false,
    };
  }

  /** The bot's own account, as verifyCredentials returned it when the bot started. */
  get me(): CredentialAccount {
    return this[internal].runtime.me;
  }

  /**
   * The client of the server, for what the helpers do not cover. Writes through it skip the send budget and the reply
   * limits: use replyIdempotencyKey for the posts.
   */
  get api(): Api {
    return this[internal].runtime.api;
  }

  /** The logger of the bot, with the event's attributes. */
  get logger(): Logger {
    const { logger } = this[internal].runtime;
    if (this.notification !== undefined) return withAttributes(logger, { notification: this.notification.id, type: this.notification.type });
    if (this.status !== undefined) return withAttributes(logger, { status: this.status.id });
    if (this.job !== "") return withAttributes(logger, { job: this.job, scheduledAt: new Date(this.scheduledAt ?? 0).toISOString() });
    return logger;
  }

  /**
   * Posts text as a reply to status. The reply mentions the sender alone, never the other accounts that the post
   * mentions; its visibility is replyVisibility of the post's; and it carries the Idempotency-Key
   * replyIdempotencyKey(event, n) for the n-th reply of the handler, so that processing the event again posts it once.
   * It counts toward the reply limits of the sender and waits for the send budget.
   */
  async reply(text: string): Promise<Status> {
    const st = this.status;
    if (st === undefined) throw new NoStatusError();
    const state = this[internal];
    const { runtime } = state;
    state.replies++;
    const key = replyIdempotencyKey(state.source, state.replies);
    const body = this.account !== undefined && this.account.id !== runtime.me.id ? `@${this.account.acct} ${text}` : text;
    const limited = runtime.replyLimits !== null && this.account !== undefined;
    const at = runtime.now();
    if (limited && !runtime.replies.take(state.sender, at)) throw new ReplyLimitError();
    try {
      await this.#waitBudget("post");
    } catch (error) {
      if (limited) runtime.replies.untake(state.sender, at);
      throw error;
    }
    const { data } = await runtime.api.POST("/statuses", {
      params: { header: { "Idempotency-Key": key } },
      body: { text: body, visibility: replyVisibility(st.visibility), in_reply_to_id: st.id },
      signal: this.signal,
    });
    return answered(data, "createStatus");
  }

  /**
   * Reacts to status with emoji: a Unicode emoji, or the shortcode of a custom emoji of the server. Reacting again
   * with the same emoji changes nothing, so it needs no idempotency key. It waits for the send budget.
   */
  async react(emoji: string): Promise<void> {
    if (this.status === undefined) throw new NoStatusError();
    await this.#waitBudget("write");
    await this.api.PUT("/statuses/{id}/reaction", { params: { path: { id: this.status.id } }, body: { emoji }, signal: this.signal });
  }

  /** Favourites status. A second favourite changes nothing. It waits for the send budget. */
  async favourite(): Promise<void> {
    if (this.status === undefined) throw new NoStatusError();
    await this.#waitBudget("write");
    await this.api.POST("/statuses/{id}/favourite", { params: { path: { id: this.status.id } }, signal: this.signal });
  }

  /** Follows the sender (for a locked account, requests to). Following again changes nothing. It waits for the send budget. */
  async follow(): Promise<void> {
    if (this.account === undefined) throw new NoAccountError();
    await this.#waitBudget("write");
    await this.api.POST("/accounts/{id}/follow", { params: { path: { id: this.account.id } }, body: {}, signal: this.signal });
  }

  /**
   * Posts text from the handler of a scheduled job, with the visibility of its EveryOptions and the Idempotency-Key
   * everyIdempotencyKey(job, scheduledAt): a run posts once, even when two processes of the bot run it, or a restart
   * runs it again. It waits for the send budget. Outside a scheduled job it throws a NotScheduledError, and a second
   * post of a run a PostedAlreadyError.
   */
  async post(text: string): Promise<Status> {
    const state = this[internal];
    const { every } = state;
    if (every === undefined || this.scheduledAt === undefined) throw new NotScheduledError();
    if (state.posted) throw new PostedAlreadyError();
    await this.#waitBudget("post");
    state.posted = true;
    const { data } = await state.runtime.api.POST("/statuses", {
      params: { header: { "Idempotency-Key": everyIdempotencyKey(every.name, this.scheduledAt) } },
      body: { text, visibility: every.visibility },
      signal: this.signal,
    });
    return answered(data, "createStatus");
  }

  /**
   * Waits for the send budget of kind when the sendBudget middleware is in the chain. A wait longer than the
   * maxSendWait option drops the write, logged, with a BudgetExceededError.
   */
  async #waitBudget(kind: SendKind): Promise<void> {
    const state = this[internal];
    if (!state.budgeted) return;
    const { runtime } = state;
    const wait = runtime.budget.reserve(kind, runtime.now(), runtime.maxSendWait);
    if (wait === null) {
      this.logger.warn("mybot: dropped a write that would wait too long for the send budget", { maxWait: runtime.maxSendWait });
      throw new BudgetExceededError();
    }
    if (wait > 0) await runtime.sleep(wait, this.signal);
  }
}
