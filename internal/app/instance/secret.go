// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// SecretRepository is the persistence port of the secrets of the instance (instance_secrets).
type SecretRepository interface {
	// InsertSecret inserts s unless a secret of the same name exists, and reports whether it did.
	InsertSecret(ctx context.Context, tx uow.Tx, s instance.Secret) (bool, error)
	// FindSecret returns the secret of name, ErrNotFound when there is none.
	FindSecret(ctx context.Context, name instance.SecretName) (instance.Secret, error)
}

// SecretAAD returns the additional authenticated data that binds the sealed value of the secret
// secretID to its row.
func SecretAAD(secretID string) []byte {
	return kernel.AAD("instance_secrets", secretID, "sealed_secret")
}

// mediaProxyKeySize is the length of the media proxy's HMAC-SHA256 key.
const mediaProxyKeySize = 32

// MediaProxyKey returns the key that signs the media proxy URLs. The first call on a database
// generates a random key and stores it sealed; every later call, on any process, returns that
// key, whatever key-encryption key seals it (ADR-0121 §2).
func (s *Service) MediaProxyKey(ctx context.Context) ([]byte, error) {
	return s.secret(ctx, instance.SecretMediaProxyKey, mediaProxyKeySize)
}

// secret returns the secret of name, generating size random bytes for it first when the database
// has none. Concurrent first calls agree on the one that was inserted first.
func (s *Service) secret(ctx context.Context, name instance.SecretName, size int) ([]byte, error) {
	if s.d.Sealer == nil {
		return nil, errors.New("instance: secrets need a Sealer")
	}
	stored, err := s.d.Repository.FindSecret(ctx, name)
	if errors.Is(err, ErrNotFound) {
		if err := s.insertSecret(ctx, name, size); err != nil {
			return nil, err
		}
		stored, err = s.d.Repository.FindSecret(ctx, name)
	}
	if err != nil {
		return nil, fmt.Errorf("instance: read secret %s: %w", name, err)
	}
	plain, err := s.d.Sealer.Open(ctx, stored.Sealed, SecretAAD(stored.ID))
	if err != nil {
		return nil, fmt.Errorf("instance: open secret %s: %w", name, err)
	}
	return plain, nil
}

// insertSecret generates and stores a secret of name unless one exists. It seals the secret
// before the transaction, so that a KeyManager's call to a KMS stays out of it.
func (s *Service) insertSecret(ctx context.Context, name instance.SecretName, size int) error {
	plain := make([]byte, size)
	_, _ = rand.Read(plain) // crypto/rand.Read never returns an error
	id := s.d.IDs.NewID()
	sealed, err := s.d.Sealer.Seal(ctx, plain, SecretAAD(id))
	if err != nil {
		return fmt.Errorf("instance: seal secret %s: %w", name, err)
	}
	sec := instance.Secret{ID: id, Name: name, Sealed: sealed, CreatedAt: s.d.Clock.Now()}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Repository.InsertSecret(ctx, tx, sec)
		return err
	})
	if err != nil {
		return fmt.Errorf("instance: store secret %s: %w", name, err)
	}
	return nil
}
