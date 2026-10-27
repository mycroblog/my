// SPDX-License-Identifier: AGPL-3.0-only

// Package kernel holds the ports and helpers shared by every module's application layer:
// the Clock and IDGenerator ports and UUIDv7 helpers, the KeyManager port with the Sealer
// that encrypts secrets for storage (ADR-0019 §6), the KeyValueStore port with the
// RateLimiter built on it and the client IP of an HTTP request that keys its IP limits
// (ADR-0019 §5), the Mailer port, the HTMLSanitizer port for remote HTML (ADR-0019 §2), and the
// Meter port of the metrics (ADR-0055), and the ErrorReporter port with kernel.Go, which starts
// the goroutines whose panics it reports (ADR-0103).
// Use cases obtain time and IDs only through these ports and pass plain values
// (time.Time, ID strings) to the domain, so that tests stay deterministic (ADR-0015).
// The production Clock and IDGenerator live in internal/adapter/system, the KeyManager adapters
// in internal/adapter/keymanager, the KeyValueStore adapters in internal/adapter/kv, the
// Mailer adapters in internal/adapter/mail, the HTMLSanitizer in internal/adapter/htmlsanitizer, and
// the Meter adapters in internal/adapter/telemetry;
// contract tests and deterministic fakes live in pkg/testkit.
package kernel

import "time"

// Clock tells the current time.
//
// Now returns the current time in UTC, truncated to millisecond precision
// (the precision every database dialect stores, ADR-0003), without a monotonic clock reading.
type Clock interface {
	Now() time.Time
}

// IDGenerator issues primary keys.
//
// NewID returns a new UUIDv7 in its canonical form (36 lowercase characters).
// IDs returned by one generator sort in increasing order, also within the same millisecond,
// unless the underlying clock moves backwards.
//
// NewIDAt returns a new UUIDv7 whose timestamp is the millisecond containing at, for records
// whose place in time comes from elsewhere, such as a remote status's AP published time
// (docs/architecture/data-model/01-conventions.md §1.3). A time after the generator's current time is rounded
// down to the current time, so a remote clock cannot place a record ahead of local ones;
// times outside the 48-bit range are clamped as in UUIDv7Floor. IDs issued for the same
// millisecond are unique, held apart by their random bits, but their order is unspecified;
// all of them sort above UUIDv7Floor of that millisecond.
type IDGenerator interface {
	NewID() string
	NewIDAt(at time.Time) string
}

// Normalize converts t to the form Clock.Now returns: UTC, truncated to milliseconds,
// with the monotonic clock reading stripped.
func Normalize(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}
