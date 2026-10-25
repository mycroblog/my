// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/identity"
)

func TestClosureEmailDueReminders(t *testing.T) {
	announcedAt := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	closesAt := announcedAt.Add(30 * 24 * time.Hour)
	exportUntil := closesAt.Add(30 * 24 * time.Hour)
	v := identity.ServiceClosureView{
		Stage: identity.ClosureStageAnnounced, AnnouncementID: "a1", ClosesAt: closesAt, ExportUntil: exportUntil, AnnouncedAt: announcedAt,
	}
	closed := v
	closed.Stage = identity.ClosureStageClosed
	short := v
	short.ClosesAt = announcedAt.Add(3 * 24 * time.Hour)

	for _, c := range []struct {
		name string
		v    identity.ServiceClosureView
		now  time.Time
		want []identity.ClosureEmail
	}{
		{"after the announcement", v, announcedAt.Add(time.Hour), nil},
		{"a week before", v, closesAt.Add(-identity.ClosureReminderWeek), []identity.ClosureEmail{identity.ClosureEmailWeekBefore}},
		{"two days before", v, closesAt.Add(-48 * time.Hour), []identity.ClosureEmail{identity.ClosureEmailWeekBefore}},
		{"a day before", v, closesAt.Add(-identity.ClosureReminderDay), []identity.ClosureEmail{identity.ClosureEmailDayBefore}},
		{"on the closing date", v, closesAt, nil},
		{"short notice: no week reminder", short, short.ClosesAt.Add(-48 * time.Hour), nil},
		{"short notice: the day reminder", short, short.ClosesAt.Add(-time.Hour), []identity.ClosureEmail{identity.ClosureEmailDayBefore}},
		{"closed, a month before the deadline", closed, closesAt, nil},
		{"closed, a week before the deadline", closed, exportUntil.Add(-time.Hour), []identity.ClosureEmail{identity.ClosureEmailExportWeekBefore}},
		{"after the deadline", closed, exportUntil, nil},
		{"no closure", identity.ServiceClosureView{}, closesAt.Add(-time.Hour), nil},
	} {
		if got := c.v.DueReminders(c.now); !slices.Equal(got, c.want) {
			t.Errorf("%s: DueReminders = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClosureEmailDue(t *testing.T) {
	announcedAt := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	closesAt := announcedAt.Add(30 * 24 * time.Hour)
	v := identity.ServiceClosureView{
		Stage: identity.ClosureStageAnnounced, AnnouncementID: "a2", ClosesAt: closesAt, ExportUntil: closesAt.Add(30 * 24 * time.Hour), AnnouncedAt: announcedAt,
	}
	closed := v
	closed.Stage = identity.ClosureStageClosed
	week := closesAt.Add(-identity.ClosureReminderWeek)

	for _, c := range []struct {
		name  string
		v     identity.ServiceClosureView
		email identity.ClosureEmail
		event string
		now   time.Time
		want  bool
	}{
		{"the announcement in effect", v, identity.ClosureEmailAnnounced, "a2", announcedAt, true},
		{"an announcement replaced", v, identity.ClosureEmailAnnounced, "a1", announcedAt, false},
		{"the change in effect", v, identity.ClosureEmailRescheduled, "a2", announcedAt, true},
		{"the withdrawal, still withdrawn", identity.ServiceClosureView{}, identity.ClosureEmailWithdrawn, "w1", announcedAt, true},
		{"the withdrawal, announced again", v, identity.ClosureEmailWithdrawn, "w1", announcedAt, false},
		{"the week reminder", v, identity.ClosureEmailWeekBefore, "a2", week, true},
		{"the week reminder of another announcement", v, identity.ClosureEmailWeekBefore, "a1", week, false},
		{"the week reminder once the day one is due", v, identity.ClosureEmailWeekBefore, "a2", closesAt.Add(-time.Hour), false},
		{"the closure", closed, identity.ClosureEmailClosed, "c1", closesAt, true},
		{"the closure before it", v, identity.ClosureEmailClosed, "c1", closesAt, false},
		{"the export reminder", closed, identity.ClosureEmailExportWeekBefore, "a2", closed.ExportUntil.Add(-time.Hour), true},
		{"the export reminder after the deadline", closed, identity.ClosureEmailExportWeekBefore, "a2", closed.ExportUntil, false},
		{"an unknown kind", v, identity.ClosureEmail("other"), "a2", announcedAt, false},
	} {
		if got := c.v.EmailDue(c.email, c.event, c.now); got != c.want {
			t.Errorf("%s: EmailDue = %v, want %v", c.name, got, c.want)
		}
	}
	for _, e := range identity.ClosureEmails() {
		if !e.Valid() {
			t.Errorf("%s is not valid", e)
		}
	}
}
