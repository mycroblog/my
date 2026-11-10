-- SPDX-License-Identifier: AGPL-3.0-only

-- Announcements and service closure events by many IDs at once, one query each, for the pages of
-- notifications. Each reads what GetAnnouncement and GetServiceClosureEvent read of one. The lists are JSON arrays.

-- name: ListAnnouncementsByIDs :many
SELECT a.id, a.created_at, h.state, h.published_at, v.id AS revision_id, v.title, v.body, v.starts_at, v.ends_at,
  v.revised_by_user_id, v.revised_at
FROM announcements a
JOIN announcement_heads h ON h.announcement_id = a.id
JOIN announcement_revisions v ON v.id = h.revision_id
WHERE a.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY a.id;

-- name: ListServiceClosureEventsByIDs :many
SELECT e.id, e.kind, e.changed_by_user_id, e.occurred_at, s.closes_at, s.export_until, s.message
FROM service_closure_events e
LEFT JOIN service_closure_schedules s ON s.event_id = e.id
WHERE e.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY e.id;
