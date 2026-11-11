# API client library (Go and TypeScript)

- Changing the authorization code flow of a client, or its PKCE: [RFC 7636](https://www.rfc-editor.org/rfc/rfc7636), search for: `code_verifier`, `code_challenge`, `S256`
- Changing the token requests or the reading of their errors: [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749), search for: Access Token Request, Refreshing an Access Token, `invalid_grant`, Error Response
- Storing or renewing a refresh token: [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700), search for: Refresh Token Protection, refresh token rotation
- Reading an error of the API: [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457), search for: `application/problem+json`, `type`, `detail`, Extension Members
- Sending an `Idempotency-Key` (an Internet-Draft): [draft-ietf-httpapi-idempotency-key-header](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/), search for: `Idempotency-Key`, retry, fingerprint
- Reading an event stream or resuming one: [HTML Standard, Server-sent events](https://html.spec.whatwg.org/multipage/server-sent-events.html), search for: `text/event-stream`, `Last-Event-ID`, `retry`, Interpreting an event stream
- Changing the Go generator or its overlay: [oapi-codegen documentation](https://github.com/oapi-codegen/oapi-codegen), search for: OpenAPI Overlay, `output-options`, `x-go-type`, `generate`
- Changing the TypeScript types or the fetch client: [openapi-typescript documentation](https://openapi-ts.dev/introduction), search for: `openapi-typescript`, `openapi-fetch`, `createClient`, Middleware
