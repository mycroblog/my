-- SPDX-License-Identifier: AGPL-3.0-only

-- Conversations (ADR-0154, data-model 4.4): the resource, the conversation of each status, and
-- the merges.

-- InsertConversationIfAbsent inserts a conversation unless one has its URI.
-- name: InsertConversationIfAbsent :exec
INSERT INTO conversations (id, uri, is_local) VALUES (?, ?, ?)
ON CONFLICT (uri) DO NOTHING;

-- name: GetConversation :one
SELECT id, uri, is_local FROM conversations WHERE id = ?;

-- name: GetConversationByURI :one
SELECT id, uri, is_local FROM conversations WHERE uri = ?;

-- GetConversationMergeTarget returns the conversation that a merged conversation was merged into.
-- name: GetConversationMergeTarget :one
SELECT into_conversation_id FROM conversation_merges WHERE merged_conversation_id = ?;

-- name: GetConversationMerge :one
SELECT merged_conversation_id, into_conversation_id, status_id, merged_at
FROM conversation_merges WHERE merged_conversation_id = ?;

-- name: InsertConversationMerge :exec
INSERT INTO conversation_merges (merged_conversation_id, into_conversation_id, status_id, merged_at)
VALUES (?, ?, ?, ?);

-- MoveStatusConversations moves the statuses of a merged conversation to the one it is merged into.
-- name: MoveStatusConversations :exec
UPDATE status_conversations SET conversation_id = sqlc.arg(into_conversation_id)
WHERE conversation_id = sqlc.arg(merged_conversation_id);

-- name: InsertStatusConversation :exec
INSERT INTO status_conversations (status_id, conversation_id) VALUES (?, ?);

-- ListStatusConversations returns the conversation of each of the statuses @ids (a JSON array)
-- that has one.
-- name: ListStatusConversations :many
SELECT sc.status_id, c.id, c.uri, c.is_local
FROM status_conversations sc JOIN conversations c ON c.id = sc.conversation_id
WHERE sc.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY sc.status_id;

-- ListLiveConversationStatuses returns, in ID order after the cursor @after ('' for the first),
-- up to @lim statuses of a conversation that are not deleted.
-- name: ListLiveConversationStatuses :many
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri, s.published_at
FROM status_conversations sc JOIN statuses s ON s.id = sc.status_id
WHERE sc.conversation_id = sqlc.arg(conversation_id)
  AND sc.status_id > CAST(sqlc.arg(after) AS TEXT)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
ORDER BY sc.status_id
LIMIT sqlc.arg(lim);

-- ExistsLocalStatusInConversationOf reports whether a local status that is not deleted belongs
-- to the conversation of the status @status_id, for the scope of remote content (ADR-0006:
-- replies in a thread that has a local status).
-- name: ExistsLocalStatusInConversationOf :one
SELECT EXISTS (
  SELECT 1 FROM status_conversations p
    JOIN status_conversations sc ON sc.conversation_id = p.conversation_id
    JOIN statuses s ON s.id = sc.status_id
  WHERE p.status_id = sqlc.arg(status_id) AND s.is_local = 1
    AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
);
