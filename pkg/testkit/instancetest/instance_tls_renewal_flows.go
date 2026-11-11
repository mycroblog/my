// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// instanceTLSRenewalCases are the cases of InstanceFlows about the renewals of the TLSManager and
// the certificates it loads (ADR-0163 §6, §7): the renewal time in the CA's window (ARI), kept
// while the window stays and moved with it, the lifetime rules without ARI, a renewal that
// replaces the certificate, one under a shorter profile, an order that skips the wait, and the
// loads of the notices of other processes.
var instanceTLSRenewalCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"RenewsInTheWindowReplacingTheCertificate", tlsManagerRenewsInTheWindow},
	{"KeepsTheRenewalTimeWhileTheWindowStays", tlsManagerKeepsTheRenewalTime},
	{"MovesTheRenewalTimeWithTheWindow", tlsManagerMovesTheRenewalTime},
	{"RenewsWithoutARIAfterTwoThirds", func(t *testing.T, e *instanceEnv) { tlsManagerRenewsWithoutARI(t, e, "", 2, 3) }},
	{"RenewsAShortCertificateWithoutARIAfterHalf", func(t *testing.T, e *instanceEnv) {
		tlsManagerRenewsWithoutARI(t, e, FakeACMEProfile, 1, 2)
	}},
	{"ServesARenewalUnderAShorterProfile", tlsManagerRenewsUnderAShorterProfile},
	{"OrdersNowSkippingTheWait", tlsManagerOrdersNow},
	{"LoadsTheCertificatesOfNotices", tlsManagerLoadsNotices},
	{"RevokesAfterOrderingTheReplacement", tlsManagerRevokes},
	{"RevokesWithoutAReplacement", tlsManagerRevokesWithoutAReplacement},
	{"CountsTheOrders", tlsManagerCountsTheOrders},
}

// fakeWindow is the window of RenewalInfo of the FakeACMEClient by default.
func fakeWindow(leaf *x509.Certificate) (start, end time.Time) {
	life := leaf.NotAfter.Sub(leaf.NotBefore)
	return leaf.NotBefore.Add(life * 2 / 3), leaf.NotAfter.Add(-life / 10)
}

// renewalProcess is a process of a single host, which orders its first certificate from ca, and
// the KeyValueStore of its leases and challenges.
func (e *instanceEnv) renewalProcess(t *testing.T, ca *FakeACMEClient, mod func(*appinstance.TLSManagerOptions)) (tlsProcess, kernel.KeyValueStore) {
	t.Helper()
	kv := testkit.NewKeyValueStore(e.clock)
	ca.Validate = ChallengeValidator(appinstance.NewACMEChallenges(kv))
	p := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) {
		o.Hosts = []string{tlsManagerHost}
		if mod != nil {
			mod(o)
		}
	})
	p.manager.Maintain(t.Context())
	if p.manager.CertificateID(tlsManagerHost) == "" {
		t.Fatal("no first certificate")
	}
	return p, kv
}

// checkRenewed checks that the last order replaced the certificate old of host, and that p serves
// the new one.
func (e *instanceEnv) checkRenewed(t *testing.T, ca *FakeACMEClient, p tlsProcess, old *x509.Certificate) {
	t.Helper()
	replaces := ca.Replaces()
	if len(replaces) != 2 || replaces[0] != "" || replaces[1] != old.SerialNumber.String() {
		t.Errorf("the orders replace %q, want nothing and then %s", replaces, old.SerialNumber)
	}
	e.checkServes(t, p, tlsManagerHost)
	if p.sink.Certificate(tlsManagerHost).Leaf.SerialNumber.Cmp(old.SerialNumber) == 0 {
		t.Error("the renewed certificate is not served")
	}
}

func tlsManagerRenewsInTheWindow(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	var mu sync.Mutex
	var notified []string
	p, _ := e.renewalProcess(t, ca, func(o *appinstance.TLSManagerOptions) {
		o.Notify = func(host, id string) {
			mu.Lock()
			defer mu.Unlock()
			notified = append(notified, host+" "+id)
		}
	})
	first := p.manager.CertificateID(tlsManagerHost)
	old := p.sink.Certificate(tlsManagerHost).Leaf
	start, end := fakeWindow(old)
	at := p.manager.RenewalTime(tlsManagerHost)
	if at.Before(start) || !at.Before(end) {
		t.Fatalf("renewal time %v outside the window [%v, %v)", at, start, end)
	}
	// Until then, the rounds come every 5 minutes, and none orders.
	e.clock.Set(at.Add(-time.Minute))
	if next := p.manager.Maintain(ctx); next.After(at) {
		t.Errorf("next round at %v, after the renewal time %v", next, at)
	}
	if got := ca.Orders(); len(got) != 1 {
		t.Fatalf("orders before the renewal time = %v, want 1", got)
	}
	e.clock.Set(at.Add(time.Millisecond))
	p.manager.Maintain(ctx)
	e.checkRenewed(t, ca, p, old)
	mu.Lock()
	want := []string{tlsManagerHost + " " + first, tlsManagerHost + " " + p.manager.CertificateID(tlsManagerHost)}
	if !slices.Equal(notified, want) {
		t.Errorf("notified %v, want %v", notified, want)
	}
	mu.Unlock()
	if attempts, err := e.h.Instance.ListUnresultedTLSOrderAttempts(ctx, tlsManagerHost, time.Time{}); err != nil || len(attempts) != 0 {
		t.Errorf("attempts without a certificate = %v, %v", attempts, err)
	}
}

func tlsManagerKeepsTheRenewalTime(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	var mu sync.Mutex
	reads := 0
	ca.RenewalWindow = func(leaf *x509.Certificate) (appinstance.ACMERenewalInfo, error) {
		mu.Lock()
		defer mu.Unlock()
		reads++
		start, end := fakeWindow(leaf)
		// A Retry-After of a second is clamped to a minute.
		return appinstance.ACMERenewalInfo{WindowStart: start, WindowEnd: end, RetryAfter: ca.now().Add(time.Second)}, nil
	}
	p, _ := e.renewalProcess(t, ca, nil)
	at := p.manager.RenewalTime(tlsManagerHost)
	for range 20 {
		e.clock.Advance(time.Minute)
		if next := p.manager.Maintain(ctx); !next.Equal(e.clock.Now().Add(time.Minute)) {
			t.Fatalf("next round at %v, want when the renewal information is read again, in a minute", next)
		}
		if got := p.manager.RenewalTime(tlsManagerHost); !got.Equal(at) {
			t.Fatalf("the renewal time moved from %v to %v while the window stayed", at, got)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if reads != 21 {
		t.Errorf("the renewal information was read %d times, want once a round", reads)
	}
}

func tlsManagerMovesTheRenewalTime(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	var mu sync.Mutex
	var shift time.Duration
	var retryAfter time.Time
	ca.RenewalWindow = func(leaf *x509.Certificate) (appinstance.ACMERenewalInfo, error) {
		mu.Lock()
		defer mu.Unlock()
		start, end := fakeWindow(leaf)
		return appinstance.ACMERenewalInfo{WindowStart: start.Add(shift), WindowEnd: end.Add(shift), RetryAfter: retryAfter}, nil
	}
	p, _ := e.renewalProcess(t, ca, nil)
	at := p.manager.RenewalTime(tlsManagerHost)
	// The CA moves the window a day earlier (as for a revocation to come); without a Retry-After,
	// the manager reads it again within 12 hours.
	mu.Lock()
	shift = -24 * time.Hour
	mu.Unlock()
	e.clock.Advance(12 * time.Hour)
	p.manager.Maintain(ctx)
	moved := p.manager.RenewalTime(tlsManagerHost)
	start, end := fakeWindow(p.sink.Certificate(tlsManagerHost).Leaf)
	if moved.Equal(at) || moved.Before(start.Add(shift)) || !moved.Before(end.Add(shift)) {
		t.Fatalf("the renewal time %v after the window moved, from %v; want one in [%v, %v)", moved, at, start.Add(shift), end.Add(shift))
	}
	// A Retry-After of two days is honoured for at most 12 hours.
	mu.Lock()
	retryAfter = e.clock.Now().Add(48 * time.Hour)
	shift = -48 * time.Hour
	mu.Unlock()
	e.clock.Advance(12 * time.Hour)
	if next := p.manager.Maintain(ctx); !next.Equal(e.clock.Now().Add(appinstance.DefaultTLSCheckInterval)) {
		t.Errorf("next round at %v, want in 5 minutes", next)
	}
	before := p.manager.RenewalTime(tlsManagerHost)
	mu.Lock()
	shift = -72 * time.Hour
	mu.Unlock()
	e.clock.Advance(12*time.Hour - time.Minute)
	p.manager.Maintain(ctx)
	if got := p.manager.RenewalTime(tlsManagerHost); !got.Equal(before) {
		t.Errorf("the renewal information was read again before 12 hours")
	}
	e.clock.Advance(time.Minute)
	p.manager.Maintain(ctx)
	if got := p.manager.RenewalTime(tlsManagerHost); got.Equal(before) || got.Before(start.Add(-72*time.Hour)) || !got.Before(end.Add(-72*time.Hour)) {
		t.Errorf("the renewal time 12 hours later is %v, want one in the window moved by 3 days", got)
	}
}

func tlsManagerRenewsWithoutARI(t *testing.T, e *instanceEnv, profile string, num, den int64) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	ca.RenewalWindow = func(*x509.Certificate) (appinstance.ACMERenewalInfo, error) {
		return appinstance.ACMERenewalInfo{}, fmt.Errorf("fake acme: %w", appinstance.ErrACMERenewalInfoUnsupported)
	}
	p, _ := e.renewalProcess(t, ca, func(o *appinstance.TLSManagerOptions) { o.Profile = profile })
	old := p.sink.Certificate(tlsManagerHost).Leaf
	life := old.NotAfter.Sub(old.NotBefore)
	want := old.NotBefore.Add(time.Duration(int64(life) * num / den))
	if at := p.manager.RenewalTime(tlsManagerHost); !at.Equal(want) {
		t.Fatalf("renewal time %v, want %v (%d/%d of %v)", at, want, num, den, life)
	}
	e.clock.Set(want.Add(-time.Millisecond))
	p.manager.Maintain(ctx)
	if got := ca.Orders(); len(got) != 1 {
		t.Fatalf("orders before the renewal time = %v, want 1", got)
	}
	e.clock.Set(want)
	p.manager.Maintain(ctx)
	e.checkRenewed(t, ca, p, old)
}

func tlsManagerRenewsUnderAShorterProfile(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	first, _ := e.renewalProcess(t, ca, nil)
	old := first.sink.Certificate(tlsManagerHost).Leaf
	// The operator moves to the profile of 6 days: the renewal ends long before the certificate
	// it replaces, and is the current one all the same.
	kv := testkit.NewKeyValueStore(e.clock)
	ca.Validate = ChallengeValidator(appinstance.NewACMEChallenges(kv))
	p := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) {
		o.Hosts = []string{tlsManagerHost}
		o.Profile = FakeACMEProfile
	})
	if err := p.manager.ReloadAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.manager.OrderNow(ctx, tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	e.checkRenewed(t, ca, p, old)
	renewed := p.sink.Certificate(tlsManagerHost).Leaf
	if !renewed.NotAfter.Before(old.NotAfter) {
		t.Fatalf("the renewal ends at %v, not before %v", renewed.NotAfter, old.NotAfter)
	}
	// Another process loads the renewal, not the certificate that ends later.
	if err := first.manager.ReloadAll(ctx); err != nil {
		t.Fatal(err)
	}
	e.checkServes(t, first, tlsManagerHost)
	if first.sink.Certificate(tlsManagerHost).Leaf.SerialNumber.Cmp(renewed.SerialNumber) != 0 {
		t.Error("the other process serves the certificate that ends later")
	}
}

func tlsManagerOrdersNow(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	p, kv := e.renewalProcess(t, ca, nil)
	old := p.sink.Certificate(tlsManagerHost).Leaf
	// A failed order: the backoff holds the rounds, but not an order asked for now.
	ca.FailOrders(errors.New("the CA is down"))
	if err := p.manager.OrderNow(ctx, tlsManagerHost); err == nil {
		t.Fatal("an order that failed reported nothing")
	}
	ca.FailOrders(nil)
	if err := p.manager.OrderNow(ctx, tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	replaces := ca.Replaces()
	if want := []string{"", old.SerialNumber.String(), old.SerialNumber.String()}; !slices.Equal(replaces, want) {
		t.Errorf("the orders replace %q, want %q", replaces, want)
	}
	e.checkServes(t, p, tlsManagerHost)
	if err := p.manager.OrderNow(ctx, "other.example"); err == nil {
		t.Error("an order of a host outside the hosts went out")
	}
	// While another process holds the lease, an order now waits for nothing and reports it.
	if err := kv.Set(ctx, appinstance.TLSOrderLeaseNamespace, tlsManagerHost, []byte("another"), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := p.manager.OrderNow(ctx, tlsManagerHost); !errors.Is(err, appinstance.ErrTLSOrderInProgress) {
		t.Errorf("an order now while another process orders: %v, want ErrTLSOrderInProgress", err)
	}
}

func tlsManagerLoadsNotices(t *testing.T, e *instanceEnv) {
	ca, kv := e.newTLSCA(t)
	one := func(o *appinstance.TLSManagerOptions) { o.Hosts = []string{tlsManagerHost} }
	renewing := e.tlsProcess(t, ca, kv, one)
	renewing.manager.Maintain(t.Context())
	first := renewing.manager.CertificateID(tlsManagerHost)
	other := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) {
		one(o)
		// Only notices load: the rounds of Run come once an hour.
		o.CheckInterval, o.RetryInterval = time.Hour, time.Hour
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		other.manager.Run(ctx)
	}()
	t.Cleanup(func() { cancel(); <-done })
	await := func(want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for other.manager.CertificateID(tlsManagerHost) != want {
			if time.Now().After(deadline) {
				t.Fatalf("the other process holds %q, want %q", other.manager.CertificateID(tlsManagerHost), want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	await(first) // the first round of Run
	if err := renewing.manager.OrderNow(t.Context(), tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	second := renewing.manager.CertificateID(tlsManagerHost)
	other.manager.Notice(tlsManagerHost)
	await(second)
	// A notice that comes late, or twice, loads the current certificate again: never the one
	// it was sent for.
	other.manager.Notice(tlsManagerHost)
	other.manager.NoticeAll()
	other.manager.Notice("other.example")
	time.Sleep(50 * time.Millisecond)
	await(second)
	e.checkServes(t, other, tlsManagerHost)
	if second == first {
		t.Fatal("the order made no new certificate")
	}
}
