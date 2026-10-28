-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of remote statuses and media (data-model section 4.4), owned by internal/app/content.

-- name: InsertRemoteStatusRevision :exec
INSERT INTO remote_status_revisions (revision_id, activity_uri, html_policy_version, omitted_attachment_count)
VALUES (@revision_id, @activity_uri, @html_policy_version, @omitted_attachment_count);

-- name: GetRemoteStatusRevision :one
SELECT revision_id, activity_uri, html_policy_version, omitted_attachment_count FROM remote_status_revisions
WHERE revision_id = @revision_id;

-- name: InsertRemoteMedia :execrows
INSERT INTO remote_media (
  media_id, owner_account_id, remote_url, declared_content_type, declared_media_type, purpose, width, height,
  blurhash, focus_x, focus_y, description
) VALUES (
  @media_id, @owner_account_id, @remote_url, @declared_content_type, @declared_media_type, @purpose, @width, @height,
  @blurhash, @focus_x, @focus_y, @description
)
ON CONFLICT (owner_account_id, remote_url) DO NOTHING;

-- name: GetRemoteMediaByURL :one
SELECT media_id, owner_account_id, remote_url, declared_content_type, declared_media_type, purpose, width, height,
  blurhash, focus_x, focus_y, description
FROM remote_media WHERE owner_account_id = @owner_account_id AND remote_url = @remote_url;

-- name: GetRemoteMedia :one
SELECT media_id, owner_account_id, remote_url, declared_content_type, declared_media_type, purpose, width, height,
  blurhash, focus_x, focus_y, description
FROM remote_media WHERE media_id = @media_id;

-- ListRemoteMedia returns the remote media files with an ID in @media_ids, for the images of a
-- page of accounts.
-- name: ListRemoteMedia :many
SELECT media_id, owner_account_id, remote_url, declared_content_type, declared_media_type, purpose, width, height,
  blurhash, focus_x, focus_y, description
FROM remote_media WHERE media_id IN (SELECT value FROM json_each(CAST(@media_ids AS TEXT))) ORDER BY media_id;

-- name: UpsertRemoteMediaCache :exec
INSERT INTO remote_media_cache (url, blob_key, content_type, file_size, fetched_at, expires_at, last_accessed_at)
VALUES (@url, @blob_key, @content_type, @file_size, @fetched_at, @expires_at, @last_accessed_at)
ON CONFLICT (url) DO UPDATE SET
  blob_key = excluded.blob_key,
  content_type = excluded.content_type,
  file_size = excluded.file_size,
  fetched_at = excluded.fetched_at,
  expires_at = excluded.expires_at,
  last_accessed_at = excluded.last_accessed_at;

-- InsertRemoteMediaCache records a copy of a remote file unless the URL has an entry already.
-- name: InsertRemoteMediaCache :execrows
INSERT INTO remote_media_cache (url, blob_key, content_type, file_size, fetched_at, expires_at, last_accessed_at)
VALUES (@url, @blob_key, @content_type, @file_size, @fetched_at, @expires_at, @last_accessed_at)
ON CONFLICT (url) DO NOTHING;

-- LockRemoteMediaCache reads the entry of @url in a transaction that replaces it. SQLite
-- transactions take the write lock when they begin, so a concurrent refresh of the URL waits and
-- reads the entry this one writes.
-- name: LockRemoteMediaCache :one
SELECT url, blob_key, content_type, file_size, fetched_at, expires_at, last_accessed_at
FROM remote_media_cache WHERE url = @url;

-- name: GetRemoteMediaCache :one
SELECT url, blob_key, content_type, file_size, fetched_at, expires_at, last_accessed_at
FROM remote_media_cache WHERE url = @url;

-- name: TouchRemoteMediaCache :exec
UPDATE remote_media_cache SET last_accessed_at = @last_accessed_at WHERE url = @url;

-- ListExpiredRemoteMediaCache returns at most @batch_limit entries past their expiry, the
-- earliest expired first, for the cleanup to delete with their blobs.
-- name: ListExpiredRemoteMediaCache :many
SELECT url, blob_key, content_type, file_size, fetched_at, expires_at, last_accessed_at
FROM remote_media_cache WHERE expires_at <= @now
ORDER BY expires_at, url LIMIT @batch_limit;

-- ListLeastRecentlyAccessedRemoteMediaCache returns the least recently accessed entries, for the
-- cleanup to bring the cache under its size limit.
-- name: ListLeastRecentlyAccessedRemoteMediaCache :many
SELECT url, blob_key, content_type, file_size, fetched_at, expires_at, last_accessed_at
FROM remote_media_cache ORDER BY last_accessed_at, url LIMIT @batch_limit;

-- name: SumRemoteMediaCacheSize :one
SELECT CAST(COALESCE(SUM(file_size), 0) AS BIGINT) FROM remote_media_cache;

-- DeleteRemoteMediaCache deletes the entry of @url unless it was fetched again after the
-- cleanup read it (its fetched_at differs).
-- name: DeleteRemoteMediaCache :execrows
DELETE FROM remote_media_cache WHERE url = @url AND fetched_at = @fetched_at;

-- ExistsLocalStatusInConversation reports whether a local status that is not deleted belongs to
-- the conversation, for the scope of remote content (ADR-0034: replies in a thread that has a
-- local status).
-- name: ExistsLocalStatusInConversation :one
SELECT EXISTS (
  SELECT 1 FROM statuses s
  WHERE s.conversation_uri = @conversation_uri AND s.is_local = 1
    AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
);

-- IsCustomEmojiImage reports whether any revision of a custom emoji, current or not, uses the
-- media file as its image, for the avatars policy of the remote media cache.
-- name: IsCustomEmojiImage :one
SELECT EXISTS (SELECT 1 FROM custom_emoji_images WHERE media_id = @media_id);

-- name: InsertCustomEmojiIfAbsent :exec
INSERT INTO custom_emojis (id, shortcode, domain, uri, created_at)
VALUES (@id, @shortcode, @domain, @uri, @created_at)
ON CONFLICT DO NOTHING;

-- name: GetCustomEmojiByShortcode :one
SELECT id, shortcode, domain, uri, created_by_user_id, created_at FROM custom_emojis
WHERE shortcode = @shortcode AND domain = @domain;

-- name: GetCustomEmoji :one
SELECT id, shortcode, domain, uri, created_by_user_id, created_at FROM custom_emojis WHERE id = @id;

-- name: InsertCustomEmojiImage :exec
INSERT INTO custom_emoji_images (id, emoji_id, media_id, revised_at)
VALUES (@id, @emoji_id, @media_id, @revised_at);

-- name: GetLatestCustomEmojiImage :one
SELECT id, emoji_id, media_id, revised_at FROM custom_emoji_images
WHERE emoji_id = @emoji_id ORDER BY id DESC LIMIT 1;

-- GetLatestCustomEmojiImageRemoteURL returns the remote URL of the newest image of an emoji, ''
-- when that image is not a remote media file.
-- name: GetLatestCustomEmojiImageRemoteURL :one
SELECT CAST(COALESCE(r.remote_url, '') AS TEXT) AS remote_url
FROM custom_emoji_images i
LEFT JOIN remote_media r ON r.media_id = i.media_id
WHERE i.emoji_id = @emoji_id ORDER BY i.id DESC LIMIT 1;

-- name: InsertPoll :exec
INSERT INTO polls (id, revision_id, multiple, hide_totals, closes_at) VALUES (@id, @revision_id, @multiple, @hide_totals, @closes_at);

-- name: InsertPollOption :exec
INSERT INTO poll_options (poll_id, position, title) VALUES (@poll_id, @position, @title);

-- name: GetPollByRevision :one
SELECT id, revision_id, multiple, hide_totals, closes_at FROM polls WHERE revision_id = @revision_id;

-- name: ListPollOptions :many
SELECT poll_id, position, title FROM poll_options WHERE poll_id = @poll_id ORDER BY position;

-- InsertPollVote inserts a vote unless the account voted for the choice, or the vote's URI is
-- recorded, already.
-- name: InsertPollVote :execrows
INSERT INTO poll_votes (id, poll_id, account_id, position, uri, voted_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: ListPollVotesOfAccount :many
SELECT id, poll_id, account_id, position, uri, voted_at FROM poll_votes
WHERE poll_id = ? AND account_id = ? ORDER BY position;

-- GetPollStatus returns the status whose revision has the poll.
-- name: GetPollStatus :one
SELECT r.status_id FROM polls p JOIN status_revisions r ON r.id = p.revision_id WHERE p.id = @poll_id;

-- name: CountPollVotes :many
SELECT position, count(*) AS votes FROM poll_votes WHERE poll_id = @poll_id GROUP BY position ORDER BY position;

-- name: CountPollVoters :one
SELECT count(DISTINCT account_id) FROM poll_votes WHERE poll_id = @poll_id;

-- ListPollVoterIDs returns the accounts that voted in a poll after @after, in order.
-- name: ListPollVoterIDs :many
SELECT DISTINCT account_id FROM poll_votes
WHERE poll_id = @poll_id AND account_id > @after ORDER BY account_id LIMIT @lim;

-- name: InsertPollTallyObservation :exec
INSERT INTO poll_tally_observations (id, poll_id, voters_count, observed_at)
VALUES (@id, @poll_id, @voters_count, @observed_at);

-- name: InsertPollTallyObservationCount :exec
INSERT INTO poll_tally_observation_counts (observation_id, position, votes)
VALUES (@observation_id, @position, @votes);

-- name: GetLatestPollTallyObservation :one
SELECT id, poll_id, voters_count, observed_at FROM poll_tally_observations
WHERE poll_id = @poll_id ORDER BY id DESC LIMIT 1;

-- name: ListPollTallyObservationCounts :many
SELECT observation_id, position, votes FROM poll_tally_observation_counts
WHERE observation_id = @observation_id ORDER BY position;

-- name: DeletePollTallies :exec
DELETE FROM poll_tallies WHERE poll_id = @poll_id;

-- name: InsertPollTally :exec
INSERT INTO poll_tallies (poll_id, position, votes, voters_count)
VALUES (@poll_id, @position, @votes, @voters_count);

-- name: ListPollTallies :many
SELECT poll_id, position, votes, voters_count FROM poll_tallies WHERE poll_id = @poll_id ORDER BY position;

-- ListCurrentCustomEmojiImages returns the custom emoji with an ID in @emoji_ids that have an
-- image, each with its current (newest) image and its settings, for the emoji of a page of
-- statuses and accounts.
-- name: ListCurrentCustomEmojiImages :many
SELECT e.id, e.shortcode, e.domain, e.uri, e.created_at,
  i.id AS image_id, i.media_id AS image_media_id, i.revised_at AS image_revised_at,
  CAST(COALESCE(s.enabled, 1) AS INTEGER) AS enabled,
  CAST(COALESCE(s.visible_in_picker, 1) AS INTEGER) AS visible_in_picker,
  CAST(COALESCE(s.category, '') AS TEXT) AS category
FROM custom_emojis e
JOIN custom_emoji_images i ON i.emoji_id = e.id
LEFT JOIN custom_emoji_settings s ON s.emoji_id = e.id
WHERE e.id IN (SELECT value FROM json_each(CAST(@emoji_ids AS TEXT)))
  AND i.id = (SELECT MAX(l.id) FROM custom_emoji_images l WHERE l.emoji_id = e.id)
ORDER BY e.id;
