-- SPDX-License-Identifier: AGPL-3.0-only

-- ListSealedInstanceSecrets returns a page of instance_secrets.sealed_secret ordered by id.
-- name: ListSealedInstanceSecrets :many
SELECT id AS row_id, sealed_secret AS sealed FROM instance_secrets WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedInstanceSecrets rewrites instance_secrets.sealed_secret of a row that still holds old_sealed.
-- name: ReplaceSealedInstanceSecrets :execrows
UPDATE instance_secrets SET sealed_secret = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_secret = sqlc.arg(old_sealed);

-- ListSealedTLSAccounts returns a page of tls_acme_accounts.sealed_private_key ordered by id.
-- name: ListSealedTLSAccounts :many
SELECT id AS row_id, sealed_private_key AS sealed FROM tls_acme_accounts WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedTLSAccounts rewrites tls_acme_accounts.sealed_private_key of a row that still holds old_sealed.
-- name: ReplaceSealedTLSAccounts :execrows
UPDATE tls_acme_accounts SET sealed_private_key = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_private_key = sqlc.arg(old_sealed);

-- ListSealedTLSCertificates returns a page of tls_certificates.sealed_private_key ordered by id.
-- name: ListSealedTLSCertificates :many
SELECT id AS row_id, sealed_private_key AS sealed FROM tls_certificates WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedTLSCertificates rewrites tls_certificates.sealed_private_key of a row that still holds old_sealed.
-- name: ReplaceSealedTLSCertificates :execrows
UPDATE tls_certificates SET sealed_private_key = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_private_key = sqlc.arg(old_sealed);
