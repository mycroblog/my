// SPDX-License-Identifier: AGPL-3.0-only

package local_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/keymanager/local"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

// kek returns a fixed, distinct test KEK for each b.
func kek(b byte) []byte {
	return bytes.Repeat([]byte{b}, local.KEKSize)
}

func mustNew(t *testing.T, current []byte, previous ...[]byte) *local.KeyManager {
	t.Helper()
	km, err := local.New(current, previous...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return km
}

func TestContract(t *testing.T) {
	kerneltest.KeyManagerContract(t, mustNew(t, kek(1)))
	kerneltest.KeyManagerContract(t, mustNew(t, kek(2), kek(1)))
}

func TestKeyID(t *testing.T) {
	// Key IDs are stored with every secret, so the derivation must never change.
	if got, want := local.KeyID(kek(0)), "local:"+keyIDOfZeroKEK; got != want {
		t.Errorf("KeyID(zero KEK) = %q, want %q", got, want)
	}
	if local.KeyID(kek(1)) == local.KeyID(kek(2)) {
		t.Error("different KEKs share a key ID")
	}
	if got := mustNew(t, kek(2), kek(1)).CurrentKeyID(); got != local.KeyID(kek(2)) {
		t.Errorf("CurrentKeyID() = %q, want the ID of the first KEK %q", got, local.KeyID(kek(2)))
	}
}

// keyIDOfZeroKEK is hex(SHA-256("my/local-kek-id/v1\x00" || 32 zero bytes)[:8]).
const keyIDOfZeroKEK = "1599a4f94b560b08"

func TestNewRejectsBadKEKs(t *testing.T) {
	tests := []struct {
		name     string
		current  []byte
		previous [][]byte
		want     string
	}{
		{"short current", kek(1)[:16], nil, "KEK #0 must be 32 bytes, got 16"},
		{"long previous", kek(1), [][]byte{append(kek(2), 0)}, "KEK #1 must be 32 bytes, got 33"},
		{"duplicate", kek(1), [][]byte{kek(2), kek(1)}, "is given more than once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := local.New(tt.current, tt.previous...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("New() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestRotation(t *testing.T) {
	ctx := context.Background()
	aad := kernel.AAD("actor-1", "private_key")
	secret := []byte("actor private key")

	before := kernel.NewSealer(mustNew(t, kek(1)))
	sealed, err := before.Seal(ctx, secret, aad)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	// The new KEK is current and the old one is kept: old values still open.
	rotating := mustNew(t, kek(2), kek(1))
	during := kernel.NewSealer(rotating)
	if got, err := during.Open(ctx, sealed, aad); err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("Open() during rotation = %q, %v; want %q", got, err, secret)
	}

	rewrapped, err := during.Rewrap(ctx, sealed, aad)
	if err != nil {
		t.Fatalf("Rewrap() error = %v", err)
	}
	if id, _ := kernel.SealedKeyID(rewrapped); id != rotating.CurrentKeyID() {
		t.Errorf("SealedKeyID(rewrapped) = %q, want the new key %q", id, rotating.CurrentKeyID())
	}
	if again, err := during.Rewrap(ctx, rewrapped, aad); err != nil || !bytes.Equal(again, rewrapped) {
		t.Errorf("Rewrap() of a current value = %x, %v; want it unchanged", again, err)
	}

	// The old KEK is removed: only rewrapped values open.
	after := kernel.NewSealer(mustNew(t, kek(2)))
	if got, err := after.Open(ctx, rewrapped, aad); err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("Open(rewrapped) after rotation = %q, %v; want %q", got, err, secret)
	}
	if _, err := after.Open(ctx, sealed, aad); !errors.Is(err, kernel.ErrUnknownKey) {
		t.Errorf("Open(not rewrapped) after rotation: error = %v, want ErrUnknownKey", err)
	}
}

func TestOpenGolden(t *testing.T) {
	// A value sealed under kek(7) by an earlier build. Stored secrets must keep opening,
	// so a change to the sealed format or the wrapping that breaks this test is a breaking change.
	sealed, err := hex.DecodeString(goldenSealed)
	if err != nil {
		t.Fatal(err)
	}
	s := kernel.NewSealer(mustNew(t, kek(7)))
	got, err := s.Open(context.Background(), sealed, kernel.AAD("actor-1", "private_key"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if want := "golden secret"; string(got) != want {
		t.Errorf("Open() = %q, want %q", got, want)
	}
}

const goldenSealed = "01166c6f63616c3a36313838383033633062633064643739003c6064a6b2db6c36a84c7755db9d575212f5b7f2e078b9cfa24fa06bad769b17c789e61fca047521a9fa8b10f6500d5c65ff570e122c498bb7d0658ee7139f7f812f47730b60e1c06636f65818b47e666d91354165bb1dfff0de789326c275fb1d8a2aaba162"

func TestNoKeysInOutput(t *testing.T) {
	km := mustNew(t, kek(0xab), kek(0xcd))
	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("km", "km", km)
	outputs := map[string]string{
		"%v":     fmt.Sprintf("%v", km),
		"%+v":    fmt.Sprintf("%+v", km),
		"String": km.String(),
		"log":    logs.String(),
	}
	for name, out := range outputs {
		for _, k := range [][]byte{kek(0xab), kek(0xcd)} {
			if strings.Contains(out, hex.EncodeToString(k)) || strings.Contains(out, string(k)) {
				t.Errorf("%s output %q contains a KEK", name, out)
			}
		}
		if !strings.Contains(out, km.CurrentKeyID()) {
			t.Errorf("%s output %q does not name the current key ID", name, out)
		}
	}
}
