-- SPDX-License-Identifier: AGPL-3.0-only

-- Retention and purges of federated data (ADR-0006, FR-803, data-model 4.4, 4.8, 8). Each purge
-- deletes a bounded batch per statement; the jobs repeat it in short transactions.

-- StatusHasNotifications reports whether a notification is about the status.
-- name: StatusHasNotifications :one
SELECT EXISTS (SELECT 1 FROM notifications WHERE status_id = $1);

-- NotifiedStatusesAmong returns the statuses among status_ids that a notification is about, as
-- StatusHasNotifications tells of one.
-- name: NotifiedStatusesAmong :many
SELECT DISTINCT status_id FROM notifications WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;
