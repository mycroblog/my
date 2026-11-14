// SPDX-License-Identifier: AGPL-3.0-only
// The browser's session with its server (ADR-0090 §16): the refresh token is the HttpOnly cookie __Host-my_rt, which
// only POST /oauth/token/refresh reads, and the access token it trades for lives in memory alone. The client API
// takes the access token as a Bearer token.
import { createClient, type TokenSource } from "@mycroblog/mykit";
import { createAppData, type AppData } from "./app-data";

/** The endpoint that trades the cookie for an access token, and the header it requires. */
const refreshPath = "/oauth/token/refresh";
const refreshHeader = "X-My-Token-Refresh";

/** How long before its expiry the access token counts as expired, so that it does not expire on the way. */
const expirySkewMs = 30_000;

/**
 * The access token of the browser's session. It trades the cookie for a token when it has none or the one it has
 * expires, or when the server refused it, once for every request that asks meanwhile. It answers null when the
 * browser has no session, or its session ended: the server refuses the cookie as an invalid grant.
 */
export class SessionTokenSource implements TokenSource {
  readonly #server: string;
  readonly #fetch: (request: Request) => Promise<Response>;
  readonly #now: () => number;
  #token: { accessToken: string; expiresAt: number } | null = null;
  #renewal: Promise<string | null> | null = null;

  constructor(server: string, fetch: (request: Request) => Promise<Response>, now: () => number = Date.now) {
    this.#server = server;
    this.#fetch = fetch;
    this.#now = now;
  }

  getAccessToken(): Promise<string | null> {
    if (this.#token !== null && this.#now() + expirySkewMs < this.#token.expiresAt)
      return Promise.resolve(this.#token.accessToken);
    return this.#renew();
  }

  refresh(refused: string): Promise<string | null> {
    if (this.#token !== null && this.#token.accessToken !== refused) return Promise.resolve(this.#token.accessToken);
    return this.#renew();
  }

  #renew(): Promise<string | null> {
    this.#renewal ??= this.#trade().finally(() => {
      this.#renewal = null;
    });
    return this.#renewal;
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
      return body.access_token;
    }
    this.#token = null;
    if (response.status === 400 && isError(body) && body.error === "invalid_grant") return null;
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

export type SessionOptions = {
  /** The origin of the server's client API (the shell's `apiOrigin`). */
  server: string;
  /** The reader's locale, sent as Accept-Language. */
  acceptLanguage?: () => string;
  /** Sends the requests; the global `fetch` by default. */
  fetch?: (request: Request) => Promise<Response>;
};

/**
 * Opens the browser's session: the data of the signed-in viewer, or null when the browser has no session. It rejects
 * when the server could not say (it did not answer, or answered with an error other than an invalid grant).
 */
export async function openSession({
  server,
  acceptLanguage,
  fetch = (request) => globalThis.fetch(request),
}: SessionOptions): Promise<AppData | null> {
  const tokenSource = new SessionTokenSource(server, fetch);
  if ((await tokenSource.getAccessToken()) === null) return null;
  return createAppData(
    createClient({ server, tokenSource, fetch, ...(acceptLanguage === undefined ? {} : { acceptLanguage }) }),
  );
}
