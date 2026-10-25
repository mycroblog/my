// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mybot"
	"github.com/mycroblog/my/pkg/mybot/mybottest"
	"github.com/mycroblog/my/pkg/mykit"
)

// start starts bot and returns its context and a function that stops it and checks that it stopped cleanly.
func start(t *testing.T, bot *mybot.Bot) (context.Context, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()
	return ctx, func() {
		t.Helper()
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMorning(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	srv := mybottest.NewServer(mybottest.WithClockAt(time.Date(2026, 10, 26, 8, 59, 0, 0, tokyo)))
	defer srv.Close()
	cfg := srv.Config()
	// The bot posted yesterday's greeting, so CatchUp Latest has nothing to run when it starts. A job that never
	// ran would post its newest past time, yesterday's 9:00, at once.
	if err := cfg.Store.SetLastRun(t.Context(), "morning", time.Date(2026, 10, 25, 9, 0, 0, 0, tokyo)); err != nil {
		t.Fatal(err)
	}
	bot, err := newBot(cfg, "Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := start(t, bot)
	defer stop()

	// The job and the pruning of the Store sleep on the clock until the test moves it on to 9:00.
	if err := srv.Clock().WaitForSleepers(ctx, 2); err != nil {
		t.Fatal(err)
	}
	srv.Clock().Advance(time.Minute)
	posts, err := srv.WaitPosted(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	p := posts[0]
	if want := "Good morning! Today is Monday, 26 October 2026."; p.Request.Text != want {
		t.Errorf("text = %q, want %q", p.Request.Text, want)
	}
	if p.Request.Visibility != mykit.VisibilityUnlisted {
		t.Errorf("visibility = %s, want unlisted", p.Request.Visibility)
	}
	if want := "mybot:every:morning:2026-10-26T00:00:00Z"; p.IdempotencyKey != want {
		t.Errorf("Idempotency-Key = %q, want %q", p.IdempotencyKey, want)
	}
}

// A bot that was stopped for three mornings posts the newest of them once when it starts, with CatchUp Latest.
func TestMorningCatchUp(t *testing.T) {
	srv := mybottest.NewServer(mybottest.WithClockAt(time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)))
	defer srv.Close()
	cfg := srv.Config()
	if err := cfg.Store.SetLastRun(t.Context(), "morning", time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	bot, err := newBot(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := start(t, bot)
	defer stop()

	posts, err := srv.WaitPosted(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Good morning! Today is Monday, 26 October 2026."; posts[0].Request.Text != want {
		t.Errorf("text = %q, want %q", posts[0].Request.Text, want)
	}
	// Nothing else is due until the next morning.
	if err := srv.Clock().WaitForSleepers(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if n := len(srv.Posted()); n != 1 {
		t.Errorf("posted %d times, want once", n)
	}
}
