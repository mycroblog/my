// SPDX-License-Identifier: MIT
import type { Token } from "./token.js";
import { NetworkError } from "./problem.js";

/** The path of the server's token endpoint under its origin. */
export const tokenPath = "/oauth/token";

/** The client's registration on a server's authorization server (ADR-0090). */
export type OAuthConfig = {
  /** The origin of the server, such as "https://example.com". */
  server: string;
  /** The client_id of the registration, or the URL of the client's metadata document. */
  clientId: string;
  /** The secret of a confidential client; absent for a public client. */
  clientSecret?: string;
  /** The redirect_uri that the authorization request sent. */
  redirectUri?: string;
  /** Sends the requests to the token endpoint; the global `fetch` by default. */
  fetch?: (input: Request) => Promise<Response>;
};

/** Encodes bytes as base64url without padding. */
function base64url(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

/** A new PKCE code_verifier (RFC 7636 §4.1): 32 random bytes, base64url-encoded. */
export function newCodeVerifier(): string {
  return base64url(crypto.getRandomValues(new Uint8Array(32)));
}

/** The S256 code_challenge of a code_verifier (RFC 7636 §4.2), the only method the server accepts. */
export async function codeChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return base64url(new Uint8Array(digest));
}

/**
 * Exchanges the authorization code that the redirect brought for the tokens, with the code_verifier whose challenge
 * the authorization request sent.
 */
export function exchange(oauth: OAuthConfig, code: string, verifier: string): Promise<Token> {
  return tokenRequest(oauth, {
    grant_type: "authorization_code",
    code,
    code_verifier: verifier,
    redirect_uri: oauth.redirectUri ?? "",
  });
}

/** An error answer of the token endpoint (RFC 6749 §5.2). */
export class OAuthError extends Error {
  override readonly name = "OAuthError";
  /** The HTTP status of the answer. */
  readonly status: number;
  /** The error code, such as "invalid_grant". */
  readonly code: string;
  readonly description: string;

  constructor(status: number, code: string, description = "") {
    super(`mykit: oauth: ${code}${description === "" ? "" : `: ${description}`}`);
    this.status = status;
    this.code = code;
    this.description = description;
  }
}

/** The form encoding of RFC 6749 Appendix B, which the client credentials get before they go into Basic. */
function formEncode(s: string): string {
  return new URLSearchParams({ s }).toString().slice("s=".length);
}

/** Posts a grant to the token endpoint, with the client's credentials, and reads the tokens. */
export async function tokenRequest(oauth: OAuthConfig, params: Record<string, string>): Promise<Token> {
  const endpoint = new URL(tokenPath, oauth.server).href;
  const form = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== "") form.set(k, v);
  const headers = new Headers({ "Content-Type": "application/x-www-form-urlencoded", Accept: "application/json" });
  if (oauth.clientSecret === undefined || oauth.clientSecret === "") {
    form.set("client_id", oauth.clientId);
  } else {
    // RFC 6749 §2.3.1: the credentials are form-encoded before they go into Basic.
    const credentials = `${formEncode(oauth.clientId)}:${formEncode(oauth.clientSecret)}`;
    headers.set("Authorization", `Basic ${btoa(String.fromCharCode(...new TextEncoder().encode(credentials)))}`);
  }
  const fetch = oauth.fetch ?? ((request: Request) => globalThis.fetch(request));
  const now = Date.now();
  let response: Response;
  let body: string;
  try {
    response = await fetch(new Request(endpoint, { method: "POST", headers, body: form }));
    body = await response.text();
  } catch (error) {
    if (isAbort(error)) throw error;
    throw new NetworkError("POST", endpoint, error);
  }
  if (response.status !== 200) {
    let code = "";
    let description = "";
    if (response.headers.get("Content-Type")?.split(";")[0]?.trim() === "application/json") {
      try {
        const v = JSON.parse(body) as { error?: unknown; error_description?: unknown };
        if (typeof v.error === "string") code = v.error;
        if (typeof v.error_description === "string") description = v.error_description;
      } catch {
        // No error document: the status below.
      }
    }
    throw new OAuthError(response.status, code === "" ? response.statusText || String(response.status) : code, description);
  }
  let tr: { access_token?: unknown; token_type?: unknown; expires_in?: unknown; refresh_token?: unknown; scope?: unknown };
  try {
    tr = JSON.parse(body) as typeof tr;
  } catch (error) {
    throw new OAuthError(response.status, "invalid_response", error instanceof Error ? error.message : String(error));
  }
  if (typeof tr.access_token !== "string" || tr.access_token === "" || typeof tr.token_type !== "string" || tr.token_type.toLowerCase() !== "bearer") {
    throw new OAuthError(response.status, "invalid_response", "no bearer access token");
  }
  const token: Token = { accessToken: tr.access_token };
  if (typeof tr.refresh_token === "string" && tr.refresh_token !== "") token.refreshToken = tr.refresh_token;
  if (typeof tr.scope === "string") token.scope = tr.scope;
  if (typeof tr.expires_in === "number" && tr.expires_in > 0) token.expiresAt = now + tr.expires_in * 1000;
  return token;
}

/** Whether error is the rejection of an aborted request. */
export function isAbort(error: unknown): boolean {
  return error instanceof DOMException && (error.name === "AbortError" || error.name === "TimeoutError");
}
