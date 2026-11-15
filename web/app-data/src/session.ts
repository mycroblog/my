// SPDX-License-Identifier: AGPL-3.0-only
// The browser's session with its server (ADR-0090 §16): the refresh token is the HttpOnly cookie __Host-my_rt, which
// only POST /oauth/token/refresh reads, and the access token it trades for lives in memory alone. The client API
// takes the access token as a Bearer token.
//
// The tabs of the browser share the session: a refresh rotates the cookie and revokes the access token it replaces,
// and two refreshes with one cookie at once revoke the whole session as a reuse. So the tabs form a group of
// @my/platform (ADR-0028 §18): one tab at a time holds its lock to renew the token, first asks the other tabs for one,
// trades the cookie only when none has a usable one, and passes the new token to every tab. The token goes over the
// channel from memory to memory, never to storage. A browser without the group renews the token in each tab.
import { createClient, type TokenSource } from "@mycroblog/mykit";
import { joinTabGroup, type TabGroup } from "@my/platform";
import { createAppData, type AppData } from "./app-data";
import type { ServerWatch } from "./server";

/** The endpoint that trades the cookie for an access token, and the header it requires. */
const refreshPath = "/oauth/token/refresh";
const refreshHeader = "X-My-Token-Refresh";

/** How long before its expiry the access token counts as expired, so that it does not expire on the way. */
const expirySkewMs = 30_000;

/** How long a tab that renews the token waits for another tab to answer with one before it trades the cookie. */
const askTimeoutMs = 100;

type Token = { readonly accessToken: string; readonly expiresAt: number };

/**
 * What the tabs of a session say to each other: a token to use, a question for one, or the end of the session (the
 * server refused the cookie).
 */
type SessionMessage = ({ readonly kind: "token" } & Token) | { readonly kind: "ask" } | { readonly kind: "ended" };

export type SessionTokenSourceOptions = {
  /** The clock, in milliseconds since the Unix epoch; Date.now by default. */
  readonly now?: () => number;
  /** The tabs that share the session; null, the default, renews the token in this tab alone. */
  readonly tabs?: TabGroup | null;
};

/**
 * The access token of the browser's session. It renews the token when it has none or the one it has expires, or when
 * the server refused it, once for every request that asks meanwhile. It answers null when the browser has no session,
 * or its session ended: the server refuses the cookie as an invalid grant.
 */
export class SessionTokenSource implements TokenSource {
  readonly #server: string;
  readonly #fetch: (request: Request) => Promise<Response>;
  readonly #now: () => number;
  readonly #tabs: TabGroup | null;
  #token: Token | null = null;
  #renewal: Promise<string | null> | null = null;
  /** Called with each token another tab passes while this tab waits for an answer to its question. */
  #answered: (() => void) | null = null;

  constructor(
    server: string,
    fetch: (request: Request) => Promise<Response>,
    { now = Date.now, tabs = null }: SessionTokenSourceOptions = {},
  ) {
    this.#server = server;
    this.#fetch = fetch;
    this.#now = now;
    this.#tabs = tabs;
    tabs?.listen((message) => {
      if (isSessionMessage(message)) this.#receive(message);
    });
  }

  getAccessToken(): Promise<string | null> {
    return this.#usableOrRenew(null);
  }

  refresh(refused: string): Promise<string | null> {
    return this.#usableOrRenew(refused);
  }

  #usableOrRenew(refused: string | null): Promise<string | null> {
    const token = this.#usable(refused);
    return token === null ? this.#renew(refused) : Promise.resolve(token);
  }

  /** The token this tab holds, unless it is about to expire or is the one the server refused. */
  #usable(refused: string | null): string | null {
    const token = this.#token;
    if (token === null || token.accessToken === refused || this.#now() + expirySkewMs >= token.expiresAt) return null;
    return token.accessToken;
  }

  #renew(refused: string | null): Promise<string | null> {
    this.#renewal ??= this.#renewInGroup(refused).finally(() => {
      this.#renewal = null;
    });
    return this.#renewal;
  }

  /**
   * Renews the token under the group's lock: the token another tab passed meanwhile, the one another tab answers with,
   * or the one the cookie trades for. The lock is held until the other tabs have the new token, so that two tabs
   * never trade one cookie.
   */
  async #renewInGroup(refused: string | null): Promise<string | null> {
    const tabs = this.#tabs;
    if (tabs === null) return this.#trade();
    let renewed: string | null = null;
    try {
      await tabs.lead(new AbortController().signal, async () => {
        renewed = this.#usable(refused) ?? (await this.#ask(tabs, refused)) ?? (await this.#trade());
      });
    } catch {
      // The browser refused the lock (a document without storage access): this tab trades the cookie alone.
      return this.#trade();
    }
    return renewed;
  }

  /**
   * Asks the other tabs for a token: a tab that has just opened has none yet, and the token of the last renewal may
   * reach this tab after its lock does. It answers the first usable token, or null when none comes in time.
   */
  #ask(tabs: TabGroup, refused: string | null): Promise<string | null> {
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        this.#answered = null;
        resolve(null);
      }, askTimeoutMs);
      this.#answered = () => {
        const token = this.#usable(refused);
        if (token === null) return;
        clearTimeout(timer);
        this.#answered = null;
        resolve(token);
      };
      tabs.post({ kind: "ask" } satisfies SessionMessage);
    });
  }

  #receive(message: SessionMessage): void {
    switch (message.kind) {
      case "token":
        // Tokens may arrive out of order; the one that expires last is the newest.
        if (this.#token === null || this.#token.expiresAt < message.expiresAt)
          this.#token = { accessToken: message.accessToken, expiresAt: message.expiresAt };
        this.#answered?.();
        break;
      case "ask":
        if (this.#token !== null && this.#usable(null) !== null)
          this.#tabs?.post({ kind: "token", ...this.#token } satisfies SessionMessage);
        break;
      case "ended":
        this.#token = null;
        break;
    }
  }

  async #trade(): Promise<string | null> {
    const response = await this.#fetch(
      new Request(this.#server + refreshPath, {
        method: "POST",
        credentials: "same-origin",
        headers: { [refreshHeader]: "1" },
      }),
    );
    const body: unknown = await response.json().catch(() => null);
    if (response.ok && isTokens(body)) {
      this.#token = { accessToken: body.access_token, expiresAt: this.#now() + body.expires_in * 1000 };
      this.#tabs?.post({ kind: "token", ...this.#token } satisfies SessionMessage);
      return body.access_token;
    }
    this.#token = null;
    if (response.status === 400 && isError(body) && body.error === "invalid_grant") {
      this.#tabs?.post({ kind: "ended" } satisfies SessionMessage);
      return null;
    }
    throw new Error(`app-data: ${refreshPath} answered ${String(response.status)}`);
  }
}

const isTokens = (body: unknown): body is { access_token: string; expires_in: number } =>
  typeof body === "object" &&
  body !== null &&
  "access_token" in body &&
  typeof body.access_token === "string" &&
  "expires_in" in body &&
  typeof body.expires_in === "number";

const isError = (body: unknown): body is { error: string } =>
  typeof body === "object" && body !== null && "error" in body && typeof body.error === "string";

/** Whether a message from another tab, which may run another version of the client (ADR-0028 §14), is one it reads. */
function isSessionMessage(message: unknown): message is SessionMessage {
  if (typeof message !== "object" || message === null || !("kind" in message)) return false;
  if (message.kind === "ask" || message.kind === "ended") return true;
  return (
    message.kind === "token" &&
    "accessToken" in message &&
    typeof message.accessToken === "string" &&
    "expiresAt" in message &&
    typeof message.expiresAt === "number"
  );
}

export type SessionOptions = {
  /** The origin of the server's client API (the shell's `apiOrigin`). */
  server: string;
  /** The reader's locale, sent as Accept-Language. */
  acceptLanguage?: () => string;
  /** Sends the requests; the global `fetch` by default. */
  fetch?: (request: Request) => Promise<Response>;
  /** The tabs that share the session: the browser's tab group by default; null renews the token in this tab alone. */
  tabs?: TabGroup | null;
  /** The watch of the server, whose `fetch` sends the requests in place of `fetch`. */
  watch?: ServerWatch;
};

/**
 * Opens the browser's session: the data of the signed-in viewer, or null when the browser has no session. It rejects
 * when the server could not say (it did not answer, or answered with an error other than an invalid grant).
 */
export async function openSession({
  server,
  acceptLanguage,
  fetch: given = (request) => globalThis.fetch(request),
  tabs = joinTabGroup(`my:session:${server}`),
  watch,
}: SessionOptions): Promise<AppData | null> {
  const fetch = watch?.fetch ?? given;
  const tokenSource = new SessionTokenSource(server, fetch, { tabs });
  if ((await tokenSource.getAccessToken()) === null) return null;
  const connection = { server, tokenSource, fetch, ...(acceptLanguage === undefined ? {} : { acceptLanguage }) };
  return { ...createAppData(createClient(connection), undefined, watch), connection };
}

/**
 * The data of a reader without a session, for what the server shows everyone (the instance, its public pages): the
 * client API without a token, through the watch of the server.
 */
export function visitorData({
  server,
  acceptLanguage,
  watch,
}: Pick<SessionOptions, "server" | "acceptLanguage" | "watch">): AppData {
  const fetch = watch?.fetch ?? ((request: Request) => globalThis.fetch(request));
  return createAppData(
    createClient({ server, fetch, ...(acceptLanguage === undefined ? {} : { acceptLanguage }) }),
    undefined,
    watch,
  );
}
