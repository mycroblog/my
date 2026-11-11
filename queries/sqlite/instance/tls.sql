-- SPDX-License-Identifier: AGPL-3.0-only

-- InsertTLSAccount inserts the ACME account of a directory unless one exists.
-- name: InsertTLSAccount :execrows
INSERT INTO tls_acme_accounts (id, directory_url, account_url, sealed_private_key, created_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (directory_url) DO NOTHING;

-- GetTLSAccount returns the ACME account of a directory.
-- name: GetTLSAccount :one
SELECT id, directory_url, account_url, sealed_private_key, created_at FROM tls_acme_accounts WHERE directory_url = ?;

-- InsertTLSOrderAttempt records an order of a certificate.
-- name: InsertTLSOrderAttempt :exec
INSERT INTO tls_order_attempts (id, host, directory_url, profile, replaces_certificate_id, occurred_at) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- InsertTLSOrderFailure records the failure of an order.
-- name: InsertTLSOrderFailure :exec
INSERT INTO tls_order_failures (attempt_id, problem_type, detail, occurred_at) VALUES (?, ?, ?, ?)
ON CONFLICT (attempt_id) DO NOTHING;

-- InsertTLSCertificate records an issued certificate unless its attempt has one.
-- name: InsertTLSCertificate :execrows
INSERT INTO tls_certificates (id, attempt_id, host, directory_url, serial, not_before, not_after, chain_pem, sealed_private_key, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (attempt_id) DO NOTHING;

-- InsertTLSCertificateRevocation records the revocation of a certificate.
-- name: InsertTLSCertificateRevocation :exec
INSERT INTO tls_certificate_revocations (id, certificate_id, reason, revoked_by_user_id, occurred_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- GetCurrentTLSCertificate returns the last certificate issued for a host that is not revoked.
-- name: GetCurrentTLSCertificate :one
SELECT c.id, c.attempt_id, c.host, c.directory_url, c.serial, c.not_before, c.not_after, c.chain_pem, c.sealed_private_key, c.created_at
FROM tls_certificates c
WHERE c.host = ? AND NOT EXISTS (SELECT 1 FROM tls_certificate_revocations r WHERE r.certificate_id = c.id)
ORDER BY c.created_at DESC, c.id DESC LIMIT 1;

-- GetCurrentTLSCertificateID returns the ID of the last certificate issued for a host that is not revoked.
-- name: GetCurrentTLSCertificateID :one
SELECT c.id
FROM tls_certificates c
WHERE c.host = ? AND NOT EXISTS (SELECT 1 FROM tls_certificate_revocations r WHERE r.certificate_id = c.id)
ORDER BY c.created_at DESC, c.id DESC LIMIT 1;

-- ListUnresultedTLSOrderAttempts returns the orders of a host after a time that have no certificate, oldest first.
-- name: ListUnresultedTLSOrderAttempts :many
SELECT a.id, a.host, a.directory_url, a.profile, a.replaces_certificate_id, a.occurred_at
FROM tls_order_attempts a
WHERE a.host = sqlc.arg(host) AND a.occurred_at > sqlc.arg(after)
  AND NOT EXISTS (SELECT 1 FROM tls_certificates c WHERE c.attempt_id = a.id)
ORDER BY a.occurred_at, a.id;
