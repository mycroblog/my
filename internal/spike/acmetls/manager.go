// SPDX-License-Identifier: AGPL-3.0-only

package acmetls

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mholt/acmez/v3"
	"github.com/mholt/acmez/v3/acme"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
)

// KV namespaces of the challenges. Every process answers a challenge, wherever the CA's
// validation request lands, so the key authorizations live in the shared KeyValueStore.
const (
	nsHTTP01   = "acme.http01"
	nsTLSALPN  = "acme.tlsalpn01"
	nsLease    = "acme.order"
	challenges = 10 * time.Minute
)

// Notice is the payload a process sends on the Bus after it changed the certificate of a host.
// It carries no secret, and the receiver treats it only as a hint to read the DB again: the DB
// decides which certificate is current, so a late or repeated notice never installs an older one.
type Notice struct {
	Kind          string `json:"k"` // "tls_certificate"
	Host          string `json:"h"`
	CertificateID string `json:"c"`
}

// Manager obtains, renews, and serves the certificates of Hosts.
//
// The handshake reads only memory: GetCertificate never touches the DB, the KMS, or the
// KeyValueStore (except for the acme-tls/1 handshake of a TLS-ALPN-01 validation). A sealed key is
// opened once when its certificate is loaded: at start, after this process renews, after a Bus
// notice, and when the periodic check finds a newer row.
type Manager struct {
	Hosts      []string
	Directory  string
	Email      string
	Profile    string
	HTTPClient *http.Client
	Store      *DBStore
	Lease      *Lease
	KV         kernel.KeyValueStore
	// Bus is nil with stream = hub: one process, nothing to tell.
	Bus    stream.Bus
	Clock  kernel.Clock
	Logger *slog.Logger
	// RenewRatio is the share of the lifetime after which a certificate renews without ARI.
	RenewRatio float64

	mu    sync.RWMutex
	certs map[string]*entry

	Reloads, Orders atomic.Int64
}

type entry struct {
	id   string
	cert *tls.Certificate
	leaf *x509.Certificate
	ari  *acme.RenewalInfo
	// at is the time picked in ari's window; it is picked once per window (RFC 9773 §4.2),
	// not at each check, or a later pick could keep moving past the clock.
	at time.Time
}

func (m *Manager) client() *acmez.Client {
	return &acmez.Client{
		Client: &acme.Client{Directory: m.Directory, HTTPClient: m.HTTPClient, Logger: m.Logger},
		ChallengeSolvers: map[string]acmez.Solver{
			acme.ChallengeTypeHTTP01:    http01Solver{m.KV},
			acme.ChallengeTypeTLSALPN01: tlsALPNSolver{m.KV},
		},
	}
}

// LoadAll reads the current certificate of every host from the DB.
func (m *Manager) LoadAll(ctx context.Context) error {
	var errs []error
	for _, h := range m.Hosts {
		errs = append(errs, m.reload(ctx, h))
	}
	return errors.Join(errs...)
}

// reload installs the DB's current certificate of host, if it is not the one in memory. Only this
// opens a sealed key.
func (m *Manager) reload(ctx context.Context, host string) error {
	id, err := m.Store.LatestID(ctx, host)
	if errors.Is(err, ErrNone) {
		return nil
	}
	if err != nil {
		return err
	}
	m.mu.RLock()
	cur := m.certs[host]
	m.mu.RUnlock()
	if cur != nil && cur.id == id {
		return nil
	}
	c, err := m.Store.Load(ctx, id)
	if err != nil {
		return err
	}
	m.Reloads.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.certs == nil {
		m.certs = map[string]*entry{}
	}
	m.certs[host] = &entry{id: id, cert: c, leaf: c.Leaf}
	return nil
}

// GetCertificate is tls.Config.GetCertificate.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	host := strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))
	if len(hello.SupportedProtos) == 1 && hello.SupportedProtos[0] == acmez.ACMETLS1Protocol {
		b, err := m.KV.Get(hello.Context(), nsTLSALPN, host)
		if err != nil {
			return nil, fmt.Errorf("acmetls: no tls-alpn-01 challenge for %q", host)
		}
		var ch acme.Challenge
		if err := json.Unmarshal(b, &ch); err != nil {
			return nil, err
		}
		return acmez.TLSALPN01ChallengeCert(ch)
	}
	m.mu.RLock()
	e := m.certs[host]
	m.mu.RUnlock()
	if e == nil {
		// Never order here: an arbitrary SNI must not start an order (HostPolicy).
		return nil, fmt.Errorf("acmetls: no certificate for %q", host)
	}
	return e.cert, nil
}

// TLSConfig returns the server's TLS configuration.
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: m.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1", acmez.ACMETLS1Protocol},
	}
}

// HTTPHandler answers HTTP-01 challenges and redirects everything else to https.
func (m *Manager) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := strings.CutPrefix(r.URL.Path, "/.well-known/acme-challenge/"); ok {
			b, err := m.KV.Get(r.Context(), nsHTTP01, token)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(b)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "use https", http.StatusBadRequest)
			return
		}
		u := *r.URL
		u.Scheme, u.Host = "https", r.Host
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
	})
}

// renewAt returns when the certificate in memory should be renewed: a time in the CA's ARI
// window when the CA offers one, else RenewRatio of the lifetime.
func (m *Manager) renewAt(ctx context.Context, e *entry) time.Time {
	if e.ari == nil || e.ari.NeedsRefresh() {
		ri, err := m.client().GetRenewalInfo(ctx, e.leaf)
		if err == nil && ri.HasWindow() {
			if e.ari == nil || !e.ari.SameWindow(ri) {
				e.at = time.Time{}
			}
			e.ari = &ri
		}
	}
	if e.ari != nil && e.ari.HasWindow() {
		if e.at.IsZero() {
			s, end := e.ari.SuggestedWindow.Start, e.ari.SuggestedWindow.End
			e.at = s
			if end.After(s) {
				n, _ := rand.Int(rand.Reader, big.NewInt(int64(end.Sub(s))))
				e.at = s.Add(time.Duration(n.Int64()))
			}
		}
		return e.at
	}
	// Without ARI: when a third of the lifetime is left, or half of it for one under 10 days
	// (Let's Encrypt's integration guide).
	life := e.leaf.NotAfter.Sub(e.leaf.NotBefore)
	ratio := m.RenewRatio
	if life < 10*24*time.Hour {
		ratio = 0.5
	}
	return e.leaf.NotBefore.Add(time.Duration(float64(life) * ratio))
}

// Maintain runs one round for every host: pick up a newer certificate, and order one where
// none is current or the renewal time has come, under the lease and the backoff of the events.
func (m *Manager) Maintain(ctx context.Context) error {
	var errs []error
	for _, h := range m.Hosts {
		errs = append(errs, m.maintain(ctx, h))
	}
	return errors.Join(errs...)
}

func (m *Manager) due(ctx context.Context, host string) (bool, *entry) {
	m.mu.RLock()
	e := m.certs[host]
	m.mu.RUnlock()
	if e == nil {
		return true, nil
	}
	return !m.Clock.Now().Before(m.renewAt(ctx, e)), e
}

func (m *Manager) maintain(ctx context.Context, host string) error {
	if err := m.reload(ctx, host); err != nil {
		return err
	}
	if due, _ := m.due(ctx, host); !due {
		return nil
	}
	next, _, err := m.Store.NextAttempt(ctx, host)
	if err != nil {
		return err
	}
	if m.Clock.Now().Before(next) {
		return nil
	}
	_, err = m.Lease.Hold(ctx, host, func(ctx context.Context) error {
		// Another process may have renewed while this one waited for the lease.
		if err := m.reload(ctx, host); err != nil {
			return err
		}
		due, cur := m.due(ctx, host)
		if !due {
			return nil
		}
		if next, _, err := m.Store.NextAttempt(ctx, host); err != nil || m.Clock.Now().Before(next) {
			return err
		}
		return m.order(ctx, host, cur)
	})
	return err
}

func (m *Manager) order(ctx context.Context, host string, cur *entry) error {
	acct, err := m.account(ctx)
	if err != nil {
		return err
	}
	replaces := ""
	var replacesLeaf *x509.Certificate
	if cur != nil {
		replaces, replacesLeaf = cur.id, cur.leaf
	}
	attempt, err := m.Store.RecordAttempt(ctx, host, m.Directory, replaces)
	if err != nil {
		return err
	}
	m.Orders.Add(1)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := acmez.NewCSR(key, []string{host})
	if err != nil {
		return err
	}
	params, err := acmez.OrderParametersFromCSR(acct, csr)
	if err != nil {
		return err
	}
	params.Profile, params.Replaces = m.Profile, replacesLeaf
	certs, err := m.client().ObtainCertificate(ctx, params)
	if err != nil {
		ptype := "client"
		var p acme.Problem
		if errors.As(err, &p) {
			ptype = p.Type
		}
		return errors.Join(err, m.Store.RecordFailure(context.WithoutCancel(ctx), attempt, ptype, err.Error()))
	}
	id, err := m.Store.InsertCertificate(ctx, attempt, host, m.Directory, certs[0].ChainPEM, key)
	if err != nil {
		return err
	}
	if err := m.reload(ctx, host); err != nil {
		return err
	}
	return m.notify(ctx, host, id)
}

func (m *Manager) notify(ctx context.Context, host, id string) error {
	if m.Bus == nil {
		return nil
	}
	b, _ := json.Marshal(Notice{Kind: "tls_certificate", Host: host, CertificateID: id})
	return m.Bus.Send(ctx, b)
}

// Listen reloads a host whenever a notice names it. When the listener breaks it may have missed
// notices, so every host is read again before listening anew. It returns when ctx ends.
func (m *Manager) Listen(ctx context.Context) {
	if m.Bus == nil {
		return
	}
	hosts := make(chan string, 64)
	for ctx.Err() == nil {
		l, err := m.Bus.Listen(ctx, func(p []byte) {
			var n Notice
			if json.Unmarshal(p, &n) == nil && n.Kind == "tls_certificate" {
				select {
				case hosts <- n.Host:
				default: // the periodic check catches up
				}
			}
		})
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		_ = m.LoadAll(ctx)
	loop:
		for {
			select {
			case <-ctx.Done():
				_ = l.Close(context.WithoutCancel(ctx))
				return
			case <-l.Broken():
				_ = l.Close(context.WithoutCancel(ctx))
				break loop
			case h := <-hosts:
				if err := m.reload(ctx, h); err != nil && m.Logger != nil {
					m.Logger.WarnContext(ctx, "reload certificate", slog.String("host", h), slog.Any("error", err))
				}
			}
		}
	}
}

// Leaf returns the leaf in memory for host, for tests.
func (m *Manager) Leaf(host string) *x509.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e := m.certs[host]; e != nil {
		return e.leaf
	}
	return nil
}

func (m *Manager) account(ctx context.Context) (acme.Account, error) {
	stored, err := m.Store.Account(ctx, m.Directory)
	if errors.Is(err, ErrNone) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		a := acme.Account{Contact: []string{"mailto:" + m.Email}, TermsOfServiceAgreed: true, PrivateKey: key}
		a, err = m.client().NewAccount(ctx, a)
		if err != nil {
			return acme.Account{}, err
		}
		if err := m.Store.InsertAccount(ctx, m.Directory, a.Location, key); err != nil {
			return acme.Account{}, err
		}
		stored, err = m.Store.Account(ctx, m.Directory)
	}
	if err != nil {
		return acme.Account{}, err
	}
	return m.client().GetAccount(ctx, acme.Account{Location: stored.URL, PrivateKey: stored.Key})
}

type http01Solver struct{ kv kernel.KeyValueStore }

func (s http01Solver) Present(ctx context.Context, ch acme.Challenge) error {
	return s.kv.Set(ctx, nsHTTP01, ch.Token, []byte(ch.KeyAuthorization), challenges)
}

func (s http01Solver) CleanUp(ctx context.Context, ch acme.Challenge) error {
	return s.kv.Delete(ctx, nsHTTP01, ch.Token)
}

type tlsALPNSolver struct{ kv kernel.KeyValueStore }

func (s tlsALPNSolver) Present(ctx context.Context, ch acme.Challenge) error {
	b, err := json.Marshal(ch)
	if err != nil {
		return err
	}
	return s.kv.Set(ctx, nsTLSALPN, strings.ToLower(ch.Identifier.Value), b, challenges)
}

func (s tlsALPNSolver) CleanUp(ctx context.Context, ch acme.Challenge) error {
	return s.kv.Delete(ctx, nsTLSALPN, strings.ToLower(ch.Identifier.Value))
}

var _ crypto.Signer = (*ecdsa.PrivateKey)(nil)
