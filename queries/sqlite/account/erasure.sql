-- SPDX-License-Identifier: AGPL-3.0-only

-- The erasure of deleted accounts' data (data-model sections 4.2 and 8, #397). Each module
-- erases its own tables; the statements whose rows grow with the number of users or followers
-- take a bounded batch (@lim). A list argument is a JSON array of strings, read with json_each.

-- ListAccountsToErase returns, in account ID order after @after, up to @lim accounts that were
-- deleted at or before @deleted_before and are not erased yet, local or remote (remote_deleted),
-- with when it was deleted and the user of a local one.
-- name: ListAccountsToErase :many
SELECT s.account_id, s.since, la.user_id FROM account_lifecycle_states s
LEFT JOIN local_accounts la ON la.account_id = s.account_id
WHERE s.state = 'deleted' AND s.since <= CAST(sqlc.arg(deleted_before) AS INTEGER)
  AND s.account_id > CAST(sqlc.arg(after) AS TEXT)
  AND NOT EXISTS (SELECT 1 FROM account_erasures e WHERE e.account_id = s.account_id)
ORDER BY s.account_id
LIMIT sqlc.arg(lim);

-- name: EraseProfileFieldSources :exec
DELETE FROM account_profile_field_sources WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileFields :exec
DELETE FROM account_profile_fields WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileRevisionEmojis :exec
DELETE FROM account_profile_revision_emojis WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileRevisionSources :exec
DELETE FROM account_profile_revision_sources WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseRemoteProfileRevisions :exec
DELETE FROM remote_profile_revisions WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileHead :exec
DELETE FROM account_profile_heads WHERE account_id = sqlc.arg(account_id);

-- name: EraseProfileRevisions :exec
DELETE FROM account_profile_revisions WHERE account_id = sqlc.arg(account_id);

-- name: ErasePrivateKeys :exec
DELETE FROM local_account_private_keys WHERE account_key_id IN (
  SELECT k.id FROM account_keys k WHERE k.account_id = sqlc.arg(account_id));

-- name: EraseKeyRevocations :exec
DELETE FROM account_key_revocations WHERE account_key_id IN (
  SELECT k.id FROM account_keys k WHERE k.account_id = sqlc.arg(account_id));

-- name: EraseKeys :exec
DELETE FROM account_keys WHERE account_id = sqlc.arg(account_id);

-- name: EraseAliases :exec
DELETE FROM account_aliases WHERE account_id = sqlc.arg(account_id);

-- A remote account keeps no snapshot, endpoint, or handle: the handle may be someone else's later.
-- A local account has none of the first three and keeps its handle, so that its username is not
-- used again.

-- name: EraseRemoteAccountSnapshots :exec
DELETE FROM remote_account_snapshots WHERE account_id = sqlc.arg(account_id);

-- name: EraseRemoteAccountHead :exec
DELETE FROM remote_account_heads WHERE account_id = sqlc.arg(account_id);

-- name: EraseRemoteAccountHandleVerifications :exec
DELETE FROM remote_account_handle_verifications WHERE account_id = sqlc.arg(account_id);

-- name: EraseRemoteAccountHandle :exec
DELETE FROM account_handles WHERE account_handles.account_id = sqlc.arg(account_id)
  AND EXISTS (SELECT 1 FROM remote_accounts r WHERE r.account_id = sqlc.arg(account_id));

-- name: InsertAccountErasure :exec
INSERT INTO account_erasures (account_id, erased_at) VALUES (sqlc.arg(account_id), sqlc.arg(erased_at))
ON CONFLICT (account_id) DO NOTHING;

-- name: AccountErased :one
SELECT EXISTS (SELECT 1 FROM account_erasures WHERE account_id = sqlc.arg(account_id));
