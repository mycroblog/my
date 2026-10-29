// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// countingKeyManager counts the calls of UnwrapKey of the KeyManager it wraps.
type countingKeyManager struct {
	kernel.KeyManager
	unwraps int
}

func (k *countingKeyManager) UnwrapKey(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
	k.unwraps++
	return k.KeyManager.UnwrapKey(ctx, id, wrapped, aad)
}

type stepClock struct{ now time.Time }

func (c *stepClock) Now() time.Time { return c.now }

func newCache(t *testing.T, ttl time.Duration, entries int) (kernel.KeyManager, *countingKeyManager, *stepClock) {
	t.Helper()
	inner := &countingKeyManager{KeyManager: newFakeKeyManager(t, "a")}
	clock := &stepClock{now: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)}
	return kernel.NewDEKCache(inner, kernel.DEKCacheOptions{TTL: ttl, Entries: entries, Clock: clock}), inner, clock
}

func wrapDEK(t *testing.T, km kernel.KeyManager, aad string) (string, []byte, []byte) {
	t.Helper()
	dek := bytes.Repeat([]byte(aad[:1]), 32)
	id, wrapped, err := km.WrapKey(t.Context(), dek, []byte(aad))
	if err != nil {
		t.Fatalf("WrapKey: %v", err)
	}
	return id, wrapped, dek
}

func unwrap(t *testing.T, km kernel.KeyManager, id string, wrapped []byte, aad string, want []byte) {
	t.Helper()
	got, err := km.UnwrapKey(t.Context(), id, wrapped, []byte(aad))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("UnwrapKey(%s) = %x, %v; want %x", aad, got, err, want)
	}
}

func TestDEKCacheKeepsUnwrappedKeysForTheTTL(t *testing.T) {
	t.Parallel()
	km, inner, clock := newCache(t, 10*time.Minute, 4)
	id, wrapped, dek := wrapDEK(t, km, "row-1")
	unwrap(t, km, id, wrapped, "row-1", dek)
	clock.now = clock.now.Add(10*time.Minute - time.Millisecond)
	unwrap(t, km, id, wrapped, "row-1", dek)
	if inner.unwraps != 1 {
		t.Errorf("unwraps within the TTL = %d, want 1", inner.unwraps)
	}
	clock.now = clock.now.Add(time.Millisecond)
	unwrap(t, km, id, wrapped, "row-1", dek)
	if inner.unwraps != 2 {
		t.Errorf("unwraps after the TTL = %d, want 2", inner.unwraps)
	}
}

func TestDEKCacheKeepsAtMostItsEntries(t *testing.T) {
	t.Parallel()
	km, inner, _ := newCache(t, time.Hour, 2)
	type sealed struct {
		id      string
		wrapped []byte
		dek     []byte
	}
	vals := map[string]sealed{}
	for _, aad := range []string{"a", "b", "c"} {
		id, w, d := wrapDEK(t, km, aad)
		vals[aad] = sealed{id, w, d}
	}
	use := func(aad string) {
		v := vals[aad]
		unwrap(t, km, v.id, v.wrapped, aad, v.dek)
	}
	use("a")
	use("b")
	use("a") // a is now the most recently used, so c pushes b out
	use("c")
	if inner.unwraps != 3 {
		t.Fatalf("unwraps = %d, want 3", inner.unwraps)
	}
	use("a")
	use("c")
	if inner.unwraps != 3 {
		t.Errorf("unwraps of the kept entries = %d, want 3", inner.unwraps)
	}
	use("b")
	if inner.unwraps != 4 {
		t.Errorf("unwraps of the evicted entry = %d, want 4", inner.unwraps)
	}
}

func TestDEKCacheMissesOnADifferentAADOrWrappedKey(t *testing.T) {
	t.Parallel()
	km, inner, _ := newCache(t, time.Hour, 8)
	id, wrapped, dek := wrapDEK(t, km, "row-1")
	unwrap(t, km, id, wrapped, "row-1", dek)
	if _, err := km.UnwrapKey(t.Context(), id, wrapped, []byte("row-2")); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("UnwrapKey with another AAD = %v, want ErrDecrypt", err)
	}
	tampered := append([]byte(nil), wrapped...)
	tampered[0] ^= 1
	if _, err := km.UnwrapKey(t.Context(), id, tampered, []byte("row-1")); !errors.Is(err, kernel.ErrDecrypt) {
		t.Errorf("UnwrapKey of a tampered key = %v, want ErrDecrypt", err)
	}
	if inner.unwraps != 3 {
		t.Errorf("unwraps = %d, want 3 (errors are not cached)", inner.unwraps)
	}
}

func TestDEKCacheReturnsACopy(t *testing.T) {
	t.Parallel()
	km, _, _ := newCache(t, time.Hour, 8)
	id, wrapped, dek := wrapDEK(t, km, "row-1")
	got, err := km.UnwrapKey(t.Context(), id, wrapped, []byte("row-1"))
	if err != nil {
		t.Fatal(err)
	}
	clear(got)
	unwrap(t, km, id, wrapped, "row-1", dek)
}

func TestDEKCacheIsDisabledByZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ttl     time.Duration
		entries int
	}{{0, 1024}, {10 * time.Minute, 0}} {
		km, inner, _ := newCache(t, tc.ttl, tc.entries)
		if km != kernel.KeyManager(inner) {
			t.Errorf("NewDEKCache(ttl %s, entries %d) = %v, want the KeyManager itself", tc.ttl, tc.entries, km)
		}
	}
}

func TestDEKCacheDoesNotPrintKeys(t *testing.T) {
	t.Parallel()
	km, _, _ := newCache(t, time.Hour, 8)
	id, wrapped, dek := wrapDEK(t, km, "row-1")
	unwrap(t, km, id, wrapped, "row-1", dek)
	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("cache", slog.Any("km", km))
	for _, out := range []string{fmt.Sprint(km), fmt.Sprintf("%v", km), logs.String()} {
		if !strings.Contains(out, "[REDACTED]") || strings.Contains(out, fmt.Sprintf("%x", dek)) {
			t.Errorf("printed cache = %q", out)
		}
	}
}
