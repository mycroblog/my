// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertTOTPEnrollment implements identity.TOTPRepository.
func (r *Repository) InsertTOTPEnrollment(ctx context.Context, tx uow.Tx, e identity.TOTPEnrollment, recoveryCodeHashes [][]byte) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertTOTPEnrollment(ctx, identitygen.InsertTOTPEnrollmentParams{
		ID: e.ID, UserID: e.UserID, SecretEnc: e.SecretEnc, EnrolledAt: sqlite.ToMillis(e.EnrolledAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert TOTP enrollment")
	}
	for _, h := range recoveryCodeHashes {
		if err := q.InsertTOTPRecoveryCode(ctx, identitygen.InsertTOTPRecoveryCodeParams{EnrollmentID: e.ID, CodeHash: h}); err != nil {
			return sqlite.Wrap(err, "insert TOTP recovery code")
		}
	}
	return nil
}

// FindActiveTOTPEnrollment implements identity.TOTPRepository.
func (r *Repository) FindActiveTOTPEnrollment(ctx context.Context, userID string) (identity.TOTPEnrollment, error) {
	row, err := r.db.reader().GetNewestTOTPEnrollment(ctx, userID)
	if err != nil {
		return identity.TOTPEnrollment{}, sqlite.NotFound(err, appidentity.ErrNotFound, "TOTP enrollment of "+userID)
	}
	if row.Revoked {
		return identity.TOTPEnrollment{}, appidentity.ErrNotFound
	}
	return identity.TOTPEnrollment{ID: row.ID, UserID: row.UserID, SecretEnc: row.SecretEnc, EnrolledAt: sqlite.FromMillis(row.EnrolledAt)}, nil
}

// InsertTOTPEnrollmentRevocation implements identity.TOTPRepository.
func (r *Repository) InsertTOTPEnrollmentRevocation(ctx context.Context, tx uow.Tx, rev identity.TOTPEnrollmentRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertTOTPEnrollmentRevocation(ctx, identitygen.InsertTOTPEnrollmentRevocationParams{
		EnrollmentID: rev.EnrollmentID, RevokedAt: sqlite.ToMillis(rev.RevokedAt),
	})
	return sqlite.Wrap(err, "insert TOTP enrollment revocation")
}

// UseTOTPRecoveryCode implements identity.TOTPRepository.
func (r *Repository) UseTOTPRecoveryCode(ctx context.Context, tx uow.Tx, u identity.TOTPRecoveryCodeUse) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.UseTOTPRecoveryCode(ctx, identitygen.UseTOTPRecoveryCodeParams{
		UsedAt: sqlite.ToMillis(u.UsedAt), EnrollmentID: u.EnrollmentID, CodeHash: u.CodeHash,
	})
	if err != nil {
		return false, sqlite.Wrap(err, "use TOTP recovery code")
	}
	return n == 1, nil
}

// CountUnusedTOTPRecoveryCodes implements identity.TOTPRepository.
func (r *Repository) CountUnusedTOTPRecoveryCodes(ctx context.Context, enrollmentID string) (int, error) {
	n, err := r.db.reader().CountUnusedTOTPRecoveryCodes(ctx, enrollmentID)
	return int(n), sqlite.Wrap(err, "count TOTP recovery codes")
}
