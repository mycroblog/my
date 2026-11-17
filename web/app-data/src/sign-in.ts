// SPDX-License-Identifier: AGPL-3.0-only
// The web client's sign-in (ADR-0090 §16): the authorization code flow with PKCE (RFC 7636) as the first-party client.
// The start keeps a state and a code verifier for the browser's tab and sends it to the authorization endpoint, where
// the server's auth screens sign the person in; the server sends it back to the callback with a code, which the
// token endpoint trades for the session: it sets the refresh token as the HttpOnly cookie __Host-my_rt and answers an
// access token, which the next page load gets again from the cookie (session.ts). The state and the issuer (RFC
// 9207) of the answer must be the ones of the start, so that no other site's answer is taken.
import { codeChallenge, exchange, newCodeVerifier } from "@mycroblog/mykit";

/** The path of the authorization endpoint on the server. */
const authorizePath = "/oauth/authorize";

/** The key under which the start keeps what the callback checks. */
const pendingKey = "my.sign-in";

/** Where the sign-in keeps the start for the callback: the tab's own storage (sessionStorage). */
export type SignInStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export type SignInOptions = {
  /** The origin of the server: its authorization server and the issuer of its answers. */
  server: string;
  /** The web client's client_id (the shell's `clientId`). */
  clientId: string;
  /** The web client's callback, a redirect URI the server registered for it. */
  redirectUri: string;
  storage: SignInStorage;
  /** Sends the token request; the global `fetch` by default. */
  fetch?: (request: Request) => Promise<Response>;
};

/** What the start kept for the callback. */
type Pending = { state: string; verifier: string; returnTo: string };

/**
 * Why a callback did not sign in:
 * - `stale`: the tab kept no start, or one of another sign-in (an old link, a second tab);
 * - `issuer`: the answer names another authorization server;
 * - `denied`: the server answered an error instead of a code;
 * - `exchange`: the token endpoint refused the code.
 */
export type SignInFailure = "stale" | "issuer" | "denied" | "exchange";

export class SignInError extends Error {
  readonly reason: SignInFailure;
  constructor(reason: SignInFailure, options?: { cause?: unknown }) {
    super(`sign-in: ${reason}`, options);
    this.name = "SignInError";
    this.reason = reason;
  }
}

/** A path on the app's own origin to go back to after the sign-in, "/" for anything else. */
export function appPath(returnTo: string | null | undefined): string {
  if (!returnTo?.startsWith("/") || returnTo.startsWith("//") || returnTo.startsWith("/\\")) return "/";
  return returnTo;
}

/**
 * Starts a sign-in that comes back to `returnTo`: it keeps a new state and code verifier in `storage`, in place of any
 * earlier start, and returns the URL of the authorization request to open.
 */
export async function startSignIn(o: SignInOptions, returnTo?: string | null): Promise<string> {
  const pending: Pending = { state: newCodeVerifier(), verifier: newCodeVerifier(), returnTo: appPath(returnTo) };
  o.storage.setItem(pendingKey, JSON.stringify(pending));
  const url = new URL(authorizePath, o.server);
  url.search = new URLSearchParams({
    response_type: "code",
    client_id: o.clientId,
    redirect_uri: o.redirectUri,
    state: pending.state,
    code_challenge: await codeChallenge(pending.verifier),
    code_challenge_method: "S256",
  }).toString();
  return url.href;
}

/** The start that `storage` keeps, which it gives up: a callback is taken once. */
function takePending(storage: SignInStorage): Pending | undefined {
  const kept = storage.getItem(pendingKey);
  storage.removeItem(pendingKey);
  if (kept === null) return undefined;
  try {
    const value: unknown = JSON.parse(kept);
    if (typeof value !== "object" || value === null) return undefined;
    const { state, verifier, returnTo } = value as Record<string, unknown>;
    return typeof state === "string" && typeof verifier === "string" && typeof returnTo === "string"
      ? { state, verifier, returnTo }
      : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Finishes the sign-in at the callback whose address is `callback`: it checks the answer against the start, trades
 * the code for the session, and returns where to go back to. It throws a SignInError.
 */
export async function finishSignIn(o: SignInOptions, callback: URL): Promise<string> {
  const pending = takePending(o.storage);
  const answer = callback.searchParams;
  if (pending === undefined || answer.get("state") !== pending.state) throw new SignInError("stale");
  const issuer = answer.get("iss");
  if (issuer !== null && issuer !== new URL(o.server).origin) throw new SignInError("issuer");
  const code = answer.get("code");
  if (answer.has("error") || code === null || code === "") throw new SignInError("denied");
  try {
    await exchange(
      {
        server: o.server,
        clientId: o.clientId,
        redirectUri: o.redirectUri,
        ...(o.fetch === undefined ? {} : { fetch: o.fetch }),
      },
      code,
      pending.verifier,
    );
  } catch (error) {
    throw new SignInError("exchange", { cause: error });
  }
  return pending.returnTo;
}
