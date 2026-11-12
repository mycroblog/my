// SPDX-License-Identifier: AGPL-3.0-only

// Package federation is the pure domain model of the federation module: ActivityPub inbox processing, delivery, actor and object fetching, signatures, and remote domain health.
// It holds entities, value objects, and rules, and imports only the standard library.
//
// An outbound activity names the local object it is about by its kind (OutboundObjectKind) and an
// ID whose table the kind decides. Two kinds point past their own row: a favourite is a current
// value whose row is gone once undone, so a favourite activity names the favourited status, as a
// poll vote names the poll's status; and a report activity, the Flag that forwards a report, is
// the instance actor's and names the report.
package federation
