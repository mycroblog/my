// SPDX-License-Identifier: AGPL-3.0-only

package instance

import "time"

// SecretName names a secret of the instance (instance_secrets), which the server generates once
// and keeps sealed in the database.
type SecretName string

// SecretMediaProxyKey is the HMAC-SHA256 key that signs the media proxy URLs (ADR-0019 §12).
const SecretMediaProxyKey SecretName = "media_proxy_key"

// SecretIdempotencyKey is the HMAC-SHA256 key of the fingerprints of the requests with an
// Idempotency-Key, which the server keeps with their answers (ADR-0028 §18).
const SecretIdempotencyKey SecretName = "idempotency_key" //nolint:gosec // the name of a secret, not one

// Secret is a stored secret of the instance. Sealed is the secret sealed with the Sealer, bound
// to ID.
type Secret struct {
	ID        string
	Name      SecretName
	Sealed    []byte
	CreatedAt time.Time
}
