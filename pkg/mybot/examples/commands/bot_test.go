// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mybot/mybottest"
	"github.com/mycroblog/my/pkg/mykit"
)

func TestCommands(t *testing.T) {
	srv := mybottest.NewServer()
	defer srv.Close()
	// The dice of the test throw 1, 2, 3, ... in turn.
	next := 0
	bot, err := newBot(srv.Config(), func(n int) int {
		next++
		return (next-1)%n + 1
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()

	alice, bob := srv.Account("alice"), srv.Account("bob")
	for i, tc := range []struct {
		from *mykit.Account
		text string
		want string
	}{
		{alice, "help", "@alice dice: throw dice, such as dice 2d6\nhelp: list the commands\nping: answer pong"},
		{alice, "PING", "@alice pong"},
		{alice, "dice 3d6", "@alice 🎲 1 + 2 + 3 = 6"},
		{bob, "dice", "@bob 🎲 4"},
		{bob, "dice 99d6", "@bob " + errDice.Error()},
		{bob, "hello", "@bob I do not know that command. Try help."},
	} {
		srv.Inject(srv.Mention(tc.from, tc.text, mykit.VisibilityPublic))
		posts, err := srv.WaitPosted(ctx, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if got := posts[i].Request.Text; got != tc.want {
			t.Errorf("%q: replied %q, want %q", tc.text, got, tc.want)
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestParseDice(t *testing.T) {
	for _, tc := range []struct {
		args     string
		n, sides int
		ok       bool
	}{
		{"", 1, 6, true},
		{"2d6", 2, 6, true},
		{"D20", 1, 20, true},
		{"10d1000", 10, 1000, true},
		{"11d6", 0, 0, false},
		{"1d1", 0, 0, false},
		{"0d6", 0, 0, false},
		{"six", 0, 0, false},
		{"2dx", 0, 0, false},
	} {
		n, sides, err := parseDice(tc.args)
		if (err == nil) != tc.ok || n != tc.n || sides != tc.sides {
			t.Errorf("parseDice(%q) = %d, %d, %v", tc.args, n, sides, err)
		}
	}
}
