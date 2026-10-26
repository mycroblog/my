// SPDX-License-Identifier: AGPL-3.0-only

package system_test

import (
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/system"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestClockContract(t *testing.T) {
	testkit.ClockContract(t, system.Clock{})
}

func TestClockTracksSystemTime(t *testing.T) {
	// Now may hold over a step back of the system clock of up to a minute.
	before := time.Now().Add(-time.Minute - time.Millisecond)
	now := system.Clock{}.Now()
	after := time.Now()
	if now.Before(before) || now.After(after) {
		t.Errorf("Now() = %v, want between %v and %v", now, before, after)
	}
}

func TestIDGeneratorContract(t *testing.T) {
	ids := testkit.IDGeneratorContract(t, system.IDGenerator{}, system.Clock{})

	// The contract already checks strict ordering; make sure that check covered IDs
	// sharing a millisecond, which is where ordering relies on the sub-millisecond counter.
	sameMilli := 0
	for i := 1; i < len(ids); i++ {
		prev, _ := testkit.UUIDv7Millis(ids[i-1])
		cur, _ := testkit.UUIDv7Millis(ids[i])
		if prev == cur {
			sameMilli++
		}
	}
	if sameMilli == 0 {
		t.Fatal("no two consecutive IDs shared a millisecond; same-millisecond ordering was not exercised")
	}
}

func TestIDGeneratorStampsCurrentTime(t *testing.T) {
	// NewID may hold over a step back of the system clock of up to a minute.
	before := time.Now().Add(-time.Minute - time.Millisecond).UnixMilli()
	ms, ok := testkit.UUIDv7Millis(system.IDGenerator{}.NewID())
	after := time.Now().UnixMilli()
	if !ok {
		t.Fatal("NewID() returned a non-UUID")
	}
	if ms < before || ms > after+1 {
		t.Errorf("ID timestamp = %d, want between %d and %d", ms, before, after+1)
	}
}
