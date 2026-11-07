-- SPDX-License-Identifier: AGPL-3.0-only

-- Boosts (resource) and their retractions (event).

-- name: InsertBoost :exec
INSERT INTO boosts (id, uri, booster_account_id, status_id, visibility, is_local, boosted_at) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: InsertBoostRetraction :exec
INSERT INTO boost_retractions (boost_id, origin, retracted_at) VALUES ($1, $2, $3);

-- name: GetBoost :one
SELECT id, uri, booster_account_id, status_id, visibility, is_local, boosted_at FROM boosts WHERE id = $1;

-- name: GetBoostByURI :one
SELECT id, uri, booster_account_id, status_id, visibility, is_local, boosted_at FROM boosts WHERE uri = $1;

-- name: GetBoostRetraction :one
SELECT boost_id, origin, retracted_at FROM boost_retractions WHERE boost_id = $1;

-- Projection boost_edges.

-- name: GetBoostEdge :one
SELECT booster_account_id, status_id, boost_id FROM boost_edges WHERE booster_account_id = $1 AND status_id = $2;

-- InsertBoostEdge inserts nothing when the pair already has a live boost.
-- name: InsertBoostEdge :execrows
INSERT INTO boost_edges (booster_account_id, status_id, boost_id) VALUES ($1, $2, $3)
ON CONFLICT (booster_account_id, status_id) DO NOTHING;

-- name: DeleteBoostEdge :execrows
DELETE FROM boost_edges WHERE booster_account_id = $1 AND status_id = $2 AND boost_id = $3;

-- name: DeleteStatusBoostEdges :exec
DELETE FROM boost_edges WHERE status_id = $1;

-- name: DeleteAllBoostEdges :exec
DELETE FROM boost_edges;

-- RebuildBoostEdges states the projector's rule: boosts that are not retracted, of statuses
-- that are not deleted.
-- name: RebuildBoostEdges :exec
INSERT INTO boost_edges (booster_account_id, status_id, boost_id)
SELECT b.booster_account_id, b.status_id, b.id
FROM boosts b
WHERE NOT EXISTS (SELECT 1 FROM boost_retractions r WHERE r.boost_id = b.id)
  AND NOT EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = b.status_id);

-- name: ListBoostEdges :many
SELECT booster_account_id, status_id, boost_id FROM boost_edges ORDER BY booster_account_id, status_id;

-- The status_counters columns that the interactions change.

-- name: AddStatusBoosts :exec
UPDATE status_counters SET boosts_count = boosts_count + sqlc.arg(delta) WHERE status_id = sqlc.arg(status_id);

-- name: AddStatusFavourites :exec
UPDATE status_counters SET favourites_count = favourites_count + sqlc.arg(delta) WHERE status_id = sqlc.arg(status_id);

-- name: CountStatusMention :one
SELECT COUNT(*) FROM status_mention_index WHERE status_id = $1 AND account_id = $2;

-- Favourites, bookmarks, and pins (current values). Each insert does nothing when the account
-- already has the row for the status.

-- name: InsertFavourite :execrows
INSERT INTO favourites (account_id, status_id, uri, created_at, id) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, status_id) DO NOTHING;

-- name: DeleteFavourite :many
DELETE FROM favourites WHERE account_id = $1 AND status_id = $2
RETURNING account_id, status_id, uri, created_at, id;

-- name: GetFavourite :one
SELECT account_id, status_id, uri, created_at, id FROM favourites WHERE account_id = $1 AND status_id = $2;

-- name: GetFavouriteByURI :one
SELECT account_id, status_id, uri, created_at, id FROM favourites WHERE uri = $1;

-- name: ListFavourites :many
SELECT account_id, status_id, uri, created_at, id FROM favourites
WHERE account_id = sqlc.arg(account_id) AND (sqlc.narg(before)::uuid IS NULL OR id < sqlc.narg(before)::uuid)
ORDER BY id DESC LIMIT sqlc.arg(page_limit);

-- name: InsertBookmark :execrows
INSERT INTO bookmarks (account_id, status_id, created_at, id) VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id, status_id) DO NOTHING;

-- name: DeleteBookmark :execrows
DELETE FROM bookmarks WHERE account_id = $1 AND status_id = $2;

-- name: GetBookmark :one
SELECT account_id, status_id, created_at, id FROM bookmarks WHERE account_id = $1 AND status_id = $2;

-- name: ListBookmarks :many
SELECT account_id, status_id, created_at, id FROM bookmarks
WHERE account_id = sqlc.arg(account_id) AND (sqlc.narg(before)::uuid IS NULL OR id < sqlc.narg(before)::uuid)
ORDER BY id DESC LIMIT sqlc.arg(page_limit);

-- name: InsertStatusPin :execrows
-- InsertStatusPin stores a pin in the lowest slot up to max_slot that the account leaves free.
-- It inserts nothing when the account already pins the status, when no slot is free, or when a
-- concurrent transaction takes the slot first.
INSERT INTO status_pins (account_id, status_id, slot, created_at, id)
SELECT sqlc.arg(account_id)::uuid, sqlc.arg(status_id)::uuid, c.slot, sqlc.arg(created_at)::timestamptz, sqlc.arg(id)::uuid
FROM (SELECT 1 AS slot UNION ALL SELECT p.slot + 1 FROM status_pins p WHERE p.account_id = sqlc.arg(account_id)::uuid) c
WHERE c.slot <= sqlc.arg(max_slot)::integer
  AND NOT EXISTS (SELECT 1 FROM status_pins u WHERE u.account_id = sqlc.arg(account_id)::uuid AND u.slot = c.slot)
ORDER BY c.slot LIMIT 1
ON CONFLICT DO NOTHING;

-- name: StatusPinExists :one
SELECT EXISTS (SELECT 1 FROM status_pins WHERE account_id = $1 AND status_id = $2);

-- name: DeleteStatusPin :execrows
DELETE FROM status_pins WHERE account_id = $1 AND status_id = $2;

-- name: DeleteStatusPinsOf :execrows
-- DeleteStatusPinsOf removes every pin of an account: the mirror of a remote account's featured
-- collection is replaced as a whole.
DELETE FROM status_pins WHERE account_id = $1;

-- name: CountStatusPins :one
SELECT COUNT(*) FROM status_pins WHERE account_id = $1;

-- name: GetStatusPin :one
SELECT account_id, status_id, slot, created_at, id FROM status_pins WHERE account_id = $1 AND status_id = $2;

-- name: ListStatusPins :many
SELECT account_id, status_id, slot, created_at, id FROM status_pins WHERE account_id = $1 ORDER BY id DESC;

-- Sources of other modules' projections.

-- ListLiveBoosts returns the boosts that boost_edges holds (not retracted, of a status that is
-- not deleted), ordered by ID.
-- name: ListLiveBoosts :many
SELECT b.id, b.uri, b.booster_account_id, b.status_id, b.visibility, b.is_local, b.boosted_at
FROM boosts b
JOIN boost_edges e ON e.boost_id = b.id
ORDER BY b.id;
