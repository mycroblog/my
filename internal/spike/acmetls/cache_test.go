// SPDX-License-Identifier: AGPL-3.0-only

package acmetls

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Two processes share the DB and a Bus. The handshake never unseals; a notice makes the other
// process reload; a stale notice never installs an older certificate; a broken listener reloads.
func TestMemoryCacheAndBusNotice(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock := testkit.NewClock(time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC))
	km := &countingKM{KeyManager: testkit.NewKeyManager("gcpkms:projects/p/locations/l/keyRings/r/cryptoKeys/k")}
	store := &DBStore{DB: openSpikeDB(t), Sealer: kernel.NewSealer(km), Clock: clock, NewID: testkit.NewIDGenerator(clock).NewID}
	broker := stream.NewMemoryBroker()
	newManager := func() *Manager {
		return &Manager{Hosts: []string{"example.com"}, Store: store, Bus: broker.Bus(), Clock: clock, KV: testkit.NewKeyValueStore(clock), RenewRatio: 2.0 / 3}
	}
	m1, m2 := newManager(), newManager()
	issue := func(notBefore time.Time) string {
		a, _ := store.RecordAttempt(ctx, "example.com", "dir", "")
		chain, key := selfSigned(t, "example.com", notBefore, 45*24*time.Hour)
		id, err := store.InsertCertificate(ctx, a, "example.com", "dir", chain, key)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := issue(clock.Now())
	for _, m := range []*Manager{m1, m2} {
		if err := m.LoadAll(ctx); err != nil {
			t.Fatal(err)
		}
		go m.Listen(ctx)
	}
	waitFor(t, "two listeners", func() bool { return broker.Listeners() == 2 })

	before := km.unwraps.Load()
	hello := &tls.ClientHelloInfo{ServerName: "example.com"}
	for range 10000 {
		if _, err := m1.GetCertificate(hello); err != nil {
			t.Fatal(err)
		}
	}
	if got := km.unwraps.Load() - before; got != 0 {
		t.Fatalf("handshakes unwrapped %d DEKs", got)
	}
	if _, err := m1.GetCertificate(&tls.ClientHelloInfo{ServerName: "evil.example"}); err == nil {
		t.Fatal("served an unknown SNI")
	}

	// m1 renews and sends a notice; m2 reloads from the DB.
	clock.Advance(30 * 24 * time.Hour)
	second := issue(clock.Now())
	_ = m1.reload(ctx, "example.com")
	if err := m1.notify(ctx, "example.com", second); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "m2 to reload", func() bool { return m2.Leaf("example.com").NotBefore.Equal(clock.Now().Truncate(time.Second)) })

	// A late notice naming the first certificate changes nothing: the DB decides.
	b, _ := json.Marshal(Notice{Kind: "tls_certificate", Host: "example.com", CertificateID: first})
	_ = broker.Bus().Send(ctx, b)
	time.Sleep(100 * time.Millisecond)
	if !m2.Leaf("example.com").NotBefore.Equal(clock.Now().Truncate(time.Second)) {
		t.Fatal("a stale notice installed the older certificate")
	}

	// A notice that never arrived (sent while the listener was down) is caught up when the
	// listener breaks and listens again. Without a break, only the periodic check (LoadAll on a
	// timer) would catch it: the Bus is best effort.
	clock.Advance(30 * 24 * time.Hour)
	issue(clock.Now()) // nobody notifies
	broker.Break()
	waitFor(t, "m2 to reload after the break", func() bool { return m2.Leaf("example.com").NotBefore.Equal(clock.Now().Truncate(time.Second)) })
	t.Logf("reloads: m1 %d, m2 %d; DEK unwraps in all: %d", m1.Reloads.Load(), m2.Reloads.Load(), km.unwraps.Load())
}

// With stream = hub there is one process and no Bus: renewing installs the certificate in
// memory directly, notify does nothing, and Listen returns at once.
func TestHubHasNothingToNotify(t *testing.T) {
	m := &Manager{Hosts: []string{"example.com"}}
	if err := m.notify(t.Context(), "example.com", "id"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { m.Listen(t.Context()); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Listen without a Bus blocked")
	}
}
