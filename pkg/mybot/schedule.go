// SPDX-License-Identifier: MIT

package mybot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed cron expression of five fields, minute hour day-of-month month day-of-week, in a time zone
// (ADR-0094 §5). Each field is *, a number, a range a-b, a step */n or a-b/n, or a list of those separated by
// commas; the day of the week is 0-7, where 0 and 7 are Sunday. When both the day of the month and the day of the
// week are other than *, a day matches when either does, as in cron. A field of seconds, the aliases such as
// @daily, names of months and days, and the other extensions of some crons (?, L, W, #) are refused.
//
// The times of a Schedule are wall-clock times of its time zone: one that a daylight saving time change skips
// does not happen, and one that the change repeats happens once, at its first occurrence.
type Schedule struct {
	expr    string
	loc     *time.Location
	minutes []int // ascending
	hours   []int // ascending
	dom     [32]bool
	month   [13]bool
	dow     [7]bool
	// domStar and dowStar report a field that is exactly *.
	domStar, dowStar bool
}

// scheduleSearchDays is how far Next and Prev look: eight years hold every day of the month and of the week in
// every combination, the 29th of February too.
const scheduleSearchDays = 8*366 + 1

// ParseSchedule parses the cron expression expr in the IANA time zone tz, such as "Asia/Tokyo"; "" is UTC.
func ParseSchedule(expr, tz string) (*Schedule, error) {
	loc := time.UTC
	if tz != "" {
		if tz == "Local" {
			return nil, errors.New(`mybot: the time zone "Local" depends on the machine: name an IANA time zone`)
		}
		l, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("mybot: the time zone %q: %w", tz, err)
		}
		loc = l
	}
	s := &Schedule{expr: expr, loc: loc}
	if strings.HasPrefix(strings.TrimSpace(expr), "@") {
		return nil, fmt.Errorf("mybot: the cron expression %q: aliases such as @daily are not supported: write the five fields", expr)
	}
	fields := strings.Fields(expr)
	switch {
	case len(fields) == 6:
		return nil, fmt.Errorf("mybot: the cron expression %q: a field of seconds is not supported: write the five fields minute hour day month weekday", expr)
	case len(fields) != 5:
		return nil, fmt.Errorf("mybot: the cron expression %q has %d fields, not the five minute hour day month weekday", expr, len(fields))
	}
	minutes, err := parseField(fields[0], "minute", 0, 59)
	if err != nil {
		return nil, s.fieldError(err)
	}
	hours, err := parseField(fields[1], "hour", 0, 23)
	if err != nil {
		return nil, s.fieldError(err)
	}
	dom, err := parseField(fields[2], "day of the month", 1, 31)
	if err != nil {
		return nil, s.fieldError(err)
	}
	months, err := parseField(fields[3], "month", 1, 12)
	if err != nil {
		return nil, s.fieldError(err)
	}
	dow, err := parseField(fields[4], "day of the week", 0, 7)
	if err != nil {
		return nil, s.fieldError(err)
	}
	for v := range 60 {
		if minutes[v] {
			s.minutes = append(s.minutes, v)
		}
	}
	for v := range 24 {
		if hours[v] {
			s.hours = append(s.hours, v)
		}
	}
	copy(s.dom[:], dom)
	copy(s.month[:], months)
	for v, ok := range dow {
		if ok {
			s.dow[v%7] = true
		}
	}
	s.domStar, s.dowStar = fields[2] == "*", fields[4] == "*"
	if !s.possible() {
		return nil, fmt.Errorf("mybot: the cron expression %q names no day that exists", expr)
	}
	return s, nil
}

func (s *Schedule) fieldError(err error) error {
	return fmt.Errorf("mybot: the cron expression %q: %w", s.expr, err)
}

// daysIn are the most days of each month, February in a leap year.
var daysIn = [13]int{0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// possible reports whether some day matches: false for "0 0 30 2 *".
func (s *Schedule) possible() bool {
	if !s.domStar && !s.dowStar {
		return true // either field matches, and every day of the week exists
	}
	for m := 1; m <= 12; m++ {
		if !s.month[m] {
			continue
		}
		for d := 1; d <= daysIn[m]; d++ {
			if s.dom[d] {
				return true
			}
		}
	}
	return false
}

// parseField parses one field into the set of its values, indexed from 0 up to max.
func parseField(field, name string, lo, hi int) ([]bool, error) {
	set := make([]bool, hi+1)
	for part := range strings.SplitSeq(field, ",") {
		rng, stepText, hasStep := strings.Cut(part, "/")
		from, to := lo, hi
		switch {
		case rng == "*":
		case strings.Contains(rng, "-"):
			a, b, _ := strings.Cut(rng, "-")
			var err error
			if from, err = parseValue(a, name, lo, hi); err != nil {
				return nil, err
			}
			if to, err = parseValue(b, name, lo, hi); err != nil {
				return nil, err
			}
			if from > to {
				return nil, fmt.Errorf("the %s range %q runs backwards", name, rng)
			}
		default:
			if hasStep {
				return nil, fmt.Errorf("the %s step %q needs * or a range before the /", name, part)
			}
			v, err := parseValue(rng, name, lo, hi)
			if err != nil {
				return nil, err
			}
			from, to = v, v
		}
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepText)
			if err != nil || !isDigits(stepText) || n < 1 || n > hi-lo+1 {
				return nil, fmt.Errorf("the %s step %q is not a number from 1 to %d", name, stepText, hi-lo+1)
			}
			step = n
		}
		for v := from; v <= to; v += step {
			set[v] = true
		}
	}
	return set, nil
}

// parseValue parses a number of a field from lo to hi, digits alone.
func parseValue(text, name string, lo, hi int) (int, error) {
	v, err := strconv.Atoi(text)
	if err != nil || !isDigits(text) || v < lo || v > hi {
		return 0, fmt.Errorf("the %s %q is not a number from %d to %d", name, text, lo, hi)
	}
	return v, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String is the expression of the schedule.
func (s *Schedule) String() string { return s.expr }

// Location is the time zone of the schedule.
func (s *Schedule) Location() *time.Location { return s.loc }

// Next is the first time of the schedule after t, in its time zone; the zero time when there is none in the next
// eight years.
func (s *Schedule) Next(t time.Time) time.Time {
	y, m, d := t.In(s.loc).Date()
	// Start a day early: a change of the offset may put a time of the previous day after t.
	for i := -1; i < scheduleSearchDays; i++ {
		yy, mm, dd := time.Date(y, m, d+i, 0, 0, 0, 0, time.UTC).Date()
		if !s.dayMatches(yy, mm, dd) {
			continue
		}
		for _, h := range s.hours {
			for _, mi := range s.minutes {
				if at, ok := s.resolve(yy, mm, dd, h, mi); ok && at.After(t) {
					return at
				}
			}
		}
	}
	return time.Time{}
}

// Prev is the last time of the schedule at or before t, in its time zone; the zero time when there is none in the
// eight years before.
func (s *Schedule) Prev(t time.Time) time.Time {
	y, m, d := t.In(s.loc).Date()
	for i := 1; i > -scheduleSearchDays; i-- {
		yy, mm, dd := time.Date(y, m, d+i, 0, 0, 0, 0, time.UTC).Date()
		if !s.dayMatches(yy, mm, dd) {
			continue
		}
		for hi := len(s.hours) - 1; hi >= 0; hi-- {
			for mi := len(s.minutes) - 1; mi >= 0; mi-- {
				if at, ok := s.resolve(yy, mm, dd, s.hours[hi], s.minutes[mi]); ok && !at.After(t) {
					return at
				}
			}
		}
	}
	return time.Time{}
}

// dayMatches reports whether the date matches the month and the days of the schedule.
func (s *Schedule) dayMatches(y int, m time.Month, d int) bool {
	if !s.month[m] {
		return false
	}
	dom := s.dom[d]
	dow := s.dow[time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Weekday()]
	if s.domStar || s.dowStar {
		return dom && dow
	}
	return dom || dow
}

// resolve is the first instant whose wall-clock time in the schedule's zone is the given one, false for a time
// that a change of the offset skips. The offsets around the time are those of the zone 14 hours before and after
// it, the widest offset there is.
func (s *Schedule) resolve(y int, m time.Month, d, h, mi int) (time.Time, bool) {
	wall := time.Date(y, m, d, h, mi, 0, 0, time.UTC)
	var best time.Time
	for _, probe := range [...]time.Time{wall.Add(-14 * time.Hour), wall, wall.Add(14 * time.Hour)} {
		_, off := probe.In(s.loc).Zone()
		at := wall.Add(-time.Duration(off) * time.Second).In(s.loc)
		ay, am, ad := at.Date()
		if ay != y || am != m || ad != d || at.Hour() != h || at.Minute() != mi {
			continue
		}
		if best.IsZero() || at.Before(best) {
			best = at
		}
	}
	return best, !best.IsZero()
}
