-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertDataExport :exec
INSERT INTO data_exports (id, user_id, requested_at) VALUES (@id, @user_id, @requested_at);

-- HasPendingDataExport reports whether the user has an export without a result.
-- name: HasPendingDataExport :one
SELECT EXISTS (
  SELECT 1 FROM data_exports e
  WHERE e.user_id = @user_id
    AND NOT EXISTS (SELECT 1 FROM data_export_results r WHERE r.export_id = e.id)
) AS pending;

-- DataExportExists reports whether an export is there.
-- name: DataExportExists :one
SELECT EXISTS (SELECT 1 FROM data_exports WHERE id = @id) AS present;

-- InsertDataExportResult records the end of a build; the first result of an export stands.
-- name: InsertDataExportResult :execrows
INSERT INTO data_export_results (export_id, outcome, blob_key, file_size, expires_at, completed_at)
VALUES (@export_id, @outcome, @blob_key, @file_size, @expires_at, @completed_at)
ON CONFLICT (export_id) DO NOTHING;

-- name: GetDataExport :one
SELECT e.id, e.user_id, e.requested_at, r.outcome, r.blob_key, r.file_size, r.expires_at, r.completed_at
FROM data_exports e LEFT JOIN data_export_results r ON r.export_id = e.id
WHERE e.id = @id;

-- ListDataExports returns the newest exports of a user with their results.
-- name: ListDataExports :many
SELECT e.id, e.user_id, e.requested_at, r.outcome, r.blob_key, r.file_size, r.expires_at, r.completed_at
FROM data_exports e LEFT JOIN data_export_results r ON r.export_id = e.id
WHERE e.user_id = @user_id
ORDER BY e.id DESC LIMIT @batch_limit;

-- ListExpiredDataExports returns the exports whose result expires at or before @expires_before.
-- name: ListExpiredDataExports :many
SELECT e.id, e.user_id, e.requested_at, r.outcome, r.blob_key, r.file_size, r.expires_at, r.completed_at
FROM data_export_results r JOIN data_exports e ON e.id = r.export_id
WHERE r.expires_at <= @expires_before
ORDER BY r.expires_at, r.export_id LIMIT @batch_limit;

-- ListStalePendingDataExports returns the IDs of the exports without a result requested before
-- @requested_before, oldest first.
-- name: ListStalePendingDataExports :many
SELECT e.id FROM data_exports e
WHERE e.requested_at < @requested_before
  AND NOT EXISTS (SELECT 1 FROM data_export_results r WHERE r.export_id = e.id)
ORDER BY e.requested_at, e.id LIMIT @batch_limit;

-- name: DeleteDataExportResult :exec
DELETE FROM data_export_results WHERE export_id = @export_id;

-- name: DeleteDataExport :exec
DELETE FROM data_exports WHERE id = @id;
