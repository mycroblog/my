// SPDX-License-Identifier: AGPL-3.0-only
// The pieces every MSW handler shares: the API base path, problem responses, the access token the default
// handlers accept, and the OAuth responses (web-client/10-api-client-and-auth.md §10).
import { http, HttpResponse } from "msw";
import { instanceFixture, type Schemas, viewerFixture } from "../fixtures";

/** The client API base path, `servers[0].url` in api/openapi.yaml. */
export const apiBase = "/api/v1";

/** An RFC 9457 problem response, as the client API writes it. */
export function problem(status: number, type: string, title: string, init: { headers?: Record<string, string> } = {}) {
  const body: Schemas["Problem"] = { type, title, status };
  return HttpResponse.json(body, { status, headers: { "Content-Type": "application/problem+json", "Cache-Control": "no-store", ...init.headers } });
}

/** The access token that the default handlers accept. */
const validAccessToken = "test-access-token";

/** Whether the request carries the access token the default handlers accept. */
export function authorized(request: Request): boolean {
  return request.headers.get("Authorization") === `Bearer ${validAccessToken}`;
}

/** The 401 of a request without a usable access token. */
export function unauthenticated(request: Request) {
  return request.headers.get("Authorization") === null
    ? problem(401, "/api/v1/problems/unauthenticated", "Unauthenticated", { headers: { "WWW-Authenticate": "Bearer" } })
    : problem(401, "/api/v1/problems/invalid-token", "Invalid token", { headers: { "WWW-Authenticate": 'Bearer error="invalid_token"' } });
}

/** Answers with `respond()` when the request carries the access token the default handlers accept. */
export function withViewer(request: Request, respond: () => Response): Response {
  return authorized(request) ? respond() : unauthenticated(request);
}

/**
 * The viewer's account with the permissions `permissions`, for GET /accounts/verify_credentials: the staff
 * screens' tests start with it, as the default viewer has no permissions.
 */
export function operatorHandler(permissions: Schemas["Permission"][]) {
  return http.get(`${apiBase}/accounts/verify_credentials`, ({ request }) => withViewer(request, () => HttpResponse.json({ ...viewerFixture, permissions })));
}

/** The OAuth endpoints (internal/interface/oauth). */
export const oauthPaths = {
  authorize: "/oauth/authorize",
  token: "/oauth/token",
  refresh: "/oauth/token/refresh",
  revoke: "/oauth/revoke",
} as const;

/** A token response of the OAuth endpoints (RFC 6749 §5.1), without a refresh token as for the web client. */
export function tokenResponse(accessToken: string) {
  return HttpResponse.json({ access_token: accessToken, token_type: "Bearer", expires_in: 3600, scope: "read write follow push" }, { headers: { "Cache-Control": "no-store" } });
}

/** An OAuth error response (RFC 6749 §5.2). */
export function oauthError(error: string, status = 400) {
  return HttpResponse.json({ error }, { status, headers: { "Cache-Control": "no-store" } });
}

/** Makes the session resume: the refresh endpoint answers with the access token the default handlers accept. */
export const signedInHandler = http.post(oauthPaths.refresh, () => tokenResponse(validAccessToken));

/**
 * A refresh token cookie that resumes a session with the access token the default handlers accept: pass it
 * to `server.use()` before `renderApp` to start signed in.
 */
export function resumableSession() {
  return http.post(oauthPaths.refresh, () => tokenResponse(validAccessToken));
}

/**
 * The default instance, error report, and OAuth handlers: no refresh token cookie, so the browser has no session to
 * resume.
 */
export const handlers = [
  http.get(`${apiBase}/instance`, () => HttpResponse.json(instanceFixture)),
  http.post(`${apiBase}/client_errors`, () => new HttpResponse(null, { status: 204 })),
  http.post(oauthPaths.refresh, () => oauthError("invalid_grant")),
  http.post(oauthPaths.token, () => oauthError("invalid_grant")),
  http.post(oauthPaths.revoke, () => new HttpResponse(null, { status: 200 })),
];
