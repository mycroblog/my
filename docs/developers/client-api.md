# The client API

Clients and bots talk to a server of `my` through its client API: JSON over HTTPS under `/api/v1`, with OAuth 2.0 access tokens. It is not compatible with the Mastodon client API, so Mastodon client libraries do not work with it.

The [API reference](../../api/openapi.yaml) lists every operation with its parameters, request body, answers, and the scopes it needs. It is made from the OpenAPI 3.1 document of the API, which every server also serves at `GET /api/v1/openapi.yaml` without a token; generate a client from it, or use one of the [client libraries](libraries.md).

## Read first

These guides explain what the operations share:

- [Getting a token](authentication.md): registering an app, the authorization flow, tokens, redirect URIs, and scopes
- [Paging](paging.md): cursors and limits
- [Errors](errors.md): problem documents and the problem types to handle
- [Rate limits](rate-limits.md): what to read and how to back off
- [The event stream](streaming.md): Server-Sent Events, reconnecting, and catching up
- [Client libraries](libraries.md): getting started with mykit and mybot in Go and TypeScript
- [Stability of the API](stability.md): what changes without notice and what is announced
