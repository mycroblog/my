// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestNormalizeReminders(t *testing.T) {
	for _, tc := range []struct {
		in, want []int
		err      error
	}{
		{in: nil, want: []int{}},
		{in: []int{60, 1440}, want: []int{1440, 60}},
		{in: []int{5, MaxReminderMinutes, 30}, want: []int{MaxReminderMinutes, 30, 5}},
		{in: []int{4}, err: ErrInvalidReminders},
		{in: []int{MaxReminderMinutes + 1}, err: ErrInvalidReminders},
		{in: []int{60, 60}, err: ErrInvalidReminders},
		{in: []int{5, 10, 15, 20}, err: ErrInvalidReminders},
	} {
		got, err := NormalizeReminders(tc.in)
		if !errors.Is(err, tc.err) || (tc.err == nil && !slices.Equal(got, tc.want)) {
			t.Errorf("NormalizeReminders(%v) = %v, %v; want %v, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}

func TestDues(t *testing.T) {
	start := time.Date(2026, 12, 5, 12, 0, 0, 0, time.UTC)
	dues := Dues(start, []int{60, 1440, 60, 0})
	want := []Due{
		{Kind: DueReminder, At: start.Add(-24 * time.Hour), MinutesBefore: 1440},
		{Kind: DueReminder, At: start.Add(-time.Hour), MinutesBefore: 60},
		{Kind: DueStart, At: start},
	}
	if !slices.Equal(dues, want) {
		t.Fatalf("Dues = %+v, want %+v", dues, want)
	}
	if next, ok := NextDue(dues, start.Add(-2*time.Hour)); !ok || !next.Equal(start.Add(-time.Hour)) {
		t.Errorf("NextDue two hours before = %v, %v; want the hour before", next, ok)
	}
	if next, ok := NextDue(dues, start.Add(-time.Hour)); !ok || !next.Equal(start) {
		t.Errorf("NextDue at the hour before = %v, %v; want the start", next, ok)
	}
	if _, ok := NextDue(dues, start); ok {
		t.Error("NextDue at the start: want none left")
	}
	// A late run handles everything from the due time through now.
	if got := DuesBetween(dues, start.Add(-24*time.Hour), start.Add(-time.Hour)); len(got) != 2 {
		t.Errorf("DuesBetween = %+v, want both reminders", got)
	}
	if got := DuesBetween(dues, start.Add(-time.Minute), start.Add(time.Minute)); len(got) != 1 || got[0].Kind != DueStart {
		t.Errorf("DuesBetween around the start = %+v, want the start", got)
	}
}
