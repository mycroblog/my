// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"strings"
)

// The pair of a rotation is kept for a retry of the rotation (ADR-0090 §18), encrypted under a
// key derived from the refresh token the rotation replaced. The server stores only that token's
// SHA-256 digest, a different derivation, so the database alone cannot open the pair.

// replayKeyInfo is the HKDF info of the key of a replay pair; it separates the key from any
// other use of the refresh token.
const replayKeyInfo = "my oauth refresh replay pair v1"

// replayAEAD returns AES-256-GCM, with a random nonce, under the key derived from refreshToken.
func replayAEAD(refreshToken string) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, []byte(refreshToken), nil, replayKeyInfo, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

// sealReplayPair encrypts the access token and the refresh token of issued, the pair a rotation
// of refreshToken issued, bound to the ID of issued's token.
func sealReplayPair(refreshToken string, issued IssuedTokens) ([]byte, error) {
	aead, err := replayAEAD(refreshToken)
	if err != nil {
		return nil, err
	}
	pair := []byte(issued.AccessToken + " " + issued.RefreshToken)
	return aead.Seal(nil, nil, pair, []byte(issued.Token.ID)), nil //nolint:gosec // G407 false positive: NewGCMWithRandomNonce generates the nonce and requires a nil one
}

// openReplayPair decrypts the replay pair of the token with the ID tokenID with the key derived
// from refreshToken; ok is false when refreshToken is not the token the pair was sealed for.
func openReplayPair(refreshToken, tokenID string, sealed []byte) (accessToken, newRefreshToken string, ok bool) {
	aead, err := replayAEAD(refreshToken)
	if err != nil {
		return "", "", false
	}
	plain, err := aead.Open(nil, nil, sealed, []byte(tokenID))
	if err != nil {
		return "", "", false
	}
	return strings.Cut(string(plain), " ")
}
