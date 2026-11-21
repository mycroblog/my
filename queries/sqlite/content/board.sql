-- SPDX-License-Identifier: AGPL-3.0-only

-- The posts of the boards of the events (ADR-0161 section 17): a status with a
-- status_gathering_boards row is a post of the board of that event. The lists are JSON arrays.

-- name: InsertStatusGatheringBoard :exec
INSERT INTO status_gathering_boards (status_id, gathering_id) VALUES (?, ?);

-- ListStatusGatheringBoards returns the boards of the statuses among ids that are board posts.
-- name: ListStatusGatheringBoards :many
SELECT status_id, gathering_id FROM status_gathering_boards
WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)))
ORDER BY status_id;

-- ListLiveBoardStatusIDs returns the posts of a board that are not deleted, newest first, below
-- an ID ('' for the first page).
-- name: ListLiveBoardStatusIDs :many
SELECT b.status_id FROM status_gathering_boards b
WHERE b.gathering_id = sqlc.arg(gathering_id)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR b.status_id < sqlc.arg(before))
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id)
ORDER BY b.status_id DESC
LIMIT sqlc.arg(lim);

-- ListLiveBoardStatusIDsOfAuthor returns the board posts of an author that are not deleted,
-- newest first, below an ID ('' for the first page): those of the author's data export.
-- name: ListLiveBoardStatusIDsOfAuthor :many
SELECT b.status_id FROM status_gathering_boards b JOIN statuses s ON s.id = b.status_id
WHERE s.author_account_id = sqlc.arg(author_account_id)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR b.status_id < sqlc.arg(before))
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id)
ORDER BY b.status_id DESC
LIMIT sqlc.arg(lim);

-- ListLiveBoardStatusesOfGatherings returns at most lim posts that are not deleted of the boards
-- of the events gathering_ids, in ID order.
-- name: ListLiveBoardStatusesOfGatherings :many
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri, s.published_at
FROM statuses s
WHERE s.id IN (SELECT b.status_id FROM status_gathering_boards b WHERE b.gathering_id IN (SELECT value FROM json_each(CAST(sqlc.arg(gathering_ids) AS TEXT))))
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id)
ORDER BY s.id
LIMIT sqlc.arg(lim);

-- ListBoardStatusIDs returns every board post, in ID order: those that the replay of the statuses
-- leaves out.
-- name: ListBoardStatusIDs :many
SELECT status_id FROM status_gathering_boards ORDER BY status_id;

-- name: PurgeStatusGatheringBoards :exec
DELETE FROM status_gathering_boards WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- ListBoardGatheringIDs returns at most lim events, in ID order after the event after ('' for the
-- first), whose boards have posts that are not deleted.
-- name: ListBoardGatheringIDs :many
SELECT DISTINCT b.gathering_id FROM status_gathering_boards b
WHERE b.gathering_id > CAST(sqlc.arg(after) AS TEXT)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id)
ORDER BY b.gathering_id
LIMIT sqlc.arg(lim);
