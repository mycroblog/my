// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/voice"
)

// roomsNotifier records the notifications of the rooms.
type roomsNotifier struct {
	mu      sync.Mutex
	started map[string][]string // room -> recipients
	invited map[string][]string // room -> invitees
}

func (n *roomsNotifier) RoomStartedTx(_ context.Context, _ uow.Tx, roomID, _, recipientID string, _ time.Time) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.started == nil {
		n.started = map[string][]string{}
	}
	if !slices.Contains(n.started[roomID], recipientID) {
		n.started[roomID] = append(n.started[roomID], recipientID)
	}
	return nil
}

func (n *roomsNotifier) RoomInvitedTx(_ context.Context, _ uow.Tx, _, roomID, _, inviteeID string, _ time.Time) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.invited == nil {
		n.invited = map[string][]string{}
	}
	n.invited[roomID] = append(n.invited[roomID], inviteeID)
	return nil
}

// recipients returns the recipients of voice_room_started of a room, in ID order.
func (n *roomsNotifier) recipients(roomID string) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := slices.Clone(n.started[roomID])
	slices.Sort(out)
	return out
}

// roomsPosts records the start posts: what Open prepared, and how many it wrote.
type roomsPosts struct {
	mu       sync.Mutex
	prepared []appcontent.CreateInput
	written  int
}

func (p *roomsPosts) PrepareCreate(_ context.Context, in appcontent.CreateInput) (appcontent.PreparedCreate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepared = append(p.prepared, in)
	return appcontent.PreparedCreate{}, nil
}

func (p *roomsPosts) CreateTx(context.Context, uow.Tx, appcontent.PreparedCreate) (appcontent.Post, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.written++
	return appcontent.Post{}, nil
}

// roomsNotifications checks the notifications of the rooms (ADR-0100 §8): voice_room_started goes
// to the followers of the host who see the room, once each, and to nobody for a room by
// invitation; a follower in a block relation with the host gets none; voice_room_invited goes
// to the invited account.
func roomsNotifications(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	follower, mutual, blocked, stranger := e.account(false), e.account(false), e.account(false), e.account(false)
	e.members.set(func() {
		for _, a := range []string{follower, mutual, blocked} {
			e.members.follows[[2]string{a, host}] = true
		}
		e.members.follows[[2]string{host, mutual}] = true
		e.members.blocks[[2]string{host, blocked}] = true
	})
	want := func(room string, accounts ...string) {
		t.Helper()
		slices.Sort(accounts)
		if got := e.notices.recipients(room); !slices.Equal(got, accounts) {
			t.Errorf("voice_room_started of %s went to %v, want %v", room, got, accounts)
		}
	}
	for _, tc := range []struct {
		visibility voice.Visibility
		want       []string
	}{
		{voice.VisibilityLocal, []string{follower, mutual}},
		{voice.VisibilityFollowers, []string{follower, mutual}},
		{voice.VisibilityMutuals, []string{mutual}},
		{voice.VisibilityInvited, nil},
	} {
		j := e.open(t, host, appvoice.SettingsInput{Visibility: tc.visibility})
		want(j.Room.ID, tc.want...)
		if err := e.svc.End(ctx, host, j.Room.ID); err != nil {
			t.Fatal(err)
		}
	}
	// The job runs again as it ran (the notification module keeps one per room and recipient),
	// and one of a room that ended notifies nobody.
	j := e.open(t, host, appvoice.SettingsInput{})
	var job queue.Job
	for _, r := range e.jobs.Results() {
		if r.Job.Type == appvoice.JobNotifyStarted {
			job = r.Job
		}
	}
	e.notices.mu.Lock()
	delete(e.notices.started, j.Room.ID)
	e.notices.mu.Unlock()
	if r := e.svc.NotifyStarted(ctx, job); !r.IsDone() {
		t.Errorf("NotifyStarted again = %+v", r)
	}
	want(j.Room.ID, follower, mutual)

	if _, err := e.svc.Invite(ctx, host, j.Room.ID, stranger, voice.RoleListener); err != nil {
		t.Fatal(err)
	}
	e.notices.mu.Lock()
	invited := slices.Clone(e.notices.invited[j.Room.ID])
	e.notices.mu.Unlock()
	if !slices.Equal(invited, []string{stranger}) {
		t.Errorf("voice_room_invited went to %v, want %v", invited, []string{stranger})
	}
	if err := e.svc.End(ctx, host, j.Room.ID); err != nil {
		t.Fatal(err)
	}
	e.notices.mu.Lock()
	delete(e.notices.started, j.Room.ID)
	e.notices.mu.Unlock()
	if r := e.svc.NotifyStarted(ctx, job); !r.IsDone() {
		t.Errorf("NotifyStarted after the end = %+v", r)
	}
	want(j.Room.ID)
}

// roomsStartPost checks the start post of a room (ADR-0100 §9): a room of the local users posts
// as local and one of the followers as private, both with the link of the room after the host's
// text; the rooms of the mutuals and by invitation have none; and a room without one posts
// nothing.
func roomsStartPost(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	for _, tc := range []struct {
		visibility voice.Visibility
		want       content.Visibility
	}{
		{voice.VisibilityLocal, content.VisibilityLocal},
		{voice.VisibilityFollowers, content.VisibilityPrivate},
	} {
		j := e.open(t, host, appvoice.SettingsInput{Visibility: tc.visibility, Post: &appvoice.StartPost{Text: "Come and talk"}})
		e.posts.mu.Lock()
		in := e.posts.prepared[len(e.posts.prepared)-1]
		e.posts.mu.Unlock()
		link := appvoice.RoomURL(contractHost, j.Room.ID)
		if in.Visibility != tc.want || in.AuthorAccountID != host || in.Text != "Come and talk\n\n"+link {
			t.Errorf("the start post of a %s room = %+v, want %s with the text and %s", tc.visibility, in, tc.want, link)
		}
		if err := e.svc.End(ctx, host, j.Room.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []voice.Visibility{voice.VisibilityMutuals, voice.VisibilityInvited} {
		_, err := e.svc.Open(ctx, host, appvoice.SettingsInput{Name: "No post", Visibility: v, Post: &appvoice.StartPost{}})
		if !errors.Is(err, appvoice.ErrNoStartPost) {
			t.Errorf("Open(%s) with a start post = %v, want ErrNoStartPost", v, err)
		}
	}
	j := e.open(t, host, appvoice.SettingsInput{Post: &appvoice.StartPost{Text: "  "}})
	e.posts.mu.Lock()
	in, written := e.posts.prepared[len(e.posts.prepared)-1], e.posts.written
	e.posts.mu.Unlock()
	if in.Text != appvoice.RoomURL(contractHost, j.Room.ID) || written != 3 {
		t.Errorf("a start post without text = %q (%d written), want only the link (3 written)", in.Text, written)
	}
	if err := e.svc.End(ctx, host, j.Room.ID); err != nil {
		t.Fatal(err)
	}
	e.open(t, host, appvoice.SettingsInput{})
	e.posts.mu.Lock()
	defer e.posts.mu.Unlock()
	if e.posts.written != 3 {
		t.Errorf("a room without a start post wrote %d posts, want 3 in all", e.posts.written)
	}
}

// roomsSummary checks the summary of the page of a room for the visitors who do not see it
// (ADR-0100 §9): nothing but whether it ended without voice_public_summary or for a room that is
// not for the local users; the name, the description, and the host of an open room of the local
// users with it; and only the name of such a room that ended.
func roomsSummary(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	local := e.open(t, host, appvoice.SettingsInput{Name: "Open talk", Description: "About Go"})
	summary := func(room string) appvoice.Summary {
		t.Helper()
		s, err := e.svc.Summary(ctx, room)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := summary(local.Room.ID); s.Shown || s.Ended || s.Name != "" || s.HostAccountID != "" {
		t.Errorf("summary without voice_public_summary = %+v, want nothing shown", s)
	}
	e.members.set(func() { e.members.public = true })
	if s := summary(local.Room.ID); !s.Shown || s.Ended || s.Name != "Open talk" || s.Description != "About Go" || s.HostAccountID != host {
		t.Errorf("summary with voice_public_summary = %+v, want the name, the description, and the host", s)
	}
	if err := e.svc.End(ctx, host, local.Room.ID); err != nil {
		t.Fatal(err)
	}
	if s := summary(local.Room.ID); !s.Shown || !s.Ended || s.Name != "Open talk" || s.Description != "" || s.HostAccountID != "" {
		t.Errorf("summary of an ended room = %+v, want only the name", s)
	}
	followers := e.open(t, host, appvoice.SettingsInput{Name: "Followers", Visibility: voice.VisibilityFollowers})
	if s := summary(followers.Room.ID); s.Shown || s.Name != "" {
		t.Errorf("summary of a room of the followers = %+v, want nothing shown", s)
	}
	if err := e.svc.End(ctx, host, followers.Room.ID); err != nil {
		t.Fatal(err)
	}
	if s := summary(followers.Room.ID); s.Shown || !s.Ended || s.Name != "" {
		t.Errorf("summary of an ended room of the followers = %+v, want only that it ended", s)
	}
	if _, err := e.svc.Summary(ctx, e.ids.NewID()); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("summary of an unknown room = %v, want ErrNotFound", err)
	}
}

// roomsProfileAndHistory checks the entry of a room on the profiles of its host and speakers,
// for the viewers who see the room, and none for a listener (ADR-0100 §8), and the history of
// the rooms that an account hosted (§13).
func roomsProfileAndHistory(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, speaker, listener, viewer, outsider := e.account(true), e.account(true), e.account(false), e.account(false), e.account(false)
	e.members.set(func() {
		for _, a := range []string{viewer, speaker, listener} {
			e.members.follows[[2]string{a, host}] = true
		}
	})
	first := e.open(t, host, appvoice.SettingsInput{Name: "First"})
	if err := e.svc.End(ctx, host, first.Room.ID); err != nil {
		t.Fatal(err)
	}
	j := e.open(t, host, appvoice.SettingsInput{Name: "Second", Visibility: voice.VisibilityFollowers, Speakers: voice.SpeakersEveryone})
	room := j.Room.ID
	e.join(t, speaker, room)
	e.join(t, listener, room)
	if _, err := e.svc.Speak(ctx, speaker, room); err != nil {
		t.Fatal(err)
	}
	entry := func(viewerID, accountID string) bool {
		t.Helper()
		v, ok, err := e.svc.ProfileRoom(ctx, viewerID, accountID)
		if err != nil {
			t.Fatal(err)
		}
		if ok && v.ID != room {
			t.Errorf("ProfileRoom(%s) = %s, want %s", accountID, v.ID, room)
		}
		return ok
	}
	if !entry(viewer, host) || !entry(viewer, speaker) || entry(viewer, listener) || entry(outsider, host) || entry("", host) {
		t.Error("the profile entry is not shown to the viewers who see the room, of its host and speakers only")
	}

	// The history lists the rooms that an account hosted, newest first, the handed-over too.
	if _, err := e.svc.HandOver(ctx, host, room, speaker); err != nil {
		t.Fatal(err)
	}
	hosted, more, err := e.svc.History(ctx, host, "", 1)
	if err != nil || !more || len(hosted) != 1 || hosted[0].ID != room || hosted[0].Revision.Settings.Name != "Second" {
		t.Fatalf("History(host, 1) = %+v, %v, %v; want the second room and more", hosted, more, err)
	}
	hosted, more, err = e.svc.History(ctx, host, hosted[0].ID, 10)
	if err != nil || more || len(hosted) != 1 || hosted[0].ID != first.Room.ID || hosted[0].Ending == nil || hosted[0].Ending.Reason != voice.EndHost {
		t.Fatalf("History(host, after) = %+v, %v, %v; want the first room, ended by its host", hosted, more, err)
	}
	if hosted, _, err := e.svc.History(ctx, speaker, "", 10); err != nil || len(hosted) != 1 || hosted[0].ID != room {
		t.Errorf("History(new host) = %+v, %v; want the handed-over room", hosted, err)
	}
	if hosted, _, err := e.svc.History(ctx, listener, "", 10); err != nil || len(hosted) != 0 {
		t.Errorf("History(listener) = %+v, %v; want none", hosted, err)
	}

	// A room whose heartbeats expired shows no entry once its evaluation ends it, before
	// voice.sweep runs: the heartbeats stop, the host's grace begins, and past it the room ends.
	e.clock.Advance(time.Minute)
	entry(viewer, speaker)
	e.clock.Advance(3 * time.Minute)
	if entry(viewer, speaker) {
		t.Error("the profile entry shows a room whose host is gone past the grace")
	}
}

// roomsEmojis is the content module's reading of the emoji for the icons (FR-923): the local
// custom emoji by shortcode, and whether each can be shown (enabled, with an image).
type roomsEmojis struct {
	mu    sync.Mutex
	local map[string]string
	shown map[string]bool
}

func (e *roomsEmojis) ReactionEmoji(context.Context, string) (string, error) {
	return "", appcontent.ErrInvalidReaction
}

func (e *roomsEmojis) LocalEmoji(_ context.Context, name string) (string, string, error) {
	if u, ok := appcontent.NormalizeUnicodeEmoji(name); ok {
		return u, "", nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	id, ok := e.local[appcontent.TrimShortcode(name)]
	if !ok || strings.Contains(name, "@") || !e.shown[id] {
		return "", "", fmt.Errorf("emoji %q: %w", name, appcontent.ErrInvalidReaction)
	}
	return "", id, nil
}

func (e *roomsEmojis) CustomEmojis(_ context.Context, ids []string) (map[string]appcontent.CustomEmojiView, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]appcontent.CustomEmojiView{}
	for code, id := range e.local {
		if slices.Contains(ids, id) && e.shown[id] {
			out[id] = appcontent.CustomEmojiView{ID: id, Shortcode: code, URL: "https://" + contractHost + "/emoji/" + code + ".png"}
		}
	}
	return out, nil
}

// roomsIcon checks the icon of a room (FR-923): a Unicode emoji, normalized, or a local custom
// emoji that can be shown, set when the room opens and changed or removed with its settings,
// kept by a change of the other settings, a 422 field for any other emoji, and no icon shown
// once its custom emoji can no longer be shown.
func roomsIcon(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	blob, hidden := e.ids.NewID(), e.ids.NewID()
	emojis := &roomsEmojis{local: map[string]string{"blob": blob, "hidden": hidden}, shown: map[string]bool{blob: true}}
	e.emojis = emojis
	svc := e.newSvc(appvoice.Config{Transport: voice.TransportP2P})
	host := e.account(true)
	const note = "\U0001F3B5"
	j, err := svc.Open(ctx, host, appvoice.SettingsInput{Name: "Icon", Icon: note})
	if err != nil {
		t.Fatalf("Open with a Unicode icon: %v", err)
	}
	room := j.Room.ID
	if got := e.room(t, room).Revision.Settings.Icon; got != (voice.Icon{Unicode: note}) {
		t.Errorf("icon = %+v, want %q", got, note)
	}
	update := func(p appvoice.SettingsPatch) voice.Icon {
		t.Helper()
		if _, err := svc.Update(ctx, host, room, p); err != nil {
			t.Fatalf("Update(%+v): %v", p, err)
		}
		return e.room(t, room).Revision.Settings.Icon
	}
	icon := func(s string) *string { return &s }
	if got := update(appvoice.SettingsPatch{Icon: icon("blob")}); got != (voice.Icon{CustomEmojiID: blob}) {
		t.Errorf("icon after :blob: = %+v, want the custom emoji", got)
	}
	name := "Renamed"
	if got := update(appvoice.SettingsPatch{Name: &name}); got != (voice.Icon{CustomEmojiID: blob}) {
		t.Errorf("icon after a change of the name = %+v, want it kept", got)
	}
	shown, err := svc.Icons(ctx, []voice.Icon{{CustomEmojiID: blob}, {Unicode: note}})
	if err != nil {
		t.Fatal(err)
	}
	if v := shown[voice.Icon{CustomEmojiID: blob}]; v.Custom == nil || v.Custom.Shortcode != "blob" || shown[voice.Icon{Unicode: note}].Unicode != note {
		t.Errorf("Icons = %+v, want the custom emoji and the Unicode emoji", shown)
	}
	emojis.mu.Lock()
	emojis.shown[blob] = false
	emojis.mu.Unlock()
	if shown, err := svc.Icons(ctx, []voice.Icon{{CustomEmojiID: blob}}); err != nil || len(shown) != 0 {
		t.Errorf("Icons of a disabled emoji = %+v, %v; want none", shown, err)
	}
	if got := update(appvoice.SettingsPatch{Icon: icon("")}); !got.IsZero() {
		t.Errorf("icon after removing it = %+v, want none", got)
	}
	var se *voice.SettingsError
	for _, bad := range []string{":hidden:", "blob@remote.example", ":unknown:", "abc"} {
		if _, err := svc.Open(ctx, e.account(true), appvoice.SettingsInput{Name: "Bad", Icon: bad}); !errors.As(err, &se) || se.Field != voice.FieldIcon {
			t.Errorf("Open with the icon %q: %v, want the icon invalid", bad, err)
		}
		if _, err := svc.Update(ctx, host, room, appvoice.SettingsPatch{Icon: &bad}); !errors.As(err, &se) || se.Field != voice.FieldIcon {
			t.Errorf("Update with the icon %q: %v, want the icon invalid", bad, err)
		}
	}
}
