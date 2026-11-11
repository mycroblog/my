// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"math"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertTLSAccount implements appinstance.TLSRepository.
func (r *Repository) InsertTLSAccount(ctx context.Context, tx uow.Tx, a instance.TLSAccount) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertTLSAccount(ctx, instancegen.InsertTLSAccountParams{
		ID: a.ID, DirectoryUrl: a.DirectoryURL, AccountUrl: a.AccountURL, SealedPrivateKey: a.SealedPrivateKey,
		CreatedAt: a.CreatedAt,
	})
	return n > 0, postgres.Wrap(err, "insert TLS account")
}

// FindTLSAccount implements appinstance.TLSRepository.
func (r *Repository) FindTLSAccount(ctx context.Context, directoryURL string) (instance.TLSAccount, error) {
	row, err := r.db.reader().GetTLSAccount(ctx, directoryURL)
	if err != nil {
		return instance.TLSAccount{}, postgres.NotFound(err, appinstance.ErrNotFound, "TLS account of "+directoryURL)
	}
	return instance.TLSAccount{
		ID: row.ID, DirectoryURL: row.DirectoryUrl, AccountURL: row.AccountUrl, SealedPrivateKey: row.SealedPrivateKey,
		CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

// InsertTLSOrderAttempt implements appinstance.TLSRepository.
func (r *Repository) InsertTLSOrderAttempt(ctx context.Context, tx uow.Tx, a instance.TLSOrderAttempt) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertTLSOrderAttempt(ctx, instancegen.InsertTLSOrderAttemptParams{
		ID: a.ID, Host: a.Host, DirectoryUrl: a.DirectoryURL, Profile: a.Profile,
		ReplacesCertificateID: postgres.ToNullString(a.ReplacesCertificateID), OccurredAt: a.OccurredAt,
	}), "insert TLS order attempt")
}

// InsertTLSOrderFailure implements appinstance.TLSRepository.
func (r *Repository) InsertTLSOrderFailure(ctx context.Context, tx uow.Tx, f instance.TLSOrderFailure) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertTLSOrderFailure(ctx, instancegen.InsertTLSOrderFailureParams{
		AttemptID: f.AttemptID, ProblemType: f.ProblemType, Detail: f.Detail, OccurredAt: f.OccurredAt,
	}), "insert TLS order failure")
}

// InsertTLSCertificate implements appinstance.TLSRepository.
func (r *Repository) InsertTLSCertificate(ctx context.Context, tx uow.Tx, c instance.TLSCertificate) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertTLSCertificate(ctx, instancegen.InsertTLSCertificateParams{
		ID: c.ID, AttemptID: c.AttemptID, Host: c.Host, DirectoryUrl: c.DirectoryURL, Serial: c.Serial,
		NotBefore: c.NotBefore, NotAfter: c.NotAfter, ChainPem: c.ChainPEM,
		SealedPrivateKey: c.SealedPrivateKey, CreatedAt: c.CreatedAt,
	})
	return n > 0, postgres.Wrap(err, "insert TLS certificate")
}

// InsertTLSCertificateRevocation implements appinstance.TLSRepository.
func (r *Repository) InsertTLSCertificateRevocation(ctx context.Context, tx uow.Tx, v instance.TLSCertificateRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertTLSCertificateRevocation(ctx, instancegen.InsertTLSCertificateRevocationParams{
		ID: v.ID, CertificateID: v.CertificateID, Reason: string(v.Reason),
		RevokedByUserID: postgres.ToNullString(v.ByUserID), OccurredAt: v.OccurredAt,
	}), "insert TLS certificate revocation")
}

// CurrentTLSCertificate implements appinstance.TLSRepository.
func (r *Repository) CurrentTLSCertificate(ctx context.Context, host string) (instance.TLSCertificate, error) {
	row, err := r.db.reader().GetCurrentTLSCertificate(ctx, host)
	if err != nil {
		return instance.TLSCertificate{}, postgres.NotFound(err, appinstance.ErrNotFound, "TLS certificate of "+host)
	}
	return instance.TLSCertificate{
		ID: row.ID, AttemptID: row.AttemptID, Host: row.Host, DirectoryURL: row.DirectoryUrl, Serial: row.Serial,
		NotBefore: row.NotBefore.UTC(), NotAfter: row.NotAfter.UTC(), ChainPEM: row.ChainPem,
		SealedPrivateKey: row.SealedPrivateKey, CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

// CurrentTLSCertificateID implements appinstance.TLSRepository.
func (r *Repository) CurrentTLSCertificateID(ctx context.Context, host string) (string, error) {
	id, err := r.db.reader().GetCurrentTLSCertificateID(ctx, host)
	if err != nil {
		return "", postgres.NotFound(err, appinstance.ErrNotFound, "TLS certificate of "+host)
	}
	return id, nil
}

// ListUnresultedTLSOrderAttempts implements appinstance.TLSRepository.
func (r *Repository) ListUnresultedTLSOrderAttempts(ctx context.Context, host string, after time.Time) ([]instance.TLSOrderAttempt, error) {
	rows, err := r.db.reader().ListUnresultedTLSOrderAttempts(ctx, instancegen.ListUnresultedTLSOrderAttemptsParams{
		Host: host, After: after,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list TLS order attempts")
	}
	out := make([]instance.TLSOrderAttempt, len(rows))
	for i, row := range rows {
		out[i] = instance.TLSOrderAttempt{
			ID: row.ID, Host: row.Host, DirectoryURL: row.DirectoryUrl, Profile: row.Profile,
			ReplacesCertificateID: postgres.FromNullString(row.ReplacesCertificateID), OccurredAt: row.OccurredAt.UTC(),
		}
	}
	return out, nil
}

// ListTLSOrderAttempts implements appinstance.TLSRepository.
func (r *Repository) ListTLSOrderAttempts(ctx context.Context, host string, limit int) ([]instance.TLSOrderAttemptResult, error) {
	rows, err := r.db.reader().ListTLSOrderAttempts(ctx, instancegen.ListTLSOrderAttemptsParams{Host: host, MaxRows: int32(min(limit, math.MaxInt32))}) //nolint:gosec // G115: clamped
	if err != nil {
		return nil, postgres.Wrap(err, "list TLS order attempts")
	}
	out := make([]instance.TLSOrderAttemptResult, len(rows))
	for i, row := range rows {
		out[i] = instance.TLSOrderAttemptResult{
			Attempt: instance.TLSOrderAttempt{
				ID: row.ID, Host: row.Host, DirectoryURL: row.DirectoryUrl, Profile: row.Profile,
				ReplacesCertificateID: postgres.FromNullString(row.ReplacesCertificateID), OccurredAt: row.OccurredAt.UTC(),
			},
			CertificateID: postgres.FromNullString(row.CertificateID),
		}
		if row.ProblemType != nil {
			f := &instance.TLSOrderFailure{AttemptID: row.ID, ProblemType: *row.ProblemType, Detail: postgres.FromNullString(row.Detail)}
			if row.FailedAt != nil {
				f.OccurredAt = row.FailedAt.UTC()
			}
			out[i].Failure = f
		}
	}
	return out, nil
}

// TLSCertificateSuperseded implements appinstance.TLSRepository.
func (r *Repository) TLSCertificateSuperseded(ctx context.Context, certificateID string) (bool, error) {
	ok, err := r.db.reader().TLSCertificateSuperseded(ctx, certificateID)
	return ok, postgres.Wrap(err, "read the TLS certificates")
}
