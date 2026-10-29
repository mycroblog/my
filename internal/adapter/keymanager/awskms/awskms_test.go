// SPDX-License-Identifier: AGPL-3.0-only

package awskms_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/smithy-go"

	"github.com/mycroblog/my/internal/adapter/keymanager/awskms"
	"github.com/mycroblog/my/internal/adapter/keymanager/awskms/awskmstest"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	keyA = "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
	keyB = "arn:aws:kms:us-east-1:111122223333:key/mrk-0123456789abcdef0123456789abcdef"
)

var (
	dek = bytes.Repeat([]byte{0x42}, 32)
	aad = kernel.AAD("actor-1", "private_key")
)

func newKeyManager(t *testing.T, key string, previous ...string) (*awskms.KeyManager, *awskmstest.Server) {
	t.Helper()
	srv, cfg := awskmstest.Start(t, keyA, keyB)
	km, err := awskms.New(t.Context(), cfg, awskms.Options{Key: key, PreviousKeys: previous})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	if got, want := km.CurrentKeyID(), "awskms:"+keyA; got != want {
		t.Errorf("CurrentKeyID() = %q, want %q", got, want)
	}
	if got := kernel.KeyIDKind(km.CurrentKeyID()); got != awskms.Kind {
		t.Errorf("KeyIDKind() = %q, want %q", got, awskms.Kind)
	}
}

// TestResolvesAliases checks that an alias, an alias ARN, and a key ID resolve to the key ARN,
// so that the key ID does not change when the alias is pointed at another key.
func TestResolvesAliases(t *testing.T) {
	t.Parallel()
	srv, cfg := awskmstest.Start(t, keyA, keyB)
	srv.AddAlias("alias/my-kek", keyA)
	for _, name := range []string{"alias/my-kek", "arn:aws:kms:us-east-1:111122223333:alias/my-kek", "1234abcd-12ab-34cd-56ef-1234567890ab"} {
		km, err := awskms.New(t.Context(), cfg, awskms.Options{Key: name})
		if err != nil {
			t.Fatalf("New(%q) error = %v", name, err)
		}
		if got, want := km.CurrentKeyID(), awskms.KeyID(keyA); got != want {
			t.Errorf("New(%q).CurrentKeyID() = %q, want %q", name, got, want)
		}
	}

	km, err := awskms.New(t.Context(), cfg, awskms.Options{Key: "alias/my-kek"})
	if err != nil {
		t.Fatal(err)
	}
	id, wrapped, err := km.WrapKey(t.Context(), dek, aad)
	if err != nil {
		t.Fatal(err)
	}
	srv.AddAlias("alias/my-kek", keyB)
	if got, err := km.UnwrapKey(t.Context(), id, wrapped, aad); err != nil || !bytes.Equal(got, dek) {
		t.Errorf("UnwrapKey() after the alias moved = %x, %v, want the DEK", got, err)
	}
	if id2, _, err := km.WrapKey(t.Context(), dek, aad); err != nil || id2 != id {
		t.Errorf("WrapKey() after the alias moved = %q, %v, want the resolved key %q until a restart", id2, err, id)
	}
}

func TestCheckKeyName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		keyA, keyB,
		"arn:aws-us-gov:kms:us-gov-west-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab",
		"arn:aws:kms:eu-west-1:111122223333:alias/my/kek_1",
		"alias/my-kek",
		"1234abcd-12ab-34cd-56ef-1234567890ab",
		"mrk-0123456789abcdef0123456789abcdef",
	} {
		if err := awskms.CheckKeyName(name); err != nil {
			t.Errorf("CheckKeyName(%q) = %v, want nil", name, err)
		}
	}
	for name, want := range map[string]string{
		"":    "ARN, an alias",
		"kek": "ARN, an alias",
		"projects/p/locations/global/keyRings/r/cryptoKeys/k":                        "ARN, an alias",
		"arn:aws:kms:us-east-1:1111:key/1234abcd-12ab-34cd-56ef-1234567890ab":        "ARN, an alias",
		"arn:aws:s3:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab": "ARN, an alias",
		"alias/aws/ebs": "AWS managed key",
		"arn:aws:kms:us-east-1:111122223333:alias/aws/rds": "AWS managed key",
		"alias/" + strings.Repeat("k", 243):                "at most 248 bytes",
	} {
		if err := awskms.CheckKeyName(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckKeyName(%q) = %v, want an error with %q", name, err, want)
		}
	}
}

func TestNewRejects(t *testing.T) {
	t.Parallel()
	const asymmetric = "arn:aws:kms:us-east-1:111122223333:key/00000000-0000-0000-0000-000000000001"
	const missing = "arn:aws:kms:us-east-1:111122223333:key/00000000-0000-0000-0000-000000000002"
	const disabled = "arn:aws:kms:us-east-1:111122223333:key/00000000-0000-0000-0000-000000000003"
	srv, cfg := awskmstest.Start(t, keyA, disabled)
	srv.AddAsymmetricKey(asymmetric)
	srv.Disable(disabled)
	srv.AddAlias("alias/a", keyA)
	for name, c := range map[string]struct {
		o    awskms.Options
		want string
	}{
		"bad name":   {awskms.Options{Key: "kek"}, "ARN, an alias"},
		"repeated":   {awskms.Options{Key: keyA, PreviousKeys: []string{"alias/a"}}, "more than once"},
		"asymmetric": {awskms.Options{Key: asymmetric}, "not a symmetric encryption key"},
		"missing":    {awskms.Options{Key: missing}, "does not exist"},
		"disabled":   {awskms.Options{Key: disabled}, "is Disabled"},
	} {
		if _, err := awskms.New(t.Context(), cfg, c.o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: New() error = %v, want one with %q", name, err, c.want)
		}
	}
	noRegion := cfg
	noRegion.Region = ""
	if _, err := awskms.New(t.Context(), noRegion, awskms.Options{Key: "alias/a"}); err == nil || !strings.Contains(err.Error(), "AWS_REGION") {
		t.Errorf("New() of an alias without a region: error = %v, want one naming AWS_REGION", err)
	}
	if _, err := awskms.New(t.Context(), noRegion, awskms.Options{Key: keyA}); err != nil {
		t.Errorf("New() of a key ARN without a region: error = %v, want the region of the ARN", err)
	}
}

func TestPreviousKeyOnlyUnwraps(t *testing.T) {
	t.Parallel()
	_, cfg := awskmstest.Start(t, keyA, keyB)
	old, err := awskms.New(t.Context(), cfg, awskms.Options{Key: keyA})
	if err != nil {
		t.Fatal(err)
	}
	id, wrapped, err := old.WrapKey(t.Context(), dek, aad)
	if err != nil {
		t.Fatal(err)
	}

	km, err := awskms.New(t.Context(), cfg, awskms.Options{Key: keyB, PreviousKeys: []string{keyA}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := km.UnwrapKey(t.Context(), id, wrapped, aad); err != nil || !bytes.Equal(got, dek) {
		t.Errorf("UnwrapKey() under the previous key = %x, %v, want the DEK", got, err)
	}
	if newID, _, err := km.WrapKey(t.Context(), dek, aad); err != nil || newID != "awskms:"+keyB {
		t.Errorf("WrapKey() = %q, %v, want the current key %q", newID, err, "awskms:"+keyB)
	}
	// A ciphertext of one key does not open under another (IncorrectKeyException).
	if _, err := km.UnwrapKey(t.Context(), "awskms:"+keyB, wrapped, aad); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("UnwrapKey() under another key: error = %v, want ErrDecrypt", err)
	}
	if _, err := km.UnwrapKey(t.Context(), "local:0011223344556677", wrapped, aad); !errors.Is(err, kernel.ErrUnknownKey) {
		t.Errorf("UnwrapKey() with a local key ID: error = %v, want ErrUnknownKey", err)
	}
	if _, err := km.UnwrapKey(t.Context(), id, wrapped, nil); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("UnwrapKey() without the AAD: error = %v, want ErrDecrypt", err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		setup func(*awskmstest.Server)
		code  string
		hint  string
	}{
		{"access denied", func(s *awskmstest.Server) { s.Deny(keyA) }, "AccessDeniedException", "kms:Decrypt"},
		{"disabled", func(s *awskmstest.Server) { s.Disable(keyA) }, "DisabledException", "disabled"},
		{"throttled", func(s *awskmstest.Server) {
			for range 3 {
				s.FailNext("ThrottlingException")
			}
		}, "ThrottlingException", ""},
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
			var api smithy.APIError
			if !errors.As(err, &api) || api.ErrorCode() != c.code || errors.Is(err, kernel.ErrDecrypt) || errors.Is(err, kernel.ErrUnknownKey) {
				t.Errorf("UnwrapKey() error = %v, want %s and neither ErrDecrypt nor ErrUnknownKey", err, c.code)
			}
			if !strings.Contains(fmt.Sprint(err), c.hint) {
				t.Errorf("UnwrapKey() error = %v, want the hint %q", err, c.hint)
			}
		})
	}

	t.Run("transient failure is retried", func(t *testing.T) {
		t.Parallel()
		km, srv := newKeyManager(t, keyA)
		id, wrapped, err := km.WrapKey(t.Context(), dek, aad)
		if err != nil {
			t.Fatal(err)
		}
		srv.FailNext("ThrottlingException")
		srv.FailNext("KMSInternalException")
		if got, err := km.UnwrapKey(t.Context(), id, wrapped, aad); err != nil || !bytes.Equal(got, dek) {
			t.Errorf("UnwrapKey() after a throttling and an internal error = %x, %v, want the DEK on the retry", got, err)
		}
	})
}

func TestNeverPrintsKeys(t *testing.T) {
	t.Parallel()
	km, _ := newKeyManager(t, keyA, keyB)
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("km", "km", km)
	for _, s := range []string{fmt.Sprint(km), buf.String()} {
		if !strings.Contains(s, "awskms:"+keyA) || strings.Contains(s, keyB) {
			t.Errorf("printed %q, want only the current key ID", s)
		}
	}
}
