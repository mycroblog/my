-- SPDX-License-Identifier: AGPL-3.0-only

-- The restrictions in effect of many accounts and domains at once, one query each, for the pages
-- that present many accounts' statuses. Each reads what ListAccountRestrictionsInEffect and
-- ListDomainRestrictionsInEffect read, by account and by domain.

-- name: ListAccountRestrictionsInEffectOf :many
SELECT DISTINCT account_id, restriction FROM account_restrictions
WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[])
  AND (effective_until IS NULL OR effective_until > sqlc.arg(now)::timestamptz)
ORDER BY account_id, restriction;

-- name: ListDomainRestrictionsInEffectOf :many
SELECT DISTINCT domain, restriction FROM domain_restrictions
WHERE (effective_until IS NULL OR effective_until > sqlc.arg(now)::timestamptz)
  AND domain = ANY(sqlc.arg(domains)::text[])
ORDER BY domain, restriction;
