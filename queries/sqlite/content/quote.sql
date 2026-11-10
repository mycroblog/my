-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Queries of the quotes of statuses (FR-114, ADR-0069, data-model section 4.4), owned by
-- internal/app/content.

-- InsertStatusQuote records the status that a status quotes; a status that has one keeps it.
-- name: InsertStatusQuote :execrows
INSERT INTO status_quotes (status_id, quoted_uri, form) VALUES (@status_id, @quoted_uri, @form)
ON CONFLICT (status_id) DO NOTHING;

-- name: GetStatusQuote :one
SELECT status_id, quoted_uri, form FROM status_quotes WHERE status_id = @status_id;

-- name: ListStatusQuotesAfter :many
SELECT status_id, quoted_uri, form FROM status_quotes
WHERE status_id > @after ORDER BY status_id LIMIT @page_limit;

-- The state of a quote is derived from its form, its detachment, and the state of its request
-- (content.QuoteStateFrom): state is empty while the quote has no request.

-- name: GetStatusQuoteState :one
SELECT q.status_id, q.form, CAST(d.status_id IS NOT NULL AS INTEGER) AS detached, CAST(COALESCE(rs.state, '') AS TEXT) AS state, rs.authorization_uri
FROM status_quotes q LEFT JOIN statuses s ON s.id = q.status_id
LEFT JOIN status_quote_detachments d ON d.status_id = q.status_id
LEFT JOIN interaction_requests r ON r.kind = 'quote' AND r.interacting_uri = s.uri AND r.target_uri = q.quoted_uri
LEFT JOIN interaction_request_states rs ON rs.request_id = r.id
WHERE q.status_id = sqlc.arg(status_id);

-- ListQuoteStatesByAuthorization returns the states of the quotes whose request the
-- QuoteAuthorization authorization_uri accepted.
-- name: ListQuoteStatesByAuthorization :many
SELECT q.status_id, q.form, CAST(d.status_id IS NOT NULL AS INTEGER) AS detached, rs.state, rs.authorization_uri
FROM interaction_request_states rs JOIN interaction_requests r ON r.id = rs.request_id AND r.kind = 'quote'
JOIN statuses s ON s.uri = r.interacting_uri
JOIN status_quotes q ON q.status_id = s.id AND q.quoted_uri = r.target_uri
LEFT JOIN status_quote_detachments d ON d.status_id = q.status_id
WHERE rs.authorization_uri = @authorization_uri ORDER BY q.status_id;

-- name: PurgeStatusQuotes :exec
DELETE FROM status_quotes WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- ListStatusQuotesOf pages the statuses that quote a URI, newest first.
-- name: ListStatusQuotesOf :many
SELECT status_id, quoted_uri, form FROM status_quotes
WHERE quoted_uri = @quoted_uri AND (CAST(@before AS TEXT) = '' OR status_id < CAST(@before AS TEXT))
ORDER BY status_id DESC LIMIT @page_limit;
