-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertFollow :exec
INSERT INTO follows (id, uri, follower_account_id, followee_account_id, requested_at) VALUES ($1, $2, $3, $4, $5);

-- name: InsertFollowAcceptance :exec
INSERT INTO follow_acceptances (follow_id, accepted_by, activity_uri, accepted_at) VALUES ($1, $2, $3, $4);

-- name: InsertFollowRejection :exec
INSERT INTO follow_rejections (follow_id, rejected_by, activity_uri, rejected_at) VALUES ($1, $2, $3, $4);

-- name: InsertFollowTermination :exec
INSERT INTO follow_terminations (follow_id, reason, terminated_at) VALUES ($1, $2, $3);

-- name: UpsertFollowPreferences :exec
INSERT INTO follow_preferences (follow_id, show_boosts) VALUES ($1, $2)
ON CONFLICT (follow_id) DO UPDATE SET show_boosts = excluded.show_boosts;

-- name: GetFollow :one
SELECT id, uri, follower_account_id, followee_account_id, requested_at FROM follows WHERE id = $1;

-- name: GetFollowByURI :one
SELECT id, uri, follower_account_id, followee_account_id, requested_at FROM follows WHERE uri = $1;

-- name: GetFollowPreferences :one
SELECT follow_id, show_boosts FROM follow_preferences WHERE follow_id = $1;

-- name: GetFollowEdge :one
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges WHERE follower_account_id = $1 AND followee_account_id = $2;

-- InsertFollowEdge inserts nothing when the pair already has an edge.
-- name: InsertFollowEdge :execrows
INSERT INTO follow_edges (follower_account_id, followee_account_id, follow_id, state, since) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (follower_account_id, followee_account_id) DO NOTHING;

-- UpdateFollowEdge and DeleteFollowEdge change the edge only in the state that the command read,
-- so that a projector learns from the rows affected that a concurrent transaction changed it.
-- name: UpdateFollowEdge :execrows
UPDATE follow_edges SET state = $4, since = $5
WHERE follower_account_id = $1 AND followee_account_id = $2 AND follow_id = $3 AND state = sqlc.arg(from_state);

-- name: DeleteFollowEdge :execrows
DELETE FROM follow_edges
WHERE follower_account_id = $1 AND followee_account_id = $2 AND follow_id = $3 AND state = $4;

-- name: DeleteAllFollowEdges :exec
DELETE FROM follow_edges;

-- AddToFollowCounters adds to an existing row; a new row goes through AddFollowCounters, whose
-- values must satisfy the CHECK constraints before ON CONFLICT applies.
-- name: AddToFollowCounters :execrows
UPDATE follow_counters SET followers_count = followers_count + $2, following_count = following_count + $3
WHERE account_id = $1;

-- name: AddFollowCounters :exec
INSERT INTO follow_counters (account_id, followers_count, following_count) VALUES ($1, $2, $3)
ON CONFLICT (account_id) DO UPDATE SET
  followers_count = follow_counters.followers_count + excluded.followers_count,
  following_count = follow_counters.following_count + excluded.following_count;

-- name: DeleteZeroFollowCounters :exec
DELETE FROM follow_counters WHERE account_id = $1 AND followers_count = 0 AND following_count = 0;

-- name: UpsertFollowCounters :exec
INSERT INTO follow_counters (account_id, followers_count, following_count) VALUES ($1, $2, $3)
ON CONFLICT (account_id) DO UPDATE SET
  followers_count = excluded.followers_count,
  following_count = excluded.following_count;

-- name: DeleteAllFollowCounters :exec
DELETE FROM follow_counters;

-- name: ListFollowHistories :many
SELECT
  f.id, f.uri, f.follower_account_id, f.followee_account_id, f.requested_at,
  a.accepted_by, a.activity_uri AS acceptance_activity_uri, a.accepted_at,
  r.rejected_by, r.activity_uri AS rejection_activity_uri, r.rejected_at,
  t.reason AS termination_reason, t.terminated_at
FROM follows f
LEFT JOIN follow_acceptances a ON a.follow_id = f.id
LEFT JOIN follow_rejections r ON r.follow_id = f.id
LEFT JOIN follow_terminations t ON t.follow_id = f.id
ORDER BY f.id;

-- name: ListFollowEdges :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges ORDER BY follower_account_id, followee_account_id;

-- ListFollowEdgesOf returns the edges into and out of an account, pending and active, ordered
-- by follower and followee.
-- name: ListFollowEdgesOf :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges WHERE follower_account_id = sqlc.arg(account_id) OR followee_account_id = sqlc.arg(account_id)
ORDER BY follower_account_id, followee_account_id;

-- name: ListFollowCounters :many
SELECT account_id, followers_count, following_count FROM follow_counters ORDER BY account_id;

-- name: GetFollowCounters :one
SELECT account_id, followers_count, following_count FROM follow_counters WHERE account_id = $1;

-- CountFollowersInState counts the edges into an account in one state.
-- name: CountFollowersInState :one
SELECT count(*) FROM follow_edges
WHERE followee_account_id = sqlc.arg(account_id) AND state = sqlc.arg(state);

-- ListFollowersPage pages the edges into an account in one state, newest follow first; an empty
-- before starts at the newest.
-- name: ListFollowersPage :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges
WHERE followee_account_id = sqlc.arg(account_id) AND state = sqlc.arg(state)
  AND (sqlc.narg(before)::uuid IS NULL OR follow_id < sqlc.narg(before)::uuid)
ORDER BY follow_id DESC
LIMIT sqlc.arg(page_limit);

-- ListFollowingPage pages the edges out of an account in one state, newest follow first; an
-- empty before starts at the newest.
-- name: ListFollowingPage :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges
WHERE follower_account_id = sqlc.arg(account_id) AND state = sqlc.arg(state)
  AND (sqlc.narg(before)::uuid IS NULL OR follow_id < sqlc.narg(before)::uuid)
ORDER BY follow_id DESC
LIMIT sqlc.arg(page_limit);

-- InsertFollowResend inserts nothing when the URI is already recorded.
-- name: InsertFollowResend :execrows
INSERT INTO follow_resends (uri, follow_id, received_at) VALUES ($1, $2, $3)
ON CONFLICT (uri) DO NOTHING;

-- name: GetFollowByResendURI :one
SELECT f.id, f.uri, f.follower_account_id, f.followee_account_id, f.requested_at
FROM follow_resends r JOIN follows f ON f.id = r.follow_id WHERE r.uri = $1;
