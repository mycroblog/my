// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"crypto/sha256"
	"time"
)

// KeyAlgorithm is the signature algorithm of an actor key.
type KeyAlgorithm string

// Key algorithms (account_keys.algorithm).
const (
	// KeyRSASHA256 is RSASSA-PKCS1-v1_5 with SHA-256 over a 2048-bit RSA key: the key every
	// implementation verifies with draft-cavage HTTP signatures (ADR-0006, ADR-0019 §6).
	KeyRSASHA256 KeyAlgorithm = "rsa_sha256"
	// KeyEd25519 is Ed25519, for RFC 9421 signatures and object integrity proofs.
	KeyEd25519 KeyAlgorithm = "ed25519"
)

// Key is a public signing key of an actor.
type Key struct {
	ID           string
	AccountID    string
	KeyID        string // the key's IRI, which signatures name in keyId
	Algorithm    KeyAlgorithm
	PublicKeyPEM string // PKIX "PUBLIC KEY" block
	Fingerprint  []byte // SHA-256 of the PKIX DER, which tells a rotated key under the same KeyID apart
	CreatedAt    time.Time
}

// KeyIRI returns the IRI of a local actor's key of algorithm alg. The RSA key is
// {actor}#main-key (ADR-0018), where Mastodon and others look for publicKey; the Ed25519 key is
// {actor}#ed25519-key.
func KeyIRI(actorURI string, alg KeyAlgorithm) string {
	if alg == KeyEd25519 {
		return actorURI + "#ed25519-key"
	}
	return actorURI + "#main-key"
}

// KeyFingerprint returns the fingerprint of a public key: the SHA-256 of its PKIX DER encoding
// (account_keys.fingerprint).
func KeyFingerprint(pkixDER []byte) []byte {
	sum := sha256.Sum256(pkixDER)
	return sum[:]
}
