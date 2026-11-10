// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/gathering"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// eventEnv is an app with a clock, its users, and an event of alice's that starts three hours
// after the start of the clock.
type eventEnv struct {
	app   *platform.App
	clock *testkit.Clock
	start time.Time
	ids   map[string]string
	event appgathering.View
}

func newEventEnv(t *testing.T, users ...string) *eventEnv {
	t.Helper()
	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	clock := testkit.NewClock(start)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "db"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Clock: clock})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &eventEnv{app: app, clock: clock, start: start, ids: map[string]string{}}
	for _, name := range append([]string{"alice"}, users...) {
		reg, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple", DisplayName: name})
		if err != nil {
			t.Fatal(err)
		}
		e.ids[name] = reg.AccountID
	}
	e.event, err = app.Events.Create(t.Context(), e.ids["alice"], appgathering.CreateInput{
		Settings: gathering.Settings{Title: "Board game night", TimeZone: "Asia/Tokyo"},
		Schedule: gathering.Schedule{StartsAt: start.Add(3 * time.Hour), EndsAt: start.Add(5 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	return e
}

// at moves the clock to d after the start, runs gathering.tick, and runs the jobs.
func (e *eventEnv) at(t *testing.T, d time.Duration) {
	t.Helper()
	e.clock.Set(e.start.Add(d))
	if err := e.app.Events.Tick(t.Context()); err != nil {
		t.Fatalf("Tick = %v", err)
	}
	e.drain(t)
}

func (e *eventEnv) drain(t *testing.T) {
	t.Helper()
	if _, err := e.app.Runner.Drain(t.Context()); err != nil {
		t.Fatalf("Drain = %v", err)
	}
}

func (e *eventEnv) answer(t *testing.T, name string, a gathering.Answer) {
	t.Helper()
	if _, err := e.app.Events.Answer(t.Context(), e.ids[name], e.event.ID, a); err != nil {
		t.Fatalf("Answer(%s) = %v", name, err)
	}
}

// eventNotice is a notification of an event as the tests compare it.
type eventNotice struct {
	Type domainnotification.Type
	Key  string
}

// notices returns the notifications of the events of name, oldest first.
func (e *eventEnv) notices(t *testing.T, name string) []eventNotice {
	t.Helper()
	page, err := e.app.Notifications.List(t.Context(), e.ids[name], notification.Query{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var out []eventNotice
	for i := len(page) - 1; i >= 0; i-- {
		if n := page[i]; n.Type.Event() {
			if n.GatheringID != e.event.ID {
				t.Errorf("%s: %+v names another event", name, n)
			}
			out = append(out, eventNotice{Type: n.Type, Key: n.DedupeKey})
		}
	}
	return out
}

func wantNotices(t *testing.T, name string, got []eventNotice, want ...eventNotice) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: notifications = %+v, want %+v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: notification %d = %+v, want %+v", name, i, got[i], want[i])
		}
	}
}

// TestEventRemindersFollowTheDate checks ADR-0161 §11 and §12 through the app: the reminders of
// each attendee come at the times it chose before the start, a new date sends the change and the
// reminders and start of the new time, and none of the old time, and running the tick and the
// jobs again notifies nobody twice.
func TestEventRemindersFollowTheDate(t *testing.T) {
	t.Parallel()
	e := newEventEnv(t, "bob", "carol", "dave")
	ctx := t.Context()
	e.answer(t, "bob", gathering.AnswerGoing)
	e.answer(t, "carol", gathering.AnswerMaybe)
	e.answer(t, "dave", gathering.AnswerDeclined)
	if _, err := e.app.Events.SetReminders(ctx, e.ids["carol"], []int{30}); err != nil {
		t.Fatalf("SetReminders = %v", err)
	}
	first := e.event.Confirmation.ID
	e.drain(t)

	// The reminder an hour before, for bob's defaults; carol chose only 30 minutes before.
	e.at(t, 2*time.Hour+time.Minute)
	wantNotices(t, "bob", e.notices(t, "bob"), eventNotice{domainnotification.TypeEventReminder, domainnotification.EventReminderKey(first, 60)})
	wantNotices(t, "carol", e.notices(t, "carol"))

	// A new date, two hours later.
	later := gathering.Schedule{StartsAt: e.start.Add(5 * time.Hour), EndsAt: e.start.Add(6 * time.Hour)}
	v, err := e.app.Events.Update(ctx, e.ids["alice"], e.event.ID, appgathering.Patch{Schedule: &later})
	if err != nil {
		t.Fatalf("Update = %v", err)
	}
	second := v.Confirmation.ID
	e.drain(t)
	changed := eventNotice{domainnotification.TypeEventChanged, domainnotification.EventChangedKey(second)}

	// The old start and the old 30 minutes before pass without a notification.
	e.at(t, 2*time.Hour+31*time.Minute)
	e.at(t, 3*time.Hour+time.Minute)
	// The new hour before, the new 30 minutes before, and the new start.
	e.at(t, 4*time.Hour+time.Minute)
	e.at(t, 4*time.Hour+30*time.Minute)
	e.at(t, 5*time.Hour)
	for range 2 {
		e.at(t, 5*time.Hour+2*time.Minute)
	}
	// The jobs of the new start run again: nobody gets it twice.
	b, err := json.Marshal(map[string]any{
		"kind": "started", "gathering": e.event.ID, "ref": second, "source": e.ids["alice"], "at": later.StartsAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.app.Jobs.Enqueue(ctx, queue.NewJob{Type: appgathering.JobNotify, Payload: b}); err != nil {
		t.Fatal(err)
	}
	e.drain(t)

	wantNotices(t, "bob", e.notices(t, "bob"),
		eventNotice{domainnotification.TypeEventReminder, domainnotification.EventReminderKey(first, 60)},
		changed,
		eventNotice{domainnotification.TypeEventReminder, domainnotification.EventReminderKey(second, 60)},
		eventNotice{domainnotification.TypeEventStarted, domainnotification.EventStartedKey(second)},
	)
	wantNotices(t, "carol", e.notices(t, "carol"),
		changed,
		eventNotice{domainnotification.TypeEventReminder, domainnotification.EventReminderKey(second, 30)},
		eventNotice{domainnotification.TypeEventStarted, domainnotification.EventStartedKey(second)},
	)
	wantNotices(t, "dave", e.notices(t, "dave"))
	wantNotices(t, "alice", e.notices(t, "alice"))
}

// TestEventNotificationsExclusions checks ADR-0161 §11 through the app: an invitation notifies
// the invitee, a cancellation those who answered going or maybe and the invitees, and the
// exclusions of the other notifications apply: a block hides the event, and a mute that hides
// notifications hides them.
func TestEventNotificationsExclusions(t *testing.T) {
	t.Parallel()
	e := newEventEnv(t, "bob", "carol", "dave", "erin")
	ctx := t.Context()
	for _, name := range []string{"bob", "carol", "dave"} {
		e.answer(t, name, gathering.AnswerGoing)
	}
	if _, err := e.app.Graph.Mute(ctx, e.ids["carol"], e.ids["alice"], appgraph.MuteInput{HideNotifications: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Graph.Block(ctx, e.ids["dave"], e.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	inv, err := e.app.Events.Invite(ctx, e.ids["alice"], e.event.ID, e.ids["erin"])
	if err != nil {
		t.Fatalf("Invite = %v", err)
	}
	e.drain(t)
	e.at(t, 2*time.Hour+time.Minute)
	if _, err := e.app.Events.Cancel(ctx, e.ids["alice"], e.event.ID, ""); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	e.drain(t)
	e.at(t, 3*time.Hour+time.Minute)

	conf := e.event.Confirmation.ID
	cancelled := eventNotice{domainnotification.TypeEventCancelled, domainnotification.EventCancelledKey(e.event.ID)}
	wantNotices(t, "bob", e.notices(t, "bob"), eventNotice{domainnotification.TypeEventReminder, domainnotification.EventReminderKey(conf, 60)}, cancelled)
	wantNotices(t, "carol", e.notices(t, "carol"))
	wantNotices(t, "dave", e.notices(t, "dave"))
	wantNotices(t, "erin", e.notices(t, "erin"), eventNotice{domainnotification.TypeEventInvited, domainnotification.EventInvitedKey(inv.ID)}, cancelled)
	wantNotices(t, "alice", e.notices(t, "alice"))
}
