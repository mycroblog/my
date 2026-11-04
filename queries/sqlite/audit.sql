-- SPDX-License-Identifier: AGPL-3.0-only

-- The audit log (data-model 4.3, ADR-0058). It is append-only: there is no query that updates or
-- deletes an entry, except the retention cleanup (ADR-0058).

-- name: InsertAuditLogEntry :exec
INSERT INTO audit_log_entries (id, actor_kind, actor_user_id, action, target_kind, target_id, target_label,
  source_event_id, occurred_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- ListAuditLogEntries returns the entries newest first: those of an actor, an action, a kind of
-- target, and a target (each empty for any), before the entry before (empty for the first page).
-- name: ListAuditLogEntries :many
SELECT id, actor_kind, actor_user_id, action, target_kind, target_id, target_label, source_event_id, occurred_at
FROM audit_log_entries
WHERE (CAST(sqlc.arg(actor_user_id) AS TEXT) = '' OR actor_user_id = sqlc.arg(actor_user_id))
  AND (CAST(sqlc.arg(action) AS TEXT) = '' OR action = sqlc.arg(action))
  AND (CAST(sqlc.arg(target_kind) AS TEXT) = '' OR target_kind = sqlc.arg(target_kind))
  AND (CAST(sqlc.arg(target_id) AS TEXT) = '' OR target_id = sqlc.arg(target_id))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR id < sqlc.arg(before))
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- The targets of the entries that still exist, for the audit log to tell those that are gone
-- (erased past their retention, ADR-0058). Each returns, in ID order, the records among ids
-- that exist. The ids are a JSON array.

-- name: ListExistingReportIDs :many
SELECT id FROM reports WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;

-- name: ListExistingAppealIDs :many
SELECT id FROM moderation_appeals WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;

-- name: ListExistingSignupBlockIDs :many
SELECT id FROM signup_blocks WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;
