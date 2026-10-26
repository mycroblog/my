// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// VoiceHarness is what VoiceRepositoryContract needs from a database adapter: its unit of work
// and the voice repository.
type VoiceHarness struct {
	UnitOfWork uow.UnitOfWork
	Voice      appvoice.Repository
}

// VoiceFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type VoiceFactory func(t *testing.T, opts uow.Options) VoiceHarness

// VoiceRepositoryContract verifies the voice repository of a database adapter (ADR-0100 §12,
// data-model §4.14): a room reads back with its settings, hashtags, host, and ending; the open
// rooms list by the most participants and filter by a hashtag of the current revision; the
// participants carry their role, co-host nomination, and raised hand, one room per account; a
// co-host nomination lasts while its participation does and ends when the co-host becomes the
// host; an ended room keeps no participants;
// participation ends, invitations, their revocations, bans, and endings are recorded once; the
// speak grants and the host grace read back; and the projections that the projectors keep
// rebuild from the events alone.
func VoiceRepositoryContract(t *testing.T, newHarness VoiceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *voiceEnv)
	}{
		{"OpensAndReadsARoom", voiceOpen},
		{"RevisesAndChangesTheHost", voiceRevise},
		{"TracksParticipants", voiceParticipants},
		{"KeepsOneRoomPerAccount", voiceOneRoom},
		{"EndsCohostsWithTheirParticipationOrHosting", voiceCohosts},
		{"KeepsNoParticipantsInAnEndedRoom", voiceEndedParticipants},
		{"RecordsInvitationsBansAndGrants", voiceInvitations},
		{"EndsARoomOnce", voiceEnd},
		{"KeepsTheHostGrace", voiceGrace},
		{"RebuildsItsProjections", voiceRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newVoiceEnv(t, newHarness))
		})
	}
}

type voiceEnv struct {
	h     VoiceHarness
	clock *Clock
	ids   *IDGenerator
}

func newVoiceEnv(t *testing.T, newHarness VoiceFactory) *voiceEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appvoice.RegisterProjectors(projectors, h.Voice)
	clock := NewClock(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC))
	return &voiceEnv{h: h, clock: clock, ids: NewIDGenerator(clock)}
}

func (e *voiceEnv) tx(t *testing.T, fn func(tx uow.Tx) error) {
	t.Helper()
	if err := e.h.UnitOfWork.Tx(t.Context(), fn); err != nil {
		t.Fatal(err)
	}
}

func (e *voiceEnv) now() time.Time { return e.clock.Advance(time.Second) }

func voiceSettings(name string, tags ...string) voice.Settings {
	s := voice.Settings{Name: name, Description: "About " + name, Visibility: voice.VisibilityLocal, Speakers: voice.SpeakersManual}
	for _, tag := range tags {
		s.Hashtags = append(s.Hashtags, voice.Hashtag{Name: "Tag" + tag, NameNormalized: "tag" + tag})
	}
	return s
}

// openRoom writes a room, its first revision, and its host, and emits RoomOpened when emit is
// true; without it, the projections do not see the room until a rebuild.
func (e *voiceEnv) openRoom(t *testing.T, host string, s voice.Settings, emit bool) appvoice.RoomRecord {
	t.Helper()
	at := e.now()
	room := appvoice.Room{ID: e.ids.NewID(), OpenedByAccountID: host, OpenedAt: at, Transport: voice.TransportP2P}
	rev := appvoice.Revision{ID: e.ids.NewID(), RoomID: room.ID, Settings: s, RevisedByAccountID: host, RevisedAt: at}
	change := appvoice.HostChange{ID: e.ids.NewID(), RoomID: room.ID, HostAccountID: host, Reason: voice.HostOpened, ChangedAt: at}
	e.tx(t, func(tx uow.Tx) error {
		ctx := t.Context()
		if err := e.h.Voice.InsertRoom(ctx, tx, room); err != nil {
			return err
		}
		if err := e.h.Voice.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := e.h.Voice.InsertHostChange(ctx, tx, change); err != nil {
			return err
		}
		if !emit {
			return nil
		}
		return tx.Emit(ctx, appvoice.RoomOpened{Room: room, Revision: rev, Host: change})
	})
	return appvoice.RoomRecord{
		Room:     room,
		Head:     appvoice.Head{RoomID: room.ID, RevisionID: rev.ID, HostAccountID: host, State: voice.StateOpen},
		Revision: rev,
	}
}

func (e *voiceEnv) revise(t *testing.T, roomID, by string, s voice.Settings) appvoice.Revision {
	t.Helper()
	rev := appvoice.Revision{ID: e.ids.NewID(), RoomID: roomID, Settings: s, RevisedByAccountID: by, RevisedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		if err := e.h.Voice.InsertRevision(t.Context(), tx, rev); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appvoice.RoomRevised{Revision: rev})
	})
	return rev
}

func (e *voiceEnv) changeHost(t *testing.T, roomID, host string, reason voice.HostChangeReason) {
	t.Helper()
	c := appvoice.HostChange{ID: e.ids.NewID(), RoomID: roomID, HostAccountID: host, Reason: reason, ChangedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		if err := e.h.Voice.InsertHostChange(t.Context(), tx, c); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appvoice.HostChanged{Change: c})
	})
}

func (e *voiceEnv) cohost(t *testing.T, roomID, account string, change voice.CohostChange) appvoice.CohostChange {
	t.Helper()
	c := appvoice.CohostChange{ID: e.ids.NewID(), RoomID: roomID, AccountID: account, Change: change, ChangedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		if err := e.h.Voice.InsertCohostChange(t.Context(), tx, c); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appvoice.CohostChanged{Change: c})
	})
	return c
}

func (e *voiceEnv) participation(roomID, account string) appvoice.Participation {
	return appvoice.Participation{ID: e.ids.NewID(), RoomID: roomID, AccountID: account, JoinedAt: e.now()}
}

// joinIn writes a participation, and its first role change for a speaker, and emits
// ParticipationStarted when emit is true.
func (e *voiceEnv) joinIn(t *testing.T, tx uow.Tx, p appvoice.Participation, role voice.Role, emit bool) error {
	t.Helper()
	ctx := t.Context()
	if err := e.h.Voice.InsertParticipation(ctx, tx, p); err != nil {
		return err
	}
	if role == voice.RoleSpeaker {
		c := appvoice.RoleChange{ID: e.ids.NewID(), ParticipationID: p.ID, Role: role, ChangedByAccountID: p.AccountID, ChangedAt: p.JoinedAt}
		if err := e.h.Voice.InsertRoleChange(ctx, tx, c); err != nil {
			return err
		}
	}
	if !emit {
		return nil
	}
	return tx.Emit(ctx, appvoice.ParticipationStarted{Participation: p})
}

func (e *voiceEnv) join(t *testing.T, roomID, account string, role voice.Role) appvoice.Participation {
	t.Helper()
	p := e.participation(roomID, account)
	e.tx(t, func(tx uow.Tx) error { return e.joinIn(t, tx, p, role, true) })
	return p
}

func (e *voiceEnv) leaveIn(t *testing.T, tx uow.Tx, p appvoice.Participation, reason voice.ParticipationEndReason) (bool, error) {
	t.Helper()
	end := appvoice.ParticipationEnd{ParticipationID: p.ID, Reason: reason, EndedAt: e.now()}
	ok, err := e.h.Voice.InsertParticipationEnd(t.Context(), tx, end)
	if err != nil || !ok {
		return ok, err
	}
	return true, tx.Emit(t.Context(), appvoice.ParticipationEnded{Participation: p, End: end})
}

func (e *voiceEnv) leave(t *testing.T, p appvoice.Participation, reason voice.ParticipationEndReason) bool {
	t.Helper()
	var ended bool
	e.tx(t, func(tx uow.Tx) error {
		var err error
		ended, err = e.leaveIn(t, tx, p, reason)
		return err
	})
	return ended
}

func (e *voiceEnv) role(t *testing.T, p appvoice.Participation, role voice.Role, by string) {
	t.Helper()
	c := appvoice.RoleChange{ID: e.ids.NewID(), ParticipationID: p.ID, Role: role, ChangedByAccountID: by, ChangedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		if err := e.h.Voice.InsertRoleChange(t.Context(), tx, c); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appvoice.RoleChanged{Participation: p, Change: c})
	})
}

func (e *voiceEnv) hand(t *testing.T, p appvoice.Participation, change voice.HandRaiseChange) {
	t.Helper()
	h := appvoice.HandRaise{ID: e.ids.NewID(), ParticipationID: p.ID, Change: change, ChangedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		if err := e.h.Voice.InsertHandRaise(t.Context(), tx, h); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appvoice.HandRaiseChanged{Participation: p, Change: h})
	})
}

// end ends the participations ps with room_ended and then the room, and reports whether the
// ending was recorded.
func (e *voiceEnv) end(t *testing.T, roomID string, reason voice.EndReason, by string, ps ...appvoice.Participation) (appvoice.Ending, bool) {
	t.Helper()
	ending := appvoice.Ending{RoomID: roomID, Reason: reason, EndedByAccountID: by, EndedAt: e.now()}
	var ok bool
	e.tx(t, func(tx uow.Tx) error {
		for _, p := range ps {
			if _, err := e.leaveIn(t, tx, p, voice.RoomEnded); err != nil {
				return err
			}
		}
		var err error
		if ok, err = e.h.Voice.InsertEnding(t.Context(), tx, ending); err != nil || !ok {
			return err
		}
		return tx.Emit(t.Context(), appvoice.RoomEnded{Ending: ending})
	})
	return ending, ok
}

func (e *voiceEnv) find(t *testing.T, roomID string) appvoice.RoomRecord {
	t.Helper()
	rec, err := e.h.Voice.FindRoom(t.Context(), nil, roomID)
	if err != nil {
		t.Fatalf("FindRoom(%s) = %v", roomID, err)
	}
	return rec
}

func (e *voiceEnv) openRoomIDs(t *testing.T, tag string) []string {
	t.Helper()
	recs, err := e.h.Voice.OpenRooms(t.Context(), nil, tag)
	if err != nil {
		t.Fatalf("OpenRooms(%q) = %v", tag, err)
	}
	ids := []string{}
	for _, r := range recs {
		ids = append(ids, r.ID)
	}
	return ids
}

func (e *voiceEnv) participants(t *testing.T, roomID string) []appvoice.Participant {
	t.Helper()
	ps, err := e.h.Voice.Participants(t.Context(), nil, roomID)
	if err != nil {
		t.Fatalf("Participants(%s) = %v", roomID, err)
	}
	return ps
}

func voiceOpen(t *testing.T, e *voiceEnv) {
	host := NamedID("voice-host")
	want := e.openRoom(t, host, voiceSettings("Friday night", "go", "music"), true)
	if got := e.find(t, want.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("FindRoom =\n%+v\nwant\n%+v", got, want)
	}
	if _, err := e.h.Voice.FindRoom(t.Context(), nil, NamedID("no-such-room")); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("FindRoom(unknown) = %v, want ErrNotFound", err)
	}
	recs, err := e.h.Voice.OpenRooms(t.Context(), nil, "")
	if err != nil || len(recs) != 1 || !reflect.DeepEqual(recs[0], want) {
		t.Errorf("OpenRooms = %+v, %v; want the room", recs, err)
	}
	for tag, n := range map[string]int{"taggo": 1, "tagmusic": 1, "tagother": 0, "TagGo": 0} {
		if got := e.openRoomIDs(t, tag); len(got) != n {
			t.Errorf("OpenRooms(%q) = %v, want %d rooms", tag, got, n)
		}
	}
	// The same transaction reads it.
	e.tx(t, func(tx uow.Tx) error {
		got, err := e.h.Voice.FindRoom(t.Context(), tx, want.ID)
		if err == nil && !reflect.DeepEqual(got, want) {
			t.Errorf("FindRoom in a transaction = %+v", got)
		}
		return err
	})
}

func voiceRevise(t *testing.T, e *voiceEnv) {
	host, next := NamedID("voice-host"), NamedID("voice-next")
	room := e.openRoom(t, host, voiceSettings("Before", "go"), true)
	s := voiceSettings("After", "music", "art")
	s.Description = ""
	s.Visibility, s.Speakers = voice.VisibilityMutuals, voice.SpeakersFollowers
	rev := e.revise(t, room.ID, host, s)
	got := e.find(t, room.ID)
	if !reflect.DeepEqual(got.Revision, rev) || got.Head.RevisionID != rev.ID {
		t.Errorf("revision = %+v (head %s), want %+v", got.Revision, got.Head.RevisionID, rev)
	}
	if ids := e.openRoomIDs(t, "taggo"); len(ids) != 0 {
		t.Errorf("OpenRooms(taggo) = %v, want none: the current revision has no such tag", ids)
	}
	if ids := e.openRoomIDs(t, "tagart"); len(ids) != 1 {
		t.Errorf("OpenRooms(tagart) = %v, want the room", ids)
	}
	e.changeHost(t, room.ID, next, voice.HostHandedOver)
	if got := e.find(t, room.ID); got.Head.HostAccountID != next || got.OpenedByAccountID != host {
		t.Errorf("after the handover: host %s, opened by %s; want %s, %s", got.Head.HostAccountID, got.OpenedByAccountID, next, host)
	}
}

func voiceParticipants(t *testing.T, e *voiceEnv) {
	host, alice, bob := NamedID("voice-host"), NamedID("voice-alice"), NamedID("voice-bob")
	busy := e.openRoom(t, host, voiceSettings("Busy"), true)
	quiet := e.openRoom(t, NamedID("voice-other-host"), voiceSettings("Quiet"), true)
	if ids := e.openRoomIDs(t, ""); !reflect.DeepEqual(ids, []string{busy.ID, quiet.ID}) {
		t.Errorf("OpenRooms with no participants = %v, want by room ID", ids)
	}
	ph := e.join(t, busy.ID, host, voice.RoleSpeaker)
	pa := e.join(t, busy.ID, alice, voice.RoleListener)
	pb := e.join(t, quiet.ID, bob, voice.RoleListener)
	e.leave(t, pb, voice.LeftRoom)
	pb = e.join(t, busy.ID, bob, voice.RoleListener)
	e.join(t, quiet.ID, NamedID("voice-carol"), voice.RoleListener)
	if ids := e.openRoomIDs(t, ""); !reflect.DeepEqual(ids, []string{busy.ID, quiet.ID}) {
		t.Errorf("OpenRooms = %v, want the busy room first", ids)
	}
	if n := e.find(t, busy.ID).Head.ParticipantsCount; n != 3 {
		t.Errorf("participants of the busy room = %d, want 3", n)
	}

	e.role(t, pa, voice.RoleSpeaker, host)
	assigned := e.cohost(t, busy.ID, bob, voice.CohostAssigned)
	e.hand(t, pb, voice.HandRaised)
	want := []appvoice.Participant{
		{AccountID: host, RoomID: busy.ID, ParticipationID: ph.ID, Role: voice.RoleSpeaker},
		{AccountID: alice, RoomID: busy.ID, ParticipationID: pa.ID, Role: voice.RoleSpeaker},
		{AccountID: bob, RoomID: busy.ID, ParticipationID: pb.ID, Role: voice.RoleListener, CohostChangeID: assigned.ID, HandRaised: true},
	}
	if got := e.participants(t, busy.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("Participants =\n%+v\nwant\n%+v", got, want)
	}
	if got, err := e.h.Voice.FindParticipant(t.Context(), nil, bob); err != nil || !reflect.DeepEqual(got, want[2]) || !got.Cohost() {
		t.Errorf("FindParticipant(bob) = %+v, %v; want %+v", got, err, want[2])
	}

	e.hand(t, pb, voice.HandGranted)
	e.role(t, pb, voice.RoleSpeaker, host)
	e.cohost(t, busy.ID, bob, voice.CohostRemoved)
	e.role(t, pa, voice.RoleListener, alice)
	want[1].Role = voice.RoleListener
	want[2] = appvoice.Participant{AccountID: bob, RoomID: busy.ID, ParticipationID: pb.ID, Role: voice.RoleSpeaker}
	if got := e.participants(t, busy.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("Participants after the changes =\n%+v\nwant\n%+v", got, want)
	}

	if !e.leave(t, pa, voice.LeftRoom) {
		t.Error("the first end of a participation was not recorded")
	}
	if e.leave(t, pa, voice.Disconnected) {
		t.Error("a second end of a participation was recorded")
	}
	if got := e.participants(t, busy.ID); !reflect.DeepEqual(got, []appvoice.Participant{want[0], want[2]}) {
		t.Errorf("Participants after alice left = %+v", got)
	}
	if n := e.find(t, busy.ID).Head.ParticipantsCount; n != 2 {
		t.Errorf("participants after alice left = %d, want 2", n)
	}
	if _, err := e.h.Voice.FindParticipant(t.Context(), nil, alice); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("FindParticipant(alice) after leaving = %v, want ErrNotFound", err)
	}
}

func voiceOneRoom(t *testing.T, e *voiceEnv) {
	alice := NamedID("voice-alice")
	first := e.openRoom(t, NamedID("voice-host-1"), voiceSettings("First"), true)
	second := e.openRoom(t, NamedID("voice-host-2"), voiceSettings("Second"), true)
	p := e.join(t, first.ID, alice, voice.RoleListener)
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.joinIn(t, tx, e.participation(second.ID, alice), voice.RoleListener, true)
	})
	if !errors.Is(err, appvoice.ErrInAnotherRoom) {
		t.Fatalf("joining a second room = %v, want ErrInAnotherRoom", err)
	}
	if got, err := e.h.Voice.FindParticipant(t.Context(), nil, alice); err != nil || got.RoomID != first.ID {
		t.Errorf("FindParticipant after the refused join = %+v, %v; want the first room", got, err)
	}
	if n := e.find(t, second.ID).Head.ParticipantsCount; n != 0 {
		t.Errorf("participants of the second room = %d, want 0: the refused join wrote nothing", n)
	}
	moved := e.participation(second.ID, alice)
	e.tx(t, func(tx uow.Tx) error {
		if _, err := e.leaveIn(t, tx, p, voice.MovedRoom); err != nil {
			return err
		}
		return e.joinIn(t, tx, moved, voice.RoleListener, true)
	})
	if got, err := e.h.Voice.FindParticipant(t.Context(), nil, alice); err != nil || got.ParticipationID != moved.ID {
		t.Errorf("FindParticipant after moving = %+v, %v; want the second room", got, err)
	}
	if a, b := e.find(t, first.ID).Head.ParticipantsCount, e.find(t, second.ID).Head.ParticipantsCount; a != 0 || b != 1 {
		t.Errorf("participants after moving = %d, %d; want 0, 1", a, b)
	}
}

func voiceInvitations(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	host, alice := NamedID("voice-host"), NamedID("voice-alice")
	room := e.openRoom(t, host, voiceSettings("Invited"), true)
	other := e.openRoom(t, NamedID("voice-other-host"), voiceSettings("Other"), true)
	if _, err := e.h.Voice.FindInvitation(ctx, nil, room.ID, alice); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("FindInvitation before inviting = %v, want ErrNotFound", err)
	}
	inv := appvoice.Invitation{
		ID: e.ids.NewID(), RoomID: room.ID, InviteeAccountID: alice, Role: voice.RoleSpeaker, InvitedByAccountID: host, InvitedAt: e.now(),
	}
	var first, again bool
	e.tx(t, func(tx uow.Tx) error {
		var err error
		if first, err = e.h.Voice.InsertInvitation(ctx, tx, inv); err != nil {
			return err
		}
		dup := inv
		dup.ID, dup.Role = e.ids.NewID(), voice.RoleListener
		again, err = e.h.Voice.InsertInvitation(ctx, tx, dup)
		return err
	})
	if !first || again {
		t.Errorf("InsertInvitation = %v, then %v; want true, then false", first, again)
	}
	if got, err := e.h.Voice.FindInvitation(ctx, nil, room.ID, alice); err != nil || !reflect.DeepEqual(got, appvoice.InvitationRecord{Invitation: inv}) {
		t.Errorf("FindInvitation = %+v, %v; want %+v", got, err, inv)
	}
	rev := appvoice.InvitationRevocation{InvitationID: inv.ID, RevokedByAccountID: host, RevokedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		var err error
		if first, err = e.h.Voice.InsertInvitationRevocation(ctx, tx, rev); err != nil {
			return err
		}
		again, err = e.h.Voice.InsertInvitationRevocation(ctx, tx, rev)
		return err
	})
	if !first || again {
		t.Errorf("InsertInvitationRevocation = %v, then %v; want true, then false", first, again)
	}
	if got, err := e.h.Voice.FindInvitation(ctx, nil, room.ID, alice); err != nil || !got.Revoked {
		t.Errorf("FindInvitation after the revocation = %+v, %v; want revoked", got, err)
	}

	banned := func(roomID string) bool {
		t.Helper()
		b, err := e.h.Voice.Banned(ctx, nil, roomID, alice)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if banned(room.ID) {
		t.Error("banned before the ban")
	}
	ban := appvoice.Ban{ID: e.ids.NewID(), RoomID: room.ID, AccountID: alice, BannedByAccountID: host, BannedAt: e.now()}
	e.tx(t, func(tx uow.Tx) error {
		var err error
		if first, err = e.h.Voice.InsertBan(ctx, tx, ban); err != nil {
			return err
		}
		ban.ID = e.ids.NewID()
		again, err = e.h.Voice.InsertBan(ctx, tx, ban)
		return err
	})
	if !first || again {
		t.Errorf("InsertBan = %v, then %v; want true, then false", first, again)
	}
	if !banned(room.ID) || banned(other.ID) {
		t.Errorf("banned = %v in the room, %v in the other; want true, false", banned(room.ID), banned(other.ID))
	}

	granted := func() bool {
		t.Helper()
		g, err := e.h.Voice.SpeakGranted(ctx, nil, room.ID, alice)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	grant := func(change voice.SpeakGrantChange) {
		t.Helper()
		g := appvoice.SpeakGrant{ID: e.ids.NewID(), RoomID: room.ID, AccountID: alice, Change: change, ChangedByAccountID: host, ChangedAt: e.now()}
		e.tx(t, func(tx uow.Tx) error { return e.h.Voice.InsertSpeakGrant(ctx, tx, g) })
	}
	if granted() {
		t.Error("granted before any grant")
	}
	grant(voice.SpeakGranted)
	if !granted() {
		t.Error("not granted after the grant")
	}
	grant(voice.SpeakRevoked)
	if granted() {
		t.Error("granted after the revocation")
	}
	grant(voice.SpeakGranted)
	if !granted() {
		t.Error("not granted after a new grant")
	}
}

func voiceEnd(t *testing.T, e *voiceEnv) {
	host, alice := NamedID("voice-host"), NamedID("voice-alice")
	room := e.openRoom(t, host, voiceSettings("Ending", "go"), true)
	kept := e.openRoom(t, NamedID("voice-other-host"), voiceSettings("Kept", "go"), true)
	ph := e.join(t, room.ID, host, voice.RoleSpeaker)
	pa := e.join(t, room.ID, alice, voice.RoleListener)
	ending, ok := e.end(t, room.ID, voice.EndModerator, NamedID("voice-moderator"), ph, pa)
	if !ok {
		t.Fatal("the ending was not recorded")
	}
	if _, again := e.end(t, room.ID, voice.EndEmpty, ""); again {
		t.Error("a second ending was recorded")
	}
	got := e.find(t, room.ID)
	if got.Ending == nil || !reflect.DeepEqual(*got.Ending, ending) {
		t.Errorf("Ending = %+v, want %+v", got.Ending, ending)
	}
	if got.Head.State != voice.StateEnded || got.Head.ParticipantsCount != 0 {
		t.Errorf("head = %+v, want ended with no participants", got.Head)
	}
	if ps := e.participants(t, room.ID); len(ps) != 0 {
		t.Errorf("Participants of the ended room = %+v", ps)
	}
	if ids := e.openRoomIDs(t, "taggo"); !reflect.DeepEqual(ids, []string{kept.ID}) {
		t.Errorf("OpenRooms(taggo) = %v, want only the open room", ids)
	}
	// An automatic end has no account.
	auto, ok := e.end(t, kept.ID, voice.EndEmpty, "")
	if got := e.find(t, kept.ID); !ok || got.Ending == nil || !reflect.DeepEqual(*got.Ending, auto) {
		t.Errorf("Ending of the automatic end = %+v, want %+v", got.Ending, auto)
	}
}

func voiceGrace(t *testing.T, e *voiceEnv) {
	ctx := t.Context()
	room := e.openRoom(t, NamedID("voice-host"), voiceSettings("Grace"), true)
	if _, err := e.h.Voice.FindHostGrace(ctx, nil, room.ID); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("FindHostGrace before the grace = %v, want ErrNotFound", err)
	}
	for _, d := range []time.Duration{2 * time.Minute, 3 * time.Minute} {
		g := appvoice.HostGrace{RoomID: room.ID, EndsAt: e.now().Add(d)}
		e.tx(t, func(tx uow.Tx) error { return e.h.Voice.PutHostGrace(ctx, tx, g) })
		if got, err := e.h.Voice.FindHostGrace(ctx, nil, room.ID); err != nil || !got.EndsAt.Equal(g.EndsAt) || got.RoomID != room.ID {
			t.Errorf("FindHostGrace = %+v, %v; want %+v", got, err, g)
		}
	}
	for i, want := range []bool{true, false} {
		var deleted bool
		e.tx(t, func(tx uow.Tx) error {
			var err error
			deleted, err = e.h.Voice.DeleteHostGrace(ctx, tx, room.ID)
			return err
		})
		if deleted != want {
			t.Errorf("DeleteHostGrace #%d = %v, want %v", i+1, deleted, want)
		}
	}
	if _, err := e.h.Voice.FindHostGrace(ctx, nil, room.ID); !errors.Is(err, appvoice.ErrNotFound) {
		t.Errorf("FindHostGrace after the delete = %v, want ErrNotFound", err)
	}
}

// voiceSnapshot is what the projections show of rooms: each room with its participants, and the
// room of each account.
type voiceSnapshot struct {
	Rooms        map[string]appvoice.RoomRecord
	Participants map[string][]appvoice.Participant
	Open         []string
	Accounts     map[string]string
}

func (e *voiceEnv) snapshot(t *testing.T, rooms []string, accounts []string) voiceSnapshot {
	t.Helper()
	s := voiceSnapshot{
		Rooms: map[string]appvoice.RoomRecord{}, Participants: map[string][]appvoice.Participant{},
		Open: e.openRoomIDs(t, ""), Accounts: map[string]string{},
	}
	for _, id := range rooms {
		s.Rooms[id] = e.find(t, id)
		s.Participants[id] = e.participants(t, id)
	}
	for _, a := range accounts {
		p, err := e.h.Voice.FindParticipant(t.Context(), nil, a)
		switch {
		case errors.Is(err, appvoice.ErrNotFound):
		case err != nil:
			t.Fatal(err)
		default:
			s.Accounts[a] = p.ParticipationID
		}
	}
	return s
}

func voiceRebuild(t *testing.T, e *voiceEnv) {
	host, next, alice, bob, carol := NamedID("voice-host"), NamedID("voice-next"), NamedID("voice-alice"), NamedID("voice-bob"), NamedID("voice-carol")
	accounts := []string{host, next, alice, bob, carol}

	// An open room, revised, handed over, with a speaker, a co-host who raised a hand, and one
	// who left.
	room := e.openRoom(t, host, voiceSettings("Open", "go"), true)
	e.revise(t, room.ID, host, voiceSettings("Revised", "music", "art"))
	ph := e.join(t, room.ID, host, voice.RoleSpeaker)
	pn := e.join(t, room.ID, next, voice.RoleListener)
	pa := e.join(t, room.ID, alice, voice.RoleListener)
	e.cohost(t, room.ID, next, voice.CohostAssigned)
	e.cohost(t, room.ID, alice, voice.CohostAssigned)
	e.cohost(t, room.ID, alice, voice.CohostRemoved)
	e.hand(t, pa, voice.HandRaised)
	e.hand(t, pn, voice.HandRaised)
	e.hand(t, pn, voice.HandLowered)
	e.role(t, pn, voice.RoleSpeaker, next)
	e.leave(t, ph, voice.LeftRoom)
	e.changeHost(t, room.ID, next, voice.HostLeft)
	// An ended room.
	ended := e.openRoom(t, carol, voiceSettings("Ended"), true)
	pc := e.join(t, ended.ID, carol, voice.RoleSpeaker)
	e.end(t, ended.ID, voice.EndHost, carol, pc)
	// A room that bob is in.
	other := e.openRoom(t, bob, voiceSettings("Other", "go"), true)
	e.join(t, other.ID, bob, voice.RoleSpeaker)

	rooms := []string{room.ID, ended.ID, other.ID}
	before := e.snapshot(t, rooms, accounts)
	if n := len(before.Participants[room.ID]); n != 2 {
		t.Fatalf("participants of the open room = %d, want 2", n)
	}
	if err := appvoice.RebuildProjections(t.Context(), e.h.UnitOfWork, e.h.Voice); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if after := e.snapshot(t, rooms, accounts); !reflect.DeepEqual(after, before) {
		t.Errorf("after the rebuild:\n%+v\nwant\n%+v", after, before)
	}

	// Events written without their projectors appear only after a rebuild.
	unseen := e.openRoom(t, carol, voiceSettings("Unseen", "go"), false)
	pu := e.participation(unseen.ID, carol)
	e.tx(t, func(tx uow.Tx) error { return e.joinIn(t, tx, pu, voice.RoleSpeaker, false) })
	if _, err := e.h.Voice.FindRoom(t.Context(), nil, unseen.ID); !errors.Is(err, appvoice.ErrNotFound) {
		t.Fatalf("FindRoom of a room no projector saw = %v, want ErrNotFound", err)
	}
	if err := appvoice.RebuildProjections(t.Context(), e.h.UnitOfWork, e.h.Voice); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	got := e.find(t, unseen.ID)
	if got.Head.State != voice.StateOpen || got.Head.ParticipantsCount != 1 || got.Head.HostAccountID != carol || got.Revision.Settings.Name != "Unseen" {
		t.Errorf("the rebuilt room = %+v", got)
	}
	want := []appvoice.Participant{{AccountID: carol, RoomID: unseen.ID, ParticipationID: pu.ID, Role: voice.RoleSpeaker}}
	if ps := e.participants(t, unseen.ID); !reflect.DeepEqual(ps, want) {
		t.Errorf("rebuilt participants = %+v, want %+v", ps, want)
	}
	if after := e.snapshot(t, rooms, []string{host, next, alice, bob}); !reflect.DeepEqual(after.Rooms, before.Rooms) ||
		!reflect.DeepEqual(after.Participants, before.Participants) {
		t.Errorf("the other rooms changed in the second rebuild:\n%+v\nwant\n%+v", after, before)
	}
}

func voiceCohosts(t *testing.T, e *voiceEnv) {
	host, alice, bob := NamedID("voice-host"), NamedID("voice-alice"), NamedID("voice-bob")
	room := e.openRoom(t, host, voiceSettings("Cohosts"), true)
	ph := e.join(t, room.ID, host, voice.RoleSpeaker)
	pa := e.join(t, room.ID, alice, voice.RoleSpeaker)
	pb := e.join(t, room.ID, bob, voice.RoleListener)
	e.cohost(t, room.ID, alice, voice.CohostAssigned)
	bobAssigned := e.cohost(t, room.ID, bob, voice.CohostAssigned)

	// A co-host who becomes the host is no longer a co-host.
	e.changeHost(t, room.ID, alice, voice.HostHandedOver)
	want := []appvoice.Participant{
		{AccountID: host, RoomID: room.ID, ParticipationID: ph.ID, Role: voice.RoleSpeaker},
		{AccountID: alice, RoomID: room.ID, ParticipationID: pa.ID, Role: voice.RoleSpeaker},
		{AccountID: bob, RoomID: room.ID, ParticipationID: pb.ID, Role: voice.RoleListener, CohostChangeID: bobAssigned.ID},
	}
	if got := e.participants(t, room.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("Participants after the handover =\n%+v\nwant\n%+v", got, want)
	}

	// A co-host who leaves and joins again is a listener until nominated again.
	e.leave(t, pb, voice.LeftRoom)
	again := e.join(t, room.ID, bob, voice.RoleListener)
	want[2] = appvoice.Participant{AccountID: bob, RoomID: room.ID, ParticipationID: again.ID, Role: voice.RoleListener}
	if got := e.participants(t, room.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("Participants after bob joined again =\n%+v\nwant\n%+v", got, want)
	}
	before := e.participants(t, room.ID)
	if err := appvoice.RebuildProjections(t.Context(), e.h.UnitOfWork, e.h.Voice); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if got := e.participants(t, room.ID); !reflect.DeepEqual(got, before) {
		t.Errorf("Participants after the rebuild =\n%+v\nwant\n%+v", got, before)
	}

	renominated := e.cohost(t, room.ID, bob, voice.CohostAssigned)
	if got, err := e.h.Voice.FindParticipant(t.Context(), nil, bob); err != nil || got.CohostChangeID != renominated.ID {
		t.Errorf("FindParticipant(bob) after the new nomination = %+v, %v; want co-host %s", got, err, renominated.ID)
	}
	// The former host, nominated after the handover, is a co-host again.
	hostAssigned := e.cohost(t, room.ID, host, voice.CohostAssigned)
	if got, err := e.h.Voice.FindParticipant(t.Context(), nil, host); err != nil || got.CohostChangeID != hostAssigned.ID {
		t.Errorf("FindParticipant(former host) = %+v, %v; want co-host %s", got, err, hostAssigned.ID)
	}
}

func voiceEndedParticipants(t *testing.T, e *voiceEnv) {
	alice := NamedID("voice-alice")
	room := e.openRoom(t, NamedID("voice-host"), voiceSettings("Ended"), true)
	e.join(t, room.ID, alice, voice.RoleListener)
	// The ending is recorded without ending the participation first.
	if _, ok := e.end(t, room.ID, voice.EndModerator, NamedID("voice-moderator")); !ok {
		t.Fatal("the ending was not recorded")
	}
	check := func(when string) {
		t.Helper()
		if ps := e.participants(t, room.ID); len(ps) != 0 {
			t.Errorf("Participants of the ended room %s = %+v, want none", when, ps)
		}
		if _, err := e.h.Voice.FindParticipant(t.Context(), nil, alice); !errors.Is(err, appvoice.ErrNotFound) {
			t.Errorf("FindParticipant(alice) %s = %v, want ErrNotFound", when, err)
		}
	}
	check("after the end")
	if err := appvoice.RebuildProjections(t.Context(), e.h.UnitOfWork, e.h.Voice); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	check("after the rebuild")
	other := e.openRoom(t, NamedID("voice-other-host"), voiceSettings("Other"), true)
	if p := e.join(t, other.ID, alice, voice.RoleListener); e.participants(t, other.ID)[0].ParticipationID != p.ID {
		t.Error("alice did not join another room after the end")
	}
}
