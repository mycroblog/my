// SPDX-License-Identifier: MIT

// Package mybot builds bots for my, an ActivityPub microblog server, on the API client mykit (ADR-0094).
//
// The author of a bot writes the handlers of its events; the library receives the events, so that none is lost
// or answered twice, and keeps the bot polite by default:
//
//	bot, err := mybot.New(mybot.Config{
//		Server:       "https://example.com",
//		OAuth:        &mykit.OAuthConfig{ClientID: id, ClientSecret: secret},
//		RefreshToken: refreshToken,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	bot.OnCommand("ping", "answer pong", func(c *mybot.Context) error {
//		_, err := c.Reply("pong")
//		return err
//	})
//	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
//	defer stop()
//	if err := bot.Run(ctx); err != nil {
//		log.Fatal(err)
//	}
//
// Receiving: Run verifies the account (verifyCredentials) and opens the event stream; after every connection,
// the first and each reconnection, it reads the notifications (listNotifications) back to the position in the
// Store, or to Config.CatchUpLimit (24 hours by default, zero turns it off), and handles those it missed. The
// notifications reach the handlers in the order of time: those of the stream wait for a running catch-up, which
// is tried again until it succeeds, and then both are handed out oldest first. Every notification is recorded in
// the Store after its handler returns (at least once), and one recorded before is skipped. Mode Polling reads the
// notifications every minute instead. The new posts of the home timeline (OnStatus) come from the event stream
// alone and are not caught up. Stream delivery is best effort: the catch-up covers the time the stream is closed,
// not a notification that the server sends while the stream stays open but that never reaches the bot (a proxy
// drops it), which no later catch-up finds; a bot that needs every notification uses Mode Polling, reconnects
// periodically, or lists the notifications itself. A handler's error is logged and is not retried.
//
// Scheduled jobs: Every runs a handler at the times of a cron expression of five fields (minute hour day month
// weekday; numbers, ranges a-b, steps */n, and lists a,b) in an IANA time zone, UTC by default. The times missed
// while the bot was stopped are skipped, or with CatchUp Latest the newest of them runs once at the start. The
// Context's Post posts with the visibility unlisted (public and local on request) and the Idempotency-Key
// mybot:every:<name>:<time>, so that one time posts once. RunOnce handles what is due, the notifications and the
// scheduled jobs, once and returns, for a bot that a cron starts. Config.Clock replaces the clock in tests.
//
// State: the Store keeps the processed notifications (for 7 days), the position, the last runs of the scheduled
// jobs, and the rotated refresh token.
// The default is a FileStore at ./mybot-state.json; MemoryStore is for tests.
//
// Handlers: OnMention, OnCommand, OnFollow, OnFollowRequest, OnReaction, OnFavourite, OnBoost, OnQuote,
// OnNotification, and OnStatus. The Context of a handler carries the event, its plain text, and the helpers
// Reply, React, Favourite, and Follow. Up to 4 handlers run at once, those of one sender in order. Middleware
// wraps every handler; the default chain recovers and logs (log/slog), applies the manners, and the send budget.
//
// Manners (ADR-0094 §8), each of which can be turned off in Config but the first: no event of the bot's own
// account reaches a handler; accounts with the bot flag are not answered; one sender gets at most 5 replies in
// 10 minutes and 15 in an hour; a reply is never more visible than the post it answers, and a public post gets
// an unlisted reply; a reply mentions its sender alone. The bot refuses to start on an account without the bot
// flag (Config.SetBotFlag sets it). Replies carry an Idempotency-Key derived from the event, so that handling an
// event again posts nothing twice, and writes wait for a client-side budget (60 posts an hour, 60 other writes a
// minute) and for the server's rate limit.
//
// Tests: the package mybottest is a fake server of the operations that a bot calls, for the tests of a bot's
// handlers without a server: it sends the bot notifications (Inject), returns what the bot posted (Posted), and
// moves the clock of its scheduled jobs.
//
// The library is MIT-licensed, apart from the AGPL-3.0 server.
package mybot
