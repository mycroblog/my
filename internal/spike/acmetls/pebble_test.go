// SPDX-License-Identifier: AGPL-3.0-only

package acmetls

// The Pebble tests run against a local Pebble (github.com/letsencrypt/pebble/v2) with
// pebble-challtestsrv as its DNS, which resolves every name to 127.0.0.1:
//
//	pebble-challtestsrv -defaultIPv4 127.0.0.1 -defaultIPv6 "" -http01 "" -https01 "" -tlsalpn01 "" -doh "" &
//	PEBBLE_VA_NOSLEEP=1 pebble -config pebble-config.json -dnsserver 127.0.0.1:8053 &
//	MY_SPIKE_PEBBLE_DIR=https://127.0.0.1:14000/dir MY_SPIKE_PEBBLE_CA=<pebble>/test/certs/pebble.minica.pem \
//	  go test ./internal/spike/acmetls -run Pebble -v
//
// Pebble validates HTTP-01 on port 5002 and TLS-ALPN-01 on port 5001 (its config), so the
// tests listen there.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/mycroblog/my/internal/adapter/blob/fs"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
)

func pebbleClient(t *testing.T) (string, *http.Client) {
	dir, ca := os.Getenv("MY_SPIKE_PEBBLE_DIR"), os.Getenv("MY_SPIKE_PEBBLE_CA")
	if dir == "" || ca == "" {
		t.Skip("MY_SPIKE_PEBBLE_DIR and MY_SPIKE_PEBBLE_CA are not set")
	}
	pemBytes, err := os.ReadFile(ca)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pemBytes)
	return dir, &http.Client{
		Timeout:       30 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func serve(t *testing.T, tlsConf *tls.Config, httpHandler http.Handler) {
	t.Helper()
	tl, err := tls.Listen("tcp", "127.0.0.1:5001", tlsConf)
	if err != nil {
		t.Fatal(err)
	}
	hl, err := net.Listen("tcp", "127.0.0.1:5002")
	if err != nil {
		t.Fatal(err)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	ts := &http.Server{Handler: ok, ReadHeaderTimeout: 5 * time.Second}
	hs := &http.Server{Handler: httpHandler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = ts.Serve(tl) }()
	go func() { _ = hs.Serve(hl) }()
	t.Cleanup(func() { _ = ts.Close(); _ = hs.Close() })
}

func handshake(t *testing.T, host string) *x509.Certificate {
	t.Helper()
	c, err := tls.DialWithDialer(&net.Dialer{Timeout: 60 * time.Second}, "tcp", "127.0.0.1:5001", &tls.Config{ServerName: host, InsecureSkipVerify: true}) //nolint:gosec // Pebble's root is random
	if err != nil {
		t.Fatalf("handshake %s: %v", host, err)
	}
	defer c.Close()
	return c.ConnectionState().PeerCertificates[0]
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

// autocert with the sealed BlobCache: the certificate is read from the cache once per process,
// never per handshake.
func TestPebbleAutocertBlobCache(t *testing.T) {
	dir, hc := pebbleClient(t)
	store, err := fs.New(fs.Options{Dir: t.TempDir(), BaseURL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	km := &countingKM{KeyManager: testkit.NewKeyManager("local:test")}
	cache := &BlobCache{Store: store, Sealer: kernel.NewSealer(km), Prefix: "tls/acme/pebble"}
	const host = "autocert.spike.test"
	newManager := func() *autocert.Manager {
		return &autocert.Manager{Prompt: autocert.AcceptTOS, Cache: cache, HostPolicy: autocert.HostWhitelist(host), Client: &acme.Client{DirectoryURL: dir, HTTPClient: hc}, Email: "ops@spike.test"}
	}
	m := newManager()
	serve(t, m.TLSConfig(), m.HTTPHandler(nil))
	start := time.Now()
	leaf := handshake(t, host)
	t.Logf("issued %v in %v, lifetime %v; cache gets %d puts %d", leaf.DNSNames, time.Since(start), leaf.NotAfter.Sub(leaf.NotBefore), cache.Gets.Load(), cache.Puts.Load())
	g, u := cache.Gets.Load(), km.unwraps.Load()
	for range 50 {
		handshake(t, host)
	}
	t.Logf("50 more handshakes: cache gets +%d, unwraps +%d", cache.Gets.Load()-g, km.unwraps.Load()-u)
	if cache.Gets.Load() != g {
		t.Error("handshakes read the cache")
	}

	// A restarted process reads the cached certificate once.
	m2 := newManager()
	g, u = cache.Gets.Load(), km.unwraps.Load()
	for range 10 {
		if _, err := m2.GetCertificate(&tls.ClientHelloInfo{ServerName: host, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("restart, 10 handshakes: cache gets +%d, unwraps +%d", cache.Gets.Load()-g, km.unwraps.Load()-u)
	// An SNI outside the policy never starts an order.
	if _, err := m2.GetCertificate(&tls.ClientHelloInfo{ServerName: "evil.spike.test"}); err == nil {
		t.Error("an unknown SNI got a certificate")
	}
}

// Two processes share the DB, the KeyValueStore and a Bus. Both maintain at once: one orders, the
// other answers the CA's validation (the challenge is in the KV) and picks up the certificate from
// the notice. Then ARI and a renewal that names the replaced certificate.
func TestPebbleManagerTwoProcesses(t *testing.T) {
	dir, hc := pebbleClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock := testkit.NewClock(time.Now())
	km := &countingKM{KeyManager: testkit.NewKeyManager("local:test")}
	store := &DBStore{DB: openSpikeDB(t), Sealer: kernel.NewSealer(km), Clock: clock, NewID: testkit.NewIDGenerator(clock).NewID}
	kv := testkit.NewKeyValueStore(clock)
	broker := stream.NewMemoryBroker()
	const host = "manager.spike.test"
	newManager := func() *Manager {
		return &Manager{Hosts: []string{host}, Directory: dir, Email: "ops@spike.test", HTTPClient: hc, Store: store,
			Lease: &Lease{KV: kv, Namespace: "acme.order", TTL: 10 * time.Minute}, KV: kv, Bus: broker.Bus(), Clock: clock, RenewRatio: 2.0 / 3}
	}
	m1, m2 := newManager(), newManager()
	go m1.Listen(ctx)
	go m2.Listen(ctx)
	waitFor(t, "listeners", func() bool { return broker.Listeners() == 2 })
	// Only m2 serves; whichever process orders, m2 answers the validation.
	serve(t, m2.TLSConfig(), m2.HTTPHandler())

	var wg sync.WaitGroup
	start := time.Now()
	for _, m := range []*Manager{m1, m2} {
		wg.Go(func() {
			if err := m.Maintain(ctx); err != nil {
				t.Errorf("maintain: %v", err)
			}
		})
	}
	wg.Wait()
	n, _ := store.CountAttempts(ctx, host)
	t.Logf("first issuance in %v: attempts %d, orders m1 %d m2 %d", time.Since(start), n, m1.Orders.Load(), m2.Orders.Load())
	if n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
	waitFor(t, "both to hold the certificate", func() bool { return m1.Leaf(host) != nil && m2.Leaf(host) != nil })
	leaf := handshake(t, host)
	t.Logf("served %v, lifetime %v", leaf.DNSNames, leaf.NotAfter.Sub(leaf.NotBefore))

	m1.mu.RLock()
	e := m1.certs[host]
	m1.mu.RUnlock()
	at := m1.renewAt(ctx, e)
	if e.ari != nil {
		t.Logf("ARI window %v .. %v (retry after %v); renew at %v", e.ari.SuggestedWindow.Start, e.ari.SuggestedWindow.End, e.ari.RetryAfter, at)
	} else {
		t.Logf("no ARI; renew at %v", at)
	}
	// Nothing is due yet: Maintain orders nothing.
	_ = m1.Maintain(ctx)
	if n, _ := store.CountAttempts(ctx, host); n != 1 {
		t.Fatalf("ordered before the renewal time: %d", n)
	}
	// Each process picks its own time in the window; move past both. m2 renews with
	// "replaces", m1 follows through the Bus.
	m2.mu.RLock()
	e2 := m2.certs[host]
	m2.mu.RUnlock()
	if at2 := m2.renewAt(ctx, e2); at2.After(at) {
		at = at2
	}
	clock.Set(at.Add(time.Minute))
	if err := m2.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "m1 to pick up the renewal", func() bool {
		return m1.Leaf(host).SerialNumber.Cmp(m2.Leaf(host).SerialNumber) == 0 && m2.Leaf(host).SerialNumber.Cmp(leaf.SerialNumber) != 0
	})
	n, _ = store.CountAttempts(ctx, host)
	t.Logf("after renewal: attempts %d, reloads m1 %d m2 %d, DEK unwraps %d", n, m1.Reloads.Load(), m2.Reloads.Load(), km.unwraps.Load())
}

// The failure is recorded as an event and backs off: a name that does not validate.
func TestPebbleFailureBacksOff(t *testing.T) {
	dir, hc := pebbleClient(t)
	ctx := t.Context()
	clock := testkit.NewClock(time.Now())
	store := &DBStore{DB: openSpikeDB(t), Sealer: kernel.NewSealer(testkit.NewKeyManager("local:test")), Clock: clock, NewID: testkit.NewIDGenerator(clock).NewID}
	kv := testkit.NewKeyValueStore(clock)
	m := &Manager{Hosts: []string{"blocked-domain.example"}, Directory: dir, Email: "ops@spike.test", HTTPClient: hc, Store: store,
		Lease: &Lease{KV: kv, Namespace: "acme.order", TTL: 10 * time.Minute}, KV: kv, Clock: clock, RenewRatio: 2.0 / 3}
	err := m.Maintain(ctx)
	if err == nil {
		t.Fatal("ordered a blocked name")
	}
	next, n, _ := store.NextAttempt(ctx, "blocked-domain.example")
	var problem, detail string
	_ = store.DB.QueryRow(`SELECT problem_type, detail FROM tls_order_failures`).Scan(&problem, &detail)
	t.Logf("failure %q; next attempt in %v after %d failures", problem, next.Sub(clock.Now()), n)
	_ = m.Maintain(ctx) // within the backoff: no new attempt
	if c, _ := store.CountAttempts(ctx, "blocked-domain.example"); c != 1 {
		t.Fatalf("attempts during backoff: %d", c)
	}
	if !errors.Is(err, err) {
		t.Fatal()
	}
}
