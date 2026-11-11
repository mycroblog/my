// SPDX-License-Identifier: AGPL-3.0-only

package acmetls

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// SQLiteSchema is the spike's schema, in the shape the instance module would own (ADR-0020):
//
//   - tls_acme_accounts: resource. One account per CA directory, its key sealed. INSERT only.
//   - tls_order_attempts: event. A process that holds the lease is about to place an order.
//   - tls_order_failures: event, the outcome of an attempt that failed, with the CA's problem.
//   - tls_certificates: resource, the outcome of an attempt that succeeded. INSERT only; the
//     current certificate of a host is the one issued last (a query, or a projection if the
//     admin screen needs one).
//
// Backoff and "when may we order again" are derived from the events, so they survive restarts and
// the loss of the KeyValueStore.
const SQLiteSchema = `
CREATE TABLE tls_acme_accounts (
  id TEXT NOT NULL PRIMARY KEY,
  directory_url TEXT NOT NULL,
  account_url TEXT NOT NULL,
  sealed_private_key BLOB NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX tls_acme_accounts_directory ON tls_acme_accounts (directory_url);

CREATE TABLE tls_order_attempts (
  id TEXT NOT NULL PRIMARY KEY,
  host TEXT NOT NULL,
  directory_url TEXT NOT NULL,
  replaces_certificate_id TEXT REFERENCES tls_certificates (id),
  occurred_at INTEGER NOT NULL
) STRICT;
CREATE INDEX tls_order_attempts_host ON tls_order_attempts (host, occurred_at);

CREATE TABLE tls_order_failures (
  attempt_id TEXT NOT NULL PRIMARY KEY REFERENCES tls_order_attempts (id),
  problem_type TEXT NOT NULL,
  detail TEXT NOT NULL,
  occurred_at INTEGER NOT NULL
) STRICT;

CREATE TABLE tls_certificates (
  id TEXT NOT NULL PRIMARY KEY,
  attempt_id TEXT NOT NULL REFERENCES tls_order_attempts (id),
  host TEXT NOT NULL,
  directory_url TEXT NOT NULL,
  serial TEXT NOT NULL,
  not_before INTEGER NOT NULL,
  not_after INTEGER NOT NULL,
  chain_pem BLOB NOT NULL,
  sealed_private_key BLOB NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX tls_certificates_attempt ON tls_certificates (attempt_id);
CREATE INDEX tls_certificates_host ON tls_certificates (host, created_at);
`

// DBStore keeps the ACME state in the tables of SQLiteSchema.
type DBStore struct {
	DB     *sql.DB
	Sealer *kernel.Sealer
	Clock  kernel.Clock
	NewID  func() string

	// Opens counts Sealer.Open calls, to show how often a key is unsealed.
	Opens atomic.Int64
}

// AccountKeyAAD binds the sealed account key to its row and column.
func AccountKeyAAD(id string) []byte {
	return kernel.AAD("tls_acme_accounts", id, "sealed_private_key")
}

// CertificateKeyAAD binds the sealed certificate key to its row and column.
func CertificateKeyAAD(id string) []byte {
	return kernel.AAD("tls_certificates", id, "sealed_private_key")
}

// ErrNone reports that nothing is stored yet.
var ErrNone = errors.New("acmetls: none")

// StoredAccount is an ACME account of a directory.
type StoredAccount struct {
	URL string
	Key crypto.Signer
}

// Account returns the account of directory, or ErrNone.
func (s *DBStore) Account(ctx context.Context, directory string) (StoredAccount, error) {
	var id, url string
	var sealed []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id, account_url, sealed_private_key FROM tls_acme_accounts WHERE directory_url = ?`, directory).Scan(&id, &url, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredAccount{}, ErrNone
	}
	if err != nil {
		return StoredAccount{}, err
	}
	s.Opens.Add(1)
	der, err := s.Sealer.Open(ctx, sealed, AccountKeyAAD(id))
	if err != nil {
		return StoredAccount{}, err
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return StoredAccount{}, err
	}
	return StoredAccount{URL: url, Key: key.(crypto.Signer)}, nil
}

// InsertAccount stores the account of directory unless one is there; the caller then reads it
// back, so that two processes that registered at once settle on one.
func (s *DBStore) InsertAccount(ctx context.Context, directory, url string, key crypto.Signer) error {
	id := s.NewID()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	sealed, err := s.Sealer.Seal(ctx, der, AccountKeyAAD(id))
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO tls_acme_accounts (id, directory_url, account_url, sealed_private_key, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT (directory_url) DO NOTHING`,
		id, directory, url, sealed, s.Clock.Now().UnixMilli())
	return err
}

// RecordAttempt records that an order for host is about to be placed.
func (s *DBStore) RecordAttempt(ctx context.Context, host, directory, replaces string) (string, error) {
	id := s.NewID()
	var rep any
	if replaces != "" {
		rep = replaces
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO tls_order_attempts (id, host, directory_url, replaces_certificate_id, occurred_at) VALUES (?, ?, ?, ?, ?)`,
		id, host, directory, rep, s.Clock.Now().UnixMilli())
	return id, err
}

// RecordFailure records the failure of an attempt.
func (s *DBStore) RecordFailure(ctx context.Context, attemptID, problemType, detail string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO tls_order_failures (attempt_id, problem_type, detail, occurred_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		attemptID, problemType, detail, s.Clock.Now().UnixMilli())
	return err
}

// InsertCertificate records the certificate an attempt obtained, its key sealed.
func (s *DBStore) InsertCertificate(ctx context.Context, attemptID, host, directory string, chainPEM []byte, key crypto.Signer) (string, error) {
	leaf, err := leafOf(chainPEM)
	if err != nil {
		return "", err
	}
	id := s.NewID()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	sealed, err := s.Sealer.Seal(ctx, der, CertificateKeyAAD(id))
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO tls_certificates (id, attempt_id, host, directory_url, serial, not_before, not_after, chain_pem, sealed_private_key, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, attemptID, host, directory, leaf.SerialNumber.Text(16), leaf.NotBefore.UnixMilli(), leaf.NotAfter.UnixMilli(), chainPEM, sealed, s.Clock.Now().UnixMilli())
	return id, err
}

// LatestID returns the ID of the certificate of host issued last, or ErrNone. It is the cheap
// query that every process polls to notice a renewal by another one. "Last issued", not "latest
// not_after": a renewal under a shorter profile (90 days, then 45 or 6) ends before the
// certificate it replaces, and must still replace it.
func (s *DBStore) LatestID(ctx context.Context, host string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM tls_certificates WHERE host = ? ORDER BY created_at DESC, id DESC LIMIT 1`, host).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNone
	}
	return id, err
}

// Load opens the certificate id.
func (s *DBStore) Load(ctx context.Context, id string) (*tls.Certificate, error) {
	var chain, sealed []byte
	if err := s.DB.QueryRowContext(ctx, `SELECT chain_pem, sealed_private_key FROM tls_certificates WHERE id = ?`, id).Scan(&chain, &sealed); err != nil {
		return nil, err
	}
	s.Opens.Add(1)
	der, err := s.Sealer.Open(ctx, sealed, CertificateKeyAAD(id))
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	c, err := tls.X509KeyPair(chain, keyPEM)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Backoff is the wait after the n-th consecutive failure: 15 minutes doubled each time, at most a
// day. It keeps one identifier under Let's Encrypt's 5 failed authorizations per hour, and once
// at the cap, at one failure a day, it never drains the 1,152 consecutive failures that pause an account.
func Backoff(n int) time.Duration {
	if n <= 0 {
		return 0
	}
	d := 15 * time.Minute
	for i := 1; i < n && d < 24*time.Hour; i++ {
		d *= 2
	}
	return min(d, 24*time.Hour)
}

// NextAttempt returns the earliest time an order for host may be placed, and the number of
// attempts since the last certificate that did not obtain one: failed, or abandoned by a process
// that died while ordering (no outcome). Both are counted, since either may have spent the CA's
// limits.
func (s *DBStore) NextAttempt(ctx context.Context, host string) (time.Time, int, error) {
	var n int
	var last sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `
SELECT COUNT(*), MAX(a.occurred_at)
FROM tls_order_attempts a
LEFT JOIN tls_certificates c ON c.attempt_id = a.id
WHERE a.host = ? AND c.id IS NULL
  AND a.occurred_at > COALESCE((SELECT MAX(created_at) FROM tls_certificates WHERE host = ?), 0)`, host, host).Scan(&n, &last)
	if err != nil {
		return time.Time{}, 0, err
	}
	if n == 0 {
		return time.Time{}, 0, nil
	}
	return time.UnixMilli(last.Int64).Add(Backoff(n)), n, nil
}

// CountAttempts returns how many attempts were recorded for host.
func (s *DBStore) CountAttempts(ctx context.Context, host string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tls_order_attempts WHERE host = ?`, host).Scan(&n)
	return n, err
}

func leafOf(chainPEM []byte) (*x509.Certificate, error) {
	b, _ := pem.Decode(chainPEM)
	if b == nil {
		return nil, fmt.Errorf("acmetls: no certificate in the chain")
	}
	return x509.ParseCertificate(b.Bytes)
}
