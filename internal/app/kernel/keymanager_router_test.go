// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

func TestKeyIDKind(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]string{
		"local:0123456789abcdef": "local",
		"gcpkms:projects/p/locations/global/keyRings/r/cryptoKeys/k": "gcpkms",
		"awskms:arn:aws:kms:us-east-1:111122223333:key/k":            "awskms",
		"no-kind": "",
	} {
		if got := kernel.KeyIDKind(id); got != want {
			t.Errorf("KeyIDKind(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestKeyRouterMovesSealedValuesToAnotherKind seals under a local KeyManager, then opens and
// rewraps under a router whose current KeyManager is of another kind.
func TestKeyRouterMovesSealedValuesToAnotherKind(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	local := newFakeKeyManager(t, "local:old")
	kms := newFakeKeyManager(t, "gcpkms:projects/p/locations/global/keyRings/r/cryptoKeys/k")
	aad := kernel.AAD("row-1", "secret")
	sealed, err := kernel.NewSealer(local).Seal(ctx, []byte("actor key"), aad)
	if err != nil {
		t.Fatal(err)
	}

	router, err := kernel.NewKeyRouter(kms, local)
	if err != nil {
		t.Fatal(err)
	}
	if router.CurrentKeyID() != kms.CurrentKeyID() {
		t.Errorf("CurrentKeyID = %q, want %q", router.CurrentKeyID(), kms.CurrentKeyID())
	}
	s := kernel.NewSealer(router)
	if got, err := s.Open(ctx, sealed, aad); err != nil || string(got) != "actor key" {
		t.Fatalf("Open of the local value = %q, %v", got, err)
	}
	moved, err := s.Rewrap(ctx, sealed, aad)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := kernel.SealedKeyID(moved); id != kms.CurrentKeyID() {
		t.Errorf("key ID after Rewrap = %q, want %q", id, kms.CurrentKeyID())
	}
	// The KMS alone opens the moved value, so the local KEK can go.
	if got, err := kernel.NewSealer(kms).Open(ctx, moved, aad); err != nil || string(got) != "actor key" {
		t.Errorf("Open under the KMS alone = %q, %v", got, err)
	}
	if fresh, err := s.Seal(ctx, []byte("new"), aad); err != nil {
		t.Fatal(err)
	} else if id, _ := kernel.SealedKeyID(fresh); id != kms.CurrentKeyID() {
		t.Errorf("Seal wraps under %q, want the current %q", id, kms.CurrentKeyID())
	}
}

func TestKeyRouterRejectsUnknownKinds(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	local := newFakeKeyManager(t, "local:k")
	router, err := kernel.NewKeyRouter(newFakeKeyManager(t, "gcpkms:k"), local)
	if err != nil {
		t.Fatal(err)
	}
	other, err := kernel.NewSealer(newFakeKeyManager(t, "awskms:k")).Seal(ctx, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kernel.NewSealer(router).Open(ctx, other, nil); !errors.Is(err, kernel.ErrUnknownKey) {
		t.Errorf("Open of a value of another kind = %v, want ErrUnknownKey", err)
	}
}

func TestNewKeyRouter(t *testing.T) {
	t.Parallel()
	only := newFakeKeyManager(t, "local:k")
	if km, err := kernel.NewKeyRouter(only); err != nil || km != kernel.KeyManager(only) {
		t.Errorf("NewKeyRouter without openers = %v, %v; want the current KeyManager", km, err)
	}
	if _, err := kernel.NewKeyRouter(newFakeKeyManager(t, "gcpkms:a"), newFakeKeyManager(t, "gcpkms:b")); err == nil {
		t.Error("NewKeyRouter of two KeyManagers of one kind succeeded")
	}
	if _, err := kernel.NewKeyRouter(newFakeKeyManager(t, "plain"), only); err == nil {
		t.Error("NewKeyRouter of a key ID without a kind succeeded")
	}
}
