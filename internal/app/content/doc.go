// SPDX-License-Identifier: AGPL-3.0-only

// Package content holds the use cases and port definitions of the content module: statuses, media attachments, mentions, tags, custom emoji, and polls.
// It is the module's public API; other modules reach the content module only through this package.
//
// Every command writes its rows and emits one domain event of events.go in the same transaction,
// and the projectors update the projections from what the event carries. A status's creation,
// revision, and deletion emit content.status_created, content.status_revised, and
// content.status_deleted, whether the status is local or arrived by federation, so that the
// timelines and the mention notifications follow both alike. Boosts, favourites, and reactions
// emit an event only when a row really changed; bookmarks and pins emit none, since only their
// owner sees them. A deletion by the automatic deletion of old posts is recorded and emitted the
// way the author's own deletion is.
//
// The projections of statuses hold live statuses only: the transaction that records a deletion
// removes the status's rows from all of them. Favourites, bookmarks, and pins of a deleted status
// stay as long as the status's rows do, and change no count, since a deleted status has no
// counters. local_post_counters is the count NodeInfo reports as usage.localPosts (ADR-0006 §1).
// It is one row per local account, summed
// on each request, rather than one row for the instance that every post would contend for, so
// deleting a status also subtracts its local boosts from each booster's row.
//
// status_sensitivity_overrides is written in the same transaction as the moderation action or the
// hook decision that forces a status sensitive, and loses that row when the source is lifted.
// This module never reads the tables of moderation or hook, so its rebuild replays the sources
// through their public APIs and drops the statuses that no longer exist, since a source can
// outlive its status. status_link_cards is written by the projectors of a status's creation and
// revision, which enqueue the fetch of the card, and removed with the status. What a viewer may
// see of a quote (its target deleted, blocked, muted, or out of sight) is never stored: Reader
// judges it for each viewer on each read. A draft has no status yet; the status's ID and
// publication time are made when it is published.
package content
