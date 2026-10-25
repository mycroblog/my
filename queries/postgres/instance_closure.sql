-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertServiceClosureEvent :exec
INSERT INTO service_closure_events (id, kind, by_user_id, occurred_at) VALUES ($1, $2, $3, $4);

-- name: InsertServiceClosureSchedule :exec
INSERT INTO service_closure_schedules (event_id, closes_at, export_until, message) VALUES ($1, $2, $3, $4);

-- name: ListServiceClosureEvents :many
SELECT e.id, e.kind, e.by_user_id, e.occurred_at, s.closes_at, s.export_until, s.message
FROM service_closure_events e
LEFT JOIN service_closure_schedules s ON s.event_id = e.id
ORDER BY e.id;

-- name: GetServiceClosureHead :one
SELECT state, event_id, announcement_event_id FROM service_closure_heads WHERE id = 1;

-- name: UpsertServiceClosureHead :exec
INSERT INTO service_closure_heads (id, state, event_id, announcement_event_id) VALUES (1, $1, $2, $3)
ON CONFLICT (id) DO UPDATE SET state = excluded.state, event_id = excluded.event_id,
  announcement_event_id = excluded.announcement_event_id;

-- name: DeleteServiceClosureHead :exec
DELETE FROM service_closure_heads;

-- name: GetCurrentServiceClosure :one
SELECT h.state, e.occurred_at AS changed_at, a.occurred_at AS announced_at, s.closes_at, s.export_until, s.message
FROM service_closure_heads h
JOIN service_closure_events e ON e.id = h.event_id
JOIN service_closure_events a ON a.id = h.announcement_event_id
JOIN service_closure_schedules s ON s.event_id = h.announcement_event_id
WHERE h.id = 1;
