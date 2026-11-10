-- SPDX-License-Identifier: AGPL-3.0-only

-- The featured hashtags of profiles (data-model 4.6, ADR-0006 section 17): the account's command
-- or the fetch of a remote account's featuredTags collection replaces featured_tags, and the
-- profile counts the account's public and unlisted posts of each tag in feed_tag_entries.

-- name: DeleteFeaturedTagsOf :exec
DELETE FROM featured_tags WHERE account_id = $1;

-- name: InsertFeaturedTag :exec
INSERT INTO featured_tags (account_id, tag_id, ordinal, featured_at) VALUES ($1, $2, $3, $4);

-- ListFeaturedTags returns the hashtags that an account features, in its order.
-- name: ListFeaturedTags :many
SELECT account_id, tag_id, ordinal, featured_at FROM featured_tags
WHERE account_id = $1
ORDER BY ordinal;

-- name: ListAllFeaturedTags :many
SELECT account_id, tag_id, ordinal, featured_at FROM featured_tags ORDER BY account_id, ordinal;

-- ListFeaturedTagUsage returns, for each hashtag that an account features, how many of its
-- public and unlisted posts carry the tag now and the latest time one of them was revised with
-- it. A tag that no such post carries has no row.
-- name: ListFeaturedTagUsage :many
SELECT t.tag_id, count(*) AS statuses_count, max(t.revised_at)::timestamptz AS last_status_at
FROM feed_tag_entries t
WHERE t.author_account_id = sqlc.arg(account_id)::uuid
  AND t.tag_id IN (SELECT f.tag_id FROM featured_tags f WHERE f.account_id = sqlc.arg(account_id)::uuid)
  AND t.visibility IN ('public', 'unlisted')
GROUP BY t.tag_id
ORDER BY t.tag_id;
