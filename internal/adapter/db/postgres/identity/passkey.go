// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertPasskeyCredential implements identity.PasskeyRepository.
func (r *Repository) InsertPasskeyCredential(ctx context.Context, tx uow.Tx, c identity.PasskeyCredential) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasskeyCredential(ctx, identitygen.InsertPasskeyCredentialParams{
		ID: c.ID, UserID: c.UserID, CredentialID: c.CredentialID, PublicKey: c.PublicKey,
		BackupEligible: c.BackupEligible, Name: c.Name, CreatedAt: c.CreatedAt,
	})
	return postgres.Wrap(err, "insert passkey credential")
}

// ListActivePasskeyCredentials implements identity.PasskeyRepository.
func (r *Repository) ListActivePasskeyCredentials(ctx context.Context, userID string) ([]identity.PasskeyCredential, error) {
	rows, err := r.db.reader().ListActivePasskeyCredentials(ctx, userID)
	if err != nil {
		return nil, postgres.Wrap(err, "list passkey credentials")
	}
	out := make([]identity.PasskeyCredential, len(rows))
	for i, row := range rows {
		out[i] = identity.PasskeyCredential{
			ID: row.ID, UserID: row.UserID, CredentialID: row.CredentialID, PublicKey: row.PublicKey,
			BackupEligible: row.BackupEligible, Name: row.Name, CreatedAt: postgres.UTC(row.CreatedAt),
		}
	}
	return out, nil
}

// FindPasskeyCredential implements identity.PasskeyRepository.
func (r *Repository) FindPasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, error) {
	c, _, err := r.getPasskeyCredential(ctx, credentialID)
	return c, err
}

// FindActivePasskeyCredential implements identity.PasskeyRepository.
func (r *Repository) FindActivePasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, error) {
	c, revoked, err := r.getPasskeyCredential(ctx, credentialID)
	if err == nil && revoked {
		return identity.PasskeyCredential{}, appidentity.ErrNotFound
	}
	return c, err
}

func (r *Repository) getPasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, bool, error) {
	row, err := r.db.reader().GetPasskeyCredential(ctx, credentialID)
	if err != nil {
		return identity.PasskeyCredential{}, false, postgres.NotFound(err, appidentity.ErrNotFound, "passkey credential")
	}
	return identity.PasskeyCredential{
		ID: row.ID, UserID: row.UserID, CredentialID: row.CredentialID, PublicKey: row.PublicKey,
		BackupEligible: row.BackupEligible, Name: row.Name, CreatedAt: postgres.UTC(row.CreatedAt),
	}, row.Revoked, nil
}

// InsertPasskeyCredentialRevocation implements identity.PasskeyRepository.
func (r *Repository) InsertPasskeyCredentialRevocation(ctx context.Context, tx uow.Tx, rev identity.PasskeyCredentialRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasskeyCredentialRevocation(ctx, identitygen.InsertPasskeyCredentialRevocationParams{
		PasskeyID: rev.PasskeyID, RevokedAt: rev.RevokedAt,
	})
	return postgres.Wrap(err, "insert passkey credential revocation")
}
