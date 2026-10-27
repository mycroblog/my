// SPDX-License-Identifier: AGPL-3.0-only

// Package diagnostics is the pure domain model of the error groups (ADR-0103 §4 (2), §7): the
// reported errors grouped by their fingerprint, the states of a group (open, resolved, muted),
// the events that change them, and the rule that tells a regression from a recurrence. It
// imports only the standard library.
package diagnostics

import (
	"strings"
	"time"
	"unicode/utf8"
)

// State is the current state of an error group (error_group_states.state).
type State string

// States of a group.
const (
	// StateOpen is a group that nobody has resolved or muted, or one that regressed or was
	// unmuted.
	StateOpen State = "open"
	// StateResolved is a group that an administrator resolved; a recurrence with another version
	// is a regression.
	StateResolved State = "resolved"
	// StateMuted is a group that an administrator muted: it is counted but never notified.
	StateMuted State = "muted"
)

// EventKind is the kind of an error group event (error_group_events.kind).
type EventKind string

// Kinds of the events of a group.
const (
	EventResolved  EventKind = "resolved"
	EventRegressed EventKind = "regressed"
	EventMuted     EventKind = "muted"
	EventUnmuted   EventKind = "unmuted"
)

// ActorKind is who recorded an event (error_group_events.actor_kind).
type ActorKind string

// Kinds of actors.
const (
	// ActorUser is an administrator of the web client or the API; the event has their user ID.
	ActorUser ActorKind = "user"
	// ActorCLI is the command line, which acts as no user.
	ActorCLI ActorKind = "cli"
	// ActorSystem is the server itself: a regression.
	ActorSystem ActorKind = "system"
)

// Next returns the state after an event of kind on a group in state s, and whether the event
// changes anything. An event that changes nothing is not recorded:
//
//   - resolved: from open or muted;
//   - muted: from open or resolved;
//   - unmuted: from muted, to open;
//   - regressed: from resolved, to open.
func Next(s State, kind EventKind) (State, bool) {
	switch kind {
	case EventResolved:
		return StateResolved, s != StateResolved
	case EventMuted:
		return StateMuted, s != StateMuted
	case EventUnmuted:
		if s == StateMuted {
			return StateOpen, true
		}
	case EventRegressed:
		if s == StateResolved {
			return StateOpen, true
		}
	}
	return s, false
}

// Regresses reports whether an occurrence with version of a group in state s, resolved when its
// last occurrence had resolvedVersion, is a regression (ADR-0103 §7): the group is resolved and
// the version differs. A recurrence of the same version is only counted, since it is what was
// still running before the deployment of the fix.
func Regresses(s State, resolvedVersion, version string) bool {
	return s == StateResolved && version != resolvedVersion
}

// MessageHeadRunes bounds the head of the message that an error group keeps
// (error_groups.message): its first line, at most this many runes.
const MessageHeadRunes = 200

// MessageHead returns the first line of the scrubbed message msg, cut to MessageHeadRunes runes.
func MessageHead(msg string) string {
	msg = CleanText(msg)
	if i := strings.IndexAny(msg, "\r\n"); i >= 0 {
		msg = msg[:i]
	}
	if utf8.RuneCountInString(msg) <= MessageHeadRunes {
		return msg
	}
	n := 0
	for i := range msg {
		if n == MessageHeadRunes {
			return msg[:i]
		}
		n++
	}
	return msg
}

// CleanText returns s as valid UTF-8 without the null character, each replaced by U+FFFD
// (data-model §1.2): every text column of a group holds what the reporter received.
func CleanText(s string) string {
	s = strings.ToValidUTF8(s, string(utf8.RuneError))
	return strings.ReplaceAll(s, "\x00", string(utf8.RuneError))
}

// LastType returns the last type of the chain of Go type names typ ("*fmt.wrapError >
// *pgconn.PgError" gives "*pgconn.PgError"), which names a group.
func LastType(typ string) string {
	if i := strings.LastIndex(typ, " > "); i >= 0 {
		return typ[i+len(" > "):]
	}
	return typ
}

// Hour returns the start of the hour of t in UTC: the bucket of error_group_counts.
func Hour(t time.Time) time.Time { return t.UTC().Truncate(time.Hour) }

// CountRetention is how long the hourly counts of a group stay (ADR-0103 §7).
const CountRetention = 90 * 24 * time.Hour
