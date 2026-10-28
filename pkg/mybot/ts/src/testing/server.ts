// SPDX-License-Identifier: MIT
import { staticToken, type components } from "@mycroblog/mykit";
import type { BotOptions } from "../bot.js";
import type { Logger } from "../logger.js";
import { MemoryStore, type Store } from "../store.js";
import { Clock, waitFor } from "./clock.js";

type Notification = components["schemas"]["Notification"];
type NotificationType = components["schemas"]["NotificationType"];
type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];
type CredentialAccount = components["schemas"]["CredentialAccount"];
type Visibility = components["schemas"]["Visibility"];
type CreateStatusRequest = components["schemas"]["CreateStatusRequest"];
type UpdateCredentialsRequest = components["schemas"]["UpdateCredentialsRequest"];
type Relationship = components["schemas"]["Relationship"];

/** The domain of the server's accounts, and of its origin https://bot.test. */
export const domain = "bot.test";

/** The access token of Server.options; the server answers 401 to a request without an access token. */
export const token = "mybottest-token";

/** The interval of the comments that keep an event stream alive, as the server sends them. */
const heartbeat = 30_000;

/** A post that the bot created with createStatus. */
export type Post = {
  /** The Idempotency-Key of the request, such as replyIdempotencyKey of the event; null without one. */
  readonly idempotencyKey: string | null;
  /** The body of the request. */
  readonly request: CreateStatusRequest;
  /** The post as the server answered it. */
  readonly status: Status;
};

/** An emoji reaction of the bot to a post (reactToStatus). */
export type Reaction = { readonly statusId: string; readonly emoji: string };

/** An open event stream. */
type Stream = { send(event: string, data: string): void; close(): void };

/** The options of the waits of Server. */
export type WaitOptions = {
  /** Gives up waiting when it aborts; after 10 seconds by default. */
  signal?: AbortSignal;
};

/** The options of new Server. */
export type ServerOptions = {
  /**
   * The time at which the server's clock starts, in milliseconds since the Unix epoch; the current time by default.
   * A test that moves the clock to fixed times (clock.set) starts it at or before the first of them, so that it runs
   * the same on any day.
   */
  now?: number;
};

/** A logger that drops everything, the logger of Server.options. */
export const quietLogger: Logger = { debug() {}, info() {}, warn() {}, error() {} };

/** Escapes text for HTML. */
function escapeHTML(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&#34;")
    .replace(/'/g, "&#39;");
}

/** An account of acct ("alice" local, "alice@remote.example" remote), with every field that the spec requires. */
function newAccount(id: string, acct: string, createdAt: number): Account {
  const at = acct.indexOf("@");
  const username = at < 0 ? acct : acct.slice(0, at);
  const d = at < 0 ? domain : acct.slice(at + 1);
  return {
    id,
    username,
    acct,
    domain: d,
    uri: `https://${d}/users/${username}`,
    display_name: "",
    note: "",
    url: `https://${d}/@${username}`,
    locked: false,
    bot: false,
    created_at: new Date(createdAt).toISOString(),
    fields: [],
    emojis: [],
    badges: [],
  };
}

/** The account of a post of the bot's own account. */
function accountOf(me: CredentialAccount): Account {
  const { discoverable: _d, indexable: _i, ai_opt_out: _a, source: _s, posting: _p, permissions: _m, ...account } = me;
  return account;
}

/**
 * A fake server of the client API for the tests of a bot (ADR-0094 §10): a fetch, which Server.options hands the bot,
 * that answers the operations that mybot calls (verifyAccountCredentials, updateAccountCredentials,
 * listNotifications, streamEvents, createStatus, and those of the helpers of Context: reactToStatus, favouriteStatus,
 * followAccount) with the types of api/openapi.yaml, and keeps in memory what the bot did. inject sends a notification
 * to the bot, posted returns what it posted, and clock moves the time of its scheduled jobs. It mirrors mybottest of
 * the Go library.
 *
 * ```ts
 * const srv = new Server();
 * const bot = new Bot(srv.options());
 * bot.onCommand("ping", "answer pong", async (ctx) => {
 *   await ctx.reply("pong");
 * });
 * const controller = new AbortController();
 * const done = bot.run({ signal: controller.signal });
 * srv.inject(srv.mention(srv.account("alice"), "ping", "public"));
 * const [post] = await srv.waitPosted(1); // post.request.text === "@alice pong"
 * controller.abort();
 * await done;
 * ```
 *
 * injectMissed adds a notification that only the bot's catch-up finds, such as one that came while the bot was
 * stopped; dropStreams ends the event streams, for the bot to connect again and catch up, and disconnect holds its
 * connections until reconnect, so that the notifications of inject meanwhile come by the catch-up alone. Like the
 * server, it posts once per Idempotency-Key. It answers what the library needs and no more: it keeps no timelines,
 * follows, or visibility rules.
 */
export class Server {
  /** The origin of the server, for the option server of a bot. */
  readonly url = `https://${domain}`;
  /** The clock of the server: options sets it as the bot's, and the notifications and posts take its time. */
  readonly clock: Clock;

  #seq = 0;
  #me: CredentialAccount;
  /** Newest first. */
  #notifications: Notification[] = [];
  #statuses = new Map<string, Status>();
  #posts: Post[] = [];
  /** Idempotency-Key to the index in #posts. */
  #keys = new Map<string, number>();
  #reactions: Reaction[] = [];
  #favourites: string[] = [];
  #follows: string[] = [];
  #streams = new Set<Stream>();
  /** While disconnect holds the connections, the resolvers of the held ones. */
  #held: (() => void)[] | null = null;
  #listeners = new Set<() => void>();

  /**
   * A server whose own account, me, is the bot account "bot" with the bot flag, and whose clock is at the current
   * time, or at the option now.
   */
  constructor(options: ServerOptions = {}) {
    this.clock = new Clock(options.now ?? Math.floor(Date.now() / 1000) * 1000);
    const me = newAccount(this.#newId(), "bot", this.clock.now());
    this.#me = {
      ...me,
      bot: true,
      discoverable: false,
      indexable: false,
      ai_opt_out: false,
      source: { note: "", fields: [] },
      posting: { visibilities: ["public", "local", "unlisted", "private", "direct"], visibility_ceiling: "public" },
      permissions: [],
      statuses_count: 0,
      follow_requests_count: 0,
    };
  }

  /**
   * The options of a bot of the server: its url and fetch, the access token token, a MemoryStore, the server's clock,
   * and a logger that drops everything. Override what the test needs.
   */
  options(extra: Partial<BotOptions> = {}): BotOptions & { store: Store } {
    return {
      server: this.url,
      fetch: this.fetch,
      tokenSource: staticToken(token),
      store: new MemoryStore(),
      now: this.clock.now,
      sleep: this.clock.sleep,
      logger: quietLogger,
      ...extra,
    };
  }

  /** Ends the event streams and lets the held connections through. */
  close(): void {
    this.reconnect();
    this.dropStreams();
  }

  /** The bot's own account, as verifyAccountCredentials answers it. */
  get me(): CredentialAccount {
    return this.#me;
  }

  /** Sets the bot flag of the bot's own account, such as to off to test the option setBotFlag. */
  setBot(on: boolean): void {
    this.#me = { ...this.#me, bot: on };
  }

  /** A new account of another user, with acct such as "alice" (a local account) or "alice@remote.example" (a remote one). */
  account(acct: string): Account {
    return newAccount(this.#newId(), acct, this.clock.now());
  }

  /**
   * A new post of from with the plain text text, whose content mentions the accounts of mentions (none for a post that
   * mentions nobody) at its start, as the server renders them.
   */
  status(from: Account, text: string, visibility: Visibility, ...mentions: Account[]): Status {
    const st = this.#newStatus(from, text, visibility);
    st.mentions = mentions.map((m) => ({ id: m.id, acct: m.acct, username: m.username, url: m.url }));
    const prefix = mentions
      .map(
        (m) =>
          `<span class="h-card"><a href="${escapeHTML(m.url)}" class="u-url mention">@<span>${escapeHTML(m.username)}</span></a></span> `,
      )
      .join("");
    st.content = `<p>${prefix}${escapeHTML(text)}</p>`;
    this.#statuses.set(st.id, st);
    return st;
  }

  /**
   * A new mention notification: a post of from with visibility that mentions the bot, then says text, such as
   * mention(alice, "ping", "public") for "@bot ping". Hand it to inject.
   */
  mention(from: Account, text: string, visibility: Visibility): Notification {
    const st = this.status(from, text, visibility, accountOf(this.#me));
    return this.notification("mention", from, st);
  }

  /** A new notification of type from from about st (none for one without a post, such as a follow). Hand it to inject. */
  notification(type: NotificationType, from: Account, st?: Status): Notification {
    return { id: "", type, created_at: "", account: from, ...(st === undefined ? {} : { status: st }) };
  }

  /**
   * Adds the notification n and sends it on the open event streams, as the server does when it happens. An empty ID
   * and creation time are filled in (a new ID, newer than those before, and the clock's time); it returns n as it was
   * added. A bot whose stream is not open yet finds it when it catches up.
   */
  inject(n: Notification): Notification {
    const added = this.#add(n);
    this.#broadcast("notification", added);
    return added;
  }

  /**
   * Adds the notification n without sending it on the event streams, as one that came while the bot was stopped: only
   * the bot's catch-up (listNotifications) finds it, when the bot starts or connects again. The catch-up reads back to
   * the newest notification that the bot processed, so add the missed ones before the bot runs, or between disconnect
   * and reconnect. It fills n in as inject does.
   */
  injectMissed(n: Notification): Notification {
    return this.#add(n);
  }

  /** Sends the post st on the open event streams as a new post of the home timeline (the event update, Bot.onStatus). */
  injectStatus(st: Status): void {
    this.#statuses.set(st.id, st);
    this.#broadcast("update", st);
  }

  /** Ends every open event stream, as the server does when it shuts down: the bot connects again and catches up. */
  dropStreams(): void {
    for (const s of this.#streams) s.close();
    this.#streams.clear();
    this.#notify();
  }

  /**
   * Ends every open event stream and holds the bot's next connections until reconnect, as a network that is down: the
   * notifications that inject adds meanwhile reach the bot only by its catch-up after reconnect.
   */
  disconnect(): void {
    this.#held ??= [];
    this.dropStreams();
  }

  /** Lets the connections that disconnect holds through. */
  reconnect(): void {
    const held = this.#held;
    this.#held = null;
    for (const resolve of held ?? []) resolve();
  }

  /** How many event streams are open. */
  get streams(): number {
    return this.#streams.size;
  }

  /** The notifications of the server, newest first, as listNotifications answers them. */
  notifications(): Notification[] {
    return [...this.#notifications];
  }

  /** The posts that the bot created, oldest first: one per Idempotency-Key. */
  posted(): Post[] {
    return [...this.#posts];
  }

  /** The reactions of the bot, oldest first; reacting again with the same emoji adds none. */
  reactions(): Reaction[] {
    return [...this.#reactions];
  }

  /** The IDs of the posts that the bot favourited, oldest first. */
  favourites(): string[] {
    return [...this.#favourites];
  }

  /** The IDs of the accounts that the bot followed, oldest first. */
  follows(): string[] {
    return [...this.#follows];
  }

  /** Waits until the bot has created at least n posts and resolves to them; rejects when the signal of options aborts first. */
  async waitPosted(n: number, { signal = AbortSignal.timeout(10_000) }: WaitOptions = {}): Promise<Post[]> {
    await waitFor(this.#listeners, () => this.#posts.length >= n, signal, `${String(n)} posts`);
    return this.posted();
  }

  /** Waits until at least n event streams are open; rejects when the signal of options aborts first. */
  waitStreams(n: number, { signal = AbortSignal.timeout(10_000) }: WaitOptions = {}): Promise<void> {
    return waitFor(this.#listeners, () => this.#streams.size >= n, signal, `${String(n)} event streams`);
  }

  #notify(): void {
    for (const listener of this.#listeners) listener();
  }

  /** A new UUIDv7-shaped ID, after every ID before it in their order, as the server's IDs are. */
  #newId(): string {
    this.#seq++;
    return `01900000-0000-7000-8000-${String(this.#seq).padStart(12, "0")}`;
  }

  #add(n: Notification): Notification {
    const added: Notification = {
      ...n,
      id: n.id === "" ? this.#newId() : n.id,
      created_at: n.created_at === "" ? new Date(this.clock.now()).toISOString() : n.created_at,
    };
    if (added.status !== undefined) this.#statuses.set(added.status.id, added.status);
    this.#notifications.unshift(added);
    this.#notify();
    return added;
  }

  #broadcast(event: string, data: unknown): void {
    const text = JSON.stringify(data);
    for (const s of this.#streams) s.send(event, text);
  }

  /** A post of account by the server, at the clock's time. */
  #newStatus(account: Account, text: string, visibility: Visibility): Status {
    const id = this.#newId();
    return {
      id,
      uri: `https://${domain}/statuses/${id}`,
      account,
      content: `<p>${escapeHTML(text)}</p>`,
      spoiler_text: "",
      sensitive: false,
      visibility,
      language: "und",
      created_at: new Date(this.clock.now()).toISOString(),
      media_attachments: [],
      mentions: [],
      tags: [],
      emojis: [],
      replies_count: 0,
      boosts_count: 0,
      favourites_count: 0,
      reactions: [],
    };
  }

  /** Answers a request of the bot: the option fetch of a bot. */
  readonly fetch = async (request: Request): Promise<Response> => {
    if (!(request.headers.get("Authorization") ?? "").startsWith("Bearer ")) {
      return problem(401, "/api/v1/problems/unauthenticated", "The operation needs an access token.", {
        "WWW-Authenticate": 'Bearer error="invalid_token"',
      });
    }
    const url = new URL(request.url);
    const route = `${request.method} ${url.pathname}`;
    switch (route) {
      case "GET /api/v1/accounts/verify_credentials":
        return json(this.#me);
      case "PATCH /api/v1/accounts/update_credentials":
        return this.#updateCredentials(request);
      case "GET /api/v1/notifications":
        return this.#listNotifications(url);
      case "GET /api/v1/streaming":
        return this.#streamEvents(request);
      case "POST /api/v1/statuses":
        return this.#createStatus(request);
      default:
        break;
    }
    const reaction = /^PUT \/api\/v1\/statuses\/([^/]+)\/reaction$/.exec(route);
    if (reaction !== null) return this.#reactToStatus(decodeURIComponent(reaction[1] ?? ""), request);
    const favourite = /^POST \/api\/v1\/statuses\/([^/]+)\/favourite$/.exec(route);
    if (favourite !== null) return this.#favouriteStatus(decodeURIComponent(favourite[1] ?? ""));
    const follow = /^POST \/api\/v1\/accounts\/([^/]+)\/follow$/.exec(route);
    if (follow !== null) return this.#followAccount(decodeURIComponent(follow[1] ?? ""));
    return problem(404, "about:blank", `mybot/testing does not serve ${request.method} ${url.pathname}.`);
  };

  async #updateCredentials(request: Request): Promise<Response> {
    const body = (await request.json()) as UpdateCredentialsRequest;
    this.#me = {
      ...this.#me,
      ...(body.bot === undefined ? {} : { bot: body.bot }),
      ...(body.display_name === undefined ? {} : { display_name: body.display_name }),
    };
    return json(this.#me);
  }

  /** The notifications newest first, a page of limit (20 by default, 40 at most) from the opaque cursor. */
  #listNotifications(url: URL): Response {
    const q = url.searchParams;
    let limit = 20;
    const l = q.get("limit");
    if (l !== null) {
      limit = Number(l);
      if (!/^[0-9]+$/.test(l) || limit < 1 || limit > 40) return problem(400, "about:blank", "limit is from 1 to 40.");
    }
    const types = q.getAll("types");
    let items = this.notifications();
    if (types.length > 0) items = items.filter((n) => types.includes(n.type));
    // The cursor is the ID of the last notification of the previous page.
    const cursor = q.get("cursor");
    let start = 0;
    if (cursor !== null && cursor !== "") {
      start = items.findIndex((n) => n.id < cursor);
      if (start < 0) start = items.length;
    }
    const end = Math.min(start + limit, items.length);
    const last = items[end - 1];
    return json({
      items: items.slice(start, end),
      ...(end < items.length && last !== undefined ? { next_cursor: last.id } : {}),
    });
  }

  async #streamEvents(request: Request): Promise<Response> {
    const held = this.#held;
    if (held !== null) {
      await new Promise<void>((resolve) => {
        held.push(resolve);
        request.signal.addEventListener(
          "abort",
          () => {
            resolve();
          },
          { once: true },
        );
      });
      if (request.signal.aborted) return new Response(null, { status: 499 });
    }
    const encoder = new TextEncoder();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    let open = true;
    const stream: Stream = {
      send(event, data) {
        if (open) controller.enqueue(encoder.encode(`event: ${event}\ndata: ${data}\n\n`));
      },
      close: () => {
        end();
        try {
          controller.close();
        } catch {
          // Cancelled by the client already.
        }
      },
    };
    const end = () => {
      if (!open) return;
      open = false;
      clearInterval(timer);
      if (this.#streams.delete(stream)) this.#notify();
    };
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        controller = c;
      },
      cancel() {
        end();
      },
    });
    request.signal.addEventListener(
      "abort",
      () => {
        stream.close();
      },
      { once: true },
    );
    const timer = setInterval(() => {
      if (open) controller.enqueue(encoder.encode(": heartbeat\n\n"));
    }, heartbeat);
    this.#streams.add(stream);
    this.#notify();
    return new Response(body, {
      status: 200,
      headers: { "Content-Type": "text/event-stream", "Cache-Control": "no-store" },
    });
  }

  async #createStatus(request: Request): Promise<Response> {
    const body = (await request.json()) as CreateStatusRequest;
    if ((body.visibility as string | undefined) === undefined)
      return problem(422, "about:blank", "visibility is required.");
    const key = request.headers.get("Idempotency-Key");
    const seen = key === null || key === "" ? undefined : this.#keys.get(key);
    if (seen !== undefined) return json(this.#posts[seen]?.status);
    const st = this.#newStatus(accountOf(this.#me), body.text, body.visibility);
    if (body.in_reply_to_id !== undefined) {
      st.in_reply_to_id = body.in_reply_to_id;
      const parent = this.#statuses.get(body.in_reply_to_id);
      if (parent !== undefined) st.in_reply_to_account_id = parent.account.id;
    }
    if (body.spoiler_text !== undefined) st.spoiler_text = body.spoiler_text;
    if (body.language !== undefined) st.language = body.language;
    this.#statuses.set(st.id, st);
    if (key !== null && key !== "") this.#keys.set(key, this.#posts.length);
    this.#posts.push({ idempotencyKey: key, request: body, status: st });
    this.#notify();
    return json(st);
  }

  async #reactToStatus(id: string, request: Request): Promise<Response> {
    const body = (await request.json()) as { emoji?: string };
    if (body.emoji === undefined || body.emoji === "") return problem(400, "about:blank", "emoji is required.");
    const st = this.#statuses.get(id);
    if (st === undefined) return problem(404, "about:blank", `No status ${id}.`);
    const emoji = body.emoji;
    if (!this.#reactions.some((r) => r.statusId === id && r.emoji === emoji)) {
      this.#reactions.push({ statusId: id, emoji });
      this.#notify();
    }
    return json(st);
  }

  #favouriteStatus(id: string): Response {
    const st = this.#statuses.get(id);
    if (st === undefined) return problem(404, "about:blank", `No status ${id}.`);
    if (!this.#favourites.includes(id)) {
      this.#favourites.push(id);
      this.#notify();
    }
    return json({ ...st, favourited: true });
  }

  #followAccount(id: string): Response {
    if (!this.#follows.includes(id)) {
      this.#follows.push(id);
      this.#notify();
    }
    const relationship: Relationship = {
      id,
      following: true,
      requested: false,
      followed_by: false,
      requested_by: false,
      show_boosts: true,
      notify: false,
      blocking: false,
      muting: false,
      muting_notifications: false,
    };
    return json(relationship);
  }
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
  });
}

/** An RFC 9457 problem document, as the server writes it. */
function problem(status: number, type: string, detail: string, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify({ type, title: statusText(status), status, detail }), {
    status,
    headers: { "Content-Type": "application/problem+json", "Cache-Control": "no-store", ...headers },
  });
}

function statusText(status: number): string {
  switch (status) {
    case 400:
      return "Bad Request";
    case 401:
      return "Unauthorized";
    case 404:
      return "Not Found";
    default:
      return "Unprocessable Entity";
  }
}
