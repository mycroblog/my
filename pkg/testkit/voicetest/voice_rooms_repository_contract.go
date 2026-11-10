// SPDX-License-Identifier: AGPL-3.0-only

package voicetest

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

// VoiceRoomsContract verifies the methods of the voice repository that the room use cases
// read beyond VoiceRepositoryContract, by the methods alone (the port contract of ADR-0153 §1):
// the rooms an account hosted, newest first in pages; the end of a participation; the
// invitations that were not revoked, in pages; the records of a room (its revisions,
// participations with their ends, and bans), the accounts they name, the rooms whose records
// the purge erases and the purge itself; the rooms an account revised and the erasure of the
// texts of its revisions; and, on every dialect (ADR-0153 §5), the locks of the opening of rooms
// and of a room, under which concurrent transactions that count and then write apply one at a
// time. The room use cases on top of it are VoiceRoomsFlows, which internal/flowtest/voice runs
// once on SQLite.
func VoiceRoomsContract(t *testing.T, newHarness VoiceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *voiceEnv)
	}{
		{"ListsTheRoomsAnAccountHosted", voiceRepoHostedRooms},
		{"ReadsTheEndOfAParticipation", voiceRepoParticipationEnd},
		{"ListsTheInvitations", voiceRepoInvitations},
		{"ReadsAndPurgesTheRecords", voiceRepoRecords},
		{"ErasesTheTextsOfAnAccount", voiceRepoErasure},
		{"SerializesTheOpeningOfRooms", voiceRepoLockOpening},
		{"SerializesTheCommandsOfARoom", voiceRepoLockRoom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newVoiceEnv(t, newHarness))
		})
	}
}

func roomIDs(recs []appvoice.RoomRecord) []string {
	var out []string
	for _, r := range recs {
		out = append(out, r.ID)
	}
	return out
}

func voiceRepoHostedRooms(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	alice, bob := e.ids.NewID(), e.ids.NewID()
	first := e.openRoom(t, alice, voiceSettings("First"), true)
	handed := e.openRoom(t, alice, voiceSettings("Handed"), true)
	e.changeHost(t, handed.ID, bob, voice.HostHandedOver)
	latest := e.openRoom(t, alice, voiceSettings("Latest"), true)
	e.openRoom(t, e.ids.NewID(), voiceSettings("Other"), true)
	for _, c := range []struct {
		name, account, before string
		limit                 int
		want                  []string
	}{
		{"every room", alice, "", 10, []string{latest.ID, handed.ID, first.ID}},
		{"a page", alice, "", 2, []string{latest.ID, handed.ID}},
		{"the next page", alice, handed.ID, 2, []string{first.ID}},
		{"a room handed over", bob, "", 10, []string{handed.ID}},
		{"none", e.ids.NewID(), "", 10, nil},
	} {
		got, err := e.h.Voice.HostedRooms(ctx, nil, c.account, c.before, c.limit)
		if err != nil || !slices.Equal(roomIDs(got), c.want) {
			t.Errorf("HostedRooms(%s) = %v, %v; want %v", c.name, roomIDs(got), err, c.want)
		}
	}
	got, err := e.h.Voice.HostedRooms(ctx, nil, bob, "", 1)
	if err != nil || len(got) != 1 || got[0].Head.HostAccountID != bob || got[0].Revision.Settings.Name != "Handed" {
		t.Errorf("HostedRooms(bob) = %+v, %v; want the room with its head and revision", got, err)
	}
}

func voiceRepoParticipationEnd(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	room := e.openRoom(t, e.ids.NewID(), voiceSettings("Room"), true)
	p := e.join(t, room.ID, e.ids.NewID(), voice.RoleListener)
	if _, err := e.h.Voice.FindParticipationEnd(ctx, nil, p.ID); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("FindParticipationEnd before the end = %v, want ErrNotFound", err)
	}
	e.leave(t, p, voice.LeftRoom)
	end, err := e.h.Voice.FindParticipationEnd(ctx, nil, p.ID)
	if err != nil || end.ParticipationID != p.ID || end.Reason != voice.LeftRoom || end.EndedAt.IsZero() {
		t.Errorf("FindParticipationEnd = %+v, %v; want the end of the participation", end, err)
	}
	e.tx(t, func(tx uow.Tx) error {
		got, err := e.h.Voice.FindParticipationEnd(ctx, tx, p.ID)
		if err == nil && got != end {
			t.Errorf("FindParticipationEnd in a transaction = %+v, want %+v", got, end)
		}
		return err
	})
}

func voiceRepoInvitations(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	host := e.ids.NewID()
	room := e.openRoom(t, host, voiceSettings("Invited"), true)
	other := e.openRoom(t, e.ids.NewID(), voiceSettings("Other"), true)
	invite := func(roomID string) appvoice.Invitation {
		inv := appvoice.Invitation{ID: e.ids.NewID(), RoomID: roomID, InviteeAccountID: e.ids.NewID(), Role: voice.RoleSpeaker, InvitedByAccountID: host, InvitedAt: e.now()}
		e.tx(t, func(tx uow.Tx) error {
			_, err := e.h.Voice.InsertInvitation(ctx, tx, inv)
			return err
		})
		return inv
	}
	a, revoked, b, c := invite(room.ID), invite(room.ID), invite(room.ID), invite(room.ID)
	invite(other.ID)
	e.tx(t, func(tx uow.Tx) error {
		_, err := e.h.Voice.InsertInvitationRevocation(ctx, tx, appvoice.InvitationRevocation{InvitationID: revoked.ID, RevokedByAccountID: host, RevokedAt: e.now()})
		return err
	})
	for _, q := range []struct {
		name, after string
		limit       int
		want        []appvoice.Invitation
	}{
		{"every invitation", "", 10, []appvoice.Invitation{a, b, c}},
		{"a page", "", 2, []appvoice.Invitation{a, b}},
		{"the next page", b.ID, 2, []appvoice.Invitation{c}},
		{"after a revoked one", revoked.ID, 10, []appvoice.Invitation{b, c}},
	} {
		got, err := e.h.Voice.Invitations(ctx, nil, room.ID, q.after, q.limit)
		if err != nil || !reflect.DeepEqual(got, q.want) {
			t.Errorf("Invitations(%s) = %+v, %v; want %+v", q.name, got, err, q.want)
		}
	}
}

func voiceRepoRecords(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	host, alice, bob := e.ids.NewID(), e.ids.NewID(), e.ids.NewID()
	room := e.openRoom(t, host, voiceSettings("Recorded", "a"), true)
	rev := e.revise(t, room.ID, host, voiceSettings("Renamed", "b", "c"))
	hostPart := e.join(t, room.ID, host, voice.RoleSpeaker)
	alicePart := e.join(t, room.ID, alice, voice.RoleListener)
	e.role(t, alicePart, voice.RoleSpeaker, host)
	e.leave(t, alicePart, voice.LeftRoom)
	ban := appvoice.Ban{ID: e.ids.NewID(), RoomID: room.ID, AccountID: bob, BannedByAccountID: host, BannedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		_, err := e.h.Voice.InsertBan(ctx, tx, ban)
		return err
	})
	ending, _ := e.end(t, room.ID, voice.EndHost, host, hostPart)
	other := e.openRoom(t, e.ids.NewID(), voiceSettings("Other"), true)
	otherPart := e.join(t, other.ID, e.ids.NewID(), voice.RoleSpeaker)
	e.end(t, other.ID, voice.EndHost, other.Head.HostAccountID, otherPart)
	open := e.openRoom(t, e.ids.NewID(), voiceSettings("Open"), true)
	e.join(t, open.ID, e.ids.NewID(), voice.RoleListener)

	e.tx(t, func(tx uow.Tx) error {
		revs, err := e.h.Voice.Revisions(ctx, tx, room.ID)
		if err != nil {
			return err
		}
		if len(revs) != 2 || !reflect.DeepEqual(revs[0], room.Revision) || !reflect.DeepEqual(revs[1], rev) {
			t.Errorf("Revisions = %+v; want the two revisions with their hashtags, oldest first", revs)
		}
		parts, err := e.h.Voice.Participations(ctx, tx, room.ID)
		if err != nil {
			return err
		}
		if len(parts) != 2 || parts[0].Participation != hostPart || parts[0].End == nil || parts[0].End.Reason != voice.RoomEnded ||
			parts[1].Participation != alicePart || parts[1].End == nil || parts[1].End.Reason != voice.LeftRoom {
			t.Errorf("Participations = %+v; want the two with their ends, in the order they started", parts)
		}
		bans, err := e.h.Voice.Bans(ctx, tx, room.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(bans, []appvoice.Ban{ban}) {
			t.Errorf("Bans = %+v, want %+v", bans, ban)
		}
		accounts, err := e.h.Voice.RecordAccounts(ctx, tx, room.ID)
		slices.Sort(accounts)
		if want := testkit.SortedIDs(host, alice, bob); !slices.Equal(accounts, want) {
			t.Errorf("RecordAccounts = %v, want %v", accounts, want)
		}
		return err
	})

	toPurge := func(after string, limit int) []string {
		t.Helper()
		var ids []string
		e.tx(t, func(tx uow.Tx) (err error) {
			ids, err = e.h.Voice.RoomsToPurge(ctx, tx, e.clock.Now().Add(1), after, limit)
			return err
		})
		return ids
	}
	ended := testkit.SortedIDs(room.ID, other.ID)
	if got := toPurge("", 10); !slices.Equal(got, ended) {
		t.Errorf("RoomsToPurge = %v, want the ended rooms %v", got, ended)
	}
	if got := toPurge("", 1); !slices.Equal(got, ended[:1]) {
		t.Errorf("RoomsToPurge(limit 1) = %v, want %v", got, ended[:1])
	}
	if got := toPurge(ended[0], 10); !slices.Equal(got, ended[1:]) {
		t.Errorf("RoomsToPurge(after the first) = %v, want %v", got, ended[1:])
	}
	e.tx(t, func(tx uow.Tx) (err error) {
		ids, err := e.h.Voice.RoomsToPurge(ctx, tx, ending.EndedAt, "", 10)
		if !slices.Equal(ids, nil) {
			t.Errorf("RoomsToPurge before the first ending = %v, want none", ids)
		}
		return err
	})
	e.tx(t, func(tx uow.Tx) error { return e.h.Voice.PurgeParticipantRecords(ctx, tx, []string{room.ID}) })
	if got := toPurge("", 10); !slices.Equal(got, []string{other.ID}) {
		t.Errorf("RoomsToPurge after the purge = %v, want the other room", got)
	}
	e.tx(t, func(tx uow.Tx) error {
		parts, err := e.h.Voice.Participations(ctx, tx, room.ID)
		if err != nil {
			return err
		}
		bans, err := e.h.Voice.Bans(ctx, tx, room.ID)
		if err != nil {
			return err
		}
		accounts, err := e.h.Voice.RecordAccounts(ctx, tx, room.ID)
		if len(parts)+len(bans)+len(accounts) != 0 {
			t.Errorf("after the purge: participations %+v, bans %+v, accounts %v; want none", parts, bans, accounts)
		}
		revs, rerr := e.h.Voice.Revisions(ctx, tx, room.ID)
		if rerr != nil || len(revs) != 2 {
			t.Errorf("Revisions after the purge = %+v, %v; want them kept", revs, rerr)
		}
		return err
	})
	if rec := e.find(t, room.ID); rec.Head.State != voice.StateEnded {
		t.Errorf("FindRoom after the purge = %+v, want the ended room kept", rec.Head)
	}
}

func voiceRepoErasure(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	alice, bob := e.ids.NewID(), e.ids.NewID()
	first := e.openRoom(t, alice, voiceSettings("Alice's"), true)
	second := e.openRoom(t, bob, voiceSettings("Bob's"), true)
	e.revise(t, second.ID, alice, voiceSettings("Renamed by Alice", "a"))
	third := e.openRoom(t, alice, voice.Settings{Visibility: voice.VisibilityLocal, Speakers: voice.SpeakersManual}, true)
	revised := func(limit int) []string {
		t.Helper()
		var ids []string
		e.tx(t, func(tx uow.Tx) (err error) {
			ids, err = e.h.Voice.RoomsRevisedBy(ctx, tx, alice, limit)
			return err
		})
		return ids
	}
	want := testkit.SortedIDs(first.ID, second.ID)
	if got := revised(10); !slices.Equal(got, want) {
		t.Errorf("RoomsRevisedBy = %v, want the rooms with texts %v (not %s)", got, want, third.ID)
	}
	if got := revised(1); !slices.Equal(got, want[:1]) {
		t.Errorf("RoomsRevisedBy(limit 1) = %v, want %v", got, want[:1])
	}
	e.tx(t, func(tx uow.Tx) error { return e.h.Voice.EraseRevisionTexts(ctx, tx, alice, want) })
	if got := revised(10); len(got) != 0 {
		t.Errorf("RoomsRevisedBy after the erasure = %v, want none", got)
	}
	e.tx(t, func(tx uow.Tx) error {
		revs, err := e.h.Voice.Revisions(ctx, tx, second.ID)
		if err == nil && (len(revs) != 2 || revs[0].Settings.Name != "Bob's" || revs[1].Settings.Name != "" || revs[1].Settings.Description != "") {
			t.Errorf("Revisions of Bob's room = %+v; want Bob's kept and Alice's emptied", revs)
		}
		return err
	})
}

// voiceRepoLockOpening opens rooms from concurrent transactions that each lock the opening,
// count the open rooms, and open one only when there is none: one room opens.
func voiceRepoLockOpening(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	opened := make([]bool, 6)
	for i, err := range parallel(len(opened), func(i int) error {
		host := e.ids.NewID()
		return e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			opened[i] = false
			if err := e.h.Voice.LockOpening(ctx, tx); err != nil {
				return err
			}
			open, err := e.h.Voice.OpenRooms(ctx, tx, "")
			if err != nil || len(open) > 0 {
				return err
			}
			at := e.clock.Now()
			room := appvoice.Room{ID: e.ids.NewID(), OpenedByAccountID: host, OpenedAt: at, Transport: voice.TransportP2P}
			rev := appvoice.Revision{ID: e.ids.NewID(), RoomID: room.ID, Settings: voiceSettings("Race"), RevisedByAccountID: host, RevisedAt: at}
			change := appvoice.HostChange{ID: e.ids.NewID(), RoomID: room.ID, HostAccountID: host, Reason: voice.HostOpened, ChangedAt: at}
			if err := errors.Join(e.h.Voice.InsertRoom(ctx, tx, room), e.h.Voice.InsertRevision(ctx, tx, rev), e.h.Voice.InsertHostChange(ctx, tx, change)); err != nil {
				return err
			}
			opened[i] = true
			return tx.Emit(ctx, appvoice.RoomOpened{Room: room, Revision: rev, Host: change})
		})
	}) {
		if err != nil {
			t.Fatalf("transaction %d: %v", i, err)
		}
	}
	n := 0
	for _, ok := range opened {
		if ok {
			n++
		}
	}
	if got := e.openRoomIDs(t, ""); n != 1 || len(got) != 1 {
		t.Errorf("%d transactions opened a room, open rooms %v; want one", n, got)
	}
}

// voiceRepoLockRoom joins a room from concurrent transactions that each lock the room, count its
// participants, and join only below a cap of 3: the room has 3 participants.
func voiceRepoLockRoom(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	room := e.openRoom(t, e.ids.NewID(), voiceSettings("Capped"), true)
	const capacity = 3
	for i, err := range parallel(8, func(int) error {
		p := appvoice.Participation{ID: e.ids.NewID(), RoomID: room.ID, AccountID: e.ids.NewID(), JoinedAt: e.clock.Now()}
		return e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			if err := e.h.Voice.LockRoom(ctx, tx, room.ID); err != nil {
				return err
			}
			ps, err := e.h.Voice.Participants(ctx, tx, room.ID)
			if err != nil || len(ps) >= capacity {
				return err
			}
			if err := e.h.Voice.InsertParticipation(ctx, tx, p); err != nil {
				return err
			}
			return tx.Emit(ctx, appvoice.ParticipationStarted{Participation: p})
		})
	}) {
		if err != nil {
			t.Fatalf("transaction %d: %v", i, err)
		}
	}
	if ps := e.participants(t, room.ID); len(ps) != capacity {
		t.Errorf("participants = %d, want the cap of %d", len(ps), capacity)
	}
	// The participants of many rooms at once, as those of each.
	empty := e.openRoom(t, e.ids.NewID(), voiceSettings("Empty"), true)
	of, err := e.h.Voice.ParticipantsOfRooms(ctx, nil, []string{room.ID, empty.ID, e.ids.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{room.ID, empty.ID} {
		one, err := e.h.Voice.Participants(ctx, nil, id)
		if err != nil || len(of[id]) != len(one) || len(one) > 0 && !reflect.DeepEqual(of[id], one) {
			t.Errorf("ParticipantsOfRooms[%s] = %+v; want %+v, as Participants (%v)", id, of[id], one, err)
		}
	}
}
