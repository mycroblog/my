-- SPDX-License-Identifier: AGPL-3.0-only

-- InsertInstanceDomains records the domains of the instance unless they are recorded.
-- name: InsertInstanceDomains :execrows
INSERT INTO instance_domains (id, host, account_domain, recorded_at) VALUES (1, $1, $2, $3)
ON CONFLICT (id) DO NOTHING;

-- GetInstanceDomains returns the recorded domains of the instance.
-- name: GetInstanceDomains :one
SELECT host, account_domain, recorded_at FROM instance_domains WHERE id = 1;
