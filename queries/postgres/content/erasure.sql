-- SPDX-License-Identifier: AGPL-3.0-only

-- The erasure of deleted accounts' data (data-model sections 4.2 and 8, #397). Each module
-- erases its own tables; the statements whose rows grow with the number of users or followers
-- take a bounded batch (@lim). A list argument is an array of strings.

-- content: the account's statuses and its interactions --------------------------------------------

-- ListAuthorStatusesToErase returns, in ID order after @after, up to @lim statuses of the author
-- with whether each is deleted already.
-- name: ListAuthorStatusesToErase :many
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri, s.published_at,
  EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id) AS deleted
FROM statuses s
WHERE s.author_account_id = sqlc.arg(author_account_id) AND s.id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000')
ORDER BY s.id
LIMIT sqlc.arg(lim);

-- The EraseOfStatuses* statements delete what refers to the statuses @ids beyond what
-- PurgeStatuses deletes: their local rows, every boost of them, and the favourites, reactions
-- (with their counts), bookmarks, and pins of them.

-- name: EraseLocalStatusesOfStatuses :exec
DELETE FROM local_statuses WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseBoostEdgesOfStatuses :exec
DELETE FROM boost_edges WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseBoostRetractionsOfStatuses :exec
DELETE FROM boost_retractions WHERE boost_id IN (
  SELECT b.id FROM boosts b WHERE b.status_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: EraseBoostsOfStatuses :exec
DELETE FROM boosts WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFavouritesOfStatuses :exec
DELETE FROM favourites WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseReactionsOfStatuses :exec
DELETE FROM reactions WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseReactionCountsOfStatuses :exec
DELETE FROM status_reaction_counts WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseBookmarksOfStatuses :exec
DELETE FROM bookmarks WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ErasePinsOfStatuses :exec
DELETE FROM status_pins WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseCountersOfStatuses :exec
DELETE FROM status_counters WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- The account's favourites: the counts of the statuses go down first, for the same batch.

-- name: UncountFavouritesBy :exec
UPDATE status_counters SET favourites_count = favourites_count - 1
WHERE favourites_count > 0 AND status_id IN (
  SELECT f.status_id FROM favourites f WHERE f.account_id = sqlc.arg(account_id)
  ORDER BY f.status_id LIMIT sqlc.arg(lim));

-- name: EraseFavouritesBy :execrows
DELETE FROM favourites WHERE favourites.account_id = sqlc.arg(account_id) AND favourites.status_id IN (
  SELECT f.status_id FROM favourites f WHERE f.account_id = sqlc.arg(account_id)
  ORDER BY f.status_id LIMIT sqlc.arg(lim));

-- The account's reactions, the oldest lim of them: the counts of their emoji go down first (a
-- count of one goes away), for the same batch.

-- name: DeleteSingleReactionCountsBy :exec
DELETE FROM status_reaction_counts WHERE count = 1 AND EXISTS (
  SELECT 1 FROM (SELECT r.status_id, r.emoji_key FROM reactions r WHERE r.account_id = sqlc.arg(account_id)
    ORDER BY r.id LIMIT sqlc.arg(lim)) x
  WHERE x.status_id = status_reaction_counts.status_id AND x.emoji_key = status_reaction_counts.emoji_key);

-- name: UncountReactionsBy :exec
UPDATE status_reaction_counts SET count = count - 1 WHERE EXISTS (
  SELECT 1 FROM (SELECT r.status_id, r.emoji_key FROM reactions r WHERE r.account_id = sqlc.arg(account_id)
    ORDER BY r.id LIMIT sqlc.arg(lim)) x
  WHERE x.status_id = status_reaction_counts.status_id AND x.emoji_key = status_reaction_counts.emoji_key);

-- name: EraseReactionsBy :execrows
DELETE FROM reactions WHERE reactions.id IN (
  SELECT r.id FROM reactions r WHERE r.account_id = sqlc.arg(account_id) ORDER BY r.id LIMIT sqlc.arg(lim));

-- name: EraseBookmarksBy :execrows
DELETE FROM bookmarks WHERE bookmarks.account_id = sqlc.arg(account_id) AND bookmarks.status_id IN (
  SELECT b.status_id FROM bookmarks b WHERE b.account_id = sqlc.arg(account_id)
  ORDER BY b.status_id LIMIT sqlc.arg(lim));

-- The account's votes in polls, the oldest lim of them.

-- name: ErasePollVotesBy :execrows
DELETE FROM poll_votes WHERE poll_votes.id IN (
  SELECT v.id FROM poll_votes v WHERE v.account_id = sqlc.arg(account_id) ORDER BY v.id LIMIT sqlc.arg(lim));

-- The remote media files of the account that no attachment and no custom emoji uses, up to lim.
-- A status that the moderation records keep keeps its attachments until a later run.

-- name: ListRemoteMediaToErase :many
SELECT r.media_id FROM remote_media r
WHERE r.owner_account_id = sqlc.arg(account_id)
  AND NOT EXISTS (SELECT 1 FROM status_revision_attachments a WHERE a.media_id = r.media_id)
  AND NOT EXISTS (SELECT 1 FROM custom_emoji_images c WHERE c.media_id = r.media_id)
ORDER BY r.media_id LIMIT sqlc.arg(lim);

-- name: ErasePinsBy :execrows
DELETE FROM status_pins WHERE account_id = sqlc.arg(account_id);

-- The account's automatic deletion policy (FR-118) goes with its kept visibilities.

-- name: EraseAutoDeleteKeptVisibilitiesBy :exec
DELETE FROM status_auto_delete_kept_visibilities WHERE account_id = sqlc.arg(account_id);

-- name: EraseAutoDeletePolicyBy :exec
DELETE FROM status_auto_delete_policies WHERE account_id = sqlc.arg(account_id);

-- name: EraseAutoDeleteLimitBy :exec
DELETE FROM status_auto_delete_limits WHERE account_id = sqlc.arg(account_id);

-- name: EraseAutoDeleteLimitNoticesBy :exec
DELETE FROM status_auto_delete_limit_notices WHERE account_id = sqlc.arg(account_id);

-- The account's boosts in force: the counts go down with their edges, then every boost of the
-- account goes with its retraction.

-- name: UncountBoostsBy :exec
UPDATE status_counters SET boosts_count = boosts_count - 1
WHERE boosts_count > 0 AND status_id IN (
  SELECT e.status_id FROM boost_edges e WHERE e.booster_account_id = sqlc.arg(account_id)
  ORDER BY e.status_id LIMIT sqlc.arg(lim));

-- name: EraseBoostEdgesBy :execrows
DELETE FROM boost_edges WHERE boost_edges.booster_account_id = sqlc.arg(account_id) AND boost_edges.status_id IN (
  SELECT e.status_id FROM boost_edges e WHERE e.booster_account_id = sqlc.arg(account_id)
  ORDER BY e.status_id LIMIT sqlc.arg(lim));

-- name: EraseBoostRetractionsBy :exec
DELETE FROM boost_retractions WHERE boost_id IN (
  SELECT b.id FROM boosts b WHERE b.booster_account_id = sqlc.arg(account_id)
  ORDER BY b.id LIMIT sqlc.arg(lim));

-- name: EraseBoostsBy :execrows
DELETE FROM boosts WHERE id IN (
  SELECT b.id FROM boosts b WHERE b.booster_account_id = sqlc.arg(account_id)
  ORDER BY b.id LIMIT sqlc.arg(lim));
