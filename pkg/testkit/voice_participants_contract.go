// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"testing"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The cases of VoiceRoomsContract for the operations of the participants, the host, and the
// co-hosts (ADR-0100 §4, §5, §7, §11).

// block records a block of blocked by blocker, as the graph module does: the relation and the
// graph.blocked event, whose projector enqueues voice.check_block.
func (e *roomsEnv) block(t *testing.T, blocker, blocked string) {
	t.Helper()
	e.members.set(func() { e.members.blocks[[2]string{blocker, blocked}] = true })
	b := graph.Block{ID: e.ids.NewID(), BlockerAccountID: blocker, BlockedAccountID: blocked, CreatedAt: e.clock.Now()}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return tx.Emit(t.Context(), appgraph.Blocked{Block: b}) })
	if err != nil {
		t.Fatal(err)
	}
}

// wantParticipant fails unless account is in room (in) or not.
func (e *roomsEnv) wantParticipant(t *testing.T, room, account string, in bool) appvoice.Participant {
	t.Helper()
	p, ok := e.participants(t, room)[account]
	if ok != in {
		t.Fatalf("%s in the room = %v, want %v", account, ok, in)
	}
	return p
}

func roomsSpeaking(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, cohost, listener, other := e.account(true), e.account(false), e.account(false), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID // speakers: manual
	e.join(t, cohost, r)
	e.cohost(t, r, cohost)
	session := e.join(t, listener, r).Session
	e.join(t, other, r)

	_, err := e.svc.Speak(ctx, listener, r)
	wantErr(t, "Speak in a manual room", err, appvoice.ErrCannotSpeak)
	if _, err := e.svc.RaiseHand(ctx, listener, r); err != nil {
		t.Fatal(err)
	}
	if p := e.wantParticipant(t, r, listener, true); !p.HandRaised {
		t.Fatal("the hand is not raised")
	}
	_, err = e.svc.Grant(ctx, other, r, listener)
	wantErr(t, "Grant by a participant", err, appvoice.ErrNotHostOrCohost)
	_, err = e.svc.Grant(ctx, cohost, r, host)
	wantErr(t, "Grant to the host by a co-host", err, appvoice.ErrInvalidParticipant)
	_, err = e.svc.Grant(ctx, host, r, host)
	wantErr(t, "Grant to oneself", err, appvoice.ErrInvalidParticipant)
	_, err = e.svc.Grant(ctx, host, r, e.account(false))
	wantErr(t, "Grant to an account not in the room", err, appvoice.ErrInvalidParticipant)

	if _, err := e.svc.Grant(ctx, cohost, r, listener); err != nil {
		t.Fatalf("Grant by a co-host: %v", err)
	}
	if p := e.wantParticipant(t, r, listener, true); p.HandRaised || p.Role != voice.RoleListener {
		t.Fatalf("after the grant = %+v, want a listener with the hand granted", p)
	}
	if _, err := e.svc.Speak(ctx, listener, r); err != nil {
		t.Fatalf("Speak after a grant: %v", err)
	}
	if p := e.wantParticipant(t, r, listener, true); p.Role != voice.RoleSpeaker {
		t.Fatalf("after Speak = %+v, want a speaker", p)
	}
	if e.transport.count("speak:"+session+":true") != 1 {
		t.Errorf("transport calls = %v, want the session allowed to speak", e.transport.calls)
	}
	if _, err := e.svc.Listen(ctx, listener, r); err != nil {
		t.Fatal(err)
	}
	if p := e.wantParticipant(t, r, listener, true); p.Role != voice.RoleListener || e.transport.count("speak:"+session+":false") != 1 {
		t.Fatalf("after Listen = %+v, calls %v", p, e.transport.calls)
	}
	if _, err := e.svc.Speak(ctx, listener, r); err != nil {
		t.Fatalf("Speak again with the grant: %v", err)
	}

	_, err = e.svc.Revoke(ctx, other, r, listener)
	wantErr(t, "Revoke by a participant", err, appvoice.ErrNotHostOrCohost)
	if _, err := e.svc.Revoke(ctx, host, r, listener); err != nil {
		t.Fatal(err)
	}
	if p := e.wantParticipant(t, r, listener, true); p.Role != voice.RoleListener || e.transport.count("speak:"+session+":false") != 2 {
		t.Fatalf("after Revoke = %+v, calls %v", p, e.transport.calls)
	}
	_, err = e.svc.Speak(ctx, listener, r)
	wantErr(t, "Speak after a revocation", err, appvoice.ErrCannotSpeak)

	if _, err := e.svc.RaiseHand(ctx, other, r); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Dismiss(ctx, listener, r, other)
	wantErr(t, "Dismiss by a participant", err, appvoice.ErrNotHostOrCohost)
	if _, err := e.svc.Dismiss(ctx, cohost, r, other); err != nil {
		t.Fatal(err)
	}
	if p := e.wantParticipant(t, r, other, true); p.HandRaised {
		t.Fatal("the dismissed hand is still raised")
	}
	_, err = e.svc.Speak(ctx, other, r)
	wantErr(t, "Speak after a dismissal", err, appvoice.ErrCannotSpeak)
	if _, err := e.svc.RaiseHand(ctx, other, r); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.LowerHand(ctx, other, r); err != nil {
		t.Fatal(err)
	}
	if p := e.wantParticipant(t, r, other, true); p.HandRaised {
		t.Fatal("the lowered hand is still raised")
	}

	// A co-host speaks freely; the speakers setting covers the followers of the host.
	if _, err := e.svc.Speak(ctx, cohost, r); err != nil {
		t.Errorf("Speak by a co-host: %v", err)
	}
	follower, limited := e.account(false), e.account(false)
	e.members.set(func() {
		e.members.follows[[2]string{follower, host}] = true
		e.members.follows[[2]string{limited, host}] = true
		e.members.limited[limited] = true
	})
	speakers := voice.SpeakersFollowers
	if _, err := e.svc.Update(ctx, host, r, appvoice.SettingsPatch{Speakers: &speakers}); err != nil {
		t.Fatal(err)
	}
	e.join(t, follower, r)
	e.join(t, limited, r)
	if _, err := e.svc.Speak(ctx, follower, r); err != nil {
		t.Errorf("Speak by a follower: %v", err)
	}
	_, err = e.svc.Speak(ctx, limited, r)
	wantErr(t, "Speak by a limited follower", err, appvoice.ErrCannotSpeak)
	if _, err := e.svc.Grant(ctx, host, r, limited); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Speak(ctx, limited, r); err != nil {
		t.Errorf("Speak by a limited follower with a grant: %v", err)
	}
	_, err = e.svc.Speak(ctx, e.account(false), r)
	wantErr(t, "Speak by an account not in the room", err, appvoice.ErrNotFound)
}

func roomsRemove(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, cohost, cohost2, listener, other := e.account(true), e.account(false), e.account(false), e.account(false), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	e.join(t, cohost, r)
	e.join(t, cohost2, r)
	e.cohost(t, r, cohost)
	e.cohost(t, r, cohost2)
	session := e.join(t, listener, r).Session
	e.join(t, other, r)

	wantErr(t, "Mute by a participant", e.svc.Mute(ctx, other, r, listener), appvoice.ErrNotHostOrCohost)
	wantErr(t, "Mute of the host by a co-host", e.svc.Mute(ctx, cohost, r, host), appvoice.ErrInvalidParticipant)
	if err := e.svc.Mute(ctx, cohost, r, listener); err != nil {
		t.Fatal(err)
	}
	if e.transport.count("mute:"+session) != 1 {
		t.Errorf("transport calls = %v, want the session muted", e.transport.calls)
	}

	wantErr(t, "Remove by a participant", e.svc.Remove(ctx, other, r, listener), appvoice.ErrNotHostOrCohost)
	wantErr(t, "Remove of the host", e.svc.Remove(ctx, cohost, r, host), appvoice.ErrInvalidParticipant)
	wantErr(t, "Remove of a co-host by the host", e.svc.Remove(ctx, host, r, cohost), appvoice.ErrInvalidParticipant)
	wantErr(t, "Remove of a co-host by a co-host", e.svc.Remove(ctx, cohost2, r, cohost), appvoice.ErrInvalidParticipant)
	if err := e.svc.Remove(ctx, cohost, r, listener); err != nil {
		t.Fatal(err)
	}
	e.wantParticipant(t, r, listener, false)
	if e.transport.count("remove:"+session) != 1 {
		t.Errorf("transport calls = %v, want the session removed", e.transport.calls)
	}
	banned, err := e.h.Voice.Banned(ctx, nil, r, listener)
	if err != nil || !banned {
		t.Fatalf("Banned = %v, %v; want the removed account banned", banned, err)
	}
	_, err = e.svc.Join(ctx, listener, r)
	wantErr(t, "Join after a removal", err, appvoice.ErrNotFound)
	_, err = e.svc.Get(ctx, listener, r)
	wantErr(t, "Get after a removal", err, appvoice.ErrNotFound)
	wantErr(t, "Remove of an account not in the room", e.svc.Remove(ctx, host, r, listener), appvoice.ErrInvalidParticipant)
	if err := e.svc.Remove(ctx, host, r, other); err != nil {
		t.Errorf("Remove by the host: %v", err)
	}
}

func roomsCohosts(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	a, b, c, d := e.account(true), e.account(false), e.account(false), e.account(false)
	for _, id := range []string{a, b, c, d} {
		e.join(t, id, r)
	}
	for _, id := range []string{a, b, c} {
		if _, err := e.svc.AssignCohost(ctx, host, r, id); err != nil {
			t.Fatalf("AssignCohost: %v", err)
		}
	}
	if p := e.wantParticipant(t, r, a, true); !p.Cohost() {
		t.Fatal("the assigned participant is not a co-host")
	}
	_, err := e.svc.AssignCohost(ctx, host, r, d)
	wantErr(t, "a fourth co-host", err, appvoice.ErrTooManyCohosts)
	_, err = e.svc.AssignCohost(ctx, a, r, d)
	wantErr(t, "AssignCohost by a co-host", err, appvoice.ErrNotHost)
	_, err = e.svc.RemoveCohost(ctx, a, r, b)
	wantErr(t, "RemoveCohost by a co-host", err, appvoice.ErrNotHost)
	_, err = e.svc.AssignCohost(ctx, host, r, host)
	wantErr(t, "AssignCohost of the host", err, appvoice.ErrInvalidParticipant)
	if _, err := e.svc.RemoveCohost(ctx, host, r, c); err != nil {
		t.Fatal(err)
	}
	if p := e.wantParticipant(t, r, c, true); p.Cohost() {
		t.Fatal("the removed co-host is still a co-host")
	}

	// The participants in a block relation with the new co-host leave (blocked).
	blocker, bystander := e.account(false), e.account(false)
	e.join(t, blocker, r)
	e.join(t, bystander, r)
	e.members.set(func() { e.members.blocks[[2]string{blocker, d}] = true })
	if _, err := e.svc.AssignCohost(ctx, host, r, d); err != nil {
		t.Fatal(err)
	}
	e.wantParticipant(t, r, blocker, false)
	e.wantParticipant(t, r, bystander, true)
	if p := e.wantParticipant(t, r, d, true); !p.Cohost() {
		t.Fatal("d is not a co-host")
	}
	// The co-host status ends with the participation (#1345 answer 44).
	if err := e.svc.Leave(ctx, b, r, appvoice.LeaveInput{}); err != nil {
		t.Fatal(err)
	}
	e.join(t, b, r)
	if p := e.wantParticipant(t, r, b, true); p.Cohost() {
		t.Error("a co-host who left and joined again is still a co-host")
	}
	// A participant in a block relation with the host does not become a co-host.
	e.members.set(func() { e.members.blocks[[2]string{host, bystander}] = true })
	_, err = e.svc.AssignCohost(ctx, host, r, bystander)
	wantErr(t, "AssignCohost of one blocked by the host", err, appvoice.ErrInvalidParticipant)

	// A co-host who becomes the host is no longer a co-host, and a co-host place frees up.
	if _, err := e.svc.HandOver(ctx, host, r, a); err != nil {
		t.Fatal(err)
	}
	if p := e.wantParticipant(t, r, a, true); p.Cohost() {
		t.Error("the new host is still a co-host")
	}
	if _, err := e.svc.AssignCohost(ctx, a, r, host); err != nil {
		t.Errorf("AssignCohost of the former host by the new host: %v", err)
	}
	if _, err := e.svc.AssignCohost(ctx, a, r, b); err != nil {
		t.Errorf("AssignCohost after the hand-over: %v", err)
	}
}

func roomsInvitations(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, cohost, other := e.account(true), e.account(false), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{Visibility: voice.VisibilityInvited}).Room.ID
	_, err := e.svc.Join(ctx, cohost, r)
	wantErr(t, "Join of an invited room without an invitation", err, appvoice.ErrNotFound)

	if _, err := e.svc.Invite(ctx, host, r, cohost, ""); err != nil {
		t.Fatal(err)
	}
	e.join(t, cohost, r)
	e.cohost(t, r, cohost)
	if _, err := e.svc.Invite(ctx, host, r, other, voice.RoleListener); err != nil {
		t.Fatal(err)
	}
	e.join(t, other, r)

	speaker, listener := e.account(false), e.account(false)
	_, err = e.svc.Invite(ctx, other, r, speaker, voice.RoleSpeaker)
	wantErr(t, "Invite by a participant", err, appvoice.ErrNotHostOrCohost)
	_, err = e.svc.Invite(ctx, cohost, r, speaker, voice.Role("host"))
	wantErr(t, "Invite with an unknown role", err, appvoice.ErrInvalidRole)
	inv, err := e.svc.Invite(ctx, cohost, r, speaker, voice.RoleSpeaker)
	if err != nil {
		t.Fatalf("Invite by a co-host: %v", err)
	}
	if inv.Role != voice.RoleSpeaker || inv.InvitedByAccountID != cohost || inv.InviteeAccountID != speaker {
		t.Errorf("invitation = %+v", inv)
	}
	_, err = e.svc.Invite(ctx, host, r, speaker, voice.RoleListener)
	wantErr(t, "a second invitation", err, appvoice.ErrAlreadyInvited)
	if _, err := e.svc.Get(ctx, speaker, r); err != nil {
		t.Fatalf("Get by the invited account: %v", err)
	}
	e.join(t, speaker, r)
	if _, err := e.svc.Speak(ctx, speaker, r); err != nil {
		t.Errorf("Speak after an invitation as a speaker: %v", err)
	}

	blocked, banned, inactive := e.account(false), e.account(false), e.account(false)
	e.members.set(func() {
		e.members.blocks[[2]string{blocked, cohost}] = true
		e.members.inactive[inactive] = true
	})
	for name, id := range map[string]string{"blocked with a co-host": blocked, "inactive": inactive, "the host": host, "oneself": cohost} {
		_, err := e.svc.Invite(ctx, cohost, r, id, "")
		wantErr(t, "Invite of "+name, err, appvoice.ErrInvalidInvitee)
	}
	if _, err := e.svc.Invite(ctx, host, r, banned, ""); err != nil {
		t.Fatal(err)
	}
	e.join(t, banned, r)
	if err := e.svc.Remove(ctx, host, r, banned); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RevokeInvitation(ctx, host, r, banned); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Invite(ctx, host, r, banned, "")
	wantErr(t, "Invite of a banned account", err, appvoice.ErrInvalidInvitee)

	if _, err := e.svc.Invite(ctx, host, r, listener, ""); err != nil {
		t.Fatal(err)
	}
	invs, more, err := e.svc.Invitations(ctx, cohost, r, "", 10)
	if err != nil || more || len(invs) != 4 || invs[0].InviteeAccountID != cohost || invs[3].InviteeAccountID != listener {
		t.Fatalf("Invitations = %+v, %v, %v; want the 4 that were not revoked", invs, more, err)
	}
	page, more, err := e.svc.Invitations(ctx, host, r, invs[1].ID, 1)
	if err != nil || !more || len(page) != 1 || page[0].ID != invs[2].ID {
		t.Errorf("the page after the second = %+v, %v, %v", page, more, err)
	}
	_, _, err = e.svc.Invitations(ctx, other, r, "", 10)
	wantErr(t, "Invitations by a participant", err, appvoice.ErrNotHostOrCohost)

	wantErr(t, "RevokeInvitation by a participant", e.svc.RevokeInvitation(ctx, other, r, listener), appvoice.ErrNotHostOrCohost)
	if err := e.svc.RevokeInvitation(ctx, cohost, r, listener); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Get(ctx, listener, r)
	wantErr(t, "Get after the revocation", err, appvoice.ErrNotFound)
	wantErr(t, "a second revocation", e.svc.RevokeInvitation(ctx, cohost, r, listener), appvoice.ErrNotFound)
	_, err = e.svc.Invite(ctx, host, r, listener, "")
	wantErr(t, "Invite after a revocation", err, appvoice.ErrAlreadyInvited)

	// A participant whose invitation is revoked stays, and loses the permission of an
	// invitation as a speaker.
	if _, err := e.svc.Listen(ctx, speaker, r); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RevokeInvitation(ctx, host, r, speaker); err != nil {
		t.Fatal(err)
	}
	e.wantParticipant(t, r, speaker, true)
	_, err = e.svc.Speak(ctx, speaker, r)
	wantErr(t, "Speak after the revocation of an invitation as a speaker", err, appvoice.ErrCannotSpeak)
}

func roomsInvitationLimit(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host := e.account(true)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	for i := range int(appvoice.InvitationLimit.Max) {
		if _, err := e.svc.Invite(ctx, host, r, e.account(false), ""); err != nil {
			t.Fatalf("invitation %d: %v", i+1, err)
		}
	}
	_, err := e.svc.Invite(ctx, host, r, e.account(false), "")
	var limited *kernel.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("an invitation past the limit: %v, want a rate limit", err)
	}
	other := e.open(t, e.account(true), appvoice.SettingsInput{}).Room
	if _, err := e.svc.Invite(ctx, other.Head.HostAccountID, other.ID, e.account(false), ""); err != nil {
		t.Errorf("an invitation to another room: %v", err)
	}
}

func roomsBlocks(t *testing.T, e *roomsEnv) {
	host, cohost := e.account(true), e.account(false)
	blockedByHost, blocksHost, blocksCohost, plainA, plainB := e.account(false), e.account(false), e.account(false), e.account(false), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID
	for _, id := range []string{cohost, blockedByHost, blocksHost, blocksCohost, plainA, plainB} {
		e.join(t, id, r)
	}
	e.cohost(t, r, cohost)
	elsewhere := e.account(true)
	e.open(t, elsewhere, appvoice.SettingsInput{})

	e.block(t, host, blockedByHost)
	e.block(t, blocksHost, host)
	e.block(t, blocksCohost, cohost)
	e.block(t, plainA, plainB)
	e.block(t, host, elsewhere)
	for id, in := range map[string]bool{
		host: true, cohost: true, blockedByHost: false, blocksHost: false, blocksCohost: false, plainA: true, plainB: true,
	} {
		e.wantParticipant(t, r, id, in)
	}
	if ps := e.participants(t, r); len(ps) != 4 {
		t.Errorf("participants = %d, want 4", len(ps))
	}

	// Of two co-hosts, the blocked one leaves.
	other := e.account(false)
	e.join(t, other, r)
	e.cohost(t, r, other)
	e.block(t, cohost, other)
	e.wantParticipant(t, r, cohost, true)
	e.wantParticipant(t, r, other, false)
	_, err := e.svc.Join(t.Context(), blockedByHost, r)
	wantErr(t, "Join after a block of the host", err, appvoice.ErrNotFound)
}

// roomsRaces commits a change between the reads of a command and its transaction, through the
// hooks of the relations, and checks that the transaction decides on what committed.
func roomsRaces(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, cohost := e.account(true), e.account(false)
	r := e.open(t, host, appvoice.SettingsInput{}).Room.ID // speakers: manual
	e.join(t, cohost, r)
	e.cohost(t, r, cohost)
	fired := func(t *testing.T, done bool) {
		t.Helper()
		if !done {
			t.Fatal("the hook did not run")
		}
	}

	// Speak: the co-host loses the role after Speak read the room.
	done := false
	e.members.after("follows", 3, func() {
		done = true
		if _, err := e.svc.RemoveCohost(ctx, host, r, cohost); err != nil {
			t.Error(err)
		}
	})
	_, err := e.svc.Speak(ctx, cohost, r)
	fired(t, done)
	wantErr(t, "Speak by a co-host removed meanwhile", err, appvoice.ErrCannotSpeak)
	if p := e.wantParticipant(t, r, cohost, true); p.Role != voice.RoleListener {
		t.Errorf("after the refused Speak = %+v, want a listener", p)
	}

	// AssignCohost: a participant joins after the blocks were read.
	target, newcomer := e.account(false), e.account(false)
	e.join(t, target, r)
	done = false
	e.members.after("blocked", 1, func() {
		done = true
		e.join(t, newcomer, r)
	})
	_, err = e.svc.AssignCohost(ctx, host, r, target)
	fired(t, done)
	wantErr(t, "AssignCohost while a participant joined", err, appvoice.ErrConflict)
	if p := e.wantParticipant(t, r, target, true); p.Cohost() {
		t.Error("the refused co-host is a co-host")
	}
	if _, err := e.svc.AssignCohost(ctx, host, r, target); err != nil {
		t.Fatalf("AssignCohost again: %v", err)
	}

	// Join: a co-host in a block relation with the account is assigned after Join read the
	// blocks; joining again sees the room no more.
	joiner, blocker := e.account(false), e.account(false)
	e.join(t, blocker, r)
	e.members.set(func() { e.members.blocks[[2]string{blocker, joiner}] = true })
	before, err := e.svc.Get(ctx, joiner, r)
	if err != nil {
		t.Fatal(err)
	}
	reads := len(guardIDs(before.Participants, before.Head.HostAccountID))
	done = false
	e.members.after("blocked", 2*reads, func() {
		done = true
		if _, err := e.svc.AssignCohost(ctx, host, r, blocker); err != nil {
			t.Error(err)
		}
	})
	_, err = e.svc.Join(ctx, joiner, r)
	fired(t, done)
	wantErr(t, "Join while a blocking co-host was assigned", err, appvoice.ErrConflict)
	e.wantParticipant(t, r, joiner, false)
	_, err = e.svc.Join(ctx, joiner, r)
	wantErr(t, "Join after the blocking co-host was assigned", err, appvoice.ErrNotFound)

	// Invite: the invitee is removed and banned after Invite read the blocks.
	invitee := e.account(false)
	e.join(t, invitee, r)
	done = false
	e.members.after("blocked", 1, func() {
		done = true
		if err := e.svc.Remove(ctx, host, r, invitee); err != nil {
			t.Error(err)
		}
	})
	_, err = e.svc.Invite(ctx, host, r, invitee, "")
	fired(t, done)
	wantErr(t, "Invite of an account banned meanwhile", err, appvoice.ErrInvalidInvitee)

	// Mute: the transport mutes the session that the participation has when the job runs.
	muted := e.account(false)
	e.join(t, muted, r)
	session := e.join(t, muted, r).Session // from another device
	if err := e.svc.Mute(ctx, host, r, muted); err != nil {
		t.Fatal(err)
	}
	if e.transport.count("mute:"+session) != 1 {
		t.Errorf("transport calls = %v, want the new session muted", e.transport.calls)
	}
}

// guardIDs returns the host and the co-hosts among ps.
func guardIDs(ps []appvoice.Participant, host string) []string {
	ids := []string{host}
	for _, p := range ps {
		if p.Cohost() && p.AccountID != host {
			ids = append(ids, p.AccountID)
		}
	}
	return ids
}
