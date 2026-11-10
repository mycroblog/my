-- SPDX-License-Identifier: AGPL-3.0-only

-- The restrictions in effect of many accounts and domains at once, one query each, for the pages
-- that present many accounts' statuses. Each reads what ListAccountRestrictionsInEffect and
-- ListDomainRestrictionsInEffect read, by account and by domain. The lists are JSON arrays.

-- name: ListAccountRestrictionsInEffectOf :many
SELECT DISTINCT account_id, restriction FROM account_restrictions
WHERE account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)))
  AND (effective_until IS NULL OR effective_until > CAST(sqlc.arg(now) AS INTEGER))
ORDER BY account_id, restriction;

-- name: ListDomainRestrictionsInEffectOf :many
SELECT DISTINCT domain, restriction FROM domain_restrictions
WHERE (effective_until IS NULL OR effective_until > CAST(sqlc.arg(now) AS INTEGER))
  AND domain IN (SELECT value FROM json_each(CAST(sqlc.arg(domains) AS TEXT)))
ORDER BY domain, restriction;
