-- SPDX-License-Identifier: AGPL-3.0-only
-- timeline: feeds (data-model 4.6, 6). Every query reads only the feed_* tables.

-- Projector writes ------------------------------------------------------------------------------

-- name: InsertFeedEntry :execrows
INSERT INTO feed_entries (
  id, kind, status_id, author_account_id, author_domain, original_author_account_id,
  original_author_domain, visibility, is_local, language, revision_seq, is_reply
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetFeedEntry :one
SELECT id, kind, status_id, author_account_id, author_domain, original_author_account_id,
  original_author_domain, visibility, is_local, language, revision_seq, is_reply
FROM feed_entries WHERE id = ?;

-- UpdateFeedEntriesRevision moves the entries of a status (the post and its boosts) to a
-- newer revision; an older or repeated revision changes nothing.
-- name: UpdateFeedEntriesRevision :execrows
UPDATE feed_entries SET language = sqlc.arg(language), revision_seq = sqlc.arg(revision_seq)
WHERE status_id = sqlc.arg(status_id) AND revision_seq < sqlc.arg(revision_seq);

-- name: DeleteFeedEntry :exec
DELETE FROM feed_entries WHERE id = ?;

-- name: DeleteFeedEntriesOfStatus :exec
DELETE FROM feed_entries WHERE status_id = ?;

-- name: InsertFeedDirectRecipient :exec
INSERT INTO feed_direct_recipients (entry_id, recipient_account_id) VALUES (?, ?)
ON CONFLICT DO NOTHING;

-- name: DeleteFeedDirectRecipients :exec
DELETE FROM feed_direct_recipients WHERE entry_id = ?;

-- name: InsertFeedTagEntry :exec
INSERT INTO feed_tag_entries (tag_id, entry_id, author_account_id, visibility, is_local, revised_at) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: DeleteFeedTagEntries :exec
DELETE FROM feed_tag_entries WHERE entry_id = ?;

-- name: UpsertFeedFollowEdge :exec
INSERT INTO feed_follow_edges (follower_account_id, followee_account_id, show_boosts) VALUES (?, ?, ?)
ON CONFLICT (follower_account_id, followee_account_id) DO UPDATE SET show_boosts = excluded.show_boosts;

-- name: DeleteFeedFollowEdge :exec
DELETE FROM feed_follow_edges WHERE follower_account_id = ? AND followee_account_id = ?;

-- name: UpsertFeedExclusion :exec
INSERT INTO feed_exclusions (viewer_account_id, target_account_id, reason, expires_at) VALUES (?, ?, ?, ?)
ON CONFLICT (viewer_account_id, target_account_id, reason) DO UPDATE SET expires_at = excluded.expires_at;

-- name: DeleteFeedExclusion :exec
DELETE FROM feed_exclusions WHERE viewer_account_id = ? AND target_account_id = ? AND reason = ?;

-- name: InsertFeedDomainMute :exec
INSERT INTO feed_domain_mutes (viewer_account_id, domain) VALUES (?, ?)
ON CONFLICT (viewer_account_id, domain) DO NOTHING;

-- name: DeleteFeedDomainMute :exec
DELETE FROM feed_domain_mutes WHERE viewer_account_id = ? AND domain = ?;

-- name: UpsertFeedAuthorRestriction :exec
INSERT INTO feed_author_restrictions (account_id, restriction, effective_until) VALUES (?, ?, ?)
ON CONFLICT (account_id, restriction) DO UPDATE SET effective_until = excluded.effective_until;

-- name: DeleteFeedAuthorRestriction :exec
DELETE FROM feed_author_restrictions WHERE account_id = ? AND restriction = ?;

-- name: UpsertFeedDomainRestriction :exec
INSERT INTO feed_domain_restrictions (domain, restriction, effective_until) VALUES (?, ?, ?)
ON CONFLICT (domain, restriction) DO UPDATE SET effective_until = excluded.effective_until;

-- name: DeleteFeedDomainRestriction :exec
DELETE FROM feed_domain_restrictions WHERE domain = ? AND restriction = ?;

-- Rebuild ---------------------------------------------------------------------------------------

-- name: DeleteAllFeedEntries :exec
DELETE FROM feed_entries;

-- name: DeleteAllFeedDirectRecipients :exec
DELETE FROM feed_direct_recipients;

-- name: DeleteAllFeedTagEntries :exec
DELETE FROM feed_tag_entries;

-- name: DeleteAllTagUsage :exec
DELETE FROM tag_usage;

-- name: DeleteAllFeedFollowEdges :exec
DELETE FROM feed_follow_edges;

-- name: DeleteAllFeedExclusions :exec
DELETE FROM feed_exclusions;

-- name: DeleteAllFeedDomainMutes :exec
DELETE FROM feed_domain_mutes;

-- Projection contents, for verifying the projections -----------------------------------------

-- name: ListAllFeedEntries :many
SELECT id, kind, status_id, author_account_id, author_domain, original_author_account_id,
  original_author_domain, visibility, is_local, language, revision_seq, is_reply
FROM feed_entries ORDER BY id;

-- name: ListAllFeedDirectRecipients :many
SELECT entry_id, recipient_account_id FROM feed_direct_recipients ORDER BY entry_id, recipient_account_id;

-- name: ListAllFeedTagEntries :many
SELECT tag_id, entry_id, author_account_id, visibility, is_local, revised_at FROM feed_tag_entries ORDER BY tag_id, entry_id;

-- name: ListAllTagUsage :many
SELECT tag_id, statuses_count, last_used_at FROM tag_usage ORDER BY tag_id;

-- name: ListAllFeedFollowEdges :many
SELECT follower_account_id, followee_account_id, show_boosts FROM feed_follow_edges
ORDER BY follower_account_id, followee_account_id;

-- name: ListAllFeedExclusions :many
SELECT viewer_account_id, target_account_id, reason, expires_at FROM feed_exclusions
ORDER BY viewer_account_id, target_account_id, reason;

-- name: ListAllFeedDomainMutes :many
SELECT viewer_account_id, domain FROM feed_domain_mutes ORDER BY viewer_account_id, domain;

-- Timelines (data-model 6) ----------------------------------------------------------------------
-- A list argument is a JSON array of strings, read with json_each: the SQLite engine of sqlc
-- expands sqlc.slice into plain ? placeholders, which SQLite numbers after the largest ?N, so a
-- slice followed by another argument would bind the wrong values.
-- before = '' starts at the newest entry. A language filter applies when all_languages is 0:
-- entries in an undetermined language ('und') always pass. A restriction or mute applies while
-- its end is NULL or after now. A domain restriction also covers the subdomains of its domain.

-- ListHomeEntries is the home timeline (FR-201) of a viewer: the posts of the viewer and of the
-- accounts the viewer follows, their boosts unless the follow hides them, and the viewer's own
-- boosts, within the read window (id >= window_floor).
-- name: ListHomeEntries :many
SELECT e.id, e.kind, e.status_id, e.author_account_id, e.author_domain, e.original_author_account_id,
  e.original_author_domain, e.visibility, e.is_local, e.language, e.revision_seq, e.is_reply
FROM feed_entries e
WHERE e.author_account_id IN (
    SELECT f.followee_account_id FROM feed_follow_edges f WHERE f.follower_account_id = sqlc.arg(viewer)
    UNION ALL SELECT CAST(sqlc.arg(viewer) AS TEXT))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR e.id < CAST(sqlc.arg(before) AS TEXT))
  AND e.id >= CAST(sqlc.arg(window_floor) AS TEXT)
  AND (e.kind = 'post' OR e.author_account_id = sqlc.arg(viewer) OR EXISTS (
    SELECT 1 FROM feed_follow_edges f
    WHERE f.follower_account_id = sqlc.arg(viewer) AND f.followee_account_id = e.author_account_id AND f.show_boosts = 1))
  AND (e.visibility <> 'direct' OR e.author_account_id = sqlc.arg(viewer) OR EXISTS (
    SELECT 1 FROM feed_direct_recipients d WHERE d.recipient_account_id = sqlc.arg(viewer) AND d.entry_id = e.id))
  AND NOT EXISTS (
    SELECT 1 FROM feed_exclusions x
    WHERE x.viewer_account_id = sqlc.arg(viewer)
      AND x.target_account_id IN (e.author_account_id, e.original_author_account_id)
      AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_author_restrictions r
    WHERE r.account_id IN (e.author_account_id, e.original_author_account_id) AND r.restriction = 'profile_view'
      AND (r.effective_until IS NULL OR r.effective_until > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_restrictions dr
    WHERE dr.restriction = 'profile_view'
      AND (dr.effective_until IS NULL OR dr.effective_until > CAST(sqlc.arg(now) AS INTEGER))
      AND (dr.domain IN (e.author_domain, e.original_author_domain)
        OR substr(e.author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain
        OR substr(e.original_author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_mutes dm
    WHERE dm.viewer_account_id = sqlc.arg(viewer) AND e.kind = 'boost' AND e.author_account_id <> sqlc.arg(viewer)
      AND (dm.domain = e.original_author_domain OR substr(e.original_author_domain, -(length(dm.domain) + 1)) = '.' || dm.domain))
  AND (CAST(sqlc.arg(all_languages) AS INTEGER) = 1 OR e.language = 'und' OR e.language IN (SELECT value FROM json_each(CAST(sqlc.arg(languages) AS TEXT))))
ORDER BY e.id DESC
LIMIT sqlc.arg(page_limit);

-- ListLocalEntries is the local timeline (FR-202): the public and local posts of local accounts
-- (public only when include_local is 0, for visitors that are not logged in), except those of
-- authors kept from discovery and those the viewer ('' for none) blocks, is blocked by, or mutes.
-- name: ListLocalEntries :many
SELECT e.id, e.kind, e.status_id, e.author_account_id, e.author_domain, e.original_author_account_id,
  e.original_author_domain, e.visibility, e.is_local, e.language, e.revision_seq, e.is_reply
FROM feed_entries e
WHERE e.is_local = 1 AND e.kind = 'post' AND e.visibility IN ('public', 'local')
  AND (CAST(sqlc.arg(include_local) AS INTEGER) = 1 OR e.visibility = 'public')
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR e.id < CAST(sqlc.arg(before) AS TEXT))
  AND NOT EXISTS (
    SELECT 1 FROM feed_author_restrictions r
    WHERE r.account_id = e.author_account_id
      AND r.restriction IN ('public_discovery', 'visibility_ceiling_private', 'profile_view')
      AND (r.effective_until IS NULL OR r.effective_until > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_exclusions x
    WHERE x.viewer_account_id = sqlc.arg(viewer) AND x.target_account_id = e.author_account_id
      AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
  AND (CAST(sqlc.arg(all_languages) AS INTEGER) = 1 OR e.language = 'und' OR e.language IN (SELECT value FROM json_each(CAST(sqlc.arg(languages) AS TEXT))))
ORDER BY e.id DESC
LIMIT sqlc.arg(page_limit);

-- ListPublicEntries is the federated timeline (FR-202): the public posts of local and remote
-- accounts, except those of authors and domains kept from discovery and those the viewer ('' for
-- none) blocks, is blocked by, or mutes.
-- name: ListPublicEntries :many
SELECT e.id, e.kind, e.status_id, e.author_account_id, e.author_domain, e.original_author_account_id,
  e.original_author_domain, e.visibility, e.is_local, e.language, e.revision_seq, e.is_reply
FROM feed_entries e
WHERE e.kind = 'post' AND e.visibility = 'public'
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR e.id < CAST(sqlc.arg(before) AS TEXT))
  AND NOT EXISTS (
    SELECT 1 FROM feed_author_restrictions r
    WHERE r.account_id = e.author_account_id
      AND r.restriction IN ('public_discovery', 'visibility_ceiling_private', 'profile_view')
      AND (r.effective_until IS NULL OR r.effective_until > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_restrictions dr
    WHERE dr.restriction IN ('public_discovery', 'visibility_ceiling_private', 'profile_view')
      AND (dr.effective_until IS NULL OR dr.effective_until > CAST(sqlc.arg(now) AS INTEGER))
      AND (dr.domain = e.author_domain OR substr(e.author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain))
  AND NOT EXISTS (
    SELECT 1 FROM feed_exclusions x
    WHERE x.viewer_account_id = sqlc.arg(viewer) AND x.target_account_id = e.author_account_id
      AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_mutes dm
    WHERE dm.viewer_account_id = sqlc.arg(viewer)
      AND (dm.domain = e.author_domain OR substr(e.author_domain, -(length(dm.domain) + 1)) = '.' || dm.domain))
  AND (CAST(sqlc.arg(all_languages) AS INTEGER) = 1 OR e.language = 'und' OR e.language IN (SELECT value FROM json_each(CAST(sqlc.arg(languages) AS TEXT))))
ORDER BY e.id DESC
LIMIT sqlc.arg(page_limit);

-- ListTagEntries is the timeline of a hashtag (FR-203): the public and local posts with the tag
-- (public only when include_local is 0), with the exclusions of ListLocalEntries and of the
-- author's domain.
-- name: ListTagEntries :many
SELECT e.id, e.kind, e.status_id, e.author_account_id, e.author_domain, e.original_author_account_id,
  e.original_author_domain, e.visibility, e.is_local, e.language, e.revision_seq, e.is_reply
FROM feed_tag_entries t
JOIN feed_entries e ON e.id = t.entry_id
WHERE t.tag_id = sqlc.arg(tag_id) AND t.visibility IN ('public', 'local')
  AND (CAST(sqlc.arg(include_local) AS INTEGER) = 1 OR t.visibility = 'public')
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR t.entry_id < CAST(sqlc.arg(before) AS TEXT))
  AND NOT EXISTS (
    SELECT 1 FROM feed_author_restrictions r
    WHERE r.account_id = t.author_account_id
      AND r.restriction IN ('public_discovery', 'visibility_ceiling_private', 'profile_view')
      AND (r.effective_until IS NULL OR r.effective_until > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_restrictions dr
    WHERE dr.restriction IN ('public_discovery', 'visibility_ceiling_private', 'profile_view')
      AND (dr.effective_until IS NULL OR dr.effective_until > CAST(sqlc.arg(now) AS INTEGER))
      AND (dr.domain = e.author_domain OR substr(e.author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain))
  AND NOT EXISTS (
    SELECT 1 FROM feed_exclusions x
    WHERE x.viewer_account_id = sqlc.arg(viewer) AND x.target_account_id = t.author_account_id
      AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_mutes dm
    WHERE dm.viewer_account_id = sqlc.arg(viewer)
      AND (dm.domain = e.author_domain OR substr(e.author_domain, -(length(dm.domain) + 1)) = '.' || dm.domain))
  AND (CAST(sqlc.arg(all_languages) AS INTEGER) = 1 OR e.language = 'und' OR e.language IN (SELECT value FROM json_each(CAST(sqlc.arg(languages) AS TEXT))))
ORDER BY t.entry_id DESC
LIMIT sqlc.arg(page_limit);

-- ListProfileEntries is the profile timeline (FR-204) of an author: the author's posts and
-- boosts of each visibility whose include_* flag is 1, and, when include_direct is 1, the
-- direct posts addressed to the viewer (all of them when the viewer is the author). Boosts of
-- statuses whose author the viewer excludes, or whose author or domain is hidden from
-- profiles, are left out. exclude_replies leaves out the posts that reply to a status, and
-- exclude_boosts the boosts.
-- name: ListProfileEntries :many
SELECT e.id, e.kind, e.status_id, e.author_account_id, e.author_domain, e.original_author_account_id,
  e.original_author_domain, e.visibility, e.is_local, e.language, e.revision_seq, e.is_reply
FROM feed_entries e
WHERE e.author_account_id = sqlc.arg(author)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR e.id < CAST(sqlc.arg(before) AS TEXT))
  AND ((e.visibility = 'public' AND CAST(sqlc.arg(include_public) AS INTEGER) = 1)
    OR (e.visibility = 'local' AND CAST(sqlc.arg(include_local) AS INTEGER) = 1)
    OR (e.visibility = 'unlisted' AND CAST(sqlc.arg(include_unlisted) AS INTEGER) = 1)
    OR (e.visibility = 'private' AND CAST(sqlc.arg(include_private) AS INTEGER) = 1)
    OR (CAST(sqlc.arg(include_direct) AS INTEGER) = 1
    AND e.visibility = 'direct' AND (e.author_account_id = sqlc.arg(viewer) OR EXISTS (
      SELECT 1 FROM feed_direct_recipients d WHERE d.recipient_account_id = sqlc.arg(viewer) AND d.entry_id = e.id))))
  AND (e.original_author_account_id = e.author_account_id OR (
    NOT EXISTS (
      SELECT 1 FROM feed_exclusions x
      WHERE x.viewer_account_id = sqlc.arg(viewer) AND x.target_account_id = e.original_author_account_id
        AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
    AND NOT EXISTS (
      SELECT 1 FROM feed_author_restrictions r
      WHERE r.account_id = e.original_author_account_id AND r.restriction = 'profile_view'
        AND (r.effective_until IS NULL OR r.effective_until > CAST(sqlc.arg(now) AS INTEGER)))
    AND NOT EXISTS (
      SELECT 1 FROM feed_domain_restrictions dr
      WHERE dr.restriction = 'profile_view'
        AND (dr.effective_until IS NULL OR dr.effective_until > CAST(sqlc.arg(now) AS INTEGER))
        AND (dr.domain = e.original_author_domain
          OR substr(e.original_author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain))))
  AND (e.original_author_account_id = e.author_account_id OR NOT EXISTS (
    SELECT 1 FROM feed_domain_mutes dm
    WHERE dm.viewer_account_id = sqlc.arg(viewer)
      AND (dm.domain = e.original_author_domain OR substr(e.original_author_domain, -(length(dm.domain) + 1)) = '.' || dm.domain)))
  AND (CAST(sqlc.arg(all_languages) AS INTEGER) = 1 OR e.language = 'und' OR e.language IN (SELECT value FROM json_each(CAST(sqlc.arg(languages) AS TEXT))))
  AND (CAST(sqlc.arg(exclude_replies) AS INTEGER) = 0 OR e.is_reply = 0)
  AND (CAST(sqlc.arg(exclude_boosts) AS INTEGER) = 0 OR e.kind = 'post')
ORDER BY e.id DESC
LIMIT sqlc.arg(page_limit);

-- Relation facts of a page, for re-applying CanView ---------------------------------------------

-- name: ListFeedFollowedAmong :many
SELECT followee_account_id FROM feed_follow_edges
WHERE follower_account_id = sqlc.arg(viewer) AND followee_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)));

-- name: ListFeedExclusionsAmong :many
SELECT target_account_id, reason FROM feed_exclusions
WHERE viewer_account_id = sqlc.arg(viewer) AND (expires_at IS NULL OR expires_at > CAST(sqlc.arg(now) AS INTEGER))
  AND target_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)));

-- name: ListFeedAuthorRestrictionsAmong :many
SELECT account_id, restriction FROM feed_author_restrictions
WHERE (effective_until IS NULL OR effective_until > CAST(sqlc.arg(now) AS INTEGER))
  AND account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)));

-- name: ListFeedDomainRestrictionsAmong :many
SELECT domain, restriction FROM feed_domain_restrictions
WHERE (effective_until IS NULL OR effective_until > CAST(sqlc.arg(now) AS INTEGER))
  AND domain IN (SELECT value FROM json_each(CAST(sqlc.arg(domains) AS TEXT)));

-- name: ListFeedDirectEntriesAmong :many
SELECT entry_id FROM feed_direct_recipients
WHERE recipient_account_id = sqlc.arg(viewer) AND entry_id IN (SELECT value FROM json_each(CAST(sqlc.arg(entry_ids) AS TEXT)));

-- tag_usage (ADR-0092 section 1): the posts of visibility public, local, and unlisted count; the
-- statements run after feed_tag_entries has changed, and last_used_at is read back from it.

-- ListCountedFeedTagIDs returns the tags of an entry that tag_usage counts.
-- name: ListCountedFeedTagIDs :many
SELECT tag_id FROM feed_tag_entries WHERE entry_id = ? AND visibility IN ('public', 'local', 'unlisted') ORDER BY tag_id;

-- AddTagUsage adds n posts to the usage of a tag, which a post that counts carries.
-- name: AddTagUsage :exec
INSERT INTO tag_usage (tag_id, statuses_count, last_used_at)
VALUES (CAST(sqlc.arg(tag_id) AS TEXT), CAST(sqlc.arg(n) AS INTEGER), (SELECT max(f.revised_at) FROM feed_tag_entries f WHERE f.tag_id = CAST(sqlc.arg(tag_id) AS TEXT) AND f.visibility IN ('public', 'local', 'unlisted')))
ON CONFLICT (tag_id) DO UPDATE SET statuses_count = tag_usage.statuses_count + excluded.statuses_count, last_used_at = excluded.last_used_at;

-- DeleteDrainedTagUsage deletes the usage of a tag that n posts fewer would leave at zero.
-- name: DeleteDrainedTagUsage :exec
DELETE FROM tag_usage WHERE tag_id = CAST(sqlc.arg(tag_id) AS TEXT) AND statuses_count <= CAST(sqlc.arg(n) AS INTEGER);

-- AdjustTagUsage adds delta (which may be zero or negative) posts to the usage of a tag that has
-- a row and that a post that counts still carries, and reads its last_used_at again.
-- name: AdjustTagUsage :exec
UPDATE tag_usage SET statuses_count = statuses_count + CAST(sqlc.arg(delta) AS INTEGER), last_used_at = (SELECT max(f.revised_at) FROM feed_tag_entries f WHERE f.tag_id = CAST(sqlc.arg(tag_id) AS TEXT) AND f.visibility IN ('public', 'local', 'unlisted'))
WHERE tag_id = CAST(sqlc.arg(tag_id) AS TEXT);

-- ListTagUsageOf returns the usage of the tags among ids that have a row.
-- name: ListTagUsageOf :many
SELECT tag_id, statuses_count, last_used_at FROM tag_usage WHERE tag_id IN (sqlc.slice(ids)) ORDER BY tag_id;

-- Lists (data-model 4.6, ADR-0122) -----------------------------------------------------------------

-- name: InsertList :exec
INSERT INTO lists (id, owner_account_id, created_at) VALUES (?, ?, ?);

-- name: UpsertListName :exec
INSERT INTO list_names (list_id, name) VALUES (?, ?)
ON CONFLICT (list_id) DO UPDATE SET name = excluded.name;

-- name: GetList :one
SELECT l.id, l.owner_account_id, l.created_at, n.name
FROM lists l JOIN list_names n ON n.list_id = l.id
WHERE l.id = ?;

-- name: ListListsOfOwner :many
SELECT l.id, l.owner_account_id, l.created_at, n.name
FROM lists l JOIN list_names n ON n.list_id = l.id
WHERE l.owner_account_id = ?
ORDER BY l.id;

-- name: CountListsOfOwner :one
SELECT count(*) FROM lists WHERE owner_account_id = ?;

-- name: DeleteListMembersOfList :exec
DELETE FROM list_members WHERE list_id = ?;

-- name: DeleteListName :exec
DELETE FROM list_names WHERE list_id = ?;

-- name: DeleteList :exec
DELETE FROM lists WHERE id = ?;

-- name: InsertListMember :execrows
INSERT INTO list_members (list_id, member_account_id, owner_account_id, added_at) VALUES (?, ?, ?, ?)
ON CONFLICT (list_id, member_account_id) DO NOTHING;

-- name: DeleteListMember :exec
DELETE FROM list_members WHERE list_id = ? AND member_account_id = ?;

-- name: CountListMembers :one
SELECT count(*) FROM list_members WHERE list_id = ?;

-- ListListMembers returns the members of a list after the member ID after (an empty one starts
-- at the first), in the order of their IDs.
-- name: ListListMembers :many
SELECT member_account_id FROM list_members
WHERE list_id = sqlc.arg(list_id) AND member_account_id > CAST(sqlc.arg(after) AS TEXT)
ORDER BY member_account_id
LIMIT sqlc.arg(page_limit);

-- name: FeedFollowEdgeExists :one
SELECT EXISTS (SELECT 1 FROM feed_follow_edges WHERE follower_account_id = ? AND followee_account_id = ?);

-- name: ListListIDsWithMember :many
SELECT list_id FROM list_members WHERE owner_account_id = ? AND member_account_id = ? ORDER BY list_id;

-- DeleteListMembersOfFollow takes the followee of an ended follow out of every list of the
-- follower.
-- name: DeleteListMembersOfFollow :exec
DELETE FROM list_members WHERE owner_account_id = ? AND member_account_id = ?;

-- DeleteListMembersWithoutFollow takes out of the lists every member whose owner no longer
-- actively follows it, after a rebuild of feed_follow_edges.
-- name: DeleteListMembersWithoutFollow :exec
DELETE FROM list_members WHERE NOT EXISTS (
  SELECT 1 FROM feed_follow_edges f
  WHERE f.follower_account_id = list_members.owner_account_id AND f.followee_account_id = list_members.member_account_id);

-- name: ListAllListMembers :many
SELECT list_id, member_account_id, owner_account_id, added_at FROM list_members ORDER BY list_id, member_account_id;

-- ListListEntries is the timeline of a list: ListHomeEntries with the authors (posters and
-- boosters) limited to the members of the list, which never include the viewer.
-- name: ListListEntries :many
SELECT e.id, e.kind, e.status_id, e.author_account_id, e.author_domain, e.original_author_account_id,
  e.original_author_domain, e.visibility, e.is_local, e.language, e.revision_seq, e.is_reply
FROM feed_entries e
WHERE e.author_account_id IN (
    SELECT m.member_account_id FROM list_members m
    JOIN feed_follow_edges f ON f.follower_account_id = m.owner_account_id AND f.followee_account_id = m.member_account_id
    WHERE m.list_id = sqlc.arg(list_id) AND m.owner_account_id = sqlc.arg(viewer))
  AND e.author_account_id <> sqlc.arg(viewer)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR e.id < CAST(sqlc.arg(before) AS TEXT))
  AND e.id >= CAST(sqlc.arg(window_floor) AS TEXT)
  AND (e.kind = 'post' OR EXISTS (
    SELECT 1 FROM feed_follow_edges f
    WHERE f.follower_account_id = sqlc.arg(viewer) AND f.followee_account_id = e.author_account_id AND f.show_boosts = 1))
  AND (e.visibility <> 'direct' OR EXISTS (
    SELECT 1 FROM feed_direct_recipients d WHERE d.recipient_account_id = sqlc.arg(viewer) AND d.entry_id = e.id))
  AND NOT EXISTS (
    SELECT 1 FROM feed_exclusions x
    WHERE x.viewer_account_id = sqlc.arg(viewer)
      AND x.target_account_id IN (e.author_account_id, e.original_author_account_id)
      AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_author_restrictions r
    WHERE r.account_id IN (e.author_account_id, e.original_author_account_id) AND r.restriction = 'profile_view'
      AND (r.effective_until IS NULL OR r.effective_until > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_restrictions dr
    WHERE dr.restriction = 'profile_view'
      AND (dr.effective_until IS NULL OR dr.effective_until > CAST(sqlc.arg(now) AS INTEGER))
      AND (dr.domain IN (e.author_domain, e.original_author_domain)
        OR substr(e.author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain
        OR substr(e.original_author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_mutes dm
    WHERE dm.viewer_account_id = sqlc.arg(viewer) AND e.kind = 'boost'
      AND (dm.domain = e.original_author_domain OR substr(e.original_author_domain, -(length(dm.domain) + 1)) = '.' || dm.domain))
  AND (CAST(sqlc.arg(all_languages) AS INTEGER) = 1 OR e.language = 'und' OR e.language IN (SELECT value FROM json_each(CAST(sqlc.arg(languages) AS TEXT))))
ORDER BY e.id DESC
LIMIT sqlc.arg(page_limit);
