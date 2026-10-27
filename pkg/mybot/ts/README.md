<!-- SPDX-License-Identifier: MIT -->

# @mycroblog/mybot

Build bots for [my](https://github.com/mycroblog/my), an ActivityPub microblog server, on its API client
[`@mycroblog/mykit`](https://github.com/mycroblog/my/blob/main/pkg/mykit/ts/README.md). You write the handlers of the bot's events; the library receives the
events so that none is lost or answered twice across disconnections and restarts, and keeps the bot polite by default.
It is ESM only and runs on Node.js 22 or later, Bun, and Deno; it depends on `@mycroblog/mykit` and on
[`croner`](https://github.com/hexagon/croner) (MIT, no dependencies), which computes the times of the scheduled jobs.

The Go library `github.com/mycroblog/my/pkg/mybot` has the same handlers, options, defaults, idempotency keys, and
state file; the [differences](#differences-from-the-go-library) are at the end.

```sh
npm install @mycroblog/mybot
```

## Quick start

```ts
import { Bot } from "@mycroblog/mybot";

const bot = new Bot({
  server: "https://example.com",
  oauth: { clientId: process.env.CLIENT_ID!, clientSecret: process.env.CLIENT_SECRET! },
  refreshToken: process.env.REFRESH_TOKEN!,
  userAgent: "echobot/1.0",
});

bot.onCommand("ping", "answer pong", async (ctx) => {
  await ctx.reply("pong");
});
bot.onMention(async (ctx) => {
  await ctx.reply(ctx.text);
});

const controller = new AbortController();
process.once("SIGINT", () => controller.abort());
process.once("SIGTERM", () => controller.abort());
await bot.run({ signal: controller.signal });
```

`bot.run` verifies the account, receives the events until the signal aborts, then shuts down gracefully and resolves.
It rejects when the bot cannot start: `NotBotError`, `ScopeError` (with `missing`), or a `ProblemError` of the server.

## Examples

Each example is a `createBot` in `bot.ts`, its tests against [`@mycroblog/mybot/testing`](#testing) in `bot.test.ts`,
and a `main.ts` that runs it; the Go library has the same examples.

| Example | What it does |
| --- | --- |
| [`examples/echo`](https://github.com/mycroblog/my/blob/main/pkg/mybot/ts/examples/echo) | replies to every mention with its text (`onMention`) |
| [`examples/schedule`](https://github.com/mycroblog/my/blob/main/pkg/mybot/ts/examples/schedule) | posts a greeting at 9:00 every morning in a time zone (`every` with `catchUp: "latest"`) |
| [`examples/commands`](https://github.com/mycroblog/my/blob/main/pkg/mybot/ts/examples/commands) | answers the commands `help`, `ping`, and `dice 2d6` (`onCommand`) |

The bots read `MYBOT_SERVER`, `MYBOT_CLIENT_ID`, `MYBOT_CLIENT_SECRET`, `MYBOT_REFRESH_TOKEN`, and `MYBOT_STATE` (the
state file, `./mybot-state.json` by default) from the environment ([`examples/env.ts`](https://github.com/mycroblog/my/blob/main/pkg/mybot/ts/examples/env.ts)):

```sh
MYBOT_SERVER=https://example.com MYBOT_CLIENT_ID=... MYBOT_CLIENT_SECRET=... MYBOT_REFRESH_TOKEN=... \
  bun examples/echo/main.ts
```

## Getting a token

A bot always acts as a user: run it on an account of its own. The steps:

1. Sign in to the server as the bot's account and open **Settings → Development** (`/settings/apps`).
2. Register an app with the type **Confidential**. The form needs a redirect URI even when no redirect is used:
   `http://127.0.0.1/callback` serves both this and the Go library's `examples/login`. Give it at least the scopes
   below.
3. Copy the client ID and the secret, which is shown once.
4. Press **Issue token** on the app and choose the scopes (empty for all of the app's). The server issues an access
   token and a refresh token for your own account, without a redirect, and shows them once.
5. Pass the client ID and the secret as `oauth` and the refresh token as `refreshToken`. The access token is not
   needed: the bot gets a new one with the refresh token.

To authorize a bot through the consent page instead, use the Authorization Code flow with PKCE (mykit's
`newCodeVerifier`, `codeChallenge`, and `exchange`): the Go library's
[`examples/login`](https://github.com/mycroblog/my/blob/main/pkg/mybot/examples/login) receives the code on a loopback
redirect and prints the refresh token, which works for a bot in either language.

The scopes the default handlers need are `read:accounts`, `read:notifications`, `read:statuses` (for the event
stream), and `write:statuses`, plus `write:accounts` with `setBotFlag`. A token without them stops the bot at start.

The server rotates the refresh token on every renewal and revokes the whole token family when a used one comes back.
The bot saves every rotated refresh token in its [store](#state) and, on the next start, uses the saved one rather than
`refreshToken`. A `tokenSource` (such as mykit's `staticToken`) instead of `oauth` is for tests and short runs: the bot
saves nothing for it.

## Receiving the events

- In the default mode `"streaming"` the bot opens the event stream (`GET /api/v1/streaming`; mykit reconnects it).
  After every connection, the first and each reconnection, it reads the notifications newest first back to the position
  in its store, or to `catchUpLimit` (24 hours; `0` turns the catch-up off), and handles those it missed, oldest first.
- The notifications reach the handlers in the order of time. Those that the event stream brings while a catch-up
  runs wait for it, and the bot then hands out the missed ones and the waiting ones together, oldest first. When the
  catch-up fails, the bot tries it again, waiting longer each time up to a minute, and the stream's notifications
  keep waiting.
- Stream delivery is best effort. The catch-up covers the time the stream is closed: a stop of the bot, or a cut
  connection, because it runs at the start and after every reconnection and reads back from the newest notification
  the bot processed. A notification that the server sends while the stream stays open but that never reaches the bot,
  such as one a proxy drops, is older than the next one the bot processes, so no later catch-up finds it. A handler
  that returns an error is not part of this gap: the error is logged and its notification counts as processed, and
  the bot does not run the handler again (the store failures are retried, as described below). A bot that needs every
  notification should use `mode: "polling"`, reconnect periodically (for example by restarting), or list the
  notifications itself.
- Every notification is recorded in the store after its handler returns, and one recorded before is not handled again.
  A process that stops in the middle of a handler handles that notification again on the next start: replies carry an
  `Idempotency-Key` derived from the notification (`replyIdempotencyKey`), so the server posts each of them once.
- `mode: "polling"` reads the notifications every `pollInterval` (a minute) instead, for networks whose proxies do not
  pass an event stream.
- `onStatus` receives the new posts of the home timeline from the event stream alone: they are not caught up after a
  disconnection or a restart, and the polling mode does not receive them.
- Up to `concurrency` (4) handlers run at once; the events of one sender run one after the other, in the order the
  bot hands them out. The handlers of different senders run side by side, so one may end before an older one of
  another sender.

## Handlers

| Handler | Runs for |
| --- | --- |
| `onMention` | a mention, replies and direct posts included (`ctx.isReply`, `ctx.isDirect`), that is no command |
| `onCommand(name, description, h)` | a mention whose first word after the bot's own mentions is `name`, without case |
| `onFollow` / `onFollowRequest` | `follow` / `follow_request` |
| `onReaction` / `onFavourite` / `onBoost` / `onQuote` | the notification of the same name |
| `onNotification` | every notification that no handler above takes |
| `onStatus` | a new post of the home timeline (the bot's own excepted) |

The context `ctx` has the `notification`, its `status` and sender `account`, `text` (the post as plain text), `command`
and `args`, `me`, `api` (the mykit client), `logger`, and `signal`, which aborts when the bot stops. Its helpers
`reply(text)`, `react(emoji)`, `favourite()`, and `follow()` keep the defaults below. A handler that throws is logged,
and its notification counts as processed. `help` answers with the registered commands and their descriptions unless
`disableHelp` is set or a command `help` is registered.

Middleware is `(ctx, next) => Promise<void>`, added with `bot.use(...)` inside the default chain `recover`, `manners`,
`sendBudget` (`noDefaultMiddleware` leaves that chain out):

```ts
bot.use(async (ctx, next) => {
  if (ctx.command === "shutdown" && ctx.account?.acct !== "admin") return; // skip the handler
  await next();
});
```

The log goes to `console`, or to any `logger` with `debug`, `info`, `warn`, and `error`; each call has a message and
an object of attributes.

## Scheduled jobs

`every` runs a handler at the times of a cron expression of five fields, minute hour day-of-month month day-of-week,
in UTC or the IANA time zone `timeZone`:

```ts
bot.every(
  "0 9 * * *",
  async (ctx) => {
    await ctx.post("Good morning");
  },
  { name: "morning", timeZone: "Asia/Tokyo" },
);
```

- A field is `*`, a number, a range `a-b`, a step `*/n` or `a-b/n`, or a list of those; the day of the week is 0-7,
  0 and 7 being Sunday, and a day matches when either day field does when both are other than `*`. A field of seconds,
  the aliases such as `@daily`, the names of months and days, and `?`, `L`, `W`, and `#` throw a `ScheduleError`, so
  that the Go library and this one take the same expressions. `parseSchedule(expr, timeZone)` gives the times without
  a bot.
- The times are wall-clock times of the time zone: one that a daylight saving time change skips does not run, and one
  that the change repeats runs once.
- `ctx.post(text)` posts once per run, with the visibility `unlisted` by default (`visibility: "public"` or `"local"`)
  and the Idempotency-Key `mybot:every:<name>:<time in RFC 3339>` (`everyIdempotencyKey`): two processes of the bot, or
  a restart in the middle of a run, post it once. `ctx.job` and `ctx.scheduledAt` name the run.
- The run counts as the job's last run in the store after the handler returns. The times missed while the bot was
  stopped are skipped by default; `catchUp: "latest"` runs the newest of them once when the bot starts (for a job that
  never ran, its newest past time), and never a pile of them.
- `name` (the expression by default) keys the last run and the idempotency key: keep it when the expression changes.

## Manners

All on by default, and each can be turned off but the first:

- The bot never handles the events of its own account.
- It does not answer accounts with the bot flag (`respondToBots: true` answers them).
- It replies to one sender at most 5 times in 10 minutes and 15 times in an hour; the events past that are skipped
  (`replyLimits`, or `disableReplyLimits: true`).
- A reply is never more visible than the post it answers: `direct`, `private`, `local`, and `unlisted` keep theirs, and
  a reply to a `public` post is `unlisted`. A `local` post reaches the accounts of the bot's server and its followers
  alone.
- A reply mentions its sender alone, never the other accounts of the post.
- Writes wait for a budget of the client: 60 posts an hour and 60 other writes a minute (`postRate`, `writeRate`), and,
  when less than a tenth of the server's rate limit remains (its `RateLimit` fields), for the limit to reset. A write
  that would wait longer than `maxSendWait` (10 minutes) is dropped with a `BudgetExceededError` and logged.
- The bot refuses to start on an account without the bot flag, which tells people and other servers that a program
  posts: turn on **This is a bot** in **Settings → Profile**, or pass `setBotFlag: true` (the bot sets it, with
  `write:accounts`) or `allowNonBot: true`. Posting from an app never sets the flag by itself.

Writes through `ctx.api` skip the budget and the reply limits: give their posts an `Idempotency-Key` from
`replyIdempotencyKey`.

These are a floor: **read the rules of the server** (whether it allows bots, how often they may post) before you run
one, and set the limits lower when they ask for it.

## State

The `store` keeps the processed notifications (for 7 days, `processedRetention`), the position of the catch-up, the last
runs of the scheduled jobs, and the rotated refresh token. The default is a `FileStore` at `./mybot-state.json`: one JSON file, rewritten through a
temporary file and a rename, readable by its owner alone, in the same format as the Go library's. A change that succeeds
is flushed to the disk, the directory too except on Windows; a change that fails leaves both the file and the store as
they were. When the store fails, such as on a full disk, the bot logs the error and tries the call again, waiting
longer each time up to a minute. The notification stays unprocessed meanwhile, and the position of the catch-up does
not move past it. `MemoryStore` is for
tests: a restart loses the rotated refresh token, and the next start sends the used one, which revokes the token. A bot
that runs all the time, or is started again and again, needs a `FileStore` or a `Store` of its own (such as in a
database). Run one process per bot: the store takes no lock.

## Shutting down

When the signal aborts, the bot closes the event stream, takes no new events, and waits `shutdownGrace` (10 seconds) for
the running handlers. Then it aborts `ctx.signal` of those still running; their notifications stay unprocessed and are
handled again by the next start (without posting twice, through the idempotency keys). Give a container longer than
the grace period to stop: `docker stop` waits 10 seconds by default, so use `docker stop -t 30`.

## Running once

`bot.runOnce({ signal })` catches up the notifications since the store's position (back to `catchUpLimit`), runs each
scheduled job's newest past time after its last run, whatever its `catchUp`, waits for the handlers, and resolves: for a
bot that cron or a job runner such as Cloud Run jobs starts again and again. Such a bot needs a store that outlives the
process.

## Testing

`@mycroblog/mybot/testing` tests a bot without a server. `Server` is a fake server of the client API behind a `fetch`
that answers the operations that the bot calls with the types of the API; `options()` gives a bot its `fetch`, a token,
a `MemoryStore`, and the server's `Clock`, which moves only when the test moves it:

```ts
import { Bot } from "@mycroblog/mybot";
import { Server } from "@mycroblog/mybot/testing";

const srv = new Server();
const bot = new Bot(srv.options());
bot.onCommand("ping", "answer pong", async (ctx) => {
  await ctx.reply("pong");
});
const controller = new AbortController();
const done = bot.run({ signal: controller.signal });
srv.inject(srv.mention(srv.account("alice"), "ping", "public"));
const [post] = await srv.waitPosted(1); // post.request.text is "@alice pong"
controller.abort();
await done;
```

- `inject` sends a notification on the event stream; `injectMissed` adds one that only the catch-up finds;
  `dropStreams`, `disconnect`, and `reconnect` cut the stream.
- `posted()`, `reactions()`, `favourites()`, and `follows()` return what the bot did; like the server, it posts once
  per Idempotency-Key.
- `clock.waitForSleepers(n)` waits for the scheduled jobs (and the pruning of the store) to sleep, and
  `clock.advance(ms)` or `clock.set(at)` moves them on. The clock starts at the current time; a test of fixed times
  starts it before them with `new Server({ now: Date.parse("2026-10-25T08:59:00Z") })`, since `clock.set` cannot move
  it back, so that the test runs the same on any day.

## Runtimes

The package uses `fetch`, web streams, and `AbortSignal`. `FileStore` loads `node:fs/promises` on first use, which
Node.js, Bun, and Deno provide; on Deno, run the bot with `--allow-net` and with `--allow-read` and `--allow-write` for
the state file. With another `Store`, nothing of `node:` is loaded.

## Differences from the Go library

- Durations are milliseconds, and times in the `Store` are milliseconds since the Unix epoch.
- A missing value of the `Store` is `null` (the Go library uses `""` and the zero time).
- Handlers throw instead of returning an error; the errors are classes (`NotBotError`, `ScopeError`,
  `ReplyLimitError`, `BudgetExceededError`, `NoStatusError`, `NoAccountError`, `AlreadyRunningError`).
- Middleware is `(ctx, next) => Promise<void>` rather than a function that wraps a handler, and `ctx.signal` replaces
  the `context.Context` of the handler.
- The scopes of a token are checked when they are known: for `oauth`, from the renewal; a `tokenSource` is not checked.
- `User-Agent` ends with `mybot-ts`.
- `catchUp` is `"skip"` or `"latest"`, and `ctx.scheduledAt` is `undefined` for an event.
- The clock is the options `now` and `sleep` rather than a `Clock` interface.
- The fake server of `@mycroblog/mybot/testing` is a `fetch` rather than an HTTP server, and its waits take a signal
  (10 seconds by default) rather than a `context.Context`.
