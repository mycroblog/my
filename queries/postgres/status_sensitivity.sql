-- SPDX-License-Identifier: AGPL-3.0-only

-- Sensitivity overrides of mark_statuses_sensitive and of the decisions of hooks (data-model section 4.4) ---

-- InsertStatusSensitivityOverride inserts nothing when the action already marks the status.
-- name: InsertStatusSensitivityOverride :exec
INSERT INTO status_sensitivity_overrides (status_id, source_kind, source_id) VALUES (@status_id, 'action', @source_id)
ON CONFLICT (status_id, source_kind, source_id) DO NOTHING;

-- InsertStatusSensitivityOverrideOfHookDecision inserts nothing when the decision already marks the status.
-- name: InsertStatusSensitivityOverrideOfHookDecision :exec
INSERT INTO status_sensitivity_overrides (status_id, source_kind, source_id) VALUES (@status_id, 'hook_decision', @source_id)
ON CONFLICT (status_id, source_kind, source_id) DO NOTHING;

-- name: DeleteStatusSensitivityOverridesOfAction :exec
DELETE FROM status_sensitivity_overrides WHERE source_kind = 'action' AND source_id = $1;

-- name: DeleteStatusSensitivityOverridesOfHookDecision :exec
DELETE FROM status_sensitivity_overrides WHERE source_kind = 'hook_decision' AND source_id = $1;

-- name: StatusSensitivityOverridden :one
SELECT EXISTS (SELECT 1 FROM status_sensitivity_overrides WHERE status_id = $1);

-- name: PurgeStatusSensitivityOverrides :exec
DELETE FROM status_sensitivity_overrides WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- ListExistingStatusIDs returns the statuses among ids that have a row, which the retention of the
-- hook decisions asks (ADR-0147 6).
-- name: ListExistingStatusIDs :many
SELECT id FROM statuses WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;
