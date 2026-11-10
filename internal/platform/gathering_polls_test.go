// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
)

// TestEventDatePollConfirmation checks ADR-0161 §7, §11, and §12 through the app: confirming a
// candidate of a date poll tells the voters and the invitees with event_schedule_confirmed, and
// nobody else, and the reminders of the attendees follow the confirmed date.
func TestEventDatePollConfirmation(t *testing.T) {
	t.Parallel()
	e := newEventEnv(t, "bob", "carol", "dave")
	ctx := t.Context()
	poll, err := e.app.Events.Create(ctx, e.ids["alice"], appgathering.CreateInput{
		Settings: gathering.Settings{Title: "Picnic", TimeZone: "Asia/Tokyo"},
		Candidates: []gathering.Schedule{
			{StartsAt: e.start.Add(3 * time.Hour), EndsAt: e.start.Add(4 * time.Hour)},
			{StartsAt: e.start.Add(27 * time.Hour), EndsAt: e.start.Add(28 * time.Hour)},
		},
	})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	e.event = poll
	cands, err := e.app.Events.Candidates(ctx, e.ids["bob"], poll.ID)
	if err != nil || len(cands) != 2 {
		t.Fatalf("Candidates = %+v, %v", cands, err)
	}
	if _, err := e.app.Events.Vote(ctx, e.ids["bob"], poll.ID, cands[0].ID, gathering.VoteYes); err != nil {
		t.Fatalf("Vote = %v", err)
	}
	if _, err := e.app.Events.Invite(ctx, e.ids["alice"], poll.ID, e.ids["carol"]); err != nil {
		t.Fatalf("Invite = %v", err)
	}
	v, err := e.app.Events.Confirm(ctx, e.ids["alice"], poll.ID, cands[0].ID)
	if err != nil {
		t.Fatalf("Confirm = %v", err)
	}
	e.drain(t)
	conf := v.Confirmation.ID
	confirmed := eventNotice{domainnotification.TypeEventScheduleConfirmed, domainnotification.EventScheduleConfirmedKey(conf)}
	wantNotices(t, "bob", e.notices(t, "bob"), confirmed)
	wantNotices(t, "dave", e.notices(t, "dave"))
	wantNotices(t, "alice", e.notices(t, "alice"))

	// The reminder an hour before the confirmed start.
	e.answer(t, "bob", gathering.AnswerGoing)
	e.at(t, 2*time.Hour+time.Minute)
	wantNotices(t, "bob", e.notices(t, "bob"), confirmed, eventNotice{domainnotification.TypeEventReminder, domainnotification.EventReminderKey(conf, 60)})
	invited := e.notices(t, "carol")
	if len(invited) != 2 || invited[1] != confirmed {
		t.Errorf("carol: notifications = %+v, want the invitation and the confirmation", invited)
	}
}
