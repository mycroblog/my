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
	mathrand "math/rand/v2"
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

// The waits of the renewal information (ARI, RFC 9773) of the CA (ADR-0163 §7).
const (
	// tlsRenewalInfoMinWait and tlsRenewalInfoMaxWait clamp the CA's Retry-After.
	tlsRenewalInfoMinWait = time.Minute
	tlsRenewalInfoMaxWait = 24 * time.Hour
	// tlsRenewalInfoRefresh is the longest time between two reads of the renewal information of
	// a certificate, and the wait when the CA says nothing, or offers no ARI.
	tlsRenewalInfoRefresh = 12 * time.Hour
	// tlsRenewalInfoErrorWait is the wait after a read of the renewal information that failed;
	// the renewal time meanwhile comes from the lifetime of the certificate.
	tlsRenewalInfoErrorWait = time.Hour
	// tlsShortLifetime is the lifetime under which a certificate without ARI is renewed after half
	// of it, rather than two thirds.
	tlsShortLifetime = 10 * 24 * time.Hour
)

// ErrTLSOrderInProgress reports that another process holds the lease of the orders of the host:
// it is ordering a certificate now.
var ErrTLSOrderInProgress = errors.New("instance: another process is ordering the TLS certificate of this host")

// errTLSOrderLeaseLost ends the context of an order whose lease could not be extended.
var errTLSOrderLeaseLost = errors.New("instance: lost the lease of the TLS order")

// TLSCertificateSink is where the manager installs the certificates it loads or obtains: the
// certificates in memory that the TLS handshake reads (tlsserve.Holder in internal/platform).
type TLSCertificateSink interface {
	Set(host string, cert *tls.Certificate) error
	// Remove stops serving the certificate of host: every certificate of host was revoked.
	Remove(host string)
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
	// Notify, when set, tells the other processes that this one put a new certificate of host
	// into the database and into its memory (the tls_certificate signal of ADR-0163 §6). It
	// must not block.
	Notify func(host, certificateID string)
	// Meter, when set, receives the metrics of the orders (TLSOrderAttempts and
	// TLSOrderConsecutiveFailures).
	Meter kernel.Meter
}

// TLSManager obtains the certificates of the hosts that the server serves itself with ACME
// (ADR-0163), keeps them in the database, and installs them into the certificates in memory.
//
// Each round, for each host, it loads the database's current certificate when it differs from the
// one in memory (the only time a sealed key is opened), and orders one when the host needs it and
// the backoff of the failed orders allows. The other processes learn of a new certificate from
// Notify, which calls their Notice, and from their own rounds. An order runs under the lease acme.order/<host> in the
// KeyValueStore, so that the processes sharing the database and the KeyValueStore place one order
// per host at a time; after taking the lease, it reads the database again, since another process
// may have finished an order meanwhile. Every attempt, failure, and certificate is recorded, and
// the backoff is derived from them, so that it survives restarts.
//
// A host needs an order when it has no certificate, or only one from another CA than Client's
// (the operator moved from a staging CA to the production one), or its certificate reached its
// renewal time (RenewalTime, ADR-0163 §7). The certificate in memory is served until the new one
// comes, and the new one replaces it (RFC 9773 §5) when the same CA issued both.
type TLSManager struct {
	opts       TLSManagerOptions
	challenges *ACMEChallenges
	lease      tlsOrderLease
	// loading serializes, for each host, the reads of its current certificate and the
	// installs, so that a slow read never installs a certificate over a newer one.
	loading map[string]*sync.Mutex

	mu sync.Mutex
	// loaded is the certificate in memory of each host, and renewals its renewal time.
	loaded   map[string]tlsLoaded
	renewals map[string]tlsRenewal
	// failures is the number of orders of each host since its current certificate that obtained
	// none, as the last round or order saw it (TLSOrderConsecutiveFailures).
	failures map[string]int
	attempts kernel.Counter
	// notices are the hosts to load again (Notice), all of them when noticeAll is set; wake
	// tells the goroutine of Run that loads them.
	notices   map[string]struct{}
	noticeAll bool
	wake      chan struct{}

	accountMu sync.Mutex
	account   *ACMEAccount
}

// tlsLoaded is the certificate in memory of a host, and the directory of the CA that issued it.
type tlsLoaded struct {
	id        string
	directory string
	leaf      *x509.Certificate
}

// tlsRenewal is the renewal time of the certificate id of a host (ADR-0163 §7).
type tlsRenewal struct {
	id string
	// start and end are the window that the CA suggested (ARI), zero without one.
	start, end time.Time
	// at is the renewal time: picked once in the window, and kept until the window changes,
	// or derived from the lifetime of the certificate without one.
	at time.Time
	// ask is when to read the renewal information again.
	ask time.Time
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
	loading := make(map[string]*sync.Mutex, len(hosts))
	for _, h := range hosts {
		loading[h] = &sync.Mutex{}
	}
	m := &TLSManager{
		opts:       opts,
		challenges: NewACMEChallenges(opts.KV),
		lease:      tlsOrderLease{kv: opts.KV, ttl: opts.LeaseTTL, renewal: opts.LeaseRenewal},
		loading:    loading,
		loaded:     map[string]tlsLoaded{},
		renewals:   map[string]tlsRenewal{},
		failures:   map[string]int{},
		notices:    map[string]struct{}{},
		wake:       make(chan struct{}, 1),
	}
	m.registerMetrics(kernel.MeterOrNop(opts.Meter))
	return m, nil
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
// for a certificate it installs. When the database has no current certificate of host, every one
// was revoked (my tls revoke), and the one in memory stops being served. A host that the manager
// does not serve is left as it is.
func (m *TLSManager) Reload(ctx context.Context, host string) error {
	host = normalizeChallengeHost(host)
	lock, ok := m.loading[host]
	if !ok {
		return nil
	}
	lock.Lock()
	defer lock.Unlock()
	id, err := m.opts.Service.CurrentTLSCertificateID(ctx, host)
	if errors.Is(err, ErrNotFound) {
		m.drop(ctx, host)
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

// Notice asks the goroutine of Run to load the current certificate of host from the database
// (Reload): another process told that it changed (the tls_certificate signal, ADR-0163 §6). It
// never blocks, and the notices that arrive before the load runs make one load. The notice says
// only that the host changed: the database decides which certificate is current, so that a late
// or repeated notice never brings an older certificate back.
func (m *TLSManager) Notice(host string) {
	m.mu.Lock()
	m.notices[normalizeChallengeHost(host)] = struct{}{}
	m.mu.Unlock()
	m.signal()
}

// NoticeAll asks for a load of every host as Notice does: the notices may have been missed (the
// listener of the other processes' signals broke, or listens anew).
func (m *TLSManager) NoticeAll() {
	m.mu.Lock()
	m.noticeAll = true
	m.mu.Unlock()
	m.signal()
}

func (m *TLSManager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// runNotices loads the hosts of the notices until ctx ends.
func (m *TLSManager) runNotices(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		}
		m.mu.Lock()
		all, hosts := m.noticeAll, m.notices
		m.noticeAll, m.notices = false, map[string]struct{}{}
		m.mu.Unlock()
		var err error
		if all {
			err = m.ReloadAll(ctx)
		} else {
			var errs []error
			for h := range hosts {
				errs = append(errs, m.Reload(ctx, h))
			}
			err = errors.Join(errs...)
		}
		if err != nil && ctx.Err() == nil {
			m.opts.Logger.WarnContext(ctx, "the TLS certificate of a notice could not be loaded; the next round loads it", slog.Any("error", err))
		}
	}
}

// install puts the certificate id, chainPEM with its key (PKCS #8 DER) issued by the CA of
// directory, into memory for host; the caller holds the loading lock of host.
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

// drop stops serving the certificate in memory of host, whose certificates were all revoked; the
// caller holds the loading lock of host.
func (m *TLSManager) drop(ctx context.Context, host string) {
	m.mu.Lock()
	c, ok := m.loaded[host]
	delete(m.loaded, host)
	delete(m.renewals, host)
	m.mu.Unlock()
	if ok {
		m.opts.Certificates.Remove(host)
		m.opts.Logger.WarnContext(ctx, "the TLS certificate was revoked and has no replacement yet; the host is not served until one is obtained",
			slog.String("host", host), slog.String("certificate", c.id))
	}
}

// RenewalTime returns the renewal time of the certificate in memory of host (ADR-0163 §7), as the
// last round computed it: zero when the host has no certificate of the CA of Client, or no round
// looked at it yet.
func (m *TLSManager) RenewalTime(host string) time.Time {
	host = normalizeChallengeHost(host)
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.loaded[host]
	r := m.renewals[host]
	if !ok || r.id != c.id || c.directory != m.opts.Client.DirectoryURL() {
		return time.Time{}
	}
	return r.at
}

// due returns when host needs an order: zero, that is now, when it has no certificate of the CA of
// Client, and otherwise the renewal time of its certificate, and when to look again before then.
// It reads the CA's renewal information when the last read is due again (ADR-0163 §7).
func (m *TLSManager) due(ctx context.Context, host string) (at, ask time.Time) {
	m.mu.Lock()
	c, ok := m.loaded[host]
	r := m.renewals[host]
	m.mu.Unlock()
	if !ok || c.directory != m.opts.Client.DirectoryURL() {
		return time.Time{}, time.Time{}
	}
	now := m.opts.Clock.Now()
	if r.id == c.id && now.Before(r.ask) {
		return r.at, r.ask
	}
	if r.id != c.id {
		r = tlsRenewal{id: c.id}
	}
	info, err := m.opts.Client.RenewalInfo(ctx, c.leaf)
	switch {
	case err == nil && info.WindowEnd.After(info.WindowStart):
		// One time in the window, kept while the window stays: picking again at each read would
		// keep moving it ahead of the clock.
		if r.at.IsZero() || !r.start.Equal(info.WindowStart) || !r.end.Equal(info.WindowEnd) {
			r.at = info.WindowStart.Add(mathrand.N(info.WindowEnd.Sub(info.WindowStart))) //nolint:gosec // spreading renewals needs no cryptographic randomness
		}
		r.start, r.end = info.WindowStart, info.WindowEnd
		r.ask = now.Add(renewalInfoWait(now, info.RetryAfter))
	case err == nil || errors.Is(err, ErrACMERenewalInfoUnsupported):
		// No window: the CA offers no ARI, or answered an empty window.
		r.start, r.end = time.Time{}, time.Time{}
		r.at = lifetimeRenewalTime(c.leaf)
		r.ask = now.Add(tlsRenewalInfoRefresh)
	default:
		m.opts.Logger.WarnContext(ctx, "the renewal information of the TLS certificate could not be read", slog.String("host", host), slog.Any("error", err))
		if r.at.IsZero() {
			r.at = lifetimeRenewalTime(c.leaf)
		}
		r.ask = now.Add(tlsRenewalInfoErrorWait)
	}
	m.mu.Lock()
	if cur, ok := m.loaded[host]; ok && cur.id == c.id {
		m.renewals[host] = r
	}
	m.mu.Unlock()
	return r.at, r.ask
}

// renewalInfoWait returns how long to wait before reading the renewal information again: the CA's
// retryAfter, clamped to between a minute and a day, and at most 12 hours (ADR-0163 §7).
func renewalInfoWait(now, retryAfter time.Time) time.Duration {
	if retryAfter.IsZero() {
		return tlsRenewalInfoRefresh
	}
	wait := min(max(retryAfter.Sub(now), tlsRenewalInfoMinWait), tlsRenewalInfoMaxWait)
	return min(wait, tlsRenewalInfoRefresh)
}

// lifetimeRenewalTime returns the renewal time of leaf without ARI: after two thirds of its
// lifetime, or half of it for a certificate valid under 10 days (ADR-0163 §7).
func lifetimeRenewalTime(leaf *x509.Certificate) time.Time {
	life := leaf.NotAfter.Sub(leaf.NotBefore)
	if life < tlsShortLifetime {
		return leaf.NotBefore.Add(life / 2)
	}
	return leaf.NotBefore.Add(life * 2 / 3)
}

// Run runs a round (Maintain) for every host, and the next one when the earliest host needs it,
// until ctx ends, and loads the certificates of the notices (Notice, NoticeAll) as they come.
// Start it once the listeners are up: the CA validates TLS-ALPN-01 on the TLS listener
// (ADR-0163 §7).
func (m *TLSManager) Run(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Go(func() { m.runNotices(ctx) })
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

// maintain runs the round of host and returns when its next one is due: after CheckInterval, or
// at the renewal time when it comes first, and after RetryInterval once the host needs an order
// that this round did not obtain (ADR-0163 §6), or earlier when the backoff ends first.
func (m *TLSManager) maintain(ctx context.Context, host string) (time.Time, error) {
	now := m.opts.Clock.Now()
	retry := now.Add(m.opts.RetryInterval)
	if err := m.Reload(ctx, host); err != nil {
		return retry, err
	}
	// The backoff is read at every round, not only when the host is due, so that the failures
	// it counts stay current (TLSOrderConsecutiveFailures).
	wait, err := m.backoff(ctx, host)
	if err != nil {
		return retry, err
	}
	if next, ok := m.notDue(ctx, host, now); ok {
		return next, nil
	}
	if !wait.IsZero() {
		return earliest(wait, retry), nil
	}
	var next time.Time
	held, err := m.lease.hold(ctx, host, func(ctx context.Context) error {
		// Another process may have obtained the certificate, or failed, while this one waited.
		if err := m.Reload(ctx, host); err != nil {
			return err
		}
		if _, ok := m.notDue(ctx, host, m.opts.Clock.Now()); ok {
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
		return earliest(next, retry), nil
	}
	now = m.opts.Clock.Now()
	if next, ok := m.notDue(ctx, host, now); ok {
		return next, nil
	}
	return now.Add(m.opts.RetryInterval), nil
}

// notDue reports whether host needs no order at now, and if so when its next round is due: after
// CheckInterval, or at its renewal time or the next read of the renewal information when either
// comes first.
func (m *TLSManager) notDue(ctx context.Context, host string, now time.Time) (time.Time, bool) {
	at, ask := m.due(ctx, host)
	if !now.Before(at) {
		return time.Time{}, false
	}
	return earliest(now.Add(m.opts.CheckInterval), at, ask), true
}

// earliest returns the earliest of ts, which are not zero.
func earliest(ts ...time.Time) time.Time {
	return slices.MinFunc(ts, func(a, b time.Time) int { return a.Compare(b) })
}

// OrderNow orders a certificate of host at once, skipping its renewal time and the backoff of the
// failed orders, for an operator who asks for one (my tls renew): under the lease of the orders
// of host, as a round orders, and replacing the current certificate when the same CA issued it.
// It reports ErrTLSOrderInProgress while another process holds the lease.
func (m *TLSManager) OrderNow(ctx context.Context, host string) error {
	host = normalizeChallengeHost(host)
	if !slices.Contains(m.opts.Hosts, host) {
		return fmt.Errorf("instance: %q is not a host of the TLS certificates", host)
	}
	held, err := m.lease.hold(ctx, host, func(ctx context.Context) error {
		if err := m.Reload(ctx, host); err != nil {
			return err
		}
		return m.order(ctx, host)
	})
	if err == nil && !held {
		return ErrTLSOrderInProgress
	}
	return err
}

// backoff returns when the backoff of the failed orders of host allows the next one, zero when it
// allows one now.
func (m *TLSManager) backoff(ctx context.Context, host string) (time.Time, error) {
	b, err := m.opts.Service.TLSOrderBackoff(ctx, host)
	if err != nil {
		return time.Time{}, fmt.Errorf("instance: read the TLS orders of %s: %w", host, err)
	}
	m.mu.Lock()
	m.failures[host] = b.Failures
	m.mu.Unlock()
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
	var replacesLeaf *x509.Certificate
	m.mu.Lock()
	if c, ok := m.loaded[host]; ok && c.directory == dir {
		replaces, replacesLeaf = c.id, c.leaf
	}
	m.mu.Unlock()
	if replaces != "" {
		// A certificate is replaced once (RFC 9773 §5): the one in memory may be an older one
		// served again because its replacement was revoked.
		superseded, err := m.opts.Service.TLSCertificateSuperseded(ctx, replaces)
		if err != nil {
			return fmt.Errorf("instance: read the TLS certificates of %s: %w", host, err)
		}
		if superseded {
			replaces, replacesLeaf = "", nil
		}
	}
	attempt, err := m.opts.Service.RecordTLSOrderAttempt(ctx, TLSOrder{
		Host: host, DirectoryURL: dir, Profile: m.opts.Profile, ReplacesCertificateID: replaces,
	})
	if err != nil {
		return err
	}
	m.opts.Logger.InfoContext(ctx, "ordering a TLS certificate", slog.String("host", host), slog.String("directory", attempt.DirectoryURL))
	chain, key, err := m.obtain(ctx, host, replacesLeaf)
	m.countOrder(ctx, host, err == nil)
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
		slog.String("serial", c.Serial), slog.Time("not_after", c.NotAfter), slog.Bool("renewal", replaces != ""))
	lock := m.loading[host]
	lock.Lock()
	err = m.install(host, c.ID, c.DirectoryURL, c.ChainPEM, key)
	lock.Unlock()
	if err != nil {
		return err
	}
	if m.opts.Notify != nil {
		m.opts.Notify(host, c.ID)
	}
	return nil
}

// obtain orders the certificate of host with a new key, replacing the certificate replaces when
// not nil, and returns its chain and the key in PKCS #8 DER.
func (m *TLSManager) obtain(ctx context.Context, host string, replaces *x509.Certificate) (string, []byte, error) {
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
		Account: account, Host: host, Key: key, Profile: m.opts.Profile, Replaces: replaces,
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
