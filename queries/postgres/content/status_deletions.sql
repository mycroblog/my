-- SPDX-License-Identifier: AGPL-3.0-only

-- The projections of many deleted statuses, each statement as the one of one status in
-- content.sql, interaction.sql, reaction.sql, and link_card.sql does for every status it names.

-- name: ListStatusReplyLinksOf :many
SELECT status_id, parent_status_id, parent_author_account_id FROM status_reply_links
WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: AddManyStatusReplies :exec
UPDATE status_counters c SET replies_count = c.replies_count + d.delta
FROM (SELECT unnest(sqlc.arg(status_ids)::uuid[]) AS status_id, unnest(sqlc.arg(deltas)::bigint[]) AS delta) d
WHERE c.status_id = d.status_id;

-- name: DeleteStatusHeadsOf :exec
DELETE FROM status_heads WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]);

-- name: DeleteStatusMentionsOf :exec
DELETE FROM status_mention_index WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]);

-- name: DeleteStatusCountersOf :exec
DELETE FROM status_counters WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]);

-- name: DeleteStatusReplyLinksOf :exec
DELETE FROM status_reply_links WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]) OR parent_status_id = ANY(sqlc.arg(status_ids)::uuid[]);

-- name: RemoveAccountPosts :exec
UPDATE account_post_counters SET statuses_count = statuses_count - sqlc.arg(removed)::bigint, last_status_at = sqlc.arg(last_status_at)
WHERE account_id = sqlc.arg(account_id);

-- name: DeleteManyStatusReactionCounts :exec
DELETE FROM status_reaction_counts WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]);

-- name: ListLocalStatusBoostersOf :many
SELECT e.booster_account_id FROM boost_edges e JOIN boosts b ON b.id = e.boost_id
WHERE e.status_id = ANY(sqlc.arg(status_ids)::uuid[]) AND b.is_local
ORDER BY e.booster_account_id;

-- name: DeleteManyStatusBoostEdges :exec
DELETE FROM boost_edges WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]);

-- name: DeleteManyStatusLinkCards :exec
DELETE FROM status_link_cards WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]);

-- DeleteManySpentLocalPostCounters and SubtractManyLocalPosts are DeleteSpentLocalPostCounter and
-- SubtractLocalPosts of many accounts.
-- name: DeleteManySpentLocalPostCounters :exec
DELETE FROM local_post_counters c
USING (SELECT unnest(sqlc.arg(account_ids)::uuid[]) AS account_id, unnest(sqlc.arg(removed)::bigint[]) AS removed) d
WHERE c.account_id = d.account_id AND c.posts_count <= d.removed;

-- name: SubtractManyLocalPosts :exec
UPDATE local_post_counters c SET posts_count = c.posts_count - d.removed
FROM (SELECT unnest(sqlc.arg(account_ids)::uuid[]) AS account_id, unnest(sqlc.arg(removed)::bigint[]) AS removed) d
WHERE c.account_id = d.account_id;
