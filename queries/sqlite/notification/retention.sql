-- SPDX-License-Identifier: AGPL-3.0-only

-- Retention and purges of federated data (ADR-0006, FR-803, data-model 4.4, 4.8, 8). Each purge
-- deletes a bounded batch per statement; the jobs repeat it in short transactions. A list
-- argument is a JSON array of strings, read with json_each.

-- StatusHasNotifications reports whether a notification is about the status.
-- name: StatusHasNotifications :one
SELECT EXISTS (SELECT 1 FROM notifications WHERE status_id = ?);
