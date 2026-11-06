-- SPDX-License-Identifier: AGPL-3.0-only

-- The decisions of the decision hooks (data-model 4.16, ADR-0147 6). Rows are written once and only
-- the retention and the erasure delete them, except the exemptions, which are current values that
-- only the erasure deletes.

-- name: InsertHookDecision :execrows
INSERT INTO hook_decisions (id, hook, subject_key, account_id, mode, outcome, reason_code, note, latency_ms, decided_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (hook, subject_key) DO NOTHING;

-- name: GetHookDecisionBySubject :one
SELECT d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decisions d
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE d.hook = $1 AND d.subject_key = $2;

-- name: GetHookDecision :one
SELECT d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decisions d
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE d.id = $1;

-- name: InsertHookDecisionStatus :exec
INSERT INTO hook_decision_statuses (decision_id, status_id) VALUES ($1, $2)
ON CONFLICT (decision_id, status_id) DO NOTHING;

-- name: InsertHookDecisionOverride :execrows
INSERT INTO hook_decision_overrides (decision_id, overridden_by_user_id, overridden_at) VALUES ($1, $2, $3)
ON CONFLICT (decision_id) DO NOTHING;

-- Without an outcome, the list leaves out allow: it shows what a hook would stop or could not check.
-- name: ListHookDecisions :many
SELECT d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decisions d
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE (sqlc.narg(before)::uuid IS NULL OR d.id < sqlc.narg(before)::uuid)
  AND (sqlc.arg(hook)::text = '' OR d.hook = sqlc.arg(hook)::text)
  AND (sqlc.arg(mode)::text = '' OR d.mode = sqlc.arg(mode)::text)
  AND (sqlc.arg(outcome)::text = '' OR d.outcome = sqlc.arg(outcome)::text)
  AND (sqlc.arg(outcome)::text <> '' OR d.outcome <> 'allow')
ORDER BY d.id DESC
LIMIT sqlc.arg(row_limit);

-- name: SummarizeHookDecisions :many
SELECT hook, mode, outcome, COUNT(*) AS decisions,
  CAST(COALESCE(SUM(latency_ms), 0) AS bigint) AS total_latency_ms,
  CAST(COALESCE(MAX(latency_ms), 0) AS bigint) AS max_latency_ms
FROM hook_decisions
WHERE decided_at >= $1
GROUP BY hook, mode, outcome
ORDER BY hook, mode, outcome;

-- The decisions past their retention, oldest first after the position of the last batch.
-- name: ListExpiredHookDecisions :many
SELECT d.id, d.hook, d.account_id, d.mode, d.outcome, d.decided_at,
  CAST(EXISTS (SELECT 1 FROM hook_decision_overrides o WHERE o.decision_id = d.id) AS boolean) AS overridden,
  CAST(EXISTS (SELECT 1 FROM hook_decision_appeals a WHERE a.decision_id = d.id
    AND NOT EXISTS (SELECT 1 FROM hook_decision_appeal_decisions r WHERE r.appeal_id = a.id)) AS boolean) AS appeal_pending
FROM hook_decisions d
WHERE d.decided_at < sqlc.arg(before)
  AND (sqlc.narg(after_id)::uuid IS NULL OR d.decided_at > sqlc.arg(after_at)
    OR (d.decided_at = sqlc.arg(after_at) AND d.id > sqlc.narg(after_id)::uuid))
ORDER BY d.decided_at, d.id
LIMIT sqlc.arg(row_limit);

-- name: ListStatusesOfHookDecisions :many
SELECT decision_id, status_id FROM hook_decision_statuses WHERE decision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY decision_id, status_id;

-- name: ListHookDecisionIDsOfAccount :many
SELECT id FROM hook_decisions WHERE account_id = $1 ORDER BY id LIMIT $2;

-- name: DeleteHookDecisionStatuses :exec
DELETE FROM hook_decision_statuses WHERE decision_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteHookDecisionOverrides :exec
DELETE FROM hook_decision_overrides WHERE decision_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteHookDecisionAppealDecisions :exec
DELETE FROM hook_decision_appeal_decisions
WHERE appeal_id IN (SELECT id FROM hook_decision_appeals WHERE decision_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: DeleteHookDecisionAppeals :exec
DELETE FROM hook_decision_appeals WHERE decision_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteHookDecisions :exec
DELETE FROM hook_decisions WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: InsertHookDecisionAppeal :execrows
INSERT INTO hook_decision_appeals (id, decision_id, account_id, status_text, comment, submitted_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (decision_id) DO NOTHING;

-- name: GetHookDecisionAppeal :one
SELECT a.id AS appeal_id, a.decision_id AS appeal_decision_id, a.account_id AS appeal_account_id, a.status_text, a.comment, a.submitted_at,
  r.decision AS review, r.decided_by_user_id AS reviewed_by_user_id, r.decided_at AS reviewed_at,
  d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decision_appeals a
JOIN hook_decisions d ON d.id = a.decision_id
LEFT JOIN hook_decision_appeal_decisions r ON r.appeal_id = a.id
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE a.id = $1;

-- name: GetHookDecisionAppealOfDecision :one
SELECT a.id AS appeal_id, a.decision_id AS appeal_decision_id, a.account_id AS appeal_account_id, a.status_text, a.comment, a.submitted_at,
  r.decision AS review, r.decided_by_user_id AS reviewed_by_user_id, r.decided_at AS reviewed_at,
  d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decision_appeals a
JOIN hook_decisions d ON d.id = a.decision_id
LEFT JOIN hook_decision_appeal_decisions r ON r.appeal_id = a.id
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE a.decision_id = $1;

-- The appeals newest first; pending keeps those that no moderator has decided, and outcome those
-- decided with it ('' keeps every one).
-- name: ListHookDecisionAppeals :many
SELECT a.id AS appeal_id, a.decision_id AS appeal_decision_id, a.account_id AS appeal_account_id, a.status_text, a.comment, a.submitted_at,
  r.decision AS review, r.decided_by_user_id AS reviewed_by_user_id, r.decided_at AS reviewed_at,
  d.id, d.hook, d.subject_key, d.account_id, d.mode, d.outcome, d.reason_code, d.note, d.latency_ms, d.decided_at,
  o.decision_id AS overridden_decision_id, o.overridden_by_user_id, o.overridden_at
FROM hook_decision_appeals a
JOIN hook_decisions d ON d.id = a.decision_id
LEFT JOIN hook_decision_appeal_decisions r ON r.appeal_id = a.id
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE (sqlc.narg(before)::uuid IS NULL OR a.id < sqlc.narg(before)::uuid)
  AND (NOT sqlc.arg(pending)::boolean OR r.appeal_id IS NULL)
  AND (sqlc.arg(outcome)::text = '' OR r.decision = sqlc.arg(outcome)::text)
ORDER BY a.id DESC
LIMIT sqlc.arg(row_limit);

-- name: InsertHookDecisionAppealDecision :execrows
INSERT INTO hook_decision_appeal_decisions (appeal_id, decision, decided_by_user_id, decided_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (appeal_id) DO NOTHING;

-- name: InsertHookAccountExemption :exec
INSERT INTO hook_account_exemptions (hook, account_id, exempted_at) VALUES ($1, $2, $3)
ON CONFLICT (hook, account_id) DO NOTHING;

-- name: HookAccountExempt :one
SELECT EXISTS (SELECT 1 FROM hook_account_exemptions WHERE hook = $1 AND account_id = $2) AS exempt;

-- name: DeleteHookAccountExemptionsOfAccount :exec
DELETE FROM hook_account_exemptions WHERE account_id = $1;

-- name: DeleteHookDecisionAppealDecisionsOfAccount :exec
DELETE FROM hook_decision_appeal_decisions
WHERE appeal_id IN (SELECT id FROM hook_decision_appeals WHERE account_id = $1);

-- name: DeleteHookDecisionAppealsOfAccount :exec
DELETE FROM hook_decision_appeals WHERE account_id = $1;

-- ListHookSensitivityMarks returns the statuses of the mark_sensitive decisions that enforce and that
-- no moderator cancelled, the source of the rebuild of status_sensitivity_overrides (ADR-0147 8).
-- name: ListHookSensitivityMarks :many
SELECT s.status_id, s.decision_id
FROM hook_decision_statuses s
JOIN hook_decisions d ON d.id = s.decision_id
LEFT JOIN hook_decision_overrides o ON o.decision_id = d.id
WHERE d.mode = 'enforce' AND d.outcome = 'mark_sensitive' AND o.decision_id IS NULL
ORDER BY s.decision_id, s.status_id;
