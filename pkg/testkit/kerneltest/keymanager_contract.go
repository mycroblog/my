// SPDX-License-Identifier: AGPL-3.0-only

package kerneltest

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

// KeyManagerContract verifies that km satisfies the kernel.KeyManager contract: wrapped keys
// round-trip under the current key ID, wrapping is randomized, a different AAD or any changed byte
// fails with kernel.ErrDecrypt, an unknown key ID fails with kernel.ErrUnknownKey, and a Sealer
// built on km opens what it seals.
func KeyManagerContract(t *testing.T, km kernel.KeyManager) {
	t.Helper()
	ctx := context.Background()
	dek := bytes.Repeat([]byte{0x42}, 32)
	aad := kernel.AAD("actor-1", "private_key")

	id := km.CurrentKeyID()
	if len(id) == 0 || len(id) > 255 {
		t.Fatalf("CurrentKeyID() = %q, want 1 to 255 bytes", id)
	}
	gotID, wrapped, err := km.WrapKey(ctx, dek, aad)
	if err != nil {
		t.Fatalf("WrapKey() error = %v", err)
	}
	if gotID != id {
		t.Fatalf("WrapKey() key ID = %q, want CurrentKeyID() %q", gotID, id)
	}
	if bytes.Contains(wrapped, dek) {
		t.Fatalf("WrapKey() output contains the plaintext key")
	}
	got, err := km.UnwrapKey(ctx, id, wrapped, aad)
	if err != nil {
		t.Fatalf("UnwrapKey() error = %v", err)
	}
	if !bytes.Equal(got, dek) {
		t.Fatalf("UnwrapKey() = %x, want %x", got, dek)
	}

	if _, again, err := km.WrapKey(ctx, dek, aad); err != nil {
		t.Fatalf("WrapKey() error = %v", err)
	} else if bytes.Equal(again, wrapped) {
		t.Fatalf("WrapKey() returned the same output twice, want randomized wrapping")
	}

	if _, err := km.UnwrapKey(ctx, id, wrapped, kernel.AAD("actor-2", "private_key")); !errors.Is(err, kernel.ErrDecrypt) {
		t.Fatalf("UnwrapKey() with another AAD: error = %v, want ErrDecrypt", err)
	}
	for i := range wrapped {
		tampered := bytes.Clone(wrapped)
		tampered[i] ^= 0x01
		if _, err := km.UnwrapKey(ctx, id, tampered, aad); !errors.Is(err, kernel.ErrDecrypt) {
			t.Fatalf("UnwrapKey() with byte %d flipped: error = %v, want ErrDecrypt", i, err)
		}
	}
	if _, err := km.UnwrapKey(ctx, id, wrapped[:len(wrapped)-1], aad); !errors.Is(err, kernel.ErrDecrypt) {
		t.Fatalf("UnwrapKey() truncated: error = %v, want ErrDecrypt", err)
	}
	if _, err := km.UnwrapKey(ctx, "testkit:unknown", wrapped, aad); !errors.Is(err, kernel.ErrUnknownKey) {
		t.Fatalf("UnwrapKey() with an unknown key ID: error = %v, want ErrUnknownKey", err)
	}

	s := kernel.NewSealer(km)
	secret := []byte("-----BEGIN PRIVATE KEY-----")
	sealed, err := s.Seal(ctx, secret, aad)
	if err != nil {
		t.Fatalf("Sealer.Seal() error = %v", err)
	}
	opened, err := s.Open(ctx, sealed, aad)
	if err != nil {
		t.Fatalf("Sealer.Open() error = %v", err)
	}
	if !bytes.Equal(opened, secret) {
		t.Fatalf("Sealer.Open() = %q, want %q", opened, secret)
	}
}
