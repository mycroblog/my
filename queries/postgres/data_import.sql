-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertDataImport :exec
INSERT INTO data_imports (id, user_id, account_id, kind, mode, created_at)
VALUES (@id, @user_id, @account_id, @kind, @mode, @created_at);

-- InsertDataImportItems inserts items of one import in one statement, from parallel arrays.
-- name: InsertDataImportItems :exec
INSERT INTO data_import_items (import_id, position, action, target, show_boosts, notify, hide_notifications)
SELECT sqlc.arg(import_id)::uuid, unnest(sqlc.arg(positions)::integer[]), unnest(sqlc.arg(actions)::text[]),
  unnest(sqlc.arg(targets)::text[]), unnest(sqlc.arg(show_boosts)::boolean[]), unnest(sqlc.arg(notifies)::boolean[]),
  unnest(sqlc.arg(hide_notifications)::boolean[]);

-- InsertDataImportItemResult records how an item ended; the first result of an item stands.
-- name: InsertDataImportItemResult :exec
INSERT INTO data_import_item_results (import_id, position, outcome, failure, processed_at)
VALUES (@import_id, @position, @outcome, @failure, @processed_at)
ON CONFLICT (import_id, position) DO NOTHING;

-- GetDataImportProgress returns an import with the numbers of its items, processed items, and
-- failed items.
-- name: GetDataImportProgress :one
SELECT i.id, i.user_id, i.account_id, i.kind, i.mode, i.created_at,
  (SELECT COUNT(*) FROM data_import_items t WHERE t.import_id = i.id) AS total,
  (SELECT COUNT(*) FROM data_import_item_results r WHERE r.import_id = i.id) AS processed,
  (SELECT COUNT(*) FROM data_import_item_results r WHERE r.import_id = i.id AND r.outcome = 'failed') AS failed
FROM data_imports i
WHERE i.id = sqlc.arg(id)::uuid;

-- ListDataImportProgress returns the newest imports of a user, as GetDataImportProgress.
-- name: ListDataImportProgress :many
SELECT i.id, i.user_id, i.account_id, i.kind, i.mode, i.created_at,
  (SELECT COUNT(*) FROM data_import_items t WHERE t.import_id = i.id) AS total,
  (SELECT COUNT(*) FROM data_import_item_results r WHERE r.import_id = i.id) AS processed,
  (SELECT COUNT(*) FROM data_import_item_results r WHERE r.import_id = i.id AND r.outcome = 'failed') AS failed
FROM data_imports i
WHERE i.user_id = sqlc.arg(user_id)::uuid
ORDER BY i.id DESC LIMIT @batch_limit;

-- ListPendingDataImportItems returns the first items of an import without a result.
-- name: ListPendingDataImportItems :many
SELECT t.import_id, t.position, t.action, t.target, t.show_boosts, t.notify, t.hide_notifications
FROM data_import_items t
WHERE t.import_id = sqlc.arg(import_id)::uuid
  AND NOT EXISTS (SELECT 1 FROM data_import_item_results r WHERE r.import_id = t.import_id AND r.position = t.position)
ORDER BY t.position LIMIT @batch_limit;

-- ListDataImportsCreatedBefore returns the IDs of the imports created before created_before,
-- oldest first.
-- name: ListDataImportsCreatedBefore :many
SELECT id FROM data_imports WHERE created_at < @created_before ORDER BY created_at, id LIMIT @batch_limit;

-- name: DeleteDataImportItemResults :exec
DELETE FROM data_import_item_results WHERE import_id = sqlc.arg(import_id)::uuid;

-- name: DeleteDataImportItems :exec
DELETE FROM data_import_items WHERE import_id = sqlc.arg(import_id)::uuid;

-- name: DeleteDataImport :exec
DELETE FROM data_imports WHERE id = sqlc.arg(id)::uuid;

-- name: DeleteDataImportItemFailures :exec
DELETE FROM data_import_item_failures WHERE import_id = sqlc.arg(import_id)::uuid;

-- HasUnfinishedDataImport reports whether the user has an import with an item without a result.
-- name: HasUnfinishedDataImport :one
SELECT EXISTS (
  SELECT 1 FROM data_imports i JOIN data_import_items t ON t.import_id = i.id
  WHERE i.user_id = sqlc.arg(user_id)::uuid
    AND NOT EXISTS (SELECT 1 FROM data_import_item_results r WHERE r.import_id = t.import_id AND r.position = t.position)
)::boolean AS unfinished;

-- LockDataImport locks an import against its deletion until the transaction ends.
-- name: LockDataImport :one
SELECT id FROM data_imports WHERE id = sqlc.arg(id)::uuid FOR SHARE;

-- InsertDataImportItemFailure records a failed run of an item as the attempt after its last one
-- and returns that attempt.
-- name: InsertDataImportItemFailure :one
INSERT INTO data_import_item_failures (import_id, position, attempt, failed_at)
SELECT sqlc.arg(import_id)::uuid, sqlc.arg(position)::integer, COALESCE(MAX(f.attempt), 0) + 1, sqlc.arg(failed_at)::timestamptz
FROM data_import_item_failures f
WHERE f.import_id = sqlc.arg(import_id)::uuid AND f.position = sqlc.arg(position)::integer
RETURNING attempt;
