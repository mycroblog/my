# Client libraries: mykit and mybot

Two libraries, each in Go and in TypeScript with the same behavior, do the plumbing the other guides describe. They are MIT-licensed, so you can use them in software under any license.

| Library | For | Go | TypeScript |
| --- | --- | --- | --- |
| mykit | Any client of the API | [`github.com/mycroblog/my/pkg/mykit`](https://pkg.go.dev/github.com/mycroblog/my/pkg/mykit) | [`@mycroblog/mykit`](https://www.npmjs.com/package/@mycroblog/mykit) |
| mybot | Bots, built on mykit | [`github.com/mycroblog/my/pkg/mybot`](https://pkg.go.dev/github.com/mycroblog/my/pkg/mybot) | [`@mycroblog/mybot`](https://www.npmjs.com/package/@mycroblog/mybot) |

The full API is on pkg.go.dev and in the types of the npm packages; the READMEs ([mykit](https://github.com/mycroblog/my/blob/main/pkg/mykit/README.md), [mybot](https://github.com/mycroblog/my/blob/main/pkg/mybot/README.md)) go deeper than this page. Mastodon client libraries do not work with this server ([getting a token](authentication.md)).

## mykit

mykit's types and requests are generated from the OpenAPI document of the API, so every operation is there with typed parameters and answers; there are no per-endpoint helper functions. The hand-written layer adds the access token and its renewal, `Accept-Language`, errors, paging, retries, idempotency keys, media upload, PKCE, and the event stream.

Install it. The Go module has its own `go.mod`, so none of the server's dependencies reach your `go.sum`. The TypeScript package is ESM only and runs where `fetch` and web streams do (Node.js 22 or later, Bun, Deno, browsers).

```sh
go get github.com/mycroblog/my/pkg/mykit
npm install @mycroblog/mykit
```

Post with a token you already have (see [getting a token](authentication.md)):

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
fmt.Println("posted", resp.JSON201.Id)
```

```ts
import { createClient, staticToken } from "@mycroblog/mykit";

const api = createClient({
  server: "https://example.com", // the origin, without a path
  tokenSource: staticToken(process.env.MY_TOKEN!),
});

const { data } = await api.POST("/statuses", { body: { text: "Hello, fediverse!", visibility: "public" } });
console.log(data?.id);
```

### Tokens

mykit never stores tokens: you give it a token source and you save the tokens. For anything that runs longer than an access token lives (an hour), use `NewRefreshingTokenSource` (Go) or `RefreshingTokenSource` (TypeScript), which renews the token when it has expired or the server answers `401`, and serializes the renewals. **The server rotates the refresh token on every use**, so the callback you give it (`onTokens`) must save the new tokens before they are used again. `OAuthConfig`, `NewCodeVerifier` and `CodeChallenge` (`newCodeVerifier`, `codeChallenge` and `exchange` in TypeScript) run the authorization code flow with PKCE.

### Paging

`mykit.All` (Go, an `iter.Seq2`) and `all` (TypeScript, an `AsyncIterable`) follow the `next_cursor` of the [paging](paging.md) and fetch a page when your loop needs it.

```go
for status, err := range mykit.All(ctx, func(ctx context.Context, cursor *mykit.Cursor) ([]mykit.Status, *mykit.Cursor, error) {
    resp, err := api.GetHomeTimelineWithResponse(ctx, &mykit.GetHomeTimelineParams{Cursor: cursor})
    if err != nil {
        return nil, nil, err
    }
    return resp.JSON200.Items, resp.JSON200.NextCursor, nil
}) {
    if err != nil {
        return err
    }
    fmt.Println(status.Id)
}
```

### Errors

An answer of 400 or more is an error that carries the [problem document](errors.md): a `*mykit.Problem` in Go (use `errors.As`) and a `ProblemError` in TypeScript, with `Type`, `Title`, `Status`, `Detail`, and the extension members. A request that got no answer is a `*mykit.NetworkError` (`NetworkError`), and a failure at the token endpoint an `*mykit.OAuthError` (`OAuthError`). Branch on the type and the status, never on the text.

```go
var problem *mykit.Problem
if errors.As(err, &problem) && problem.Type == "/api/v1/problems/blocking" {
    // ...
}
```

### Retries and idempotency keys

A `429` or `503` is retried up to 3 times after the `Retry-After` it gives, when the request is safe to repeat (`GET`, `HEAD`, and the creation of a post). The server applies a post with an `Idempotency-Key` once, so mykit adds a key to the operations that take one and sends the same key on every retry. A `NetworkError` carries the key, so you can send the request again with it and it is applied once. `Retry` in the configuration changes or turns off the retries; see [rate limits](rate-limits.md).

### Streaming

`API.Stream` (Go) and `runStream` (TypeScript) read the [event stream](streaming.md) as typed events, reconnect with a backoff or after `Retry-After`, treat 60 seconds without even a heartbeat as a failure, and stop on a final `4xx`. They do not replay anything: `OnOpen` (`onOpen`) tells you when a stream opens again so that you can read the home timeline and the notifications once more. In TypeScript, `runStream` also reads the local and federated timeline streams. Do not set a `Timeout` on the `http.Client` of the Go client; it would end every stream.

## mybot

mybot builds bots on mykit. You write the handlers of the bot's events (`OnMention`, `OnCommand`, `OnFollow`, and others) and the library receives the events so that none is lost or answered twice across disconnections and restarts, runs scheduled jobs (`Every`), and keeps the bot polite by default. It keeps its state, such as the newest refresh token and the notifications it has handled, in a file, so run it with storage that survives a restart.

```sh
go get github.com/mycroblog/my/pkg/mybot
npm install @mycroblog/mybot
```

```go
bot, err := mybot.New(mybot.Config{
    Server:       "https://example.com",
    OAuth:        &mykit.OAuthConfig{ClientID: os.Getenv("CLIENT_ID"), ClientSecret: os.Getenv("CLIENT_SECRET")},
    RefreshToken: os.Getenv("REFRESH_TOKEN"),
    UserAgent:    "echobot/1.0",
})
if err != nil {
    log.Fatal(err)
}
bot.OnMention(func(c *mybot.Context) error {
    _, err := c.Reply(c.Text)
    return err
})

ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := bot.Run(ctx); err != nil {
    log.Fatal(err)
}
```

```ts
import { Bot } from "@mycroblog/mybot";

const bot = new Bot({
  server: "https://example.com",
  oauth: { clientId: process.env.CLIENT_ID!, clientSecret: process.env.CLIENT_SECRET! },
  refreshToken: process.env.REFRESH_TOKEN!,
  userAgent: "echobot/1.0",
});
bot.onMention(async (ctx) => {
  await ctx.reply(ctx.text);
});

const controller = new AbortController();
process.once("SIGINT", () => controller.abort());
await bot.run({ signal: controller.signal });
```

A bot acts as a user, so run it on an account of its own and give it the bot flag (the account's setting, or `Config.SetBotFlag` in Go with the scope `write:accounts`). To get its credentials:

1. Sign in as the bot's account and open **Settings → Your apps**.
2. Register an app of the type **Confidential**, with `http://127.0.0.1/callback` as its redirect URI and at least the scopes `read:accounts`, `read:notifications`, `read:statuses`, and `write:statuses`.
3. Copy the client ID and the secret, which is shown once.
4. Press **Issue token** on the app. The server shows an access token and a refresh token for your account once, without a redirect. Pass the refresh token to the bot; it gets its own access tokens and saves each rotated refresh token.

A bot without the scopes it needs stops at start with a scope error. Both packages ship a test harness (`mybottest` for Go, `@mycroblog/mybot/testing` for TypeScript) for testing handlers without a server, and the READMEs list runnable examples: an echo bot, a scheduled greeting, and a command bot.
