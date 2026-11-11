// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// TLSOrderLeaseNamespace is the KeyValueStore namespace of the leases of the orders, keyed by the
// host: acme.order/<host> (ADR-0163 §5).
const TLSOrderLeaseNamespace = "acme.order"

// The defaults of the lease of an order (ADR-0163 §5): it lives 10 minutes and is extended every 3
// minutes, so that two extensions may fail to arrive before it expires.
const (
	DefaultTLSOrderLeaseTTL     = 10 * time.Minute
	DefaultTLSOrderLeaseRenewal = 3 * time.Minute
)

// tlsOrderLease is a lease over KeyValueStore.CompareAndSwap: its holder writes a random token
// with a TTL, extends it while it works, and removes it at the end. A holder that dies leaves the
// entry to expire, so that another process takes over after at most the TTL. Every process must
// share the store (kv = db or valkey): kv = memory gives each process its own leases.
type tlsOrderLease struct {
	kv      kernel.KeyValueStore
	ttl     time.Duration
	renewal time.Duration
}

// hold runs fn while it holds the lease of name, and reports false without running it while
// another process holds it. fn's context ends when an extension fails, so that a process that
// lost the lease (it stalled past the TTL, or the store failed) stops ordering.
func (l tlsOrderLease) hold(ctx context.Context, name string, fn func(ctx context.Context) error) (bool, error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := []byte(hex.EncodeToString(b))
	ok, err := l.kv.CompareAndSwap(ctx, TLSOrderLeaseNamespace, name, nil, token, l.ttl)
	if err != nil || !ok {
		return false, err
	}
	held, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	// The extensions run on a timer that schedules the next one; stop ends them, and waits for
	// one in progress so that none follows the release.
	var mu sync.Mutex
	stopped := false
	var timer *time.Timer
	extend := func() {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return
		}
		ok, err := l.kv.CompareAndSwap(held, TLSOrderLeaseNamespace, name, token, token, l.ttl)
		if err != nil || !ok {
			cancel(errTLSOrderLeaseLost)
			return
		}
		timer.Reset(l.renewal)
	}
	mu.Lock()
	timer = time.AfterFunc(l.renewal, extend)
	mu.Unlock()
	err = fn(held)
	mu.Lock()
	stopped = true
	timer.Stop()
	mu.Unlock()
	_, _ = l.kv.CompareAndSwap(context.WithoutCancel(ctx), TLSOrderLeaseNamespace, name, token, nil, 0)
	return true, err
}
