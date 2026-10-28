// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// sealedVersion is the first byte of every sealed value.
const sealedVersion = 1

// dekSize is the length of the AES-256 data-encryption key.
const dekSize = 32

// Sealer encrypts secrets for storage with envelope encryption (ADR-0019 §6):
// each secret gets a fresh random AES-256 data-encryption key (DEK), the secret is encrypted with
// AES-256-GCM under the DEK, and the DEK is wrapped by the KeyManager's current key-encryption key (KEK).
//
// A sealed value is self-describing:
//
//	version (1 byte, = 1)
//	key ID length (1 byte) | key ID
//	wrapped DEK length (2 bytes, big endian) | wrapped DEK
//	nonce (12 bytes) | AES-256-GCM ciphertext and tag of the secret under the DEK
//
// The additional authenticated data (AAD) binds a sealed value to where it is stored,
// so that a ciphertext copied to another row or column fails to open. Build it with AAD
// from the owning row's ID and the column name. Both the DEK wrap and the secret are bound to it.
type Sealer struct {
	km KeyManager
}

// NewSealer returns a Sealer that wraps DEKs with km.
func NewSealer(km KeyManager) *Sealer {
	return &Sealer{km: km}
}

// CurrentKeyID returns the ID of the KEK that Seal and Rewrap wrap DEKs with.
func (s *Sealer) CurrentKeyID() string { return s.km.CurrentKeyID() }

// AAD encodes parts, such as an actor ID and a column name, as additional authenticated data.
// Each part is length-prefixed, so different lists of parts never encode to the same bytes.
func AAD(parts ...string) []byte {
	var b []byte
	for _, p := range parts {
		b = binary.AppendUvarint(b, uint64(len(p)))
		b = append(b, p...)
	}
	return b
}

// Seal encrypts plaintext bound to aad.
func (s *Sealer) Seal(ctx context.Context, plaintext, aad []byte) ([]byte, error) {
	dek := make([]byte, dekSize)
	_, _ = rand.Read(dek) // crypto/rand.Read never returns an error
	keyID, wrapped, err := s.km.WrapKey(ctx, dek, aad)
	if err != nil {
		return nil, fmt.Errorf("seal: wrap key: %w", err)
	}
	aead, err := newAEAD(dek)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	out, err := appendHeader(nil, keyID, wrapped)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	return aead.Seal(out, nil, plaintext, aad), nil //nolint:gosec // G407 false positive: NewGCMWithRandomNonce generates the nonce and requires a nil one
}

// Open decrypts a value sealed by Seal with the same aad. The error wraps ErrUnknownKey when the
// value was sealed under a KEK the KeyManager no longer holds, and ErrDecrypt when the value is
// malformed, was tampered with, or aad differs.
func (s *Sealer) Open(ctx context.Context, sealed, aad []byte) ([]byte, error) {
	h, err := parseSealed(sealed)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	dek, err := s.km.UnwrapKey(ctx, h.keyID, h.wrapped, aad)
	if err != nil {
		return nil, fmt.Errorf("open: unwrap key %q: %w", h.keyID, err)
	}
	aead, err := newAEAD(dek)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	plaintext, err := aead.Open(nil, nil, h.body, aad)
	if err != nil {
		return nil, fmt.Errorf("open: %w", ErrDecrypt)
	}
	return plaintext, nil
}

// Rewrap re-wraps the DEK of a sealed value under the KeyManager's current KEK, keeping the
// encrypted secret as is. It returns sealed unchanged when it is already wrapped under the current KEK.
// Rewrapping every stored secret lets the previous KEK be removed after a rotation.
func (s *Sealer) Rewrap(ctx context.Context, sealed, aad []byte) ([]byte, error) {
	h, err := parseSealed(sealed)
	if err != nil {
		return nil, fmt.Errorf("rewrap: %w", err)
	}
	if h.keyID == s.km.CurrentKeyID() {
		return sealed, nil
	}
	dek, err := s.km.UnwrapKey(ctx, h.keyID, h.wrapped, aad)
	if err != nil {
		return nil, fmt.Errorf("rewrap: unwrap key %q: %w", h.keyID, err)
	}
	// Check the secret too, so that a tampered body is not carried over under the new KEK.
	aead, err := newAEAD(dek)
	if err != nil {
		return nil, fmt.Errorf("rewrap: %w", err)
	}
	if _, err := aead.Open(nil, nil, h.body, aad); err != nil {
		return nil, fmt.Errorf("rewrap: %w", ErrDecrypt)
	}
	keyID, wrapped, err := s.km.WrapKey(ctx, dek, aad)
	if err != nil {
		return nil, fmt.Errorf("rewrap: wrap key: %w", err)
	}
	out, err := appendHeader(nil, keyID, wrapped)
	if err != nil {
		return nil, fmt.Errorf("rewrap: %w", err)
	}
	return append(out, h.body...), nil
}

// SealedKeyID returns the ID of the KEK that wraps a sealed value's DEK, without decrypting it.
// Comparing it with KeyManager.CurrentKeyID finds values that still need Rewrap.
func SealedKeyID(sealed []byte) (string, error) {
	h, err := parseSealed(sealed)
	if err != nil {
		return "", err
	}
	return h.keyID, nil
}

type sealedParts struct {
	keyID   string
	wrapped []byte
	body    []byte // nonce, ciphertext, and tag
}

func appendHeader(b []byte, keyID string, wrapped []byte) ([]byte, error) {
	if len(keyID) == 0 || len(keyID) > 0xff {
		return nil, fmt.Errorf("key ID must be 1 to 255 bytes, got %d", len(keyID))
	}
	if len(wrapped) > 0xffff {
		return nil, fmt.Errorf("wrapped key must be at most 65535 bytes, got %d", len(wrapped))
	}
	b = append(b, sealedVersion, byte(len(keyID))) //nolint:gosec // G115: length checked above
	b = append(b, keyID...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(wrapped))) //nolint:gosec // G115: length checked above
	return append(b, wrapped...), nil
}

func parseSealed(b []byte) (sealedParts, error) {
	malformed := fmt.Errorf("malformed sealed value: %w", ErrDecrypt)
	if len(b) < 2 || b[0] != sealedVersion {
		return sealedParts{}, malformed
	}
	n := int(b[1])
	b = b[2:]
	if n == 0 || len(b) < n+2 {
		return sealedParts{}, malformed
	}
	keyID := string(b[:n])
	b = b[n:]
	m := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	// The body holds at least a nonce and a tag.
	const minBody = 12 + 16
	if len(b) < m+minBody {
		return sealedParts{}, malformed
	}
	return sealedParts{keyID: keyID, wrapped: b[:m], body: b[m:]}, nil
}

// newAEAD returns AES-256-GCM with a random 96-bit nonce prepended to each ciphertext.
// A DEK encrypts exactly one secret, so nonce reuse cannot happen.
func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != dekSize {
		return nil, fmt.Errorf("data-encryption key must be %d bytes, got %d: %w", dekSize, len(key), ErrDecrypt)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
