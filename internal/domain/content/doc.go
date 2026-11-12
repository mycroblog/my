// SPDX-License-Identifier: AGPL-3.0-only

// Package content is the pure domain model of the content module: statuses, media attachments, mentions, tags, custom emoji, and polls.
// It holds entities, value objects, and rules, and imports only the standard library.
//
// A status is a resource whose facts never change: its URI (a local status gets one too, ADR-0018),
// author, visibility, the URI it replies to, and when it was published. A remote status's
// visibility comes from its addressing (to and cc). Everything an edit can change (the body, the
// content warning, the language, the attachments, mentions, tags, and emoji) belongs to a
// revision; revisions are numbered from 1 and the highest is current (FR-109). The body of a
// revision is one row per language, the normal form of ActivityStreams contentMap; a document with
// only content has one row in the revision's language.
//
// A reply holds only the URI of its parent, never the parent's ID: the parent may arrive later,
// and the reply then needs no update. The link between a known reply and a known parent is a
// projection (status_reply_links).
//
// Deleting a status records a deletion, which is also its tombstone: a late Create or Update of
// the URI, or a fetch of it, never brings the status back. A deletion by retention is the
// exception, since the status may be fetched again as a new row and expire again, so a URI has at
// most one tombstone but any number of retention deletions.
package content
