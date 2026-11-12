// SPDX-License-Identifier: AGPL-3.0-only

// Package instance is the pure domain model of the instance module: instance settings, terms, rules, and NodeInfo.
// It holds entities, value objects, and rules, and imports only the standard library.
//
// An instance setting is one scalar per key, defined with its type and default in this package;
// a setting that holds several values is several keys, never one structured value. Each change
// is a revision, and the current value is a projection of the newest one. A text has one value,
// whatever the reader's locale, and an empty one counts as absent.
//
// An announcement is drafted, published, and withdrawn by the holders of manage_announcements.
// Every change of its title, body, or display window is a revision, and its head points at the
// newest revision and the last state change. A withdrawal is final: a withdrawn announcement is
// neither changed nor published again, and a new one takes its place.
package instance
