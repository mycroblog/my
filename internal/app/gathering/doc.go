// SPDX-License-Identifier: AGPL-3.0-only

// Package gathering runs the events of the local users (ADR-0161): creating, revising, and
// cancelling them with a fixed date, a time zone, places, and a visibility; the invitations; the
// answers of the attendees, their approval, and the capacity; and the co-organizers. Every read
// and every operation decides who sees an event with gathering.CanSee, and who sees each place
// with gathering.CanSeePlace (internal/domain/gathering), so that a hidden place never leaves the
// Service.
//
// The module owns the tables of schema/19-gathering (data-model §4.19). Its commands write the
// rows of the events and then emit the domain events of events.go in the same transaction, whose
// projectors derive gathering_heads, gathering_attendees, and gathering_coorganizers
// (RebuildProjections derives them again). It reads the accounts, the roles, the instance
// settings, and the voice chat through Members, and the follows and blocks through Relations,
// which the composition root implements; nothing is federated (ADR-0161 §1).
package gathering
