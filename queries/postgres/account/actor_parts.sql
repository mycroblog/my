-- SPDX-License-Identifier: AGPL-3.0-only

-- ListActorsByIDs reads the accounts of ids with every part of their actors in one query
-- (FindActorParts), each part as the reads of one account of account.sql read it: the columns of
-- the local account, the handle, the current profile revision with its note source, and the
-- lifecycle state are NULL when the account has none; the fields of the revision (with the source
-- of each value, NULL without one), its custom emoji, and the account's keys are JSON arrays in
-- the order of those reads. A key's fingerprint is hex and its created_at microseconds since the
-- epoch. The private keys are not read.
-- name: ListActorsByIDs :many
SELECT a.id, a.uri, a.domain, a.actor_type, a.created_at,
  la.user_id AS local_user_id, la.username AS local_username, la.username_normalized AS local_username_normalized,
  h.username AS handle_username, h.username_normalized AS handle_username_normalized, h.domain AS handle_domain,
  r.id AS revision_id, r.seq, r.display_name, r.note_html, r.locked, r.bot, r.discoverable, r.indexable, r.ai_opt_out,
  r.avatar_media_id, r.header_media_id, r.revised_at, r.feed, r.follows_visibility, rs.note_source,
  COALESCE((
    SELECT json_agg(json_build_object('position', f.position, 'name', f.name, 'value_html', f.value_html, 'value_source', fs.value_source) ORDER BY f.position)
    FROM account_profile_fields f
    LEFT JOIN account_profile_field_sources fs ON fs.revision_id = f.revision_id AND fs.position = f.position
    WHERE f.revision_id = r.id
  ), '[]'::json)::text AS fields,
  COALESCE((
    SELECT json_agg(e.emoji_id ORDER BY e.emoji_id) FROM account_profile_revision_emojis e WHERE e.revision_id = r.id
  ), '[]'::json)::text AS emoji_ids,
  COALESCE((
    SELECT json_agg(json_build_object(
      'id', k.id, 'key_id', k.key_id, 'algorithm', k.algorithm, 'public_key_pem', k.public_key_pem,
      'fingerprint', encode(k.fingerprint, 'hex'), 'created_at', (extract(epoch FROM k.created_at) * 1000000)::bigint
    ) ORDER BY k.id)
    FROM account_keys k WHERE k.account_id = a.id
  ), '[]'::json)::text AS keys,
  s.state, s.since, s.deletion_due_at
FROM accounts a
LEFT JOIN local_accounts la ON la.account_id = a.id
LEFT JOIN account_handles h ON h.account_id = a.id
LEFT JOIN account_profile_heads ph ON ph.account_id = a.id
LEFT JOIN account_profile_revisions r ON r.id = ph.revision_id
LEFT JOIN account_profile_revision_sources rs ON rs.revision_id = r.id
LEFT JOIN account_lifecycle_states s ON s.account_id = a.id
WHERE a.id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY a.id;

-- name: ListAccountsByIDs :many
SELECT id, uri, domain, actor_type, created_at FROM accounts WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

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

-- The handles of the pairs of the arrays, one pair per index, as GetAccountHandleByName reads
-- one.
-- name: ListAccountHandlesByNames :many
SELECT h.account_id, h.username, h.username_normalized, h.domain FROM account_handles h
JOIN (SELECT unnest(sqlc.arg(usernames)::text[]) AS username_normalized, unnest(sqlc.arg(domains)::text[]) AS domain) n
  ON n.username_normalized = h.username_normalized AND n.domain = h.domain;
