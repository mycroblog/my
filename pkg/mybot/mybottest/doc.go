// SPDX-License-Identifier: MIT

// Package mybottest tests bots of mybot without a server (ADR-0094 §10). Server is a fake server of the client
// API, an httptest.Server that answers the operations that a bot calls with the types of api/openapi.yaml; the
// test sends the bot notifications with Inject and checks what it posted with Posted or WaitPosted:
//
//	srv := mybottest.NewServer()
//	defer srv.Close()
//	bot, err := mybot.New(srv.Config())
//	...
//	bot.OnCommand("ping", "answer pong", func(c *mybot.Context) error {
//		_, err := c.Reply("pong")
//		return err
//	})
//	go bot.Run(ctx)
//	srv.Inject(srv.Mention(srv.Account("alice"), "ping", mykit.VisibilityPublic))
//	posts, err := srv.WaitPosted(ctx, 1) // posts[0].Request.Text == "@alice pong"
//
// Server.Config gives the bot the server's Clock, which moves only when the test moves it (Clock.Advance), so that
// a test runs the scheduled jobs (Every) at once; it starts at the current time, or at the time of WithClockAt for a
// test of fixed times, which Clock.Set cannot reach once they are past. InjectMissed adds a notification that only the bot's catch-up
// finds, such as one that came while the bot was stopped; DropStreams ends the event streams, for the bot to connect
// again and catch up, and Disconnect holds its connections until Reconnect, so that the notifications of Inject
// meanwhile come by the catch-up alone. Like the server, Server posts once per Idempotency-Key.
//
// The fake answers what the library needs and no more: it keeps no timelines, follows, or visibility rules. The
// integration tests of mybot check its answers against api/openapi.yaml.
package mybottest
