// SPDX-License-Identifier: MIT

package mybot

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // the zones of the fixture, whatever the machine has
)

// cronFixture is testdata/cron.json, which the TypeScript mybot reads too.
type cronFixture struct {
	Next []struct {
		Name, Expr, TZ, From string
		Next                 []string
	}
	Prev []struct {
		Name, Expr, TZ, At, Prev string
	}
	Invalid []struct {
		Expr, Why string
	}
}

func loadCronFixture(t *testing.T) cronFixture {
	t.Helper()
	b, err := os.ReadFile("testdata/cron.json")
	if err != nil {
		t.Fatal(err)
	}
	var f cronFixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestScheduleNext(t *testing.T) {
	for _, c := range loadCronFixture(t).Next {
		t.Run(c.Name, func(t *testing.T) {
			s, err := ParseSchedule(c.Expr, c.TZ)
			if err != nil {
				t.Fatalf("ParseSchedule(%q, %q): %v", c.Expr, c.TZ, err)
			}
			at := mustTime(t, c.From)
			for _, want := range c.Next {
				at = s.Next(at)
				if got := at.UTC().Format(time.RFC3339); got != want {
					t.Fatalf("%q: next %s, want %s", c.Expr, got, want)
				}
			}
		})
	}
}

func TestSchedulePrev(t *testing.T) {
	for _, c := range loadCronFixture(t).Prev {
		t.Run(c.Name, func(t *testing.T) {
			s, err := ParseSchedule(c.Expr, c.TZ)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Prev(mustTime(t, c.At)).UTC().Format(time.RFC3339); got != c.Prev {
				t.Errorf("%q: prev of %s is %s, want %s", c.Expr, c.At, got, c.Prev)
			}
		})
	}
}

func TestScheduleRefuses(t *testing.T) {
	for _, c := range loadCronFixture(t).Invalid {
		t.Run(c.Why, func(t *testing.T) {
			if _, err := ParseSchedule(c.Expr, ""); err == nil {
				t.Errorf("ParseSchedule(%q) took %s", c.Expr, c.Why)
			}
		})
	}
	for _, tz := range []string{"Local", "Mars/Olympus"} {
		if _, err := ParseSchedule("0 0 * * *", tz); err == nil {
			t.Errorf("ParseSchedule took the time zone %q", tz)
		}
	}
	if _, err := ParseSchedule("0 0 * * * *", ""); err == nil || !strings.Contains(err.Error(), "seconds") {
		t.Errorf("a field of seconds: %v, want an error that names it", err)
	}
	if _, err := ParseSchedule("@daily", ""); err == nil || !strings.Contains(err.Error(), "alias") {
		t.Errorf("an alias: %v, want an error that names it", err)
	}
}

// Every time of a schedule is the Prev of itself, and the Prev of the second before it is the time before.
func TestScheduleNextAndPrevAgree(t *testing.T) {
	s, err := ParseSchedule("*/7 1-3 * * *", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	prev := s.Prev(mustTime(t, "2026-10-30T00:00:00Z"))
	at := prev
	for range 300 {
		next := s.Next(at)
		if !next.After(at) {
			t.Fatalf("Next(%s) = %s", at, next)
		}
		if p := s.Prev(next); !p.Equal(next) {
			t.Fatalf("Prev(%s) = %s", next, p)
		}
		if p := s.Prev(next.Add(-time.Second)); !p.Equal(at) {
			t.Fatalf("Prev just before %s = %s, want %s", next, p, at)
		}
		at = next
	}
}
