-- SPDX-License-Identifier: AGPL-3.0-only

-- The erasure of deleted accounts' data (data-model sections 4.2 and 8, #397). Each module
-- erases its own tables; the statements whose rows grow with the number of users or followers
-- take a bounded batch (@lim). A list argument is an array of strings.

-- name: ListFeedEntryIDsToErase :many
SELECT id FROM feed_entries
WHERE author_account_id = sqlc.arg(account_id) OR original_author_account_id = sqlc.arg(account_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseFeedDirectRecipientsOfEntries :exec
DELETE FROM feed_direct_recipients WHERE entry_id = ANY(sqlc.arg(ids)::uuid[]);

-- CountUsedFeedTagsOfEntries returns how many of the entries among ids that tag_usage counts
-- carry each tag.
-- name: CountUsedFeedTagsOfEntries :many
SELECT tag_id, count(*) AS n FROM feed_tag_entries
WHERE entry_id = ANY(sqlc.arg(ids)::uuid[]) AND visibility IN ('public', 'local', 'unlisted')
GROUP BY tag_id ORDER BY tag_id;

-- name: EraseFeedTagEntriesOfEntries :exec
DELETE FROM feed_tag_entries WHERE entry_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFeedEntries :exec
DELETE FROM feed_entries WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFeedDirectRecipientsOf :execrows
DELETE FROM feed_direct_recipients WHERE feed_direct_recipients.recipient_account_id = sqlc.arg(account_id) AND feed_direct_recipients.entry_id IN (
  SELECT r.entry_id FROM feed_direct_recipients r WHERE r.recipient_account_id = sqlc.arg(account_id)
  ORDER BY r.entry_id LIMIT sqlc.arg(lim));

-- name: EraseFeedFollowEdgesAsFollower :execrows
DELETE FROM feed_follow_edges WHERE feed_follow_edges.follower_account_id = sqlc.arg(account_id) AND feed_follow_edges.followee_account_id IN (
  SELECT e.followee_account_id FROM feed_follow_edges e WHERE e.follower_account_id = sqlc.arg(account_id)
  ORDER BY e.followee_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedFollowEdgesAsFollowee :execrows
DELETE FROM feed_follow_edges WHERE feed_follow_edges.followee_account_id = sqlc.arg(account_id) AND feed_follow_edges.follower_account_id IN (
  SELECT e.follower_account_id FROM feed_follow_edges e WHERE e.followee_account_id = sqlc.arg(account_id)
  ORDER BY e.follower_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedExclusionsAsViewer :execrows
DELETE FROM feed_exclusions WHERE feed_exclusions.viewer_account_id = sqlc.arg(account_id) AND feed_exclusions.target_account_id IN (
  SELECT x.target_account_id FROM feed_exclusions x WHERE x.viewer_account_id = sqlc.arg(account_id)
  ORDER BY x.target_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedExclusionsAsTarget :execrows
DELETE FROM feed_exclusions WHERE feed_exclusions.target_account_id = sqlc.arg(account_id) AND feed_exclusions.viewer_account_id IN (
  SELECT x.viewer_account_id FROM feed_exclusions x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.viewer_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedDomainMutesOf :execrows
DELETE FROM feed_domain_mutes WHERE feed_domain_mutes.viewer_account_id = sqlc.arg(account_id) AND feed_domain_mutes.domain IN (
  SELECT m.domain FROM feed_domain_mutes m WHERE m.viewer_account_id = sqlc.arg(account_id)
  ORDER BY m.domain LIMIT sqlc.arg(lim));

-- The lists of an account and its membership in the lists of others (data-model 4.6). A list's
-- name goes before the list, and the steps pick the same first lists by ID.

-- name: EraseListMembersAsOwner :execrows
DELETE FROM list_members WHERE list_members.owner_account_id = sqlc.arg(account_id) AND list_members.list_id IN (
  SELECT m.list_id FROM list_members m WHERE m.owner_account_id = sqlc.arg(account_id)
  ORDER BY m.list_id LIMIT sqlc.arg(lim));

-- name: EraseListMembersAsMember :execrows
DELETE FROM list_members WHERE list_members.member_account_id = sqlc.arg(account_id) AND list_members.list_id IN (
  SELECT m.list_id FROM list_members m WHERE m.member_account_id = sqlc.arg(account_id)
  ORDER BY m.list_id LIMIT sqlc.arg(lim));

-- name: EraseListNamesOf :execrows
DELETE FROM list_names WHERE list_names.list_id IN (
  SELECT l.id FROM lists l WHERE l.owner_account_id = sqlc.arg(account_id)
  ORDER BY l.id LIMIT sqlc.arg(lim));

-- name: EraseListsOf :execrows
DELETE FROM lists WHERE lists.owner_account_id = sqlc.arg(account_id) AND lists.id IN (
  SELECT l.id FROM lists l WHERE l.owner_account_id = sqlc.arg(account_id)
  ORDER BY l.id LIMIT sqlc.arg(lim));
