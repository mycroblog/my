// SPDX-License-Identifier: MIT
import { type OAuthConfig, tokenRequest } from "./oauth.js";

/** How long before its expiry a token counts as expired, so that it does not expire on the way. */
const expirySkewMs = 30_000;

/** An OAuth 2.0 access token and the refresh token that renews it. */
export type Token = {
  accessToken: string;
  /**
   * Renews the access token. The server rotates it on every use (RFC 9700 §4.14): the previous one stops working, so
   * the new one must be saved.
   */
  refreshToken?: string;
  /** When the access token expires, in milliseconds since the Unix epoch; absent means unknown. */
  expiresAt?: number;
  /** The space-separated scopes of the token. */
  scope?: string;
};

/** Whether the access token is set and not about to expire at now. */
function valid(token: Token | null, now: number): token is Token {
  return token !== null && token.accessToken !== "" && (token.expiresAt === undefined || now + expirySkewMs < token.expiresAt);
}

/**
 * Gives the access token of a request; null sends it without one. The library never stores tokens itself.
 *
 * `refresh`, when present, renews the token after the server refused it with a 401: the client calls it once per
 * request, with the access token the server refused, and sends the request again with the new one. Null leaves the
 * 401 as it is.
 */
export interface TokenSource {
  getAccessToken(): Promise<string | null>;
  refresh?(refused: string): Promise<string | null>;
}

/**
 * A TokenSource of one access token that is never renewed, for tests and short tasks. Use a RefreshingTokenSource
 * for anything that runs longer than the token lives (an hour).
 */
export function staticToken(accessToken: string): TokenSource {
  return {
    getAccessToken: () => Promise.resolve(accessToken),
  };
}

/** Receives the tokens of every renewal; it must save the rotated refresh token. */
export type OnTokens = (token: Token) => void | Promise<void>;

/**
 * The TokenSource of a client that holds a refresh token: it renews the access token with the refresh_token grant
 * when it expires or the server refuses it. Renewals are serialized, so that concurrent requests that meet a 401
 * renew it once: a used refresh token sent again revokes the whole token family. Every renewal passes the new tokens
 * to `onTokens`; when it throws, the new tokens are kept in the source, the request that needed them fails with its
 * error, and every later request passes them to `onTokens` again before it uses them, until `onTokens` saves them.
 * The source never hands out tokens that `onTokens` has not saved, so an app that restarts after a failed save does
 * not lose the only valid refresh token while requests went on with it.
 */
export class RefreshingTokenSource implements TokenSource {
  readonly #oauth: OAuthConfig;
  readonly #onTokens: OnTokens | undefined;
  readonly #now: () => number;
  #token: Token | null;
  /** The renewal in flight, which the requests that need a token meanwhile wait for. */
  #renewal: Promise<Token> | null = null;
  /** Whether `onTokens` has not saved #token, the tokens of the last renewal. */
  #unsaved = false;
  /** The save in flight, which the requests that need a token meanwhile wait for. */
  #saving: Promise<void> | null = null;

  /**
   * Starts from `token` (from `exchange`, or saved before) and renews it through the token endpoint of `oauth`.
   * `now` is for tests.
   */
  constructor(oauth: OAuthConfig, token: Token | null, onTokens?: OnTokens, now: () => number = Date.now) {
    this.#oauth = oauth;
    this.#token = token;
    this.#onTokens = onTokens;
    this.#now = now;
  }

  /** The current tokens, or null before the first. */
  get token(): Token | null {
    return this.#token;
  }

  /** The current access token, renewed first when it has expired. */
  async getAccessToken(): Promise<string | null> {
    if (this.#renewal !== null) return (await this.#renewal).accessToken;
    await this.#save();
    if (valid(this.#token, this.#now())) return this.#token.accessToken;
    return (await this.#renew()).accessToken;
  }

  /**
   * Renews the token after the server refused `refused`. When another request renewed it since `refused` was handed
   * out, it returns that token instead of renewing again.
   */
  async refresh(refused: string): Promise<string | null> {
    if (this.#renewal !== null) return (await this.#renewal).accessToken;
    await this.#save();
    if (valid(this.#token, this.#now()) && this.#token.accessToken !== refused) return this.#token.accessToken;
    return (await this.#renew()).accessToken;
  }

  /** Runs the refresh_token grant, once for every caller that asks while it runs. */
  #renew(): Promise<Token> {
    this.#renewal ??= this.#grant().finally(() => {
      this.#renewal = null;
    });
    return this.#renewal;
  }

  async #grant(): Promise<Token> {
    const current = this.#token;
    if (current?.refreshToken === undefined || current.refreshToken === "") {
      throw new Error("mykit: the token has expired and there is no refresh token");
    }
    const token = await tokenRequest(this.#oauth, { grant_type: "refresh_token", refresh_token: current.refreshToken });
    token.refreshToken ??= current.refreshToken;
    this.#token = token;
    this.#unsaved = true;
    await this.#save();
    return token;
  }

  /** Passes the tokens of the last renewal to `onTokens` when it has not saved them yet, once for every caller. */
  #save(): Promise<void> {
    if (!this.#unsaved) return Promise.resolve();
    this.#saving ??= this.#persist().finally(() => {
      this.#saving = null;
    });
    return this.#saving;
  }

  async #persist(): Promise<void> {
    if (this.#token !== null) await this.#onTokens?.(this.#token);
    this.#unsaved = false;
  }
}
