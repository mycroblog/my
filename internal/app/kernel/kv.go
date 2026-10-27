// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"time"
)

// ErrKeyNotFound reports that a KeyValueStore has no live entry for a key.
var ErrKeyNotFound = errors.New("kernel: key not found")

// KeyValueStore keeps short-lived values that need no schema: caches, rate-limit counters, and
// the like (ADR-0003, ADR-0019 §5). Keys live in namespaces, so that callers do not collide;
// a namespace names its owner, for example "ratelimit".
//
// An entry with a TTL expires that long after it was written; a TTL <= 0 means it never
// expires. Expired entries behave as absent: Get reports ErrKeyNotFound for them and IncrBy
// starts them again. DeleteExpired removes them physically, a bounded batch at a time, so that
// one call stays a short transaction on a large store.
//
// The production adapters live in internal/adapter/kv; their shared contract test suite is
// testkit.KeyValueStoreContract in pkg/testkit.
type KeyValueStore interface {
	// Get returns the value of a live entry, or an error wrapping ErrKeyNotFound.
	Get(ctx context.Context, namespace, key string) ([]byte, error)
	// Set writes value, replacing any entry and its expiry.
	Set(ctx context.Context, namespace, key string, value []byte, ttl time.Duration) error
	// Delete removes an entry; deleting an absent key is not an error.
	Delete(ctx context.Context, namespace, key string) error
	// IncrBy atomically adds n to the counter at key and returns the new value. A missing or
	// expired entry starts at n and expires after ttl; a live entry keeps its expiry.
	// Counters are stored as their decimal representation, which is what Get returns for them;
	// IncrBy on an entry written by Set with anything else is undefined.
	IncrBy(ctx context.Context, namespace, key string, n int64, ttl time.Duration) (int64, error)
	// CompareAndSwap atomically replaces the entry at key with value and expiry ttl, or removes
	// it when value is nil, only while it holds old, and reports whether it did. A nil old
	// expects no live entry (absent or expired); an empty, non-nil old expects an empty value.
	CompareAndSwap(ctx context.Context, namespace, key string, old, value []byte, ttl time.Duration) (bool, error)
	// DeleteExpired removes at most limit expired entries and returns how many it removed;
	// fewer than limit means none are left. A limit <= 0 removes nothing.
	DeleteExpired(ctx context.Context, limit int) (int64, error)
}
