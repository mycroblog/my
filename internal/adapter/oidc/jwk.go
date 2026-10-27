// SPDX-License-Identifier: AGPL-3.0-only

package oidc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
)

// minRSABits is the smallest RSA modulus accepted (RFC 7518 §3.3).
const minRSABits = 2048

// jwk is a verification key of a JWKS: its key ID, the algorithm it signs with, and the key.
type jwk struct {
	kid string
	alg string
	key crypto.PublicKey
}

// rawJWK is a JSON Web Key as RFC 7517 §4 and RFC 7518 §6 write it.
type rawJWK struct {
	Kty    string   `json:"kty"`
	Use    string   `json:"use"`
	KeyOps []string `json:"key_ops"`
	Alg    string   `json:"alg"`
	Kid    string   `json:"kid"`
	Crv    string   `json:"crv"`
	N      string   `json:"n"`
	E      string   `json:"e"`
	X      string   `json:"x"`
	Y      string   `json:"y"`
}

// parseJWKS returns the verification keys of a JWKS for the algorithms of
// identity.OIDCSigningAlgs. Keys for encryption, of other types or curves, or malformed are
// skipped: the set may hold keys this client does not use.
func parseJWKS(body []byte) ([]jwk, error) {
	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, err
	}
	var keys []jwk
	for _, raw := range set.Keys {
		var k rawJWK
		if json.Unmarshal(raw, &k) != nil {
			continue
		}
		if k.Use != "" && k.Use != "sig" || len(k.KeyOps) > 0 && !slices.Contains(k.KeyOps, "verify") {
			continue
		}
		if key, alg, err := k.publicKey(); err == nil && (k.Alg == "" || k.Alg == alg) {
			keys = append(keys, jwk{kid: k.Kid, alg: alg, key: key})
		}
	}
	return keys, nil
}

// publicKey returns the key and the one algorithm of identity.OIDCSigningAlgs it verifies.
func (k rawJWK) publicKey() (crypto.PublicKey, string, error) {
	switch k.Kty {
	case "RSA":
		n, err := decodeBase64(k.N)
		if err != nil {
			return nil, "", err
		}
		e, err := decodeBase64(k.E)
		if err != nil {
			return nil, "", err
		}
		bigE := new(big.Int).SetBytes(e)
		if !bigE.IsInt64() || bigE.Int64() < 3 || bigE.Int64() > 1<<31-1 || bigE.Bit(0) == 0 {
			return nil, "", errors.New("bad RSA exponent")
		}
		key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(bigE.Int64())}
		if key.N.BitLen() < minRSABits {
			return nil, "", fmt.Errorf("RSA key of %d bits", key.N.BitLen())
		}
		return key, "RS256", nil
	case "EC":
		if k.Crv != "P-256" {
			return nil, "", fmt.Errorf("curve %q", k.Crv)
		}
		x, err := decodeBase64(k.X)
		if err != nil {
			return nil, "", err
		}
		y, err := decodeBase64(k.Y)
		if err != nil {
			return nil, "", err
		}
		if len(x) != 32 || len(y) != 32 {
			return nil, "", errors.New("bad P-256 coordinates")
		}
		key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			return nil, "", err
		}
		return key, "ES256", nil
	case "OKP":
		if k.Crv != "Ed25519" {
			return nil, "", fmt.Errorf("curve %q", k.Crv)
		}
		x, err := decodeBase64(k.X)
		if err != nil {
			return nil, "", err
		}
		if len(x) != ed25519.PublicKeySize {
			return nil, "", errors.New("bad Ed25519 key")
		}
		return ed25519.PublicKey(x), "EdDSA", nil
	}
	return nil, "", fmt.Errorf("key type %q", k.Kty)
}

// verify checks the JWS signature sig of input with k (RFC 7518 §3.3, §3.4; RFC 8037 §3.1).
func (k jwk) verify(input, sig []byte) bool {
	switch key := k.key.(type) {
	case *rsa.PublicKey:
		h := sha256.Sum256(input)
		return rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], sig) == nil
	case *ecdsa.PublicKey:
		if len(sig) != 64 {
			return false
		}
		h := sha256.Sum256(input)
		return ecdsa.Verify(key, h[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:]))
	case ed25519.PublicKey:
		return ed25519.Verify(key, input, sig)
	}
	return false
}

// decodeBase64 decodes unpadded base64url (RFC 7515 §2).
func decodeBase64(s string) ([]byte, error) {
	return base64.RawURLEncoding.Strict().DecodeString(s)
}
