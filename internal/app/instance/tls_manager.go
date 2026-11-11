// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The defaults of the rounds of the TLSManager.
const (
	// DefaultTLSCheckInterval is how often the manager reads the current certificate of each host
	// from the database while it serves one (ADR-0163 §6).
	DefaultTLSCheckInterval = 5 * time.Minute
	// DefaultTLSRetryInterval is how often it looks again while a host has no certificate: another
	// process may be ordering it, or the last round failed before an order was recorded.
	DefaultTLSRetryInterval = time.Minute
)

// errTLSOrderLeaseLost ends the context of an order whose lease could not be extended.
var errTLSOrderLeaseLost = errors.New("instance: lost the lease of the TLS order")

// TLSCertificateSink is where the manager installs the certificates it loads or obtains: the
// certificates in memory that the TLS handshake reads (tlsserve.Holder in internal/platform).
type TLSCertificateSink interface {
	Set(host string, cert *tls.Certificate) error
}

// TLSManagerOptions configure a TLSManager.
type TLSManagerOptions struct {
	// Service records the accounts, the orders, and the certificates in the database.
	Service *Service
	// Client is the CA.
	Client ACMEClient
	// KV is shared by every process of the server: it holds the leases of the orders and the
	// answers of the challenges (ADR-0163 §5).
	KV kernel.KeyValueStore
	// Certificates receives the certificate of each host.
	Certificates TLSCertificateSink
	// Hosts are the names to obtain certificates for, fixed for the life of the manager
	// (ADR-0163 §8): no handshake adds one.
	Hosts []string
	// Email is the contact of the account, empty for none.
	Email string
	// Profile is the CA's profile of the certificates, empty for the CA's default.
	Profile string
	// Challenges are the challenge types the orders may answer, by preference; empty means
	// TLS-ALPN-01 and then HTTP-01.
	Challenges []string
	Clock      kernel.Clock
	// Logger logs the orders and their failures; nil logs nothing.
	Logger *slog.Logger
	// LeaseTTL and LeaseRenewal are the life of the lease of an order and how often it is
	// extended; zero means DefaultTLSOrderLeaseTTL and DefaultTLSOrderLeaseRenewal.
	LeaseTTL     time.Duration
	LeaseRenewal time.Duration
	// CheckInterval and RetryInterval are the waits of Run; zero means DefaultTLSCheckInterval
	// and DefaultTLSRetryInterval.
	CheckInterval time.Duration
	RetryInterval time.Duration
}

// TLSManager obtains the certificates of the hosts that the server serves itself with ACME
// (ADR-0163), keeps them in the database, and installs them into the certificates in memory.
//
// Each round, for each host, it loads the database's current certificate when it differs from the
// one in memory (the only time a sealed key is opened), and orders one when the host needs it and
// the backoff of the failed orders allows. An order runs under the lease acme.order/<host> in the
// KeyValueStore, so that the processes sharing the database and the KeyValueStore place one order
// per host at a time; after taking the lease, it reads the database again, since another process
// may have finished an order meanwhile. Every attempt, failure, and certificate is recorded, and
// the backoff is derived from them, so that it survives restarts.
//
// A host needs an order when it has no certificate, or only one from another CA than Client's
// (the operator moved from a staging CA to the production one): that one is served until the new
// one comes. The renewal of a certificate is decided in needsOrder too, and a notice to the other
// processes would go out from install.
type TLSManager struct {
	opts       TLSManagerOptions
	challenges *ACMEChallenges
	lease      tlsOrderLease

	mu sync.Mutex
	// loaded is the certificate in memory of each host.
	loaded map[string]tlsLoaded

	accountMu sync.Mutex
	account   *ACMEAccount
}

// tlsLoaded is the certificate in memory of a host, and the directory of the CA that issued it.
type tlsLoaded struct {
	id        string
	directory string
	leaf      *x509.Certificate
}

// NewTLSManager returns the manager of opts.
func NewTLSManager(opts TLSManagerOptions) (*TLSManager, error) {
	switch {
	case opts.Service == nil || opts.Client == nil || opts.KV == nil || opts.Certificates == nil || opts.Clock == nil:
		return nil, errors.New("instance: a TLS manager needs a Service, a Client, a KV, Certificates, and a Clock")
	case len(opts.Hosts) == 0:
		return nil, errors.New("instance: a TLS manager needs a host")
	}
	hosts := make([]string, 0, len(opts.Hosts))
	for _, h := range opts.Hosts {
		if h = normalizeChallengeHost(h); !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	opts.Hosts = hosts
	if len(opts.Challenges) == 0 {
		opts.Challenges = []string{ACMEChallengeTLSALPN01, ACMEChallengeHTTP01}
	}
	for _, c := range opts.Challenges {
		if c != ACMEChallengeTLSALPN01 && c != ACMEChallengeHTTP01 {
			return nil, fmt.Errorf("instance: the %q challenge is not answered", c)
		}
	}
	if opts.LeaseTTL <= 0 {
		opts.LeaseTTL = DefaultTLSOrderLeaseTTL
	}
	if opts.LeaseRenewal <= 0 {
		opts.LeaseRenewal = DefaultTLSOrderLeaseRenewal
	}
	if opts.CheckInterval <= 0 {
		opts.CheckInterval = DefaultTLSCheckInterval
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = DefaultTLSRetryInterval
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &TLSManager{
		opts:       opts,
		challenges: NewACMEChallenges(opts.KV),
		lease:      tlsOrderLease{kv: opts.KV, ttl: opts.LeaseTTL, renewal: opts.LeaseRenewal},
		loaded:     map[string]tlsLoaded{},
	}, nil
}

// Hosts returns the hosts of the manager.
func (m *TLSManager) Hosts() []string { return slices.Clone(m.opts.Hosts) }

// Challenges returns the answers of the challenges, which the listeners serve to the CA.
func (m *TLSManager) Challenges() *ACMEChallenges { return m.challenges }

// CertificateID returns the ID of the certificate in memory of host, empty when it has none.
func (m *TLSManager) CertificateID(host string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loaded[normalizeChallengeHost(host)].id
}

// ReloadAll loads the current certificate of every host from the database (Reload).
func (m *TLSManager) ReloadAll(ctx context.Context) error {
	var errs []error
	for _, h := range m.opts.Hosts {
		errs = append(errs, m.Reload(ctx, h))
	}
	return errors.Join(errs...)
}

// Reload loads the current certificate of host from the database into memory, unless it is the
// one there already: it reads only the certificate's ID to compare, and opens the sealed key only
// for a certificate it installs. A host without a certificate in the database is left as it is.
func (m *TLSManager) Reload(ctx context.Context, host string) error {
	host = normalizeChallengeHost(host)
	id, err := m.opts.Service.CurrentTLSCertificateID(ctx, host)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("instance: read the TLS certificate of %s: %w", host, err)
	}
	if m.CertificateID(host) == id {
		return nil
	}
	pair, err := m.opts.Service.CurrentTLSCertificate(ctx, host)
	if err != nil {
		return err
	}
	return m.install(host, pair.Record.ID, pair.Record.DirectoryURL, pair.Record.ChainPEM, pair.PrivateKey)
}

// install puts the certificate id, chainPEM with its key (PKCS #8 DER) issued by the CA of
// directory, into memory for host.
func (m *TLSManager) install(host, id, directory, chainPEM string, pkcs8 []byte) error {
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	cert, err := tls.X509KeyPair([]byte(chainPEM), keyPEM)
	if err != nil {
		return fmt.Errorf("instance: the TLS certificate %s of %s: %w", id, host, err)
	}
	if err := m.opts.Certificates.Set(host, &cert); err != nil {
		return err
	}
	m.mu.Lock()
	m.loaded[host] = tlsLoaded{id: id, directory: directory, leaf: cert.Leaf}
	m.mu.Unlock()
	return nil
}

// needsOrder reports whether host needs a new certificate: it has none from the CA of Client.
func (m *TLSManager) needsOrder(host string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.loaded[host]
	return !ok || c.directory != m.opts.Client.DirectoryURL()
}

// Run runs a round (Maintain) for every host, and the next one when the earliest host needs it,
// until ctx ends. Start it once the listeners are up: the CA validates TLS-ALPN-01 on the TLS
// listener (ADR-0163 §7).
func (m *TLSManager) Run(ctx context.Context) {
	for {
		next := m.Maintain(ctx)
		wait := min(max(next.Sub(m.opts.Clock.Now()), time.Second), m.opts.CheckInterval)
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// Maintain runs one round for every host at once (see TLSManager) and returns when the next round
// is due. It logs the failures.
func (m *TLSManager) Maintain(ctx context.Context) time.Time {
	nexts := make([]time.Time, len(m.opts.Hosts))
	var wg sync.WaitGroup
	for i, host := range m.opts.Hosts {
		wg.Go(func() {
			next, err := m.maintain(ctx, host)
			if err != nil && ctx.Err() == nil {
				m.opts.Logger.WarnContext(ctx, "the TLS certificate could not be obtained", slog.String("host", host), slog.Any("error", err))
			}
			nexts[i] = next
		})
	}
	wg.Wait()
	return slices.MinFunc(nexts, func(a, b time.Time) int { return a.Compare(b) })
}

// maintain runs the round of host and returns when its next one is due.
func (m *TLSManager) maintain(ctx context.Context, host string) (time.Time, error) {
	now := m.opts.Clock.Now()
	retry := now.Add(m.opts.RetryInterval)
	if err := m.Reload(ctx, host); err != nil {
		return retry, err
	}
	if !m.needsOrder(host) {
		return now.Add(m.opts.CheckInterval), nil
	}
	wait, err := m.backoff(ctx, host)
	if err != nil {
		return retry, err
	}
	if !wait.IsZero() {
		return wait, nil
	}
	var next time.Time
	held, err := m.lease.hold(ctx, host, func(ctx context.Context) error {
		// Another process may have obtained the certificate, or failed, while this one waited.
		if err := m.Reload(ctx, host); err != nil {
			return err
		}
		if !m.needsOrder(host) {
			return nil
		}
		wait, err := m.backoff(ctx, host)
		if err != nil || !wait.IsZero() {
			next = wait
			return err
		}
		return m.order(ctx, host)
	})
	switch {
	case err != nil:
		return retry, err
	case !held:
		// Another process orders it; the next round loads what it obtains.
		return retry, nil
	case !next.IsZero():
		return next, nil
	case m.needsOrder(host):
		return retry, nil
	}
	return m.opts.Clock.Now().Add(m.opts.CheckInterval), nil
}

// backoff returns when the backoff of the failed orders of host allows the next one, zero when it
// allows one now.
func (m *TLSManager) backoff(ctx context.Context, host string) (time.Time, error) {
	b, err := m.opts.Service.TLSOrderBackoff(ctx, host)
	if err != nil {
		return time.Time{}, fmt.Errorf("instance: read the TLS orders of %s: %w", host, err)
	}
	if b.Allows(m.opts.Clock.Now()) {
		return time.Time{}, nil
	}
	return b.NotBefore, nil
}

// order records an attempt, orders the certificate of host, and records and installs the result.
// The attempt is recorded first, so that every failure, the account's included, counts toward the
// backoff.
func (m *TLSManager) order(ctx context.Context, host string) error {
	dir := m.opts.Client.DirectoryURL()
	// A certificate replaces one of the same CA only.
	var replaces string
	m.mu.Lock()
	if c := m.loaded[host]; c.directory == dir {
		replaces = c.id
	}
	m.mu.Unlock()
	attempt, err := m.opts.Service.RecordTLSOrderAttempt(ctx, TLSOrder{
		Host: host, DirectoryURL: dir, Profile: m.opts.Profile, ReplacesCertificateID: replaces,
	})
	if err != nil {
		return err
	}
	m.opts.Logger.InfoContext(ctx, "ordering a TLS certificate", slog.String("host", host), slog.String("directory", attempt.DirectoryURL))
	chain, key, err := m.obtain(ctx, host)
	if err != nil {
		problem, detail := classifyTLSOrderFailure(ctx, err)
		recErr := m.opts.Service.RecordTLSOrderFailure(context.WithoutCancel(ctx), attempt.ID, problem, detail)
		return errors.Join(fmt.Errorf("instance: the TLS order of %s failed: %w", host, err), recErr)
	}
	// The CA issued it: keep it even when the lease ended meanwhile.
	c, err := m.opts.Service.RecordTLSCertificate(context.WithoutCancel(ctx), attempt, chain, key)
	if err != nil {
		return err
	}
	m.opts.Logger.InfoContext(ctx, "obtained a TLS certificate", slog.String("host", host),
		slog.String("serial", c.Serial), slog.Time("not_after", c.NotAfter))
	return m.install(host, c.ID, c.DirectoryURL, c.ChainPEM, key)
}

// obtain orders the certificate of host with a new key, and returns its chain and the key in
// PKCS #8 DER.
func (m *TLSManager) obtain(ctx context.Context, host string) (string, []byte, error) {
	account, err := m.acmeAccount(ctx)
	if err != nil {
		return "", nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", nil, err
	}
	chain, err := m.opts.Client.Order(ctx, ACMEOrderRequest{
		Account: account, Host: host, Key: key, Profile: m.opts.Profile,
		Challenges: m.opts.Challenges, Solver: m.challenges,
	})
	if err != nil {
		return "", nil, err
	}
	return chain, pkcs8, nil
}

// acmeAccount returns the account of the CA, registering one when the database has none. When
// two processes register at once, both use the account stored first.
func (m *TLSManager) acmeAccount(ctx context.Context) (ACMEAccount, error) {
	m.accountMu.Lock()
	defer m.accountMu.Unlock()
	if m.account != nil {
		return *m.account, nil
	}
	dir := m.opts.Client.DirectoryURL()
	stored, err := m.opts.Service.TLSAccount(ctx, dir)
	if errors.Is(err, ErrNotFound) {
		stored, err = m.registerAccount(ctx, dir)
	}
	if err != nil {
		return ACMEAccount{}, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(stored.PrivateKey)
	if err != nil {
		return ACMEAccount{}, fmt.Errorf("instance: the key of the ACME account %s: %w", stored.Record.ID, err)
	}
	signer, ok := parsed.(crypto.Signer)
	if !ok {
		return ACMEAccount{}, fmt.Errorf("instance: the key of the ACME account %s cannot sign", stored.Record.ID)
	}
	m.account = &ACMEAccount{URL: stored.Record.AccountURL, Key: signer}
	return *m.account, nil
}

func (m *TLSManager) registerAccount(ctx context.Context, dir string) (TLSKeyPair[instance.TLSAccount], error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return TLSKeyPair[instance.TLSAccount]{}, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return TLSKeyPair[instance.TLSAccount]{}, err
	}
	var contact []string
	if m.opts.Email != "" {
		contact = []string{"mailto:" + m.opts.Email}
	}
	url, err := m.opts.Client.RegisterAccount(ctx, key, contact)
	if err != nil {
		return TLSKeyPair[instance.TLSAccount]{}, fmt.Errorf("instance: register the ACME account: %w", err)
	}
	m.opts.Logger.InfoContext(ctx, "registered an ACME account", slog.String("directory", dir), slog.String("account", url))
	return m.opts.Service.RegisterTLSAccount(ctx, dir, url, pkcs8)
}

// classifyTLSOrderFailure returns the problem type and the detail of a failed order: the CA's
// problem, or the class of a failure on the client's side.
func classifyTLSOrderFailure(ctx context.Context, err error) (string, string) {
	var p *ACMEProblem
	switch {
	case errors.As(err, &p):
		return p.Type, err.Error()
	case ctx.Err() != nil:
		if cause := context.Cause(ctx); cause != nil && !errors.Is(err, cause) {
			err = fmt.Errorf("%w (%w)", err, cause)
		}
		return instance.TLSOrderFailureCanceled, err.Error()
	}
	return instance.TLSOrderFailureClient, err.Error()
}
