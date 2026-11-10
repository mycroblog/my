-- SPDX-License-Identifier: AGPL-3.0-only

-- The link cards of many statuses at once, one query per part, for the pages that present many
-- statuses. Each reads what the query of one of link_card.sql reads.

-- name: ListStatusLinkCardURLs :many
SELECT status_id, url FROM status_link_cards WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: ListLinkCardsByURLs :many
SELECT url, card_url, title, description, site_name, author_name, published_at, image_media_id,
  creator_account_id, language, fetched_at
FROM link_cards WHERE url = ANY(sqlc.arg(urls)::text[]) ORDER BY url;

-- name: ListLinkCardMediaByIDs :many
SELECT media_id, remote_url, content_type, width, height, blurhash, description
FROM link_card_media WHERE media_id = ANY(sqlc.arg(media_ids)::uuid[]) ORDER BY media_id;
