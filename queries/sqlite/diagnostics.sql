-- SPDX-License-Identifier: AGPL-3.0-only

-- The error groups (data-model 4.15, ADR-0103 4 (2) and 7). Groups and events are written
-- once; error_group_states is the projection; the counts and the samples are current values
-- that the Recorder writes outside transactions.

-- name: InsertErrorGroup :execrows
INSERT INTO error_groups (id, fingerprint, source, kind, error_type, message, where_name, first_version, unauthenticated, first_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (fingerprint) DO NOTHING;

-- name: GetErrorGroupCurrentByFingerprint :one
SELECT g.id, g.fingerprint, g.source, g.kind, g.error_type, g.message, g.where_name, g.first_version, g.unauthenticated, g.first_seen_at,
  s.state, s.resolved_version, s.since, COALESCE(sm.version, g.first_version) AS last_version
FROM error_groups g
JOIN error_group_states s ON s.group_id = g.id
LEFT JOIN error_group_samples sm ON sm.group_id = g.id
WHERE g.fingerprint = ?;

-- name: GetErrorGroupCurrent :one
SELECT g.id, g.fingerprint, g.source, g.kind, g.error_type, g.message, g.where_name, g.first_version, g.unauthenticated, g.first_seen_at,
  s.state, s.resolved_version, s.since, COALESCE(sm.version, g.first_version) AS last_version
FROM error_groups g
JOIN error_group_states s ON s.group_id = g.id
LEFT JOIN error_group_samples sm ON sm.group_id = g.id
WHERE g.id = ?;

-- name: InsertErrorGroupConfirmation :execrows
INSERT INTO error_group_confirmations (group_id, confirmed_at) VALUES (?, ?)
ON CONFLICT (group_id) DO NOTHING;

-- name: InsertErrorGroupEvent :exec
INSERT INTO error_group_events (id, group_id, kind, actor_kind, actor_user_id, version, occurred_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpsertErrorGroupState :exec
INSERT INTO error_group_states (group_id, state, resolved_version, since) VALUES (?, ?, ?, ?)
ON CONFLICT (group_id) DO UPDATE SET
  state = excluded.state, resolved_version = excluded.resolved_version, since = excluded.since;

-- AddErrorGroupCount adds occurrences to the count of the hour of the group of fingerprint, in
-- one statement, and keeps the time and version of the latest occurrence.
-- name: AddErrorGroupCount :exec
INSERT INTO error_group_counts (group_id, hour_start, occurrences, last_seen_at, last_version)
SELECT g.id, CAST(sqlc.arg(hour_start) AS INTEGER), CAST(sqlc.arg(occurrences) AS INTEGER),
  CAST(sqlc.arg(last_seen_at) AS INTEGER), CAST(sqlc.arg(last_version) AS TEXT)
FROM error_groups g WHERE g.fingerprint = CAST(sqlc.arg(fingerprint) AS TEXT)
ON CONFLICT (group_id, hour_start) DO UPDATE SET
  occurrences = error_group_counts.occurrences + excluded.occurrences,
  last_version = CASE WHEN excluded.last_seen_at >= error_group_counts.last_seen_at
    THEN excluded.last_version ELSE error_group_counts.last_version END,
  last_seen_at = max(error_group_counts.last_seen_at, excluded.last_seen_at);

-- PutErrorGroupSample makes the occurrence the sample of the group of fingerprint, unless the
-- sample there occurred later.
-- name: PutErrorGroupSample :exec
INSERT INTO error_group_samples (group_id, error_type, message, stack, request_id, trace_id, version, role, occurred_at)
SELECT g.id, CAST(sqlc.arg(error_type) AS TEXT), CAST(sqlc.arg(message) AS TEXT), CAST(sqlc.arg(stack) AS TEXT),
  CAST(sqlc.arg(request_id) AS TEXT), CAST(sqlc.arg(trace_id) AS TEXT), CAST(sqlc.arg(version) AS TEXT),
  CAST(sqlc.arg(role) AS TEXT), CAST(sqlc.arg(occurred_at) AS INTEGER)
FROM error_groups g WHERE g.fingerprint = CAST(sqlc.arg(fingerprint) AS TEXT)
ON CONFLICT (group_id) DO UPDATE SET
  error_type = excluded.error_type, message = excluded.message, stack = excluded.stack,
  request_id = excluded.request_id, trace_id = excluded.trace_id, version = excluded.version,
  role = excluded.role, occurred_at = excluded.occurred_at
WHERE excluded.occurred_at >= error_group_samples.occurred_at;

-- name: DeleteErrorGroupCountsBefore :execrows
DELETE FROM error_group_counts WHERE hour_start < ?;

-- name: GetErrorGroup :one
SELECT g.id, g.fingerprint, g.source, g.kind, g.error_type, g.message, g.where_name, g.first_version, g.unauthenticated, g.first_seen_at,
  s.state, s.resolved_version, s.since
FROM error_groups g JOIN error_group_states s ON s.group_id = g.id
WHERE g.id = ?;

-- name: GetErrorGroupSample :one
SELECT g.fingerprint, sm.error_type, sm.message, sm.stack, sm.request_id, sm.trace_id, sm.version, sm.role, sm.occurred_at
FROM error_group_samples sm JOIN error_groups g ON g.id = sm.group_id
WHERE sm.group_id = ?;

-- name: ListErrorGroupCounts :many
SELECT hour_start, occurrences, last_seen_at, last_version FROM error_group_counts
WHERE group_id = ? ORDER BY hour_start;

-- name: ListErrorGroupEvents :many
SELECT id, group_id, kind, actor_kind, actor_user_id, version, occurred_at FROM error_group_events
WHERE group_id = ? ORDER BY id;

-- name: DeleteErrorGroupStates :exec
DELETE FROM error_group_states;

-- RebuildErrorGroupStates derives the state of every group from its newest event: resolved
-- and muted as they are, open for unmuted, regressed, and no event.
-- name: RebuildErrorGroupStates :exec
INSERT INTO error_group_states (group_id, state, resolved_version, since)
SELECT g.id,
  CASE e.kind WHEN 'resolved' THEN 'resolved' WHEN 'muted' THEN 'muted' ELSE 'open' END,
  CASE WHEN e.kind = 'resolved' THEN e.version ELSE '' END,
  COALESCE(e.occurred_at, g.first_seen_at)
FROM error_groups g
LEFT JOIN error_group_events e ON e.id = (
  SELECT x.id FROM error_group_events x WHERE x.group_id = g.id ORDER BY x.id DESC LIMIT 1);
