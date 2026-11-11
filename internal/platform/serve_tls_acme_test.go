// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/acme"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
	"github.com/mycroblog/my/pkg/testkit/pebbletest"
)

// The hosts of the servers with tls = acme: host and the host of web_origin.
const (
	acmeHost    = "example.test"
	acmeWebHost = "app.example.test"
)

// listenOn listens on 127.0.0.1:port, a free port when 0.
func listenOn(t *testing.T, port int) net.Listener {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// startACME starts a server with tls = acme on tlsLn and, when not nil, httpLn, which orders from
// client. env adds settings.
func startACME(t *testing.T, env map[string]string, client instance.ACMEClient, tlsLn, httpLn net.Listener) *server {
	t.Helper()
	return startACMEApp(t, acmeConfig(t, env, httpLn != nil),
		platform.AppOptions{ACMEClient: client, TLSRetryInterval: 100 * time.Millisecond, TLSCheckInterval: 200 * time.Millisecond},
		tlsLn, httpLn)
}

// acmeConfig returns the configuration of a server with tls = acme and the settings of env, with
// port 80 when withHTTP is set.
func acmeConfig(t *testing.T, env map[string]string, withHTTP bool) *config.Config {
	t.Helper()
	env["MY_TLS"] = "acme"
	env["MY_TLS_ACME_AGREE_TERMS"] = "true"
	env["MY_WEB_ORIGIN"] = "https://" + acmeWebHost
	if env["MY_DATABASE_URL"] == "" {
		env["MY_DATABASE_URL"] = newDatabase(t, true)
	}
	if !withHTTP {
		env["MY_TLS_HTTP_LISTEN"] = ""
	}
	return loadConfig(t, env)
}

// startACMEApp starts a server of cfg, with tls = acme, on tlsLn and, when not nil, httpLn.
func startACMEApp(t *testing.T, cfg *config.Config, opts platform.AppOptions, tlsLn, httpLn net.Listener) *server {
	t.Helper()
	return startApp(t, cfg, opts, platform.ServeOptions{Listener: tlsLn, HTTPListener: httpLn}, nil)
}

// handshake returns the chain that addr presents for serverName, the leaf first.
func handshake(ctx context.Context, addr, serverName string) ([]*x509.Certificate, error) {
	d := tls.Dialer{Config: &tls.Config{
		ServerName: serverName, MinVersion: tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // G402: the caller verifies the leaf
	}}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	return conn.(*tls.Conn).ConnectionState().PeerCertificates, nil
}

// awaitCertificate waits until addr presents a certificate for host that chains to roots.
func awaitCertificate(t *testing.T, addr, host string, roots *x509.CertPool) *x509.Certificate {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		chain, err := handshake(t.Context(), addr, host)
		if err == nil {
			inter := x509.NewCertPool()
			for _, c := range chain[1:] {
				inter.AddCert(c)
			}
			if _, err := chain[0].Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: inter}); err != nil {
				t.Fatalf("the certificate of %s: %v", host, err)
			}
			return chain[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no certificate of %s: %v", host, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// With tls = acme, the server obtains the certificates of its hosts after it starts listening:
// the CA validates through the server's own listeners, on the TLS listener (TLS-ALPN-01) or on
// port 80 (HTTP-01). A server name outside the hosts gets no certificate and orders nothing.
func TestServeTLSACMEObtainsTheCertificates(t *testing.T) {
	t.Parallel()
	for _, challenge := range []string{instance.ACMEChallengeTLSALPN01, instance.ACMEChallengeHTTP01} {
		t.Run(challenge, func(t *testing.T) {
			t.Parallel()
			tlsLn, httpLn := listenOn(t, 0), listenOn(t, 0)
			ca := instancetest.NewFakeACMEClient(t)
			ca.ChallengeType = challenge
			var mu sync.Mutex
			var validated []string
			ca.Validate = func(ctx context.Context, ch instance.ACMEChallenge) error {
				mu.Lock()
				validated = append(validated, ch.Type)
				mu.Unlock()
				if ch.Type == instance.ACMEChallengeHTTP01 {
					return instancetest.GetHTTP01(ctx, httpLn.Addr().String(), ch)
				}
				return instancetest.DialTLSALPN01(ctx, tlsLn.Addr().String(), ch)
			}
			s := startACME(t, map[string]string{}, ca, tlsLn, httpLn)
			addr := tlsLn.Addr().String()
			for _, host := range []string{acmeHost, acmeWebHost} {
				awaitCertificate(t, addr, host, ca.Roots())
			}
			if _, err := handshake(t.Context(), addr, "other.test"); err == nil {
				t.Error("a server name outside the hosts got a certificate")
			}
			orders := ca.Orders()
			slices.Sort(orders)
			if want := []string{acmeWebHost, acmeHost}; !slices.Equal(orders, want) {
				t.Errorf("orders = %v, want %v", orders, want)
			}
			mu.Lock()
			if len(validated) != 2 || validated[0] != challenge || validated[1] != challenge {
				t.Errorf("validated %v, want %s for each host", validated, challenge)
			}
			mu.Unlock()
			if id := s.app.TLSManager.CertificateID(acmeHost); id == "" {
				t.Error("the manager holds no certificate of host")
			}
		})
	}
}

// A failed order is recorded, and the next one waits for the backoff: the rounds that follow
// order nothing.
func TestServeTLSACMEBacksOffAfterAFailure(t *testing.T) {
	t.Parallel()
	ca := instancetest.NewFakeACMEClient(t)
	ca.FailOrders(&instance.ACMEProblem{Type: "urn:ietf:params:acme:error:rateLimited", Detail: "too many"})
	s := startACME(t, map[string]string{}, ca, listenOn(t, 0), nil)
	deadline := time.Now().Add(10 * time.Second)
	for len(ca.Orders()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("orders = %v, want one per host", ca.Orders())
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Several rounds pass (100 ms apart) within the backoff of a minute.
	time.Sleep(500 * time.Millisecond)
	if orders := ca.Orders(); len(orders) != 2 {
		t.Errorf("orders = %v, want none during the backoff", orders)
	}
	for _, host := range []string{acmeHost, acmeWebHost} {
		b, err := s.app.Instance.TLSOrderBackoff(t.Context(), host)
		if err != nil || b.Failures != 1 || b.NotBefore.IsZero() {
			t.Errorf("backoff of %s = %+v, %v; want the failure recorded", host, b, err)
		}
	}
}

// onlyChallenge is an ACME client whose orders answer the challenge typ alone, as when the CA
// cannot reach the other listener.
type onlyChallenge struct {
	instance.ACMEClient
	typ string
}

func (c onlyChallenge) Order(ctx context.Context, req instance.ACMEOrderRequest) (string, error) {
	req.Challenges = []string{c.typ}
	return c.ACMEClient.Order(ctx, req)
}

// countingClient counts the orders of each host.
type countingClient struct {
	instance.ACMEClient
	mu     *sync.Mutex
	orders map[string]int
}

func (c countingClient) Order(ctx context.Context, req instance.ACMEOrderRequest) (string, error) {
	c.mu.Lock()
	c.orders[req.Host]++
	c.mu.Unlock()
	return c.ACMEClient.Order(ctx, req)
}

func pebbleClient(t *testing.T, srv pebbletest.Server) instance.ACMEClient {
	t.Helper()
	c, err := acme.New(acme.Options{DirectoryURL: srv.DirectoryURL, HTTPClient: srv.HTTPClient(), PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A fresh server obtains the certificates of its hosts from Pebble, through TLS-ALPN-01 without
// port 80, and through HTTP-01 when TLS-ALPN-01 cannot be answered (TEST_ACME=pebble).
func TestPebbleServeTLSACME(t *testing.T) {
	t.Parallel()
	t.Run("TLS-ALPN-01", func(t *testing.T) {
		t.Parallel()
		srv := pebbletest.New(t, pebbletest.Options{TLSPort: pebbletest.FreePort(t)})
		tlsLn := listenOn(t, srv.TLSPort)
		startACME(t, map[string]string{}, pebbleClient(t, srv), tlsLn, nil)
		roots := srv.Roots(t)
		for _, host := range []string{acmeHost, acmeWebHost} {
			awaitCertificate(t, tlsLn.Addr().String(), host, roots)
		}
		if _, err := handshake(t.Context(), tlsLn.Addr().String(), "other.test"); err == nil {
			t.Error("a server name outside the hosts got a certificate")
		}
	})
	t.Run("HTTP-01", func(t *testing.T) {
		t.Parallel()
		srv := pebbletest.New(t, pebbletest.Options{HTTPPort: pebbletest.FreePort(t)})
		tlsLn, httpLn := listenOn(t, 0), listenOn(t, srv.HTTPPort)
		client := onlyChallenge{ACMEClient: pebbleClient(t, srv), typ: instance.ACMEChallengeHTTP01}
		startACME(t, map[string]string{}, client, tlsLn, httpLn)
		roots := srv.Roots(t)
		for _, host := range []string{acmeHost, acmeWebHost} {
			awaitCertificate(t, tlsLn.Addr().String(), host, roots)
		}
	})
}

// Two processes share the database and the KV (kv = db): they place one order per host, and
// both serve its certificate. Only the first listens where Pebble validates; it answers the
// orders of the second from the KV.
func TestPebbleServeTLSACMETwoProcesses(t *testing.T) {
	t.Parallel()
	srv := pebbletest.New(t, pebbletest.Options{TLSPort: pebbletest.FreePort(t)})
	// The processes share the database and its key-encryption key.
	db := newDatabase(t, true)
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	var mu sync.Mutex
	orders := map[string]int{}
	client := countingClient{ACMEClient: pebbleClient(t, srv), mu: &mu, orders: orders}
	first, second := listenOn(t, srv.TLSPort), listenOn(t, 0)
	for _, ln := range []net.Listener{first, second} {
		s := startACME(t, map[string]string{"MY_DATABASE_URL": db, "MY_KV": "db", "MY_KEK": base64.StdEncoding.EncodeToString(kek)}, client, ln, nil)
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf("the server on %s:\n%s", ln.Addr(), s.logs.String())
			}
		})
	}
	roots := srv.Roots(t)
	for _, host := range []string{acmeHost, acmeWebHost} {
		a := awaitCertificate(t, first.Addr().String(), host, roots)
		b := awaitCertificate(t, second.Addr().String(), host, roots)
		if a.SerialNumber.Cmp(b.SerialNumber) != 0 {
			t.Errorf("the processes serve %s with different certificates", host)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if orders[acmeHost] != 1 || orders[acmeWebHost] != 1 || len(orders) != 2 {
		t.Errorf("orders = %v, want one per host", orders)
	}
}
