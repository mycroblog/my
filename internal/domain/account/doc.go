// SPDX-License-Identifier: AGPL-3.0-only

// Package account is the pure domain model of the account module: local and remote accounts (actors), their profiles, and their keys.
// It holds entities, value objects, and rules, and imports only the standard library.
//
// An account is the identity of an actor, local or remote, and only the facts fixed when it is
// created; whether it is local or remote is a subtype of it, not a nullable column. What changes
// is kept as events: the profile as revisions, the lifecycle (deactivation, scheduled deletion,
// deletion) as lifecycle events, the moves as moves and their cancellations, and the signing keys
// as keys and their revocations. Each projects a current value that can be rebuilt from them.
//
// The flags of a profile, such as locked and bot, are part of its revision, so the history says
// since when an account was locked. A local revision keeps the bio and the field values as their
// author wrote them, and the HTML rendered from them by the rules of a status body; an update
// writes a whole new revision that copies what it does not change. A remote revision keeps only
// the sanitized HTML of the fetched document.
//
// The lifecycle is the axis of the owner and the system; moderation and moves are other axes and
// do not change it. An account without a lifecycle state is active, and a deleted account stays
// deleted, but for the system's restore of a remote actor whose server answers again
// (LifecycleState.Apply).
package account
