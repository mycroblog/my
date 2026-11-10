-- SPDX-License-Identifier: AGPL-3.0-only

-- The direct messages by conversation (data-model section 6.4, ADR-0154 section 6). Every
-- statement touches a set of rows at once: none runs once per party or per conversation of a
-- page. A list argument is a JSON array of strings, read with json_each.

-- name: InsertFeedConversationParticipants :exec
INSERT INTO feed_conversation_participants (conversation_id, entry_id, account_id)
SELECT CAST(sqlc.arg(conversation_id) AS TEXT), CAST(sqlc.arg(entry_id) AS TEXT), value
FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)) WHERE true
ON CONFLICT DO NOTHING;

-- AdvanceFeedConversations makes the entry the last one of its conversation for each of its
-- parties whose last one is older.
-- name: AdvanceFeedConversations :exec
INSERT INTO feed_conversations (account_id, conversation_id, last_entry_id)
SELECT p.account_id, p.conversation_id, p.entry_id FROM feed_conversation_participants p WHERE p.entry_id = sqlc.arg(entry_id)
ON CONFLICT (account_id, conversation_id) DO UPDATE SET last_entry_id = max(feed_conversations.last_entry_id, excluded.last_entry_id);

-- name: ListFeedConversationIDsOfEntries :many
SELECT DISTINCT conversation_id FROM feed_conversation_participants
WHERE entry_id IN (SELECT value FROM json_each(CAST(sqlc.arg(entry_ids) AS TEXT)))
ORDER BY conversation_id;

-- name: DeleteFeedConversationParticipantsOfEntries :exec
DELETE FROM feed_conversation_participants WHERE entry_id IN (SELECT value FROM json_each(CAST(sqlc.arg(entry_ids) AS TEXT)));

-- DeleteFeedConversationsOf and InsertFeedConversationsOf derive the rows of feed_conversations
-- of the conversations again from their participants.
-- name: DeleteFeedConversationsOf :exec
DELETE FROM feed_conversations WHERE conversation_id IN (SELECT value FROM json_each(CAST(sqlc.arg(conversation_ids) AS TEXT)));

-- name: InsertFeedConversationsOf :exec
INSERT INTO feed_conversations (account_id, conversation_id, last_entry_id)
SELECT account_id, conversation_id, max(entry_id) FROM feed_conversation_participants
WHERE conversation_id IN (SELECT value FROM json_each(CAST(sqlc.arg(conversation_ids) AS TEXT)))
GROUP BY account_id, conversation_id;

-- The merge of a conversation into another (content.conversation_merged): the participants move,
-- and each account keeps the newer of its two last entries, read boundaries, and hide
-- boundaries.

-- name: MoveFeedConversationParticipants :exec
UPDATE feed_conversation_participants SET conversation_id = sqlc.arg(into_id) WHERE conversation_id = sqlc.arg(merged_id);

-- name: MergeFeedConversations :exec
INSERT INTO feed_conversations (account_id, conversation_id, last_entry_id)
SELECT m.account_id, CAST(sqlc.arg(into_id) AS TEXT), m.last_entry_id FROM feed_conversations m WHERE m.conversation_id = sqlc.arg(merged_id)
ON CONFLICT (account_id, conversation_id) DO UPDATE SET last_entry_id = max(feed_conversations.last_entry_id, excluded.last_entry_id);

-- name: DeleteFeedConversationsOfConversation :exec
DELETE FROM feed_conversations WHERE conversation_id = sqlc.arg(conversation_id);

-- name: MergeFeedConversationReads :exec
INSERT INTO feed_conversation_reads (account_id, conversation_id, entry_id)
SELECT m.account_id, CAST(sqlc.arg(into_id) AS TEXT), m.entry_id FROM feed_conversation_reads m WHERE m.conversation_id = sqlc.arg(merged_id)
ON CONFLICT (account_id, conversation_id) DO UPDATE SET entry_id = max(feed_conversation_reads.entry_id, excluded.entry_id);

-- name: DeleteFeedConversationReadsOfConversation :exec
DELETE FROM feed_conversation_reads WHERE conversation_id = sqlc.arg(conversation_id);

-- name: MergeFeedConversationHides :exec
INSERT INTO feed_conversation_hides (account_id, conversation_id, entry_id)
SELECT m.account_id, CAST(sqlc.arg(into_id) AS TEXT), m.entry_id FROM feed_conversation_hides m WHERE m.conversation_id = sqlc.arg(merged_id)
ON CONFLICT (account_id, conversation_id) DO UPDATE SET entry_id = max(feed_conversation_hides.entry_id, excluded.entry_id);

-- name: DeleteFeedConversationHidesOfConversation :exec
DELETE FROM feed_conversation_hides WHERE conversation_id = sqlc.arg(conversation_id);

-- MarkFeedConversationRead and HideFeedConversation set the boundary of an account in a
-- conversation to its last entry there, keeping a newer one; they write nothing, and report no
-- row, for a conversation the account has no direct post in.
-- name: MarkFeedConversationRead :execrows
INSERT INTO feed_conversation_reads (account_id, conversation_id, entry_id)
SELECT c.account_id, c.conversation_id, c.last_entry_id FROM feed_conversations c
WHERE c.account_id = sqlc.arg(account_id) AND c.conversation_id = sqlc.arg(conversation_id)
ON CONFLICT (account_id, conversation_id) DO UPDATE SET entry_id = max(feed_conversation_reads.entry_id, excluded.entry_id);

-- name: HideFeedConversation :execrows
INSERT INTO feed_conversation_hides (account_id, conversation_id, entry_id)
SELECT c.account_id, c.conversation_id, c.last_entry_id FROM feed_conversations c
WHERE c.account_id = sqlc.arg(account_id) AND c.conversation_id = sqlc.arg(conversation_id)
ON CONFLICT (account_id, conversation_id) DO UPDATE SET entry_id = max(feed_conversation_hides.entry_id, excluded.entry_id);

-- ListFeedConversations is a page of the list of the viewer's conversations, by the last entry
-- newest first (keyset after before, '' for the first page), without the hidden ones: those
-- whose hide boundary is at or after the last entry.
-- name: ListFeedConversations :many
SELECT c.conversation_id, c.last_entry_id, CAST(COALESCE(r.entry_id, '') AS TEXT) AS read_entry_id
FROM feed_conversations c
LEFT JOIN feed_conversation_reads r ON r.account_id = c.account_id AND r.conversation_id = c.conversation_id
WHERE c.account_id = sqlc.arg(viewer)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR c.last_entry_id < CAST(sqlc.arg(before) AS TEXT))
  AND NOT EXISTS (
    SELECT 1 FROM feed_conversation_hides h
    WHERE h.account_id = c.account_id AND h.conversation_id = c.conversation_id AND h.entry_id >= c.last_entry_id)
ORDER BY c.last_entry_id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListFeedConversationsAmong :many
SELECT c.conversation_id, c.last_entry_id, CAST(COALESCE(r.entry_id, '') AS TEXT) AS read_entry_id,
  CAST(EXISTS (
    SELECT 1 FROM feed_conversation_hides h
    WHERE h.account_id = c.account_id AND h.conversation_id = c.conversation_id AND h.entry_id >= c.last_entry_id) AS INTEGER) AS hidden
FROM feed_conversations c
LEFT JOIN feed_conversation_reads r ON r.account_id = c.account_id AND r.conversation_id = c.conversation_id
WHERE c.account_id = sqlc.arg(viewer) AND c.conversation_id IN (SELECT value FROM json_each(CAST(sqlc.arg(conversation_ids) AS TEXT)))
ORDER BY c.last_entry_id DESC;

-- ListLastVisibleFeedConversationEntries returns, for each of the conversations, the newest
-- direct entry of the viewer's that its blocks, mutes, and the restrictions of the author and
-- the author's domain leave (as the home timeline does), and the newest of those by another
-- account ('' for none).
-- name: ListLastVisibleFeedConversationEntries :many
SELECT p.conversation_id, CAST(max(e.id) AS TEXT) AS last_entry_id,
  CAST(COALESCE(max(CASE WHEN e.author_account_id <> sqlc.arg(viewer) THEN e.id END), '') AS TEXT) AS last_other_entry_id
FROM feed_conversation_participants p
JOIN feed_entries e ON e.id = p.entry_id
WHERE p.account_id = sqlc.arg(viewer)
  AND p.conversation_id IN (SELECT value FROM json_each(CAST(sqlc.arg(conversation_ids) AS TEXT)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_exclusions x
    WHERE x.viewer_account_id = sqlc.arg(viewer) AND x.target_account_id = e.author_account_id
      AND (x.expires_at IS NULL OR x.expires_at > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_author_restrictions ar
    WHERE ar.account_id = e.author_account_id AND ar.restriction = 'profile_view'
      AND (ar.effective_until IS NULL OR ar.effective_until > CAST(sqlc.arg(now) AS INTEGER)))
  AND NOT EXISTS (
    SELECT 1 FROM feed_domain_restrictions dr
    WHERE dr.restriction = 'profile_view'
      AND (dr.effective_until IS NULL OR dr.effective_until > CAST(sqlc.arg(now) AS INTEGER))
      AND (dr.domain = e.author_domain OR substr(e.author_domain, -(length(dr.domain) + 1)) = '.' || dr.domain))
GROUP BY p.conversation_id
ORDER BY p.conversation_id;

-- ListFeedConversationParticipants returns the other parties of the viewer's direct entries in
-- each of the conversations: the accounts of the posts the viewer sent or received there.
-- name: ListFeedConversationParticipants :many
SELECT DISTINCT o.conversation_id, o.account_id
FROM feed_conversation_participants v
JOIN feed_conversation_participants o ON o.conversation_id = v.conversation_id AND o.entry_id = v.entry_id
WHERE v.account_id = sqlc.arg(viewer)
  AND v.conversation_id IN (SELECT value FROM json_each(CAST(sqlc.arg(conversation_ids) AS TEXT)))
  AND o.account_id <> sqlc.arg(viewer)
ORDER BY o.conversation_id, o.account_id;

-- name: ListFeedEntriesByID :many
SELECT id, kind, status_id, author_account_id, author_domain, original_author_account_id,
  original_author_domain, visibility, is_local, language, revision_seq, is_reply
FROM feed_entries WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY id;

-- The projections, for rebuilding and verifying them.

-- name: DeleteAllFeedConversationParticipants :exec
DELETE FROM feed_conversation_participants;

-- name: DeleteAllFeedConversations :exec
DELETE FROM feed_conversations;

-- name: ListAllFeedConversationParticipants :many
SELECT conversation_id, entry_id, account_id FROM feed_conversation_participants ORDER BY account_id, conversation_id, entry_id;

-- name: ListAllFeedConversations :many
SELECT account_id, conversation_id, last_entry_id FROM feed_conversations ORDER BY account_id, conversation_id;

-- The erasure of a deleted account's rows (data-model section 8), a bounded batch (lim) each.

-- name: EraseFeedConversationParticipantsOf :execrows
DELETE FROM feed_conversation_participants WHERE feed_conversation_participants.account_id = sqlc.arg(account_id) AND feed_conversation_participants.entry_id IN (
  SELECT p.entry_id FROM feed_conversation_participants p WHERE p.account_id = sqlc.arg(account_id)
  ORDER BY p.entry_id LIMIT sqlc.arg(lim));

-- name: EraseFeedConversationsOf :execrows
DELETE FROM feed_conversations WHERE feed_conversations.account_id = sqlc.arg(account_id) AND feed_conversations.conversation_id IN (
  SELECT c.conversation_id FROM feed_conversations c WHERE c.account_id = sqlc.arg(account_id)
  ORDER BY c.conversation_id LIMIT sqlc.arg(lim));

-- name: EraseFeedConversationReadsOf :execrows
DELETE FROM feed_conversation_reads WHERE feed_conversation_reads.account_id = sqlc.arg(account_id) AND feed_conversation_reads.conversation_id IN (
  SELECT r.conversation_id FROM feed_conversation_reads r WHERE r.account_id = sqlc.arg(account_id)
  ORDER BY r.conversation_id LIMIT sqlc.arg(lim));

-- name: EraseFeedConversationHidesOf :execrows
DELETE FROM feed_conversation_hides WHERE feed_conversation_hides.account_id = sqlc.arg(account_id) AND feed_conversation_hides.conversation_id IN (
  SELECT h.conversation_id FROM feed_conversation_hides h WHERE h.account_id = sqlc.arg(account_id)
  ORDER BY h.conversation_id LIMIT sqlc.arg(lim));
