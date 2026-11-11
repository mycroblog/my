// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/pkg/testkit"
)

// notices records the calls of TLSManagerOptions.Notify.
type notices struct {
	mu   sync.Mutex
	sent []string
}

func (n *notices) notify(host, id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, host+"="+id)
}

func (n *notices) list() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.sent)
}

// A revocation orders the replacement first, replacing the certificate, then revokes it at the
// CA and records it; every process loads the replacement, and none serves the revoked one.
func tlsManagerRevokes(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	var sent notices
	p, kv := e.renewalProcess(t, ca, func(o *appinstance.TLSManagerOptions) { o.Notify = sent.notify })
	other := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) { o.Hosts = []string{tlsManagerHost} })
	if err := other.manager.ReloadAll(ctx); err != nil {
		t.Fatal(err)
	}
	old := p.sink.Certificate(tlsManagerHost).Leaf
	oldID := p.manager.CertificateID(tlsManagerHost)

	res, err := p.manager.Revoke(ctx, tlsManagerHost, instance.TLSRevocationKeyCompromise)
	if err != nil || res.ReplacementErr != nil {
		t.Fatalf("Revoke = %+v, %v", res, err)
	}
	if res.Revoked.ID != oldID || res.ReplacementID == "" || res.ReplacementID == oldID {
		t.Errorf("Revoke revoked %s and replaced it with %q; want %s and a new one", res.Revoked.ID, res.ReplacementID, oldID)
	}
	if !ca.Revoked(old.SerialNumber.String()) {
		t.Error("the CA did not revoke the certificate")
	}
	if replaces := ca.Replaces(); len(replaces) != 2 || replaces[1] != old.SerialNumber.String() {
		t.Errorf("the orders replace %q, want the revoked certificate last", replaces)
	}
	e.checkServes(t, p, tlsManagerHost)
	if got := sent.list(); len(got) == 0 || got[len(got)-1] != tlsManagerHost+"="+res.ReplacementID {
		t.Errorf("notices = %q, want the replacement last", got)
	}
	// The other process loads the replacement on the notice.
	if err := other.manager.Reload(ctx, tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	e.checkServes(t, other, tlsManagerHost)
	if other.sink.Certificate(tlsManagerHost).Leaf.SerialNumber.Cmp(old.SerialNumber) == 0 {
		t.Error("the other process serves the revoked certificate")
	}

	// The CA revoked it already: a second request records nothing new and reports it.
	if _, err := p.manager.Revoke(ctx, "other.example", instance.TLSRevocationOperator); err == nil {
		t.Error("a revocation of a host outside the hosts went out")
	}
	if _, err := p.manager.Revoke(ctx, tlsManagerHost, "lost"); err == nil {
		t.Error("a revocation with an unknown reason went out")
	}
	if err := kv.Set(ctx, appinstance.TLSOrderLeaseNamespace, tlsManagerHost, []byte("another"), time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := p.manager.Revoke(ctx, tlsManagerHost, instance.TLSRevocationOperator); !errors.Is(err, appinstance.ErrTLSOrderInProgress) {
		t.Errorf("a revocation while another process orders: %v, want ErrTLSOrderInProgress", err)
	}
}

// When the replacement cannot be ordered, the revocation still goes out: the processes serve the
// certificate issued before the revoked one, or none, and the next order replaces nothing, since
// the older certificate was replaced already.
func tlsManagerRevokesWithoutAReplacement(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	p, kv := e.renewalProcess(t, ca, nil)
	other := e.tlsProcess(t, ca, kv, func(o *appinstance.TLSManagerOptions) { o.Hosts = []string{tlsManagerHost} })
	first := p.sink.Certificate(tlsManagerHost).Leaf
	e.clock.Advance(time.Minute)
	if err := p.manager.OrderNow(ctx, tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	second := p.sink.Certificate(tlsManagerHost).Leaf
	if err := other.manager.ReloadAll(ctx); err != nil {
		t.Fatal(err)
	}

	ca.FailOrders(errors.New("the CA is down"))
	res, err := p.manager.Revoke(ctx, tlsManagerHost, instance.TLSRevocationSuperseded)
	if err != nil || res.ReplacementErr == nil || res.ReplacementID != "" {
		t.Fatalf("Revoke with the orders failing = %+v, %v; want a revocation without a replacement", res, err)
	}
	if !ca.Revoked(second.SerialNumber.String()) {
		t.Error("the CA did not revoke the certificate")
	}
	for _, proc := range []tlsProcess{p, other} {
		if err := proc.manager.Reload(ctx, tlsManagerHost); err != nil {
			t.Fatal(err)
		}
		if got := proc.sink.Certificate(tlsManagerHost); got == nil || got.Leaf.SerialNumber.Cmp(first.SerialNumber) != 0 {
			t.Errorf("a process serves %v, want the certificate before the revoked one", got)
		}
	}
	ca.FailOrders(nil)
	if err := p.manager.OrderNow(ctx, tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	if replaces := ca.Replaces(); replaces[len(replaces)-1] != "" {
		t.Errorf("the orders replace %q, want nothing last: the first certificate was replaced already", replaces)
	}

	// Every certificate revoked: no process serves one until a new one comes.
	ca.FailOrders(errors.New("the CA is down"))
	for range 2 {
		if _, err := p.manager.Revoke(ctx, tlsManagerHost, instance.TLSRevocationOperator); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.manager.Revoke(ctx, tlsManagerHost, instance.TLSRevocationOperator); !errors.Is(err, appinstance.ErrNoTLSCertificate) {
		t.Errorf("a revocation without a certificate: %v, want ErrNoTLSCertificate", err)
	}
	if err := other.manager.Reload(ctx, tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	for _, proc := range []tlsProcess{p, other} {
		if proc.sink.Certificate(tlsManagerHost) != nil || proc.manager.CertificateID(tlsManagerHost) != "" {
			t.Error("a process serves a revoked certificate")
		}
	}
	ca.FailOrders(nil)
	e.clock.Advance(48 * time.Hour) // past the backoff of the failed replacements
	p.manager.Maintain(ctx)
	e.checkServes(t, p, tlsManagerHost)
}

// The orders count by result, and the failures since the current certificate are a gauge.
func tlsManagerCountsTheOrders(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	ca := NewFakeACMEClient(t)
	ca.Clock = e.clock
	meter := testkit.NewMeter()
	p, _ := e.renewalProcess(t, ca, func(o *appinstance.TLSManagerOptions) { o.Meter = meter })
	ca.FailOrders(errors.New("the CA is down"))
	for range 2 {
		e.clock.Advance(time.Minute)
		if err := p.manager.OrderNow(ctx, tlsManagerHost); err == nil {
			t.Fatal("an order that failed reported nothing")
		}
	}
	host := map[string]string{appinstance.LabelTLSHost: tlsManagerHost}
	failures := func() float64 {
		t.Helper()
		before := len(meter.Measurements(appinstance.TLSOrderConsecutiveFailures.Name, host))
		if err := meter.Collect(ctx); err != nil {
			t.Fatal(err)
		}
		ms := meter.Measurements(appinstance.TLSOrderConsecutiveFailures.Name, host)
		if len(ms) != before+1 {
			t.Fatalf("the gauge reported %d values, want one", len(ms)-before)
		}
		return ms[len(ms)-1].Value
	}
	if n := failures(); n != 2 {
		t.Errorf("consecutive failures = %v, want 2", n)
	}
	// A round reads them from the database.
	p.manager.Maintain(ctx)
	if n := failures(); n != 2 {
		t.Errorf("consecutive failures after a round = %v, want 2", n)
	}
	ca.FailOrders(nil)
	if err := p.manager.OrderNow(ctx, tlsManagerHost); err != nil {
		t.Fatal(err)
	}
	if n := failures(); n != 0 {
		t.Errorf("consecutive failures after a certificate = %v, want 0", n)
	}
	issued := map[string]string{appinstance.LabelTLSHost: tlsManagerHost, appinstance.LabelTLSResult: appinstance.TLSOrderIssued}
	failed := map[string]string{appinstance.LabelTLSHost: tlsManagerHost, appinstance.LabelTLSResult: appinstance.TLSOrderFailed}
	if got := meter.Sum(appinstance.TLSOrderAttempts.Name, issued); got != 2 {
		t.Errorf("issued orders = %v, want 2", got)
	}
	if got := meter.Sum(appinstance.TLSOrderAttempts.Name, failed); got != 2 {
		t.Errorf("failed orders = %v, want 2", got)
	}
	if err := meter.Err(); err != nil {
		t.Error(err)
	}
}
