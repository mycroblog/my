-- SPDX-License-Identifier: AGPL-3.0-only

-- The parts of many actors at once, one query per part, for the pages that present many accounts
-- (FindActorParts). Each reads what the query of one account of account.sql reads.

-- name: ListAccountsByIDs :many
SELECT id, uri, domain, actor_type, created_at FROM accounts WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- name: ListLocalAccountsByIDs :many
SELECT account_id, user_id, username, username_normalized FROM local_accounts WHERE account_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY account_id;

-- name: ListAccountHandlesByIDs :many
SELECT account_id, username, username_normalized, domain FROM account_handles WHERE account_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY account_id;

-- name: ListCurrentProfileRevisions :many
SELECT r.id, r.account_id, r.seq, r.display_name, r.note_html, r.locked, r.bot, r.discoverable, r.indexable, r.ai_opt_out,
  r.avatar_media_id, r.header_media_id, r.revised_at
FROM account_profile_heads h JOIN account_profile_revisions r ON r.id = h.revision_id
WHERE h.account_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY r.account_id;

-- name: ListProfileFieldsOfRevisions :many
SELECT revision_id, position, name, value_html FROM account_profile_fields
WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, position;

-- name: ListProfileRevisionEmojisOfRevisions :many
SELECT revision_id, emoji_id FROM account_profile_revision_emojis WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, emoji_id;

-- name: ListProfileRevisionSourcesOfRevisions :many
SELECT revision_id, note_source FROM account_profile_revision_sources WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id;

-- name: ListProfileFieldSourcesOfRevisions :many
SELECT revision_id, position, value_source FROM account_profile_field_sources WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, position;

-- name: ListAccountKeysOfAccounts :many
SELECT id, account_id, key_id, algorithm, public_key_pem, fingerprint, created_at FROM account_keys
WHERE account_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY account_id, id;

-- name: ListLifecycleStatesByIDs :many
SELECT account_id, state, since, deletion_due_at FROM account_lifecycle_states WHERE account_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY account_id;

-- name: ListLocalAccountsByUsers :many
SELECT account_id, user_id, username, username_normalized FROM local_accounts WHERE user_id = ANY(sqlc.arg(user_ids)::uuid[]) ORDER BY user_id;

-- name: ListAccountMovesByIDs :many
SELECT id, account_id, target_account_id, initiated_by, activity_uri, occurred_at
FROM account_moves WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- The accounts of uris that exist, as GetAccountByURI reads one.
-- name: ListAccountsByURIs :many
SELECT id, uri, domain, actor_type, created_at FROM accounts WHERE uri = ANY(sqlc.arg(uris)::text[]);
