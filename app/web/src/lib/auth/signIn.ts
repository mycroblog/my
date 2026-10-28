// SPDX-License-Identifier: AGPL-3.0-only
// Signing in with the Authorization Code grant and PKCE (RFC 6749 §4.1, RFC 7636, ADR-0030). The app
// sends the browser to the server's authorization screen, which redirects back to /login/callback with a
// code; the callback checks `state` and exchanges the code for an access token (session.ts).
//
// The PKCE verifier, `state` and where to return must survive the round trip through the server's pages,
// so they wait in sessionStorage (this tab only, lib/storage's signInKey) and are deleted when the callback reads them. They are
// not credentials: the code they go with is single-use, short-lived and bound to the verifier.
import type { Session } from "./session";
import { webClientId } from "./session";
import { signInKey, type StoredSignIn } from "../storage";

/** The authorization endpoint (authpages.PathAuthorize). */
const authorizePath = "/oauth/authorize";

/** The path of the redirect URI on the web client's origin (platform.WebClientCallbackPath). */
const callbackPath = "/login/callback";

/** Unpadded base64url (RFC 4648 §5), the encoding of PKCE's verifier and challenge. */
function base64url(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

/** `byteLength` random bytes, base64url-encoded. 32 bytes make a 43-character PKCE verifier (RFC 7636 §4.1). */
function randomToken(byteLength = 32): string {
  return base64url(crypto.getRandomValues(new Uint8Array(byteLength)));
}

/** The S256 code challenge of `verifier` (RFC 7636 §4.2). */
async function codeChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return base64url(new Uint8Array(digest));
}

/**
 * Whether `path` is a path of this app to return to after signing in: it starts with one slash, so it
 * cannot name another origin ("//evil.example") or a scheme ("javascript:").
 */
export function isReturnPath(path: string): boolean {
  return path.startsWith("/") && !path.startsWith("//") && !path.startsWith("/\\");
}

export type BeginSignInOptions = {
  /** The origin of the server's authorization endpoint (the runtime config's apiOrigin). */
  apiOrigin: string;
  /** The web client's origin; the redirect URI is on it. */
  appOrigin: string;
  /** The path of this app to open after signing in. */
  returnTo: string;
};

/**
 * Starts a sign-in: remembers a new verifier and state, and returns the URL of the authorization screen
 * to send the browser to. The request names no scope, so the token gets every scope the web client is
 * registered with.
 */
export async function beginSignIn({ apiOrigin, appOrigin, returnTo }: BeginSignInOptions): Promise<URL> {
  const pending: StoredSignIn = {
    state: randomToken(),
    codeVerifier: randomToken(),
    returnTo: isReturnPath(returnTo) ? returnTo : "/",
  };
  const url = new URL(authorizePath, apiOrigin);
  url.search = new URLSearchParams({
    response_type: "code",
    client_id: webClientId,
    redirect_uri: new URL(callbackPath, appOrigin).href,
    state: pending.state,
    code_challenge: await codeChallenge(pending.codeVerifier),
    code_challenge_method: "S256",
  }).toString();
  signInKey.write(pending);
  return url;
}

/** What the authorization server sent back to the callback (RFC 6749 §4.1.2, §4.1.2.1). */
export type AuthorizationResponse = {
  code?: string | undefined;
  state?: string | undefined;
  error?: string | undefined;
};

/** Why a sign-in failed at the callback. */
export type SignInFailure =
  /** The user declined on the authorization screen. */
  | "denied"
  /** The callback was not for the sign-in this tab started (no sign-in in progress, or another state). */
  | "invalid-state"
  /** The server refused the code or answered with an error. */
  | "rejected";

export class SignInError extends Error {
  override name = "SignInError";
  readonly reason: SignInFailure;

  constructor(reason: SignInFailure, options?: ErrorOptions) {
    super(`sign-in failed: ${reason}`, options);
    this.reason = reason;
  }
}

/** The callbacks being completed, by state, so that a second run of the route gets the same result. */
const completions = new Map<string, Promise<string>>();

/**
 * Completes a sign-in at the callback: checks that `state` is the one this tab sent (RFC 6749 §10.12),
 * exchanges the code with its verifier, and returns the path to open next. Throws a SignInError, or a
 * NetworkError when the server cannot be reached.
 */
export function completeSignIn(session: Session, response: AuthorizationResponse, appOrigin: string): Promise<string> {
  const key = response.state ?? "";
  let completion = completions.get(key);
  if (completion === undefined) {
    completion = exchange(session, response, appOrigin);
    completions.set(key, completion);
  }
  return completion;
}

async function exchange(
  session: Session,
  { code, state, error }: AuthorizationResponse,
  appOrigin: string,
): Promise<string> {
  const pending = takePending();
  if (pending === null || state === undefined || state !== pending.state) throw new SignInError("invalid-state");
  if (error !== undefined) throw new SignInError(error === "access_denied" ? "denied" : "rejected");
  if (code === undefined || code === "") throw new SignInError("rejected");
  const accepted = await session.exchangeCode({
    code,
    redirectUri: new URL(callbackPath, appOrigin).href,
    codeVerifier: pending.codeVerifier,
  });
  if (!accepted) throw new SignInError("rejected");
  return pending.returnTo;
}

/** Reads and deletes the sign-in in progress. */
function takePending(): StoredSignIn | null {
  const pending = signInKey.read();
  signInKey.remove();
  return pending;
}
