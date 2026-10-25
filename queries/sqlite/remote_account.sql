-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the remote actors and their keys (data-model section 4.2), owned by
-- internal/app/account.

-- name: InsertRemoteAccount :execrows
INSERT INTO remote_accounts (account_id, first_seen_at) VALUES (@account_id, @first_seen_at)
ON CONFLICT (account_id) DO NOTHING;

-- name: InsertRemoteAccountSnapshot :exec
INSERT INTO remote_account_snapshots (
  id, account_id, preferred_username, profile_url, inbox_url, shared_inbox_url, outbox_url,
  followers_url, following_url, featured_url, followers_total, following_total, statuses_total,
  document_digest, fetched_at
) VALUES (
  @id, @account_id, @preferred_username, @profile_url, @inbox_url, @shared_inbox_url, @outbox_url,
  @followers_url, @following_url, @featured_url, @followers_total, @following_total, @statuses_total,
  @document_digest, @fetched_at
);

-- GetLatestRemoteAccountSnapshot returns the newest snapshot of an actor.
-- name: GetLatestRemoteAccountSnapshot :one
SELECT id, account_id, preferred_username, profile_url, inbox_url, shared_inbox_url, outbox_url,
  followers_url, following_url, featured_url, followers_total, following_total, statuses_total,
  document_digest, fetched_at
FROM remote_account_snapshots WHERE account_id = @account_id ORDER BY id DESC LIMIT 1;

-- ListRemoteProfileURLs returns the profile page (url) of the current snapshot of each actor in
-- @account_ids that declares one, for presenting a page of accounts.
-- name: ListRemoteProfileURLs :many
SELECT h.account_id, s.profile_url FROM remote_account_heads h
JOIN remote_account_snapshots s ON s.id = h.snapshot_id
WHERE h.account_id IN (SELECT value FROM json_each(CAST(@account_ids AS TEXT))) AND s.profile_url IS NOT NULL
ORDER BY h.account_id;

-- name: UpsertRemoteAccountHead :exec
INSERT INTO remote_account_heads (
  account_id, snapshot_id, inbox_url, shared_inbox_url, last_fetched_at, consecutive_fetch_failures
) VALUES (@account_id, @snapshot_id, @inbox_url, @shared_inbox_url, @last_fetched_at, @consecutive_fetch_failures)
ON CONFLICT (account_id) DO UPDATE SET
  snapshot_id = excluded.snapshot_id,
  inbox_url = excluded.inbox_url,
  shared_inbox_url = excluded.shared_inbox_url,
  last_fetched_at = excluded.last_fetched_at,
  consecutive_fetch_failures = excluded.consecutive_fetch_failures;

-- name: GetRemoteAccountHead :one
SELECT account_id, snapshot_id, inbox_url, shared_inbox_url, last_fetched_at, consecutive_fetch_failures
FROM remote_account_heads WHERE account_id = @account_id;

-- ListStaleRemoteAccountHeads returns the actors fetched before @fetched_before, the least
-- recently fetched first, after the cursor (@after_fetched_at, @after_account_id), for the
-- periodic refresh.
-- name: ListStaleRemoteAccountHeads :many
SELECT account_id, snapshot_id, inbox_url, shared_inbox_url, last_fetched_at, consecutive_fetch_failures
FROM remote_account_heads
WHERE last_fetched_at < @fetched_before
  AND (last_fetched_at > @after_fetched_at OR (last_fetched_at = @after_fetched_at AND account_id > @after_account_id))
ORDER BY last_fetched_at, account_id LIMIT @batch_limit;

-- ListKnownSharedInboxes returns the distinct shared inboxes that the heads of remote actors not
-- deleted declare, in order, after @after, for the Delete of a local actor (data-model section 8).
-- name: ListKnownSharedInboxes :many
SELECT DISTINCT h.shared_inbox_url FROM remote_account_heads h
WHERE h.shared_inbox_url IS NOT NULL AND h.shared_inbox_url > CAST(@after AS TEXT)
  AND NOT EXISTS (
    SELECT 1 FROM account_lifecycle_states s WHERE s.account_id = h.account_id AND s.state = 'deleted'
  )
ORDER BY h.shared_inbox_url LIMIT @batch_limit;

-- CountKnownSharedInboxes counts the shared inboxes of ListKnownSharedInboxes, for the estimate of
-- the deliveries of the closure of the service (ADR-0088 section 1).
-- name: CountKnownSharedInboxes :one
SELECT COUNT(DISTINCT h.shared_inbox_url) FROM remote_account_heads h
WHERE h.shared_inbox_url IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM account_lifecycle_states s WHERE s.account_id = h.account_id AND s.state = 'deleted'
  );

-- name: InsertRemoteAccountHandleVerification :exec
INSERT INTO remote_account_handle_verifications (id, account_id, username, domain, outcome, checked_at)
VALUES (@id, @account_id, @username, @domain, @outcome, @checked_at);

-- GetLatestRemoteAccountHandleVerification returns the newest handle check of an actor.
-- name: GetLatestRemoteAccountHandleVerification :one
SELECT id, account_id, username, domain, outcome, checked_at FROM remote_account_handle_verifications
WHERE account_id = @account_id ORDER BY id DESC LIMIT 1;

-- ListLatestRemoteAccountHandleVerifications returns the newest handle check of every actor,
-- oldest first, for rebuilding account_handles.
-- name: ListLatestRemoteAccountHandleVerifications :many
SELECT v.id, v.account_id, v.username, v.domain, v.outcome, v.checked_at
FROM remote_account_handle_verifications v
WHERE v.id = (
  SELECT w.id FROM remote_account_handle_verifications w
  WHERE w.account_id = v.account_id ORDER BY w.id DESC LIMIT 1
)
ORDER BY v.id;

-- name: DeleteAccountHandle :exec
DELETE FROM account_handles WHERE account_id = @account_id;

-- DeleteOtherAccountHandle removes the handle of another account that holds the same name,
-- so that the account that verified it last keeps it.
-- name: DeleteOtherAccountHandle :exec
DELETE FROM account_handles
WHERE username_normalized = @username_normalized AND domain = @domain AND account_id <> @account_id;

-- RecordRemoteAccountFetchFailure counts a failed fetch of an actor and defers its next refresh.
-- name: RecordRemoteAccountFetchFailure :execrows
UPDATE remote_account_heads
SET consecutive_fetch_failures = consecutive_fetch_failures + 1, last_fetched_at = @last_fetched_at
WHERE account_id = @account_id;

-- ListActiveAccountKeys returns the keys of an account without a revocation, oldest first.
-- name: ListActiveAccountKeys :many
SELECT k.id, k.account_id, k.key_id, k.algorithm, k.public_key_pem, k.fingerprint, k.created_at
FROM account_keys k
WHERE k.account_id = @account_id
  AND NOT EXISTS (SELECT 1 FROM account_key_revocations r WHERE r.account_key_id = k.id)
ORDER BY k.id;

-- name: InsertRemoteProfileRevision :exec
INSERT INTO remote_profile_revisions (revision_id, snapshot_id, html_policy_version)
VALUES (@revision_id, @snapshot_id, @html_policy_version);

-- GetAccountKeyByKeyIDAndFingerprint finds a key fetched before, to tell a rotation apart.
-- name: GetAccountKeyByKeyIDAndFingerprint :one
SELECT id, account_id, key_id, algorithm, public_key_pem, fingerprint, created_at FROM account_keys
WHERE key_id = @key_id AND fingerprint = @fingerprint;

-- GetCurrentAccountKey returns the newest key of a key IRI without a revocation, the one that
-- verifies signatures.
-- name: GetCurrentAccountKey :one
SELECT k.id, k.account_id, k.key_id, k.algorithm, k.public_key_pem, k.fingerprint, k.created_at
FROM account_keys k
WHERE k.key_id = @key_id
  AND NOT EXISTS (SELECT 1 FROM account_key_revocations r WHERE r.account_key_id = k.id)
ORDER BY k.id DESC LIMIT 1;

-- name: InsertAccountKeyRevocation :execrows
INSERT INTO account_key_revocations (account_key_id, revoked_at) VALUES (@account_key_id, @revoked_at)
ON CONFLICT (account_key_id) DO NOTHING;

-- Rebuilding remote_account_heads (data-model section 9).

-- ListRemoteAccountHeads returns every head, ordered by account ID.
-- name: ListRemoteAccountHeads :many
SELECT account_id, snapshot_id, inbox_url, shared_inbox_url, last_fetched_at, consecutive_fetch_failures
FROM remote_account_heads ORDER BY account_id;

-- ListLatestRemoteAccountSnapshots returns the endpoints of the newest snapshot of every actor,
-- ordered by account ID.
-- name: ListLatestRemoteAccountSnapshots :many
SELECT s.id, s.account_id, s.inbox_url, s.shared_inbox_url, s.fetched_at
FROM remote_account_snapshots s
WHERE s.id = (
  SELECT n.id FROM remote_account_snapshots n WHERE n.account_id = s.account_id ORDER BY n.id DESC LIMIT 1
)
ORDER BY s.account_id;

-- name: DeleteAllRemoteAccountHeads :exec
DELETE FROM remote_account_heads;
