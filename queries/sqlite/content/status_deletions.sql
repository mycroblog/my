-- SPDX-License-Identifier: AGPL-3.0-only

-- The projections of many deleted statuses, each statement as the one of one status in
-- content.sql, interaction.sql, reaction.sql, and link_card.sql does for every status it names.
-- The lists are JSON arrays.

-- name: ListStatusReplyLinksOf :many
SELECT status_id, parent_status_id, parent_author_account_id FROM status_reply_links
WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT))) ORDER BY status_id;

-- name: AddManyStatusReplies :exec
UPDATE status_counters SET
  replies_count = replies_count + (SELECT CAST(value ->> '$.n' AS INTEGER) FROM json_each(CAST(sqlc.arg(deltas) AS TEXT)) WHERE value ->> '$.id' = status_counters.status_id)
WHERE status_id IN (SELECT value ->> '$.id' FROM json_each(CAST(sqlc.arg(deltas) AS TEXT)));

-- name: DeleteStatusHeadsOf :exec
DELETE FROM status_heads WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)));

-- name: DeleteStatusMentionsOf :exec
DELETE FROM status_mention_index WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)));

-- name: DeleteStatusCountersOf :exec
DELETE FROM status_counters WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)));

-- name: DeleteStatusReplyLinksOf :exec
DELETE FROM status_reply_links WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)))
  OR parent_status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)));

-- name: RemoveAccountPosts :exec
UPDATE account_post_counters SET statuses_count = statuses_count - CAST(sqlc.arg(removed) AS INTEGER), last_status_at = sqlc.arg(last_status_at)
WHERE account_id = sqlc.arg(account_id);

-- name: DeleteManyStatusReactionCounts :exec
DELETE FROM status_reaction_counts WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)));

-- name: ListLocalStatusBoostersOf :many
SELECT e.booster_account_id FROM boost_edges e JOIN boosts b ON b.id = e.boost_id
WHERE e.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT))) AND b.is_local
ORDER BY e.booster_account_id;

-- name: DeleteManyStatusBoostEdges :exec
DELETE FROM boost_edges WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)));

-- name: DeleteManyStatusLinkCards :exec
DELETE FROM status_link_cards WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)));

-- DeleteManySpentLocalPostCounters and SubtractManyLocalPosts are DeleteSpentLocalPostCounter and
-- SubtractLocalPosts of many accounts, a JSON array of {"id": account, "n": removed}.
-- name: DeleteManySpentLocalPostCounters :exec
DELETE FROM local_post_counters WHERE EXISTS (
  SELECT 1 FROM json_each(CAST(sqlc.arg(removed) AS TEXT))
  WHERE value ->> '$.id' = local_post_counters.account_id AND local_post_counters.posts_count <= CAST(value ->> '$.n' AS INTEGER));

-- name: SubtractManyLocalPosts :exec
UPDATE local_post_counters SET
  posts_count = posts_count - (SELECT CAST(value ->> '$.n' AS INTEGER) FROM json_each(CAST(sqlc.arg(removed) AS TEXT)) WHERE value ->> '$.id' = local_post_counters.account_id)
WHERE account_id IN (SELECT value ->> '$.id' FROM json_each(CAST(sqlc.arg(removed) AS TEXT)));
