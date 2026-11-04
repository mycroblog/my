-- SPDX-License-Identifier: AGPL-3.0-only

-- Emoji reactions (resource) and their counts per emoji (projection), ADR-0051.

-- InsertReaction inserts nothing when the account already reacts to the status with the emoji.
-- name: InsertReaction :execrows
INSERT INTO reactions (id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at)
VALUES (@id, @account_id, @status_id, @emoji_key, @custom_emoji_id, @uri, @created_at)
ON CONFLICT (account_id, status_id, emoji_key) DO NOTHING;

-- name: DeleteReaction :many
DELETE FROM reactions WHERE id = @id
RETURNING id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at;

-- DeleteAccountStatusReactions removes the reactions of an account to a status, except the one with
-- the emoji except_key ('' removes them all): what the Like slot of a local account gives up
-- (ADR-0051 section 1).
-- name: DeleteAccountStatusReactions :many
DELETE FROM reactions
WHERE account_id = @account_id AND status_id = @status_id AND emoji_key <> CAST(@except_key AS TEXT)
RETURNING id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at;

-- name: GetReaction :one
SELECT id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at FROM reactions
WHERE account_id = @account_id AND status_id = @status_id AND emoji_key = @emoji_key;

-- name: GetReactionByURI :one
SELECT id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at FROM reactions WHERE uri = @uri;

-- name: ListAccountStatusReactions :many
SELECT id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at FROM reactions
WHERE account_id = @account_id AND status_id = @status_id ORDER BY id;

-- ListStatusReactions returns the reactions to a status newest first, with one emoji unless
-- emoji_key is empty: those with an ID below before ('' starts at the newest).
-- name: ListStatusReactions :many
SELECT id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at FROM reactions
WHERE status_id = @status_id
  AND (CAST(@emoji_key AS TEXT) = '' OR emoji_key = CAST(@emoji_key AS TEXT))
  AND (CAST(@before AS TEXT) = '' OR id < CAST(@before AS TEXT))
ORDER BY id DESC LIMIT @page_limit;

-- IncrementReactionCount counts one more reaction to a status that has counters (one that is
-- not deleted).
-- name: IncrementReactionCount :exec
INSERT INTO status_reaction_counts (status_id, emoji_key, custom_emoji_id, count)
SELECT c.status_id, @emoji_key, @custom_emoji_id, 1 FROM status_counters c WHERE c.status_id = @status_id
ON CONFLICT (status_id, emoji_key) DO UPDATE SET count = status_reaction_counts.count + 1;

-- name: DecrementReactionCount :exec
UPDATE status_reaction_counts SET count = count - 1
WHERE status_id = @status_id AND emoji_key = @emoji_key AND count > 1;

-- DeleteSingleReactionCount removes the count of an emoji that the last reaction left.
-- name: DeleteSingleReactionCount :exec
DELETE FROM status_reaction_counts WHERE status_id = @status_id AND emoji_key = @emoji_key AND count = 1;

-- name: DeleteStatusReactionCounts :exec
DELETE FROM status_reaction_counts WHERE status_id = @status_id;

-- name: DeleteAllReactionCounts :exec
DELETE FROM status_reaction_counts;

-- RebuildReactionCounts states the projector's rule: the reactions to statuses that are not
-- deleted, per emoji.
-- name: RebuildReactionCounts :exec
INSERT INTO status_reaction_counts (status_id, emoji_key, custom_emoji_id, count)
SELECT r.status_id, r.emoji_key, MAX(r.custom_emoji_id), COUNT(*)
FROM reactions r
WHERE NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = r.status_id)
GROUP BY r.status_id, r.emoji_key;

-- name: ListReactionCounts :many
SELECT status_id, emoji_key, custom_emoji_id, count FROM status_reaction_counts
WHERE status_id = @status_id ORDER BY count DESC, emoji_key LIMIT @page_limit;

-- name: ListAllReactionCounts :many
SELECT status_id, emoji_key, custom_emoji_id, count FROM status_reaction_counts ORDER BY status_id, emoji_key;
