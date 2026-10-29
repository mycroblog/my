// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Defaults of the cache of unwrapped DEKs (ADR-0121 §3).
const (
	DefaultDEKCacheTTL     = 10 * time.Minute
	DefaultDEKCacheEntries = 1024
)

// DEKCacheOptions configure NewDEKCache.
type DEKCacheOptions struct {
	// TTL is how long an unwrapped DEK is kept; zero or less disables the cache.
	TTL time.Duration
	// Entries is how many unwrapped DEKs are kept at most, the least recently used going first;
	// zero or less disables the cache.
	Entries int
	// Clock tells the time of the expiry.
	Clock Clock
}

// NewDEKCache returns a KeyManager that keeps the results of km.UnwrapKey for opts.TTL, at most
// opts.Entries of them, so that a KeyManager that calls a KMS is not called for every use of a
// sealed secret (ADR-0121 §3). An entry is found by a hash of the key ID, the wrapped DEK, and
// the AAD, so a different wrapped DEK or AAD always reaches km. WrapKey is never cached. Errors
// are not cached either. It returns km itself when opts disable the cache.
//
// The cache holds DEKs in memory, where the process can read them; it never prints them.
func NewDEKCache(km KeyManager, opts DEKCacheOptions) KeyManager {
	if opts.TTL <= 0 || opts.Entries <= 0 {
		return km
	}
	return &dekCache{km: km, ttl: opts.TTL, max: opts.Entries, clock: opts.Clock, entries: map[[sha256.Size]byte]*list.Element{}, lru: list.New()}
}

type dekCache struct {
	km    KeyManager
	ttl   time.Duration
	max   int
	clock Clock

	mu      sync.Mutex
	entries map[[sha256.Size]byte]*list.Element
	lru     *list.List // of *dekEntry, the most recently used first
}

type dekEntry struct {
	key     [sha256.Size]byte
	dek     []byte
	expires time.Time
}

func (c *dekCache) CurrentKeyID() string { return c.km.CurrentKeyID() }

func (c *dekCache) WrapKey(ctx context.Context, dek, aad []byte) (string, []byte, error) {
	return c.km.WrapKey(ctx, dek, aad)
}

func (c *dekCache) UnwrapKey(ctx context.Context, keyID string, wrapped, aad []byte) ([]byte, error) {
	key := sha256.Sum256(AAD(keyID, string(wrapped), string(aad)))
	if dek, ok := c.get(key); ok {
		return dek, nil
	}
	dek, err := c.km.UnwrapKey(ctx, keyID, wrapped, aad)
	if err != nil {
		return nil, err
	}
	c.put(key, dek)
	return dek, nil
}

func (c *dekCache) get(key [sha256.Size]byte) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	e := el.Value.(*dekEntry)
	if !c.clock.Now().Before(e.expires) {
		c.lru.Remove(el)
		delete(c.entries, key)
		return nil, false
	}
	c.lru.MoveToFront(el)
	return append([]byte(nil), e.dek...), true
}

func (c *dekCache) put(key [sha256.Size]byte, dek []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := &dekEntry{key: key, dek: append([]byte(nil), dek...), expires: c.clock.Now().Add(c.ttl)}
	if el, ok := c.entries[key]; ok {
		el.Value = e
		c.lru.MoveToFront(el)
		return
	}
	c.entries[key] = c.lru.PushFront(e)
	for c.lru.Len() > c.max {
		last := c.lru.Back()
		c.lru.Remove(last)
		delete(c.entries, last.Value.(*dekEntry).key)
	}
}

// String describes the cache without its DEKs.
func (c *dekCache) String() string {
	return fmt.Sprintf("kernel.DEKCache{ttl: %s, entries: %d, dek: [REDACTED]}", c.ttl, c.max)
}

// LogValue keeps the DEKs out of logs.
func (c *dekCache) LogValue() slog.Value {
	return slog.GroupValue(slog.Duration("ttl", c.ttl), slog.Int("entries", c.max), slog.String("dek", "[REDACTED]"))
}
