-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the relays (ADR-0006, data-model section 4.8), owned by internal/app/federation.

-- name: InsertRelay :exec
INSERT INTO relays (id, protocol, url, inbox_url, receive, created_by_user_id, created_at)
VALUES (@id, @protocol, @url, @inbox_url, @receive, @created_by_user_id, @created_at);

-- LockRelay reads a relay with its state in the transaction that changes it. SQLite serializes
-- writers, so it needs no row lock.
-- name: LockRelay :one
SELECT r.id, r.protocol, r.url, r.inbox_url, r.receive, r.created_by_user_id, r.created_at,
  s.state, s.follow_activity_uri, s.relay_actor_uri, s.since
FROM relays r LEFT JOIN relay_states s ON s.relay_id = r.id
WHERE r.id = @id;

-- name: GetRelay :one
SELECT r.id, r.protocol, r.url, r.inbox_url, r.receive, r.created_by_user_id, r.created_at,
  s.state, s.follow_activity_uri, s.relay_actor_uri, s.since
FROM relays r LEFT JOIN relay_states s ON s.relay_id = r.id
WHERE r.id = @id;

-- ListRelays returns every relay in registration order (the IDs are UUIDv7).
-- name: ListRelays :many
SELECT r.id, r.protocol, r.url, r.inbox_url, r.receive, r.created_by_user_id, r.created_at,
  s.state, s.follow_activity_uri, s.relay_actor_uri, s.since
FROM relays r LEFT JOIN relay_states s ON s.relay_id = r.id
ORDER BY r.id;

-- GetRelayByFollow returns the relay whose current Follow is @follow_activity_uri.
-- name: GetRelayByFollow :one
SELECT r.id, r.protocol, r.url, r.inbox_url, r.receive, r.created_by_user_id, r.created_at,
  s.state, s.follow_activity_uri, s.relay_actor_uri, s.since
FROM relays r LEFT JOIN relay_states s ON s.relay_id = r.id
WHERE s.follow_activity_uri = @follow_activity_uri
ORDER BY r.id
LIMIT 1;

-- GetAcceptedRelayByActor returns the first accepted relay whose actor is @relay_actor_uri.
-- name: GetAcceptedRelayByActor :one
SELECT r.id, r.protocol, r.url, r.inbox_url, r.receive, r.created_by_user_id, r.created_at,
  s.state, s.follow_activity_uri, s.relay_actor_uri, s.since
FROM relays r LEFT JOIN relay_states s ON s.relay_id = r.id
WHERE s.relay_actor_uri = @relay_actor_uri AND s.state = 'accepted'
ORDER BY r.id
LIMIT 1;

-- name: ListAcceptedRelayInboxes :many
SELECT DISTINCT r.inbox_url FROM relays r JOIN relay_states s ON s.relay_id = r.id
WHERE s.state = 'accepted'
ORDER BY r.inbox_url;

-- InsertRelayEvent inserts nothing when the relay has an event of the kind about the Follow.
-- name: InsertRelayEvent :execrows
INSERT INTO relay_events (id, relay_id, kind, follow_activity_uri, relay_actor_uri, actor_user_id, occurred_at)
VALUES (@id, @relay_id, @kind, @follow_activity_uri, @relay_actor_uri, @actor_user_id, @occurred_at)
ON CONFLICT (relay_id, follow_activity_uri, kind) DO NOTHING;

-- name: UpsertRelayState :exec
INSERT INTO relay_states (relay_id, state, follow_activity_uri, relay_actor_uri, since)
VALUES (@relay_id, @state, @follow_activity_uri, @relay_actor_uri, @since)
ON CONFLICT (relay_id) DO UPDATE SET state = excluded.state, follow_activity_uri = excluded.follow_activity_uri,
  relay_actor_uri = excluded.relay_actor_uri, since = excluded.since;

-- name: DeleteRelayState :exec
DELETE FROM relay_states WHERE relay_id = @relay_id;

-- name: DeleteRelayEvents :exec
DELETE FROM relay_events WHERE relay_id = @relay_id;

-- name: DeleteRelay :exec
DELETE FROM relays WHERE id = @id;

-- name: DeleteAllRelayStates :exec
DELETE FROM relay_states;

-- RebuildRelayStates derives the state of each relay from its newest event (the IDs are UUIDv7).
-- name: RebuildRelayStates :exec
INSERT INTO relay_states (relay_id, state, follow_activity_uri, relay_actor_uri, since)
SELECT e.relay_id,
  CASE e.kind WHEN 'follow_sent' THEN 'pending' WHEN 'accepted' THEN 'accepted' WHEN 'rejected' THEN 'rejected' ELSE 'left' END,
  e.follow_activity_uri, e.relay_actor_uri, e.occurred_at
FROM relay_events e
WHERE e.id = (SELECT x.id FROM relay_events x WHERE x.relay_id = e.relay_id ORDER BY x.id DESC LIMIT 1);
