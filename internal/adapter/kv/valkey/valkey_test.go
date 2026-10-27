// SPDX-License-Identifier: AGPL-3.0-only

package valkey_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"

	kvvalkey "github.com/mycroblog/my/internal/adapter/kv/valkey"
	"github.com/mycroblog/my/internal/adapter/valkey"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/valkeytest"
)

// TestMain stops the Valkey container of package valkeytest after the tests.
func TestMain(m *testing.M) {
	os.Exit(valkeytest.Main(m))
}

func TestKeyValueStoreContract(t *testing.T) {
	testkit.KeyValueStoreContract(t, func(t *testing.T, clock *testkit.Clock) kernel.KeyValueStore {
		s := valkeytest.New(t)
		return kvvalkey.New(open(t, s), s.KeyPrefix, clock)
	})
}

// Stores with different key prefixes on one server keep their entries apart, and a namespace
// with the separator of the key layout does not collide with another namespace and key.
func TestStoresAreSeparate(t *testing.T) {
	s := valkeytest.New(t)
	c := open(t, s)
	clock := testkit.NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	a := kvvalkey.New(c, s.KeyPrefix+"a:", clock)
	b := kvvalkey.New(c, s.KeyPrefix+"b:", clock)
	ctx := t.Context()
	if err := a.Set(ctx, "ns", "k", []byte("in a"), 0); err != nil {
		t.Fatal(err)
	}
	if v, err := b.Get(ctx, "ns", "k"); err == nil {
		t.Fatalf("b.Get = %q; want ErrKeyNotFound", v)
	}
	if err := a.Set(ctx, "x:y", "z", []byte("x:y/z"), 0); err != nil {
		t.Fatal(err)
	}
	if v, err := a.Get(ctx, "x", "y:z"); err == nil {
		t.Fatalf("Get(x, y:z) = %q; want ErrKeyNotFound", v)
	}
}

// afterZrange runs a hook once, after the first ZRANGE that the store sends.
type afterZrange struct {
	valkey.Client
	once sync.Once
	hook func()
}

func (c *afterZrange) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	args := cmd.Commands() // the client recycles cmd once it ran
	zrange := len(args) > 0 && args[0] == "ZRANGE"
	r := c.Client.Do(ctx, cmd)
	if zrange {
		c.once.Do(c.hook)
	}
	return r
}

// A write that renews an entry between the listing and the removal of a DeleteExpired batch
// does not cut the batch short while other expired entries remain: a short batch still means
// that none are left.
func TestDeleteExpiredSkipsRenewedEntries(t *testing.T) {
	s := valkeytest.New(t)
	c := open(t, s)
	clock := testkit.NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	plain := kvvalkey.New(c, s.KeyPrefix, clock)
	ctx := t.Context()
	if err := plain.Set(ctx, "ns", "a", []byte("v"), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := plain.Set(ctx, "ns", "b", []byte("v"), 2*time.Second); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)

	hooked := &afterZrange{Client: c, hook: func() {
		// "a" is listed first, being the first to expire; renew it before its removal.
		if err := plain.Set(ctx, "ns", "a", []byte("renewed"), time.Hour); err != nil {
			t.Error(err)
		}
	}}
	store := kvvalkey.New(hooked, s.KeyPrefix, clock)
	if n, err := store.DeleteExpired(ctx, 1); err != nil || n != 1 {
		t.Fatalf("DeleteExpired(1) with a renewed entry = %d, %v; want 1 (the other expired entry)", n, err)
	}
	if n, err := store.DeleteExpired(ctx, 1); err != nil || n != 0 {
		t.Fatalf("DeleteExpired(1) again = %d, %v; want 0", n, err)
	}
	if v, err := store.Get(ctx, "ns", "a"); err != nil || string(v) != "renewed" {
		t.Fatalf("Get(a) = %q, %v; want the renewed value", v, err)
	}
}

// open connects to the test server and closes the client when t ends.
func open(t *testing.T, s valkeytest.Server) valkey.Client {
	t.Helper()
	c, err := valkey.Open(t.Context(), valkey.Options{URL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}
