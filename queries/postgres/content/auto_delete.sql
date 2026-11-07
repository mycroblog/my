-- SPDX-License-Identifier: AGPL-3.0-only

-- The automatic deletion of local accounts' old posts (FR-118, data-model 4.4).

-- name: GetStatusAutoDeletePolicy :one
SELECT account_id, enabled, min_age_days, keep_pinned, keep_bookmarked, keep_favourited, keep_media,
  keep_polls, keep_min_boosts, keep_min_favourites, updated_at
FROM status_auto_delete_policies WHERE account_id = $1;

-- name: ListStatusAutoDeleteKeptVisibilities :many
SELECT visibility FROM status_auto_delete_kept_visibilities WHERE account_id = $1 ORDER BY visibility;

-- name: UpsertStatusAutoDeletePolicy :exec
INSERT INTO status_auto_delete_policies (
  account_id, enabled, min_age_days, keep_pinned, keep_bookmarked, keep_favourited, keep_media,
  keep_polls, keep_min_boosts, keep_min_favourites, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (account_id) DO UPDATE SET
  enabled = excluded.enabled, min_age_days = excluded.min_age_days, keep_pinned = excluded.keep_pinned,
  keep_bookmarked = excluded.keep_bookmarked, keep_favourited = excluded.keep_favourited,
  keep_media = excluded.keep_media, keep_polls = excluded.keep_polls,
  keep_min_boosts = excluded.keep_min_boosts, keep_min_favourites = excluded.keep_min_favourites,
  updated_at = excluded.updated_at;

-- name: DeleteStatusAutoDeleteKeptVisibilities :exec
DELETE FROM status_auto_delete_kept_visibilities WHERE account_id = $1;

-- name: InsertStatusAutoDeleteKeptVisibility :exec
INSERT INTO status_auto_delete_kept_visibilities (account_id, visibility) VALUES ($1, $2);

-- ListEnabledStatusAutoDeletePolicies returns up to @batch_limit enabled policies in account order
-- after the cursor @after.
-- name: ListEnabledStatusAutoDeletePolicies :many
SELECT account_id, enabled, min_age_days, keep_pinned, keep_bookmarked, keep_favourited, keep_media,
  keep_polls, keep_min_boosts, keep_min_favourites, updated_at
FROM status_auto_delete_policies
WHERE enabled AND account_id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000')
ORDER BY account_id
LIMIT sqlc.arg(batch_limit)::integer;

-- ListStatusesToAutoDelete returns, oldest first, up to @batch_limit local statuses of
-- @account_id below @id_before that are not deleted, were published before @published_before,
-- and that none of the policy's exceptions keeps: a visibility in @kept_visibilities, a pin (one
-- of the author's newest @max_kept_pins pins when it is positive), the
-- author's bookmark or favourite, media or a poll in the current revision, or at least
-- @keep_min_boosts boosts or @keep_min_favourites favourites (0 for no threshold).
-- name: ListStatusesToAutoDelete :many
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri,
  s.conversation_uri, s.published_at
FROM statuses s
WHERE s.author_account_id = sqlc.arg(account_id)::uuid AND s.id < sqlc.arg(id_before)::uuid
  AND s.is_local AND s.published_at < sqlc.arg(published_before)::timestamptz
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
  AND NOT (s.visibility = ANY(sqlc.arg(kept_visibilities)::text[]))
  AND (NOT sqlc.arg(keep_pinned)::boolean
    OR NOT EXISTS (SELECT 1 FROM status_pins p WHERE p.status_id = s.id
      AND (sqlc.arg(max_kept_pins)::integer = 0 OR p.id IN (
        SELECT k.id FROM status_pins k WHERE k.account_id = s.author_account_id
          AND NOT EXISTS (SELECT 1 FROM status_deletions kd WHERE kd.status_id = k.status_id)
        ORDER BY k.id DESC LIMIT sqlc.arg(max_kept_pins)::integer))))
  AND (NOT sqlc.arg(keep_bookmarked)::boolean
    OR NOT EXISTS (SELECT 1 FROM bookmarks b WHERE b.account_id = s.author_account_id AND b.status_id = s.id))
  AND (NOT sqlc.arg(keep_favourited)::boolean
    OR NOT EXISTS (SELECT 1 FROM favourites f WHERE f.account_id = s.author_account_id AND f.status_id = s.id))
  AND (NOT sqlc.arg(keep_media)::boolean OR NOT EXISTS (
    SELECT 1 FROM status_heads h JOIN status_revision_attachments a ON a.revision_id = h.revision_id
    WHERE h.status_id = s.id))
  AND (NOT sqlc.arg(keep_polls)::boolean OR NOT EXISTS (
    SELECT 1 FROM status_heads h JOIN polls q ON q.revision_id = h.revision_id WHERE h.status_id = s.id))
  AND (sqlc.arg(keep_min_boosts)::integer = 0 OR NOT EXISTS (
    SELECT 1 FROM status_counters c WHERE c.status_id = s.id
      AND c.boosts_count >= sqlc.arg(keep_min_boosts)::integer))
  AND (sqlc.arg(keep_min_favourites)::integer = 0 OR NOT EXISTS (
    SELECT 1 FROM status_counters c WHERE c.status_id = s.id
      AND c.favourites_count >= sqlc.arg(keep_min_favourites)::integer))
ORDER BY s.id
LIMIT sqlc.arg(batch_limit)::integer;

-- CountStatusAutoDeletePins counts the account's pins of statuses that are not deleted.
-- name: CountStatusAutoDeletePins :one
SELECT count(*) FROM status_pins p
WHERE p.account_id = $1 AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = p.status_id);

-- name: InsertStatusAutoDeleteLimitNotice :exec
INSERT INTO status_auto_delete_limit_notices (id, account_id, max_age_days, noticed_at, enforce_from)
VALUES ($1, $2, $3, $4, $5);

-- name: ListStatusAutoDeleteLimitNotices :many
SELECT id, account_id, max_age_days, noticed_at, enforce_from FROM status_auto_delete_limit_notices
WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- name: GetStatusAutoDeleteLimit :one
SELECT account_id, notice_id, max_age_days, enforce_from FROM status_auto_delete_limits WHERE account_id = $1;

-- name: UpsertStatusAutoDeleteLimit :exec
INSERT INTO status_auto_delete_limits (account_id, notice_id, max_age_days, enforce_from) VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id) DO UPDATE SET
  notice_id = excluded.notice_id, max_age_days = excluded.max_age_days, enforce_from = excluded.enforce_from;

-- name: DeleteStatusAutoDeleteLimit :exec
DELETE FROM status_auto_delete_limits WHERE account_id = $1;

-- name: DeleteAllStatusAutoDeleteLimits :exec
DELETE FROM status_auto_delete_limits;
