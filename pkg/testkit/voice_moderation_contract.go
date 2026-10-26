// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The cases of VoiceRoomsContract for the moderation of the rooms and the retention of their
// records (ADR-0100 §11, §13).

// roomsRetention is the instance setting retention_voice_participants, the rooms that reports
// point at, and the accounts that holds preserve, as the moderation module answers them.
type roomsRetention struct {
	mu       sync.Mutex
	period   time.Duration
	reported map[string]bool
	held     map[string]bool
}

func newRoomsRetention() *roomsRetention {
	return &roomsRetention{period: 90 * 24 * time.Hour, reported: map[string]bool{}, held: map[string]bool{}}
}

func (r *roomsRetention) set(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn()
}

func (r *roomsRetention) ParticipantRetention(context.Context) (time.Duration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.period, nil
}

func (r *roomsRetention) ReportedVoiceRooms(_ context.Context, ids []string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		if r.reported[id] {
			out[id] = true
		}
	}
	return out, nil
}

func (r *roomsRetention) HeldAccountIDs(context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for id, ok := range r.held {
		if ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// roomsAudits collects the audit log entries that the rooms emit, in their transactions.
type roomsAudits struct {
	mu      sync.Mutex
	entries []appmoderation.AuditEntry
}

func (a *roomsAudits) register(p *uow.Projectors) {
	p.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, e uow.Event) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.entries = append(a.entries, e.(appmoderation.AuditRecorded).Entry)
		return nil
	})
}

func (a *roomsAudits) all() []appmoderation.AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.entries)
}

func (e *roomsEnv) records(t *testing.T, room string) appvoice.RoomRecords {
	t.Helper()
	r, err := e.svc.RoomRecords(t.Context(), room)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func roomsModeration(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, listener, blocked, moderator := e.account(true), e.account(false), e.account(false), e.account(false)
	room := e.open(t, host, appvoice.SettingsInput{Name: "Room"}).Room.ID
	e.join(t, listener, room)
	name := "Renamed"
	v, err := e.svc.Update(ctx, host, room, appvoice.SettingsPatch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}

	// A report points at a room that the reporter sees, its current revision, and its host.
	target, err := e.svc.ReportTarget(ctx, listener, room)
	if err != nil || target.RoomID != room || target.HostAccountID != host || target.RevisionID != v.Head.RevisionID {
		t.Fatalf("ReportTarget = %+v, %v; want the room, its host, and revision %s", target, err, v.Head.RevisionID)
	}
	e.block(t, host, blocked)
	_, err = e.svc.ReportTarget(ctx, blocked, room)
	wantErr(t, "ReportTarget of a room that the reporter does not see", err, appvoice.ErrNotFound)
	_, err = e.svc.ReportTarget(ctx, listener, e.ids.NewID())
	wantErr(t, "ReportTarget of an unknown room", err, appvoice.ErrNotFound)

	// A moderator ends the room with a reason, which the ending keeps, and the audit log
	// records it in the same transaction.
	in := appvoice.ModeratorEndInput{RoomID: room, ByUserID: "user-" + moderator, ByAccountID: moderator, Note: strings.Repeat("a", appvoice.MaxEndNote+1)}
	wantErr(t, "EndByModerator with a long reason", e.svc.EndByModerator(ctx, in), appvoice.ErrInvalidEndNote)
	in.Note = " spam \r\n"
	if err := e.svc.EndByModerator(ctx, in); err != nil {
		t.Fatal(err)
	}
	e.wantEnded(t, room, voice.EndModerator)
	if end := e.room(t, room).Ending; end.EndedByAccountID != moderator || end.Note != "spam" {
		t.Errorf("ending = %+v, want the moderator and the reason", end)
	}
	if len(e.participants(t, room)) != 0 {
		t.Error("the participants stay in a room that a moderator ended")
	}
	audits := e.audits.all()
	if len(audits) != 1 {
		t.Fatalf("audit entries = %+v, want one", audits)
	}
	a := audits[0]
	if a.Action != appmoderation.AuditVoiceRoomEnded || a.TargetKind != appmoderation.AuditTargetVoiceRoom || a.TargetID != room ||
		a.TargetLabel != room || a.SourceEventID != room || a.ActorUserID != in.ByUserID || a.Validate() != nil {
		t.Errorf("audit entry = %+v (%v), want voice_room_ended on the room by the moderator", a, a.Validate())
	}
	wantErr(t, "EndByModerator of an ended room", e.svc.EndByModerator(ctx, in), appvoice.ErrNotFound)
	wantErr(t, "EndByModerator of an unknown room", e.svc.EndByModerator(ctx, appvoice.ModeratorEndInput{RoomID: e.ids.NewID()}), appvoice.ErrNotFound)
	if n := len(e.audits.all()); n != 1 {
		t.Errorf("audit entries = %d after the ends that changed nothing, want 1", n)
	}

	// The moderators read every revision and the participations of the room.
	rec := e.records(t, room)
	if len(rec.Revisions) != 2 || rec.Revisions[0].Settings.Name != "Room" || rec.Revisions[1].Settings.Name != "Renamed" {
		t.Errorf("revisions = %+v, want both, oldest first", rec.Revisions)
	}
	if len(rec.Participations) != 2 || rec.Participations[1].AccountID != listener || rec.Participations[1].End == nil ||
		rec.Participations[1].End.Reason != voice.RoomEnded {
		t.Errorf("participations = %+v, want the host's and the listener's, ended with the room", rec.Participations)
	}
	if rec.Ending == nil || rec.Ending.Note != "spam" {
		t.Errorf("ending = %+v, want the moderator's", rec.Ending)
	}
}

func roomsPurge(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	day := 24 * time.Hour
	// run opens a room of a new host with a listener and a removed account, and ends it.
	run := func() (string, string) {
		host, listener, removed := e.account(true), e.account(false), e.account(false)
		room := e.open(t, host, appvoice.SettingsInput{Name: "Room"}).Room.ID
		e.join(t, listener, room)
		e.join(t, removed, room)
		if err := e.svc.Remove(ctx, host, room, removed); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.End(ctx, host, room); err != nil {
			t.Fatal(err)
		}
		return room, listener
	}
	plain, _ := run()
	reported, _ := run()
	held, heldListener := run()
	e.clock.Advance(60 * day)
	recent, _ := run()
	e.retention.set(func() {
		e.retention.reported[reported] = true
		e.retention.held[heldListener] = true
	})
	kept := func(room string, want bool) {
		t.Helper()
		rec := e.records(t, room)
		if got := len(rec.Participations) > 0 || len(rec.Bans) > 0; got != want {
			t.Errorf("room %s keeps its records = %v (%+v, %+v), want %v", room, got, rec.Participations, rec.Bans, want)
		}
		if len(rec.Revisions) != 1 || rec.Ending == nil {
			t.Errorf("room %s = %+v, want the room, its revision, and its ending kept", room, rec)
		}
	}
	purge := func(want int) {
		t.Helper()
		n, err := e.svc.PurgeRecords(ctx)
		if err != nil || n != want {
			t.Fatalf("PurgeRecords = %d, %v; want %d", n, err, want)
		}
	}

	// Nothing ended 90 days ago yet.
	purge(0)
	e.clock.Advance(31 * day)
	// The plain room ended 91 days ago; the reported one and the one naming a held account
	// stay, as does the room that ended 31 days ago.
	purge(1)
	kept(plain, false)
	kept(reported, true)
	kept(held, true)
	kept(recent, true)
	purge(0)

	// A shorter retention takes the recent room too.
	e.retention.set(func() { e.retention.period = 30 * day })
	purge(1)
	kept(recent, false)

	// Once the report is erased and the hold released, their rooms go.
	e.retention.set(func() {
		delete(e.retention.reported, reported)
		delete(e.retention.held, heldListener)
	})
	purge(2)
	kept(reported, false)
	kept(held, false)
	purge(0)

	// More reported rooms than one run looks at do not starve the room after them: the next
	// run goes on from where the last one stopped, and wraps around at the end.
	var many []string
	last := ""
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		at := e.clock.Now().Add(-100 * day)
		for i := range appvoice.RecordPurgeBatch*recordPurgeRuns + 1 {
			id := e.ids.NewID()
			if err := e.h.Voice.InsertRoom(ctx, tx, appvoice.Room{ID: id, OpenedByAccountID: id, OpenedAt: at, Transport: voice.TransportP2P}); err != nil {
				return err
			}
			rev := appvoice.Revision{ID: e.ids.NewID(), RoomID: id, RevisedByAccountID: id, RevisedAt: at,
				Settings: voice.Settings{Name: "Room", Visibility: voice.VisibilityLocal, Speakers: voice.SpeakersManual}}
			if err := e.h.Voice.InsertRevision(ctx, tx, rev); err != nil {
				return err
			}
			c := appvoice.HostChange{ID: e.ids.NewID(), RoomID: id, HostAccountID: id, Reason: voice.HostOpened, ChangedAt: at}
			if err := e.h.Voice.InsertHostChange(ctx, tx, c); err != nil {
				return err
			}
			p := appvoice.Participation{ID: e.ids.NewID(), RoomID: id, AccountID: id, JoinedAt: at}
			if err := e.h.Voice.InsertParticipation(ctx, tx, p); err != nil {
				return err
			}
			if _, err := e.h.Voice.InsertEnding(ctx, tx, appvoice.Ending{RoomID: id, Reason: voice.EndHost, EndedAt: at}); err != nil {
				return err
			}
			if err := e.h.Voice.RefreshHead(ctx, tx, id); err != nil {
				return err
			}
			if i < appvoice.RecordPurgeBatch*recordPurgeRuns {
				many = append(many, id)
			} else {
				last = id
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e.retention.set(func() {
		for _, id := range many {
			e.retention.reported[id] = true
		}
	})
	purge(0)
	purge(1)
	if ps, err := e.h.Voice.Participations(ctx, nil, last); err != nil || len(ps) != 0 {
		t.Errorf("the room after the reported ones keeps %+v, %v; want its records erased", ps, err)
	}
	purge(0)
}

// recordPurgeRuns is how many batches of rooms one run of voice.purge_records looks at.
const recordPurgeRuns = 20

func roomsErasure(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, other := e.account(true), e.account(true)
	room := e.open(t, host, appvoice.SettingsInput{Name: "Secret", Description: "About me"}).Room.ID
	name := "Renamed"
	if _, err := e.svc.Update(ctx, host, room, appvoice.SettingsPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.End(ctx, host, room); err != nil {
		t.Fatal(err)
	}
	second := e.open(t, host, appvoice.SettingsInput{Name: "Second"}).Room.ID
	if err := e.svc.End(ctx, host, second); err != nil {
		t.Fatal(err)
	}
	others := e.open(t, other, appvoice.SettingsInput{Name: "Other", Description: "Kept"}).Room.ID

	eraser := appvoice.NewEraser(e.h.UnitOfWork, e.h.Voice)
	target := appaccount.ErasureTarget{AccountID: host}
	step, err := eraser.EraseAccountData(ctx, target, "", 1)
	if err != nil || step.Done {
		t.Fatalf("EraseAccountData(1) = %+v, %v; want a batch with more to come", step, err)
	}
	for range 3 {
		if step, err = eraser.EraseAccountData(ctx, target, "", 1); err != nil {
			t.Fatal(err)
		}
		if step.Done {
			break
		}
	}
	if !step.Done {
		t.Fatal("the erasure does not finish")
	}
	for _, id := range []string{room, second} {
		rec := e.records(t, id)
		for _, rv := range rec.Revisions {
			if rv.Settings.Name != "" || rv.Settings.Description != "" {
				t.Errorf("room %s revision %+v, want the name and the description empty", id, rv.Settings)
			}
		}
		if rec.Ending == nil || len(rec.Revisions) == 0 {
			t.Errorf("room %s = %+v, want the room, its revisions, and its ending kept", id, rec)
		}
	}
	if rec := e.records(t, others); rec.Revisions[0].Settings.Name != "Other" || rec.Revisions[0].Settings.Description != "Kept" {
		t.Errorf("the room of another account = %+v, want it untouched", rec.Revisions)
	}
}
