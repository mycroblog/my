-- SPDX-License-Identifier: AGPL-3.0-only

-- The quotes of many statuses, and statuses by many URIs, one query each, for the pages that
-- present many statuses. Each reads what the query of one status of quote.sql and content.sql
-- reads.

-- name: ListQuotesOfStatuses :many
SELECT status_id, quoted_uri, form FROM status_quotes WHERE status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: ListQuoteStatesOfStatuses :many
SELECT q.status_id, q.form, (d.status_id IS NOT NULL)::bool AS detached, COALESCE(rs.state, '')::text AS state, rs.authorization_uri
FROM status_quotes q LEFT JOIN statuses s ON s.id = q.status_id
LEFT JOIN status_quote_detachments d ON d.status_id = q.status_id
LEFT JOIN interaction_requests r ON r.kind = 'quote' AND r.interacting_uri = s.uri AND r.target_uri = q.quoted_uri
LEFT JOIN interaction_request_states rs ON rs.request_id = r.id
WHERE q.status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY q.status_id;

-- name: ListStatusesByURIs :many
SELECT id, uri, url, author_account_id, is_local, visibility, in_reply_to_uri, published_at
FROM statuses WHERE uri = ANY(sqlc.arg(uris)::text[]) ORDER BY id;
