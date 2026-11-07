-- SPDX-License-Identifier: AGPL-3.0-only

-- Retention and purges of federated data (ADR-0006, FR-803, data-model 4.4, 4.8, 8). Each purge
-- deletes a bounded batch per statement; the jobs repeat it in short transactions. A list
-- argument is a JSON array of strings, read with json_each.

-- ListRemoteAccountsWithPastProfiles returns, in ID order after @after, the remote accounts that
-- keep a profile revision other than the current one, or a snapshot that neither the head nor a
-- remaining profile revision points to.
-- name: ListRemoteAccountsWithPastProfiles :many
SELECT ra.account_id FROM remote_accounts ra
WHERE ra.account_id > CAST(sqlc.arg(after) AS TEXT)
  AND (EXISTS (
      SELECT 1 FROM account_profile_revisions r
      JOIN account_profile_heads h ON h.account_id = r.account_id
      WHERE r.account_id = ra.account_id AND r.id <> h.revision_id)
    OR EXISTS (
      SELECT 1 FROM remote_account_snapshots s
      JOIN remote_account_heads h ON h.account_id = s.account_id
      WHERE s.account_id = ra.account_id AND s.id <> h.snapshot_id
        AND NOT EXISTS (
          SELECT 1 FROM remote_profile_revisions pr
          JOIN account_profile_heads ph ON ph.revision_id = pr.revision_id
          WHERE ph.account_id = ra.account_id AND pr.snapshot_id = s.id)))
ORDER BY ra.account_id
LIMIT sqlc.arg(batch_limit);

-- The PrunePast* statements delete the profile revisions of a remote account other than its
-- current one, with their children, and then its snapshots that neither the head nor the
-- remaining revision points to. Without a head, the comparison with the missing head is NULL and
-- nothing is deleted.

-- name: PrunePastProfileFields :exec
DELETE FROM account_profile_fields WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r
  WHERE r.account_id = sqlc.arg(account_id)
    AND r.id <> (SELECT h.revision_id FROM account_profile_heads h WHERE h.account_id = sqlc.arg(account_id)));

-- name: PrunePastProfileRevisionEmojis :exec
DELETE FROM account_profile_revision_emojis WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r
  WHERE r.account_id = sqlc.arg(account_id)
    AND r.id <> (SELECT h.revision_id FROM account_profile_heads h WHERE h.account_id = sqlc.arg(account_id)));

-- name: PrunePastRemoteProfileRevisions :exec
DELETE FROM remote_profile_revisions WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r
  WHERE r.account_id = sqlc.arg(account_id)
    AND r.id <> (SELECT h.revision_id FROM account_profile_heads h WHERE h.account_id = sqlc.arg(account_id)));

-- name: PrunePastProfileRevisions :exec
DELETE FROM account_profile_revisions
WHERE account_profile_revisions.account_id = sqlc.arg(account_id)
  AND account_profile_revisions.id <> (SELECT h.revision_id FROM account_profile_heads h WHERE h.account_id = sqlc.arg(account_id));

-- name: PrunePastRemoteSnapshots :exec
DELETE FROM remote_account_snapshots
WHERE remote_account_snapshots.account_id = sqlc.arg(account_id)
  AND remote_account_snapshots.id <> (SELECT h.snapshot_id FROM remote_account_heads h WHERE h.account_id = sqlc.arg(account_id))
  AND remote_account_snapshots.id NOT IN (
    SELECT pr.snapshot_id FROM remote_profile_revisions pr
    JOIN account_profile_revisions r ON r.id = pr.revision_id
    WHERE r.account_id = sqlc.arg(account_id));

-- DeleteHandleVerificationsBefore removes at most @batch_limit handle checks made before
-- @checked_before, except the newest of each account, which rebuilds account_handles. @id_before
-- (the UUIDv7 floor of @checked_before) bounds the scan of the primary key.
-- name: DeleteHandleVerificationsBefore :execrows
DELETE FROM remote_account_handle_verifications
WHERE id IN (
  SELECT v.id FROM remote_account_handle_verifications v
  WHERE v.id < @id_before AND v.checked_at < @checked_before
    AND EXISTS (SELECT 1 FROM remote_account_handle_verifications n WHERE n.account_id = v.account_id AND n.id > v.id)
  ORDER BY v.id LIMIT @batch_limit
);
