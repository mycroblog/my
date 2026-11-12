// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"encoding/binary"
	"time"
	"uuid"
)

// maxUnixMilli is the largest timestamp the 48-bit unix_ts_ms field of a UUIDv7 can hold.
const maxUnixMilli = 1<<48 - 1

// UUIDv7Floor returns the smallest UUIDv7 whose timestamp is the millisecond containing t,
// in canonical form. Every UUIDv7 generated at or after that millisecond compares greater than
// or equal to it, both as a UUID and as a string, so `id >= UUIDv7Floor(t)` bounds a keyset scan
// by time (ADR-0003 §6).
//
// Times before the Unix epoch map to the floor of the epoch, and times beyond the 48-bit range
// map to the floor of the last representable millisecond.
func UUIDv7Floor(t time.Time) string {
	ms := min(max(t.UnixMilli(), 0), maxUnixMilli)

	var u uuid.UUID
	// unix_ts_ms occupies the most significant 48 bits; the low 16 bits of the word are
	// the version nibble (7) followed by rand_a, which is zero for the floor.
	binary.BigEndian.PutUint64(u[0:8], uint64(ms)<<16|0x7000)
	// The variant bits are 0b10; rand_b is zero for the floor.
	u[8] = 0x80
	return u.String()
}

// UUIDv7Time returns the millisecond that the UUIDv7 id was generated in, in UTC, and false
// when id is not a UUIDv7.
func UUIDv7Time(id string) (time.Time, bool) {
	u, err := uuid.Parse(id)
	if err != nil || u[6]>>4 != 7 {
		return time.Time{}, false
	}
	ms := binary.BigEndian.Uint64(u[0:8]) >> 16
	return time.UnixMilli(int64(ms)).UTC(), true
}
