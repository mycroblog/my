// SPDX-License-Identifier: AGPL-3.0-only

package gcpkms_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mycroblog/my/internal/adapter/keymanager/gcpkms"
	"github.com/mycroblog/my/internal/adapter/keymanager/gcpkms/gcpkmstest"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	keyA = "projects/my-project/locations/global/keyRings/my/cryptoKeys/kek"
	keyB = "projects/my-project/locations/asia-northeast1/keyRings/my/cryptoKeys/kek-2"
)

var (
	dek = bytes.Repeat([]byte{0x42}, 32)
	aad = kernel.AAD("actor-1", "private_key")
)

func newKeyManager(t *testing.T, key string, previous ...string) (*gcpkms.KeyManager, *gcpkmstest.Server) {
	t.Helper()
	srv, opts := gcpkmstest.Start(t, append([]string{key}, previous...)...)
	km, err := gcpkms.New(t.Context(), gcpkms.Options{Key: key, PreviousKeys: previous, ClientOptions: opts})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = km.Close() })
	return km, srv
}

func TestContract(t *testing.T) {
	t.Parallel()
	km, _ := newKeyManager(t, keyA)
	testkit.KeyManagerContract(t, km)
	km2, _ := newKeyManager(t, keyB, keyA)
	testkit.KeyManagerContract(t, km2)
}

func TestKeyID(t *testing.T) {
	t.Parallel()
	km, _ := newKeyManager(t, keyA)
	if got, want := km.CurrentKeyID(), "gcpkms:"+keyA; got != want {
		t.Errorf("CurrentKeyID() = %q, want %q", got, want)
	}
	if got := kernel.KeyIDKind(km.CurrentKeyID()); got != gcpkms.Kind {
		t.Errorf("KeyIDKind() = %q, want %q", got, gcpkms.Kind)
	}
}

func TestCheckKeyName(t *testing.T) {
	t.Parallel()
	longest := "projects/" + strings.Repeat("p", 30) + "/locations/northamerica-northeast1/keyRings/" + strings.Repeat("r", 63) + "/cryptoKeys/" + strings.Repeat("k", 63)
	for _, name := range []string{keyA, keyB, "projects/example.com:my-project/locations/us/keyRings/r/cryptoKeys/k_1", longest} {
		if err := gcpkms.CheckKeyName(name); err != nil {
			t.Errorf("CheckKeyName(%q) = %v, want nil", name, err)
		}
	}
	tooLong := "projects/" + strings.Repeat("p", 120) + "/locations/global/keyRings/" + strings.Repeat("r", 63) + "/cryptoKeys/" + strings.Repeat("k", 63)
	for name, want := range map[string]string{
		"":                            "resource name",
		"kek":                         "resource name",
		keyA + "/cryptoKeyVersions/1": "without a version",
		"projects/p/locations/global/keyRings/r/cryptoKeys/" + strings.Repeat("k", 64): "resource name",
		"projects/p/locations/global/keyRings/r/cryptoKeys/k\t":                        "resource name",
		tooLong: "at most 248 bytes",
	} {
		if err := gcpkms.CheckKeyName(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckKeyName(%q) = %v, want an error with %q", name, err, want)
		}
	}
}

func TestNewRejects(t *testing.T) {
	t.Parallel()
	_, opts := gcpkmstest.Start(t, keyA)
	for name, o := range map[string]gcpkms.Options{
		"bad name":  {Key: "kek"},
		"versioned": {Key: keyA, PreviousKeys: []string{keyB + "/cryptoKeyVersions/2"}},
		"repeated":  {Key: keyA, PreviousKeys: []string{keyA}},
	} {
		o.ClientOptions = opts
		if km, err := gcpkms.New(t.Context(), o); err == nil {
			_ = km.Close()
			t.Errorf("%s: New() succeeded", name)
		}
	}
}

func TestPreviousKeyOnlyUnwraps(t *testing.T) {
	t.Parallel()
	_, opts := gcpkmstest.Start(t, keyA, keyB)
	old, err := gcpkms.New(t.Context(), gcpkms.Options{Key: keyA, ClientOptions: opts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = old.Close() })
	id, wrapped, err := old.WrapKey(t.Context(), dek, aad)
	if err != nil {
		t.Fatal(err)
	}

	km, err := gcpkms.New(t.Context(), gcpkms.Options{Key: keyB, PreviousKeys: []string{keyA}, ClientOptions: opts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = km.Close() })
	if got, err := km.UnwrapKey(t.Context(), id, wrapped, aad); err != nil || !bytes.Equal(got, dek) {
		t.Errorf("UnwrapKey() under the previous key = %x, %v, want the DEK", got, err)
	}
	if newID, _, err := km.WrapKey(t.Context(), dek, aad); err != nil || newID != "gcpkms:"+keyB {
		t.Errorf("WrapKey() = %q, %v, want the current key %q", newID, err, "gcpkms:"+keyB)
	}
	// A ciphertext of one CryptoKey does not open under another.
	if _, err := km.UnwrapKey(t.Context(), "gcpkms:"+keyB, wrapped, aad); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("UnwrapKey() under another key: error = %v, want ErrDecrypt", err)
	}
	if _, err := km.UnwrapKey(t.Context(), "local:0011223344556677", wrapped, aad); !errors.Is(err, kernel.ErrUnknownKey) {
		t.Errorf("UnwrapKey() with a local key ID: error = %v, want ErrUnknownKey", err)
	}
}

// TestRotation checks that a rotation in Cloud KMS keeps the key ID and opens what the earlier
// versions wrapped, and that a disabled version no longer opens its values.
func TestRotation(t *testing.T) {
	t.Parallel()
	km, srv := newKeyManager(t, keyA)
	id, wrapped, err := km.WrapKey(t.Context(), dek, aad)
	if err != nil {
		t.Fatal(err)
	}
	srv.Rotate(keyA)
	id2, wrapped2, err := km.WrapKey(t.Context(), dek, aad)
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Errorf("key ID after a rotation = %q, want %q", id2, id)
	}
	for _, w := range [][]byte{wrapped, wrapped2} {
		if got, err := km.UnwrapKey(t.Context(), id, w, aad); err != nil || !bytes.Equal(got, dek) {
			t.Errorf("UnwrapKey() after a rotation = %x, %v, want the DEK", got, err)
		}
	}

	srv.DisableVersion(keyA, 1)
	_, err = km.UnwrapKey(t.Context(), id, wrapped, aad)
	if status.Code(err) != codes.FailedPrecondition || errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("UnwrapKey() under a disabled version: error = %v, want FAILED_PRECONDITION and not ErrDecrypt", err)
	}
	if got, err := km.UnwrapKey(t.Context(), id, wrapped2, aad); err != nil || !bytes.Equal(got, dek) {
		t.Errorf("UnwrapKey() under the enabled version = %x, %v, want the DEK", got, err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		setup func(*gcpkmstest.Server)
		code  codes.Code
		hint  string
	}{
		{"permission denied", func(s *gcpkmstest.Server) { s.Deny(keyA) }, codes.PermissionDenied, "roles/cloudkms.cryptoKeyEncrypterDecrypter"},
		{"quota", func(s *gcpkmstest.Server) { s.FailNext(codes.ResourceExhausted) }, codes.ResourceExhausted, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			km, srv := newKeyManager(t, keyA)
			id, wrapped, err := km.WrapKey(t.Context(), dek, aad)
			if err != nil {
				t.Fatal(err)
			}
			c.setup(srv)
			_, err = km.UnwrapKey(t.Context(), id, wrapped, aad)
			if status.Code(err) != c.code || errors.Is(err, kernel.ErrDecrypt) || errors.Is(err, kernel.ErrUnknownKey) {
				t.Errorf("UnwrapKey() error = %v, want %s and neither ErrDecrypt nor ErrUnknownKey", err, c.code)
			}
			if !strings.Contains(fmt.Sprint(err), c.hint) {
				t.Errorf("UnwrapKey() error = %v, want the hint %q", err, c.hint)
			}
		})
	}

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()
		_, opts := gcpkmstest.Start(t)
		km, err := gcpkms.New(t.Context(), gcpkms.Options{Key: keyA, ClientOptions: opts})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = km.Close() })
		if _, _, err := km.WrapKey(t.Context(), dek, aad); status.Code(err) != codes.NotFound {
			t.Errorf("WrapKey() error = %v, want NOT_FOUND", err)
		}
	})

	t.Run("transient failure is retried", func(t *testing.T) {
		t.Parallel()
		km, srv := newKeyManager(t, keyA)
		id, wrapped, err := km.WrapKey(t.Context(), dek, aad)
		if err != nil {
			t.Fatal(err)
		}
		srv.FailNext(codes.Unavailable)
		if got, err := km.UnwrapKey(t.Context(), id, wrapped, aad); err != nil || !bytes.Equal(got, dek) {
			t.Errorf("UnwrapKey() after an UNAVAILABLE = %x, %v, want the DEK on the retry", got, err)
		}
	})
}

func TestNeverPrintsKeys(t *testing.T) {
	t.Parallel()
	km, _ := newKeyManager(t, keyA, keyB)
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("km", "km", km)
	for _, s := range []string{fmt.Sprint(km), buf.String()} {
		if !strings.Contains(s, "gcpkms:"+keyA) || strings.Contains(s, keyB) {
			t.Errorf("printed %q, want only the current key ID", s)
		}
	}
}
