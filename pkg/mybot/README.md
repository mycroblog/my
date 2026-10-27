<!-- SPDX-License-Identifier: MIT -->

# mybot

mybot builds bots for [my](https://github.com/mycroblog/my), an ActivityPub microblog server, on its API client
[mykit](../mykit/README.md). You write the handlers of the bot's events; the library receives the events so that none
is lost or answered twice across disconnections and restarts, runs the scheduled jobs, and keeps the bot polite by
default. It comes in two packages with the same handlers, options, defaults, idempotency keys, and state file:

| Language | Package | Directory |
| --- | --- | --- |
| Go | `github.com/mycroblog/my/pkg/mybot` | this directory |
| TypeScript | `@mycroblog/mybot` on npm | [`ts/`](ts/README.md) |

This README describes the Go package; the [TypeScript README](ts/README.md) describes the other, and ends with the
differences between the two.

## Install

The module has its own `go.mod` and depends on mykit alone, so none of the server's dependencies reach your `go.sum`.
It builds with `CGO_ENABLED=0`.

```sh
go get github.com/mycroblog/my/pkg/mybot
```

## Quick start

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/mycroblog/my/pkg/mybot"
    "github.com/mycroblog/my/pkg/mykit"
)

func main() {
    bot, err := mybot.New(mybot.Config{
        Server:       "https://example.com",
        OAuth:        &mykit.OAuthConfig{ClientID: os.Getenv("CLIENT_ID"), ClientSecret: os.Getenv("CLIENT_SECRET")},
        RefreshToken: os.Getenv("REFRESH_TOKEN"),
        UserAgent:    "echobot/1.0",
    })
    if err != nil {
        log.Fatal(err)
    }
    bot.OnCommand("ping", "answer pong", func(c *mybot.Context) error {
        _, err := c.Reply("pong")
        return err
    })
    bot.OnMention(func(c *mybot.Context) error {
        _, err := c.Reply(c.Text)
        return err
    })

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    if err := bot.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

`Run` verifies the account, receives the events until the context ends, then [shuts down](#shutting-down) and returns
nil. It returns an error when the bot cannot start: `mybot.ErrNotBot`, a `*mybot.ScopeError` (with `Missing`), or a
`*mykit.Problem` of the server.

## Examples

Each example is a `package main` with its tests against [`mybottest`](#testing), in Go here and in TypeScript in
[`ts/examples`](ts/examples):

| Example | What it does |
| --- | --- |
| [`examples/echo`](examples/echo) | replies to every mention with its text (`OnMention`) |
| [`examples/schedule`](examples/schedule) | posts a greeting at 9:00 every morning in a time zone (`Every` with `CatchUp: mybot.Latest`) |
| [`examples/commands`](examples/commands) | answers the commands `help`, `ping`, and `dice 2d6` (`OnCommand`) |
| [`examples/login`](examples/login) | gets a token with the Authorization Code flow and PKCE on a loopback redirect (Go only) |

The bots read `MYBOT_SERVER`, `MYBOT_CLIENT_ID`, `MYBOT_CLIENT_SECRET`, `MYBOT_REFRESH_TOKEN`, and `MYBOT_STATE` (the
state file, `./mybot-state.json` by default) from the environment:

```sh
MYBOT_SERVER=https://example.com MYBOT_CLIENT_ID=... MYBOT_CLIENT_SECRET=... MYBOT_REFRESH_TOKEN=... \
    go run ./examples/echo
```

[`examples/Dockerfile`](examples/Dockerfile) builds any of them into a static, non-root image whose state file lives
on the volume `/data`. Build it from the root of the repository, which holds the mykit that this module is built with:

```sh
docker build -f pkg/mybot/examples/Dockerfile --build-arg EXAMPLE=echo -t mybot-echo .
docker run -d --env-file bot.env -v mybot-echo:/data mybot-echo
```

Keep the volume: a container that starts with an empty state uses `MYBOT_REFRESH_TOKEN` again, which the server has
seen used (see [State](#state)).

## Getting a token

A bot always acts as a user: run it on an account of its own. The steps:

1. Sign in to the server as the bot's account and open **Settings → Development** (`/settings/apps`).
2. Register an app with the type **Confidential**. The form needs a redirect URI even when no redirect is used:
   `http://127.0.0.1/callback` serves both this and [`examples/login`](examples/login). Give it at least the scopes
   below.
3. Copy the client ID and the secret, which is shown once.
4. Press **Issue token** on the app and choose the scopes (empty for all of the app's). The server issues an access
   token and a refresh token for your own account, without a redirect, and shows them once.
5. Pass the client ID and the secret as `Config.OAuth` and the refresh token as `Config.RefreshToken`. The access
   token is not needed: the bot gets a new one with the refresh token.

The scopes the bot needs are `read:accounts`, `read:notifications`, `read:statuses` (for the event stream), and
`write:statuses`, plus `write:accounts` with `Config.SetBotFlag`. A token without them stops the bot at start with a
`*mybot.ScopeError`. The administration and supporter scopes are not available to apps.

The server rotates the refresh token on every renewal and revokes the whole token family when a used one comes back.
The bot saves every rotated refresh token in its [Store](#state) and, on the next start, uses the saved one rather than
`Config.RefreshToken`. A `Config.TokenSource` (such as `mykit.StaticToken`) instead of `OAuth` is for tests and short
runs: the bot saves nothing for it.

To authorize a bot through the consent page instead, as an app that other people's accounts can run, use the
Authorization Code flow with PKCE: [`examples/login`](examples/login) listens on a port of `127.0.0.1`, prints the URL
of the authorization page, and prints the refresh token after you approve. The server ignores the port of a loopback
redirect URI, so `http://127.0.0.1/callback` matches.

## Receiving the events

- In the default `Mode` `mybot.Streaming` the bot opens the event stream (`GET /api/v1/streaming`; mykit reconnects
  it). After every connection, the first and each reconnection, it reads the notifications newest first back to the
  position in its Store, or to `Config.CatchUpLimit` (24 hours; zero turns the catch-up off), and handles those it
  missed, oldest first.
- The notifications reach the handlers in the order of time. Those that the event stream brings while a catch-up
  runs wait for it, and the bot then hands out the missed ones and the waiting ones together, oldest first. When the
  catch-up fails, the bot tries it again, waiting longer each time up to a minute, and the stream's notifications
  keep waiting.
- Every notification is recorded in the Store after its handler returns, and one recorded before is not handled again.
  A process that stops in the middle of a handler handles that notification again on the next start: replies carry an
  `Idempotency-Key` derived from the notification (`mybot.ReplyIdempotencyKey`), so the server posts each of them once.
- The catch-up covers the time the stream is closed: a stop of the bot, or a cut connection. A notification that the
  server sends while the stream stays open but that never reaches the bot, such as one a proxy drops, is older than the
  next one the bot processes, and the catch-up does not find it
  ([#1291](https://github.com/mycroblog/my/issues/1291)). Behind a proxy that drops events, use `mybot.Polling`.
- `Mode: mybot.Polling` reads the notifications every `Config.PollInterval` (a minute) instead, for networks whose
  proxies do not pass an event stream.
- `OnStatus` receives the new posts of the home timeline from the event stream alone: they are not caught up after a
  disconnection or a restart, and the polling mode does not receive them.
- Up to `Config.Concurrency` (4) handlers run at once; the events of one sender run one after the other, in the
  order the bot hands them out. The handlers of different senders run side by side, so one may end before an older
  one of another sender.

## Handlers

| Handler | Runs for |
| --- | --- |
| `OnMention` | a mention, replies and direct posts included (`c.IsReply`, `c.IsDirect`), that is no command |
| `OnCommand(name, description, h)` | a mention whose first word after the bot's own mentions is `name`, without case |
| `OnFollow` / `OnFollowRequest` | `follow` / `follow_request` |
| `OnReaction` / `OnFavourite` / `OnBoost` / `OnQuote` | the notification of the same name |
| `OnNotification` | every notification that no handler above takes |
| `OnStatus` | a new post of the home timeline (the bot's own excepted) |

The `*mybot.Context` is a `context.Context` that ends when the bot stops, with the `Notification`, its `Status` and
sender `Account`, `Text` (the post as plain text), `Command` and `Args`, `Me()`, `API()` (the mykit client), and
`Logger()` (`log/slog`). Its helpers `Reply(text)`, `React(emoji)`, `Favourite()`, and `Follow()` keep the defaults
below. A handler's error is logged, and its notification counts as processed. `help` answers with the registered
commands and their descriptions unless `Config.DisableHelp` is set or a command `help` is registered.

Middleware is `func(mybot.Handler) mybot.Handler`, added with `bot.Use(...)` inside the default chain `Recover`,
`Manners`, `SendBudget` (`Config.NoDefaultMiddleware` leaves that chain out):

```go
bot.Use(func(next mybot.Handler) mybot.Handler {
    return func(c *mybot.Context) error {
        if c.Command == "shutdown" && (c.Account == nil || c.Account.Acct != "admin") {
            return nil // skip the handler
        }
        return next(c)
    }
})
```

## Scheduled jobs

`Every` runs a handler at the times of a cron expression of five fields, minute hour day-of-month month day-of-week,
in UTC or the IANA time zone `EveryOptions.TimeZone`:

```go
err := bot.Every("0 9 * * *", func(c *mybot.Context) error {
    _, err := c.Post("Good morning")
    return err
}, mybot.EveryOptions{Name: "morning", TimeZone: "Asia/Tokyo"})
```

- A field is `*`, a number, a range `a-b`, a step `*/n` or `a-b/n`, or a list of those; the day of the week is 0-7,
  0 and 7 being Sunday. A field of seconds, the aliases such as `@daily`, and the names of months and days are
  refused, so that the Go and the TypeScript library take the same expressions.
- `c.Post(text)` posts once per run, with the visibility `unlisted` by default (`EveryOptions.Visibility` takes
  `public` or `local`) and the Idempotency-Key `mybot:every:<name>:<time in RFC 3339>` (`mybot.EveryIdempotencyKey`):
  two processes of the bot, or a restart in the middle of a run, post it once.
- The times missed while the bot was stopped are skipped by default; `CatchUp: mybot.Latest` runs the newest of them
  once when the bot starts (for a job that never ran, its newest past time), and never a pile of them.
- `Name` (the expression by default) keys the last run and the idempotency key: keep it when the expression changes.
- `bot.RunOnce(ctx)` handles what is due, the notifications since the Store's position and each job's newest past
  time after its last run, and returns: for a bot that cron or a job runner such as Cloud Run jobs starts again and
  again. Such a bot needs a Store that outlives the process.

## Manners

All on by default, and each can be turned off but the first:

| Default | How to change it |
| --- | --- |
| The bot never handles the events of its own account. | (always on) |
| It does not answer accounts with the bot flag, so that two bots never answer each other forever. | `RespondToBots: true` |
| It replies to one sender at most 5 times in 10 minutes and 15 times in an hour; the events past that are skipped. | `ReplyLimits`, or `DisableReplyLimits: true` |
| A reply is never more visible than the post it answers: `direct`, `private`, `local`, and `unlisted` keep theirs, and a reply to a `public` post is `unlisted`. | post with `c.API()` |
| A reply mentions its sender alone, never the other accounts of the post. | post with `c.API()` |
| Writes wait for a budget of the client: 60 posts an hour and 60 other writes a minute, and, when less than a tenth of the server's rate limit remains, for the limit to reset. A write that would wait longer than 10 minutes is dropped and logged. | `PostRate`, `WriteRate`, `MaxSendWait` |
| The bot refuses to start on an account without the bot flag, which tells people and other servers that a program posts. | turn on **This is a bot** in **Settings → Profile**, `SetBotFlag: true` (the bot sets it, with `write:accounts`), or `AllowNonBot: true` |

Posting from an app never sets the bot flag by itself: the flag belongs to the account. Writes through `c.API()` skip
the budget and the reply limits; give their posts an `Idempotency-Key` from `mybot.ReplyIdempotencyKey`.

The visibility `local` reaches the accounts of the bot's own server and its followers alone; it is never delivered to
other servers beyond the followers. `unlisted` is public but stays out of the public timelines.

These are a floor: **read the rules of the server** (whether it allows bots, how often they may post) before you run
one, and set the limits lower when they ask for it.

## State

The `Store` keeps the processed notifications (for 7 days, `Config.ProcessedRetention`), the position of the catch-up,
the last runs of the scheduled jobs, and the rotated refresh token.

- `FileStore` (the default, at `./mybot-state.json`) is one JSON file, rewritten through a temporary file and a rename,
  readable by its owner alone, in the same format as the TypeScript library's. Put it on a disk that outlives the
  process, such as a volume of the container. A change that succeeds is flushed to the disk, the directory too
  except on Windows; a change that fails leaves both the file and the store as they were.
- When the Store fails, such as on a full disk, the bot logs the error and tries the call again, waiting longer each
  time up to a minute. The notification stays unprocessed meanwhile, and the position of the catch-up does not move
  past it.
- `MemoryStore` is for tests. A restart loses the rotated refresh token, so the next start sends the used one, the
  server revokes the token family, and the bot can no longer sign in until you issue a new token.
- Implement `Store` to keep the state elsewhere, such as in a database.

Run one process per bot: the Store takes no lock. Two processes do not post twice (the idempotency keys), but the order
of one conversation is not kept.

## Shutting down

When the context of `Run` ends (`signal.NotifyContext` on SIGINT and SIGTERM), the bot closes the event stream, takes no
new events, and waits `Config.ShutdownGrace` (10 seconds) for the running handlers. Then it cancels the contexts of
those still running; their notifications stay unprocessed and are handled again by the next start, without posting
twice. Give a container longer than the grace period to stop: `docker stop` waits 10 seconds by default, so use
`docker stop -t 30` (or `stop_grace_period` in Compose).

## Testing

The package `mybottest` is a fake server of the operations that a bot calls, for the tests of its handlers without a
server. `srv.Config()` gives a bot its URL, a token, a `MemoryStore`, and the server's `Clock`, which moves only when
the test moves it:

```go
srv := mybottest.NewServer()
defer srv.Close()
bot, _ := mybot.New(srv.Config())
bot.OnCommand("ping", "answer pong", func(c *mybot.Context) error {
    _, err := c.Reply("pong")
    return err
})
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
go bot.Run(ctx)

srv.Inject(srv.Mention(srv.Account("alice"), "ping", mykit.VisibilityPublic))
posts, _ := srv.WaitPosted(ctx, 1) // posts[0].Request.Text is "@alice pong"
```

- `Inject` sends a notification on the event stream; `InjectMissed` adds one that only the catch-up finds;
  `DropStreams`, `Disconnect`, and `Reconnect` cut the stream.
- `Posted`, `Reactions`, `Favourites`, and `Follows` return what the bot did; like the server, it posts once per
  Idempotency-Key.
- `srv.Clock().WaitForSleepers(ctx, n)` waits for the scheduled jobs (and the pruning of the Store) to sleep, and
  `Advance` or `Set` moves them on. The `Clock` starts at the current time; a test of fixed times starts it before
  them with `mybottest.NewServer(mybottest.WithClockAt(t))`, since `Set` cannot move it back, so that the test runs
  the same on any day.

The [examples](#examples) have tests of each kind.

## Versions

mybot is released with the server and mykit under the same version number (the tag `pkg/mybot/vX.Y.Z`). Before
v1, a minor release may change the library; a library newer than the server is not promised to work with it.

## License

MIT ([LICENSE](LICENSE)), apart from the server, which is AGPL-3.0-only.
