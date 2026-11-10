-- SPDX-License-Identifier: AGPL-3.0-only

-- The allowlist of federation (resource) and its cutoffs (event, ADR-0159). The lists are
-- JSON arrays.

-- InsertDomainAllow inserts nothing when the domain is already allowed.
-- name: InsertDomainAllow :execrows
INSERT INTO domain_allows (domain, comment, created_by_user_id, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (domain) DO NOTHING;

-- name: DeleteDomainAllow :one
DELETE FROM domain_allows WHERE domain = ?
RETURNING domain, comment, created_by_user_id, created_at;

-- name: GetDomainAllow :one
SELECT domain, comment, created_by_user_id, created_at FROM domain_allows WHERE domain = ?;

-- name: ListDomainAllows :many
SELECT domain, comment, created_by_user_id, created_at FROM domain_allows ORDER BY domain;

-- name: CountDomainAllows :one
SELECT count(*) FROM domain_allows;

-- name: ListAllowedDomains :many
SELECT domain FROM domain_allows WHERE domain IN (SELECT value FROM json_each(CAST(sqlc.arg(domains) AS TEXT))) ORDER BY domain;

-- name: ListExistingDomainAllows :many
SELECT domain FROM domain_allows WHERE domain IN (SELECT value FROM json_each(CAST(sqlc.arg(domains) AS TEXT))) ORDER BY domain;

-- name: InsertAllowlistCutoff :exec
INSERT INTO allowlist_cutoffs (id, cause, domain, issued_by_user_id, occurred_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetAllowlistCutoff :one
SELECT id, cause, domain, issued_by_user_id, occurred_at FROM allowlist_cutoffs WHERE id = ?;
