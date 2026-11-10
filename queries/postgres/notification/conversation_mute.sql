-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Thread mutes: the conversations that local accounts mute (notification, ADR-0158 section 8,
-- data-model section 4.7).

-- InsertConversationMute inserts nothing when the account already mutes the conversation.
-- name: InsertConversationMute :execrows
INSERT INTO conversation_mutes (account_id, conversation_id, created_at) VALUES ($1, $2, $3)
ON CONFLICT (account_id, conversation_id) DO NOTHING;

-- name: DeleteConversationMute :execrows
DELETE FROM conversation_mutes WHERE account_id = $1 AND conversation_id = $2;

-- The accounts among account_ids that mute the conversation.
-- name: ListConversationMuters :many
SELECT account_id FROM conversation_mutes
WHERE conversation_id = sqlc.arg(conversation_id)
  AND account_id = ANY(sqlc.arg(account_ids)::uuid[]);

-- The mutes of the accounts among account_ids of the conversations among conversation_ids.
-- name: ListConversationMutesAmong :many
SELECT account_id, conversation_id, created_at FROM conversation_mutes
WHERE conversation_id = ANY(sqlc.arg(conversation_ids)::uuid[])
  AND account_id = ANY(sqlc.arg(account_ids)::uuid[]);

-- MoveConversationMutes gives the mutes of a merged conversation to the one it was merged into;
-- an account that mutes both keeps its mute of the latter. DeleteMergedConversationMutes then
-- deletes the mutes of the merged conversation.
-- name: MoveConversationMutes :exec
INSERT INTO conversation_mutes (account_id, conversation_id, created_at)
SELECT m.account_id, sqlc.arg(into_conversation_id)::uuid, m.created_at
FROM conversation_mutes m WHERE m.conversation_id = sqlc.arg(merged_conversation_id)
ON CONFLICT (account_id, conversation_id) DO NOTHING;

-- name: DeleteMergedConversationMutes :exec
DELETE FROM conversation_mutes WHERE conversation_id = $1;
