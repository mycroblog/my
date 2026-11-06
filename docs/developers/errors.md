# Errors

Every error of the client API is an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem document, served as `application/problem+json`:

```json
{
  "type": "/api/v1/problems/insufficient-scope",
  "title": "...",
  "status": 403,
  "detail": "...",
  "request_id": "..."
}
```

- `type` identifies the kind of problem. It is `about:blank` when the problem means no more than its HTTP status. Some problems add members to the document (`reason`, for example); the OpenAPI document lists them.
- `status` is the HTTP status code.
- `title` and `detail` are written in the language of the request's `Accept-Language` header, and the response has a `Content-Language` header. **Branch on `type` and `status`, never on `title` or `detail`.**
- `request_id` is also the `X-Request-Id` response header. Quote it when you report a problem to the operator of the instance.
- Problems are never cached (`Cache-Control: no-store`).

## Authentication and authorization

| Status | `type` | Meaning | What to do |
| --- | --- | --- | --- |
| 401 | `/api/v1/problems/unauthenticated` | The operation needs an access token and the request has none. | Send `Authorization: Bearer ...`. |
| 401 | `/api/v1/problems/invalid-token` | The token is malformed, unknown, expired, or revoked. | Refresh it; authorize again when that fails ([tokens](authentication.md#tokens)). |
| 403 | `/api/v1/problems/insufficient-scope` | The token lacks a scope, which `WWW-Authenticate` names. | Ask the user to authorize with the scope. |
| 403 | `/api/v1/problems/first-party-only` | Only the server's own clients may call the operation, whatever the scopes. | Do not call it ([scopes](authentication.md#scopes)). |
| 403 | `/api/v1/problems/account-unavailable` | The token is valid, but its user cannot use the app (the account is deactivated, or the email is unverified, for example). | Tell the user to sign in to the server, which says why. |

A `401`, and a `403` for a missing scope, carry a `WWW-Authenticate: Bearer` challenge.

## Limits and availability

- `429 Too Many Requests` and `503 Service Unavailable` carry `Retry-After`; see [rate limits](rate-limits.md). The type `/api/v1/problems/too-many-streams` is the `429` of opening more [event streams](streaming.md) than the server allows.
- Operation-specific problems (for example `/api/v1/problems/blocking` and `/api/v1/problems/media-too-large`) are listed in the description of the problem schema and of each operation in the OpenAPI document. A new type can appear at any time ([stability](stability.md)): treat a `type` you do not know as its `status`.

## The OAuth endpoints

The endpoints under `/oauth/` do not answer with problem documents. They answer with the error bodies of OAuth 2.0 (`{"error": "invalid_grant", ...}`, [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749) §5.2), and the dynamic registration with those of [RFC 7591](https://www.rfc-editor.org/rfc/rfc7591) (`invalid_client_metadata`). When a refresh token is rejected with `invalid_grant`, send the user through the authorization again.

The [client libraries](libraries.md#errors) turn both into errors you can inspect.
