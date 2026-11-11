// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// TLSRepository is the persistence port of the TLS certificates that the server serves itself:
// the ACME accounts, the orders with their failures, the issued certificates, and their
// revocations (ADR-0163 §4). Every insert is safe to repeat.
type TLSRepository interface {
	// InsertTLSAccount inserts a unless an account of the same directory exists, and reports
	// whether it did.
	InsertTLSAccount(ctx context.Context, tx uow.Tx, a instance.TLSAccount) (bool, error)
	// FindTLSAccount returns the account of directoryURL, ErrNotFound when there is none.
	FindTLSAccount(ctx context.Context, directoryURL string) (instance.TLSAccount, error)
	InsertTLSOrderAttempt(ctx context.Context, tx uow.Tx, a instance.TLSOrderAttempt) error
	// InsertTLSOrderFailure records f unless its attempt has a failure.
	InsertTLSOrderFailure(ctx context.Context, tx uow.Tx, f instance.TLSOrderFailure) error
	// InsertTLSCertificate inserts c unless its attempt has a certificate, and reports whether it
	// did.
	InsertTLSCertificate(ctx context.Context, tx uow.Tx, c instance.TLSCertificate) (bool, error)
	InsertTLSCertificateRevocation(ctx context.Context, tx uow.Tx, r instance.TLSCertificateRevocation) error
	// CurrentTLSCertificate returns the certificate of host issued last, by created_at, that is
	// not revoked; ErrNotFound when there is none.
	CurrentTLSCertificate(ctx context.Context, host string) (instance.TLSCertificate, error)
	// CurrentTLSCertificateID is CurrentTLSCertificate reading only the ID.
	CurrentTLSCertificateID(ctx context.Context, host string) (string, error)
	// ListUnresultedTLSOrderAttempts returns the attempts of host that occurred after after and
	// have no certificate, oldest first.
	ListUnresultedTLSOrderAttempts(ctx context.Context, host string, after time.Time) ([]instance.TLSOrderAttempt, error)
}

// TLSAccountAAD returns the additional authenticated data that binds the sealed key of the ACME
// account accountID to its row.
func TLSAccountAAD(accountID string) []byte {
	return kernel.AAD("tls_acme_accounts", accountID, "sealed_private_key")
}

// TLSCertificateAAD returns the additional authenticated data that binds the sealed key of the
// certificate certificateID to its row.
func TLSCertificateAAD(certificateID string) []byte {
	return kernel.AAD("tls_certificates", certificateID, "sealed_private_key")
}

// TLSKeyPair is a stored account or certificate with its private key opened (PKCS #8 DER).
type TLSKeyPair[T any] struct {
	Record     T
	PrivateKey []byte
}

// RegisterTLSAccount stores the ACME account of directoryURL with its key (PKCS #8 DER), unless
// one exists, and returns the stored account: when two processes register at once, both get the
// account inserted first.
func (s *Service) RegisterTLSAccount(ctx context.Context, directoryURL, accountURL string, privateKey []byte) (TLSKeyPair[instance.TLSAccount], error) {
	if s.d.Sealer == nil {
		return TLSKeyPair[instance.TLSAccount]{}, errors.New("instance: TLS accounts need a Sealer")
	}
	id := s.d.IDs.NewID()
	sealed, err := s.d.Sealer.Seal(ctx, privateKey, TLSAccountAAD(id))
	if err != nil {
		return TLSKeyPair[instance.TLSAccount]{}, fmt.Errorf("instance: seal the TLS account key: %w", err)
	}
	a := instance.TLSAccount{
		ID: id, DirectoryURL: directoryURL, AccountURL: accountURL, SealedPrivateKey: sealed, CreatedAt: s.d.Clock.Now(),
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Repository.InsertTLSAccount(ctx, tx, a)
		return err
	}); err != nil {
		return TLSKeyPair[instance.TLSAccount]{}, fmt.Errorf("instance: store the TLS account: %w", err)
	}
	return s.TLSAccount(ctx, directoryURL)
}

// TLSAccount returns the ACME account of directoryURL with its key opened, ErrNotFound when
// none is registered.
func (s *Service) TLSAccount(ctx context.Context, directoryURL string) (TLSKeyPair[instance.TLSAccount], error) {
	if s.d.Sealer == nil {
		return TLSKeyPair[instance.TLSAccount]{}, errors.New("instance: TLS accounts need a Sealer")
	}
	a, err := s.d.Repository.FindTLSAccount(ctx, directoryURL)
	if err != nil {
		return TLSKeyPair[instance.TLSAccount]{}, err
	}
	key, err := s.d.Sealer.Open(ctx, a.SealedPrivateKey, TLSAccountAAD(a.ID))
	if err != nil {
		return TLSKeyPair[instance.TLSAccount]{}, fmt.Errorf("instance: open the TLS account key: %w", err)
	}
	return TLSKeyPair[instance.TLSAccount]{Record: a, PrivateKey: key}, nil
}

// TLSOrder describes an order to record with RecordTLSOrderAttempt.
type TLSOrder struct {
	Host         string
	DirectoryURL string
	// Profile is empty for the CA's default.
	Profile string
	// ReplacesCertificateID is empty for a first order.
	ReplacesCertificateID string
}

// RecordTLSOrderAttempt records an order just before it is sent and returns it; its ID is what
// the result of the order refers to.
func (s *Service) RecordTLSOrderAttempt(ctx context.Context, o TLSOrder) (instance.TLSOrderAttempt, error) {
	a := instance.TLSOrderAttempt{
		ID: s.d.IDs.NewID(), Host: o.Host, DirectoryURL: o.DirectoryURL, Profile: o.Profile,
		ReplacesCertificateID: o.ReplacesCertificateID, OccurredAt: s.d.Clock.Now(),
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertTLSOrderAttempt(ctx, tx, a) })
	if err != nil {
		return instance.TLSOrderAttempt{}, fmt.Errorf("instance: record the TLS order of %s: %w", o.Host, err)
	}
	return a, nil
}

// RecordTLSOrderFailure records the failure of the order attemptID. A detail longer than
// instance.MaxTLSOrderFailureDetail characters is cut.
func (s *Service) RecordTLSOrderFailure(ctx context.Context, attemptID, problemType, detail string) error {
	f := instance.TLSOrderFailure{
		AttemptID: attemptID, ProblemType: problemType,
		Detail: instance.TruncateTLSOrderFailureDetail(detail), OccurredAt: s.d.Clock.Now(),
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertTLSOrderFailure(ctx, tx, f) })
	if err != nil {
		return fmt.Errorf("instance: record the failure of the TLS order %s: %w", attemptID, err)
	}
	return nil
}

// RecordTLSCertificate stores the certificate that the order attempt issued: chainPEM is the
// leaf first and then its intermediates, privateKey the leaf's key (PKCS #8 DER), which is
// stored sealed. The serial and the validity are read from the leaf.
func (s *Service) RecordTLSCertificate(ctx context.Context, attempt instance.TLSOrderAttempt, chainPEM string, privateKey []byte) (instance.TLSCertificate, error) {
	if s.d.Sealer == nil {
		return instance.TLSCertificate{}, errors.New("instance: TLS certificates need a Sealer")
	}
	leaf, err := parseLeaf(chainPEM)
	if err != nil {
		return instance.TLSCertificate{}, err
	}
	id := s.d.IDs.NewID()
	sealed, err := s.d.Sealer.Seal(ctx, privateKey, TLSCertificateAAD(id))
	if err != nil {
		return instance.TLSCertificate{}, fmt.Errorf("instance: seal the TLS certificate key: %w", err)
	}
	c := instance.TLSCertificate{
		ID: id, AttemptID: attempt.ID, Host: attempt.Host, DirectoryURL: attempt.DirectoryURL,
		Serial: leaf.SerialNumber.Text(16), NotBefore: leaf.NotBefore.UTC(), NotAfter: leaf.NotAfter.UTC(),
		ChainPEM: chainPEM, SealedPrivateKey: sealed, CreatedAt: s.d.Clock.Now(),
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Repository.InsertTLSCertificate(ctx, tx, c)
		return err
	}); err != nil {
		return instance.TLSCertificate{}, fmt.Errorf("instance: store the TLS certificate of %s: %w", attempt.Host, err)
	}
	return c, nil
}

// parseLeaf returns the first certificate of chainPEM.
func parseLeaf(chainPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(chainPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("instance: the TLS chain does not start with a PEM certificate")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("instance: parse the TLS certificate: %w", err)
	}
	return leaf, nil
}

// RecordTLSCertificateRevocation records that the CA revoked certificateID; byUserID is empty for
// the CLI.
func (s *Service) RecordTLSCertificateRevocation(ctx context.Context, certificateID string, reason instance.TLSRevocationReason, byUserID string) error {
	if !reason.Valid() {
		return fmt.Errorf("instance: unknown revocation reason %q", reason)
	}
	r := instance.TLSCertificateRevocation{
		ID: s.d.IDs.NewID(), CertificateID: certificateID, Reason: reason, ByUserID: byUserID, OccurredAt: s.d.Clock.Now(),
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertTLSCertificateRevocation(ctx, tx, r) })
	if err != nil {
		return fmt.Errorf("instance: record the revocation of the TLS certificate %s: %w", certificateID, err)
	}
	return nil
}

// CurrentTLSCertificate returns the current certificate of host, the last one issued that is not
// revoked, with its key opened; ErrNotFound when there is none. It is what the listener loads.
func (s *Service) CurrentTLSCertificate(ctx context.Context, host string) (TLSKeyPair[instance.TLSCertificate], error) {
	if s.d.Sealer == nil {
		return TLSKeyPair[instance.TLSCertificate]{}, errors.New("instance: TLS certificates need a Sealer")
	}
	c, err := s.d.Repository.CurrentTLSCertificate(ctx, host)
	if err != nil {
		return TLSKeyPair[instance.TLSCertificate]{}, err
	}
	key, err := s.d.Sealer.Open(ctx, c.SealedPrivateKey, TLSCertificateAAD(c.ID))
	if err != nil {
		return TLSKeyPair[instance.TLSCertificate]{}, fmt.Errorf("instance: open the TLS certificate key of %s: %w", host, err)
	}
	return TLSKeyPair[instance.TLSCertificate]{Record: c, PrivateKey: key}, nil
}

// CurrentTLSCertificateID returns the ID of the current certificate of host, ErrNotFound when
// there is none: the periodic check compares it with the certificate in memory without opening a
// key.
func (s *Service) CurrentTLSCertificateID(ctx context.Context, host string) (string, error) {
	return s.d.Repository.CurrentTLSCertificateID(ctx, host)
}

// TLSOrderBackoff returns the wait before the next order of host, derived from the attempts after
// its current certificate that have no certificate, so that it survives restarts.
func (s *Service) TLSOrderBackoff(ctx context.Context, host string) (instance.TLSOrderBackoff, error) {
	var after time.Time
	c, err := s.d.Repository.CurrentTLSCertificate(ctx, host)
	switch {
	case err == nil:
		after = c.CreatedAt
	case !errors.Is(err, ErrNotFound):
		return instance.TLSOrderBackoff{}, err
	}
	attempts, err := s.d.Repository.ListUnresultedTLSOrderAttempts(ctx, host, after)
	if err != nil {
		return instance.TLSOrderBackoff{}, err
	}
	times := make([]time.Time, len(attempts))
	for i, a := range attempts {
		times[i] = a.OccurredAt
	}
	return instance.NewTLSOrderBackoff(times), nil
}
