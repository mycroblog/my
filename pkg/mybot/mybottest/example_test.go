// SPDX-License-Identifier: MIT

package mybottest_test

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/pkg/mybot"
	"github.com/mycroblog/my/pkg/mybot/mybottest"
	"github.com/mycroblog/my/pkg/mykit"
)

// A test of a command: the fake server sends the bot a mention, and the test reads the reply that it posted.
func Example() {
	srv := mybottest.NewServer()
	defer srv.Close()

	bot, err := mybot.New(srv.Config())
	if err != nil {
		panic(err)
	}
	bot.OnCommand("echo", "say the text again", func(c *mybot.Context) error {
		_, err := c.Reply(c.Args)
		return err
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()

	alice := srv.Account("alice")
	srv.Inject(srv.Mention(alice, "echo hello", mykit.VisibilityPublic))
	posts, err := srv.WaitPosted(ctx, 1)
	if err != nil {
		panic(err)
	}
	p := posts[0]
	fmt.Println(p.Request.Text)
	fmt.Println(p.Request.Visibility)
	fmt.Println(p.IdempotencyKey == mybot.ReplyIdempotencyKey(srv.Notifications()[0].Id, 1))

	cancel()
	if err := <-done; err != nil {
		panic(err)
	}
	// Output:
	// @alice hello
	// unlisted
	// true
}

// A test of a scheduled job: the bot's clock is the server's, which the test moves on to the job's next time.
func Example_every() {
	srv := mybottest.NewServer()
	defer srv.Close()
	srv.Clock().Set(time.Date(2026, 10, 25, 8, 59, 0, 0, time.UTC))

	bot, err := mybot.New(srv.Config())
	if err != nil {
		panic(err)
	}
	if err := bot.Every("0 9 * * *", func(c *mybot.Context) error {
		_, err := c.Post("Good morning")
		return err
	}, mybot.EveryOptions{Name: "morning"}); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()

	// The job and the pruning of the Store sleep on the clock.
	if err := srv.Clock().WaitForSleepers(ctx, 2); err != nil {
		panic(err)
	}
	srv.Clock().Advance(time.Minute)
	posts, err := srv.WaitPosted(ctx, 1)
	if err != nil {
		panic(err)
	}
	fmt.Println(posts[0].Request.Text, posts[0].Request.Visibility)
	fmt.Println(posts[0].IdempotencyKey)

	cancel()
	if err := <-done; err != nil {
		panic(err)
	}
	// Output:
	// Good morning unlisted
	// mybot:every:morning:2026-10-25T09:00:00Z
}
