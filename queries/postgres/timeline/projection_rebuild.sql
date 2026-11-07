-- SPDX-License-Identifier: AGPL-3.0-only

-- The sources and the emptying of the projections of moderation, identity, content (the
-- sensitivity overrides and the poll tallies), and timeline (the restrictions) that
-- `my projection rebuild` derives again (data-model section 9). The Rebuild queries state the
-- rules of the commands and projectors in SQL.

-- name: ListAllFeedAuthorRestrictions :many
SELECT account_id, restriction, effective_until FROM feed_author_restrictions ORDER BY account_id, restriction;

-- name: ListAllFeedDomainRestrictions :many
SELECT domain, restriction, effective_until FROM feed_domain_restrictions ORDER BY domain, restriction;

-- name: DeleteAllFeedAuthorRestrictions :exec
DELETE FROM feed_author_restrictions;

-- name: DeleteAllFeedDomainRestrictions :exec
DELETE FROM feed_domain_restrictions;
