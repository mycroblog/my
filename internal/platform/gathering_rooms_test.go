// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/domain/instance"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// newVoiceEventEnv is newEventEnv with the voice chat on (p2p, at most one open room), every
// local user allowed to host rooms, and no event yet.
func newVoiceEventEnv(t *testing.T, users ...string) *eventEnv {
	t.Helper()
	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	clock := testkit.NewClock(start)
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "db", "MY_VOICE": "p2p", "MY_VOICE_MAX_OPEN_ROOMS": "1",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Clock: clock})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if _, err := app.Instance.SetSetting(t.Context(), appinstance.SetSettingInput{Key: instance.SettingVoiceRoomStarters, Value: "everyone"}); err != nil {
		t.Fatal(err)
	}
	e := &eventEnv{app: app, clock: clock, start: start, ids: map[string]string{}}
	for _, name := range append([]string{"alice"}, users...) {
		reg, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple", DisplayName: name})
		if err != nil {
			t.Fatal(err)
		}
		e.ids[name] = reg.AccountID
	}
	return e
}

// createVoiceEvent creates an event of organizer with a voice place that starts at after the
// start of the clock.
func (e *eventEnv) createVoiceEvent(t *testing.T, organizer string, vis gathering.Visibility, after time.Duration) appgathering.View {
	t.Helper()
	v, err := e.app.Events.Create(t.Context(), e.ids[organizer], appgathering.CreateInput{
		Settings: gathering.Settings{
			Title: "Voice night", TimeZone: "Asia/Tokyo", Visibility: vis,
			Places: []gathering.Place{{Kind: gathering.PlaceVoice, Speakers: gathering.SpeakersEveryone}},
		},
		Schedule: gathering.Schedule{StartsAt: e.start.Add(after), EndsAt: e.start.Add(after + 2*time.Hour)},
	})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	return v
}

// TestEventVoiceRoomOpensAtTheStart checks ADR-0161 §10 and ADR-0100 §16 through the app: the
// start opens the room of the event with the organizer as its host, the event links it, an
// attendee who answered going still sees the room after the host's followers no longer include
// it, and cancelling the event ends the room.
func TestEventVoiceRoomOpensAtTheStart(t *testing.T) {
	t.Parallel()
	e := newVoiceEventEnv(t, "bob")
	ctx := t.Context()
	if _, err := e.app.Graph.Follow(ctx, e.ids["bob"], e.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	e.event = e.createVoiceEvent(t, "alice", gathering.VisibilityFollowers, 3*time.Hour)
	e.answer(t, "bob", gathering.AnswerGoing)
	e.drain(t)
	e.at(t, 3*time.Hour+time.Minute)

	v, err := e.app.Events.Get(ctx, e.ids["bob"], e.event.ID)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if v.Room == nil || v.Room.RoomID == "" {
		t.Fatalf("Room = %+v, want the room that opened", v.Room)
	}
	room, err := e.app.VoiceRooms.Get(ctx, e.ids["alice"], v.Room.RoomID)
	if err != nil {
		t.Fatalf("VoiceRooms.Get = %v", err)
	}
	if room.Head.HostAccountID != e.ids["alice"] || room.Head.State != voice.StateOpen || room.Revision.Settings.Visibility != voice.VisibilityFollowers {
		t.Errorf("room = %+v, %+v; want alice's open room for her followers", room.Head, room.Revision.Settings)
	}
	// bob stops following alice: the invitation that the event copied keeps the room in sight.
	if _, err := e.app.Graph.Unfollow(ctx, e.ids["bob"], e.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.VoiceRooms.Get(ctx, e.ids["bob"], v.Room.RoomID); err != nil {
		t.Errorf("VoiceRooms.Get(bob) = %v, want the room he was invited to", err)
	}
	// No start post and no voice_room_started: event_started is the notification of the start.
	wantNotices(t, "bob", e.notices(t, "bob"), eventNotice{domainnotification.TypeEventStarted, domainnotification.EventStartedKey(e.event.Confirmation.ID)})

	if _, err := e.app.Events.Cancel(ctx, e.ids["alice"], e.event.ID, ""); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	e.drain(t)
	ended, err := e.app.VoiceRooms.Get(ctx, e.ids["alice"], v.Room.RoomID)
	if err == nil && ended.Head.State == voice.StateOpen {
		t.Errorf("after the cancellation: room %+v, want it ended", ended.Head)
	}
	if err != nil && !errors.Is(err, appvoice.ErrNotFound) {
		t.Fatalf("VoiceRooms.Get = %v", err)
	}
}

// TestEventVoiceRoomThatCannotOpen checks ADR-0161 §10 through the app: when the room cannot open
// (as many rooms are open as the server allows), the event goes on, the organizer gets
// event_room_unavailable, the attendees get event_started, and the event shows why.
func TestEventVoiceRoomThatCannotOpen(t *testing.T) {
	t.Parallel()
	e := newVoiceEventEnv(t, "bob", "carol")
	ctx := t.Context()
	first := e.createVoiceEvent(t, "alice", gathering.VisibilityPublic, 3*time.Hour)
	e.event = e.createVoiceEvent(t, "bob", gathering.VisibilityPublic, 3*time.Hour+10*time.Minute)
	e.answer(t, "carol", gathering.AnswerGoing)
	e.drain(t)
	e.at(t, 3*time.Hour+time.Minute)
	e.at(t, 3*time.Hour+11*time.Minute)

	if v, err := e.app.Events.Get(ctx, e.ids["alice"], first.ID); err != nil || v.Room == nil || v.Room.RoomID == "" {
		t.Fatalf("the first event: %+v, %v; want its room", v.Room, err)
	}
	v, err := e.app.Events.Get(ctx, e.ids["carol"], e.event.ID)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if v.Cancellation != nil || v.Room == nil || v.Room.RoomID != "" || v.Room.Unavailable != gathering.RoomTooManyRooms {
		t.Errorf("event = %+v, room %+v; want the event with too_many_rooms", v.Cancellation, v.Room)
	}
	conf := e.event.Confirmation.ID
	wantNotices(t, "bob", e.notices(t, "bob"), eventNotice{
		domainnotification.TypeEventRoomUnavailable,
		domainnotification.EventRoomUnavailableKey(conf, string(gathering.RoomTooManyRooms), string(gathering.CheckpointStart)),
	})
	wantNotices(t, "carol", e.notices(t, "carol"),
		eventNotice{domainnotification.TypeEventReminder, domainnotification.EventReminderKey(conf, 60)},
		eventNotice{domainnotification.TypeEventStarted, domainnotification.EventStartedKey(conf)},
	)
}
