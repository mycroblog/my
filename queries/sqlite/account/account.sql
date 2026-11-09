-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertAccount :exec
INSERT INTO accounts (id, uri, domain, actor_type, created_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertLocalAccount :exec
INSERT INTO local_accounts (account_id, user_id, username, username_normalized) VALUES (?, ?, ?, ?);

-- name: InsertProfileRevision :exec
INSERT INTO account_profile_revisions (
  id, account_id, seq, display_name, note_html, locked, bot, discoverable, indexable, ai_opt_out,
  avatar_media_id, header_media_id, revised_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertProfileField :exec
INSERT INTO account_profile_fields (revision_id, position, name, value_html) VALUES (?, ?, ?, ?);

-- name: InsertProfileRevisionEmoji :exec
INSERT INTO account_profile_revision_emojis (revision_id, emoji_id) VALUES (?, ?);

-- name: InsertAccountKey :exec
INSERT INTO account_keys (id, account_id, key_id, algorithm, public_key_pem, fingerprint, created_at) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: InsertLocalAccountPrivateKey :exec
INSERT INTO local_account_private_keys (account_key_id, private_key_enc) VALUES (?, ?);

-- name: InsertLifecycleEvent :exec
INSERT INTO account_lifecycle_events (
  id, account_id, kind, initiated_by, moderation_action_id, deletion_due_at, occurred_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpsertAccountHandle :exec
INSERT INTO account_handles (account_id, username, username_normalized, domain) VALUES (?, ?, ?, ?)
ON CONFLICT (account_id) DO UPDATE SET
  username = excluded.username,
  username_normalized = excluded.username_normalized,
  domain = excluded.domain;

-- name: DeleteAllAccountHandles :exec
DELETE FROM account_handles;

-- name: GetProfileHead :one
SELECT h.account_id, h.revision_id, r.seq
FROM account_profile_heads h JOIN account_profile_revisions r ON r.id = h.revision_id
WHERE h.account_id = ?;

-- name: UpsertProfileHead :exec
INSERT INTO account_profile_heads (account_id, revision_id) VALUES (?, ?)
ON CONFLICT (account_id) DO UPDATE SET revision_id = excluded.revision_id;

-- name: DeleteAllProfileHeads :exec
DELETE FROM account_profile_heads;

-- name: GetLifecycleState :one
SELECT account_id, state, since, deletion_due_at FROM account_lifecycle_states WHERE account_id = ?;

-- name: UpsertLifecycleState :exec
INSERT INTO account_lifecycle_states (account_id, state, since, deletion_due_at) VALUES (?, ?, ?, ?)
ON CONFLICT (account_id) DO UPDATE SET
  state = excluded.state,
  since = excluded.since,
  deletion_due_at = excluded.deletion_due_at;

-- name: DeleteAllLifecycleStates :exec
DELETE FROM account_lifecycle_states;

-- name: ListLocalAccounts :many
SELECT account_id, user_id, username, username_normalized FROM local_accounts ORDER BY account_id;

-- name: ListProfileRevisionSeqs :many
SELECT account_id, id, seq FROM account_profile_revisions ORDER BY account_id, seq;

-- name: ListLifecycleEvents :many
SELECT id, account_id, kind, initiated_by, moderation_action_id, deletion_due_at, occurred_at
FROM account_lifecycle_events ORDER BY account_id, id;

-- name: ListAccountHandles :many
SELECT account_id, username, username_normalized, domain FROM account_handles ORDER BY account_id;

-- name: ListProfileHeads :many
SELECT h.account_id, h.revision_id, r.seq
FROM account_profile_heads h JOIN account_profile_revisions r ON r.id = h.revision_id
ORDER BY h.account_id;

-- name: ListLifecycleStates :many
SELECT account_id, state, since, deletion_due_at FROM account_lifecycle_states ORDER BY account_id;

-- name: GetAccount :one
SELECT id, uri, domain, actor_type, created_at FROM accounts WHERE id = ?;

-- name: GetInstanceActor :one
SELECT id, uri, domain, actor_type, created_at FROM accounts
WHERE domain = '' AND actor_type = 'application';

-- name: GetAccountByURI :one
SELECT id, uri, domain, actor_type, created_at FROM accounts WHERE uri = ?;

-- AnyAccountOnDomain reports whether one of the accounts ids is on domain. The ids are a JSON array.
-- name: AnyAccountOnDomain :one
SELECT EXISTS (SELECT 1 FROM accounts WHERE domain = sqlc.arg(domain) AND id IN (SELECT value FROM json_each(sqlc.arg(ids)))) AS found;

-- ListAccountsOnDomain returns, in ID order, the accounts among ids that are on domain or one
-- of its subdomains. The ids are a JSON array.
-- name: ListAccountsOnDomain :many
SELECT id FROM accounts
WHERE id IN (SELECT value FROM json_each(sqlc.arg(ids)))
  AND (domain = sqlc.arg(domain)
    OR (length(domain) > length(sqlc.arg(domain)) AND substr(domain, length(domain) - length(sqlc.arg(domain))) = '.' || sqlc.arg(domain)))
ORDER BY id;

-- ListAccountIDsOnDomainAfter pages the accounts on domain or one of its subdomains in ID order
-- after the ID after ("" for the first page).
-- name: ListAccountIDsOnDomainAfter :many
SELECT id FROM accounts
WHERE id > CAST(sqlc.arg(after) AS TEXT)
  AND (domain = sqlc.arg(domain)
    OR (length(domain) > length(sqlc.arg(domain)) AND substr(domain, length(domain) - length(sqlc.arg(domain))) = '.' || sqlc.arg(domain)))
ORDER BY id
LIMIT sqlc.arg(page_limit);

-- ListExistingAccountIDs returns, in ID order, the accounts among ids that exist. The ids are a JSON array.
-- name: ListExistingAccountIDs :many
SELECT id FROM accounts WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;

-- ListLocalAccountIDsAfter pages the local accounts in ID order after the ID after ("" for the
-- first page).
-- name: ListLocalAccountIDsAfter :many
SELECT account_id FROM local_accounts WHERE account_id > CAST(sqlc.arg(after) AS TEXT) ORDER BY account_id LIMIT sqlc.arg(page_limit);

-- name: GetLocalAccount :one
SELECT account_id, user_id, username, username_normalized FROM local_accounts WHERE account_id = ?;

-- name: GetLocalAccountByUserID :one
SELECT account_id, user_id, username, username_normalized FROM local_accounts WHERE user_id = ?;

-- name: GetAccountHandleByName :one
SELECT account_id, username, username_normalized, domain FROM account_handles
WHERE username_normalized = ? AND domain = ?;

-- name: GetAccountHandle :one
SELECT account_id, username, username_normalized, domain FROM account_handles WHERE account_id = ?;

-- name: GetProfileRevision :one
SELECT id, account_id, seq, display_name, note_html, locked, bot, discoverable, indexable, ai_opt_out,
  avatar_media_id, header_media_id, revised_at
FROM account_profile_revisions WHERE id = ?;

-- name: ListProfileFields :many
SELECT revision_id, position, name, value_html FROM account_profile_fields
WHERE revision_id = ? ORDER BY position;

-- name: ListProfileRevisionEmojis :many
SELECT emoji_id FROM account_profile_revision_emojis WHERE revision_id = ? ORDER BY emoji_id;

-- name: ListAccountKeys :many
SELECT id, account_id, key_id, algorithm, public_key_pem, fingerprint, created_at FROM account_keys
WHERE account_id = ? ORDER BY id;

-- The keys of an account, as ListAccountKeys reads them, with their sealed private keys (NULL
-- for a key without one), as GetLocalAccountPrivateKey reads each.
-- name: ListAccountKeysWithPrivate :many
SELECT k.id, k.account_id, k.key_id, k.algorithm, k.public_key_pem, k.fingerprint, k.created_at, p.private_key_enc
FROM account_keys k LEFT JOIN local_account_private_keys p ON p.account_key_id = k.id
WHERE k.account_id = ? ORDER BY k.id;

-- name: GetLocalAccountPrivateKey :one
SELECT private_key_enc FROM local_account_private_keys WHERE account_key_id = ?;

-- name: CountLocalAccounts :one
SELECT COUNT(*) FROM local_accounts la
LEFT JOIN account_lifecycle_states s ON s.account_id = la.account_id
WHERE s.state IS NULL OR s.state <> 'deleted';

-- Whether any profile revision uses a media file as its avatar or header; the content module's
-- cleanup keeps such files.
-- name: IsMediaReferencedByProfiles :one
SELECT EXISTS (
  SELECT 1 FROM account_profile_revisions
  WHERE avatar_media_id = sqlc.arg(media_id) OR header_media_id = sqlc.arg(media_id)
);

-- name: IsEmojiReferencedByProfiles :one
SELECT EXISTS (SELECT 1 FROM account_profile_revision_emojis WHERE emoji_id = sqlc.arg(emoji_id));

-- name: InsertProfileRevisionSource :exec
INSERT INTO account_profile_revision_sources (revision_id, note_source) VALUES (?, ?);

-- name: InsertProfileFieldSource :exec
INSERT INTO account_profile_field_sources (revision_id, position, value_source) VALUES (?, ?, ?);

-- name: GetProfileRevisionSource :one
SELECT note_source FROM account_profile_revision_sources WHERE revision_id = ?;

-- name: ListProfileFieldSources :many
SELECT position, value_source FROM account_profile_field_sources WHERE revision_id = ? ORDER BY position;
