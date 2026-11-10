// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"errors"
	"slices"
	"time"
)

// The reminders and the due times of an event (ADR-0161 §12): an attendee who answers going or
// maybe is reminded at the times before the start that it chose, one day and one hour before by
// default; gathering.tick handles an event at its next due time, a reminder or the start.

// Bounds of the reminders of an account.
const (
	// MaxReminders is how many reminders an account chooses at most; none is a choice too.
	MaxReminders = 3
	// MinReminderMinutes and MaxReminderMinutes bound how long before the start a reminder is:
	// 5 minutes to 7 days.
	MinReminderMinutes = 5
	MaxReminderMinutes = 7 * 24 * 60
)

// DefaultReminders are the reminders of an account that has not chosen: one day and one hour
// before the start, in minutes.
var DefaultReminders = []int{24 * 60, 60}

// ErrInvalidReminders reports reminders out of their bounds: more than MaxReminders, one outside
// MinReminderMinutes to MaxReminderMinutes, or the same one twice.
var ErrInvalidReminders = errors.New("gathering: invalid reminders")

// NormalizeReminders returns minutes, reminders in minutes before the start, from the earliest
// (the most minutes) to the latest, or ErrInvalidReminders.
func NormalizeReminders(minutes []int) ([]int, error) {
	if len(minutes) > MaxReminders {
		return nil, ErrInvalidReminders
	}
	out := slices.Clone(minutes)
	slices.SortFunc(out, func(a, b int) int { return b - a })
	for i, m := range out {
		if m < MinReminderMinutes || m > MaxReminderMinutes || (i > 0 && out[i-1] == m) {
			return nil, ErrInvalidReminders
		}
	}
	if out == nil {
		out = []int{}
	}
	return out, nil
}

// DueKind is what gathering.tick does at a due time of an event.
type DueKind string

// Due kinds.
const (
	// DueReminder reminds the attendees whose reminders include its minutes.
	DueReminder DueKind = "reminder"
	// DueStart tells the attendees that the event starts.
	DueStart DueKind = "start"
)

// Due is one thing that gathering.tick does for an event, at At.
type Due struct {
	Kind DueKind
	At   time.Time
	// MinutesBefore is how long before the start a DueReminder is.
	MinutesBefore int
}

// Dues returns what an event that starts at start has to do, from the earliest: a reminder for
// each of reminders (minutes before the start, the union of its attendees' choices) and the start.
func Dues(start time.Time, reminders []int) []Due {
	out := make([]Due, 0, len(reminders)+1)
	seen := map[int]bool{}
	for _, m := range reminders {
		if m <= 0 || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, Due{Kind: DueReminder, At: start.Add(-time.Duration(m) * time.Minute), MinutesBefore: m})
	}
	out = append(out, Due{Kind: DueStart, At: start})
	slices.SortStableFunc(out, func(a, b Due) int { return a.At.Compare(b.At) })
	return out
}

// NextDue returns the earliest of dues after after, and false when none is left.
func NextDue(dues []Due, after time.Time) (time.Time, bool) {
	for _, d := range dues {
		if d.At.After(after) {
			return d.At, true
		}
	}
	return time.Time{}, false
}

// DuesBetween returns the dues from since through until, both included, from the earliest: what a
// run of gathering.tick at until does for an event that was due at since.
func DuesBetween(dues []Due, since, until time.Time) []Due {
	var out []Due
	for _, d := range dues {
		if !d.At.Before(since) && !d.At.After(until) {
			out = append(out, d)
		}
	}
	return out
}
