// SPDX-License-Identifier: AGPL-3.0-only
// The web client's OAuth session (ADR-0019 §4, ADR-0030). The access token lives only in this object's
// memory: never in localStorage, sessionStorage, IndexedDB or a readable cookie. The refresh token is in
// the HttpOnly __Host-my_rt cookie, which only POST /oauth/token/refresh reads; that endpoint returns a
// new access token and rotates the cookie.
//
// The server rotates refresh tokens strictly and revokes the whole token family when one is reused
// (RFC 9700 §4.14.2), so two tabs must never send the same cookie at once. Every refresh therefore runs
// under one Web Lock of the origin (navigator.locks, shared by every tab), and the tab that refreshed
// posts the new access token on a BroadcastChannel. A tab that waited for the lock uses the token that
// arrived meanwhile; if none did, its own refresh sends the cookie that the previous one rotated in.
//
// The server's maintenance mode answers the OAuth endpoints with the 503 of the maintenance problem (ADR-0084 §3). The
// session survives it: a refresh that meets it keeps the token and the cookie, throws the MaintenanceError, and says
// nothing to the other tabs, as when the server cannot be reached.
import { maintenanceFromResponse, NetworkError } from "../errors";
import type { Maintenance } from "../maintenance";

/** The client_id of the first-party web client (identity.DefaultWebClientID on the server). */
export const webClientId = "my-web";

/** The Web Lock every tab of the origin takes around a refresh. */
const refreshLockName = "my:oauth-refresh";

/** The BroadcastChannel on which tabs share the session. */
const sessionChannelName = "my:oauth-session";

/** The OAuth endpoints of the server (internal/interface/oauth). */
const paths = {
  token: "/oauth/token",
  refresh: "/oauth/token/refresh",
  revoke: "/oauth/revoke",
} as const;

/** Whether this tab knows of a signed-in user. `unknown` until the first refresh has answered. */
export type SessionStatus = "unknown" | "signed-in" | "signed-out";

/** What one tab tells the others. */
type SessionMessage =
  /** A refresh: the same user with a new access token. */
  | { type: "refreshed"; accessToken: string }
  /** A sign-in, possibly as another user. */
  | { type: "signed-in"; accessToken: string }
  /** A sign-out, or a session that the server ended. */
  | { type: "signed-out" };

function isSessionMessage(value: unknown): value is SessionMessage {
  if (typeof value !== "object" || value === null) return false;
  const { type, accessToken } = value as Record<string, unknown>;
  if (type === "signed-out") return true;
  return (type === "refreshed" || type === "signed-in") && typeof accessToken === "string";
}

/** The part of the Web Locks API the session uses. */
export type Locks = Pick<LockManager, "request">;

/** The part of a BroadcastChannel the session uses. */
export type Channel = Pick<BroadcastChannel, "postMessage" | "close" | "addEventListener" | "removeEventListener">;

export type SessionOptions = {
  /** The origin of the server's client API and OAuth endpoints (the runtime config's apiOrigin). */
  apiOrigin: string;
  /** The origin's lock manager; navigator.locks by default. Without one, refreshes queue within the tab only. */
  locks?: Locks | undefined;
  /** Opens the channel shared with the other tabs; a BroadcastChannel by default. */
  openChannel?: ((name: string) => Channel) | undefined;
  /** Where the OAuth endpoints' maintenance problem is reported (lib/maintenance.ts). */
  maintenance?: Pick<Maintenance, "report"> | undefined;
};

/** Why a sign-out did not reach the server: this tab is signed out, but the server may keep the session. */
export class SignOutError extends Error {
  override name = "SignOutError";
}

/** The token response of the OAuth endpoints (RFC 6749 §5.1); the web client never gets a refresh token. */
type TokenResponse = { access_token: string; token_type: string; expires_in: number };

function isTokenResponse(value: unknown): value is TokenResponse {
  if (typeof value !== "object" || value === null) return false;
  const { access_token, token_type } = value as Record<string, unknown>;
  return typeof access_token === "string" && access_token !== "" && token_type === "Bearer";
}

/**
 * The access token of `response`, or null when the server refused the grant (a 4xx). Throws a MaintenanceError for the
 * maintenance problem (after reporting it to `maintenance`) and a NetworkError for any other failure.
 */
async function readTokenResponse(response: Response, maintenance: Pick<Maintenance, "report"> | undefined): Promise<string | null> {
  if (response.status >= 400 && response.status < 500) return null;
  const underMaintenance = await maintenanceFromResponse(response);
  if (underMaintenance !== null) {
    maintenance?.report(underMaintenance);
    throw underMaintenance;
  }
  if (!response.ok) throw new NetworkError(`${response.url}: ${response.status}`);
  const body: unknown = await response.json();
  if (!isTokenResponse(body)) throw new NetworkError(`${response.url}: not a token response`);
  return body.access_token;
}

/** A queue for tabs without the Web Locks API: refreshes in this tab still run one at a time. */
function localLocks(): Locks {
  let tail: Promise<unknown> = Promise.resolve();
  return {
    request: ((name: string, callback: (lock: Lock | null) => unknown) => {
      const run = tail.then(() => callback({ name, mode: "exclusive" }));
      tail = run.catch(() => undefined);
      return run;
    }) as Locks["request"],
  };
}

export class Session {
  readonly #apiOrigin: string;
  readonly #locks: Locks;
  readonly #channel: Channel | null;
  readonly #maintenance: Pick<Maintenance, "report"> | undefined;
  #accessToken: string | null = null;
  #status: SessionStatus = "unknown";
  /** Whether resuming the session failed because the server could not be reached (see `unreachable`). */
  #unreachable = false;
  /** The refresh this tab is running or waiting to run; concurrent callers share it. */
  #refreshing: Promise<string | null> | null = null;
  readonly #listeners = new Set<() => void>();
  readonly #userListeners = new Set<(status: SessionStatus) => void>();

  constructor({ apiOrigin, locks, openChannel, maintenance }: SessionOptions) {
    this.#apiOrigin = apiOrigin;
    this.#maintenance = maintenance;
    const navigatorLocks = typeof navigator === "undefined" ? undefined : (navigator.locks as LockManager | undefined);
    this.#locks = locks ?? navigatorLocks ?? localLocks();
    const open = openChannel ?? (typeof BroadcastChannel === "undefined" ? undefined : (name: string) => new BroadcastChannel(name));
    this.#channel = open === undefined ? null : open(sessionChannelName);
    this.#channel?.addEventListener("message", this.#onMessage);
  }

  /** The access token for the Authorization header, or null when there is none. */
  get accessToken(): string | null {
    return this.#accessToken;
  }

  get status(): SessionStatus {
    return this.#status;
  }

  /**
   * Whether resuming the session failed because the server could not be reached (a page opened offline):
   * the tab acts signed out, but the session may still be there, so the next restore() tries again. A page
   * may show the user's data saved on this browser meanwhile (the home timeline, web-client/11-pwa.md §11).
   */
  get unreachable(): boolean {
    return this.#unreachable;
  }

  /** Calls `listener` whenever the status or `unreachable` changes (the subscribe of useSyncExternalStore). */
  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  /**
   * Calls `listener` when the user changes: a sign-in, or the end of a session this tab had, in this tab
   * or another. Data cached for the previous user must go then. Resuming at start-up is no change.
   */
  onUserChange(listener: (status: SessionStatus) => void): () => void {
    this.#userListeners.add(listener);
    return () => this.#userListeners.delete(listener);
  }

  /**
   * Resumes the session from the refresh token cookie when the status is still unknown, and again while the
   * server could not be reached (`unreachable`).
   */
  async restore(): Promise<void> {
    if (this.#status !== "unknown" && !this.#unreachable) return;
    try {
      await this.refresh(null);
    } catch (error) {
      // The server is unreachable or under maintenance; without a token this tab acts signed out until a later
      // refresh, which the cookie still allows.
      console.error(error);
      // A tab may have shared a token meanwhile.
      if (this.#status === "unknown") {
        this.#unreachable = true;
        this.#setStatus("signed-out");
      }
    }
  }

  /**
   * Gets a new access token after `stale` stopped working (null: there was none), and returns it, or null
   * when the session has ended. Concurrent calls in this tab share one refresh, and tabs take turns
   * under the Web Lock. Throws a NetworkError when the server cannot be reached, and a MaintenanceError while it is
   * under maintenance; neither ends the session.
   */
  refresh(stale: string | null): Promise<string | null> {
    this.#refreshing ??= this.#locks
      .request(refreshLockName, async () => {
        // Another tab refreshed while this one waited for the lock, and its token has arrived.
        if (this.#accessToken !== null && this.#accessToken !== stale) return this.#accessToken;
        return this.#refreshNow();
      })
      .finally(() => {
        this.#refreshing = null;
      });
    return this.#refreshing;
  }

  async #refreshNow(): Promise<string | null> {
    let response: Response;
    try {
      response = await fetch(new URL(paths.refresh, this.#apiOrigin), {
        method: "POST",
        // The endpoint requires this header, which no cross-site form can send (oauth.HeaderRefresh).
        headers: { "X-My-Token-Refresh": "1" },
        credentials: "include",
        cache: "no-store",
      });
    } catch (error) {
      throw new NetworkError("the token refresh failed", { cause: error });
    }
    const token = await readTokenResponse(response, this.#maintenance);
    if (token === null) {
      // The cookie is missing, expired or revoked. If this tab had a session, every tab has lost it. A tab
      // that could not reach the server may have shown the user's saved data meanwhile, which goes too.
      const ended = this.#status === "signed-in" || this.#unreachable;
      this.#signedOut();
      if (ended) this.#post({ type: "signed-out" });
      return null;
    }
    const wasSignedOut = this.#status === "signed-out";
    this.#unreachable = false;
    this.#accessToken = token;
    this.#setStatus("signed-in");
    if (wasSignedOut) this.#emitUserChange();
    this.#post({ type: "refreshed", accessToken: token });
    return token;
  }

  /**
   * Exchanges an authorization code for an access token (RFC 6749 §4.1.3, RFC 7636 §4.5). The response
   * sets the refresh token cookie. Returns false when the server refuses the code.
   */
  exchangeCode(code: { code: string; redirectUri: string; codeVerifier: string }): Promise<boolean> {
    // Under the refresh lock: a refresh that resumes the session (main.tsx) must not answer "no cookie"
    // after the exchange has set it, and one that runs after the exchange finds the new token.
    return this.#locks.request(refreshLockName, () => this.#exchangeCodeNow(code));
  }

  async #exchangeCodeNow({ code, redirectUri, codeVerifier }: { code: string; redirectUri: string; codeVerifier: string }): Promise<boolean> {
    let response: Response;
    try {
      response = await fetch(new URL(paths.token, this.#apiOrigin), {
        method: "POST",
        body: new URLSearchParams({ grant_type: "authorization_code", code, redirect_uri: redirectUri, client_id: webClientId, code_verifier: codeVerifier }),
        credentials: "include",
        cache: "no-store",
      });
    } catch (error) {
      throw new NetworkError("the authorization code exchange failed", { cause: error });
    }
    const token = await readTokenResponse(response, this.#maintenance);
    if (token === null) return false;
    this.#unreachable = false;
    this.#accessToken = token;
    this.#setStatus("signed-in");
    this.#emitUserChange();
    this.#post({ type: "signed-in", accessToken: token });
    return true;
  }

  /**
   * Signs out in every tab: revokes the token family on the server (which also deletes the cookie), forgets
   * the access token and tells the other tabs. This tab is signed out even when the server cannot be
   * reached; a SignOutError then says that the server may keep the session.
   */
  async signOut(): Promise<void> {
    let token = this.#accessToken;
    let failure: unknown = null;
    if (token === null && this.#status !== "signed-out") {
      // Revoking needs a token of the family; the cookie gives one.
      token = await this.refresh(null).catch((error: unknown) => {
        failure = error;
        return null;
      });
    }
    if (token !== null) {
      try {
        const response = await fetch(new URL(paths.revoke, this.#apiOrigin), {
          method: "POST",
          body: new URLSearchParams({ token, token_type_hint: "access_token", client_id: webClientId }),
          credentials: "include",
          cache: "no-store",
        });
        if (!response.ok) failure = new Error(`${response.url}: ${response.status}`);
      } catch (error) {
        failure = error;
      }
    }
    this.#signedOut();
    this.#post({ type: "signed-out" });
    if (failure !== null) throw new SignOutError("the server did not revoke the session", { cause: failure });
  }

  /** Stops listening to the other tabs. */
  dispose(): void {
    this.#channel?.removeEventListener("message", this.#onMessage);
    this.#channel?.close();
  }

  readonly #onMessage = (event: Event): void => {
    const message: unknown = (event as MessageEvent).data;
    if (!isSessionMessage(message)) return;
    switch (message.type) {
      case "refreshed":
      case "signed-in": {
        const changed = message.type === "signed-in" || this.#status === "signed-out";
        this.#unreachable = false;
        this.#accessToken = message.accessToken;
        this.#setStatus("signed-in");
        if (changed) this.#emitUserChange();
        return;
      }
      case "signed-out":
        this.#signedOut();
        return;
    }
  };

  #signedOut(): void {
    const wasUnreachable = this.#unreachable;
    const changed = this.#status === "signed-in" || wasUnreachable;
    this.#unreachable = false;
    this.#accessToken = null;
    this.#setStatus("signed-out");
    // The status stays signed out, but the tab no longer waits for the server.
    if (wasUnreachable) this.#notify();
    if (changed) this.#emitUserChange();
  }

  #setStatus(status: SessionStatus): void {
    if (this.#status === status) return;
    this.#status = status;
    this.#notify();
  }

  #notify(): void {
    for (const listener of this.#listeners) listener();
  }

  #emitUserChange(): void {
    for (const listener of this.#userListeners) listener(this.#status);
  }

  #post(message: SessionMessage): void {
    this.#channel?.postMessage(message);
  }
}
