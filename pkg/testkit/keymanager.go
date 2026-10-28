// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
)

// KeyManager is an in-memory kernel.KeyManager for tests: it wraps with AES-256-GCM under a key
// derived from its key ID. It passes KeyManagerContract but protects nothing; never use it outside tests.
type KeyManager struct {
	id   string
	aead cipher.AEAD
}

var _ kernel.KeyManager = (*KeyManager)(nil)

// NewKeyManager returns a KeyManager whose only KEK has the given ID.
func NewKeyManager(id string) *KeyManager {
	key := sha256.Sum256([]byte("testkit/kek/" + id))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		panic(err)
	}
	return &KeyManager{id: id, aead: aead}
}

// CurrentKeyID implements kernel.KeyManager.
func (k *KeyManager) CurrentKeyID() string { return k.id }

// WrapKey implements kernel.KeyManager.
func (k *KeyManager) WrapKey(_ context.Context, dek, aad []byte) (string, []byte, error) {
	return k.id, k.aead.Seal(nil, nil, dek, aad), nil //nolint:gosec // G407 false positive: NewGCMWithRandomNonce generates the nonce and requires a nil one
}

// KeyRing is a kernel.KeyManager for tests that holds several KEKs of KeyManager, like a
// configuration with kek and kek_previous: it wraps under the current one and unwraps under any.
type KeyRing struct {
	current *KeyManager
	keys    map[string]*KeyManager
}

var _ kernel.KeyManager = (*KeyRing)(nil)

// NewKeyRing returns a KeyRing that wraps under the KEK current and also unwraps under previous.
// Each ID names the same KEK as NewKeyManager(id).
func NewKeyRing(current string, previous ...string) *KeyRing {
	r := &KeyRing{current: NewKeyManager(current), keys: map[string]*KeyManager{}}
	r.keys[current] = r.current
	for _, id := range previous {
		r.keys[id] = NewKeyManager(id)
	}
	return r
}

// CurrentKeyID implements kernel.KeyManager.
func (r *KeyRing) CurrentKeyID() string { return r.current.id }

// WrapKey implements kernel.KeyManager.
func (r *KeyRing) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	return r.current.WrapKey(ctx, dek, aad)
}

// UnwrapKey implements kernel.KeyManager.
func (r *KeyRing) UnwrapKey(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
	k, ok := r.keys[id]
	if !ok {
		return nil, fmt.Errorf("key %q: %w", id, kernel.ErrUnknownKey)
	}
	return k.UnwrapKey(ctx, id, wrapped, aad)
}

// UnwrapKey implements kernel.KeyManager.
func (k *KeyManager) UnwrapKey(_ context.Context, id string, wrapped, aad []byte) ([]byte, error) {
	if id != k.id {
		return nil, fmt.Errorf("key %q: %w", id, kernel.ErrUnknownKey)
	}
	dek, err := k.aead.Open(nil, nil, wrapped, aad)
	if err != nil {
		return nil, kernel.ErrDecrypt
	}
	return dek, nil
}
