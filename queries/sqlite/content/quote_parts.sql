-- SPDX-License-Identifier: AGPL-3.0-only

-- The quotes of many statuses, and statuses by many URIs, one query each, for the pages that
-- present many statuses. Each reads what the query of one status of quote.sql and content.sql
-- reads. The lists are JSON arrays.

-- name: ListQuotesOfStatuses :many
SELECT status_id, quoted_uri, form FROM status_quotes WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT))) ORDER BY status_id;

-- name: ListQuoteStatesOfStatuses :many
SELECT q.status_id, q.form, CAST(d.status_id IS NOT NULL AS INTEGER) AS detached, CAST(COALESCE(rs.state, '') AS TEXT) AS state, rs.authorization_uri
FROM status_quotes q LEFT JOIN statuses s ON s.id = q.status_id
LEFT JOIN status_quote_detachments d ON d.status_id = q.status_id
LEFT JOIN interaction_requests r ON r.kind = 'quote' AND r.interacting_uri = s.uri AND r.target_uri = q.quoted_uri
LEFT JOIN interaction_request_states rs ON rs.request_id = r.id
WHERE q.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT))) ORDER BY q.status_id;

-- name: ListStatusesByURIs :many
SELECT id, uri, url, author_account_id, is_local, visibility, in_reply_to_uri, published_at
FROM statuses WHERE uri IN (SELECT value FROM json_each(CAST(sqlc.arg(uris) AS TEXT))) ORDER BY id;
