// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"crypto/x509"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
	"github.com/mycroblog/my/pkg/testkit/pebbletest"
)

// tlsALPNCA returns a fake CA that validates TLS-ALPN-01 on ln, the listener of one process: the
// answers are in the KV that every process shares, so that it answers the orders of all.
func tlsALPNCA(t *testing.T, ln net.Listener) *instancetest.FakeACMEClient {
	t.Helper()
	ca := instancetest.NewFakeACMEClient(t)
	ca.Validate = func(ctx context.Context, ch instance.ACMEChallenge) error {
		return instancetest.DialTLSALPN01(ctx, ln.Addr().String(), ch)
	}
	return ca
}

// awaitSerial waits until addr presents the certificate serial for host.
func awaitSerial(t *testing.T, addr, host string, serial *big.Int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		chain, err := handshake(t.Context(), addr, host)
		if err == nil && chain[0].SerialNumber.Cmp(serial) == 0 {
			return
		}
		if time.Now().After(deadline) {
			got := "none"
			if err == nil {
				got = chain[0].SerialNumber.String()
			}
			t.Fatalf("%s serves %s with the certificate %s (%v), want %s within %v", addr, host, got, err, serial, within)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// renew orders a certificate of host now in s, and returns the serial of the certificate it serves.
func renew(t *testing.T, s *server, host string) *big.Int {
	t.Helper()
	if err := s.app.TLSManager.OrderNow(t.Context(), host); err != nil {
		t.Fatal(err)
	}
	c := s.app.TLSCertificates.Certificate(host)
	if c == nil {
		t.Fatalf("no certificate of %s after the order", host)
	}
	return c.Leaf.SerialNumber
}

// slowRounds are the options of a process whose rounds come once an hour: only a notice makes it
// load a certificate that another process obtained.
func slowRounds(client instance.ACMEClient) platform.AppOptions {
	return platform.AppOptions{ACMEClient: client, TLSCheckInterval: time.Hour, TLSRetryInterval: time.Hour}
}

// countingKMS counts the keys it unwraps.
type countingKMS struct {
	*testkit.KeyManager
	unwraps atomic.Int64
}

func (k *countingKMS) UnwrapKey(ctx context.Context, keyID string, wrapped, aad []byte) ([]byte, error) {
	k.unwraps.Add(1)
	return k.KeyManager.UnwrapKey(ctx, keyID, wrapped, aad)
}

// The handshakes read only the certificates in memory: 10,000 of them unwrap no key, while the
// load of each certificate unwraps one (ADR-0163 §6). The KMS sits behind no cache of the DEKs.
func TestServeTLSACMEHandshakesUnwrapNoKey(t *testing.T) {
	t.Parallel()
	tlsLn := listenOn(t, 0)
	const key = "projects/p/locations/global/keyRings/r/cryptoKeys/k"
	kms := &countingKMS{KeyManager: testkit.NewKeyManager("gcpkms:" + key)}
	cfg := acmeConfig(t, map[string]string{}, false)
	cfg.KeyManager, cfg.KMSKey, cfg.KEK, cfg.DEKCacheTTL = "gcpkms", key, nil, 0
	ca := tlsALPNCA(t, tlsLn)
	opts := platform.AppOptions{ACMEClient: ca, KeyManager: kms, TLSRetryInterval: 100 * time.Millisecond, TLSCheckInterval: 200 * time.Millisecond}
	s := startACMEApp(t, cfg, opts, tlsLn, nil)
	addr := tlsLn.Addr().String()
	for _, host := range []string{acmeHost, acmeWebHost} {
		awaitCertificate(t, addr, host, ca.Roots())
	}
	before := kms.unwraps.Load()
	var wg sync.WaitGroup
	var failed atomic.Int64
	for w := range 10 {
		wg.Go(func() {
			host := acmeHost
			if w%2 == 1 {
				host = acmeWebHost
			}
			for range 1000 {
				if _, err := handshake(t.Context(), addr, host); err != nil {
					failed.Add(1)
				}
			}
		})
	}
	wg.Wait()
	if n := failed.Load(); n != 0 {
		t.Errorf("%d handshakes failed", n)
	}
	if n := kms.unwraps.Load() - before; n != 0 {
		t.Errorf("10,000 handshakes unwrapped %d keys, want none", n)
	}
	// A renewal loads a certificate, and unwraps its key, in the process that renews it alone.
	before = kms.unwraps.Load()
	serial := renew(t, s, acmeHost)
	awaitSerial(t, addr, acmeHost, serial, time.Second)
	if n := kms.unwraps.Load() - before; n != 0 {
		t.Errorf("the renewing process unwrapped %d keys, want none: it holds the new key", n)
	}
}

// sharedEnv returns the settings of the processes that share the database db, its KEK, and the
// KV (kv = db), with the settings of extra.
func sharedEnv(db, kek string, extra map[string]string) map[string]string {
	env := map[string]string{"MY_DATABASE_URL": db, "MY_KEK": kek, "MY_KV": "db"}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// Two web processes on PostgreSQL with stream = postgres (pgmem in pull requests): a renewal in
// one is served by the other within seconds, by the tls_certificate signal on the Bus, although
// the rounds of the other come only once an hour.
func TestServeTLSACMERenewalReachesTheOtherProcess(t *testing.T) {
	t.Parallel()
	db, kek := newPostgresDatabase(t), randomKEK(t)
	pg := map[string]string{"MY_DATABASE": "postgres", "MY_STREAM": "postgres", "MY_ROLE": "web"}
	firstLn, secondLn := listenOn(t, 0), listenOn(t, 0)
	ca := tlsALPNCA(t, firstLn)
	first := startACME(t, sharedEnv(db, kek, pg), ca, firstLn, nil)
	for _, host := range []string{acmeHost, acmeWebHost} {
		awaitCertificate(t, firstLn.Addr().String(), host, ca.Roots())
	}
	second := startACMEApp(t, acmeConfig(t, sharedEnv(db, kek, pg), false), slowRounds(ca), secondLn, nil)
	if _, ok := second.app.Streams.(*stream.Relay); !ok {
		t.Fatalf("the streams are a %T, want a *stream.Relay", second.app.Streams)
	}
	awaitSerial(t, secondLn.Addr().String(), acmeHost, first.app.TLSCertificates.Certificate(acmeHost).Leaf.SerialNumber, 10*time.Second)
	// The listener of the second process may still be starting: a renewal before it is ready
	// reaches it when it is, as it loads every host then.
	serial := renew(t, first, acmeHost)
	awaitSerial(t, secondLn.Addr().String(), acmeHost, serial, 10*time.Second)
	serial = renew(t, first, acmeHost)
	awaitSerial(t, secondLn.Addr().String(), acmeHost, serial, 5*time.Second)
}

// On the Bus, a late tls_certificate signal of an older certificate does not bring it back, and a
// broken listener loads every host again: it may have missed the signals of the certificates that
// changed meanwhile.
func TestServeTLSACMENoticesOnTheBus(t *testing.T) {
	t.Parallel()
	db, kek := newDatabase(t, true), randomKEK(t)
	broker := stream.NewMemoryBroker()
	firstLn, secondLn := listenOn(t, 0), listenOn(t, 0)
	ca := tlsALPNCA(t, firstLn)
	start := func(ln net.Listener, opts platform.AppOptions) *server {
		opts.StreamBus = broker.Bus()
		return startACMEApp(t, acmeConfig(t, sharedEnv(db, kek, nil), false), opts, ln, nil)
	}
	first := start(firstLn, platform.AppOptions{ACMEClient: ca, TLSRetryInterval: 100 * time.Millisecond, TLSCheckInterval: 200 * time.Millisecond})
	for _, host := range []string{acmeHost, acmeWebHost} {
		awaitCertificate(t, firstLn.Addr().String(), host, ca.Roots())
	}
	oldID := first.app.TLSManager.CertificateID(acmeHost)
	second := start(secondLn, slowRounds(ca))
	addr := secondLn.Addr().String()
	serial := renew(t, first, acmeHost)
	awaitSerial(t, addr, acmeHost, serial, 10*time.Second)

	// A signal of the older certificate, late or repeated, leaves the newer one.
	late := []byte(`{"v":1,"o":"another","k":"tls_certificate","h":"` + acmeHost + `","c":"` + oldID + `"}`)
	if err := broker.Bus().Send(t.Context(), late); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	awaitSerial(t, addr, acmeHost, serial, time.Second)

	// A process without the Bus (a my command, or stream = hub) renews: no signal goes out, and
	// the second process loads the renewal when its listener breaks.
	hubLn := listenOn(t, 0)
	hub := startACMEApp(t, acmeConfig(t, sharedEnv(db, kek, nil), false), slowRounds(ca), hubLn, nil)
	serial = renew(t, hub, acmeWebHost)
	time.Sleep(200 * time.Millisecond)
	if c := second.app.TLSCertificates.Certificate(acmeWebHost); c.Leaf.SerialNumber.Cmp(serial) == 0 {
		t.Fatal("the renewal reached the other process without a signal")
	}
	broker.Break()
	awaitSerial(t, addr, acmeWebHost, serial, 10*time.Second)
}

// Without a Bus (stream = hub), the process that renews serves the renewal at once, and the other
// processes load it by their periodic check.
func TestServeTLSACMERenewalWithoutTheBus(t *testing.T) {
	t.Parallel()
	db, kek := newDatabase(t, true), randomKEK(t)
	firstLn, secondLn := listenOn(t, 0), listenOn(t, 0)
	ca := tlsALPNCA(t, firstLn)
	first := startACME(t, sharedEnv(db, kek, nil), ca, firstLn, nil)
	if _, ok := first.app.Streams.(*stream.Hub); !ok {
		t.Fatalf("the streams are a %T, want a *stream.Hub", first.app.Streams)
	}
	for _, host := range []string{acmeHost, acmeWebHost} {
		awaitCertificate(t, firstLn.Addr().String(), host, ca.Roots())
	}
	// The rounds of the second process come every 300 ms, and they order nothing: the
	// certificates are far from their renewal time.
	second := startACMEApp(t, acmeConfig(t, sharedEnv(db, kek, nil), false),
		platform.AppOptions{ACMEClient: ca, TLSCheckInterval: 300 * time.Millisecond, TLSRetryInterval: time.Hour}, secondLn, nil)
	awaitSerial(t, secondLn.Addr().String(), acmeHost, first.app.TLSCertificates.Certificate(acmeHost).Leaf.SerialNumber, 10*time.Second)
	if second.app.TLSManager.CertificateID(acmeHost) == "" {
		t.Fatal("the second process holds no certificate")
	}
	serial := renew(t, first, acmeHost)
	awaitSerial(t, firstLn.Addr().String(), acmeHost, serial, time.Second)
	awaitSerial(t, secondLn.Addr().String(), acmeHost, serial, 10*time.Second)
	if orders := ca.Orders(); len(orders) != 3 {
		t.Errorf("orders = %v, want the first two and the renewal", orders)
	}
}

// Against Pebble with ARI, the certificate is renewed inside the window that Pebble suggests,
// replacing the one before (TEST_ACME=pebble). The server's clock moves to the renewal time it
// picked.
func TestPebbleServeTLSACMERenewsInTheWindow(t *testing.T) {
	t.Parallel()
	tlsLn := listenOn(t, 0)
	srv := pebbletest.New(t, pebbletest.Options{TLSPort: portOf(tlsLn)})
	client := &replacesClient{ACMEClient: pebbleClient(t, srv)}
	clock := testkit.NewClock(time.Now())
	cfg := acmeConfig(t, map[string]string{}, false)
	s := startACMEApp(t, cfg, platform.AppOptions{
		ACMEClient: client, Clock: clock, TLSRetryInterval: 100 * time.Millisecond, TLSCheckInterval: 200 * time.Millisecond,
	}, tlsLn, nil)
	roots := srv.Roots(t)
	addr := tlsLn.Addr().String()
	old := awaitCertificate(t, addr, acmeHost, roots)
	info, err := client.RenewalInfo(t.Context(), old)
	if err != nil {
		t.Fatalf("Pebble offers no renewal information: %v", err)
	}
	var at time.Time
	deadline := time.Now().Add(10 * time.Second)
	for at.IsZero() {
		if time.Now().After(deadline) {
			t.Fatal("no renewal time")
		}
		time.Sleep(20 * time.Millisecond)
		at = s.app.TLSManager.RenewalTime(acmeHost)
	}
	if at.Before(info.WindowStart) || !at.Before(info.WindowEnd) {
		t.Fatalf("renewal time %v outside Pebble's window [%v, %v)", at, info.WindowStart, info.WindowEnd)
	}
	clock.Set(at.Add(time.Millisecond))
	renewed := awaitRenewal(t, addr, acmeHost, roots, old)
	if got := client.replaced(acmeHost); len(got) != 1 || got[0].Cmp(old.SerialNumber) != 0 {
		t.Errorf("the orders replaced %v, want %s once", got, old.SerialNumber)
	}
	if renewed.NotBefore.After(info.WindowEnd) {
		t.Errorf("the renewal was issued at %v, after the window", renewed.NotBefore)
	}
}

// awaitRenewal waits until addr presents a certificate for host other than old, which chains to
// roots.
func awaitRenewal(t *testing.T, addr, host string, roots *x509.CertPool, old *x509.Certificate) *x509.Certificate {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		c := awaitCertificate(t, addr, host, roots)
		if c.SerialNumber.Cmp(old.SerialNumber) != 0 {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still serves %s", host, old.SerialNumber)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// replacesClient records the certificates that the orders of each host replace.
type replacesClient struct {
	instance.ACMEClient
	mu       sync.Mutex
	replaces map[string][]*big.Int
}

func (c *replacesClient) Order(ctx context.Context, req instance.ACMEOrderRequest) (string, error) {
	if req.Replaces != nil {
		c.mu.Lock()
		if c.replaces == nil {
			c.replaces = map[string][]*big.Int{}
		}
		c.replaces[req.Host] = append(c.replaces[req.Host], req.Replaces.SerialNumber)
		c.mu.Unlock()
	}
	return c.ACMEClient.Order(ctx, req)
}

func (c *replacesClient) replaced(host string) []*big.Int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*big.Int(nil), c.replaces[host]...)
}
