// SPDX-License-Identifier: AGPL-3.0-only

package voicetest

import (
	"errors"
	"slices"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// roomsFollowing checks the following of the visibility and the speakers (ADR-0100 §3, §4): a
// room of the following admits exactly the accounts the host follows and the invited ones, and
// with speakers = following those the host follows speak without asking while the others ask.
func roomsFollowing(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	followed, mutual, follower, stranger, invitee := e.account(false), e.account(false), e.account(false), e.account(false), e.account(false)
	e.members.set(func() {
		e.members.follows[[2]string{host, followed}] = true
		e.members.follows[[2]string{host, mutual}] = true
		e.members.follows[[2]string{mutual, host}] = true
		e.members.follows[[2]string{follower, host}] = true
	})
	r := e.open(t, host, appvoice.SettingsInput{Visibility: voice.VisibilityFollowing, Speakers: voice.SpeakersFollowing}).Room.ID
	if got := e.room(t, r).Revision.Settings; got.Visibility != voice.VisibilityFollowing || got.Speakers != voice.SpeakersFollowing {
		t.Fatalf("settings = %+v, want following for both", got)
	}
	for _, a := range []string{follower, stranger, invitee} {
		_, err := e.svc.Get(ctx, a, r)
		wantErr(t, "Get of a following room by an account the host does not follow", err, appvoice.ErrNotFound)
		_, err = e.svc.Join(ctx, a, r)
		wantErr(t, "Join of a following room by an account the host does not follow", err, appvoice.ErrNotFound)
		if rooms, _, err := e.svc.List(ctx, a, appvoice.ListQuery{}); err != nil || len(rooms) != 0 {
			t.Errorf("List by an account the host does not follow = %d rooms, %v; want none", len(rooms), err)
		}
	}
	for _, a := range []string{followed, mutual} {
		if rooms, _, err := e.svc.List(ctx, a, appvoice.ListQuery{}); err != nil || len(rooms) != 1 || rooms[0].ID != r {
			t.Errorf("List by an account the host follows = %d rooms, %v; want the room", len(rooms), err)
		}
		e.join(t, a, r)
	}
	if _, err := e.svc.Invite(ctx, host, r, invitee, voice.RoleListener); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	e.join(t, invitee, r)

	// The accounts the host follows speak without asking; the invited listener asks.
	for _, a := range []string{followed, mutual} {
		if _, err := e.svc.Speak(ctx, a, r); err != nil {
			t.Fatalf("Speak by an account the host follows: %v", err)
		}
		if p := e.wantParticipant(t, r, a, true); p.Role != voice.RoleSpeaker {
			t.Errorf("participant = %+v, want a speaker", p)
		}
	}
	_, err := e.svc.Speak(ctx, invitee, r)
	wantErr(t, "Speak by an account the host does not follow", err, appvoice.ErrCannotSpeak)
	if _, err := e.svc.RaiseHand(ctx, invitee, r); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Grant(ctx, host, r, invitee); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if _, err := e.svc.Speak(ctx, invitee, r); err != nil {
		t.Errorf("Speak after a grant: %v", err)
	}
	// The host follows the follower now: the follower sees the room and joins.
	e.members.set(func() { e.members.follows[[2]string{host, follower}] = true })
	e.join(t, follower, r)
}

// scheduledInput is a scheduled room of host under key.
func scheduledInput(host, key string) appvoice.ScheduledInput {
	return appvoice.ScheduledInput{HostAccountID: host, Key: key, Settings: appvoice.SettingsInput{Name: "Event"}}
}

func (e *roomsEnv) openScheduled(t *testing.T, in appvoice.ScheduledInput) appvoice.RoomView {
	t.Helper()
	v, err := e.svc.OpenScheduled(t.Context(), in)
	if err != nil {
		t.Fatalf("OpenScheduled: %v", err)
	}
	return v
}

func (e *roomsEnv) sweep(t *testing.T) {
	t.Helper()
	if err := e.svc.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
}

// roomsScheduled checks the opening of a scheduled room (ADR-0100 §16): with nobody in it, no
// grant, no start post or notification, and no media path; once per key; with the errors of
// Open; outside the one room of a participation; and with the invitations it copies.
func roomsScheduled(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, invitee, blocked := e.account(true), e.account(false), e.account(false)
	e.members.set(func() {
		e.members.follows[[2]string{invitee, host}] = true
		e.members.blocks[[2]string{host, blocked}] = true
	})
	// The host listens in another room meanwhile: a participation is not hosting.
	other := e.open(t, e.account(true), appvoice.SettingsInput{}).Room.ID
	e.join(t, host, other)
	if err := e.svc.CheckScheduledHost(ctx, host); err != nil {
		t.Fatalf("CheckScheduledHost: %v", err)
	}
	in := scheduledInput(host, "event:1")
	in.Invitees = []string{invitee, blocked, host, invitee}
	v := e.openScheduled(t, in)
	if v.Head.HostAccountID != host || v.Head.State != voice.StateOpen || len(v.Participants) != 0 {
		t.Fatalf("scheduled room = %+v with %d participants, want open by the host with nobody", v.Head, len(v.Participants))
	}
	if e.transport.count("open:"+v.ID) != 0 {
		t.Errorf("transport calls = %v, want no open before the first join", e.transport.calls)
	}
	if got := e.notices.recipients(v.ID); len(got) != 0 {
		t.Errorf("voice_room_started went to %v, want nobody", got)
	}
	if g, err := e.h.Voice.FindHostGrace(ctx, nil, v.ID); err != nil || !g.EndsAt.Equal(v.OpenedAt.Add(voice.DefaultScheduledWait)) {
		t.Errorf("the wait = %+v, %v; want the default wait from the opening", g, err)
	}
	invs, err := e.h.Voice.Invitations(ctx, nil, v.ID, "", 10)
	if err != nil || len(invs) != 1 || invs[0].InviteeAccountID != invitee || invs[0].InvitedByAccountID != host || invs[0].Role != voice.RoleListener {
		t.Errorf("invitations = %+v, %v; want the invitee alone, as a listener invited by the host", invs, err)
	}
	e.notices.mu.Lock()
	notified := len(e.notices.invited[v.ID])
	e.notices.mu.Unlock()
	if notified != 0 {
		t.Errorf("voice_room_invited went to %d accounts, want nobody", notified)
	}
	if err := e.svc.AddScheduledInvitees(ctx, "event:1", []string{invitee}); err != nil {
		t.Fatalf("AddScheduledInvitees: %v", err)
	}
	wantErr(t, "AddScheduledInvitees of an unknown key", e.svc.AddScheduledInvitees(ctx, "event:none", []string{invitee}), appvoice.ErrNotFound)

	// The same key returns the room; the host may not open another one.
	again := e.openScheduled(t, in)
	if again.ID != v.ID {
		t.Errorf("OpenScheduled again = %s, want %s", again.ID, v.ID)
	}
	_, err = e.svc.OpenScheduled(ctx, scheduledInput(host, "event:2"))
	wantErr(t, "a second scheduled room of the host", err, appvoice.ErrHostingElsewhere)
	_, err = e.svc.Open(ctx, host, appvoice.SettingsInput{Name: "Now"})
	wantErr(t, "Open by the host of a scheduled room", err, appvoice.ErrHostingElsewhere)
	_, err = e.svc.OpenScheduled(ctx, scheduledInput(e.account(false), "event:3"))
	wantErr(t, "a host without start_voice_rooms", err, appvoice.ErrNotEligible)
	wantErr(t, "CheckScheduledHost without start_voice_rooms", e.svc.CheckScheduledHost(ctx, e.account(false)), appvoice.ErrNotEligible)
	for name, in := range map[string]appvoice.ScheduledInput{
		"no key": scheduledInput(e.account(true), ""),
		"short wait": func() appvoice.ScheduledInput {
			in := scheduledInput(e.account(true), "event:4")
			in.Wait = time.Minute
			return in
		}(),
		"long wait": func() appvoice.ScheduledInput {
			in := scheduledInput(e.account(true), "event:5")
			in.Wait = 3 * time.Hour
			return in
		}(),
		"a post": func() appvoice.ScheduledInput {
			in := scheduledInput(e.account(true), "event:6")
			in.Settings.Post = &appvoice.StartPost{}
			return in
		}(),
		"four candidates": func() appvoice.ScheduledInput {
			in := scheduledInput(e.account(true), "event:7")
			in.CohostCandidates = []string{e.account(false), e.account(false), e.account(false), e.account(false)}
			return in
		}(),
	} {
		if _, err := e.svc.OpenScheduled(ctx, in); !errors.Is(err, appvoice.ErrInvalidSchedule) {
			t.Errorf("OpenScheduled with %s = %v, want ErrInvalidSchedule", name, err)
		}
	}
	capped := e.newSvc(appvoice.Config{Transport: voice.TransportP2P, MaxOpenRooms: 2})
	_, err = capped.OpenScheduled(ctx, scheduledInput(e.account(true), "event:8"))
	wantErr(t, "a third room of two", err, appvoice.ErrTooManyRooms)
	off := e.newSvc(appvoice.Config{})
	_, err = off.OpenScheduled(ctx, scheduledInput(e.account(true), "event:9"))
	wantErr(t, "voice = off", err, appvoice.ErrDisabled)
	wantErr(t, "CheckScheduledHost with voice = off", off.CheckScheduledHost(ctx, host), appvoice.ErrDisabled)

	// The invitee joins first: the media path opens once, and the host who comes after
	// speaks and ends the wait.
	ji := e.join(t, invitee, v.ID)
	if e.transport.count("open:"+v.ID) != 1 {
		t.Errorf("transport calls = %v, want one open at the first join", e.transport.calls)
	}
	jh := e.join(t, host, v.ID)
	if p := e.wantParticipant(t, v.ID, host, true); p.Role != voice.RoleSpeaker {
		t.Errorf("the host = %+v, want a speaker", p)
	}
	if e.transport.count("open:"+v.ID) != 1 {
		t.Errorf("transport calls = %v, want still one open", e.transport.calls)
	}
	if _, err := e.h.Voice.FindHostGrace(ctx, nil, v.ID); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("the wait after the host came = %v, want none", err)
	}
	if _, ok := e.participants(t, other)[host]; ok {
		t.Error("the host is still in the other room")
	}
	// The room goes on as one opened by the button: past the wait, it stays open.
	e.keep(t, v.ID, voice.DefaultScheduledWait, map[string]appvoice.Joined{host: jh, invitee: ji})
	e.sweep(t)
	if r := e.room(t, v.ID); r.Head.State != voice.StateOpen || r.Head.HostAccountID != host {
		t.Errorf("room after the wait = %+v, want open with the host", r.Head)
	}
}

// keep advances the clock by d in steps of 30 seconds, with a heartbeat of every session of
// joined in room at each step.
func (e *roomsEnv) keep(t *testing.T, room string, d time.Duration, joined map[string]appvoice.Joined) {
	t.Helper()
	for step := 30 * time.Second; d > 0; d -= step {
		e.clock.Advance(min(step, d))
		for account, j := range joined {
			e.beat(t, account, room, j.Session)
		}
	}
}

// roomsWaits checks the wait of a scheduled room (ADR-0100 §16): it survives the sweep with
// nobody in it, and at the end of the wait it ends unattended without anybody, moves to an
// eligible co-host who came (host_absent), ends host_absent with only others in it, and ends
// empty once everybody who came left.
func roomsWaits(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	wait := 10 * time.Minute
	scheduled := func(key string, candidates ...string) string {
		in := scheduledInput(e.account(true), key)
		in.Wait, in.CohostCandidates = wait, candidates
		return e.openScheduled(t, in).ID
	}

	// Nobody comes.
	alone := scheduled("event:alone")
	for range 9 {
		e.clock.Advance(time.Minute)
		e.sweep(t)
		if _, _, err := e.svc.List(ctx, e.account(false), appvoice.ListQuery{}); err != nil {
			t.Fatal(err)
		}
	}
	if r := e.room(t, alone); r.Head.State != voice.StateOpen {
		t.Fatalf("a scheduled room that waits = %+v %+v, want open", r.Head, r.Ending)
	}
	e.clock.Advance(time.Minute)
	e.sweep(t)
	e.wantEnded(t, alone, voice.EndUnattended)

	// A co-host candidate comes before the host and becomes a co-host; past the wait, the
	// eligible co-host takes over.
	cand, notEligible, guest := e.account(true), e.account(false), e.account(false)
	moved := scheduled("event:moved", notEligible, cand)
	jn := e.join(t, notEligible, moved)
	jc := e.join(t, cand, moved)
	jg := e.join(t, guest, moved)
	ps := e.participants(t, moved)
	if !ps[cand].Cohost() || !ps[notEligible].Cohost() || ps[guest].Cohost() {
		t.Fatalf("participants = %+v, want the candidates as co-hosts and the guest not", ps)
	}
	e.keep(t, moved, wait-time.Minute, map[string]appvoice.Joined{notEligible: jn, cand: jc, guest: jg})
	if r := e.room(t, moved); r.Head.State != voice.StateOpen || r.Head.HostAccountID == cand {
		t.Fatalf("room within the wait = %+v %+v, want open with the absent host", r.Head, r.Ending)
	}
	e.keep(t, moved, time.Minute, map[string]appvoice.Joined{notEligible: jn, cand: jc, guest: jg})
	e.sweep(t)
	r := e.room(t, moved)
	if r.Head.State != voice.StateOpen || r.Head.HostAccountID != cand {
		t.Fatalf("room past the wait = %+v %+v, want open with the eligible candidate as the host", r.Head, r.Ending)
	}
	if _, err := e.h.Voice.FindHostGrace(ctx, nil, moved); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("the wait past its end = %v, want none", err)
	}
	if p := e.participants(t, moved)[cand]; p.Cohost() {
		t.Errorf("the new host = %+v, want no longer a co-host", p)
	}

	// Only guests come: the room ends past the wait.
	absent := scheduled("event:absent")
	g2 := e.account(false)
	e.keep(t, absent, wait-30*time.Second, map[string]appvoice.Joined{g2: e.join(t, g2, absent)})
	e.clock.Advance(30 * time.Second)
	e.sweep(t)
	e.wantEnded(t, absent, voice.EndHostAbsent)

	// A guest comes and leaves within the wait: the room ends empty.
	left := scheduled("event:left")
	g := e.account(false)
	e.join(t, g, left)
	if err := e.svc.Leave(ctx, g, left, appvoice.LeaveInput{}); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	e.wantEnded(t, left, voice.EndEmpty)

	// A guest whose heartbeat expires within the wait leaves, and the room ends empty.
	expired := scheduled("event:expired")
	e.join(t, e.account(false), expired)
	e.clock.Advance(time.Minute)
	e.sweep(t)
	e.wantEnded(t, expired, voice.EndEmpty)
}

// roomsScheduledCohosts checks the co-host candidates and the end of a scheduled room by its
// caller (ADR-0100 §16): a candidate whom the host removed does not come back on its own, a
// candidate becomes a co-host only while fewer than three are, and EndScheduled ends the room once.
func roomsScheduledCohosts(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	a, b, c := e.account(false), e.account(false), e.account(false)
	in := scheduledInput(host, "event:cohosts")
	in.CohostCandidates = []string{a, b, c}
	r := e.openScheduled(t, in).ID
	e.join(t, host, r)
	e.join(t, a, r)
	if _, err := e.svc.RemoveCohost(ctx, host, r, a); err != nil {
		t.Fatalf("RemoveCohost: %v", err)
	}
	if err := e.svc.Leave(ctx, a, r, appvoice.LeaveInput{}); err != nil {
		t.Fatal(err)
	}
	e.join(t, a, r)
	if p := e.participants(t, r)[a]; p.Cohost() {
		t.Errorf("a removed candidate who joins again = %+v, want no co-host", p)
	}
	others := []string{e.account(false), e.account(false), e.account(false)}
	for _, o := range others {
		e.join(t, o, r)
		if _, err := e.svc.AssignCohost(ctx, host, r, o); err != nil {
			t.Fatalf("AssignCohost: %v", err)
		}
		if o == others[1] {
			break
		}
	}
	e.join(t, b, r)
	e.join(t, c, r)
	ps := e.participants(t, r)
	n := 0
	for _, p := range ps {
		if p.Cohost() {
			n++
		}
	}
	if !ps[b].Cohost() || ps[c].Cohost() || n != voice.MaxCohosts {
		t.Errorf("co-hosts = %d (b %t, c %t), want three with b and without c", n, ps[b].Cohost(), ps[c].Cohost())
	}

	err := e.svc.EndScheduled(ctx, appvoice.EndScheduledInput{Key: "event:cohosts", Reason: voice.EndDisabled})
	wantErr(t, "EndScheduled with another reason", err, appvoice.ErrInvalidEndReason)
	wantErr(t, "EndScheduled of an unknown key", e.svc.EndScheduled(ctx, appvoice.EndScheduledInput{Key: "event:none", Reason: voice.EndHost}), appvoice.ErrNotFound)
	for range 2 {
		if err := e.svc.EndScheduled(ctx, appvoice.EndScheduledInput{Key: "event:cohosts", Reason: voice.EndHost, ByAccountID: host}); err != nil {
			t.Fatalf("EndScheduled: %v", err)
		}
	}
	e.wantEnded(t, r, voice.EndHost)
	if ps := e.participants(t, r); len(ps) != 0 {
		t.Errorf("participants of the ended room = %d, want none", len(ps))
	}

	// A moderator ends a waiting room, with the audit log.
	m := e.openScheduled(t, scheduledInput(e.account(true), "event:moderated")).ID
	moderator := e.account(false)
	if err := e.svc.EndScheduled(ctx, appvoice.EndScheduledInput{
		Key: "event:moderated", Reason: voice.EndModerator, ByAccountID: moderator, ByUserID: e.ids.NewID(), Note: "Cancelled",
	}); err != nil {
		t.Fatalf("EndScheduled by a moderator: %v", err)
	}
	e.wantEnded(t, m, voice.EndModerator)
	if rec := e.room(t, m); rec.Ending.EndedByAccountID != moderator || rec.Ending.Note != "Cancelled" {
		t.Errorf("ending = %+v, want the moderator and the note", rec.Ending)
	}
	if !slices.ContainsFunc(e.audits.all(), func(a appmoderation.AuditEntry) bool { return a.TargetID == m }) {
		t.Errorf("audit log = %v, want the end of %s", e.audits.all(), m)
	}
}

// roomsScheduledMedia checks a scheduled room on a media server (ADR-0100 §16): the sweep does
// not reconcile it while nobody joined, and the first join opens its media path.
func roomsScheduledMedia(t *testing.T, e *roomsEnv) {
	e.transport.sfu()
	host, guest := e.account(true), e.account(false)
	r := e.openScheduled(t, scheduledInput(host, "event:media")).ID
	called := false
	e.transport.mu.Lock()
	e.transport.onSessions = func() { called = true }
	e.transport.mu.Unlock()
	e.sweep(t)
	if called || e.transport.count("open:"+r) != 0 {
		t.Errorf("sweep of a room nobody joined: Sessions %t, calls %v; want neither Sessions nor open", called, e.transport.calls)
	}
	e.join(t, guest, r)
	if e.transport.count("open:"+r) != 1 {
		t.Errorf("transport calls = %v, want one open at the first join", e.transport.calls)
	}
}
