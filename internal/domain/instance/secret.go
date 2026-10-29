// SPDX-License-Identifier: AGPL-3.0-only

package instance

import "time"

// SecretName names a secret of the instance (instance_secrets), which the server generates once
// and keeps sealed in the database.
type SecretName string

// SecretMediaProxyKey is the HMAC-SHA256 key that signs the media proxy URLs (ADR-0121 §2).
const SecretMediaProxyKey SecretName = "media_proxy_key"

// Secret is a stored secret of the instance. Sealed is the secret sealed with the Sealer, bound
// to ID.
type Secret struct {
	ID        string
	Name      SecretName
	Sealed    []byte
	CreatedAt time.Time
}
