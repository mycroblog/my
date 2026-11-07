-- SPDX-License-Identifier: AGPL-3.0-only

-- Scheduled deletions of accounts (data-model sections 4.2 and 8) ---------------------------------------

-- ListDueDeletions returns up to lim accounts whose deletion is scheduled at or before now,
-- earliest first.
-- name: ListDueDeletions :many
SELECT account_id, state, since, deletion_due_at FROM account_lifecycle_states
WHERE state = 'deletion_scheduled' AND deletion_due_at <= sqlc.arg(now)::timestamptz
ORDER BY deletion_due_at, account_id
LIMIT sqlc.arg(lim);

-- ListDueDeactivations returns up to lim accounts whose owner deactivated them at or before cutoff,
-- longest deactivated first (ADR-0089 section 2).
-- name: ListDueDeactivations :many
SELECT account_id, state, since, deletion_due_at FROM account_lifecycle_states
WHERE state = 'deactivated' AND since <= sqlc.arg(cutoff)::timestamptz
ORDER BY since, account_id
LIMIT sqlc.arg(lim);

-- FindLatestReactivation returns when an account was last reactivated (ADR-0089 section 8).
-- name: FindLatestReactivation :one
SELECT occurred_at FROM account_lifecycle_events
WHERE account_id = $1 AND kind = 'reactivated'
ORDER BY occurred_at DESC
LIMIT 1;

-- FindNewestRemoteDeletion returns who initiated the newest remote_deleted event of an account:
-- the actor's signed Delete (remote) or this server after a 410 (system).
-- name: FindNewestRemoteDeletion :one
SELECT initiated_by FROM account_lifecycle_events
WHERE account_id = $1 AND kind = 'remote_deleted'
ORDER BY id DESC
LIMIT 1;
