// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"errors"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceSecretCases are the cases of InstanceRepositoryContract about the secrets of the
// instance (ADR-0121 §2): a secret is generated once and kept, one row per name, sealed and bound
// to its row.
var instanceSecretCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"GeneratesTheMediaProxyKeyOnce", instanceGeneratesSecretOnce},
	{"KeepsOneSecretPerName", instanceKeepsOneSecretPerName},
}

func instanceGeneratesSecretOnce(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	if _, err := e.h.Instance.FindSecret(ctx, instance.SecretMediaProxyKey); !errors.Is(err, appinstance.ErrNotFound) {
		t.Fatalf("FindSecret of a new instance = %v, want ErrNotFound", err)
	}
	first, err := e.service.MediaProxyKey(ctx)
	if err != nil {
		t.Fatalf("MediaProxyKey = %v", err)
	}
	if len(first) != 32 || bytes.Equal(first, make([]byte, 32)) {
		t.Errorf("MediaProxyKey = %x, want 32 random bytes", first)
	}
	e.clock.Advance(time.Second)
	again, err := e.service.MediaProxyKey(ctx)
	if err != nil || !bytes.Equal(again, first) {
		t.Errorf("MediaProxyKey again = %x, %v; want %x", again, err, first)
	}
	stored, err := e.h.Instance.FindSecret(ctx, instance.SecretMediaProxyKey)
	if err != nil {
		t.Fatalf("FindSecret = %v", err)
	}
	if stored.Name != instance.SecretMediaProxyKey || stored.ID == "" || bytes.Contains(stored.Sealed, first) {
		t.Errorf("stored secret = %+v, want it named, with an ID, and sealed", stored)
	}
	// The sealed value is bound to its row.
	if _, err := e.sealer.Open(ctx, stored.Sealed, appinstance.SecretAAD("another-id")); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("Open with another row's AAD = %v, want ErrDecrypt", err)
	}
}

func instanceKeepsOneSecretPerName(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	key, err := e.service.MediaProxyKey(ctx)
	if err != nil {
		t.Fatalf("MediaProxyKey = %v", err)
	}
	// Another process that generated a secret of the same name at the same time loses.
	other := instance.Secret{ID: "0190f5a4-8c4e-7000-8000-0000000000aa", Name: instance.SecretMediaProxyKey, Sealed: []byte("x"), CreatedAt: e.clock.Now()}
	var inserted bool
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		inserted, err = e.h.Instance.InsertSecret(ctx, tx, other)
		return err
	}); err != nil {
		t.Fatalf("InsertSecret of a second secret of the name = %v", err)
	}
	if inserted {
		t.Error("InsertSecret inserted a second secret of the name")
	}
	if got, err := e.service.MediaProxyKey(ctx); err != nil || !bytes.Equal(got, key) {
		t.Errorf("MediaProxyKey after the second insert = %x, %v; want %x", got, err, key)
	}
}
