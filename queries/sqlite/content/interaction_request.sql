-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the approvals of interactions (ADR-0155 section 8, data-model section 4.4): the
-- requests, their answers and current states, this server's authorizations, and the quotes taken
-- away. Owned by internal/app/content.

-- InsertInteractionRequest records a request; an interaction that has one keeps it.
-- name: InsertInteractionRequest :execrows
INSERT INTO interaction_requests (id, kind, direction, target_uri, target_account_id, interacting_uri,
  interacting_account_id, activity_uri, visibility, created_at)
VALUES (@id, @kind, @direction, @target_uri, sqlc.narg(target_account_id), @interacting_uri,
  @interacting_account_id, @activity_uri, @visibility, @created_at)
ON CONFLICT (kind, interacting_uri, target_uri) DO NOTHING;

-- name: GetInteractionRequest :one
SELECT id, kind, direction, target_uri, target_account_id, interacting_uri, interacting_account_id,
  activity_uri, visibility, created_at
FROM interaction_requests WHERE id = sqlc.arg(id);

-- name: GetInteractionRequestFor :one
SELECT id, kind, direction, target_uri, target_account_id, interacting_uri, interacting_account_id,
  activity_uri, visibility, created_at
FROM interaction_requests WHERE kind = @kind AND interacting_uri = @interacting_uri AND target_uri = @target_uri;

-- name: ListInteractionRequestsAfter :many
SELECT id, kind, direction, target_uri, target_account_id, interacting_uri, interacting_account_id,
  activity_uri, visibility, created_at
FROM interaction_requests
WHERE id > @after ORDER BY id LIMIT @page_limit;

-- name: InsertInteractionRequestEvent :exec
INSERT INTO interaction_request_events (id, request_id, kind, authorization_uri, occurred_at)
VALUES (@id, @request_id, @kind, @authorization_uri, @occurred_at);

-- name: ListInteractionRequestEvents :many
SELECT id, request_id, kind, authorization_uri, occurred_at FROM interaction_request_events
WHERE request_id = sqlc.arg(request_id) ORDER BY id;

-- name: UpsertInteractionRequestState :exec
INSERT INTO interaction_request_states (request_id, kind, target_account_id, state, authorization_uri)
VALUES (@request_id, @kind, sqlc.narg(target_account_id), @state, @authorization_uri)
ON CONFLICT (request_id) DO UPDATE SET state = excluded.state, authorization_uri = excluded.authorization_uri,
  target_account_id = excluded.target_account_id;

-- name: GetInteractionRequestState :one
SELECT request_id, kind, target_account_id, state, authorization_uri FROM interaction_request_states
WHERE request_id = sqlc.arg(request_id);

-- name: ListInteractionRequestStatesByAuthorization :many
SELECT request_id, kind, target_account_id, state, authorization_uri FROM interaction_request_states
WHERE authorization_uri = @authorization_uri ORDER BY request_id;

-- name: ListAllInteractionRequestStates :many
SELECT request_id, kind, target_account_id, state, authorization_uri FROM interaction_request_states ORDER BY request_id;

-- name: DeleteAllInteractionRequestStates :exec
DELETE FROM interaction_request_states;

-- ListPendingInteractionRequests pages the requests that await the answer of an author, newest
-- first, of the kinds given (all of them for an empty list).
-- name: ListPendingInteractionRequests :many
SELECT r.id, r.kind, r.direction, r.target_uri, r.target_account_id, r.interacting_uri, r.interacting_account_id,
  r.activity_uri, r.visibility, r.created_at
FROM interaction_request_states s JOIN interaction_requests r ON r.id = s.request_id
WHERE s.target_account_id = sqlc.arg(account_id) AND s.state = 'pending'
  AND (json_array_length(CAST(sqlc.arg(kinds) AS TEXT)) = 0 OR s.kind IN (SELECT value FROM json_each(CAST(sqlc.arg(kinds) AS TEXT))))
  AND (CAST(@before AS TEXT) = '' OR s.request_id < CAST(@before AS TEXT))
ORDER BY s.request_id DESC LIMIT @page_limit;

-- ListPendingInteractionRequestsTo returns the pending requests of interactions with the status
-- target_uri, oldest first.
-- name: ListPendingInteractionRequestsTo :many
SELECT r.id, r.kind, r.direction, r.target_uri, r.target_account_id, r.interacting_uri, r.interacting_account_id,
  r.activity_uri, r.visibility, r.created_at
FROM interaction_requests r JOIN interaction_request_states s ON s.request_id = r.id
WHERE r.target_uri = @target_uri AND s.state = 'pending'
ORDER BY r.id;

-- ListOwnInteractionStates returns the states of the replies and boosts of an account among
-- uris: its replies whose URI is among them, and its boosts of the statuses among them.
-- name: ListOwnInteractionStates :many
SELECT r.id, r.kind, r.target_uri, r.interacting_uri, s.state
FROM interaction_requests r JOIN interaction_request_states s ON s.request_id = r.id
WHERE r.interacting_account_id = sqlc.arg(account_id)
  AND ((r.kind = 'reply' AND r.interacting_uri IN (SELECT value FROM json_each(CAST(sqlc.arg(uris) AS TEXT))))
    OR (r.kind = 'announce' AND r.target_uri IN (SELECT value FROM json_each(CAST(sqlc.arg(uris) AS TEXT)))))
ORDER BY r.id;

-- ListOwnInteractionStatesOfAccounts is ListOwnInteractionStates of each of many accounts.
-- name: ListOwnInteractionStatesOfAccounts :many
SELECT r.id, r.kind, r.target_uri, r.interacting_uri, r.interacting_account_id, s.state
FROM interaction_requests r JOIN interaction_request_states s ON s.request_id = r.id
WHERE r.interacting_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)))
  AND ((r.kind = 'reply' AND r.interacting_uri IN (SELECT value FROM json_each(CAST(sqlc.arg(uris) AS TEXT))))
    OR (r.kind = 'announce' AND r.target_uri IN (SELECT value FROM json_each(CAST(sqlc.arg(uris) AS TEXT)))))
ORDER BY r.id;

-- HeldRepliesAmong returns the statuses among ids that are replies held out of the thread of
-- their parent: a reply request of theirs is not accepted.
-- name: HeldRepliesAmong :many
SELECT s.id FROM statuses s
WHERE s.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) AND s.in_reply_to_uri IS NOT NULL
  AND EXISTS (SELECT 1 FROM interaction_requests r WHERE r.kind = 'reply' AND r.interacting_uri = s.uri
    AND r.target_uri = s.in_reply_to_uri
    AND NOT EXISTS (SELECT 1 FROM interaction_request_events e WHERE e.request_id = r.id AND e.kind = 'accepted'))
ORDER BY s.id;

-- name: ListHeldReplyIDs :many
SELECT s.id FROM statuses s JOIN interaction_requests r
  ON r.kind = 'reply' AND r.interacting_uri = s.uri AND r.target_uri = s.in_reply_to_uri
WHERE NOT EXISTS (SELECT 1 FROM interaction_request_events e WHERE e.request_id = r.id AND e.kind = 'accepted')
ORDER BY s.id;

-- InsertInteractionAuthorization records an approval; an interaction approved already keeps its own.
-- name: InsertInteractionAuthorization :execrows
INSERT INTO interaction_authorizations (id, kind, interacting_uri, interacting_account_id, target_status_id, created_at)
VALUES (@id, @kind, @interacting_uri, @interacting_account_id, @target_status_id, @created_at)
ON CONFLICT (kind, interacting_uri, target_status_id) DO NOTHING;

-- name: GetInteractionAuthorization :one
SELECT a.id, a.kind, a.interacting_uri, a.interacting_account_id, a.target_status_id, a.created_at, r.revoked_at
FROM interaction_authorizations a LEFT JOIN interaction_authorization_revocations r ON r.authorization_id = a.id
WHERE a.id = sqlc.arg(id);

-- name: GetInteractionAuthorizationFor :one
SELECT a.id, a.kind, a.interacting_uri, a.interacting_account_id, a.target_status_id, a.created_at, r.revoked_at
FROM interaction_authorizations a LEFT JOIN interaction_authorization_revocations r ON r.authorization_id = a.id
WHERE a.kind = @kind AND a.interacting_uri = @interacting_uri AND a.target_status_id = sqlc.arg(target_status_id);

-- name: InsertInteractionAuthorizationRevocation :execrows
INSERT INTO interaction_authorization_revocations (authorization_id, revoked_at) VALUES (@authorization_id, @revoked_at)
ON CONFLICT (authorization_id) DO NOTHING;

-- name: InsertStatusQuoteDetachment :exec
INSERT INTO status_quote_detachments (status_id, detached_at) VALUES (@status_id, @detached_at)
ON CONFLICT (status_id) DO NOTHING;

-- The approvals of the statuses being purged: the requests whose target or interaction is one of
-- them, with their answers and states, and the approvals given to interactions with them.

-- name: PurgeInteractionRequestEvents :exec
DELETE FROM interaction_request_events WHERE request_id IN (
  SELECT r.id FROM interaction_requests r JOIN statuses s ON s.uri IN (r.target_uri, r.interacting_uri)
  WHERE s.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: PurgeInteractionRequestStates :exec
DELETE FROM interaction_request_states WHERE request_id IN (
  SELECT r.id FROM interaction_requests r JOIN statuses s ON s.uri IN (r.target_uri, r.interacting_uri)
  WHERE s.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: PurgeInteractionRequests :exec
DELETE FROM interaction_requests WHERE id IN (
  SELECT r.id FROM interaction_requests r JOIN statuses s ON s.uri IN (r.target_uri, r.interacting_uri)
  WHERE s.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: PurgeInteractionAuthorizationRevocations :exec
DELETE FROM interaction_authorization_revocations WHERE authorization_id IN (
  SELECT a.id FROM interaction_authorizations a WHERE a.target_status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: PurgeInteractionAuthorizations :exec
DELETE FROM interaction_authorizations WHERE target_status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: PurgeStatusQuoteDetachments :exec
DELETE FROM status_quote_detachments WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- ListReplyAuthorizationsOf returns the authorizations of the approved reply requests of the
-- statuses among ids, which their Notes name (replyAuthorization).
-- name: ListReplyAuthorizationsOf :many
SELECT s.id, CAST(rs.authorization_uri AS TEXT) AS authorization_uri
FROM statuses s
JOIN interaction_requests r ON r.kind = 'reply' AND r.interacting_uri = s.uri AND r.target_uri = s.in_reply_to_uri
JOIN interaction_request_states rs ON rs.request_id = r.id
WHERE s.id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) AND rs.state = 'accepted' AND rs.authorization_uri IS NOT NULL
ORDER BY s.id;
