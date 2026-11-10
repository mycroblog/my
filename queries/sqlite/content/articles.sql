-- SPDX-License-Identifier: AGPL-3.0-only

-- Articles, the long posts in Markdown (ADR-0160 section 2): the article subtype of a status and the
-- article children of its revisions.

-- name: InsertStatusArticle :exec
INSERT INTO status_articles (status_id, author_account_id) VALUES (?, ?);

-- name: InsertStatusRevisionArticle :exec
INSERT INTO status_revision_articles (revision_id, title, summary, summary_origin) VALUES (?, ?, ?, ?);

-- name: InsertStatusRevisionArticleCover :exec
INSERT INTO status_revision_article_covers (revision_id, media_id, description) VALUES (?, ?, ?);

-- name: InsertStatusRevisionArticleImage :exec
INSERT INTO status_revision_article_images (revision_id, position, media_id, description) VALUES (?, ?, ?, ?);

-- name: GetStatusRevisionArticle :one
SELECT revision_id, title, summary, summary_origin FROM status_revision_articles WHERE revision_id = ?;

-- name: GetStatusRevisionArticleCover :one
SELECT revision_id, media_id, description FROM status_revision_article_covers WHERE revision_id = ?;

-- name: ListStatusRevisionArticleImages :many
SELECT revision_id, position, media_id, description FROM status_revision_article_images
WHERE revision_id = ? ORDER BY position;

-- name: ListLiveArticleIDsOfAuthor :many
-- The articles of an author that are not deleted, newest first, below an ID ('' for the
-- first page): the articles of a profile (ADR-0160 section 11).
SELECT a.status_id FROM status_articles a
WHERE a.author_account_id = sqlc.arg(author_account_id)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR a.status_id < sqlc.arg(before))
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = a.status_id)
ORDER BY a.status_id DESC
LIMIT sqlc.arg(lim);

-- The article children of many revisions, read as the queries above read one (FindCurrentRevisions).
-- The lists are JSON arrays.

-- name: ListStatusRevisionArticlesOf :many
SELECT revision_id, title, summary, summary_origin FROM status_revision_articles
WHERE revision_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY revision_id;

-- name: ListStatusRevisionArticleCoversOf :many
SELECT revision_id, media_id, description FROM status_revision_article_covers
WHERE revision_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY revision_id;

-- name: ListStatusRevisionArticleImagesOf :many
SELECT revision_id, position, media_id, description FROM status_revision_article_images
WHERE revision_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY revision_id, position;

-- The Purge* statements of the article rows of deleted statuses (@ids), with those of retention.sql.

-- name: PurgeStatusRevisionArticles :exec
DELETE FROM status_revision_articles WHERE revision_id IN (
  SELECT r.id FROM status_revisions r WHERE r.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: PurgeStatusRevisionArticleCovers :exec
DELETE FROM status_revision_article_covers WHERE revision_id IN (
  SELECT r.id FROM status_revisions r WHERE r.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: PurgeStatusRevisionArticleImages :exec
DELETE FROM status_revision_article_images WHERE revision_id IN (
  SELECT r.id FROM status_revisions r WHERE r.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: PurgeStatusArticles :exec
DELETE FROM status_articles WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));
