// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/stream"
)

// KV measures the operations of the key-value store s, whose adapter is named adapter (such
// as kv.valkey). A Get of a missing key is a success.
func KV(s kernel.KeyValueStore, m kernel.Meter, adapter string) kernel.KeyValueStore {
	if kernel.IsNop(m) {
		return s
	}
	return &kv{next: s, ops: newOperations(m, adapter)}
}

type kv struct {
	next kernel.KeyValueStore
	ops  operations
}

func keyNotFound(err error) bool { return errors.Is(err, kernel.ErrKeyNotFound) }

func (s *kv) Get(ctx context.Context, namespace, key string) ([]byte, error) {
	start := time.Now()
	v, err := s.next.Get(ctx, namespace, key)
	s.ops.done(ctx, "get", start, err, keyNotFound)
	return v, err
}

func (s *kv) Set(ctx context.Context, namespace, key string, value []byte, ttl time.Duration) error {
	start := time.Now()
	err := s.next.Set(ctx, namespace, key, value, ttl)
	s.ops.done(ctx, "set", start, err, nil)
	return err
}

func (s *kv) Delete(ctx context.Context, namespace, key string) error {
	start := time.Now()
	err := s.next.Delete(ctx, namespace, key)
	s.ops.done(ctx, "delete", start, err, nil)
	return err
}

func (s *kv) IncrBy(ctx context.Context, namespace, key string, n int64, ttl time.Duration) (int64, error) {
	start := time.Now()
	v, err := s.next.IncrBy(ctx, namespace, key, n, ttl)
	s.ops.done(ctx, "incr_by", start, err, nil)
	return v, err
}

func (s *kv) CompareAndSwap(ctx context.Context, namespace, key string, old, value []byte, ttl time.Duration) (bool, error) {
	start := time.Now()
	ok, err := s.next.CompareAndSwap(ctx, namespace, key, old, value, ttl)
	s.ops.done(ctx, "compare_and_swap", start, err, nil)
	return ok, err
}

func (s *kv) DeleteExpired(ctx context.Context, limit int) (int64, error) {
	start := time.Now()
	v, err := s.next.DeleteExpired(ctx, limit)
	s.ops.done(ctx, "delete_expired", start, err, nil)
	return v, err
}

// Blobs measures the operations of the blob store s, whose adapter is named adapter (such as
// blob.fs). A Get or Head of a missing blob is a success; a Get measures the time until the
// blob can be read, not the read.
func Blobs(s blob.Store, m kernel.Meter, adapter string) blob.Store {
	if kernel.IsNop(m) {
		return s
	}
	return &blobs{next: s, ops: newOperations(m, adapter)}
}

type blobs struct {
	next blob.Store
	ops  operations
}

func blobNotFound(err error) bool { return errors.Is(err, blob.ErrNotFound) }

func (s *blobs) Put(ctx context.Context, key string, r io.Reader, opts blob.PutOptions) (blob.Info, error) {
	start := time.Now()
	info, err := s.next.Put(ctx, key, r, opts)
	s.ops.done(ctx, "put", start, err, nil)
	return info, err
}

func (s *blobs) Get(ctx context.Context, key string) (io.ReadCloser, blob.Info, error) {
	start := time.Now()
	rc, info, err := s.next.Get(ctx, key)
	s.ops.done(ctx, "get", start, err, blobNotFound)
	return rc, info, err
}

func (s *blobs) Head(ctx context.Context, key string) (blob.Info, error) {
	start := time.Now()
	info, err := s.next.Head(ctx, key)
	s.ops.done(ctx, "head", start, err, blobNotFound)
	return info, err
}

func (s *blobs) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := s.next.Delete(ctx, key)
	s.ops.done(ctx, "delete", start, err, nil)
	return err
}

func (s *blobs) PublicURL(key string) string { return s.next.PublicURL(key) }

// Jobs measures the enqueues of the job queue q, whose adapter is named adapter (such as
// queue.db).
func Jobs(q queue.JobQueue, m kernel.Meter, adapter string) queue.JobQueue {
	if kernel.IsNop(m) {
		return q
	}
	return &jobs{next: q, ops: newOperations(m, adapter)}
}

type jobs struct {
	next queue.JobQueue
	ops  operations
}

func (q *jobs) Enqueue(ctx context.Context, js ...queue.NewJob) error {
	start := time.Now()
	err := q.next.Enqueue(ctx, js...)
	q.ops.done(ctx, "enqueue", start, err, nil)
	return err
}

// Source measures the operations of the pull queue s, whose adapter is named adapter (such as
// queue.db). A lease lost to another worker is a success: the queue answered. A claim that
// finds no due job is not measured, so that an idle worker's polls record nothing (an
// exporter that writes each measurement, emf, would write a line per poll).
func Source(s queue.Source, m kernel.Meter, adapter string) queue.Source {
	if kernel.IsNop(m) {
		return s
	}
	return &source{next: s, ops: newOperations(m, adapter)}
}

type source struct {
	next queue.Source
	ops  operations
}

func leaseLost(err error) bool { return errors.Is(err, queue.ErrLeaseLost) }

func (s *source) Claim(ctx context.Context, worker string, now time.Time, lease time.Duration) (queue.Job, bool, error) {
	start := time.Now()
	j, ok, err := s.next.Claim(ctx, worker, now, lease)
	if ok || err != nil {
		s.ops.done(ctx, "claim", start, err, nil)
	}
	return j, ok, err
}

// ClaimJob is measured as the operation claim, whatever its outcome: it answers a signal, not a
// poll.
func (s *source) ClaimJob(ctx context.Context, id, worker string, now time.Time, lease time.Duration) (queue.ClaimResult, error) {
	start := time.Now()
	r, err := s.next.ClaimJob(ctx, id, worker, now, lease)
	s.ops.done(ctx, "claim", start, err, nil)
	return r, err
}

func (s *source) Complete(ctx context.Context, worker string, j queue.Job) error {
	start := time.Now()
	err := s.next.Complete(ctx, worker, j)
	s.ops.done(ctx, "complete", start, err, leaseLost)
	return err
}

func (s *source) Release(ctx context.Context, worker string, j queue.Job, attempt int, runAt time.Time, lastErr string) error {
	start := time.Now()
	err := s.next.Release(ctx, worker, j, attempt, runAt, lastErr)
	s.ops.done(ctx, "release", start, err, leaseLost)
	return err
}

func (s *source) Bury(ctx context.Context, worker string, j queue.Job, lastErr string, failedAt time.Time) error {
	start := time.Now()
	err := s.next.Bury(ctx, worker, j, lastErr, failedAt)
	s.ops.done(ctx, "bury", start, err, leaseLost)
	return err
}

// Bus measures the sends and listens of the stream bus b, whose adapter is named adapter (such
// as stream.postgres), and counts its listener as a long-lived connection: open from Listen
// until it breaks or closes, and a reconnect when Listen follows an earlier listener.
// Sweeping is not measured; take the Sweeper from b before wrapping it. reporter receives a
// panic of the goroutine that waits for the listener's end (kernel.Go).
func Bus(b stream.Bus, m kernel.Meter, adapter string, reporter kernel.ErrorReporter) stream.Bus {
	if kernel.IsNop(m) {
		return b
	}
	return &bus{
		next: b, ops: newOperations(m, adapter),
		conns: m.UpDownCounter(AdapterConnections), reconnects: m.Counter(AdapterReconnects),
		labels: kernel.NewLabels(LabelAdapter, adapter), reporter: reporter,
	}
}

type bus struct {
	next       stream.Bus
	ops        operations
	conns      kernel.UpDownCounter
	reconnects kernel.Counter
	labels     kernel.Labels
	reporter   kernel.ErrorReporter

	mu       sync.Mutex
	listened bool
}

func (b *bus) Send(ctx context.Context, payload []byte) error {
	start := time.Now()
	err := b.next.Send(ctx, payload)
	b.ops.done(ctx, "send", start, err, nil)
	return err
}

func (b *bus) Listen(ctx context.Context, receive func(payload []byte)) (stream.Listener, error) {
	start := time.Now()
	l, err := b.next.Listen(ctx, receive)
	b.ops.done(ctx, "listen", start, err, nil)
	if err != nil {
		return l, err
	}
	b.mu.Lock()
	again := b.listened
	b.listened = true
	b.mu.Unlock()
	ctx = context.WithoutCancel(ctx)
	if again {
		b.reconnects.Add(ctx, 1, b.labels)
	}
	b.conns.Add(ctx, 1, b.labels)
	tl := &listener{Listener: l, closed: make(chan struct{})}
	kernel.Go(ctx, "stream.metrics.listener", b.reporter, func(ctx context.Context) {
		select {
		case <-l.Broken():
		case <-tl.closed:
		}
		b.conns.Add(ctx, -1, b.labels)
	})
	return tl, nil
}

// listener ends the count of its connection when it closes.
type listener struct {
	stream.Listener
	once   sync.Once
	closed chan struct{}
}

func (l *listener) Close(ctx context.Context) error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close(ctx)
}
