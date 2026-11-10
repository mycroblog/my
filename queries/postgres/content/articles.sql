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

-- name: GetStatusRevisionArticle :one
SELECT revision_id, title, summary, summary_origin FROM status_revision_articles WHERE revision_id = $1;

-- name: GetStatusRevisionArticleCover :one
SELECT revision_id, media_id, description FROM status_revision_article_covers WHERE revision_id = $1;

-- name: ListStatusRevisionArticleImages :many
SELECT revision_id, position, media_id, description FROM status_revision_article_images
WHERE revision_id = $1 ORDER BY position;

-- The article children of many revisions, read as the queries above read one (FindCurrentRevisions).

-- name: ListStatusRevisionArticlesOf :many
SELECT revision_id, title, summary, summary_origin FROM status_revision_articles
WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id;

-- name: ListStatusRevisionArticleCoversOf :many
SELECT revision_id, media_id, description FROM status_revision_article_covers
WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id;

-- name: ListStatusRevisionArticleImagesOf :many
SELECT revision_id, position, media_id, description FROM status_revision_article_images
WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, position;

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
