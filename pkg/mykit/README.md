<!-- SPDX-License-Identifier: MIT -->

# mykit

mykit is the client library of the client API of [my](https://github.com/mycroblog/my), an ActivityPub microblog
server. It comes in two packages with the same behavior:

| Language | Package | Directory |
| --- | --- | --- |
| Go | `github.com/mycroblog/my/pkg/mykit` | this directory |
| TypeScript | `@mycroblog/mykit` on npm | [`ts/`](ts/README.md) |

The types and the requests of every operation are generated from the server's OpenAPI spec
([`api/openapi.yaml`](../../api/openapi.yaml)). The hand-written layer adds only what every call needs: the access
token and its renewal, `Accept-Language`, errors, paging, retries, idempotency keys, media upload, PKCE, and the
event stream. There are no per-endpoint helper functions: use the generated operations directly.

> **Mastodon client libraries do not work with my.** The client API is not Mastodon's: there is no `/api/v1/apps`, and
> the client registration is OAuth 2.0 (see [Getting a token](#getting-a-token)). Federation with Mastodon servers is
> unaffected.

## Install

Go (the module has its own `go.mod`, so none of the server's dependencies reach your `go.sum`):

```sh
go get github.com/mycroblog/my/pkg/mykit
```

TypeScript (ESM only; Node.js 22 or later, Bun, Deno, or a browser: anything with `fetch` and web streams):

```sh
npm install @mycroblog/mykit
```

## Quick start (Go)

```go
api, err := mykit.New(mykit.Config{
    Server:      "https://example.com", // the origin, without a path
    TokenSource: mykit.StaticToken(os.Getenv("MY_TOKEN")),
    UserAgent:   "mybot/1.0",
})
if err != nil {
    return err
}
resp, err := api.CreateStatusWithResponse(ctx, &mykit.CreateStatusParams{}, mykit.CreateStatusRequest{
    Text:       "Hello, fediverse!",
    Visibility: mykit.VisibilityPublic,
})
if err != nil {
    return err
}
fmt.Println("posted", resp.JSON200.Id)
```

Every operation is a method `<Operation>WithResponse` of `*mykit.API`. The runnable versions of the snippets in this
README are the examples in [`example_test.go`](example_test.go) (`go test -run Example`), and the TypeScript ones are in
[`ts/examples/`](ts/examples), which the package's type check covers.

## Getting a token

mykit never stores tokens: you give it a `TokenSource`, and you save the tokens. The server's authorization server
follows OAuth 2.0 with PKCE (S256 only). The ways to get a token, by use:

- **A script or a bot of your own account.** Register an app in the settings screen of the server and issue a token for
  your own account from it, without a redirect. Use it with `mykit.StaticToken`, or keep its refresh token in a
  `RefreshingTokenSource` for anything that runs longer than the access token lives (an hour).
- **A distributed CLI or app.** Publish a Client ID Metadata Document at an `https` URL and use that URL as the
  `client_id`; no registration is needed. A CLI receives the code on a loopback redirect (`http://127.0.0.1:<port>/...`).
- **A generic tool.** Dynamic client registration (RFC 7591) at `POST /oauth/register`.

The three ways of registering, the scopes, and the rate limits are in
[third-party-apps.md](../../docs/reference/third-party-apps.md).

The flow in Go (`ExampleOAuthConfig_Exchange`): send the user to `/oauth/authorize` with the challenge, then exchange
the code from the redirect.

```go
cfg := mykit.OAuthConfig{
    Server:      "https://example.com",
    ClientID:    "https://app.example/client.json",
    RedirectURI: "http://127.0.0.1:8123/callback",
}
verifier := mykit.NewCodeVerifier()
challenge := mykit.CodeChallenge(verifier) // send as code_challenge, with code_challenge_method=S256
// ... the user authorizes in the browser, and the redirect brings the code ...
tok, err := cfg.Exchange(ctx, code, verifier)
```

### Saving the tokens

`NewRefreshingTokenSource` (Go) and `RefreshingTokenSource` (TypeScript) renew the access token when it has expired or
the server answers a request with 401, once per request, and serialize the renewals. **The server rotates the refresh
token on every use**, and a refresh token that is used twice revokes the whole token family. So the callback
(`onTokens` in both languages) must save the new tokens before they are used again:

```go
tokens := mykit.NewRefreshingTokenSource(cfg, saved, func(ctx context.Context, t *mykit.Token) error {
    return store.Save(ctx, t) // your own storage
})
api, err := mykit.New(mykit.Config{Server: cfg.Server, TokenSource: tokens})
```

When `onTokens` fails, the request that needed the renewal fails with its error, and the source keeps the new tokens
and hands them out to no request until `onTokens` saves them: every later request first passes them to `onTokens`
again. So a process that restarts after a failed save never runs on with a refresh token it did not save.

The token endpoint gets the authorization code, the PKCE verifier, and the refresh tokens, so mykit never follows a
redirect from it: a redirect fails the request with an `*mykit.OAuthError` (`OAuthError`) of the redirect's status.

If the refresh token has been revoked, the renewal fails with an `*mykit.OAuthError` (`OAuthError` in TypeScript) whose
code is `invalid_grant`: send the user through the authorization again.

To use your own scheme, implement `TokenSource` (Go: `Token(ctx) (*Token, error)`, and `Refresh` to renew after a
401; TypeScript: `getAccessToken()` and `refresh()`).

## Paging

Collections use keyset pagination: a page has `items` and a `next_cursor`. `mykit.All` (Go, an `iter.Seq2[T, error]`)
and `all` (TypeScript, an `AsyncIterable<T>`) follow the cursor and fetch a page only when the loop needs it. You stop
the loop; a timeline has no end.

```go
fetch := func(ctx context.Context, cursor *mykit.Cursor) ([]mykit.Status, *mykit.Cursor, error) {
    resp, err := api.GetHomeTimelineWithResponse(ctx, &mykit.GetHomeTimelineParams{Cursor: cursor})
    if err != nil {
        return nil, nil, err
    }
    return resp.JSON200.Items, resp.JSON200.NextCursor, nil
}
for status, err := range mykit.All(ctx, fetch) {
    if err != nil {
        return err
    }
    fmt.Println(status.Id)
}
```

## Streaming

`API.Stream` (Go) and `runStream` (TypeScript) read the Server-Sent Events of `GET /streaming`: new posts of the home
timeline (and, for each of the user's lists that shows one, a list update), notifications, and announcements, as typed
events. The stream needs the scopes `read:statuses` and
`read:notifications`.

- It connects again whenever the stream ends or fails, with an exponential backoff, or after `Retry-After` for a 429
  (too many streams) or a 503. A stream that sends nothing, heartbeats included, for 60 seconds (the default; the server
  sends one every 30 seconds) counts as failed, and so does an event with more than 4 MiB of data. Do not set a
  `Timeout` on the Go `http.Client`: it would end every stream.
- **The server replays nothing, and the library sends no `Last-Event-ID`.** Events that happened while the stream was
  closed are lost to the stream. Reading what you missed is up to you: `OnOpen(reconnected)` (`onOpen` in TypeScript)
  tells you each time a stream opens, and when `reconnected` is true, read the home timeline and the notifications again.
- It stops, returning the error, when the server refuses for good: a 4xx other than 408 and 429, such as a 401 that
  a refresh did not cure or a 403 for a missing scope.
- An event that a newer server sends and the library does not know arrives as an unknown event (`*mykit.UnknownEvent`,
  `type: "unknown"`); ignore it.

See `ExampleAPI_Stream` and [`ts/examples/stream.ts`](ts/examples/stream.ts).

## Voice chat rooms

The REST operations of the voice chat rooms (`/voice/...`, and `POST /admin/voice_rooms/{id}/end`) are generated like
every other operation, for example `OpenVoiceRoomWithResponse`, `JoinVoiceRoomWithResponse`, and
`SendVoiceHeartbeatWithResponse` in Go, and `api.POST("/voice/rooms")` in TypeScript. They need the scopes `read:voice`
and `write:voice`. mykit has no audio connection: the room's event stream (`GET /voice/rooms/{id}/stream`) and the
signaling (`POST /voice/rooms/{id}/signals`) belong to the WebRTC connection of a client, so the Go client leaves them
out and the library does not read the stream.

## Errors

An answer with a status of 400 or more is an error. The body is an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)
problem document, and the error is a `*mykit.Problem` (`ProblemError` in TypeScript) with `Type`, `Title`, `Status`,
`Detail`, and the extension members of the problem (such as `reason`):

```go
var problem *mykit.Problem
if errors.As(err, &problem) && problem.Type == "/api/v1/problems/blocking" {
    // ...
}
```

**Branch on `Type` (and `Status`), never on `Title` or `Detail`:** the server localizes those from `Accept-Language`
(`Config.AcceptLanguage`, `acceptLanguage`), and mykit has no messages of its own. A request that got no answer is a
`*mykit.NetworkError` (`NetworkError`). An answer that is no problem document, from a proxy for example, becomes a
problem of type `about:blank` with the answer's status.

## Retries and idempotency keys

- A 429 or 503 is retried up to 3 times, after the time `Retry-After` gives. When `Retry-After` asks for more than
  `MaxWait` (one minute), the answer becomes a problem at once. Set `Retry.Disabled` (`retry: { disabled: true }`) to
  turn the retries off, or `MaxRetries` and `MaxWait` to change them.
- Only requests that are safe to repeat are retried: `GET`, `HEAD`, and the requests of an operation that takes an
  `Idempotency-Key` (creating a status). An `Idempotency-Key` header on any other operation does not make it retried:
  the server ignores it there.
- For the operations that take an `Idempotency-Key`, mykit adds a new UUID when you do not pass one, and sends the
  same key on every retry, so a retried post is applied once. The server answers a request with a key it has seen
  with the status posted with it, before it checks the post limit or anything else of a new post.
- A request that got no answer may still have been applied. Its `*mykit.NetworkError` (`NetworkError`) carries the key
  it was sent with (`IdempotencyKey`, `idempotencyKey`): send the request again with that key (the `IdempotencyKey`
  parameter) and it is applied once. Pass your own key to make a repeat safe across a restart, for example after a
  crash.
- A media upload reads its body once, so a Go upload is never sent again; a TypeScript `Blob` upload is sent again only
  after a 401.

## Media upload

`UploadMedia` (Go, from an `io.Reader`) and `uploadMedia` (TypeScript, from a `Blob`) build the multipart request. The
answer is usually still being processed: poll `GetMedia` until its state is ready or failed.

The Go readers stay yours: `UploadMedia` never closes them. It returns when the request or its context ends, even
while a `Read` of a reader blocks, and reads no more after that, but the goroutine that reads them lives on until that
`Read` returns. When a reader can block (a pipe, a socket, a terminal), make its `Read` return after `UploadMedia`
returns, for example by closing it.

## Admin operations

The spec's `admin` operations (`/admin/*`) are in the client like every other operation. The scopes `admin`,
`admin:read`, and `admin:write` can be granted to first-party clients only, so **calling them with a token of a
third-party app is a 403**; the server also checks the user's role for each operation.

## Versions and API stability

The client API, `/api/v1`, follows the stability policy of the server
([ADR-0090](../../docs/adr/0090-third-party-oauth-apps.md) section 8):

- Changes are additions by default: new fields, operations, scopes, and enum values are not breaking. mykit ignores
  fields and enum values it does not know, and your code should too.
- A breaking change is announced with `deprecated: true` in the spec (a `Deprecated:` comment in Go, `@deprecated` in
  TypeScript) and the `Deprecation` and `Sunset` headers, and stays for at least one minor release of v0.x. The
  release notes have an "API changes" section.
- `/api/v1` has no breaking change from v1.0.

The Go module is versioned with tags of the form `pkg/mykit/vX.Y.Z`, and the npm package with its own `version`. Because the types are generated from the spec, update the library together with
the server when you need its new operations.

## License

MIT, see [LICENSE](LICENSE). The server itself is AGPL-3.0-only; the client libraries and the OpenAPI spec they are
generated from are MIT-licensed, so you can use them in software under any license.
