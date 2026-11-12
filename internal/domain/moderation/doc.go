// SPDX-License-Identifier: AGPL-3.0-only

// Package moderation is the pure domain model of the moderation module: reports, domain blocks, suspensions, and the audit log.
// It holds entities, value objects, and rules, and imports only the standard library.
//
// An action is a moderator's decision, recorded once and never changed; a lift is another event,
// and an expiry is no event at all, only a time that reads stop honoring. The restrictions are
// the capabilities that the actions in effect take away, projected per account and per domain
// from the mapping of each kind of action to its restrictions in this package: several actions
// add up, and a lift removes only the rows of its own action. A domain's restrictions are never
// copied onto its accounts; they are combined with the account's when a policy asks.
//
// The entries of a subscribed domain list are what the last readable fetch of the list held, one
// per normalized domain. Their severity becomes an action kind when it is read, by the
// subscription's current revision, so a change of the settings rewrites no entry; an entry keeps
// the first fetch since which the domain has been listed unchanged. The list's body is not kept:
// forgetting the validators of the last fetch makes the next one rebuild the entries.
//
// An audit log entry names its target by kind and ID with a label copied when it is written (a
// handle, a domain, a setting's key), so that it still reads after the target is gone, and
// points with source_event_id at the row of the event that it records, from which its reason and
// notes are read instead of being copied. Each AuditAction belongs to one kind of target.
package moderation
