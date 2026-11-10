-- SPDX-License-Identifier: AGPL-3.0-only

-- Keyword filters (data-model 4.6, ADR-0157): the account's commands write keyword_filters and
-- their settings, contexts, and words; the reads that show statuses read the active filters of
-- their viewers. A list argument is a JSON array of strings, read with json_each.

-- name: InsertKeywordFilter :exec
INSERT INTO keyword_filters (id, account_id, created_at) VALUES (?, ?, ?);

-- name: UpsertKeywordFilterSettings :exec
INSERT INTO keyword_filter_settings (filter_id, title, action, expires_at) VALUES (?, ?, ?, ?)
ON CONFLICT (filter_id) DO UPDATE SET title = excluded.title, action = excluded.action, expires_at = excluded.expires_at;

-- name: InsertKeywordFilterContext :exec
INSERT INTO keyword_filter_contexts (filter_id, context) VALUES (?, ?);

-- name: DeleteKeywordFilterContexts :exec
DELETE FROM keyword_filter_contexts WHERE filter_id = ?;

-- name: InsertKeywordFilterWord :exec
INSERT INTO keyword_filter_words (filter_id, normalized_word, word, whole_word, position) VALUES (?, ?, ?, ?, ?);

-- name: DeleteKeywordFilterWords :exec
DELETE FROM keyword_filter_words WHERE filter_id = ?;

-- name: DeleteKeywordFilterSettings :exec
DELETE FROM keyword_filter_settings WHERE filter_id = ?;

-- name: DeleteKeywordFilter :exec
DELETE FROM keyword_filters WHERE id = ?;

-- name: GetKeywordFilter :one
SELECT f.id, f.account_id, f.created_at, s.title, s.action, s.expires_at
FROM keyword_filters f JOIN keyword_filter_settings s ON s.filter_id = f.id
WHERE f.id = ?;

-- ListKeywordFilters returns the filters of an account, oldest first.
-- name: ListKeywordFilters :many
SELECT f.id, f.account_id, f.created_at, s.title, s.action, s.expires_at
FROM keyword_filters f JOIN keyword_filter_settings s ON s.filter_id = f.id
WHERE f.account_id = ?
ORDER BY f.id;

-- name: ListKeywordFilterContexts :many
SELECT c.filter_id, c.context FROM keyword_filter_contexts c
WHERE c.filter_id IN (SELECT value FROM json_each(CAST(sqlc.arg(filter_ids) AS TEXT)))
ORDER BY c.filter_id, c.context;

-- name: ListKeywordFilterWords :many
SELECT w.filter_id, w.word, w.normalized_word, w.whole_word FROM keyword_filter_words w
WHERE w.filter_id IN (SELECT value FROM json_each(CAST(sqlc.arg(filter_ids) AS TEXT)))
ORDER BY w.filter_id, w.position;

-- CountKeywordFilters returns how many filters an account has and how many words they have in all.
-- name: CountKeywordFilters :one
SELECT CAST(count(DISTINCT f.id) AS INTEGER) AS filters, CAST(count(w.filter_id) AS INTEGER) AS words
FROM keyword_filters f LEFT JOIN keyword_filter_words w ON w.filter_id = f.id
WHERE f.account_id = ?;

-- ListActiveKeywordFilterWords returns the words of the filters of the accounts that apply in a
-- context at a time (no expiry, or one after it), by account, filter, and position, with what
-- the matches show of their filter.
-- name: ListActiveKeywordFilterWords :many
SELECT f.account_id, f.id, s.title, s.action, w.word, w.normalized_word, w.whole_word
FROM keyword_filters f
JOIN keyword_filter_settings s ON s.filter_id = f.id
JOIN keyword_filter_contexts c ON c.filter_id = f.id AND c.context = sqlc.arg(context)
JOIN keyword_filter_words w ON w.filter_id = f.id
WHERE f.account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)))
  AND (s.expires_at IS NULL OR s.expires_at > CAST(sqlc.arg(now) AS INTEGER))
ORDER BY f.account_id, f.id, w.position;

-- ListKeywordFiltersExpiredBefore returns at most lim filters whose expiry is before a time, the
-- earliest first, for the cleanup job.
-- name: ListKeywordFiltersExpiredBefore :many
SELECT filter_id FROM keyword_filter_settings
WHERE expires_at IS NOT NULL AND expires_at < CAST(sqlc.arg(before) AS INTEGER)
ORDER BY expires_at, filter_id
LIMIT sqlc.arg(lim);
