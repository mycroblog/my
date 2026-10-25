// SPDX-License-Identifier: AGPL-3.0-only

// Package system implements the Clock and IDGenerator ports of internal/app/kernel with
// the operating system's clock and the standard library's UUIDv7 generator.
package system

import (
	"crypto/rand"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Clock reads the system clock.
type Clock struct{}

var _ kernel.Clock = Clock{}

// Now returns the current system time in UTC, truncated to milliseconds. Within the process it
// never goes back by up to maxHold: when the system clock steps back that little (time
// synchronization does, every few seconds on some virtual machines), Now keeps the latest time
// it returned until the system clock passes it, so that the times one process records in a row
// keep their order. A step back beyond maxHold is a correction of a wrong clock, which Now
// follows.
func (Clock) Now() time.Time {
	return processClock.at(time.Now())
}

// maxHold is the longest step back of the system clock that Now holds over.
const maxHold = time.Minute

// processClock is the latest time that Now returned in the process.
var processClock floor

// floor holds the latest time it returned, in Unix milliseconds.
type floor struct{ last atomic.Int64 }

// at returns wall truncated to milliseconds in UTC, or the latest time it returned when wall is
// before it by at most maxHold.
func (f *floor) at(wall time.Time) time.Time {
	t := kernel.Normalize(wall)
	ms := t.UnixMilli()
	for {
		prev := f.last.Load()
		if ms < prev && prev-ms <= maxHold.Milliseconds() {
			return time.UnixMilli(prev).UTC()
		}
		if f.last.CompareAndSwap(prev, ms) {
			return t
		}
	}
}

// IDGenerator issues UUIDv7s with uuid.NewV7, which orders IDs within the same millisecond
// by a sub-millisecond counter, and fills the random bits of IDs for a given time from crypto/rand.
type IDGenerator struct{}

var _ kernel.IDGenerator = IDGenerator{}

// NewID returns a new UUIDv7 in canonical form.
func (IDGenerator) NewID() string {
	return uuid.NewV7().String()
}

// NewIDAt returns a new UUIDv7 in canonical form whose timestamp is at's millisecond, or the
// current system time's when at is in the future, and whose 74 random bits come from crypto/rand.
func (IDGenerator) NewIDAt(at time.Time) string {
	if now := time.Now(); at.After(now) {
		at = now
	}
	u := uuid.MustParse(kernel.UUIDv7Floor(at))
	var r [10]byte
	_, _ = rand.Read(r[:]) // crypto/rand.Read never returns an error
	// The floor's rand_a and rand_b are zero: fill rand_a (the 12 bits after the version
	// nibble) and rand_b (the 62 bits after the variant bits).
	u[6] |= r[0] & 0x0f
	u[7] = r[1]
	u[8] |= r[2] & 0x3f
	copy(u[9:], r[3:])
	return u.String()
}
