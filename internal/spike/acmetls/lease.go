// SPDX-License-Identifier: AGPL-3.0-only

package acmetls

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Lease is a named lease over KeyValueStore.CompareAndSwap: the holder writes a random token with
// a TTL, renews it while it works, and removes it when done. A holder that dies leaves the entry
// to expire, so the next process takes over after at most TTL. The store must be shared by every
// process (kv = db or valkey); kv = memory gives each process its own lease.
type Lease struct {
	KV        kernel.KeyValueStore
	Namespace string
	TTL       time.Duration
}

// TryAcquire takes the lease name when no live entry holds it and returns the token that proves it.
func (l *Lease) TryAcquire(ctx context.Context, name string) ([]byte, bool, error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := []byte(hex.EncodeToString(b))
	ok, err := l.KV.CompareAndSwap(ctx, l.Namespace, name, nil, token, l.TTL)
	if err != nil || !ok {
		return nil, false, err
	}
	return token, true, nil
}

// Renew extends the lease while token still holds it; false means it was lost (expired and taken).
func (l *Lease) Renew(ctx context.Context, name string, token []byte) (bool, error) {
	return l.KV.CompareAndSwap(ctx, l.Namespace, name, token, token, l.TTL)
}

// Release removes the lease if token still holds it.
func (l *Lease) Release(ctx context.Context, name string, token []byte) error {
	_, err := l.KV.CompareAndSwap(ctx, l.Namespace, name, token, nil, 0)
	return err
}

// Hold runs fn while holding the lease, renewing it every TTL/3. fn's context is canceled when the
// lease is lost, so that a process that stalled longer than the TTL stops ordering. It reports
// false without running fn when another process holds the lease.
func (l *Lease) Hold(ctx context.Context, name string, fn func(ctx context.Context) error) (bool, error) {
	token, ok, err := l.TryAcquire(ctx, name)
	if err != nil || !ok {
		return false, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(l.TTL / 3)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if ok, err := l.Renew(ctx, name, token); err != nil || !ok {
					cancel()
					return
				}
			}
		}
	}()
	err = fn(ctx)
	close(done)
	_ = l.Release(context.WithoutCancel(ctx), name, token)
	return true, err
}
