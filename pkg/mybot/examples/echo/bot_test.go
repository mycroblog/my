// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mybot/mybottest"
	"github.com/mycroblog/my/pkg/mykit"
)

func TestEcho(t *testing.T) {
	srv := mybottest.NewServer()
	defer srv.Close()
	bot, err := newBot(srv.Config())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()

	alice := srv.Account("alice")
	srv.Inject(srv.Mention(alice, "hello, world", mykit.VisibilityPrivate))
	srv.Inject(srv.Mention(alice, "how are you?", mykit.VisibilityPublic))
	posts, err := srv.WaitPosted(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		text       string
		visibility mykit.Visibility
	}{
		// The reply keeps the visibility of a private mention, and a public one gets an unlisted reply.
		{"@alice hello, world", mykit.VisibilityPrivate},
		{"@alice how are you?", mykit.VisibilityUnlisted},
	}
	for i, w := range want {
		p := posts[i]
		if p.Request.Text != w.text || p.Request.Visibility != w.visibility {
			t.Errorf("post %d = %q (%s), want %q (%s)", i, p.Request.Text, p.Request.Visibility, w.text, w.visibility)
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWithoutMentions(t *testing.T) {
	for in, want := range map[string]string{
		"@bot hello":            "hello",
		"@bot @alice  hi there": "hi there",
		"@bot":                  "",
		"hello @bot":            "hello @bot",
	} {
		if got := withoutMentions(in); got != want {
			t.Errorf("withoutMentions(%q) = %q, want %q", in, got, want)
		}
	}
}
