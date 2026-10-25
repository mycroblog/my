// SPDX-License-Identifier: MIT
// @mycroblog/mykit: the client of the mycroblog/my client API (ADR-0093). The types are generated from the spec
// (schema.gen.ts); the hand-written layer adds the cross-cutting behaviour only: the access token and its refresh,
// Accept-Language, RFC 9457 errors, cursor pagination, retries after 429 and 503, the Idempotency-Key, media upload,
// and PKCE.
export { apiBasePath, createClient, type Api, type ClientOptions } from "./client.js";
export { idempotencyKeyOperations } from "./idempotency.js";
export { codeChallenge, exchange, newCodeVerifier, OAuthError, tokenPath, type OAuthConfig } from "./oauth.js";
export { all, type Page, type PageFetcher } from "./paging.js";
export { NetworkError, ProblemError, problemTypeAboutBlank, type Problem } from "./problem.js";
export type { components, operations, paths } from "./schema.gen.js";
export { RefreshingTokenSource, staticToken, type OnTokens, type Token, type TokenSource } from "./token.js";
export type { RetryPolicy, Sleep } from "./transport.js";
export { uploadMedia, type MediaAttachment, type MediaUploadInput, type MediaUploadPurpose } from "./upload.js";
