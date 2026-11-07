-- SPDX-License-Identifier: AGPL-3.0-only

-- The instance's webhooks (data-model 4.11, ADR-0060). Endpoints, revisions, secrets, state
-- changes, messages, deliveries, and attempts are written once; webhook_endpoint_heads and
-- webhook_endpoint_failures are the projections.

-- name: InsertWebhookEndpoint :exec
INSERT INTO webhook_endpoints (id, created_by_user_id, created_at) VALUES ($1, $2, $3);

-- name: InsertWebhookEndpointRevision :exec
INSERT INTO webhook_endpoint_revisions (id, endpoint_id, url, description, revised_by_user_id, revised_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertWebhookEndpointRevisionEvent :exec
INSERT INTO webhook_endpoint_revision_events (revision_id, event_type) VALUES ($1, $2);

-- name: InsertWebhookSecret :exec
INSERT INTO webhook_secrets (id, endpoint_id, sealed_secret, created_by_user_id, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertWebhookEndpointStateChange :exec
INSERT INTO webhook_endpoint_state_changes (id, endpoint_id, state, reason, changed_by_user_id, changed_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpsertWebhookEndpointHead :exec
INSERT INTO webhook_endpoint_heads (endpoint_id, revision_id, state, secret_id) VALUES ($1, $2, $3, $4)
ON CONFLICT (endpoint_id) DO UPDATE SET
  revision_id = excluded.revision_id, state = excluded.state, secret_id = excluded.secret_id;

-- name: GetWebhookEndpointHeadForUpdate :one
SELECT endpoint_id, revision_id, state, secret_id FROM webhook_endpoint_heads WHERE endpoint_id = $1
FOR UPDATE;

-- ListWebhookEndpoints returns the endpoints that are not deleted, newest first, with their
-- current revision, last state change, and run of failures; only the endpoint id unless it is
-- empty.
-- name: ListWebhookEndpoints :many
SELECT e.id, e.created_by_user_id, e.created_at, h.revision_id, h.state, h.secret_id,
  r.url, r.description, r.revised_at, c.reason AS state_reason, c.changed_at AS state_changed_at,
  f.first_failure_attempt_id, f.first_failed_at, f.consecutive_failures
FROM webhook_endpoint_heads h
JOIN webhook_endpoints e ON e.id = h.endpoint_id
JOIN webhook_endpoint_revisions r ON r.id = h.revision_id
JOIN webhook_endpoint_state_changes c ON c.id = (
  SELECT c2.id FROM webhook_endpoint_state_changes c2 WHERE c2.endpoint_id = h.endpoint_id
  ORDER BY c2.id DESC LIMIT 1
)
LEFT JOIN webhook_endpoint_failures f ON f.endpoint_id = h.endpoint_id
WHERE h.state <> 'deleted' AND (sqlc.narg(id)::uuid IS NULL OR h.endpoint_id = sqlc.narg(id)::uuid)
ORDER BY e.id DESC;

-- name: ListWebhookRevisionEvents :many
SELECT revision_id, event_type FROM webhook_endpoint_revision_events
WHERE revision_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY revision_id, event_type;

-- name: ListSubscribedWebhookEndpoints :many
SELECT h.endpoint_id FROM webhook_endpoint_heads h
JOIN webhook_endpoint_revision_events v ON v.revision_id = h.revision_id
WHERE h.state = 'enabled' AND v.event_type = $1
ORDER BY h.endpoint_id;

-- name: ListWebhookSecrets :many
SELECT id, endpoint_id, sealed_secret, created_by_user_id, created_at FROM webhook_secrets
WHERE endpoint_id = sqlc.arg(endpoint_id) ORDER BY id DESC LIMIT sqlc.arg(lim);

-- name: InsertWebhookMessage :exec
INSERT INTO webhook_messages (id, event_type, body, created_at) VALUES ($1, $2, $3, $4);

-- name: InsertWebhookDelivery :exec
INSERT INTO webhook_deliveries (id, message_id, endpoint_id, created_at) VALUES ($1, $2, $3, $4);

-- name: GetWebhookSend :one
SELECT d.id, d.message_id, d.endpoint_id, d.created_at, m.event_type, m.body,
  m.created_at AS message_created_at, r.url, r.description, h.state,
  EXISTS (
    SELECT 1 FROM webhook_delivery_attempts a WHERE a.delivery_id = d.id AND a.outcome = 'succeeded'
  )::boolean AS succeeded
FROM webhook_deliveries d
JOIN webhook_messages m ON m.id = d.message_id
JOIN webhook_endpoint_heads h ON h.endpoint_id = d.endpoint_id
JOIN webhook_endpoint_revisions r ON r.id = h.revision_id
WHERE d.id = $1;

-- name: InsertWebhookDeliveryAttempt :exec
INSERT INTO webhook_delivery_attempts (id, delivery_id, outcome, status_code, duration_ms, attempted_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetWebhookEndpointFailures :one
SELECT endpoint_id, first_failure_attempt_id, first_failed_at, consecutive_failures
FROM webhook_endpoint_failures WHERE endpoint_id = $1;

-- AddWebhookEndpointFailure records a failed attempt in the run of failures of an endpoint in
-- one statement: it starts the run with the attempt, or adds one to the run that goes on, so
-- that concurrent failures each count.
-- name: AddWebhookEndpointFailure :exec
INSERT INTO webhook_endpoint_failures (endpoint_id, first_failure_attempt_id, first_failed_at, consecutive_failures)
VALUES ($1, $2, $3, 1)
ON CONFLICT (endpoint_id) DO UPDATE SET
  consecutive_failures = webhook_endpoint_failures.consecutive_failures + 1;

-- name: DeleteWebhookEndpointFailures :exec
DELETE FROM webhook_endpoint_failures WHERE endpoint_id = $1;

-- ListWebhookDeliveries returns the deliveries of an endpoint, newest first, before the delivery
-- before (empty for the first page), only the delivery id unless it is empty, with their event,
-- number of attempts, whether one succeeded, and their last attempt.
-- name: ListWebhookDeliveries :many
SELECT d.id, d.message_id, d.endpoint_id, d.created_at, m.event_type,
  (SELECT count(*) FROM webhook_delivery_attempts a WHERE a.delivery_id = d.id)::bigint AS attempts,
  EXISTS (
    SELECT 1 FROM webhook_delivery_attempts a WHERE a.delivery_id = d.id AND a.outcome = 'succeeded'
  )::boolean AS succeeded,
  la.id AS last_id, la.outcome AS last_outcome, la.status_code AS last_status_code,
  la.duration_ms AS last_duration_ms, la.attempted_at AS last_attempted_at
FROM webhook_deliveries d
JOIN webhook_messages m ON m.id = d.message_id
LEFT JOIN webhook_delivery_attempts la ON la.id = (
  SELECT a.id FROM webhook_delivery_attempts a WHERE a.delivery_id = d.id
  ORDER BY a.id DESC LIMIT 1
)
WHERE d.endpoint_id = sqlc.arg(endpoint_id)::uuid
  AND (sqlc.narg(before)::uuid IS NULL OR d.id < sqlc.narg(before)::uuid)
  AND (sqlc.narg(id)::uuid IS NULL OR d.id = sqlc.narg(id)::uuid)
ORDER BY d.id DESC
LIMIT sqlc.arg(lim);

-- name: ListWebhookMessagesBefore :many
SELECT id FROM webhook_messages WHERE created_at < sqlc.arg(before) ORDER BY created_at, id LIMIT sqlc.arg(lim);

-- name: DeleteWebhookAttemptsOfMessages :exec
DELETE FROM webhook_delivery_attempts WHERE delivery_id IN (
  SELECT d.id FROM webhook_deliveries d WHERE d.message_id = ANY(sqlc.arg(ids)::uuid[])
);

-- name: DeleteWebhookDeliveriesOfMessages :exec
DELETE FROM webhook_deliveries WHERE message_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteWebhookMessages :execrows
DELETE FROM webhook_messages WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- ListExistingWebhookEndpointIDs returns, in ID order, the endpoints among ids that are not
-- deleted (the audit log).
-- name: ListExistingWebhookEndpointIDs :many
SELECT endpoint_id FROM webhook_endpoint_heads WHERE endpoint_id = ANY(sqlc.arg(ids)::uuid[]) AND state <> 'deleted'
ORDER BY endpoint_id;
