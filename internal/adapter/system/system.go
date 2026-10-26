// SPDX-License-Identifier: AGPL-3.0-only

// Package system implements the Clock and IDGenerator ports of internal/app/kernel with
// the operating system's clock and the standard library's UUIDv7 generator.
package system

import (
	"crypto/rand"
	"encoding/binary"
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

// IDGenerator issues UUIDv7s whose 12 bits after the millisecond hold the sub-millisecond time
// (RFC 9562 §6.2, method 3, like uuid.NewV7), and fills the random bits of IDs for a given time
// from crypto/rand.
type IDGenerator struct{}

var _ kernel.IDGenerator = IDGenerator{}

// NewID returns a new UUIDv7 in canonical form. Within the process the IDs it returns increase,
// over a step back of the system clock of up to maxHold too, the way Clock.Now holds over one:
// uuid.NewV7 restarts from the system time when the clock steps back across a second, so the
// IDs of operations made one after another would no longer sort in the order they were made.
func (IDGenerator) NewID() string {
	ts := processIDs.next(time.Now())
	var u uuid.UUID
	// unix_ts_ms (48 bits), ver (4 bits), and the sub-millisecond fraction in rand_a (12 bits).
	binary.BigEndian.PutUint64(u[0:8], uint64(ts>>12)<<16|0x7000|uint64(ts&0x0fff)) // #nosec G115 -- ts is positive: the system clock is after 1970
	_, _ = rand.Read(u[8:])                                                         // crypto/rand.Read never returns an error
	u[8] = u[8]&0x3f | 0x80                                                         // the variant bits 10
	return u.String()
}

// processIDs is the timestamp of the latest ID that NewID returned in the process.
var processIDs idFloor

// idFloor holds the latest UUIDv7 timestamp it returned, in units of 1/4096 millisecond.
type idFloor struct{ last atomic.Int64 }

// next returns the timestamp of wall in units of 1/4096 millisecond, or one unit after the
// latest one it returned when wall is not after it by at most maxHold.
func (f *idFloor) next(wall time.Time) int64 {
	ts := wall.UnixMilli()<<12 | int64(wall.Nanosecond()%int(time.Millisecond))*4096/int64(time.Millisecond)
	for {
		prev := f.last.Load()
		next := ts
		if ts <= prev && prev-ts <= maxHold.Milliseconds()<<12 {
			next = prev + 1
		}
		if f.last.CompareAndSwap(prev, next) {
			return next
		}
	}
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
