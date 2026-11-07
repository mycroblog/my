-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the link cards (ADR-0062, data-model section 4.4), owned by internal/app/content.

-- name: UpsertStatusLinkCard :exec
INSERT INTO status_link_cards (status_id, url) VALUES (@status_id, @url)
ON CONFLICT (status_id) DO UPDATE SET url = excluded.url;

-- name: DeleteStatusLinkCard :exec
DELETE FROM status_link_cards WHERE status_id = @status_id;

-- name: GetStatusLinkCardURL :one
SELECT url FROM status_link_cards WHERE status_id = @status_id;

-- name: GetLinkCard :one
SELECT url, card_url, title, description, site_name, author_name, published_at, image_media_id,
  creator_account_id, language, fetched_at
FROM link_cards WHERE url = @url;

-- name: UpsertLinkCard :exec
INSERT INTO link_cards (
  url, card_url, title, description, site_name, author_name, published_at, image_media_id,
  creator_account_id, language, fetched_at
) VALUES (
  @url, @card_url, @title, @description, @site_name, @author_name, @published_at, @image_media_id,
  @creator_account_id, @language, @fetched_at
)
ON CONFLICT (url) DO UPDATE SET
  card_url = excluded.card_url,
  title = excluded.title,
  description = excluded.description,
  site_name = excluded.site_name,
  author_name = excluded.author_name,
  published_at = excluded.published_at,
  image_media_id = excluded.image_media_id,
  creator_account_id = excluded.creator_account_id,
  language = excluded.language,
  fetched_at = excluded.fetched_at;

-- ListUnreferencedLinkCards returns at most @batch_limit cards fetched before @fetched_before
-- that no status refers to, for the cleanup.
-- name: ListUnreferencedLinkCards :many
SELECT c.url, c.card_url, c.title, c.description, c.site_name, c.author_name, c.published_at, c.image_media_id,
  c.creator_account_id, c.language, c.fetched_at
FROM link_cards c
WHERE c.fetched_at < @fetched_before
  AND NOT EXISTS (SELECT 1 FROM status_link_cards s WHERE s.url = c.url)
ORDER BY c.url LIMIT @batch_limit;

-- DeleteLinkCard deletes the card of @url; the statuses keep their references, and get a card when
-- one is fetched again.
-- name: DeleteLinkCard :exec
DELETE FROM link_cards WHERE url = @url;

-- DeleteUnreferencedLinkCard deletes the card of @url unless it was fetched again after the
-- cleanup read it or a status refers to it again.
-- name: DeleteUnreferencedLinkCard :execrows
DELETE FROM link_cards
WHERE link_cards.url = @url AND link_cards.fetched_at = @fetched_at
  AND NOT EXISTS (SELECT 1 FROM status_link_cards s WHERE s.url = @url);

-- name: InsertLinkCardMedia :execrows
INSERT INTO link_card_media (media_id, remote_url, content_type, width, height, blurhash, description)
VALUES (@media_id, @remote_url, @content_type, @width, @height, @blurhash, @description)
ON CONFLICT (remote_url) DO NOTHING;

-- name: GetLinkCardMediaByURL :one
SELECT media_id, remote_url, content_type, width, height, blurhash, description
FROM link_card_media WHERE remote_url = @remote_url;

-- name: GetLinkCardMedia :one
SELECT media_id, remote_url, content_type, width, height, blurhash, description
FROM link_card_media WHERE media_id = @media_id;

-- name: ListUnreferencedLinkCardMedia :many
SELECT m.media_id FROM link_card_media m
WHERE NOT EXISTS (SELECT 1 FROM link_cards c WHERE c.image_media_id = m.media_id)
ORDER BY m.media_id LIMIT @batch_limit;

-- name: DeleteUnreferencedLinkCardMedia :execrows
DELETE FROM link_card_media
WHERE link_card_media.media_id = @media_id
  AND NOT EXISTS (SELECT 1 FROM link_cards c WHERE c.image_media_id = @media_id);

-- DeleteLinkCardMediaFile deletes the media file of an image of link cards, which has no owner.
-- name: DeleteLinkCardMediaFile :exec
DELETE FROM media_files WHERE media_files.id = @media_id AND media_files.origin = 'remote' AND media_files.owner_account_id IS NULL;

-- name: EraseLinkCardsOfStatuses :exec
DELETE FROM status_link_cards WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));
