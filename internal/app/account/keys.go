// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
)

// RSAKeyBits is the size of generated RSA keys (ADR-0019 §6).
const RSAKeyBits = 2048

// LocalKeyAlgorithms are the algorithms of the keys every local actor gets: RSA for
// draft-cavage HTTP signatures, which every implementation verifies (ADR-0006), and Ed25519 for
// RFC 9421 signatures and object integrity proofs.
var LocalKeyAlgorithms = []account.KeyAlgorithm{account.KeyRSASHA256, account.KeyEd25519}

// KeyGenerator generates a private key of an algorithm.
type KeyGenerator func(alg account.KeyAlgorithm) (crypto.Signer, error)

// GenerateKey generates an RSA key of RSAKeyBits or an Ed25519 key with crypto/rand.
func GenerateKey(alg account.KeyAlgorithm) (crypto.Signer, error) {
	switch alg {
	case account.KeyRSASHA256:
		return rsa.GenerateKey(rand.Reader, RSAKeyBits)
	case account.KeyEd25519:
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	default:
		return nil, fmt.Errorf("account: unknown key algorithm %q", alg)
	}
}

// PrivateKeyAAD binds a sealed private key to its row and column (ADR-0019 §6).
func PrivateKeyAAD(accountKeyID string) []byte {
	return kernel.AAD(accountKeyID, "local_account_private_keys.private_key_enc")
}

// encodePublicKey returns the PKIX PEM block and the fingerprint of a public key.
func encodePublicKey(pub crypto.PublicKey) (pemBlock string, fingerprint []byte, err error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", nil, err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), account.KeyFingerprint(der), nil
}

// ParsePublicKeyPEM parses a key's PublicKeyPEM.
func ParsePublicKeyPEM(s string) (crypto.PublicKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("account: not a PEM public key")
	}
	return x509.ParsePKIXPublicKey(block.Bytes)
}

// checkAlgorithm reports whether key is of the algorithm alg.
func checkAlgorithm(alg account.KeyAlgorithm, key crypto.Signer) error {
	switch key.(type) {
	case *rsa.PrivateKey:
		if alg == account.KeyRSASHA256 {
			return nil
		}
	case ed25519.PrivateKey:
		if alg == account.KeyEd25519 {
			return nil
		}
	}
	return fmt.Errorf("account: a %T is not a %s key", key, alg)
}
