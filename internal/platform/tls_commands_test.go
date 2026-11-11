// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/stream"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/pebbletest"
)

// commandApp builds the App of a my command over the settings of env: a process that does not
// listen, as `my tls` runs it.
func commandApp(t *testing.T, env map[string]string, opts platform.AppOptions) *platform.App {
	t.Helper()
	env["MY_ROLE"] = "worker"
	opts.Logger = slog.New(slog.DiscardHandler)
	app, err := platform.NewApp(t.Context(), acmeConfig(t, env, false), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// awaitNoCertificate waits until addr presents no certificate for host.
func awaitNoCertificate(t *testing.T, addr, host string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		chain, err := handshake(t.Context(), addr, host)
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still serves %s with the certificate %s", addr, host, chain[0].SerialNumber)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// After my tls revoke, every process stops serving the revoked certificate: the process that
// revokes notifies them on the Bus, here to a process whose rounds come once an hour. They serve
// the replacement, or none when its order failed and no certificate of the host is left.
func TestTLSRevokeReachesEveryProcess(t *testing.T) {
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
	start(secondLn, slowRounds(ca))
	addrs := []string{firstLn.Addr().String(), secondLn.Addr().String()}
	old := first.app.TLSCertificates.Certificate(acmeHost).Leaf
	awaitSerial(t, addrs[1], acmeHost, old.SerialNumber, 10*time.Second)

	cli := commandApp(t, sharedEnv(db, kek, nil), platform.AppOptions{ACMEClient: ca, StreamBus: broker.Bus()})
	res, err := cli.RevokeTLS(t.Context(), acmeHost, domaininstance.TLSRevocationKeyCompromise)
	if err != nil || res.ReplacementErr != nil {
		t.Fatalf("RevokeTLS = %+v, %v", res, err)
	}
	if res.Revoked.Serial != old.SerialNumber.Text(16) {
		t.Errorf("revoked %s, want %s", res.Revoked.Serial, old.SerialNumber.Text(16))
	}
	current, err := cli.Instance.CurrentTLSCertificateRecord(t.Context(), acmeHost)
	if err != nil || current.ID != res.ReplacementID {
		t.Fatalf("the current certificate is %+v, %v; want the replacement %s", current, err, res.ReplacementID)
	}
	replacement, _ := new(big.Int).SetString(current.Serial, 16)
	for _, addr := range addrs {
		awaitSerial(t, addr, acmeHost, replacement, 10*time.Second)
	}

	// The replacement cannot be ordered: the hosts' certificates are all revoked, and no
	// process serves one.
	ca.FailOrders(errors.New("the CA is down"))
	res, err = cli.RevokeTLS(t.Context(), acmeHost, domaininstance.TLSRevocationOperator)
	if err != nil || res.ReplacementErr == nil {
		t.Fatalf("RevokeTLS with the CA down = %+v, %v; want a revocation without a replacement", res, err)
	}
	for _, addr := range addrs {
		awaitNoCertificate(t, addr, acmeHost, 10*time.Second)
	}
	// The other host is untouched.
	awaitSerial(t, addrs[1], acmeWebHost, first.app.TLSCertificates.Certificate(acmeWebHost).Leaf.SerialNumber, time.Second)
}

// The certificates in memory report the time they have left, with tls = acme and tls = files,
// and the orders count by result.
func TestTLSMetrics(t *testing.T) {
	t.Parallel()
	t.Run("acme", func(t *testing.T) {
		t.Parallel()
		tlsLn := listenOn(t, 0)
		ca := tlsALPNCA(t, tlsLn)
		meter := testkit.NewMeter()
		s := startACMEApp(t, acmeConfig(t, map[string]string{}, false),
			platform.AppOptions{ACMEClient: ca, Meter: meter, TLSRetryInterval: 100 * time.Millisecond, TLSCheckInterval: 200 * time.Millisecond}, tlsLn, nil)
		leaf := awaitCertificate(t, tlsLn.Addr().String(), acmeHost, ca.Roots())
		awaitCertificate(t, tlsLn.Addr().String(), acmeWebHost, ca.Roots())
		checkExpiry(t, meter, s.app, acmeHost, leaf.NotAfter)
		issued := map[string]string{instance.LabelTLSHost: acmeHost, instance.LabelTLSResult: instance.TLSOrderIssued}
		if got := meter.Sum(instance.TLSOrderAttempts.Name, issued); got != 1 {
			t.Errorf("issued orders of %s = %v, want 1", acmeHost, got)
		}
		if err := meter.Err(); err != nil {
			t.Error(err)
		}
	})
	t.Run("files", func(t *testing.T) {
		t.Parallel()
		cert := testkit.NewTLSCertificate(t, 7, time.Now().Add(-time.Hour), 30*24*time.Hour, acmeHost)
		certFile, keyFile := cert.WriteFiles(t, t.TempDir(), time.Time{})
		cfg := loadConfig(t, map[string]string{
			"MY_DATABASE_URL": newDatabase(t, true), "MY_TLS": "files", "MY_TLS_CERT_FILE": certFile, "MY_TLS_KEY_FILE": keyFile,
			"MY_TLS_HTTP_LISTEN": "",
		})
		meter := testkit.NewMeter()
		tlsLn := listenOn(t, 0)
		s := startApp(t, cfg, platform.AppOptions{Meter: meter}, platform.ServeOptions{Listener: tlsLn}, nil)
		leaf := awaitCertificate(t, tlsLn.Addr().String(), acmeHost, cert.Pool())
		checkExpiry(t, meter, s.app, acmeHost, leaf.NotAfter)
	})
}

// checkExpiry checks the expiry gauge of host against notAfter.
func checkExpiry(t *testing.T, meter *testkit.Meter, app *platform.App, host string, notAfter time.Time) {
	t.Helper()
	if err := meter.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	ms := meter.Measurements(instance.TLSCertificateExpiry.Name, map[string]string{instance.LabelTLSHost: host})
	if len(ms) == 0 {
		t.Fatalf("no %s of %s", instance.TLSCertificateExpiry.Name, host)
	}
	want := notAfter.Sub(app.Clock.Now()).Seconds()
	if got := ms[len(ms)-1].Value; got > want+5 || got < want-60 {
		t.Errorf("%s of %s = %v, want about %v", instance.TLSCertificateExpiry.Name, host, got, want)
	}
	if i, ok := meter.Instrument(instance.TLSCertificateExpiry.Name); !ok || i.Unit != "s" {
		t.Errorf("the instrument is %+v", i)
	}
}

// my tls status, renew, and revoke against Pebble (TEST_ACME=pebble): the command orders through
// the KV that the serving process answers the challenges from, and the serving process loads
// what it obtains by its periodic check (stream = hub).
func TestPebbleTLSCommands(t *testing.T) {
	t.Parallel()
	tlsLn := listenOn(t, 0)
	srv := pebbletest.New(t, pebbletest.Options{TLSPort: portOf(tlsLn)})
	db, kek := newDatabase(t, true), randomKEK(t)
	client := pebbleClient(t, srv)
	roots := srv.Roots(t)
	addr := tlsLn.Addr().String()
	startACMEApp(t, acmeConfig(t, sharedEnv(db, kek, nil), false),
		platform.AppOptions{ACMEClient: client, TLSRetryInterval: 100 * time.Millisecond, TLSCheckInterval: 200 * time.Millisecond}, tlsLn, nil)
	first := awaitCertificate(t, addr, acmeHost, roots)
	awaitCertificate(t, addr, acmeWebHost, roots)
	cli := commandApp(t, sharedEnv(db, kek, nil), platform.AppOptions{ACMEClient: client})

	st, err := cli.TLSStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Hosts) != 2 || st.Hosts[0].Host != acmeHost {
		t.Fatalf("status of %d hosts: %+v", len(st.Hosts), st.Hosts)
	}
	h := st.Hosts[0]
	if h.Leaf == nil || h.Leaf.SerialNumber.Cmp(first.SerialNumber) != 0 || !strings.Contains(h.Leaf.Issuer.String(), "Pebble") {
		t.Errorf("status shows the certificate %+v, want %s from Pebble", h.Leaf, first.SerialNumber)
	}
	if h.RenewalErr != nil || h.Renewal.WindowStart.IsZero() || !h.Renewal.WindowEnd.After(h.Renewal.WindowStart) {
		t.Errorf("status shows the renewal %+v, %v; want Pebble's window", h.Renewal, h.RenewalErr)
	}
	if len(h.Orders) != 1 || h.Orders[0].CertificateID != h.Certificate.ID || h.Backoff.Failures != 0 {
		t.Errorf("status shows the orders %+v and %d failures, want the one that issued the certificate", h.Orders, h.Backoff.Failures)
	}

	renewed, err := cli.RenewTLS(t.Context(), acmeHost)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := new(big.Int).SetString(renewed.Serial, 16)
	awaitSerial(t, addr, acmeHost, serial, 10*time.Second)

	res, err := cli.RevokeTLS(t.Context(), acmeHost, domaininstance.TLSRevocationKeyCompromise)
	if err != nil || res.ReplacementErr != nil {
		t.Fatalf("RevokeTLS = %+v, %v", res, err)
	}
	if res.Revoked.ID != renewed.ID || !srv.Revoked(t, serial) {
		t.Errorf("revoked %s; Pebble revoked the certificate: %t", res.Revoked.ID, srv.Revoked(t, serial))
	}
	replacement, err := cli.Instance.CurrentTLSCertificateRecord(t.Context(), acmeHost)
	if err != nil || replacement.ID != res.ReplacementID {
		t.Fatalf("the current certificate is %+v, %v; want the replacement", replacement, err)
	}
	serial, _ = new(big.Int).SetString(replacement.Serial, 16)
	awaitSerial(t, addr, acmeHost, serial, 10*time.Second)
	if c := awaitCertificate(t, addr, acmeHost, roots); c.SerialNumber.Cmp(serial) != 0 {
		t.Errorf("serves %s, want the replacement %s", c.SerialNumber, serial)
	}
	st, err = cli.TLSStatus(t.Context())
	if err != nil || len(st.Hosts[0].Orders) != 3 {
		t.Errorf("status after the revocation: %+v, %v; want three orders", st, err)
	}
}
