<!-- SPDX-License-Identifier: MIT -->

# @mycroblog/mykit

The TypeScript client of the client API of [my](https://github.com/mycroblog/my), an ActivityPub microblog server.
It is [openapi-fetch](https://openapi-ts.dev/openapi-fetch/) typed by the server's OpenAPI spec, plus what every call
needs: the access token and its renewal, `Accept-Language`, errors, paging, retries, idempotency keys, media upload,
PKCE, and the event stream. It is ESM only and runs wherever `fetch` and web streams do (Node.js 22 or later, Bun,
Deno, browsers); it imports no Node.js built-in module.

The Go client has the same behavior; the full guide, which applies to both, is the [mykit README](../README.md). This
page has the TypeScript parts.

> **Mastodon client libraries do not work with my.** The client API is not Mastodon's (there is no `/api/v1/apps`).
> Register an OAuth 2.0 client instead; see [Getting a token](../README.md#getting-a-token).

```sh
npm install @mycroblog/mykit
```

## Quick start

The runnable-by-type-check versions of these snippets are in [`examples/`](examples).

```ts
import { createClient, NetworkError, ProblemError, staticToken } from "@mycroblog/mykit";

const api = createClient({
  server: "https://example.com", // the origin, without a path
  tokenSource: staticToken("access token"),
  acceptLanguage: "en",
});

const { data } = await api.POST("/statuses", { body: { text: "Hello, fediverse!", visibility: "public" } });
console.log(data?.id);
```

`api` is an openapi-fetch client: `api.GET`, `api.POST`, and so on take the path of the operation, and the parameters,
the body, and `data` are typed by the spec. A call that gets an answer of 400 or more **rejects** with a `ProblemError`
(openapi-fetch's `error` field is not used), so when it resolves, `data` is the answer.

## Errors

```ts
try {
  await api.POST("/statuses", { body: { text: "hi", visibility: "public" } });
} catch (error) {
  if (error instanceof ProblemError && error.type === "/api/v1/problems/blocking") {
    // ...
  } else if (error instanceof NetworkError) {
    // no answer; error.cause is the error of fetch
  }
}
```

`ProblemError` is an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem: `type`, `title`, `status`, `detail`,
the extension members in `problem`, and the response `headers`. **Branch on `type` and `status`, never on `title` or
`detail`**, which the server localizes from `Accept-Language`. An aborted request rejects with the reason of its signal.

Middleware of your own that turns an error answer into an error type of yours before the `ProblemError` can read the
problem with `problemFromResponse(response)`.

## Getting a token and saving it

mykit never stores tokens. The flow with PKCE, and a `RefreshingTokenSource` whose `onTokens` callback saves the
rotated refresh token, are in [`examples/oauth.ts`](examples/oauth.ts):

```ts
const token = await exchange(oauth, code, verifier);
const api = createClient({
  server: oauth.server,
  tokenSource: new RefreshingTokenSource(oauth, token, async (renewed) => {
    await save(renewed); // your own storage
  }),
});
```

The server rotates the refresh token on every use, and a refresh token used twice revokes the whole token family, so
`onTokens` must save the new tokens. The source renews the token when it expires or after a 401, once per request.

A `TokenSource` is any object with `getAccessToken()` and, optionally, `refresh(refused)`.

## Paging

```ts
for await (const status of all(async (cursor) => {
  const { data } = await api.GET("/timelines/home", { params: { query: cursor === undefined ? {} : { cursor } } });
  if (data === undefined) throw new Error("the answer has no body");
  return data;
})) {
  console.log(status.id);
  if (done()) break; // a timeline has no end
}
```

## Streaming

```ts
const controller = new AbortController();
await runStream({
  server: "https://example.com",
  tokenSource,
  signal: controller.signal,
  onOpen: (reconnected) => {
    if (reconnected) refetchTimelinesAndNotifications();
  },
  onEvent: (event) => {
    if (event.type === "update") console.log(event.status.id);
  },
});
```

`runStream` connects again whenever the stream ends or fails. **The server replays nothing and the library sends no
`Last-Event-ID`:** after `onOpen(true)` you read the home timeline and the notifications again yourself. The events are
`update`, `notification`, `announcement`, `announcement_removed`, and `unknown` (an event of a newer server; ignore it).
Abort the signal to stop. See [`examples/stream.ts`](examples/stream.ts) and the
[mykit README](../README.md#streaming) for what makes it stop.

Other event streams of the API, such as the stream of a voice chat room, are read with `fetch` too: `EventStreamParser`
turns the text of such a body into its events (`push` each decoded piece, then `end`), as the HTML Standard reads an
event stream.

## Retries, idempotency keys, uploads, admin operations, stability

These work as in the [mykit README](../README.md): 429 and 503 are retried after `Retry-After` for `GET`, `HEAD`, and
requests with an `Idempotency-Key` (`retry: { disabled, maxRetries, maxWait }`; `maxWait` is in milliseconds); the key
of a new status is added for you and kept across retries; `uploadMedia(api, { file: blob, description })` uploads a
`Blob`; the `admin` operations answer 403 to a token of a third-party app; and `/api/v1` follows the
[stability policy](../README.md#versions-and-api-stability) (a `@deprecated` tag marks what will go).

## License

MIT, see [LICENSE](LICENSE).
