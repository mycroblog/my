// SPDX-License-Identifier: AGPL-3.0-only

package gatheringtest

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/pkg/testkit"
)

// RoomContract verifies the voice chat rooms of the events on the gathering repository of a
// database adapter through appgathering.Service (ADR-0161 §10): the start of a
// date opens its room once, with the settings, the co-host candidates, and the invitees of the
// event, and the event shows it; a room that cannot open records why, tells the organizer once,
// and the event shows the reason; the confirmation and the day before check the host; a room that
// ends unattended tells the organizer; and cancelling the event, or removing its voice place,
// ends its room and keeps the event.
func RoomContract(t *testing.T, newHarness Factory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *roomEnv)
	}{
		{"OpensTheRoomAtTheStart", opensRoom},
		{"RecordsARoomThatCannotOpen", roomFails},
		{"ChecksBeforeTheStart", checksRoom},
		{"TellsOfAnUnattendedRoom", unattendedRoom},
		{"EndsTheRoomAndKeepsTheEvent", endsRoom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newRoomEnv(t, newHarness))
		})
	}
}

// fakeRooms is the fake Rooms of the contract: it opens a room per key, unless reason is set,
// and records what it was asked.
type fakeRooms struct {
	mu      sync.Mutex
	reason  gathering.RoomReason
	opened  map[string]appgathering.RoomOpening
	ids     map[string]string
	ends    []appgathering.RoomEnd
	checked []string
	opens   int
}

func (f *fakeRooms) CheckHost(_ context.Context, accountID string) (gathering.RoomReason, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checked = append(f.checked, accountID)
	return f.reason, nil
}

func (f *fakeRooms) Open(_ context.Context, in appgathering.RoomOpening) (string, gathering.RoomReason, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	if f.reason != "" {
		return "", f.reason, nil
	}
	if id, ok := f.ids[in.Key]; ok {
		return id, "", nil
	}
	id := testkit.NamedID("room-" + in.Key)
	f.opened[in.Key], f.ids[in.Key] = in, id
	return id, "", nil
}

func (f *fakeRooms) End(_ context.Context, in appgathering.RoomEnd) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ends = append(f.ends, in)
	return nil
}

type roomEnv struct {
	*env
	outbox *testkit.RecordingOutbox
	rooms  *fakeRooms
	jobs   *queue.JobHandlerRegistry
	// notices are the event_room_unavailable notifications, as reason:checkpoint.
	notices *recordingNotifier
}

// recordingNotifier records the notifications of the events that the jobs create.
type recordingNotifier struct {
	mu  sync.Mutex
	got []string
}

func (n *recordingNotifier) NotifyEventTx(_ context.Context, _ uow.Tx, ev appnotification.EventNotice, ids []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if ev.Kind == appnotification.EventRoomUnavailable {
		for _, id := range ids {
			n.got = append(n.got, id+":"+ev.Reason+":"+ev.Checkpoint)
		}
	}
	return nil
}

func (n *recordingNotifier) take() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := n.got
	n.got = nil
	return out
}

func newRoomEnv(t *testing.T, newHarness Factory) *roomEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appgathering.RegisterProjectors(projectors, h.Gathering)
	appgathering.RegisterDueProjectors(projectors, h.Gathering)
	clock := testkit.NewClock(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	e := &roomEnv{
		env:    &env{h: h, clock: clock, members: newMembers(), relations: newRelations()},
		outbox: &testkit.RecordingOutbox{}, rooms: &fakeRooms{opened: map[string]appgathering.RoomOpening{}, ids: map[string]string{}},
		jobs: &queue.JobHandlerRegistry{}, notices: &recordingNotifier{},
	}
	e.svc = appgathering.NewService(appgathering.Deps{
		Repository: h.Gathering, UnitOfWork: h.UnitOfWork, Clock: clock, IDs: testkit.NewIDGenerator(clock),
		RateLimiter: kernel.NewRateLimiter(testkit.NewKeyValueStore(clock), clock), Members: e.members, Relations: e.relations,
		Outbox: e.outbox, Notifier: e.notices, Rooms: e.rooms,
	})
	e.svc.RegisterNotificationProjectors(projectors)
	e.svc.RegisterRoomProjectors(projectors)
	e.svc.RegisterNotificationJobs(e.jobs, nil)
	e.svc.RegisterRoomJobs(e.jobs)
	return e
}

// drain runs the jobs that the outbox holds, and those they enqueue, until none is left, and
// returns their types in the order they ran. A job of a key that ran already does not run again.
func (e *roomEnv) drain(t *testing.T) []string {
	t.Helper()
	var ran []string
	seen := map[string]bool{}
	for range 10 {
		jobs := e.outbox.Take()
		if len(jobs) == 0 {
			return ran
		}
		for _, j := range jobs {
			if j.IdempotencyKey != "" && seen[j.IdempotencyKey] {
				continue
			}
			seen[j.IdempotencyKey] = true
			h, _, ok := e.jobs.Lookup(j.Type)
			if !ok {
				t.Fatalf("no handler of %s", j.Type)
			}
			if r := h(t.Context(), queue.Job{Type: j.Type, Payload: j.Payload, IdempotencyKey: j.IdempotencyKey}); !r.IsDone() {
				t.Fatalf("%s = %+v, want done", j.Type, r)
			}
			ran = append(ran, j.Type)
		}
	}
	t.Fatal("the jobs kept enqueueing jobs")
	return nil
}

// at moves the clock to at, runs gathering.tick, and runs the jobs.
func (e *roomEnv) at(t *testing.T, at time.Time) {
	t.Helper()
	e.clock.Set(at)
	if err := e.svc.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.drain(t)
}

// createLater creates an event of alice's that starts in three days, so that the day before comes.
func (e *roomEnv) createLater(t *testing.T, s gathering.Settings) appgathering.View {
	t.Helper()
	start := e.clock.Now().Add(72 * time.Hour).Truncate(time.Minute)
	v, err := e.svc.Create(t.Context(), alice, appgathering.CreateInput{Settings: s, Schedule: gathering.Schedule{StartsAt: start, EndsAt: start.Add(2 * time.Hour)}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return v
}

func (e *roomEnv) get(t *testing.T, viewer, id string) appgathering.View {
	t.Helper()
	v, err := e.svc.Get(t.Context(), viewer, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return v
}

func voiceSettings(vis gathering.Visibility) gathering.Settings {
	return gathering.Settings{
		Title: "Voice night", Visibility: vis,
		Places: []gathering.Place{{Kind: gathering.PlaceVoice, Speakers: gathering.SpeakersEveryone}},
	}
}

func opensRoom(t *testing.T, e *roomEnv) {
	v := e.createLater(t, voiceSettings(gathering.VisibilityPublic))
	if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, frank); err != nil {
		t.Fatalf("AssignCoorganizer: %v", err)
	}
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	e.answer(t, carol, v.ID, gathering.AnswerMaybe)
	e.answer(t, dave, v.ID, gathering.AnswerDeclined)
	if _, err := e.svc.Invite(t.Context(), alice, v.ID, erin); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if got := e.get(t, bob, v.ID).Room; got == nil || *got != (appgathering.RoomState{}) {
		t.Errorf("before the start: Room = %+v, want one that has not opened", got)
	}
	e.drain(t)
	start := v.Confirmation.Schedule.StartsAt
	e.at(t, start.Add(time.Minute))
	key := appgathering.RoomKey(v.ID, v.Confirmation.ID)
	in, ok := e.rooms.opened[key]
	if !ok {
		t.Fatalf("at the start: opened %v, want %s", e.rooms.opened, key)
	}
	if in.HostAccountID != alice || in.Title != "Voice night" || in.Visibility != gathering.VisibilityPublic || in.Speakers != gathering.SpeakersEveryone {
		t.Errorf("opening = %+v, want alice's room with the event's settings", in)
	}
	if !slices.Equal(in.CohostCandidates, []string{frank}) {
		t.Errorf("co-host candidates = %v, want frank", in.CohostCandidates)
	}
	// The co-organizer, going, maybe, and the invitee who has not answered; not declined.
	got := slices.Sorted(slices.Values(in.Invitees))
	want := slices.Sorted(slices.Values([]string{frank, bob, carol, erin}))
	if !slices.Equal(got, want) {
		t.Errorf("invitees = %v, want %v", got, want)
	}
	room := e.get(t, bob, v.ID).Room
	if room == nil || room.RoomID != e.rooms.ids[key] || room.Unavailable != "" {
		t.Errorf("after the start: Room = %+v, want %s", room, e.rooms.ids[key])
	}
	// A job of the start that runs again opens nothing more.
	b, _ := json.Marshal(map[string]string{"gathering": v.ID, "ref": v.Confirmation.ID, "checkpoint": "start"})
	h, _, _ := e.jobs.Lookup(appgathering.JobRoom)
	if r := h(t.Context(), queue.Job{Type: appgathering.JobRoom, Payload: b}); !r.IsDone() || e.rooms.opens != 1 {
		t.Errorf("again: %+v, %d openings; want done and one", r, e.rooms.opens)
	}
	if n := e.notices.take(); len(n) != 0 {
		t.Errorf("notifications = %v, want none", n)
	}
}

func roomFails(t *testing.T, e *roomEnv) {
	v := e.createLater(t, voiceSettings(gathering.VisibilityFollowers))
	e.drain(t)
	start := v.Confirmation.Schedule.StartsAt
	e.at(t, start.Add(-gathering.RoomCheckBefore))
	if n := e.notices.take(); len(n) != 0 {
		t.Errorf("the day before, with a host who may host: notifications = %v, want none", n)
	}
	e.rooms.reason = gathering.RoomTooManyRooms
	e.at(t, start.Add(time.Minute))
	if got, want := e.notices.take(), []string{alice + ":too_many_rooms:start"}; !slices.Equal(got, want) {
		t.Errorf("notifications = %v, want %v", got, want)
	}
	room := e.get(t, alice, v.ID).Room
	if room == nil || room.RoomID != "" || room.Unavailable != gathering.RoomTooManyRooms {
		t.Errorf("Room = %+v, want too_many_rooms", room)
	}
	// The event stays.
	if got := e.get(t, alice, v.ID); got.Cancellation != nil {
		t.Errorf("event = %+v, want it to stay", got.Cancellation)
	}
	e.rooms.reason = ""
	b, _ := json.Marshal(map[string]string{"gathering": v.ID, "ref": v.Confirmation.ID, "checkpoint": "start"})
	h, _, _ := e.jobs.Lookup(appgathering.JobRoom)
	if r := h(t.Context(), queue.Job{Type: appgathering.JobRoom, Payload: b}); !r.IsDone() || e.rooms.opens != 1 {
		t.Errorf("again: %+v, %d openings; want done and one", r, e.rooms.opens)
	}
	e.drain(t)
	if n := e.notices.take(); len(n) != 0 {
		t.Errorf("again: notifications = %v, want none", n)
	}
}

func checksRoom(t *testing.T, e *roomEnv) {
	v := e.createLater(t, voiceSettings(gathering.VisibilityPublic))
	e.drain(t)
	e.rooms.reason = gathering.RoomNotEligible
	start := v.Confirmation.Schedule.StartsAt
	// A new date checks at its confirmation.
	later := gathering.Schedule{StartsAt: start.Add(time.Hour), EndsAt: start.Add(3 * time.Hour)}
	v2, err := e.svc.Update(t.Context(), alice, v.ID, appgathering.Patch{Schedule: &later})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	e.drain(t)
	if got, want := e.notices.take(), []string{alice + ":not_eligible:confirmed"}; !slices.Equal(got, want) {
		t.Errorf("at the confirmation: notifications = %v, want %v", got, want)
	}
	e.rooms.reason = gathering.RoomDisabled
	e.at(t, later.StartsAt.Add(-gathering.RoomCheckBefore))
	if got, want := e.notices.take(), []string{alice + ":disabled:day_before"}; !slices.Equal(got, want) {
		t.Errorf("the day before: notifications = %v, want %v", got, want)
	}
	if v2.Confirmation.ID == v.Confirmation.ID {
		t.Fatal("the new date kept its confirmation")
	}
	// An event without a voice place checks nothing.
	e.rooms.checked = nil
	other := e.createLater(t, gathering.Settings{Title: "Picnic", Visibility: gathering.VisibilityPublic})
	e.drain(t)
	e.at(t, other.Confirmation.Schedule.StartsAt.Add(-gathering.RoomCheckBefore))
	if len(e.rooms.checked) != 0 {
		t.Errorf("without a voice place: checked %v, want none", e.rooms.checked)
	}
}

func unattendedRoom(t *testing.T, e *roomEnv) {
	v := e.createLater(t, voiceSettings(gathering.VisibilityPublic))
	e.drain(t)
	e.at(t, v.Confirmation.Schedule.StartsAt.Add(time.Minute))
	roomID := e.rooms.ids[appgathering.RoomKey(v.ID, v.Confirmation.ID)]
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.svc.RoomEnded(t.Context(), tx, roomID, false); err != nil {
			return err
		}
		if err := e.svc.RoomEnded(t.Context(), tx, testkit.NamedID("another-room"), true); err != nil {
			return err
		}
		return e.svc.RoomEnded(t.Context(), tx, roomID, true)
	})
	if err != nil {
		t.Fatalf("RoomEnded: %v", err)
	}
	e.drain(t)
	if got, want := e.notices.take(), []string{alice + ":unattended:ended"}; !slices.Equal(got, want) {
		t.Errorf("notifications = %v, want %v", got, want)
	}
}

func endsRoom(t *testing.T, e *roomEnv) {
	v := e.createLater(t, voiceSettings(gathering.VisibilityPublic))
	e.drain(t)
	e.at(t, v.Confirmation.Schedule.StartsAt.Add(time.Minute))
	key := appgathering.RoomKey(v.ID, v.Confirmation.ID)
	// Removing the voice place ends the room and keeps the event.
	none := []gathering.Place{}
	if _, err := e.svc.Update(t.Context(), alice, v.ID, appgathering.Patch{Places: &none}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	e.drain(t)
	if want := []appgathering.RoomEnd{{Key: key, ByAccountID: alice}}; !slices.Equal(e.rooms.ends, want) {
		t.Errorf("ends = %+v, want %+v", e.rooms.ends, want)
	}
	got := e.get(t, alice, v.ID)
	if got.Cancellation != nil || got.Room != nil {
		t.Errorf("event = %+v, room %+v; want the event without a room", got.Cancellation, got.Room)
	}

	// A moderator's cancellation ends the room as a moderator.
	w := e.createLater(t, voiceSettings(gathering.VisibilityPublic))
	e.drain(t)
	e.at(t, w.Confirmation.Schedule.StartsAt.Add(time.Minute))
	e.rooms.ends = nil
	if err := e.svc.CancelByModerator(t.Context(), appgathering.ModeratorCancelInput{GatheringID: w.ID, ByAccountID: dave, Note: "spam"}); err != nil {
		t.Fatalf("CancelByModerator: %v", err)
	}
	e.drain(t)
	want := []appgathering.RoomEnd{{Key: appgathering.RoomKey(w.ID, w.Confirmation.ID), Moderator: true, ByAccountID: dave, Note: "spam"}}
	if !slices.Equal(e.rooms.ends, want) {
		t.Errorf("ends = %+v, want %+v", e.rooms.ends, want)
	}

	// An event cancelled before its start opens no room.
	c := e.createLater(t, voiceSettings(gathering.VisibilityPublic))
	e.drain(t)
	if _, err := e.svc.Cancel(t.Context(), alice, c.ID, ""); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	e.drain(t)
	e.at(t, c.Confirmation.Schedule.StartsAt.Add(time.Minute))
	if _, ok := e.rooms.opened[appgathering.RoomKey(c.ID, c.Confirmation.ID)]; ok {
		t.Error("a cancelled event opened its room")
	}
}
