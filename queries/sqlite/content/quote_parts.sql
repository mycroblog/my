-- SPDX-License-Identifier: AGPL-3.0-only

-- The quotes of many statuses, and statuses by many URIs, one query each, for the pages that
-- present many statuses. Each reads what the query of one status of quote.sql and content.sql
-- reads. The lists are JSON arrays.

-- name: ListQuotesOfStatuses :many
SELECT status_id, quoted_uri, form FROM status_quotes WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT))) ORDER BY status_id;

-- name: ListQuoteStatesOfStatuses :many
SELECT status_id, state, authorization_uri FROM status_quote_states WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT))) ORDER BY status_id;

-- name: ListQuotePoliciesOfStatuses :many
SELECT status_id, policy, updated_at FROM status_quote_policies WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT))) ORDER BY status_id;

-- name: ListStatusesByURIs :many
SELECT id, uri, url, author_account_id, is_local, visibility, in_reply_to_uri, conversation_uri, published_at
FROM statuses WHERE uri IN (SELECT value FROM json_each(CAST(sqlc.arg(uris) AS TEXT))) ORDER BY id;
