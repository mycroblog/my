// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
)

// closureEmailsReachVerifiedUsersOnce sends the service closure emails (ADR-0088 §2): the
// announcement, the withdrawal, a new announcement, the change of its dates, the reminders a
// week and a day before the closing date, the closure, and the reminder a week before the
// deadline of the export each reach every user with a verified primary address once, whether
// their jobs run again or not, and never a user whose address is unverified. They are
// transactional emails with a text and an HTML body: a complaint does not stop them, and they
// carry no unsubscribe header.
func closureEmailsReachVerifiedUsersOnce(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	alice := e.sessionUser(t)
	r := e.registration(identity.RegistrationOpen, true)
	r.signUp(t, signUpInput("carol"))
	dave := signUpInput("dave")
	r.signUp(t, dave)
	if _, err := r.VerifyEmail(ctx, r.lastToken(t, dave.Email)); err != nil {
		t.Fatalf("VerifyEmail = %v", err)
	}
	if err := e.identity.RecordDeliveryFailure(ctx, appidentity.DeliveryFailure{
		EmailAddressID: e.primaryEmailID(t, alice.User.ID), Kind: identity.EmailComplaint, Source: identity.EmailFailureSES,
		FeedbackKey: "ses:closure-complaint", OccurredAt: e.clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	m := &Mailer{}
	var view identity.ServiceClosureView
	s := e.newIdentityWith(func(d *appidentity.Deps) {
		d.Mailer, d.EmailDelivery = m, true
		d.ClosureView = func(context.Context) (identity.ServiceClosureView, error) { return view, nil }
		d.WebOrigin = "https://web." + contractHost
	})
	s.RegisterClosureEmails(e.projectors, e.mailJobs.Registry, nil)

	event := func(kind instance.ClosureEventKind, previous instance.ClosureState, schedule instance.ClosureSchedule) appinstance.ClosureRecorded {
		return appinstance.ClosureRecorded{
			Event:    instance.ClosureEvent{ID: e.ids.NewID(), Kind: kind, Schedule: schedule, OccurredAt: e.clock.Now()},
			Previous: previous,
		}
	}
	record := func(ev appinstance.ClosureRecorded) {
		t.Helper()
		// The fan-out of an event that runs again sends nothing more.
		for range 2 {
			if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, ev) }); err != nil {
				t.Fatalf("record the %s event: %v", ev.Event.Kind, err)
			}
		}
	}
	remind := func() {
		t.Helper()
		for range 2 {
			if err := s.QueueClosureReminders(ctx); err != nil {
				t.Fatalf("QueueClosureReminders = %v", err)
			}
		}
	}
	wantSent := func(step string, n int) {
		t.Helper()
		if got := len(m.Sent()); got != n {
			t.Fatalf("after %s: sent %d emails, want %d", step, got, n)
		}
	}
	schedule := func(closesIn time.Duration) instance.ClosureSchedule {
		closesAt := e.clock.Now().Add(closesIn)
		return instance.ClosureSchedule{ClosesAt: closesAt, ExportUntil: closesAt.Add(30 * 24 * time.Hour), Message: "Thank you for everything."}
	}
	// announce records an announcement with the closure view that it leads to.
	announce := func(previous instance.ClosureState, sc instance.ClosureSchedule) {
		t.Helper()
		ev := event(instance.ClosureEventAnnounced, previous, sc)
		view = identity.ServiceClosureView{
			Stage: identity.ClosureStageAnnounced, AnnouncementID: ev.Event.ID, ClosesAt: sc.ClosesAt, ExportUntil: sc.ExportUntil,
			Message: sc.Message, AnnouncedAt: ev.Event.OccurredAt,
		}
		record(ev)
	}

	// The announcement, its withdrawal, and a new announcement.
	announce(instance.ClosureNone, schedule(60*24*time.Hour))
	wantSent("the announcement", 2)
	remind()
	wantSent("reminders before their time", 2)
	e.clock.Advance(time.Hour)
	view = identity.ServiceClosureView{}
	record(event(instance.ClosureEventWithdrawn, instance.ClosureAnnounced, instance.ClosureSchedule{}))
	wantSent("the withdrawal", 4)
	e.clock.Advance(time.Hour)
	announce(instance.ClosureNone, schedule(40*24*time.Hour))
	wantSent("the second announcement", 6)

	// The change of its dates.
	e.clock.Advance(time.Hour)
	third := schedule(20 * 24 * time.Hour)
	announce(instance.ClosureAnnounced, third)
	wantSent("the change of the dates", 8)

	// The reminders of the closing date, each once.
	e.clock.Set(third.ClosesAt.Add(-identity.ClosureReminderWeek + time.Hour))
	remind()
	wantSent("the week reminder", 10)
	e.clock.Set(third.ClosesAt.Add(-identity.ClosureReminderDay + time.Hour))
	remind()
	wantSent("the day reminder", 12)

	// The closure and the reminder of the export.
	e.clock.Set(third.ClosesAt.Add(time.Hour))
	view.Stage = identity.ClosureStageClosed
	record(event(instance.ClosureEventClosed, instance.ClosureAnnounced, instance.ClosureSchedule{}))
	wantSent("the closure", 14)
	remind()
	wantSent("reminders before the export's", 14)
	e.clock.Set(third.ExportUntil.Add(-identity.ClosureReminderWeek + time.Hour))
	remind()
	wantSent("the export reminder", 16)
	e.clock.Set(third.ExportUntil.Add(time.Hour))
	remind()
	wantSent("the deadline of the export", 16)

	perUser := map[string][]kernel.MailMessage{}
	for _, msg := range m.Sent() {
		perUser[msg.To] = append(perUser[msg.To], msg)
	}
	if len(perUser) != 2 || len(perUser[aliceAddress]) != 8 || len(perUser[dave.Email]) != 8 {
		t.Fatalf("recipients = %v, want 8 emails each to %s and %s and none to the unverified carol", keys(perUser), aliceAddress, dave.Email)
	}
	subjects := map[string]bool{}
	for _, msg := range perUser[aliceAddress] {
		if msg.Category != kernel.MailTransactional || msg.Text == "" || msg.HTML == "" {
			t.Errorf("email %q: category %v, text %d bytes, HTML %d bytes; want a transactional email with both bodies",
				msg.Subject, msg.Category, len(msg.Text), len(msg.HTML))
		}
		for h := range msg.Headers {
			if strings.HasPrefix(h, "List-Unsubscribe") {
				t.Errorf("email %q has %s, want no unsubscribe header", msg.Subject, h)
			}
		}
		subjects[msg.Subject] = true
	}
	if len(subjects) < 7 {
		t.Errorf("subjects = %v, want a subject per kind of email", subjects)
	}
	if first := perUser[aliceAddress][0]; !strings.Contains(first.Text, "Thank you for everything.") ||
		!strings.Contains(first.Text, "https://web."+contractHost+"/settings/import-export") {
		t.Errorf("the announcement does not carry the message and the export link:\n%s", first.Text)
	}
}

// aliceAddress is the primary address of sessionUser as it was given.
const aliceAddress = "Alice@Example.com"

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
