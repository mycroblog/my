// SPDX-License-Identifier: AGPL-3.0-only

// Package memory implements the key-value store port in process memory (kv = "memory").
//
// Entries live only as long as the process and are not shared between processes, so rate
// limits are per process: with several instances, each one allows the full limit.
package memory

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Store is an in-memory kernel.KeyValueStore. It is safe for concurrent use.
type Store struct {
	clock   kernel.Clock
	mu      sync.Mutex
	entries map[entryKey]entry
}

type entryKey struct{ namespace, key string }

type entry struct {
	value     []byte
	expiresAt time.Time // zero: never
}

func (e entry) live(now time.Time) bool { return e.expiresAt.IsZero() || e.expiresAt.After(now) }

var _ kernel.KeyValueStore = (*Store)(nil)

// New returns an empty store whose entries expire by clock.
func New(clock kernel.Clock) *Store {
	return &Store{clock: clock, entries: map[entryKey]entry{}}
}

func (s *Store) expiry(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return s.clock.Now().Add(ttl)
}

// Get implements kernel.KeyValueStore.
func (s *Store) Get(_ context.Context, namespace, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[entryKey{namespace, key}]
	if !ok || !e.live(s.clock.Now()) {
		return nil, fmt.Errorf("kv/memory: %s/%s: %w", namespace, key, kernel.ErrKeyNotFound)
	}
	return bytes.Clone(e.value), nil
}

// Set implements kernel.KeyValueStore.
func (s *Store) Set(_ context.Context, namespace, key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entryKey{namespace, key}] = entry{value: bytes.Clone(value), expiresAt: s.expiry(ttl)}
	return nil
}

// Delete implements kernel.KeyValueStore.
func (s *Store) Delete(_ context.Context, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, entryKey{namespace, key})
	return nil
}

// IncrBy implements kernel.KeyValueStore.
func (s *Store) IncrBy(_ context.Context, namespace, key string, n int64, ttl time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := entryKey{namespace, key}
	e, ok := s.entries[k]
	if !ok || !e.live(s.clock.Now()) {
		e = entry{value: []byte("0"), expiresAt: s.expiry(ttl)}
	}
	cur, err := strconv.ParseInt(string(e.value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("kv/memory: %s/%s is not a counter: %w", namespace, key, err)
	}
	cur += n
	e.value = []byte(strconv.FormatInt(cur, 10))
	s.entries[k] = e
	return cur, nil
}

// CompareAndSwap implements kernel.KeyValueStore.
func (s *Store) CompareAndSwap(_ context.Context, namespace, key string, old, value []byte, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := entryKey{namespace, key}
	e, ok := s.entries[k]
	ok = ok && e.live(s.clock.Now())
	switch {
	case old == nil && ok, old != nil && (!ok || !bytes.Equal(e.value, old)):
		return false, nil
	case value == nil:
		delete(s.entries, k)
	default:
		s.entries[k] = entry{value: bytes.Clone(value), expiresAt: s.expiry(ttl)}
	}
	return true, nil
}

// DeleteExpired implements kernel.KeyValueStore.
func (s *Store) DeleteExpired(_ context.Context, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	var n int64
	for k, e := range s.entries {
		if n >= int64(limit) {
			break
		}
		if !e.live(now) {
			delete(s.entries, k)
			n++
		}
	}
	return n, nil
}
