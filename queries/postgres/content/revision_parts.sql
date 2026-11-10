-- SPDX-License-Identifier: AGPL-3.0-only

-- The current revisions of many statuses and their parts, one query per part, for the pages that
-- present many statuses (FindCurrentRevisions). Each reads what the query of one revision of
-- content.sql and remote_status.sql reads, in the same order within a revision.

-- name: ListCurrentStatusRevisions :many
SELECT r.id, r.status_id, r.seq, r.spoiler_text, r.sensitive, r.language, r.revised_at
FROM status_heads h JOIN status_revisions r ON r.id = h.revision_id
WHERE h.status_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY r.status_id;

-- name: ListStatusRevisionBodiesOf :many
SELECT revision_id, language, content_html FROM status_revision_bodies WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, language;

-- name: ListStatusRevisionSourcesOf :many
SELECT revision_id, source_text, source_format FROM status_revision_sources WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id;

-- name: ListStatusRevisionAttachmentsOf :many
SELECT revision_id, position, media_id, description FROM status_revision_attachments
WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, position;

-- name: ListStatusRevisionMentionsOf :many
SELECT revision_id, account_id FROM status_revision_mentions WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, account_id;

-- name: ListStatusRevisionTagsOf :many
SELECT revision_id, tag_id FROM status_revision_tags WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, tag_id;

-- name: ListStatusRevisionEmojisOf :many
SELECT revision_id, emoji_id FROM status_revision_emojis WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id, emoji_id;

-- name: ListPollsOfRevisions :many
SELECT id, revision_id, multiple, hide_totals, closes_at FROM polls WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id;

-- name: ListPollOptionsOf :many
SELECT poll_id, position, title FROM poll_options WHERE poll_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY poll_id, position;

-- name: ListRemoteStatusRevisionsOf :many
SELECT revision_id, activity_uri, html_policy_version, omitted_attachment_count FROM remote_status_revisions
WHERE revision_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY revision_id;
