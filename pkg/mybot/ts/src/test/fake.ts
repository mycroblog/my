// SPDX-License-Identifier: MIT
// A fake server for the unit tests: the part of the client API that a bot uses, in memory, behind a fetch.
import { staticToken, type components } from "@mycroblog/mykit";
import type { BotOptions } from "../bot.js";
import type { Logger } from "../logger.js";
import { MemoryStore, type Store } from "../store.js";

type Notification = components["schemas"]["Notification"];
type Status = components["schemas"]["Status"];
type Account = components["schemas"]["Account"];
type CredentialAccount = components["schemas"]["CredentialAccount"];
type Visibility = components["schemas"]["Visibility"];
type CreateStatusRequest = components["schemas"]["CreateStatusRequest"];

export const server = "https://example.com";

/** A UUIDv7-shaped ID whose order is the order of n. */
export function id(n: number): string {
  return `01900000-0000-7000-8000-${String(n).padStart(12, "0")}`;
}

/** An account of another user. */
export function account(n: number, acct: string, bot = false): Account {
  return {
    id: id(n),
    username: acct.split("@")[0] ?? acct,
    acct,
    domain: acct.includes("@") ? (acct.split("@")[1] ?? "") : "example.com",
    uri: `${server}/users/${acct}`,
    display_name: "",
    note: "",
    url: `${server}/@${acct}`,
    locked: false,
    bot,
    created_at: new Date(0).toISOString(),
    fields: [],
    emojis: [],
    badges: [],
  };
}

/** A post of a with content. */
export function status(n: number, a: Account, content: string, visibility: Visibility): Status {
  return {
    id: id(n),
    uri: `${server}/statuses/${id(n)}`,
    account: a,
    content,
    spoiler_text: "",
    sensitive: false,
    visibility,
    language: "en",
    created_at: new Date().toISOString(),
    media_attachments: [],
    mentions: [
      { id: id(0), acct: "bot", username: "bot", url: `${server}/@bot` },
      { id: id(77), acct: "carol", username: "carol", url: `${server}/@carol` },
    ],
    tags: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
  };
}

/** A mention notification n from a, of a post with text and visibility v. */
export function mention(n: number, a: Account, text: string, v: Visibility = "public"): Notification {
  return {
    id: id(n),
    type: "mention",
    created_at: new Date().toISOString(),
    account: a,
    status: status(
      n + 500,
      a,
      `<p><span class="h-card"><a href="https://example.com/@bot" class="u-url mention">@<span>bot</span></a></span> ${text}</p>`,
      v,
    ),
  };
}

/** A logger that drops everything. */
export const quiet: Logger = { debug() {}, info() {}, warn() {}, error() {} };

/** A connection of the event stream, for FakeServer.stream. */
export type StreamConnection = {
  /** The number of the connection, from 1. */
  readonly n: number;
  /** Sends an event. */
  send(event: string, data: unknown): void;
  /** Ends the stream from the server's side. */
  close(): void;
  /** Resolves when the client leaves. */
  readonly left: Promise<void>;
};

/** A request of createStatus. */
export type Posted = { key: string | null; body: CreateStatusRequest };

export class FakeServer {
  me: CredentialAccount = { ...account(0, "bot", true), discoverable: false } as CredentialAccount;
  /** Newest first. */
  notifications: Notification[] = [];
  posts: Posted[] = [];
  reactions: string[] = [];
  listCalls = 0;
  streamCalls = 0;
  botUpdates: boolean[] = [];
  refreshTokens: string[] = [];
  /** Maps a valid refresh token to the one that replaces it. */
  tokens = new Map<string, string>();
  scope = "read write";
  /** Serves a connection of GET /streaming; absent keeps it open until the client leaves. */
  stream: ((c: StreamConnection) => void | Promise<void>) | undefined;
  /** Runs before the n-th call (from 1) of listNotifications, which fails when it resolves to false. */
  list: ((n: number) => boolean | Promise<boolean>) | undefined;
  /** The User-Agent of every request. */
  userAgents: string[] = [];

  /** Puts a notification at the top of the list. */
  add(n: Notification): void {
    this.notifications.unshift(n);
  }

  /** The options of a bot of this server, in the polling mode with a MemoryStore. */
  options(extra: Partial<BotOptions> = {}): BotOptions & { store: Store } {
    return {
      server,
      fetch: this.fetch,
      tokenSource: staticToken("token"),
      store: new MemoryStore(),
      logger: quiet,
      mode: "polling",
      pollInterval: 5,
      ...extra,
    };
  }

  readonly fetch = async (request: Request): Promise<Response> => {
    this.userAgents.push(request.headers.get("User-Agent") ?? "");
    const url = new URL(request.url);
    const route = `${request.method} ${url.pathname}`;
    if (route === "GET /api/v1/accounts/verify_credentials") return json(this.me);
    if (route === "PATCH /api/v1/accounts/update_credentials") {
      const body = (await request.json()) as { bot?: boolean };
      if (body.bot !== undefined) {
        this.botUpdates.push(body.bot);
        this.me = { ...this.me, bot: body.bot };
      }
      return json(this.me);
    }
    if (route === "GET /api/v1/notifications") {
      this.listCalls++;
      if (this.list !== undefined && !(await this.list(this.listCalls))) {
        return json({ type: "about:blank", title: "failed", status: 500 }, 500, "application/problem+json");
      }
      const start = Number(url.searchParams.get("cursor") ?? "0");
      // Small pages, so that the catch-up follows the cursor.
      const end = Math.min(start + 2, this.notifications.length);
      return json({ items: this.notifications.slice(start, end), ...(end < this.notifications.length ? { next_cursor: String(end) } : {}) });
    }
    if (route === "POST /api/v1/statuses") {
      const body = (await request.json()) as CreateStatusRequest;
      this.posts.push({ key: request.headers.get("Idempotency-Key"), body });
      return json(status(9000 + this.posts.length, this.me, body.text, body.visibility));
    }
    const reaction = /^PUT \/api\/v1\/statuses\/([^/]+)\/reaction$/.exec(route);
    if (reaction !== null) {
      const body = (await request.json()) as { emoji: string };
      this.reactions.push(`${reaction[1] ?? ""} ${body.emoji}`);
      return json({ id: reaction[1] });
    }
    if (route === "GET /api/v1/streaming") return this.#streaming(request);
    if (route === "POST /oauth/token") {
      const form = new URLSearchParams(await request.text());
      const rt = form.get("refresh_token") ?? "";
      this.refreshTokens.push(rt);
      const next = this.tokens.get(rt);
      if (next === undefined) return json({ error: "invalid_grant" }, 400);
      this.tokens.delete(rt);
      this.tokens.set(next, `${next}'`);
      return json({ access_token: `at-${next}`, token_type: "Bearer", expires_in: 3600, refresh_token: next, scope: this.scope });
    }
    return json({ type: "about:blank", title: "not found", status: 404 }, 404, "application/problem+json");
  };

  #streaming(request: Request): Response {
    this.streamCalls++;
    const n = this.streamCalls;
    const encoder = new TextEncoder();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    let closed = false;
    let leave!: () => void;
    const left = new Promise<void>((resolve) => {
      leave = resolve;
    });
    const close = () => {
      if (closed) return;
      closed = true;
      try {
        controller.close();
      } catch {
        // Cancelled by the client.
      }
    };
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        controller = c;
      },
      cancel() {
        closed = true;
        leave();
      },
    });
    request.signal.addEventListener("abort", () => {
      close();
      leave();
    });
    const connection: StreamConnection = {
      n,
      send(event, data) {
        if (!closed) controller.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
      },
      close,
      left,
    };
    void Promise.resolve(this.stream?.(connection)).catch(() => undefined);
    return new Response(body, { status: 200, headers: { "Content-Type": "text/event-stream" } });
  }
}

function json(body: unknown, status = 200, type = "application/json"): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": type } });
}

/** Waits for cond, failing after a few seconds. */
export async function eventually(what: string, cond: () => boolean | Promise<boolean>): Promise<void> {
  const deadline = Date.now() + 5000;
  while (!(await cond())) {
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
}

/** Whether store has recorded every one of ids. */
export async function processed(store: Store, ...ids: string[]): Promise<boolean> {
  for (const i of ids) if (!(await store.isProcessed(i))) return false;
  return true;
}

/** Runs a bot until the returned stop is called, which resolves to what run did. */
export function start(bot: { run(o: { signal: AbortSignal }): Promise<void> }): () => Promise<void> {
  const controller = new AbortController();
  const done = bot.run({ signal: controller.signal });
  // Keep a rejection for stop.
  done.catch(() => undefined);
  return async () => {
    controller.abort();
    await done;
  };
}

/** Waits for ms. */
export function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
