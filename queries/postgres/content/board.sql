-- SPDX-License-Identifier: AGPL-3.0-only

-- The posts of the boards of the events (ADR-0161 section 17): a status with a
-- status_gathering_boards row is a post of the board of that event.

-- name: InsertStatusGatheringBoard :exec
INSERT INTO status_gathering_boards (status_id, gathering_id) VALUES ($1, $2);

-- ListStatusGatheringBoards returns the boards of the statuses among ids that are board posts.
-- name: ListStatusGatheringBoards :many
SELECT status_id, gathering_id FROM status_gathering_boards
WHERE status_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY status_id;

-- ListLiveBoardStatusIDs returns the posts of a board that are not deleted, newest first, below
-- an ID (NULL for the first page).
-- name: ListLiveBoardStatusIDs :many
SELECT b.status_id FROM status_gathering_boards b
WHERE b.gathering_id = sqlc.arg(gathering_id)
  AND (sqlc.narg(before)::uuid IS NULL OR b.status_id < sqlc.narg(before)::uuid)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id)
ORDER BY b.status_id DESC
LIMIT sqlc.arg(lim);

-- ListLiveBoardStatusIDsOfAuthor returns the board posts of an author that are not deleted,
-- newest first, below an ID (NULL for the first page): those of the author's data export.
-- name: ListLiveBoardStatusIDsOfAuthor :many
SELECT b.status_id FROM status_gathering_boards b JOIN statuses s ON s.id = b.status_id
WHERE s.author_account_id = sqlc.arg(author_account_id)
  AND (sqlc.narg(before)::uuid IS NULL OR b.status_id < sqlc.narg(before)::uuid)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id)
ORDER BY b.status_id DESC
LIMIT sqlc.arg(lim);

-- ListLiveBoardStatusIDsOfGatherings returns at most lim posts that are not deleted of the boards
-- of the events gathering_ids, in ID order.
-- name: ListLiveBoardStatusIDsOfGatherings :many
SELECT b.status_id FROM status_gathering_boards b
WHERE b.gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[])
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id)
ORDER BY b.status_id
LIMIT sqlc.arg(lim);

-- ListBoardStatusIDs returns every board post, in ID order: those that the replay of the statuses
-- leaves out.
-- name: ListBoardStatusIDs :many
SELECT status_id FROM status_gathering_boards ORDER BY status_id;

-- name: PurgeStatusGatheringBoards :exec
DELETE FROM status_gathering_boards WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);
