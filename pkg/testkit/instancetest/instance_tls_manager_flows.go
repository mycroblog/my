// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"context"
	"crypto/tls"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// instanceTLSManagerCases are the cases of InstanceFlows about the TLSManager (ADR-0163 §3, §5,
// §7, §8): the first certificate of each host through either challenge, one order per host
// across the processes sharing the database and the KeyValueStore, the backoff after a failure,
// an order stopped when its lease is lost, and a restart that loads the stored certificate.
var instanceTLSManagerCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"ObtainsTheCertificateOfEachHost", func(t *testing.T, e *instanceEnv) { tlsManagerObtains(t, e, nil) }},
	{"ObtainsThroughHTTP01Alone", func(t *testing.T, e *instanceEnv) {
		tlsManagerObtains(t, e, []string{appinstance.ACMEChallengeHTTP01})
	}},
	{"PlacesOneOrderPerHostAcrossProcesses", tlsManagerOneOrderAcrossProcesses},
	{"BacksOffAfterAFailedOrder", tlsManagerBacksOff},
	{"StopsAnOrderThatLostItsLease", tlsManagerStopsOnLostLease},
	{"LoadsTheStoredCertificateWithoutOrdering", tlsManagerLoadsStored},
	{"OrdersAgainFromAnotherCA", tlsManagerOrdersFromAnotherCA},
}

const (
	tlsManagerHost    = "social.example"
	tlsManagerWebHost = "app.social.example"
)

// CertificateSink is an instance.TLSCertificateSink in memory.
type CertificateSink struct {
	mu    sync.Mutex
	certs map[string]*tls.Certificate
}

// Set implements instance.TLSCertificateSink.
func (s *CertificateSink) Set(host string, cert *tls.Certificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.certs == nil {
		s.certs = map[string]*tls.Certificate{}
	}
	s.certs[host] = cert
	return nil
}

// Certificate returns the certificate of host, or nil.
func (s *CertificateSink) Certificate(host string) *tls.Certificate {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.certs[host]
}

// tlsProcess is a process of the server: its own service and manager over the shared database,
// KeyValueStore, and CA.
type tlsProcess struct {
	manager *appinstance.TLSManager
	sink    *CertificateSink
}

func (e *instanceEnv) tlsProcess(t *testing.T, ca *FakeACMEClient, kv kernel.KeyValueStore, mod func(*appinstance.TLSManagerOptions)) tlsProcess {
	t.Helper()
	service := appinstance.NewService(appinstance.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Instance, Clock: e.clock, IDs: e.ids, Sealer: e.sealer,
	})
	sink := &CertificateSink{}
	opts := appinstance.TLSManagerOptions{
		Service: service, Client: ca, KV: kv, Certificates: sink, Hosts: []string{tlsManagerHost, tlsManagerWebHost},
		Email: "ops@social.example", Clock: e.clock,
	}
	if mod != nil {
		mod(&opts)
	}
	m, err := appinstance.NewTLSManager(opts)
	if err != nil {
		t.Fatal(err)
	}
	return tlsProcess{manager: m, sink: sink}
}

// newTLSCA returns a fake CA that validates through the answers of challenges, and a
// KeyValueStore for them.
func (e *instanceEnv) newTLSCA(t *testing.T) (*FakeACMEClient, kernel.KeyValueStore) {
	t.Helper()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	kv := testkit.NewKeyValueStore(e.clock)
	ca.Validate = ChallengeValidator(appinstance.NewACMEChallenges(kv))
	return ca, kv
}

// checkServes checks that p serves a certificate of the CA for host, the one the database holds.
func (e *instanceEnv) checkServes(t *testing.T, p tlsProcess, host string) {
	t.Helper()
	c := p.sink.Certificate(host)
	if c == nil {
		t.Fatalf("no certificate of %s in memory", host)
	}
	if err := c.Leaf.VerifyHostname(host); err != nil {
		t.Errorf("the certificate of %s: %v", host, err)
	}
	stored, err := e.service.CurrentTLSCertificateID(t.Context(), host)
	if err != nil || stored != p.manager.CertificateID(host) {
		t.Errorf("the current certificate of %s is %q, %v; the one in memory %q", host, stored, err, p.manager.CertificateID(host))
	}
}

func tlsManagerObtains(t *testing.T, e *instanceEnv, challenges []string) {
	ctx := t.Context()
	ca, kv := e.newTLSCA(t)
	var presented []string
	var mu sync.Mutex
	validate := ca.Validate
	ca.Validate = func(ctx context.Context, ch appinstance.ACMEChallenge) error {
		mu.Lock()
		presented = append(presented, ch.Type)
		mu.Unlock()
		return validate(ctx, ch)
	}
	p := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) { o.Challenges = challenges })
	next := p.manager.Maintain(ctx)
	for _, host := range []string{tlsManagerHost, tlsManagerWebHost} {
		e.checkServes(t, p, host)
	}
	if want := e.clock.Now().Add(appinstance.DefaultTLSCheckInterval); !next.Equal(want) {
		t.Errorf("next round at %v, want %v", next, want)
	}
	want := appinstance.ACMEChallengeTLSALPN01
	if len(challenges) > 0 {
		want = challenges[0]
	}
	if len(presented) != 2 || presented[0] != want || presented[1] != want {
		t.Errorf("challenges answered %v, want %s for each host", presented, want)
	}
	// Nothing is left to answer, and the next round orders nothing.
	for _, ns := range []string{appinstance.ACMEHTTP01Namespace, appinstance.ACMETLSALPN01Namespace} {
		for _, host := range []string{tlsManagerHost, tlsManagerWebHost} {
			if _, err := kv.Get(ctx, ns, host); !errors.Is(err, kernel.ErrKeyNotFound) {
				t.Errorf("an answer of %s is left in %s", host, ns)
			}
		}
	}
	p.manager.Maintain(ctx)
	if got := ca.Orders(); len(got) != 2 {
		t.Errorf("orders = %v, want one per host", got)
	}
	acct, err := e.service.TLSAccount(ctx, FakeACMEDirectory)
	if err != nil || !strings.HasPrefix(acct.Record.AccountURL, "https://acme.fake.test/account/") {
		t.Errorf("the ACME account = %+v, %v", acct.Record, err)
	}
}

func tlsManagerOneOrderAcrossProcesses(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca, kv := e.newTLSCA(t)
	// An order takes a while, so that the rounds of the processes overlap.
	ca.OnOrder = func(context.Context, string) { time.Sleep(50 * time.Millisecond) }
	processes := make([]tlsProcess, 4)
	for i := range processes {
		processes[i] = e.tlsProcess(t, ca, kv, nil)
	}
	var wg sync.WaitGroup
	for _, p := range processes {
		wg.Go(func() { p.manager.Maintain(ctx) })
	}
	wg.Wait()
	got := ca.Orders()
	slices.Sort(got)
	if want := []string{tlsManagerWebHost, tlsManagerHost}; !slices.Equal(got, want) {
		t.Fatalf("orders = %v, want one per host: %v", got, want)
	}
	for _, host := range []string{tlsManagerHost, tlsManagerWebHost} {
		if attempts, err := e.h.Instance.ListUnresultedTLSOrderAttempts(ctx, host, time.Time{}); err != nil || len(attempts) != 0 {
			t.Errorf("attempts of %s without a certificate = %v, %v", host, attempts, err)
		}
	}
	// The next round of each process loads what the others obtained, without an order.
	for _, p := range processes {
		p.manager.Maintain(ctx)
		for _, host := range []string{tlsManagerHost, tlsManagerWebHost} {
			e.checkServes(t, p, host)
		}
	}
	if orders := ca.Orders(); len(orders) != 2 {
		t.Errorf("orders = %v, want one per host", orders)
	}
}

func tlsManagerBacksOff(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca, kv := e.newTLSCA(t)
	ca.FailOrders(&appinstance.ACMEProblem{Type: "urn:ietf:params:acme:error:rateLimited", Detail: "too many"})
	p := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) { o.Hosts = []string{tlsManagerHost} })
	start := e.clock.Now()
	if next := p.manager.Maintain(ctx); !next.Equal(start.Add(appinstance.DefaultTLSRetryInterval)) {
		t.Errorf("next round after a failure at %v, want in a minute", next)
	}
	b, err := e.service.TLSOrderBackoff(ctx, tlsManagerHost)
	if err != nil || b.Failures != 1 || !b.NotBefore.Equal(start.Add(time.Minute)) {
		t.Fatalf("backoff = %+v, %v; want 1 failure until %v", b, err, start.Add(time.Minute))
	}
	// Within the backoff, a round orders nothing, from this process or a restarted one.
	e.clock.Advance(30 * time.Second)
	restarted := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) { o.Hosts = []string{tlsManagerHost} })
	if next := restarted.manager.Maintain(ctx); !next.Equal(b.NotBefore) {
		t.Errorf("next round within the backoff at %v, want %v", next, b.NotBefore)
	}
	p.manager.Maintain(ctx)
	if got := ca.Orders(); len(got) != 1 {
		t.Fatalf("orders within the backoff = %v, want 1", got)
	}
	// After it, the next order goes out; the second failure waits 10 minutes.
	e.clock.Advance(30 * time.Second)
	p.manager.Maintain(ctx)
	if got := ca.Orders(); len(got) != 2 {
		t.Fatalf("orders after the backoff = %v, want 2", got)
	}
	if b, _ := e.service.TLSOrderBackoff(ctx, tlsManagerHost); b.Failures != 2 || !b.NotBefore.Equal(e.clock.Now().Add(10*time.Minute)) {
		t.Errorf("backoff after 2 failures = %+v, want 10 minutes", b)
	}
	ca.FailOrders(nil)
	e.clock.Advance(10 * time.Minute)
	p.manager.Maintain(ctx)
	e.checkServes(t, p, tlsManagerHost)
	if b, _ := e.service.TLSOrderBackoff(ctx, tlsManagerHost); b.Failures != 0 {
		t.Errorf("backoff after a certificate = %+v, want none", b)
	}
}

func tlsManagerStopsOnLostLease(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca, kv := e.newTLSCA(t)
	ca.OnOrder = func(ctx context.Context, host string) {
		// Another process took the lease over: the next extension fails, and the order stops.
		_ = kv.Set(ctx, appinstance.TLSOrderLeaseNamespace, host, []byte("another"), time.Hour)
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
			t.Error("the order went on after its lease was lost")
		}
	}
	p := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) {
		o.Hosts = []string{tlsManagerHost}
		o.LeaseRenewal = 10 * time.Millisecond
	})
	p.manager.Maintain(ctx)
	if c := p.sink.Certificate(tlsManagerHost); c != nil {
		t.Error("an order that lost its lease installed a certificate")
	}
	b, err := e.service.TLSOrderBackoff(ctx, tlsManagerHost)
	if err != nil || b.Failures != 1 {
		t.Fatalf("backoff = %+v, %v; want the stopped order counted", b, err)
	}
	if v, err := kv.Get(ctx, appinstance.TLSOrderLeaseNamespace, tlsManagerHost); err != nil || string(v) != "another" {
		t.Errorf("the other process's lease = %q, %v; want it kept", v, err)
	}
}

func tlsManagerLoadsStored(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca, kv := e.newTLSCA(t)
	first := e.tlsProcess(t, ca, kv, nil)
	first.manager.Maintain(ctx)
	restarted := e.tlsProcess(t, ca, kv, nil)
	if err := restarted.manager.ReloadAll(ctx); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{tlsManagerHost, tlsManagerWebHost} {
		e.checkServes(t, restarted, host)
		if restarted.manager.CertificateID(host) != first.manager.CertificateID(host) {
			t.Errorf("the restarted process serves %s with another certificate", host)
		}
	}
	restarted.manager.Maintain(ctx)
	if got := ca.Orders(); len(got) != 2 {
		t.Errorf("orders = %v, want only the first process's", got)
	}
}

func tlsManagerOrdersFromAnotherCA(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	staging, kv := e.newTLSCA(t)
	staging.Directory = "https://acme-staging.fake.test/directory"
	first := e.tlsProcess(t, staging, kv, func(o *appinstance.TLSManagerOptions) { o.Hosts = []string{tlsManagerHost} })
	first.manager.Maintain(ctx)
	stagingID := first.manager.CertificateID(tlsManagerHost)
	// The operator moves to the production CA: the staging certificate is served until the
	// production one comes, which does not replace it.
	production, _ := e.newTLSCA(t)
	production.Validate = staging.Validate
	moved := e.tlsProcess(t, production, kv, func(o *appinstance.TLSManagerOptions) { o.Hosts = []string{tlsManagerHost} })
	if err := moved.manager.ReloadAll(ctx); err != nil || moved.manager.CertificateID(tlsManagerHost) != stagingID {
		t.Fatalf("the moved process loads %q, %v; want the staging certificate %q", moved.manager.CertificateID(tlsManagerHost), err, stagingID)
	}
	moved.manager.Maintain(ctx)
	if got := production.Orders(); len(got) != 1 {
		t.Fatalf("orders from the production CA = %v, want 1", got)
	}
	e.checkServes(t, moved, tlsManagerHost)
	if err := moved.sink.Certificate(tlsManagerHost).Leaf.CheckSignatureFrom(production.root); err != nil {
		t.Errorf("the served certificate is not the production CA's: %v", err)
	}
	attempts, err := e.h.Instance.ListUnresultedTLSOrderAttempts(ctx, tlsManagerHost, time.Time{})
	if err != nil || len(attempts) != 0 {
		t.Errorf("attempts without a certificate = %v, %v", attempts, err)
	}
	moved.manager.Maintain(ctx)
	if got := production.Orders(); len(got) != 1 {
		t.Errorf("orders from the production CA after its certificate = %v, want 1", got)
	}
}
