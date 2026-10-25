// SPDX-License-Identifier: MIT
import {
  all,
  createClient,
  ProblemError,
  RefreshingTokenSource,
  runStream,
  type components,
  type OAuthConfig,
  type Sleep,
  type TokenSource,
} from "@mycroblog/mykit";
import { Budget, defaultMaxSendWait, defaultPostRate, defaultReplyLimits, defaultWriteRate, type Rate, ReplyLimiter } from "./budget.js";
import { answered, Context, type Handler, type Middleware, type Runtime } from "./context.js";
import { Dispatcher, type Job, senderOf, Tracker } from "./dispatch.js";
import type { EveryJob, EveryOptions } from "./every.js";
import { defaultStatePath, FileStore } from "./filestore.js";
import { chain, type Command, defaultHelp, defaultMiddleware, parseCommand } from "./handler.js";
import type { Logger } from "./logger.js";
import { Schedule } from "./schedule.js";
import type { Store } from "./store.js";

type Notification = components["schemas"]["Notification"];
type Account = components["schemas"]["Account"];

/**
 * How a bot receives its notifications: "streaming", the default, from the event stream (GET /streaming), catching up
 * with the notifications after every connection; "polling" by reading the notifications every pollInterval, for
 * networks whose proxies do not pass an event stream.
 */
export type Mode = "streaming" | "polling";

const minute = 60_000;
const hour = 60 * minute;

/** The interval of the polling mode: a minute. */
export const defaultPollInterval = minute;
/** How old a notification the catch-up still handles: 24 hours. */
export const defaultCatchUpLimit = 24 * hour;
/** How long the processed notifications are remembered: 7 days, longer than the catch-up limit. */
export const defaultProcessedRetention = 7 * 24 * hour;
/** How many handlers run at once. */
export const defaultConcurrency = 4;
/** How long the shutdown waits for the running handlers: 10 seconds. */
export const defaultShutdownGrace = 10_000;

/** The longest wait of setTimeout, about 24.8 days: a scheduled job further away sleeps again. */
const maxTimeout = 2 ** 31 - 1;

/** The product token of the library in User-Agent. */
const userAgentProduct = "mybot-ts";

/** Configures a bot. Every option but server and the token has a default. Durations are in milliseconds. */
export type BotOptions = {
  /** The origin of the server, such as "https://example.com". */
  server: string;
  /** Sends the requests; the global `fetch` by default. */
  fetch?: (input: Request) => Promise<Response>;

  /**
   * oauth and refreshToken make the bot renew its access token with the refresh token (mykit's RefreshingTokenSource;
   * ADR-0094 §7): the token that the server issued to the bot's own account for an app registered on its settings
   * page, or that an authorization code was exchanged for. Every renewal rotates the refresh token, which the bot
   * saves in the store; when the store holds one, the bot starts from it rather than from refreshToken, which the
   * server has seen used. The server of oauth is that of the bot when absent.
   */
  oauth?: Omit<OAuthConfig, "server"> & { server?: string };
  refreshToken?: string;
  /**
   * Gives the access token when oauth is absent: mykit's staticToken for tests and short runs, or a source of one's
   * own. The bot does not save its tokens.
   */
  tokenSource?: TokenSource;

  /** Keeps the state of the bot across restarts; a FileStore at defaultStatePath by default. */
  store?: Store;
  /** Receives the log of the bot; `console` by default. */
  logger?: Logger;
  /** The product of the bot, such as "echobot/1.0", sent before the library's own in User-Agent. */
  userAgent?: string;
  /** Sent as Accept-Language, for the language of the server's problems. */
  acceptLanguage?: string;

  /** "streaming" (the default) or "polling". */
  mode?: Mode;
  /** The interval of the polling mode; defaultPollInterval when absent. */
  pollInterval?: number;
  /**
   * How old a notification the catch-up still handles: after a long stop, older ones are skipped. Absent means
   * defaultCatchUpLimit; 0 turns the catch-up off (ADR-0094 §3).
   */
  catchUpLimit?: number;
  /** How long the processed notifications are remembered, longer than catchUpLimit; defaultProcessedRetention when absent. */
  processedRetention?: number;
  /** How many handlers run at once; the events of one sender run one after the other. defaultConcurrency when absent. */
  concurrency?: number;
  /**
   * How long run waits for the running handlers when it stops; then it aborts their signals, and their events are
   * handled again by the next start. defaultShutdownGrace when absent.
   */
  shutdownGrace?: number;

  /** Turns the bot flag of the account on when it is off (with the scope write:accounts), instead of failing to start. */
  setBotFlag?: boolean;
  /**
   * Runs the bot on an account without the bot flag. By default the bot refuses to start there, since the flag is
   * what tells people and other servers that a program posts.
   */
  allowNonBot?: boolean;
  /** Hands the events of accounts with the bot flag to the handlers, which by default skip them. */
  respondToBots?: boolean;
  /** The most replies to one sender, each a number in a window; defaultReplyLimits when absent. */
  replyLimits?: readonly Rate[];
  /** Lifts the reply limits. */
  disableReplyLimits?: boolean;
  /** The send budget of the posts; defaultPostRate when absent. */
  postRate?: Rate;
  /** The send budget of the other writes; defaultWriteRate when absent. */
  writeRate?: Rate;
  /** The longest a write waits for the send budget; a longer wait drops it. defaultMaxSendWait when absent. */
  maxSendWait?: number;

  /** Turns off the answer of the command help. */
  disableHelp?: boolean;
  /** Formats the answer of help; defaultHelp when absent. */
  help?: (commands: readonly Command[]) => string;
  /** Leaves defaultMiddleware out of the chain, for a bot that builds its own with use. */
  noDefaultMiddleware?: boolean;

  /**
   * The clock, in milliseconds since the Unix epoch, of the scheduled jobs, the send budget, and the reply limits; for
   * tests, such as the Clock of @mycroblog/mybot/testing.
   */
  now?: () => number;
  /** Waits for the scheduled jobs, the send budget, the pruning of the store, and between polls; for tests. */
  sleep?: Sleep;
};

/** The error of run on an account without the bot flag, unless the setBotFlag or allowNonBot option. */
export class NotBotError extends Error {
  override readonly name = "NotBotError";

  constructor() {
    super("mybot: the account has no bot flag: set it, or set the setBotFlag or allowNonBot option");
  }
}

/** The error of a second run of a bot. */
export class AlreadyRunningError extends Error {
  override readonly name = "AlreadyRunningError";

  constructor() {
    super("mybot: the bot runs already");
  }
}

/** The error of run when the access token lacks scopes that the bot needs. */
export class ScopeError extends Error {
  override readonly name = "ScopeError";
  /** The scopes the token lacks. */
  readonly missing: readonly string[];

  constructor(missing: readonly string[]) {
    super(`mybot: the access token lacks the scopes ${missing.join(" ")}`);
    this.missing = missing;
  }
}

/** The scopes under follow. */
const followScopes: readonly string[] = ["read:follows", "read:blocks", "read:mutes", "write:follows", "write:blocks", "write:mutes"];

/** Whether the scope have gives need: itself, or a broad scope above it (the oauth2 security scheme of api/openapi.yaml). */
function grants(have: string, need: string): boolean {
  if (have === need) return true;
  if (have === "follow") return followScopes.includes(need);
  return (have === "read" || have === "write") && need.startsWith(`${have}:`);
}

/** The scopes of need that granted (space-separated) does not give. */
export function missingScopes(granted: string, need: readonly string[]): string[] {
  const have = granted.split(/\s+/).filter((s) => s !== "");
  return need.filter((n) => !have.some((h) => grants(h, n)));
}

/** Waits for ms, or rejects with the reason of signal when it aborts first. */
function sleepSignal(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason as Error);
      return;
    }
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal.reason as Error);
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

/** The handlers that a bot registered. */
type Handlers = {
  mention?: Handler;
  follow?: Handler;
  followRequest?: Handler;
  reaction?: Handler;
  favourite?: Handler;
  boost?: Handler;
  quote?: Handler;
  notification?: Handler;
  status?: Handler;
};

/** The options with their defaults. */
type Settings = {
  mode: Mode;
  pollInterval: number;
  catchUpLimit: number;
  processedRetention: number;
  concurrency: number;
  shutdownGrace: number;
  replyLimits: readonly Rate[] | null;
  maxSendWait: number;
  help: (commands: readonly Command[]) => string;
};

/** A positive number, or the default when absent or not positive. */
function positive(value: number | undefined, fallback: number): number {
  return value !== undefined && value > 0 ? value : fallback;
}

/** The rate, or the default when absent or the zero rate. */
function rateOr(r: Rate | undefined, fallback: Rate): Rate {
  return r === undefined || (r.n === 0 && r.per === 0) ? fallback : r;
}

/**
 * A bot: the handlers of its events, and run, which receives the events and hands them to the handlers until it
 * stops. Register the handlers before run.
 */
export class Bot {
  /** The store of the bot. */
  readonly store: Store;
  readonly #options: BotOptions;
  readonly #settings: Settings;
  readonly #handlers: Handlers = {};
  readonly #commands = new Map<string, { command: Command; handler: Handler }>();
  readonly #middleware: Middleware[] = [];
  readonly #every: EveryJob[] = [];
  readonly #logger: Logger;
  readonly #budget: Budget;
  readonly #replies: ReplyLimiter;
  readonly #tracker = new Tracker();
  readonly #now: () => number;
  readonly #sleep: Sleep;
  readonly #fetch: (input: Request) => Promise<Response>;
  #started = false;

  // Set by run.
  #runtime: Runtime | null = null;
  #dispatcher: Dispatcher | null = null;
  #catchUpTask: Promise<void> | null = null;
  /** Set when the grace period of the shutdown is over: the handlers still running record nothing. */
  #abandoned = false;

  constructor(options: BotOptions) {
    if (options.oauth === undefined && options.tokenSource === undefined) {
      throw new Error("mybot: the options need oauth and refreshToken, or a tokenSource");
    }
    if (options.server === "") throw new Error("mybot: the option server is empty");
    this.#options = options;
    this.#settings = {
      mode: options.mode ?? "streaming",
      pollInterval: positive(options.pollInterval, defaultPollInterval),
      // ADR-0094 §3: 0 turns the catch-up off.
      catchUpLimit: options.catchUpLimit ?? defaultCatchUpLimit,
      processedRetention: positive(options.processedRetention, defaultProcessedRetention),
      concurrency: positive(options.concurrency, defaultConcurrency),
      shutdownGrace: positive(options.shutdownGrace, defaultShutdownGrace),
      replyLimits: options.disableReplyLimits === true ? null : (options.replyLimits ?? defaultReplyLimits),
      maxSendWait: positive(options.maxSendWait, defaultMaxSendWait),
      help: options.help ?? defaultHelp,
    };
    this.store = options.store ?? new FileStore(defaultStatePath);
    this.#logger = options.logger ?? console;
    this.#budget = new Budget(rateOr(options.postRate, defaultPostRate), rateOr(options.writeRate, defaultWriteRate));
    this.#replies = new ReplyLimiter(this.#settings.replyLimits ?? []);
    this.#now = options.now ?? Date.now;
    this.#sleep = options.sleep ?? sleepSignal;
    const fetch = options.fetch ?? ((request: Request) => globalThis.fetch(request));
    const userAgent = options.userAgent === undefined || options.userAgent === "" ? userAgentProduct : `${options.userAgent} ${userAgentProduct}`;
    this.#fetch = async (request) => {
      request.headers.set("User-Agent", userAgent);
      const response = await fetch(request);
      this.#budget.observe(response.headers, this.#now());
      return response;
    };
  }

  /** Registers the handler of the mentions of the bot, replies and direct posts included, that are no registered command. */
  onMention(handler: Handler): void {
    this.#handlers.mention = handler;
  }

  /**
   * Registers the handler of the mentions whose first word, after the mentions of the bot at their start, is name,
   * compared without case: `@bot ping` runs the command ping, whose Context has command "ping" and args the rest of
   * the text. description is the command's line in the answer of help, which the bot gives unless the disableHelp
   * option is set or a command help is registered.
   */
  onCommand(name: string, description: string, handler: Handler): void {
    const lower = name.toLowerCase();
    this.#commands.set(lower, { command: { name: lower, description }, handler });
  }

  /** Registers the handler of new followers. */
  onFollow(handler: Handler): void {
    this.#handlers.follow = handler;
  }

  /** Registers the handler of the follow requests to a locked bot account. */
  onFollowRequest(handler: Handler): void {
    this.#handlers.followRequest = handler;
  }

  /** Registers the handler of the emoji reactions to the bot's posts. */
  onReaction(handler: Handler): void {
    this.#handlers.reaction = handler;
  }

  /** Registers the handler of the favourites of the bot's posts. */
  onFavourite(handler: Handler): void {
    this.#handlers.favourite = handler;
  }

  /** Registers the handler of the boosts of the bot's posts. */
  onBoost(handler: Handler): void {
    this.#handlers.boost = handler;
  }

  /** Registers the handler of the quotes of the bot's posts. */
  onQuote(handler: Handler): void {
    this.#handlers.quote = handler;
  }

  /** Registers the handler of every notification that no handler above takes, the types without a handler of their own among them. */
  onNotification(handler: Handler): void {
    this.#handlers.notification = handler;
  }

  /**
   * Registers the handler of the new posts of the home timeline, from the event stream alone: they are not caught up
   * after a disconnection or a restart, and the polling mode does not receive them. The bot's own posts are left out.
   */
  onStatus(handler: Handler): void {
    this.#handlers.status = handler;
  }

  /**
   * Registers a scheduled job: handler runs at each time of the cron expression expr, of the five fields minute hour
   * day-of-month month day-of-week (parseSchedule), in options.timeZone (UTC by default). Its Context carries job and
   * scheduledAt, and post posts with the Idempotency-Key everyIdempotencyKey. The run counts in the store as the job's
   * last run after the handler returns. It throws for an expression or a time zone it does not take (ScheduleError),
   * a visibility other than unlisted, public, and local, and a name that another job has.
   */
  every(expr: string, handler: Handler, options: EveryOptions = {}): void {
    const schedule = Schedule.parse(expr, options.timeZone ?? "");
    const name = options.name === undefined || options.name === "" ? expr : options.name;
    if (this.#every.some((j) => j.name === name)) throw new Error(`mybot: a scheduled job is named "${name}" already`);
    const visibility = options.visibility ?? "unlisted";
    if (!["unlisted", "public", "local"].includes(visibility)) {
      throw new Error(`mybot: the scheduled job "${name}" cannot post with the visibility "${visibility}": use unlisted, public, or local`);
    }
    this.#every.push({ name, schedule, handler, catchUp: options.catchUp ?? "skip", visibility });
  }

  /** Adds middleware inside the default chain, in the order given. */
  use(...middleware: Middleware[]): void {
    this.#middleware.push(...middleware);
  }

  /** The registered commands, by name, with help when the bot answers it. */
  commands(): Command[] {
    const commands = [...this.#commands.values()].map((c) => c.command);
    if (!this.#commands.has("help") && this.#options.disableHelp !== true) commands.push({ name: "help", description: "list the commands" });
    return commands.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  }

  /** The scopes the bot needs: its own account, the notifications and the event stream, and posting (ADR-0094 §7); and write:accounts to set the bot flag. */
  #requiredScopes(): string[] {
    const scopes = ["read:accounts", "read:notifications", "read:statuses", "write:statuses"];
    if (this.#options.setBotFlag === true) scopes.push("write:accounts");
    return scopes;
  }

  /** The token source of the options: a RefreshingTokenSource that saves every rotated refresh token in the store, starting from the one the store holds. */
  async #newTokenSource(): Promise<TokenSource> {
    const { oauth, tokenSource } = this.#options;
    if (oauth === undefined) {
      if (tokenSource === undefined) throw new Error("mybot: the options need oauth and refreshToken, or a tokenSource");
      return tokenSource;
    }
    let refreshToken: string | null;
    try {
      refreshToken = await this.store.refreshToken();
    } catch (error) {
      throw new Error("mybot: read the refresh token from the store", { cause: error });
    }
    if (refreshToken === null || refreshToken === "") refreshToken = this.#options.refreshToken ?? "";
    if (refreshToken === "") throw new Error("mybot: no refresh token: set the refreshToken option");
    return new RefreshingTokenSource(
      { ...oauth, server: oauth.server ?? this.#options.server, fetch: oauth.fetch ?? this.#fetch },
      { accessToken: "", refreshToken },
      async (token) => {
        if (token.refreshToken === undefined || token.refreshToken === "") return;
        try {
          await this.store.setRefreshToken(token.refreshToken);
        } catch (error) {
          throw new Error("mybot: save the rotated refresh token", { cause: error });
        }
      },
      this.#now,
    );
  }

  /** Connects to the server: the client, the scopes of the token, the own account and its bot flag. */
  async #start(signal: AbortSignal): Promise<{ runtime: Runtime; tokenSource: TokenSource }> {
    const tokenSource = await this.#newTokenSource();
    try {
      await tokenSource.getAccessToken();
    } catch (error) {
      throw new Error("mybot: get the access token", { cause: error });
    }
    const scope = tokenSource instanceof RefreshingTokenSource ? tokenSource.token?.scope : undefined;
    if (scope !== undefined && scope !== "") {
      const missing = missingScopes(scope, this.#requiredScopes());
      if (missing.length > 0) throw new ScopeError(missing);
    }
    const api = createClient({
      server: this.#options.server,
      tokenSource,
      fetch: this.#fetch,
      ...(this.#options.acceptLanguage === undefined ? {} : { acceptLanguage: this.#options.acceptLanguage }),
    });
    let me = answered((await api.GET("/accounts/verify_credentials", { signal })).data, "verifyCredentials");
    if (!me.bot) {
      if (this.#options.setBotFlag === true) {
        me = answered((await api.PATCH("/accounts/update_credentials", { body: { bot: true }, signal })).data, "updateCredentials");
        this.#logger.info("mybot: set the bot flag of the account", { account: me.acct });
      } else if (this.#options.allowNonBot === true) {
        this.#logger.warn("mybot: the account has no bot flag", { account: me.acct });
      } else {
        throw new NotBotError();
      }
    }
    const runtime: Runtime = {
      me,
      api,
      logger: this.#logger,
      now: this.#now,
      sleep: this.#sleep,
      budget: this.#budget,
      replies: this.#replies,
      replyLimits: this.#settings.replyLimits,
      maxSendWait: this.#settings.maxSendWait,
      respondToBots: this.#options.respondToBots === true,
    };
    return { runtime, tokenSource };
  }

  /**
   * Starts the bot and handles its events until signal aborts, then shuts down gracefully: it stops receiving, waits
   * for the running handlers for the shutdownGrace option, aborts the signals of those still running, whose events
   * stay unprocessed for the next start, and resolves. It rejects when the bot cannot start (NotBotError, ScopeError,
   * a ProblemError of verifyCredentials) or when the server refuses the event stream for good, such as for a revoked
   * token.
   *
   * It verifies the account, then receives the notifications: in the streaming mode from the event stream, catching
   * up with listNotifications back to the store's position after every connection; in the polling mode by reading
   * them every pollInterval. Each notification is handled once: it counts as processed, in the store, after its
   * handler returns, and one that the store knows is skipped.
   */
  async run({ signal = new AbortController().signal }: { signal?: AbortSignal } = {}): Promise<void> {
    if (this.#started) throw new AlreadyRunningError();
    this.#started = true;

    const { runtime, tokenSource } = await this.#start(signal);
    this.#runtime = runtime;
    await this.#prune();

    const stopRun = new AbortController();
    const runSignal = AbortSignal.any([signal, stopRun.signal]);
    const stopHandlers = new AbortController();
    const dispatcher = new Dispatcher(this.#settings.concurrency, (job) => this.#process(stopHandlers.signal, job));
    this.#dispatcher = dispatcher;
    const pruning = this.#pruneLoop(runSignal);
    await this.#submitDue(false);
    const scheduling = Promise.all(this.#every.map((j) => this.#schedule(j, runSignal)));

    let failure: { error: unknown } | null = null;
    try {
      if (this.#settings.mode === "polling") await this.#poll(runSignal);
      else await this.#stream(runSignal, runtime, tokenSource);
    } catch (error) {
      failure = { error };
    }
    stopRun.abort();
    await pruning;
    await scheduling;
    await this.#catchUpTask;

    await this.#shutdown(dispatcher, stopHandlers);
    if (failure !== null && !signal.aborted) throw failure.error;
  }

  /**
   * Starts the bot, handles once what is due, and resolves, for a bot that a cron or a job runner such as Cloud Run
   * jobs starts again and again (ADR-0094 §9): the notifications since the store's position, back to the catchUpLimit
   * option, and for each scheduled job the newest of its past times after its last run, whatever its catchUp. It
   * resolves when their handlers have returned. When signal aborts first, it shuts down as run does, and what did not
   * finish is due again on the next run. It rejects with the errors of starting that run rejects with, and with that
   * of reading the notifications.
   */
  async runOnce({ signal = new AbortController().signal }: { signal?: AbortSignal } = {}): Promise<void> {
    if (this.#started) throw new AlreadyRunningError();
    this.#started = true;

    const { runtime } = await this.#start(signal);
    this.#runtime = runtime;
    await this.#prune();

    const stopHandlers = new AbortController();
    const dispatcher = new Dispatcher(this.#settings.concurrency, (job) => this.#process(stopHandlers.signal, job));
    this.#dispatcher = dispatcher;

    let failure: { error: unknown } | null = null;
    try {
      await this.#catchUp(signal);
    } catch (error) {
      failure = { error };
    }
    await this.#submitDue(true);
    let onAbort: (() => void) | undefined;
    await Promise.race([
      dispatcher.idle(),
      new Promise<void>((resolve) => {
        onAbort = resolve;
        if (signal.aborted) resolve();
        else signal.addEventListener("abort", onAbort, { once: true });
      }),
    ]);
    if (onAbort !== undefined) signal.removeEventListener("abort", onAbort);

    await this.#shutdown(dispatcher, stopHandlers);
    if (failure !== null && !signal.aborted) throw failure.error;
  }

  /**
   * Stops the dispatcher, waits for the running handlers for the shutdownGrace option, and aborts the signals of those
   * still running, which record nothing.
   */
  async #shutdown(dispatcher: Dispatcher, stopHandlers: AbortController): Promise<void> {
    dispatcher.stop();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const graceOver = await Promise.race([
      dispatcher.idle().then(() => false),
      new Promise<boolean>((resolve) => {
        timer = setTimeout(() => {
          resolve(true);
        }, this.#settings.shutdownGrace);
      }),
    ]);
    clearTimeout(timer);
    if (graceOver) {
      this.#abandoned = true;
      this.#logger.warn("mybot: aborted the handlers still running after the grace period", { grace: this.#settings.shutdownGrace });
    }
    stopHandlers.abort();
  }

  /** The newest time of job at or before now and after its last run, null when there is none. */
  async #due(job: EveryJob, now: number): Promise<number | null> {
    let last: number | null;
    try {
      last = await this.store.lastRun(job.name);
    } catch (error) {
      throw new Error(`mybot: read the last run of "${job.name}" from the store`, { cause: error });
    }
    const at = job.schedule.prev(now);
    if (at === null || (last !== null && at <= last)) return null;
    return at;
  }

  /** Hands the jobs their due time: every job (runOnce), or those of catchUp "latest" (run). */
  async #submitDue(all: boolean): Promise<void> {
    const now = this.#now();
    for (const job of this.#every) {
      if (!all && job.catchUp !== "latest") continue;
      try {
        const at = await this.#due(job, now);
        if (at !== null) this.#submitRun(job, at);
      } catch (error) {
        this.#logger.error("mybot: the scheduled job did not catch up", { job: job.name, error });
      }
    }
  }

  /** Hands the run at of job to the dispatcher; the runs of one job go one after the other. */
  #submitRun(job: EveryJob, at: number): void {
    this.#dispatcher?.submit({ every: job, at, sender: `\u0000every:${job.name}` });
  }

  /**
   * Runs job at its times until signal aborts. A run that the clock reaches late, such as after the machine slept,
   * runs once, and the times it passed are skipped.
   */
  async #schedule(job: EveryJob, signal: AbortSignal): Promise<void> {
    let next = job.schedule.next(this.#now());
    while (next !== null) {
      try {
        await this.#sleep(Math.min(next - this.#now(), maxTimeout), signal);
      } catch {
        return;
      }
      if (signal.aborted) return;
      const now = this.#now();
      // Woken early, as by the longest timeout or a clock of a test.
      if (now < next) continue;
      this.#submitRun(job, next);
      next = job.schedule.next(Math.max(this.#now(), next));
    }
  }

  /** Runs one time of a scheduled job and records it as the job's last run. */
  async #runScheduled(signal: AbortSignal, job: Job & { every: EveryJob }): Promise<void> {
    const runtime = this.#running();
    const { every } = job;
    try {
      const last = await this.store.lastRun(every.name);
      // Ran already.
      if (last !== null && job.at <= last) return;
      const ctx = new Context(runtime, job, signal);
      try {
        await chain(this.#chain(), every.handler)(ctx);
      } catch (error) {
        ctx.logger.error("mybot: the handler failed", { error });
      }
      if (this.#abandoned || signal.aborted) return;
      await this.store.setLastRun(every.name, job.at);
    } catch (error) {
      this.#logger.error("mybot: record the run of the scheduled job", { job: every.name, error });
    }
  }

  /** Receives the event stream until signal aborts or the server refuses it, catching up after every connection. */
  async #stream(signal: AbortSignal, runtime: Runtime, tokenSource: TokenSource): Promise<void> {
    let again = false;
    const takeAgain = (): boolean => {
      const a = again;
      again = false;
      return a;
    };
    const catchUp = () => {
      if (this.#catchUpTask !== null) {
        again = true;
        return;
      }
      this.#catchUpTask = (async () => {
        do {
          try {
            await this.#catchUp(signal);
          } catch (error) {
            if (!signal.aborted) this.#logger.warn("mybot: the catch-up failed; it runs again on the next connection", { error });
          }
        } while (takeAgain() && !signal.aborted);
        this.#catchUpTask = null;
      })();
    };
    await runStream({
      server: this.#options.server,
      tokenSource,
      fetch: this.#fetch,
      signal,
      ...(this.#options.acceptLanguage === undefined ? {} : { acceptLanguage: this.#options.acceptLanguage }),
      onOpen: (reconnected) => {
        if (reconnected) this.#logger.info("mybot: the event stream reconnected");
        catchUp();
      },
      onEvent: (event) => {
        if (event.type === "notification") {
          this.#submitNotification(event.notification);
        } else if (event.type === "update" && this.#handlers.status !== undefined && event.status.account.id !== runtime.me.id) {
          this.#dispatcher?.submit({ status: event.status, sender: senderOf(event.status.account) });
        }
      },
      onError: (error) => {
        this.#logger.warn("mybot: the event stream failed; connecting again", { error });
      },
    });
  }

  /** Catches up every pollInterval until signal aborts. */
  async #poll(signal: AbortSignal): Promise<void> {
    for (;;) {
      try {
        await this.#catchUp(signal);
      } catch (error) {
        if (signal.aborted) return;
        if (error instanceof ProblemError && error.status >= 400 && error.status < 500 && error.status !== 429) throw error;
        this.#logger.warn("mybot: reading the notifications failed; trying again", { error });
      }
      try {
        await this.#sleep(this.#settings.pollInterval, signal);
      } catch (error) {
        if (signal.aborted) return;
        throw error;
      }
    }
  }

  /**
   * Reads the notifications newest first back to the store's position, or to the catch-up limit, and hands them to
   * the handlers oldest first (ADR-0094 §3).
   */
  async #catchUp(signal: AbortSignal): Promise<void> {
    if (this.#settings.catchUpLimit <= 0) return;
    const { api } = this.#running();
    let position: string | null;
    try {
      position = await this.store.position();
    } catch (error) {
      throw new Error("mybot: read the position from the store", { cause: error });
    }
    this.#tracker.hold();
    try {
      const cutoff = this.#now() - this.#settings.catchUpLimit;
      const found: Notification[] = [];
      for await (const n of all(async (cursor) => {
        const { data } = await api.GET("/notifications", { params: { query: { limit: 40, ...(cursor === undefined ? {} : { cursor }) } }, signal });
        return answered(data, "listNotifications");
      })) {
        if ((position !== null && n.id <= position) || Date.parse(n.created_at) < cutoff) break;
        found.push(n);
      }
      for (const n of found.reverse()) this.#submitNotification(n);
    } finally {
      const p = this.#tracker.release();
      if (p !== null) await this.#setPosition(p);
    }
  }

  /** Hands a notification to the dispatcher. */
  #submitNotification(n: Notification): void {
    // The job starts after an await at the earliest, so it is pending in the tracker before it can complete.
    if (this.#dispatcher?.submit({ notification: n, sender: senderOf(n.account) }) === true) this.#tracker.add(n.id);
  }

  /** Handles one job and records its notification processed. */
  async #process(signal: AbortSignal, job: Job): Promise<void> {
    if (job.every !== undefined) {
      await this.#runScheduled(signal, job);
      return;
    }
    const runtime = this.#running();
    const n = job.notification;
    try {
      if (n !== undefined && (await this.store.isProcessed(n.id))) {
        await this.#completed(n.id);
        return;
      }
      const ctx = new Context(runtime, job, signal);
      if (!this.#isMe(ctx.account)) {
        const handler = this.#route(ctx);
        if (handler !== undefined) {
          try {
            await chain(this.#chain(), handler)(ctx);
          } catch (error) {
            ctx.logger.error("mybot: the handler failed", { error });
          }
        }
      }
      // Aborted by the shutdown: handled again by the next start.
      if (n === undefined || this.#abandoned || signal.aborted) return;
      await this.store.markProcessed(n.id, this.#now());
      await this.#completed(n.id);
    } catch (error) {
      this.#logger.error("mybot: record the notification processed", { notification: n?.id, error });
    }
  }

  /** Moves the position after a processed notification. */
  async #completed(id: string): Promise<void> {
    const p = this.#tracker.complete(id);
    if (p !== null) await this.#setPosition(p);
  }

  /** Saves the position when it is newer than the saved one. */
  async #setPosition(p: string): Promise<void> {
    try {
      const current = await this.store.position();
      if (current !== null && current >= p) return;
      await this.store.setPosition(p);
    } catch (error) {
      this.#logger.error("mybot: save the position", { error });
    }
  }

  /** The runtime of the running bot. */
  #running(): Runtime {
    if (this.#runtime === null) throw new Error("mybot: the bot has not started");
    return this.#runtime;
  }

  /** Whether account is the bot's own, whose events never reach a handler (ADR-0094 §8). */
  #isMe(account: Account | undefined): boolean {
    return account !== undefined && account.id === this.#runtime?.me.id;
  }

  /** The middleware of every handler: the default chain outermost, then those of use. */
  #chain(): Middleware[] {
    return this.#options.noDefaultMiddleware === true ? this.#middleware : [...defaultMiddleware(), ...this.#middleware];
  }

  /** Picks the handler of the event of ctx, filling in its command; undefined when none takes it. */
  #route(ctx: Context): Handler | undefined {
    const n = ctx.notification;
    if (n === undefined) return this.#handlers.status;
    let handler: Handler | undefined;
    switch (n.type) {
      case "mention": {
        const parsed = parseCommand(ctx.text, ctx.me);
        if (parsed !== null) {
          const command = this.#commands.get(parsed.name);
          if (command !== undefined) {
            ctx.command = parsed.name;
            ctx.args = parsed.args;
            return command.handler;
          }
          if (parsed.name === "help" && this.#options.disableHelp !== true && this.#commands.size > 0) {
            ctx.command = parsed.name;
            ctx.args = parsed.args;
            return (c) => this.#help(c);
          }
        }
        handler = this.#handlers.mention;
        break;
      }
      case "follow":
        handler = this.#handlers.follow;
        break;
      case "follow_request":
        handler = this.#handlers.followRequest;
        break;
      case "reaction":
        handler = this.#handlers.reaction;
        break;
      case "favourite":
        handler = this.#handlers.favourite;
        break;
      case "boost":
        handler = this.#handlers.boost;
        break;
      case "quote":
        handler = this.#handlers.quote;
        break;
      default:
        break;
    }
    return handler ?? this.#handlers.notification;
  }

  /** Answers the help command with the registered commands. */
  async #help(ctx: Context): Promise<void> {
    await ctx.reply(this.#settings.help(this.commands()));
  }

  /** Forgets the processed notifications older than the processedRetention option. */
  async #prune(): Promise<void> {
    try {
      await this.store.forgetProcessed(this.#now() - this.#settings.processedRetention);
    } catch (error) {
      this.#logger.warn("mybot: forget the old processed notifications", { error });
    }
  }

  /** Prunes every hour until signal aborts. */
  async #pruneLoop(signal: AbortSignal): Promise<void> {
    for (;;) {
      try {
        await this.#sleep(hour, signal);
      } catch {
        return;
      }
      await this.#prune();
    }
  }
}
