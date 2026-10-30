-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertAnnouncement :exec
INSERT INTO announcements (id, created_at) VALUES (?, ?);

-- name: InsertAnnouncementRevision :exec
INSERT INTO announcement_revisions (id, announcement_id, title, body, starts_at, ends_at, revised_by_user_id, revised_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertAnnouncementStateChange :exec
INSERT INTO announcement_state_changes (id, announcement_id, state, changed_by_user_id, changed_at) VALUES (?, ?, ?, ?, ?);

-- name: GetAnnouncementHead :one
SELECT announcement_id, revision_id, state, published_at FROM announcement_heads WHERE announcement_id = ?;

-- name: UpsertAnnouncementHead :exec
INSERT INTO announcement_heads (announcement_id, revision_id, state, published_at) VALUES (?, ?, ?, ?)
ON CONFLICT (announcement_id) DO UPDATE SET
  revision_id = excluded.revision_id, state = excluded.state, published_at = excluded.published_at;

-- name: DeleteAllAnnouncementHeads :exec
DELETE FROM announcement_heads;

-- The ID and announcement of every revision, oldest first, for rebuilding.
-- name: ListAnnouncementRevisionIDs :many
SELECT announcement_id, id FROM announcement_revisions ORDER BY id;

-- Every state change, oldest first, for rebuilding.
-- name: ListAnnouncementStateChanges :many
SELECT id, announcement_id, state, changed_by_user_id, changed_at FROM announcement_state_changes ORDER BY id;

-- An announcement with its current revision and state.
-- name: GetAnnouncement :one
SELECT a.id, a.created_at, h.state, h.published_at, v.id AS revision_id, v.title, v.body, v.starts_at, v.ends_at,
  v.revised_by_user_id, v.revised_at
FROM announcements a
JOIN announcement_heads h ON h.announcement_id = a.id
JOIN announcement_revisions v ON v.id = h.revision_id
WHERE a.id = ?;

-- Every announcement, newest first.
-- name: ListAnnouncements :many
SELECT a.id, a.created_at, h.state, h.published_at, v.id AS revision_id, v.title, v.body, v.starts_at, v.ends_at,
  v.revised_by_user_id, v.revised_at
FROM announcements a
JOIN announcement_heads h ON h.announcement_id = a.id
JOIN announcement_revisions v ON v.id = h.revision_id
ORDER BY a.id DESC;

-- The published announcements, newest first.
-- name: ListPublishedAnnouncements :many
SELECT a.id, a.created_at, h.state, h.published_at, v.id AS revision_id, v.title, v.body, v.starts_at, v.ends_at,
  v.revised_by_user_id, v.revised_at
FROM announcements a
JOIN announcement_heads h ON h.announcement_id = a.id
JOIN announcement_revisions v ON v.id = h.revision_id
WHERE h.state = 'published'
ORDER BY a.id DESC;

-- Records that a user read an announcement; reading it again keeps the first time.
-- name: InsertAnnouncementRead :exec
INSERT INTO announcement_reads (user_id, announcement_id, read_at) VALUES (?, ?, ?)
ON CONFLICT (user_id, announcement_id) DO NOTHING;

-- name: ListReadAnnouncementIDs :many
SELECT announcement_id FROM announcement_reads
WHERE user_id = sqlc.arg(user_id) AND announcement_id IN (sqlc.slice(announcement_ids))
ORDER BY announcement_id;

-- name: DeleteAnnouncementReadsOfUser :execrows
DELETE FROM announcement_reads WHERE user_id = ?;
