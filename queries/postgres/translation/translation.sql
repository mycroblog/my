-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the translation module (ADR-0148, data-model section 4.17), owned by
-- internal/app/translation.

-- InsertStatusTranslation adds a translation unless one of the same revision, language, and
-- provider exists, which a request that lost the lock may have stored first.
-- name: InsertStatusTranslation :execrows
INSERT INTO status_translations (
  id, status_id, revision_id, target_language, provider, source_language, content_html, spoiler_text, created_at
) VALUES (
  @id, @status_id, @revision_id, @target_language, @provider, @source_language, @content_html, @spoiler_text, @created_at
)
ON CONFLICT (revision_id, target_language, provider) DO NOTHING;

-- name: InsertStatusTranslationAttachment :exec
INSERT INTO status_translation_attachments (translation_id, position, description)
VALUES (@translation_id, @position, @description);

-- name: InsertStatusTranslationPollOption :exec
INSERT INTO status_translation_poll_options (translation_id, position, title)
VALUES (@translation_id, @position, @title);

-- name: ListStatusTranslations :many
SELECT id, status_id, revision_id, target_language, provider, source_language, content_html, spoiler_text, created_at
FROM status_translations
WHERE revision_id = @revision_id AND target_language = @target_language;

-- name: ListStatusTranslationAttachments :many
SELECT position, description FROM status_translation_attachments
WHERE translation_id = @translation_id ORDER BY position;

-- name: ListStatusTranslationPollOptions :many
SELECT position, title FROM status_translation_poll_options
WHERE translation_id = @translation_id ORDER BY position;

-- name: ListStatusTranslationIDs :many
SELECT id FROM status_translations WHERE status_id = @status_id ORDER BY id;

-- ListExpiredStatusTranslationIDs returns at most @batch_limit translations created before
-- @created_before, for the periodic job.
-- name: ListExpiredStatusTranslationIDs :many
SELECT id FROM status_translations WHERE created_at < @created_before ORDER BY created_at, id LIMIT @batch_limit;

-- name: DeleteStatusTranslationAttachments :exec
DELETE FROM status_translation_attachments WHERE translation_id = @translation_id;

-- name: DeleteStatusTranslationPollOptions :exec
DELETE FROM status_translation_poll_options WHERE translation_id = @translation_id;

-- name: DeleteStatusTranslation :exec
DELETE FROM status_translations WHERE id = @id;

-- name: EnsureTranslationAccountUsage :exec
INSERT INTO translation_account_usage (account_id, month, characters) VALUES (@account_id, @month, 0)
ON CONFLICT (account_id, month) DO NOTHING;

-- name: AddTranslationAccountUsageWithin :execrows
UPDATE translation_account_usage SET characters = characters + @characters
WHERE account_id = @account_id AND month = @month AND characters + @characters <= @max_characters;

-- name: EnsureTranslationServerUsage :exec
INSERT INTO translation_server_usage (month) VALUES (@month)
ON CONFLICT (month) DO NOTHING;

-- name: AddTranslationServerUsageWithin :execrows
UPDATE translation_server_usage SET characters = characters + @characters
WHERE month = @month AND characters + @characters <= @max_characters;

-- name: AddTranslationCacheLookups :exec
INSERT INTO translation_server_usage (month, cache_hits, cache_misses) VALUES (@month, @hits, @misses)
ON CONFLICT (month) DO UPDATE SET
  cache_hits = translation_server_usage.cache_hits + excluded.cache_hits,
  cache_misses = translation_server_usage.cache_misses + excluded.cache_misses;

-- name: GetTranslationAccountUsage :one
SELECT characters FROM translation_account_usage WHERE account_id = @account_id AND month = @month;

-- name: GetTranslationServerUsage :one
SELECT characters, cache_hits, cache_misses FROM translation_server_usage WHERE month = @month;

-- name: DeleteTranslationAccountUsage :exec
DELETE FROM translation_account_usage WHERE account_id = @account_id;

-- name: DeleteTranslationAccountUsageBefore :exec
DELETE FROM translation_account_usage WHERE month < @month;

-- name: DeleteTranslationServerUsageBefore :exec
DELETE FROM translation_server_usage WHERE month < @month;

-- The translations of many statuses, and the rows of many translations, each statement as the one
-- of one above does for every one it names.

-- name: ListStatusTranslationIDsOf :many
SELECT id FROM status_translations WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY id;

-- name: DeleteStatusTranslationAttachmentsOf :exec
DELETE FROM status_translation_attachments WHERE translation_id = ANY(sqlc.arg(translation_ids)::uuid[]);

-- name: DeleteStatusTranslationPollOptionsOf :exec
DELETE FROM status_translation_poll_options WHERE translation_id = ANY(sqlc.arg(translation_ids)::uuid[]);

-- name: DeleteStatusTranslationsByIDs :exec
DELETE FROM status_translations WHERE id = ANY(sqlc.arg(translation_ids)::uuid[]);
