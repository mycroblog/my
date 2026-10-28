-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertStatus :exec
INSERT INTO statuses (
  id, uri, url, author_account_id, is_local, visibility, in_reply_to_uri, conversation_uri, published_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertLocalStatus :exec
INSERT INTO local_statuses (status_id, author_account_id, client_id, idempotency_key) VALUES ($1, $2, $3, $4);

-- name: InsertStatusRevision :exec
INSERT INTO status_revisions (id, status_id, seq, spoiler_text, sensitive, language, revised_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: InsertStatusRevisionBody :exec
INSERT INTO status_revision_bodies (revision_id, language, content_html) VALUES ($1, $2, $3);

-- name: InsertStatusRevisionSource :exec
INSERT INTO status_revision_sources (revision_id, source_text, source_format) VALUES ($1, $2, $3);

-- name: InsertStatusRevisionAttachment :exec
INSERT INTO status_revision_attachments (revision_id, position, media_id, description) VALUES ($1, $2, $3, $4);

-- name: InsertStatusRevisionMention :exec
INSERT INTO status_revision_mentions (revision_id, account_id) VALUES ($1, $2);

-- name: InsertStatusRevisionTag :exec
INSERT INTO status_revision_tags (revision_id, tag_id) VALUES ($1, $2);

-- name: InsertStatusRevisionEmoji :exec
INSERT INTO status_revision_emojis (revision_id, emoji_id) VALUES ($1, $2);

-- name: InsertStatusDeletion :exec
INSERT INTO status_deletions (status_id, uri, author_account_id, origin, moderation_action_id, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertTagIfAbsent :exec
INSERT INTO tags (id, name, name_normalized, created_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (name_normalized) DO NOTHING;

-- ListTagsByPrefix returns up to lim tags whose name_normalized is in [low, high), in that order,
-- for the prefix search of hashtags (ADR-0092 section 1).
-- name: ListTagsByPrefix :many
SELECT id, name, name_normalized, created_at FROM tags
WHERE name_normalized >= sqlc.arg(low)::text AND name_normalized < sqlc.arg(high)::text
ORDER BY name_normalized
LIMIT sqlc.arg(lim);

-- name: GetTagByName :one
SELECT id, name, name_normalized, created_at FROM tags WHERE name_normalized = $1;

-- name: GetTag :one
SELECT id, name, name_normalized, created_at FROM tags WHERE id = $1;

-- Projections, written by the content module's projectors.

-- name: GetStatusHead :one
SELECT status_id, revision_id, seq, last_revised_at FROM status_heads WHERE status_id = $1;

-- name: UpsertStatusHead :exec
INSERT INTO status_heads (status_id, revision_id, seq, last_revised_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (status_id) DO UPDATE SET
  revision_id = excluded.revision_id,
  seq = excluded.seq,
  last_revised_at = excluded.last_revised_at;

-- name: DeleteStatusHead :exec
DELETE FROM status_heads WHERE status_id = $1;

-- name: InsertStatusMention :exec
INSERT INTO status_mention_index (account_id, status_id) VALUES ($1, $2)
ON CONFLICT (account_id, status_id) DO NOTHING;

-- name: DeleteStatusMentions :exec
DELETE FROM status_mention_index WHERE status_id = $1;

-- name: StatusIsDeleted :one
SELECT EXISTS (SELECT 1 FROM status_deletions WHERE status_id = $1);

-- name: GetLiveStatusByURI :one
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri,
  s.conversation_uri, s.published_at
FROM statuses s
WHERE s.uri = $1 AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id);

-- LockStatusKey takes a transaction-scoped advisory lock on a key of the content module's
-- status projections (a status, or a URI with its replies), so that the projectors of one
-- thread serialize under READ COMMITTED.
-- name: LockStatusKey :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- name: ListLiveRepliesTo :many
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri,
  s.conversation_uri, s.published_at
FROM statuses s
WHERE s.in_reply_to_uri = $1 AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
ORDER BY s.id;

-- name: ListLiveRemoteStatusIDsOfAuthor :many
-- The remote statuses of an author that are not deleted, newest first, below an ID (NULL for the
-- first page).
SELECT s.id FROM statuses s
WHERE s.author_account_id = sqlc.arg(author_account_id) AND s.is_local = FALSE
  AND (sqlc.narg(before)::uuid IS NULL OR s.id < sqlc.narg(before)::uuid)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
ORDER BY s.id DESC
LIMIT sqlc.arg(lim);

-- name: UpsertStatusReplyLink :exec
INSERT INTO status_reply_links (status_id, parent_status_id, parent_author_account_id) VALUES ($1, $2, $3)
ON CONFLICT (status_id) DO UPDATE SET
  parent_status_id = excluded.parent_status_id,
  parent_author_account_id = excluded.parent_author_account_id;

-- name: GetStatusReplyLink :one
SELECT status_id, parent_status_id, parent_author_account_id FROM status_reply_links WHERE status_id = $1;

-- name: DeleteStatusReplyLink :exec
DELETE FROM status_reply_links WHERE status_id = $1;

-- name: DeleteStatusReplyLinksToParent :exec
DELETE FROM status_reply_links WHERE parent_status_id = $1;

-- name: InsertStatusCounters :exec
INSERT INTO status_counters (status_id, replies_count, boosts_count, favourites_count) VALUES ($1, $2, $3, $4)
ON CONFLICT (status_id) DO NOTHING;

-- name: AddStatusReplies :exec
UPDATE status_counters SET replies_count = replies_count + sqlc.arg(delta) WHERE status_id = sqlc.arg(status_id);

-- name: DeleteStatusCounters :exec
DELETE FROM status_counters WHERE status_id = $1;

-- name: AddAccountPost :exec
INSERT INTO account_post_counters (account_id, statuses_count, last_status_at) VALUES ($1, 1, $2)
ON CONFLICT (account_id) DO UPDATE SET
  statuses_count = account_post_counters.statuses_count + 1,
  last_status_at = CASE
    WHEN excluded.last_status_at > account_post_counters.last_status_at THEN excluded.last_status_at
    ELSE account_post_counters.last_status_at
  END;

-- name: LatestLivePublishedAt :one
SELECT s.published_at FROM statuses s
WHERE s.author_account_id = $1 AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
ORDER BY s.published_at DESC
LIMIT 1;

-- name: RemoveAccountPost :exec
UPDATE account_post_counters SET statuses_count = statuses_count - 1, last_status_at = $1
WHERE account_id = $2;

-- name: DeleteEmptyAccountPostCounter :exec
DELETE FROM account_post_counters WHERE account_id = $1 AND statuses_count <= 0;

-- name: DeleteAccountPostCounter :exec
DELETE FROM account_post_counters WHERE account_id = $1;

-- Rebuilds: each empties a projection and derives it again from the statuses, their revisions,
-- and their deletions (data-model section 9). They state the projectors' rules in SQL.

-- name: DeleteAllStatusHeads :exec
DELETE FROM status_heads;

-- name: RebuildStatusHeads :exec
INSERT INTO status_heads (status_id, revision_id, seq, last_revised_at)
SELECT r.status_id, r.id, r.seq, r.revised_at
FROM status_revisions r JOIN statuses s ON s.id = r.status_id
WHERE NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
  AND NOT EXISTS (SELECT 1 FROM status_revisions n WHERE n.status_id = r.status_id AND n.seq > r.seq);

-- name: DeleteAllStatusMentions :exec
DELETE FROM status_mention_index;

-- name: RebuildStatusMentions :exec
INSERT INTO status_mention_index (account_id, status_id)
SELECT m.account_id, r.status_id
FROM status_revision_mentions m
  JOIN status_revisions r ON r.id = m.revision_id
  JOIN statuses s ON s.id = r.status_id
WHERE NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
  AND NOT EXISTS (SELECT 1 FROM status_revisions n WHERE n.status_id = r.status_id AND n.seq > r.seq);

-- name: DeleteAllStatusReplyLinks :exec
DELETE FROM status_reply_links;

-- name: RebuildStatusReplyLinks :exec
INSERT INTO status_reply_links (status_id, parent_status_id, parent_author_account_id)
SELECT c.id, p.id, p.author_account_id
FROM statuses c JOIN statuses p ON p.uri = c.in_reply_to_uri
WHERE NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = c.id)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = p.id);

-- name: DeleteAllStatusCounters :exec
DELETE FROM status_counters;

-- name: RebuildStatusCounters :exec
INSERT INTO status_counters (status_id, replies_count, boosts_count, favourites_count)
SELECT s.id,
  (SELECT COUNT(*) FROM statuses c
    WHERE c.in_reply_to_uri = s.uri AND c.visibility IN ('public', 'local', 'unlisted')
      AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = c.id)),
  (SELECT COUNT(*) FROM boosts b
    WHERE b.status_id = s.id AND NOT EXISTS (SELECT 1 FROM boost_retractions r WHERE r.boost_id = b.id)),
  (SELECT COUNT(*) FROM favourites f WHERE f.status_id = s.id)
FROM statuses s
WHERE NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id);

-- name: DeleteAllAccountPostCounters :exec
DELETE FROM account_post_counters;

-- name: RebuildAccountPostCounters :exec
INSERT INTO account_post_counters (account_id, statuses_count, last_status_at)
SELECT s.author_account_id, COUNT(*), MAX(s.published_at)
FROM statuses s
WHERE NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
GROUP BY s.author_account_id;

-- Projection contents, for verifying the projections.

-- name: ListStatusHeads :many
SELECT status_id, revision_id, seq, last_revised_at FROM status_heads ORDER BY status_id;

-- name: ListStatusMentions :many
SELECT account_id, status_id FROM status_mention_index ORDER BY account_id, status_id;

-- name: ListStatusReplyLinks :many
SELECT status_id, parent_status_id, parent_author_account_id FROM status_reply_links ORDER BY status_id;

-- name: ListStatusCounters :many
SELECT status_id, replies_count, boosts_count, favourites_count FROM status_counters ORDER BY status_id;

-- name: ListAccountPostCounters :many
SELECT account_id, statuses_count, last_status_at FROM account_post_counters ORDER BY account_id;

-- Lookups.

-- name: GetStatus :one
SELECT id, uri, url, author_account_id, is_local, visibility, in_reply_to_uri, conversation_uri, published_at
FROM statuses WHERE id = $1;

-- name: GetStatusByURI :one
SELECT id, uri, url, author_account_id, is_local, visibility, in_reply_to_uri, conversation_uri, published_at
FROM statuses WHERE uri = $1;

-- name: GetLocalStatus :one
SELECT status_id, author_account_id, client_id, idempotency_key FROM local_statuses WHERE status_id = $1;

-- name: GetLocalStatusByIdempotencyKey :one
SELECT status_id, author_account_id, client_id, idempotency_key FROM local_statuses
WHERE author_account_id = $1 AND idempotency_key = $2;

-- name: GetStatusRevision :one
SELECT id, status_id, seq, spoiler_text, sensitive, language, revised_at FROM status_revisions WHERE id = $1;

-- name: ListStatusRevisions :many
SELECT id, status_id, seq, spoiler_text, sensitive, language, revised_at FROM status_revisions
WHERE status_id = $1 ORDER BY seq;

-- name: ListStatusRevisionBodies :many
SELECT revision_id, language, content_html FROM status_revision_bodies WHERE revision_id = $1 ORDER BY language;

-- name: GetStatusRevisionSource :one
SELECT revision_id, source_text, source_format FROM status_revision_sources WHERE revision_id = $1;

-- name: ListStatusRevisionAttachments :many
SELECT revision_id, position, media_id, description FROM status_revision_attachments
WHERE revision_id = $1 ORDER BY position;

-- name: ListStatusRevisionMentions :many
SELECT account_id FROM status_revision_mentions WHERE revision_id = $1 ORDER BY account_id;

-- name: ListStatusRevisionTags :many
SELECT tag_id FROM status_revision_tags WHERE revision_id = $1 ORDER BY tag_id;

-- name: ListStatusRevisionEmojis :many
SELECT emoji_id FROM status_revision_emojis WHERE revision_id = $1 ORDER BY emoji_id;

-- name: GetStatusDeletion :one
SELECT status_id, uri, author_account_id, origin, moderation_action_id, deleted_at FROM status_deletions
WHERE status_id = $1;

-- GetStatusDeletionByURI returns the deletion that keeps a URI from coming back, or else the
-- latest deletion by retention (a URI may have several, ADR-0034).
-- name: GetStatusDeletionByURI :one
SELECT status_id, uri, author_account_id, origin, moderation_action_id, deleted_at FROM status_deletions
WHERE uri = $1
ORDER BY CASE WHEN origin = 'retention' THEN 1 ELSE 0 END, deleted_at DESC, status_id DESC
LIMIT 1;

-- name: GetStatusCounters :one
SELECT status_id, replies_count, boosts_count, favourites_count FROM status_counters WHERE status_id = $1;

-- name: GetAccountPostCounter :one
SELECT account_id, statuses_count, last_status_at FROM account_post_counters WHERE account_id = $1;

-- Sources of other modules' projections.

-- ListLiveStatusHeads returns every status that is not deleted (each has a head) with the ID of
-- its current revision, ordered by ID.
-- name: ListLiveStatusHeads :many
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri,
  s.conversation_uri, s.published_at, h.revision_id
FROM statuses s
JOIN status_heads h ON h.status_id = s.id
ORDER BY s.id;

-- local_post_counters: the posts of each local account that NodeInfo reports (data-model section 4.4).
-- The status projectors count the statuses and the interaction projectors the boosts.

-- name: AddLocalPosts :exec
INSERT INTO local_post_counters (account_id, posts_count) VALUES (sqlc.arg(account_id), sqlc.arg(added))
ON CONFLICT (account_id) DO UPDATE SET posts_count = local_post_counters.posts_count + excluded.posts_count;

-- DeleteSpentLocalPostCounter removes the counter that removing posts would bring to zero, before
-- SubtractLocalPosts subtracts them from one that stays positive.
-- name: DeleteSpentLocalPostCounter :exec
DELETE FROM local_post_counters WHERE account_id = sqlc.arg(account_id) AND posts_count <= sqlc.arg(removed);

-- name: SubtractLocalPosts :exec
UPDATE local_post_counters SET posts_count = posts_count - sqlc.arg(removed) WHERE account_id = sqlc.arg(account_id);

-- name: SumLocalPosts :one
SELECT COALESCE(SUM(posts_count), 0)::bigint AS total FROM local_post_counters;

-- name: DeleteAllLocalPostCounters :exec
DELETE FROM local_post_counters;

-- RebuildLocalPostCounters states the projectors' rules: the local statuses that are not deleted
-- and not direct, and the local boosts that are not retracted, of statuses that are not deleted.
-- name: RebuildLocalPostCounters :exec
INSERT INTO local_post_counters (account_id, posts_count)
SELECT p.account_id, COUNT(*) FROM (
  SELECT s.author_account_id AS account_id FROM statuses s
  WHERE s.is_local AND s.visibility <> 'direct'
    AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
  UNION ALL
  SELECT b.booster_account_id AS account_id FROM boosts b
  WHERE b.is_local
    AND NOT EXISTS (SELECT 1 FROM boost_retractions r WHERE r.boost_id = b.id)
    AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id)
) p
GROUP BY p.account_id;

-- name: ListLocalPostCounters :many
SELECT account_id, posts_count FROM local_post_counters ORDER BY account_id;

-- ListLocalStatusBoosters returns the local accounts whose live boosts of a status go with it.
-- name: ListLocalStatusBoosters :many
SELECT e.booster_account_id FROM boost_edges e JOIN boosts b ON b.id = e.boost_id
WHERE e.status_id = sqlc.arg(status_id) AND b.is_local
ORDER BY e.booster_account_id;

-- CountLocalPublicPostsByDay counts the public local statuses that are not deleted, published in
-- [from_at, to_at), by the day of 24 hours from from_at that they fall in (0 the first). The
-- statuses' IDs are UUIDv7 stamped at their creation, so from_id (the floor of from_at) bounds
-- the scan of local_statuses by its primary key.
-- name: CountLocalPublicPostsByDay :many
SELECT FLOOR(EXTRACT(EPOCH FROM (s.published_at - sqlc.arg(from_at)::timestamptz)) / 86400)::bigint AS day, COUNT(*)::bigint AS posts
FROM local_statuses ls
JOIN statuses s ON s.id = ls.status_id
JOIN status_heads h ON h.status_id = ls.status_id
WHERE ls.status_id >= sqlc.arg(from_id)::uuid AND s.visibility = 'public'
  AND s.published_at >= sqlc.arg(from_at)::timestamptz AND s.published_at < sqlc.arg(to_at)::timestamptz
GROUP BY day ORDER BY day;
