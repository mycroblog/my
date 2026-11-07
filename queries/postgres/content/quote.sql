-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the quotes of statuses (FR-114, ADR-0069, data-model section 4.4), owned by
-- internal/app/content.

-- InsertStatusQuote records the status that a status quotes; a status that has one keeps it.
-- name: InsertStatusQuote :execrows
INSERT INTO status_quotes (status_id, quoted_uri, form) VALUES (@status_id, @quoted_uri, @form)
ON CONFLICT (status_id) DO NOTHING;

-- name: GetStatusQuote :one
SELECT status_id, quoted_uri, form FROM status_quotes WHERE status_id = sqlc.arg(status_id)::uuid;

-- name: ListStatusQuotesAfter :many
SELECT status_id, quoted_uri, form FROM status_quotes
WHERE status_id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY status_id LIMIT @page_limit;

-- name: InsertStatusQuoteEvent :exec
INSERT INTO status_quote_events (id, status_id, kind, authorization_uri, occurred_at)
VALUES (@id, @status_id, @kind, @authorization_uri, @occurred_at);

-- name: ListStatusQuoteEvents :many
SELECT id, status_id, kind, authorization_uri, occurred_at FROM status_quote_events
WHERE status_id = sqlc.arg(status_id)::uuid ORDER BY id;

-- name: UpsertStatusQuoteState :exec
INSERT INTO status_quote_states (status_id, state, authorization_uri) VALUES (@status_id, @state, @authorization_uri)
ON CONFLICT (status_id) DO UPDATE SET state = excluded.state, authorization_uri = excluded.authorization_uri;

-- name: GetStatusQuoteState :one
SELECT status_id, state, authorization_uri FROM status_quote_states WHERE status_id = sqlc.arg(status_id)::uuid;

-- name: ListStatusQuoteStatesByAuthorization :many
SELECT status_id, state, authorization_uri FROM status_quote_states
WHERE authorization_uri = @authorization_uri ORDER BY status_id;

-- name: ListAllStatusQuoteStates :many
SELECT status_id, state, authorization_uri FROM status_quote_states ORDER BY status_id;

-- name: DeleteAllStatusQuoteStates :exec
DELETE FROM status_quote_states;

-- name: PurgeStatusQuoteStates :exec
DELETE FROM status_quote_states WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeStatusQuoteEvents :exec
DELETE FROM status_quote_events WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeStatusQuotes :exec
DELETE FROM status_quotes WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: UpsertStatusQuotePolicy :exec
INSERT INTO status_quote_policies (status_id, policy, updated_at) VALUES (@status_id, @policy, @updated_at)
ON CONFLICT (status_id) DO UPDATE SET policy = excluded.policy, updated_at = excluded.updated_at;

-- name: DeleteStatusQuotePolicy :exec
DELETE FROM status_quote_policies WHERE status_id = sqlc.arg(status_id)::uuid;

-- name: GetStatusQuotePolicy :one
SELECT status_id, policy, updated_at FROM status_quote_policies WHERE status_id = sqlc.arg(status_id)::uuid;

-- InsertQuoteAuthorization records an approval; a quoting status approved already keeps its own.
-- name: InsertQuoteAuthorization :execrows
INSERT INTO quote_authorizations (id, quoting_uri, quoting_account_id, quoted_status_id, created_at)
VALUES (@id, @quoting_uri, @quoting_account_id, @quoted_status_id, @created_at)
ON CONFLICT (quoting_uri, quoted_status_id) DO NOTHING;

-- name: GetQuoteAuthorization :one
SELECT a.id, a.quoting_uri, a.quoting_account_id, a.quoted_status_id, a.created_at, r.revoked_at
FROM quote_authorizations a LEFT JOIN quote_authorization_revocations r ON r.authorization_id = a.id
WHERE a.id = sqlc.arg(id)::uuid;

-- name: GetQuoteAuthorizationFor :one
SELECT a.id, a.quoting_uri, a.quoting_account_id, a.quoted_status_id, a.created_at, r.revoked_at
FROM quote_authorizations a LEFT JOIN quote_authorization_revocations r ON r.authorization_id = a.id
WHERE a.quoting_uri = @quoting_uri AND a.quoted_status_id = sqlc.arg(quoted_status_id)::uuid;

-- name: InsertQuoteAuthorizationRevocation :execrows
INSERT INTO quote_authorization_revocations (authorization_id, revoked_at) VALUES (@authorization_id, @revoked_at)
ON CONFLICT (authorization_id) DO NOTHING;

-- ListStatusQuotesOf pages the statuses that quote a URI, newest first.
-- name: ListStatusQuotesOf :many
SELECT status_id, quoted_uri, form FROM status_quotes
WHERE quoted_uri = @quoted_uri AND (sqlc.narg(before)::uuid IS NULL OR status_id < sqlc.narg(before)::uuid)
ORDER BY status_id DESC LIMIT @page_limit;

-- name: PurgeStatusQuotePolicies :exec
DELETE FROM status_quote_policies WHERE status_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeQuoteAuthorizationRevocations :exec
DELETE FROM quote_authorization_revocations WHERE authorization_id IN (
  SELECT a.id FROM quote_authorizations a WHERE a.quoted_status_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeQuoteAuthorizations :exec
DELETE FROM quote_authorizations WHERE quoted_status_id = ANY(sqlc.arg(ids)::uuid[]);
