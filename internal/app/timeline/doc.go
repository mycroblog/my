// SPDX-License-Identifier: AGPL-3.0-only

// Package timeline holds the use cases and port definitions of the timeline module: assembly of the home, local, federated, tag, and profile timelines.
// It is the module's public API; other modules reach the timeline module only through this package.
//
// Timelines are read, not written ahead (fan-out-on-read, ADR-0013): the module reads only its own
// projections (feed_*), which its projectors build from the events of content and graph. A query
// narrows the candidates, and every page applies content.CanView again before it returns: a status
// as itself, and a boost as the booster's post with the original's visibility and the original as
// a single fetch. The relations and the restrictions of the authors on the page are read in one
// batch from feed_*, and mutes are dropped at the same step.
//
// feed_entries keep the original visibility. The restrictions of moderation and of an account's
// lifecycle (feed_author_restrictions, feed_domain_restrictions) are applied when reading, so
// lifting one needs no rewrite. Their rows come from moderation.account_restrictions_changed and
// moderation.domain_restrictions_changed, which carry the restrictions in force after each change,
// so the projector writes those present and deletes those absent. A deleted, remotely deleted, or
// deactivated account is represented as profile_view, and reactivation replaces it with the
// restrictions in force. Since rows change only with an event, an expired row stays until the next
// change; it restricts nothing, and RebuildRestrictions keeps it.
//
// RebuildProjections empties feed_* (except the two restriction tables) and tag_usage, and applies
// this module's projectors to the events that content.ReplayEvents and graph.ReplayEvents emit
// from the current state inside the rebuild's transaction. Lists, followed and featured tags,
// keyword filters, and the read and hidden marks of direct messages are records of what users
// did, not projections, and are never emptied; the rebuild only removes the list members that the
// rebuilt follows no longer allow.
//
// The read and hidden marks of a direct-message conversation are boundaries, the ID of an entry,
// rather than booleans: unread and hidden are derived on each read, and one row holds for every
// device of the viewer. After a direct status commits, the stream tells each participant of the
// conversation; its mention notifications are made by notification, as for any status.
package timeline
