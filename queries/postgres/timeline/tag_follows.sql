-- SPDX-License-Identifier: AGPL-3.0-only

-- The followed hashtags (data-model 4.6, 6.1, ADR-0013 sections 24 to 28): the account's
-- commands write feed_tag_follows, and the hashtag branch of the home timeline reads the posts of
-- the followed tags from feed_tag_entries.

-- name: InsertFeedTagFollow :execrows
INSERT INTO feed_tag_follows (account_id, tag_id, followed_at) VALUES ($1, $2, $3)
ON CONFLICT (account_id, tag_id) DO NOTHING;

-- name: DeleteFeedTagFollow :exec
DELETE FROM feed_tag_follows WHERE account_id = $1 AND tag_id = $2;

-- name: CountFeedTagFollows :one
SELECT count(*) FROM feed_tag_follows WHERE account_id = $1;

-- name: FeedTagFollowExists :one
SELECT EXISTS (SELECT 1 FROM feed_tag_follows WHERE account_id = $1 AND tag_id = $2);

-- ListFeedTagFollows returns the hashtags that an account follows, the latest followed first.
-- name: ListFeedTagFollows :many
SELECT account_id, tag_id, followed_at FROM feed_tag_follows
WHERE account_id = $1
ORDER BY followed_at DESC, tag_id DESC;

-- name: ListAllFeedTagFollows :many
SELECT account_id, tag_id, followed_at FROM feed_tag_follows ORDER BY account_id, tag_id;

-- ListHomeTagEntries is the hashtag branch of the home timeline (FR-201, ADR-0013 sections 25 and
-- 26): the public and local posts with a hashtag that the viewer follows, by authors other than
-- the viewer and the accounts the viewer follows (whose posts ListHomeEntries lists), within the
-- read window, each once with the followed tags it carries (tag_ids). It leaves
-- out what the hashtag timeline leaves out (the authors and domains kept from discovery or
-- hidden, the authors the viewer blocks, is blocked by, or mutes, the viewer's muted domains),
-- and the posts in other languages than the viewer's timeline languages.
-- name: ListHomeTagEntries :many
SELECT e.id, e.kind, e.status_id, e.author_account_id, e.author_domain, e.original_author_account_id,
  e.original_author_domain, e.visibility, e.is_local, e.language, e.revision_seq, e.is_reply,
  array_agg(t.tag_id ORDER BY t.tag_id)::uuid[] AS tag_ids
FROM feed_tag_entries t
JOIN feed_entries e ON e.id = t.entry_id
WHERE t.tag_id IN (SELECT ft.tag_id FROM feed_tag_follows ft WHERE ft.account_id = sqlc.arg(viewer))
  AND t.visibility IN ('public', 'local')
  AND (sqlc.narg(before)::uuid IS NULL OR t.entry_id < sqlc.narg(before)::uuid)
  AND t.entry_id >= COALESCE(sqlc.narg(window_floor)::uuid, '00000000-0000-0000-0000-000000000000')
  AND e.kind = 'post'
  AND t.author_account_id <> sqlc.arg(viewer)
  AND NOT EXISTS (
    SELECT 1 FROM feed_follow_edges f
    WHERE f.follower_account_id = sqlc.arg(viewer) AND f.followee_account_id = t.author_account_id)
  AND NOT EXISTS (
    SELECT 1 FROM feed_author_restrictions r
    WHERE r.account_id = t.author_account_id
      AND r.restriction IN ('public_discovery', 'visibility_ceiling_private', 'profile_view')
      AND (r.effective_until IS NULL OR r.effective_until > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_restrictions dr
    WHERE dr.restriction IN ('public_discovery', 'profile_view')
      AND (dr.effective_until IS NULL OR dr.effective_until > sqlc.arg(now)::timestamptz)
      AND (dr.domain = e.author_domain OR right(e.author_domain, length(dr.domain) + 1) = '.' || dr.domain))
  AND NOT EXISTS (
    SELECT 1 FROM feed_exclusions x
    WHERE x.viewer_account_id = sqlc.arg(viewer) AND x.target_account_id = t.author_account_id
      AND (x.expires_at IS NULL OR x.expires_at > sqlc.arg(now)::timestamptz))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_mutes dm
    WHERE dm.viewer_account_id = sqlc.arg(viewer)
      AND (dm.domain = e.author_domain OR right(e.author_domain, length(dm.domain) + 1) = '.' || dm.domain))
  AND (sqlc.arg(all_languages)::boolean OR e.language = 'und' OR e.language = ANY(sqlc.arg(languages)::text[]))
GROUP BY e.id
ORDER BY e.id DESC
LIMIT sqlc.arg(page_limit);

-- ListFeedTagFollowersAmong returns the accounts among account_ids that follow a hashtag of the
-- post entry_id when its visibility is public or local, for the home timeline streams.
-- name: ListFeedTagFollowersAmong :many
SELECT DISTINCT f.account_id FROM feed_tag_entries t
JOIN feed_tag_follows f ON f.tag_id = t.tag_id
WHERE t.entry_id = sqlc.arg(entry_id) AND t.visibility IN ('public', 'local')
  AND f.account_id = ANY(sqlc.arg(account_ids)::uuid[])
ORDER BY f.account_id;
