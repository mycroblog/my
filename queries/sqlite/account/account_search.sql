-- SPDX-License-Identifier: AGPL-3.0-only

-- The prefix search of accounts (ADR-0092 section 1, FR-119, FR-306): the account_search_terms
-- projection and its reads.

-- name: DeleteAccountSearchTerms :exec
DELETE FROM account_search_terms WHERE account_id = ?;

-- name: InsertAccountSearchTerm :exec
INSERT INTO account_search_terms (account_id, kind, term, is_local) VALUES (?, ?, ?, ?)
ON CONFLICT (account_id, kind, term) DO NOTHING;

-- name: DeleteAllAccountSearchTerms :exec
DELETE FROM account_search_terms;

-- ListAccountIDsWithUsernameTerm returns the accounts that have a username term, in ID order.
-- name: ListAccountIDsWithUsernameTerm :many
SELECT account_id FROM account_search_terms WHERE kind = 'username' AND term = ? ORDER BY account_id;

-- ListHandleAccountIDs returns the accounts that have a handle, in ID order, for rebuilding the
-- search terms.
-- name: ListHandleAccountIDs :many
SELECT account_id FROM account_handles ORDER BY account_id;

-- ListAccountSearchTerms returns every search term, for verifying the projection.
-- name: ListAccountSearchTerms :many
SELECT account_id, kind, term, is_local FROM account_search_terms ORDER BY account_id, kind, term;

-- SearchAccountTerms returns up to lim search terms in [low, high) in term order, with the
-- domain of their accounts.
-- name: SearchAccountTerms :many
SELECT t.account_id, t.kind, t.term, t.is_local, a.domain
FROM account_search_terms t JOIN accounts a ON a.id = t.account_id
WHERE t.term >= CAST(sqlc.arg(low) AS TEXT) AND t.term < CAST(sqlc.arg(high) AS TEXT)
ORDER BY t.term, t.account_id
LIMIT sqlc.arg(lim);
