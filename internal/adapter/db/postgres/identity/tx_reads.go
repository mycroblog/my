// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// FindRegistrationApplicationTx implements the reads in a transaction.
func (r *Repository) FindRegistrationApplicationTx(ctx context.Context, tx uow.Tx, userID string) (identity.RegistrationApplication, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.RegistrationApplication{}, err
	}
	return (&Repository{db: db}).FindRegistrationApplication(ctx, userID)
}

// FindRegistrationStateTx implements the reads in a transaction.
func (r *Repository) FindRegistrationStateTx(ctx context.Context, tx uow.Tx, userID string) (identity.UserRegistrationState, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.UserRegistrationState{}, err
	}
	return (&Repository{db: db}).FindRegistrationState(ctx, userID)
}

// FindPrimaryEmailTx implements the reads in a transaction.
func (r *Repository) FindPrimaryEmailTx(ctx context.Context, tx uow.Tx, userID string) (identity.EmailAddress, bool, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.EmailAddress{}, false, err
	}
	return (&Repository{db: db}).FindPrimaryEmail(ctx, userID)
}

// FindCurrentPasswordCredentialTx implements the reads in a transaction.
func (r *Repository) FindCurrentPasswordCredentialTx(ctx context.Context, tx uow.Tx, userID string) (identity.PasswordCredential, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return identity.PasswordCredential{}, err
	}
	return (&Repository{db: db}).FindCurrentPasswordCredential(ctx, userID)
}

// ListActivePasskeyCredentialsTx implements the reads in a transaction.
func (r *Repository) ListActivePasskeyCredentialsTx(ctx context.Context, tx uow.Tx, userID string) ([]identity.PasskeyCredential, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListActivePasskeyCredentials(ctx, userID)
}

// ListLinkedExternalIdentitiesTx implements the reads in a transaction.
func (r *Repository) ListLinkedExternalIdentitiesTx(ctx context.Context, tx uow.Tx, userID string) ([]identity.LinkedExternalIdentity, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListLinkedExternalIdentities(ctx, userID)
}

// ListLoginProvidersTx implements the reads in a transaction.
func (r *Repository) ListLoginProvidersTx(ctx context.Context, tx uow.Tx) ([]identity.LoginProvider, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListLoginProviders(ctx)
}

// ListUserIDsInRegistrationStateTx implements the reads in a transaction.
func (r *Repository) ListUserIDsInRegistrationStateTx(ctx context.Context, tx uow.Tx, state identity.RegistrationState, before string, limit int) ([]string, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&Repository{db: db}).ListUserIDsInRegistrationState(ctx, state, before, limit)
}
