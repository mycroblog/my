-- SPDX-License-Identifier: AGPL-3.0-only

-- Articles, the long posts in Markdown (ADR-0160 section 2): the article subtype of a status and the
-- article children of its revisions.

-- name: InsertStatusArticle :exec
INSERT INTO status_articles (status_id, author_account_id) VALUES ($1, $2);

-- name: InsertStatusRevisionArticle :exec
INSERT INTO status_revision_articles (revision_id, title, summary, summary_origin) VALUES ($1, $2, $3, $4);

-- name: InsertStatusRevisionArticleCover :exec
INSERT INTO status_revision_article_covers (revision_id, media_id, description) VALUES ($1, $2, $3);

-- name: InsertStatusRevisionArticleImage :exec
INSERT INTO status_revision_article_images (revision_id, position, media_id, description) VALUES ($1, $2, $3, $4);

-- name: ListLiveArticleIDsOfAuthor :many
-- The articles of an author that are not deleted, newest first, below an ID (NULL for the
-- first page): the articles of a profile (ADR-0160 section 11).
SELECT a.status_id FROM status_articles a
WHERE a.author_account_id = sqlc.arg(author_account_id)
  AND (sqlc.narg(before)::uuid IS NULL OR a.status_id < sqlc.narg(before)::uuid)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = a.status_id)
ORDER BY a.status_id DESC
LIMIT sqlc.arg(lim);

-- The Purge* statements of the article rows of deleted statuses (@ids), with those of retention.sql.

-- name: PurgeStatusRevisionArticles :exec
DELETE FROM status_revision_articles WHERE revision_id IN (
  SELECT r.id FROM status_revisions r WHERE r.status_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeStatusRevisionArticleCovers :exec
DELETE FROM status_revision_article_covers WHERE revision_id IN (
  SELECT r.id FROM status_revisions r WHERE r.status_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeStatusRevisionArticleImages :exec
DELETE FROM status_revision_article_images WHERE revision_id IN (
  SELECT r.id FROM status_revisions r WHERE r.status_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeStatusArticles :exec
DELETE FROM status_articles WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);
