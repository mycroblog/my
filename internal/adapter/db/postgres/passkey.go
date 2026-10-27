// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertPasskeyCredential implements identity.PasskeyRepository.
func (r *IdentityRepository) InsertPasskeyCredential(ctx context.Context, tx uow.Tx, c identity.PasskeyCredential) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasskeyCredential(ctx, postgresgen.InsertPasskeyCredentialParams{
		ID: c.ID, UserID: c.UserID, CredentialID: c.CredentialID, PublicKey: c.PublicKey,
		BackupEligible: c.BackupEligible, Name: c.Name, CreatedAt: c.CreatedAt,
	})
	return wrap(err, "insert passkey credential")
}

// ListActivePasskeyCredentials implements identity.PasskeyRepository.
func (r *IdentityRepository) ListActivePasskeyCredentials(ctx context.Context, userID string) ([]identity.PasskeyCredential, error) {
	rows, err := r.db.reader().ListActivePasskeyCredentials(ctx, userID)
	if err != nil {
		return nil, wrap(err, "list passkey credentials")
	}
	out := make([]identity.PasskeyCredential, len(rows))
	for i, row := range rows {
		out[i] = identity.PasskeyCredential{
			ID: row.ID, UserID: row.UserID, CredentialID: row.CredentialID, PublicKey: row.PublicKey,
			BackupEligible: row.BackupEligible, Name: row.Name, CreatedAt: utc(row.CreatedAt),
		}
	}
	return out, nil
}

// FindPasskeyCredential implements identity.PasskeyRepository.
func (r *IdentityRepository) FindPasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, error) {
	c, _, err := r.getPasskeyCredential(ctx, credentialID)
	return c, err
}

// FindActivePasskeyCredential implements identity.PasskeyRepository.
func (r *IdentityRepository) FindActivePasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, error) {
	c, revoked, err := r.getPasskeyCredential(ctx, credentialID)
	if err == nil && revoked {
		return identity.PasskeyCredential{}, appidentity.ErrNotFound
	}
	return c, err
}

func (r *IdentityRepository) getPasskeyCredential(ctx context.Context, credentialID []byte) (identity.PasskeyCredential, bool, error) {
	row, err := r.db.reader().GetPasskeyCredential(ctx, credentialID)
	if err != nil {
		return identity.PasskeyCredential{}, false, notFound(err, appidentity.ErrNotFound, "passkey credential")
	}
	return identity.PasskeyCredential{
		ID: row.ID, UserID: row.UserID, CredentialID: row.CredentialID, PublicKey: row.PublicKey,
		BackupEligible: row.BackupEligible, Name: row.Name, CreatedAt: utc(row.CreatedAt),
	}, row.Revoked, nil
}

// InsertPasskeyCredentialRevocation implements identity.PasskeyRepository.
func (r *IdentityRepository) InsertPasskeyCredentialRevocation(ctx context.Context, tx uow.Tx, rev identity.PasskeyCredentialRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPasskeyCredentialRevocation(ctx, postgresgen.InsertPasskeyCredentialRevocationParams{
		PasskeyID: rev.PasskeyID, RevokedAt: rev.RevokedAt,
	})
	return wrap(err, "insert passkey credential revocation")
}
