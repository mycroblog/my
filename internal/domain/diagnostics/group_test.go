// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/diagnostics"
)

func TestNext(t *testing.T) {
	cases := []struct {
		from    diagnostics.State
		kind    diagnostics.EventKind
		to      diagnostics.State
		changes bool
	}{
		{diagnostics.StateOpen, diagnostics.EventResolved, diagnostics.StateResolved, true},
		{diagnostics.StateMuted, diagnostics.EventResolved, diagnostics.StateResolved, true},
		{diagnostics.StateResolved, diagnostics.EventResolved, diagnostics.StateResolved, false},
		{diagnostics.StateOpen, diagnostics.EventMuted, diagnostics.StateMuted, true},
		{diagnostics.StateResolved, diagnostics.EventMuted, diagnostics.StateMuted, true},
		{diagnostics.StateMuted, diagnostics.EventMuted, diagnostics.StateMuted, false},
		{diagnostics.StateMuted, diagnostics.EventUnmuted, diagnostics.StateOpen, true},
		{diagnostics.StateOpen, diagnostics.EventUnmuted, diagnostics.StateOpen, false},
		{diagnostics.StateResolved, diagnostics.EventUnmuted, diagnostics.StateResolved, false},
		{diagnostics.StateResolved, diagnostics.EventRegressed, diagnostics.StateOpen, true},
		{diagnostics.StateOpen, diagnostics.EventRegressed, diagnostics.StateOpen, false},
		{diagnostics.StateMuted, diagnostics.EventRegressed, diagnostics.StateMuted, false},
	}
	for _, c := range cases {
		to, changes := diagnostics.Next(c.from, c.kind)
		if to != c.to || changes != c.changes {
			t.Errorf("Next(%s, %s) = %s, %v; want %s, %v", c.from, c.kind, to, changes, c.to, c.changes)
		}
	}
}

func TestRegresses(t *testing.T) {
	if !diagnostics.Regresses(diagnostics.StateResolved, "1.0.0", "1.1.0") {
		t.Error("a resolved group with a new version does not regress")
	}
	if diagnostics.Regresses(diagnostics.StateResolved, "1.0.0", "1.0.0") {
		t.Error("a resolved group with the same version regresses")
	}
	if diagnostics.Regresses(diagnostics.StateOpen, "", "1.1.0") || diagnostics.Regresses(diagnostics.StateMuted, "", "1.1.0") {
		t.Error("an open or muted group regresses")
	}
}

func TestMessageHead(t *testing.T) {
	if got := diagnostics.MessageHead("first\nsecond"); got != "first" {
		t.Errorf("MessageHead = %q", got)
	}
	long := strings.Repeat("あ", diagnostics.MessageHeadRunes+10)
	if got := diagnostics.MessageHead(long); got != strings.Repeat("あ", diagnostics.MessageHeadRunes) {
		t.Errorf("MessageHead kept %d runes", len([]rune(got)))
	}
	if got := diagnostics.MessageHead("a\x00b\xff"); got != "a�b�" {
		t.Errorf("MessageHead = %q", got)
	}
}

func TestLastType(t *testing.T) {
	if got := diagnostics.LastType("*fmt.wrapError > *pgconn.PgError"); got != "*pgconn.PgError" {
		t.Errorf("LastType = %q", got)
	}
	if got := diagnostics.LastType("string"); got != "string" {
		t.Errorf("LastType = %q", got)
	}
}

func TestHour(t *testing.T) {
	at := time.Date(2026, 10, 27, 13, 45, 12, 0, time.FixedZone("JST", 9*3600))
	if got, want := diagnostics.Hour(at), time.Date(2026, 10, 27, 4, 0, 0, 0, time.UTC); !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("Hour = %v, want %v", got, want)
	}
}
