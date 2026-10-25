// SPDX-License-Identifier: AGPL-3.0-only

package identity

import "time"

// The emails of the service closure (ADR-0088 §2, FR-808, FR-809): every user whose primary
// address is verified hears of the announcement of the closure, of a change of its dates, and
// of its withdrawal; is reminded a week and a day before the closing date; hears of the closure
// itself, with the deadline of the export; and is reminded a week before that deadline. They
// are transactional emails (ADR-0086 §3, §4): missing one loses the user's data, so no
// preference or unsubscribe link stops them and a complaint does not suppress them.

// ClosureEmail is the kind of a service closure email (service_closure_emails.email).
type ClosureEmail string

// Kinds of service closure emails.
const (
	// ClosureEmailAnnounced tells of the announcement of the closure.
	ClosureEmailAnnounced ClosureEmail = "announced"
	// ClosureEmailRescheduled tells of a change of the dates of the announcement.
	ClosureEmailRescheduled ClosureEmail = "rescheduled"
	// ClosureEmailWithdrawn tells of the withdrawal of the announcement.
	ClosureEmailWithdrawn ClosureEmail = "withdrawn"
	// ClosureEmailWeekBefore reminds of the closing date a week before it.
	ClosureEmailWeekBefore ClosureEmail = "week_before"
	// ClosureEmailDayBefore reminds of the closing date a day before it.
	ClosureEmailDayBefore ClosureEmail = "day_before"
	// ClosureEmailClosed tells of the closure, with the deadline of the export.
	ClosureEmailClosed ClosureEmail = "closed"
	// ClosureEmailExportWeekBefore reminds of the deadline of the export a week before it.
	ClosureEmailExportWeekBefore ClosureEmail = "export_week_before"
)

// ClosureEmails returns every kind, in the order of the schema's CHECK.
func ClosureEmails() []ClosureEmail {
	return []ClosureEmail{
		ClosureEmailAnnounced, ClosureEmailRescheduled, ClosureEmailWithdrawn, ClosureEmailWeekBefore,
		ClosureEmailDayBefore, ClosureEmailClosed, ClosureEmailExportWeekBefore,
	}
}

// Valid reports whether e is a known kind.
func (e ClosureEmail) Valid() bool {
	for _, k := range ClosureEmails() {
		if e == k {
			return true
		}
	}
	return false
}

// How long before the closing date and the deadline of the export the reminders go out.
const (
	ClosureReminderWeek = 7 * 24 * time.Hour
	ClosureReminderDay  = 24 * time.Hour
)

// ServiceClosureEmailRecord is one row of service_closure_emails: the email of kind Email
// about the closure event ClosureEventID was queued for UserID. The row is the email's reason
// and its idempotency key (ADR-0086 §1): a user gets each email of an event once.
type ServiceClosureEmailRecord struct {
	ID string
	// ClosureEventID is the event that the email tells of: the announcement, the withdrawal, or
	// the closure for the emails of the events, and the announcement in effect for the
	// reminders.
	ClosureEventID string
	Email          ClosureEmail
	UserID         string
	QueuedAt       time.Time
}

// ClosureStage is where the server is in its closure, as its emails read it.
type ClosureStage int

// Stages of the service closure.
const (
	// ClosureStageNone is a server with no closure announced.
	ClosureStageNone ClosureStage = iota
	// ClosureStageAnnounced is a server whose closure is announced.
	ClosureStageAnnounced
	// ClosureStageClosed is a closed server, delivering the Delete of its accounts or done.
	ClosureStageClosed
)

// ServiceClosureView is the current state of the service closure that its emails read. The
// composition root builds it from the instance module.
type ServiceClosureView struct {
	Stage ClosureStage
	// AnnouncementID is the announcement whose schedule is in effect, "" for ClosureStageNone.
	AnnouncementID string
	// ClosesAt, ExportUntil, and Message are the schedule of that announcement.
	ClosesAt, ExportUntil time.Time
	Message               string
	// AnnouncedAt is when that announcement (the latest change of the dates) was made.
	AnnouncedAt time.Time
}

// reminderWindow reports whether now is in [at, end) and at is not before the announcement:
// a reminder whose time had already come when the dates were announced is not sent, since the
// email of the announcement told of them.
func (v ServiceClosureView) reminderWindow(at, end, now time.Time) bool {
	return !at.Before(v.AnnouncedAt) && !now.Before(at) && now.Before(end)
}

// DueReminders returns the reminders whose time has come at now: during the announcement, the
// week reminder from a week before the closing date to a day before it and the day reminder
// from then to the closing date; once closed, the reminder of the export from a week before its
// deadline to the deadline. A reminder that a later one replaces is no longer due, so a server
// that was down sends the latest one only.
func (v ServiceClosureView) DueReminders(now time.Time) []ClosureEmail {
	var out []ClosureEmail
	switch v.Stage {
	case ClosureStageAnnounced:
		week, day := v.ClosesAt.Add(-ClosureReminderWeek), v.ClosesAt.Add(-ClosureReminderDay)
		if v.reminderWindow(week, day, now) {
			out = append(out, ClosureEmailWeekBefore)
		}
		if v.reminderWindow(day, v.ClosesAt, now) {
			out = append(out, ClosureEmailDayBefore)
		}
	case ClosureStageClosed:
		if at := v.ExportUntil.Add(-ClosureReminderWeek); !now.Before(at) && now.Before(v.ExportUntil) {
			out = append(out, ClosureEmailExportWeekBefore)
		}
	}
	return out
}

// EmailDue reports whether the email of kind e about the event eventID is still worth sending
// at now, which its job checks before it sends (ADR-0086 §1): the announcement and the change
// of the dates while that announcement is in effect, the withdrawal while no closure is
// announced again, a reminder of the closing date while it is due for that announcement, and the
// closure and the reminder of the export once closed, the latter before the deadline.
func (v ServiceClosureView) EmailDue(e ClosureEmail, eventID string, now time.Time) bool {
	switch e {
	case ClosureEmailAnnounced, ClosureEmailRescheduled:
		return v.Stage == ClosureStageAnnounced && v.AnnouncementID == eventID
	case ClosureEmailWithdrawn:
		return v.Stage == ClosureStageNone
	case ClosureEmailWeekBefore, ClosureEmailDayBefore:
		if v.AnnouncementID != eventID {
			return false
		}
		for _, r := range v.DueReminders(now) {
			if r == e {
				return true
			}
		}
		return false
	case ClosureEmailClosed:
		return v.Stage == ClosureStageClosed
	case ClosureEmailExportWeekBefore:
		return v.Stage == ClosureStageClosed && now.Before(v.ExportUntil)
	}
	return false
}
