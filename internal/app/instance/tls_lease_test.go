// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// leaseKV is a kernel.KeyValueStore in memory without expiry, enough for the lease.
type leaseKV struct {
	kernel.KeyValueStore
	mu sync.Mutex
	m  map[string][]byte
}

func newLeaseKV() *leaseKV { return &leaseKV{m: map[string][]byte{}} }

func (kv *leaseKV) Get(_ context.Context, ns, key string) ([]byte, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	v, ok := kv.m[ns+"/"+key]
	if !ok {
		return nil, kernel.ErrKeyNotFound
	}
	return v, nil
}

func (kv *leaseKV) Set(_ context.Context, ns, key string, value []byte, _ time.Duration) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.m[ns+"/"+key] = value
	return nil
}

func (kv *leaseKV) CompareAndSwap(_ context.Context, ns, key string, old, value []byte, _ time.Duration) (bool, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	cur, ok := kv.m[ns+"/"+key]
	if (old == nil && ok) || (old != nil && (!ok || !bytes.Equal(cur, old))) {
		return false, nil
	}
	if value == nil {
		delete(kv.m, ns+"/"+key)
	} else {
		kv.m[ns+"/"+key] = value
	}
	return true, nil
}

func TestTLSOrderLeaseHoldsOneAtATime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		kv := newLeaseKV()
		l := tlsOrderLease{kv: kv, ttl: time.Minute, renewal: 10 * time.Millisecond}
		ran := false
		held, err := l.hold(ctx, "social.example", func(ctx context.Context) error {
			// Another process finds the lease taken, also across several extensions.
			<-time.After(50 * time.Millisecond)
			synctest.Wait()
			other, err := l.hold(ctx, "social.example", func(context.Context) error {
				t.Error("two holders of one lease")
				return nil
			})
			if other || err != nil {
				t.Errorf("hold of a held lease = %v, %v; want false", other, err)
			}
			// Another host has a lease of its own.
			if other, err := l.hold(ctx, "app.social.example", func(context.Context) error { return nil }); !other || err != nil {
				t.Errorf("hold of another host = %v, %v; want true", other, err)
			}
			ran = ctx.Err() == nil
			return nil
		})
		if !held || err != nil || !ran {
			t.Fatalf("hold = %v, %v (ran %v); want it held and run", held, err, ran)
		}
		// Released at the end: the next holder takes it at once.
		if held, err := l.hold(ctx, "social.example", func(context.Context) error { return nil }); !held || err != nil {
			t.Errorf("hold after the release = %v, %v; want true", held, err)
		}
	})
}

func TestTLSOrderLeaseEndsTheWorkWhenLost(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	kv := newLeaseKV()
	l := tlsOrderLease{kv: kv, ttl: time.Minute, renewal: 10 * time.Millisecond}
	held, err := l.hold(ctx, "social.example", func(ctx context.Context) error {
		// The entry expired and another process took it.
		if err := kv.Set(ctx, TLSOrderLeaseNamespace, "social.example", []byte("other"), time.Minute); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(10 * time.Second):
			return errors.New("the work went on")
		}
	})
	if !held || !errors.Is(err, errTLSOrderLeaseLost) {
		t.Fatalf("hold = %v, %v; want the work ended by the lost lease", held, err)
	}
	if v, err := kv.Get(ctx, TLSOrderLeaseNamespace, "social.example"); err != nil || string(v) != "other" {
		t.Errorf("the other holder's lease = %q, %v; want it kept", v, err)
	}
}
