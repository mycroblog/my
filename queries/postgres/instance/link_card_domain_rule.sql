-- SPDX-License-Identifier: AGPL-3.0-only

-- name: UpsertLinkCardDomainRule :exec
INSERT INTO link_card_domain_rules (domain, mode, note, updated_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (domain) DO UPDATE SET mode = excluded.mode, note = excluded.note, updated_at = excluded.updated_at;

-- name: DeleteLinkCardDomainRule :execrows
DELETE FROM link_card_domain_rules WHERE domain = $1;

-- name: CountLinkCardDomainRules :one
SELECT COUNT(*) FROM link_card_domain_rules;

-- name: HasLinkCardDomainRule :one
SELECT EXISTS (SELECT 1 FROM link_card_domain_rules WHERE domain = $1) AS found;

-- name: ListLinkCardDomainRules :many
SELECT domain, mode, note, updated_at FROM link_card_domain_rules ORDER BY domain COLLATE "C";

-- ListLinkCardDomainModes returns the modes of the rules of any of the domains.
-- name: ListLinkCardDomainModes :many
SELECT DISTINCT mode FROM link_card_domain_rules
WHERE domain = ANY(sqlc.arg(domains)::text[])
ORDER BY mode;

-- The modes of the rules of each of the domains, as ListLinkCardDomainModes reads those of all.
-- name: ListLinkCardDomainModesOf :many
SELECT DISTINCT domain, mode FROM link_card_domain_rules
WHERE domain = ANY(sqlc.arg(domains)::text[])
ORDER BY domain, mode;
