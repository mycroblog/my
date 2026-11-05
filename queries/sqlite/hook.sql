-- SPDX-License-Identifier: AGPL-3.0-only

-- The decisions of the decision hooks (data-model 4.16, ADR-0147 6). Every table is an event: rows
-- are written once and only the retention and the erasure delete them.

-- name: InsertHookDecision :execrows
INSERT INTO hook_decisions (id, hook, subject_key, account_id, mode, outcome, reason_code, note, latency_ms, decided_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (hook, subject_key) DO NOTHING;

-- name: GetHookDecisionBySubject :one
SELECT d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decisions d
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE d.hook = ? AND d.subject_key = ?;

-- name: GetHookDecision :one
SELECT d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decisions d
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE d.id = ?;

-- name: InsertHookDecisionStatus :exec
INSERT INTO hook_decision_statuses (decision_id, status_id) VALUES (?, ?)
ON CONFLICT (decision_id, status_id) DO NOTHING;

-- name: InsertHookDecisionOverride :execrows
INSERT INTO hook_decision_overrides (decision_id, overridden_by_user_id, overridden_at) VALUES (?, ?, ?)
ON CONFLICT (decision_id) DO NOTHING;

-- Without an outcome, the list leaves out allow: it shows what a hook would stop or could not check.
-- name: ListHookDecisions :many
SELECT d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decisions d
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE (CAST(sqlc.arg(before) AS TEXT) = '' OR d.id < sqlc.arg(before))
  AND (CAST(sqlc.arg(hook) AS TEXT) = '' OR d.hook = sqlc.arg(hook))
  AND (CAST(sqlc.arg(mode) AS TEXT) = '' OR d.mode = sqlc.arg(mode))
  AND (CAST(sqlc.arg(outcome) AS TEXT) = '' OR d.outcome = sqlc.arg(outcome))
  AND (CAST(sqlc.arg(outcome) AS TEXT) <> '' OR d.outcome <> 'allow')
ORDER BY d.id DESC
LIMIT sqlc.arg(row_limit);

-- name: SummarizeHookDecisions :many
SELECT hook, mode, outcome, COUNT(*) AS decisions,
  CAST(COALESCE(SUM(latency_ms), 0) AS INTEGER) AS total_latency_ms,
  CAST(COALESCE(MAX(latency_ms), 0) AS INTEGER) AS max_latency_ms
FROM hook_decisions
WHERE decided_at >= ?
GROUP BY hook, mode, outcome
ORDER BY hook, mode, outcome;

-- The decisions past their retention, oldest first after the position of the last batch.
-- name: ListExpiredHookDecisions :many
SELECT d.id, d.hook, d.account_id, d.mode, d.outcome, d.decided_at,
  CAST(EXISTS (SELECT 1 FROM hook_decision_overrides o WHERE o.decision_id = d.id) AS INTEGER) AS overridden
FROM hook_decisions d
WHERE d.decided_at < sqlc.arg(before)
  AND (CAST(sqlc.arg(after_id) AS TEXT) = '' OR d.decided_at > sqlc.arg(after_at)
    OR (d.decided_at = sqlc.arg(after_at) AND d.id > sqlc.arg(after_id)))
ORDER BY d.decided_at, d.id
LIMIT sqlc.arg(row_limit);

-- name: ListStatusesOfHookDecisions :many
SELECT decision_id, status_id FROM hook_decision_statuses WHERE decision_id IN (sqlc.slice(ids)) ORDER BY decision_id, status_id;

-- name: ListHookDecisionIDsOfAccount :many
SELECT id FROM hook_decisions WHERE account_id = ? ORDER BY id LIMIT ?;

-- name: DeleteHookDecisionStatuses :exec
DELETE FROM hook_decision_statuses WHERE decision_id IN (sqlc.slice(ids));

-- name: DeleteHookDecisionOverrides :exec
DELETE FROM hook_decision_overrides WHERE decision_id IN (sqlc.slice(ids));

-- name: DeleteHookDecisions :exec
DELETE FROM hook_decisions WHERE id IN (sqlc.slice(ids));

-- ListHookSensitivityMarks returns the statuses of the mark_sensitive decisions that enforce and that
-- no moderator cancelled, the source of the rebuild of status_sensitivity_overrides (ADR-0147 8).
-- name: ListHookSensitivityMarks :many
SELECT s.status_id, s.decision_id
FROM hook_decision_statuses s
JOIN hook_decisions d ON d.id = s.decision_id
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE d.mode = 'enforce' AND d.outcome = 'mark_sensitive' AND o.decision_id IS NULL
ORDER BY s.decision_id, s.status_id;
