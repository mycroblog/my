// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
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
		CreatedAt: sqlite.ToMillis(a.CreatedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert TLS account")
}

// FindTLSAccount implements appinstance.TLSRepository.
func (r *Repository) FindTLSAccount(ctx context.Context, directoryURL string) (instance.TLSAccount, error) {
	row, err := r.db.reader().GetTLSAccount(ctx, directoryURL)
	if err != nil {
		return instance.TLSAccount{}, sqlite.NotFound(err, appinstance.ErrNotFound, "TLS account of "+directoryURL)
	}
	return instance.TLSAccount{
		ID: row.ID, DirectoryURL: row.DirectoryUrl, AccountURL: row.AccountUrl, SealedPrivateKey: row.SealedPrivateKey,
		CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}, nil
}

// InsertTLSOrderAttempt implements appinstance.TLSRepository.
func (r *Repository) InsertTLSOrderAttempt(ctx context.Context, tx uow.Tx, a instance.TLSOrderAttempt) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertTLSOrderAttempt(ctx, instancegen.InsertTLSOrderAttemptParams{
		ID: a.ID, Host: a.Host, DirectoryUrl: a.DirectoryURL, Profile: a.Profile,
		ReplacesCertificateID: sqlite.ToNullString(a.ReplacesCertificateID), OccurredAt: sqlite.ToMillis(a.OccurredAt),
	}), "insert TLS order attempt")
}

// InsertTLSOrderFailure implements appinstance.TLSRepository.
func (r *Repository) InsertTLSOrderFailure(ctx context.Context, tx uow.Tx, f instance.TLSOrderFailure) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertTLSOrderFailure(ctx, instancegen.InsertTLSOrderFailureParams{
		AttemptID: f.AttemptID, ProblemType: f.ProblemType, Detail: f.Detail, OccurredAt: sqlite.ToMillis(f.OccurredAt),
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
		NotBefore: sqlite.ToMillis(c.NotBefore), NotAfter: sqlite.ToMillis(c.NotAfter), ChainPem: c.ChainPEM,
		SealedPrivateKey: c.SealedPrivateKey, CreatedAt: sqlite.ToMillis(c.CreatedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert TLS certificate")
}

// InsertTLSCertificateRevocation implements appinstance.TLSRepository.
func (r *Repository) InsertTLSCertificateRevocation(ctx context.Context, tx uow.Tx, v instance.TLSCertificateRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertTLSCertificateRevocation(ctx, instancegen.InsertTLSCertificateRevocationParams{
		ID: v.ID, CertificateID: v.CertificateID, Reason: string(v.Reason),
		RevokedByUserID: sqlite.ToNullString(v.ByUserID), OccurredAt: sqlite.ToMillis(v.OccurredAt),
	}), "insert TLS certificate revocation")
}

// CurrentTLSCertificate implements appinstance.TLSRepository.
func (r *Repository) CurrentTLSCertificate(ctx context.Context, host string) (instance.TLSCertificate, error) {
	row, err := r.db.reader().GetCurrentTLSCertificate(ctx, host)
	if err != nil {
		return instance.TLSCertificate{}, sqlite.NotFound(err, appinstance.ErrNotFound, "TLS certificate of "+host)
	}
	return instance.TLSCertificate{
		ID: row.ID, AttemptID: row.AttemptID, Host: row.Host, DirectoryURL: row.DirectoryUrl, Serial: row.Serial,
		NotBefore: sqlite.FromMillis(row.NotBefore), NotAfter: sqlite.FromMillis(row.NotAfter), ChainPEM: row.ChainPem,
		SealedPrivateKey: row.SealedPrivateKey, CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}, nil
}

// CurrentTLSCertificateID implements appinstance.TLSRepository.
func (r *Repository) CurrentTLSCertificateID(ctx context.Context, host string) (string, error) {
	id, err := r.db.reader().GetCurrentTLSCertificateID(ctx, host)
	if err != nil {
		return "", sqlite.NotFound(err, appinstance.ErrNotFound, "TLS certificate of "+host)
	}
	return id, nil
}

// ListUnresultedTLSOrderAttempts implements appinstance.TLSRepository.
func (r *Repository) ListUnresultedTLSOrderAttempts(ctx context.Context, host string, after time.Time) ([]instance.TLSOrderAttempt, error) {
	rows, err := r.db.reader().ListUnresultedTLSOrderAttempts(ctx, instancegen.ListUnresultedTLSOrderAttemptsParams{
		Host: host, After: sqlite.ToMillis(after),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list TLS order attempts")
	}
	out := make([]instance.TLSOrderAttempt, len(rows))
	for i, row := range rows {
		out[i] = instance.TLSOrderAttempt{
			ID: row.ID, Host: row.Host, DirectoryURL: row.DirectoryUrl, Profile: row.Profile,
			ReplacesCertificateID: row.ReplacesCertificateID.String, OccurredAt: sqlite.FromMillis(row.OccurredAt),
		}
	}
	return out, nil
}
