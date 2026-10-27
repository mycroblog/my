// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertTOTPEnrollment implements identity.TOTPRepository.
func (r *IdentityRepository) InsertTOTPEnrollment(ctx context.Context, tx uow.Tx, e identity.TOTPEnrollment, recoveryCodeHashes [][]byte) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertTOTPEnrollment(ctx, postgresgen.InsertTOTPEnrollmentParams{
		ID: e.ID, UserID: e.UserID, SecretEnc: e.SecretEnc, EnrolledAt: e.EnrolledAt,
	})
	if err != nil {
		return wrap(err, "insert TOTP enrollment")
	}
	for _, h := range recoveryCodeHashes {
		if err := q.InsertTOTPRecoveryCode(ctx, postgresgen.InsertTOTPRecoveryCodeParams{EnrollmentID: e.ID, CodeHash: h}); err != nil {
			return wrap(err, "insert TOTP recovery code")
		}
	}
	return nil
}

// FindActiveTOTPEnrollment implements identity.TOTPRepository.
func (r *IdentityRepository) FindActiveTOTPEnrollment(ctx context.Context, userID string) (identity.TOTPEnrollment, error) {
	row, err := r.db.reader().GetNewestTOTPEnrollment(ctx, userID)
	if err != nil {
		return identity.TOTPEnrollment{}, notFound(err, appidentity.ErrNotFound, "TOTP enrollment of "+userID)
	}
	if row.Revoked {
		return identity.TOTPEnrollment{}, appidentity.ErrNotFound
	}
	return identity.TOTPEnrollment{ID: row.ID, UserID: row.UserID, SecretEnc: row.SecretEnc, EnrolledAt: utc(row.EnrolledAt)}, nil
}

// InsertTOTPEnrollmentRevocation implements identity.TOTPRepository.
func (r *IdentityRepository) InsertTOTPEnrollmentRevocation(ctx context.Context, tx uow.Tx, rev identity.TOTPEnrollmentRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertTOTPEnrollmentRevocation(ctx, postgresgen.InsertTOTPEnrollmentRevocationParams{
		EnrollmentID: rev.EnrollmentID, RevokedAt: rev.RevokedAt,
	})
	return wrap(err, "insert TOTP enrollment revocation")
}

// UseTOTPRecoveryCode implements identity.TOTPRepository.
func (r *IdentityRepository) UseTOTPRecoveryCode(ctx context.Context, tx uow.Tx, u identity.TOTPRecoveryCodeUse) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.UseTOTPRecoveryCode(ctx, postgresgen.UseTOTPRecoveryCodeParams{
		UsedAt: u.UsedAt, EnrollmentID: u.EnrollmentID, CodeHash: u.CodeHash,
	})
	if err != nil {
		return false, wrap(err, "use TOTP recovery code")
	}
	return n == 1, nil
}

// CountUnusedTOTPRecoveryCodes implements identity.TOTPRepository.
func (r *IdentityRepository) CountUnusedTOTPRecoveryCodes(ctx context.Context, enrollmentID string) (int, error) {
	n, err := r.db.reader().CountUnusedTOTPRecoveryCodes(ctx, enrollmentID)
	return int(n), wrap(err, "count TOTP recovery codes")
}
