-- SPDX-License-Identifier: AGPL-3.0-only

-- Scheduled deletions of accounts (data-model sections 4.2 and 8) ---------------------------------------

-- ListDueDeletions returns up to lim accounts whose deletion is scheduled at or before now,
-- earliest first.
-- name: ListDueDeletions :many
SELECT account_id, state, since, deletion_due_at FROM account_lifecycle_states
WHERE state = 'deletion_scheduled' AND deletion_due_at <= CAST(sqlc.arg(now) AS INTEGER)
ORDER BY deletion_due_at, account_id
LIMIT sqlc.arg(lim);

-- ListDueDeactivations returns up to lim accounts whose owner deactivated them at or before cutoff,
-- longest deactivated first (ADR-0145 section 2).
-- name: ListDueDeactivations :many
SELECT account_id, state, since, deletion_due_at FROM account_lifecycle_states
WHERE state = 'deactivated' AND since <= CAST(sqlc.arg(cutoff) AS INTEGER)
ORDER BY since, account_id
LIMIT sqlc.arg(lim);

-- FindLatestReactivation returns when an account was last reactivated (ADR-0145 section 8).
-- name: FindLatestReactivation :one
SELECT occurred_at FROM account_lifecycle_events
WHERE account_id = ? AND kind = 'reactivated'
ORDER BY occurred_at DESC
LIMIT 1;
