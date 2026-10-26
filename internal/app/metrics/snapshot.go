// SPDX-License-Identifier: AGPL-3.0-only

// Package metrics keeps the snapshot of the instance-wide gauges of the service metrics
// (ADR-0055 §4): the queue backlog, the local users and posts, the bytes of the media files and
// of the database, and the remote servers that deliveries fail to reach.
// They come from the database, and every process reports the same values: one process computes
// them and puts them in the KeyValueStore, and the others read them from there.
//
// A process reads the snapshot when the exporter collects, at most once per CacheTTL, and never
// queries the database for it. When a part of the snapshot is older than its limit (QueueMaxAge,
// TotalsMaxAge), the process that takes the lock in the KeyValueStore computes that part again;
// the others report the older values. The periodic job JobSnapshot computes every part, so that
// the targets without a scrape (Cloud Run, Lambda) get the values in their logs.
package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

const (
	// Namespace is the KeyValueStore namespace of the snapshot and its lock.
	Namespace   = "metrics"
	snapshotKey = "snapshot"
	lockKey     = "lock"

	// QueueMaxAge is how old the queue part of the snapshot may get before a collection
	// computes it again.
	QueueMaxAge = 30 * time.Second
	// TotalsMaxAge is how old the counts and sizes may get before a collection computes them
	// again.
	TotalsMaxAge = 15 * time.Minute
	// CacheTTL is how long a process reuses the snapshot it read from the KeyValueStore.
	CacheTTL = 15 * time.Second
	// SnapshotTTL is how long the snapshot stays in the KeyValueStore: long enough that the
	// processes report older values while another one computes them again.
	SnapshotTTL = time.Hour
	// LockTTL bounds how long a process that stopped while computing holds the others back.
	LockTTL = time.Minute

	// JobSnapshot computes the snapshot, writes it to the KeyValueStore, and exports it once;
	// no payload.
	JobSnapshot = "metrics.snapshot"
	// SnapshotInterval is the period of the schedule of JobSnapshot.
	SnapshotInterval = 15 * time.Minute
)

// Totals are the counts and sizes of the instance.
type Totals struct {
	// Users and Statuses are the local users and posts, as NodeInfo's usage reports them.
	Users    int64 `json:"users"`
	Statuses int64 `json:"statuses"`
	// MediaLocalBytes and MediaRemoteCacheBytes are the bytes of the local uploads and of the
	// cache of remote media files.
	MediaLocalBytes       int64 `json:"media_local_bytes"`
	MediaRemoteCacheBytes int64 `json:"media_remote_cache_bytes"`
	// DatabaseBytes is the size of the database; nil when the server does not measure it.
	DatabaseBytes *int64 `json:"database_bytes,omitempty"`
	// InstancesFailing are the remote servers that deliveries still go to and that failed on a
	// day since they last answered; InstancesUnavailable those whose deliveries are paused.
	InstancesFailing     int64 `json:"instances_failing"`
	InstancesUnavailable int64 `json:"instances_unavailable"`
	// BillingImportFailing tells, for every enabled billing source, whether its newest import
	// attempt failed.
	BillingImportFailing map[string]bool `json:"billing_import_failing,omitempty"`
}

// DatabaseSizer measures the database. The database adapters implement it and pass
// testkit.DatabaseSizerContract.
type DatabaseSizer interface {
	// DatabaseSize returns the bytes that the database takes in its storage. It is cheap enough
	// to run once per TotalsMaxAge.
	DatabaseSize(ctx context.Context) (int64, error)
}

// QueuePart is the queue part of a snapshot.
type QueuePart struct {
	At time.Time `json:"at"`
	// Ready counts the ready jobs by job type, the types without a handler as
	// queue.JobTypeOther.
	Ready map[string]int64 `json:"ready"`
	// OldestReadyAge is how long the oldest ready job had waited at At, in seconds.
	OldestReadyAge float64 `json:"oldest_ready_age"`
	Dead           int64   `json:"dead"`
}

// TotalsPart is the counts and sizes part of a snapshot.
type TotalsPart struct {
	At time.Time `json:"at"`
	Totals
}

// Snapshot is the instance-wide gauges at the times of its parts. A nil part has no values:
// it was never computed, or the queue is not one the server counts.
type Snapshot struct {
	Queue  *QueuePart  `json:"queue,omitempty"`
	Totals *TotalsPart `json:"totals,omitempty"`
}

// Deps are the dependencies of a Service. KV, Clock, and Totals are required.
type Deps struct {
	KV    kernel.KeyValueStore
	Clock kernel.Clock
	// Backlog reads the queue; nil when the queue is not one the server counts (the
	// platforms' queues have metrics of their own).
	Backlog queue.BacklogReader
	// KnownJobType reports whether a job type has a handler; nil accepts every type.
	KnownJobType func(jobType string) bool
	// Totals computes the counts and sizes from the database.
	Totals func(ctx context.Context) (Totals, error)
	// Logger receives the failures to compute the snapshot and the line of JobSnapshot; the
	// default is slog.Default().
	Logger *slog.Logger
}

// Service keeps the snapshot of one process.
type Service struct {
	d Deps

	mu     sync.Mutex // serializes the reads, so that one collection reads the store once
	cached Snapshot
	readAt time.Time
}

// NewService returns a Service on d.
func NewService(d Deps) *Service {
	if d.KV == nil || d.Clock == nil || d.Totals == nil {
		panic("metrics: Deps.KV, Deps.Clock, and Deps.Totals are required")
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{d: d}
}

// Current returns the snapshot to report: the one this process read less than CacheTTL ago,
// or the one in the KeyValueStore, with its stale parts computed again when this process
// takes the lock. Without a snapshot in the store and the lock, it returns an empty snapshot,
// whose gauges report nothing rather than zeros.
func (s *Service) Current(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.d.Clock.Now()
	if !s.readAt.IsZero() && now.Sub(s.readAt) < CacheTTL {
		return s.cached, nil
	}
	snap, err := s.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	queueStale, totalsStale := s.stale(snap, now)
	if queueStale || totalsStale {
		snap = s.refreshLocked(ctx, snap, queueStale, totalsStale)
	}
	s.cached, s.readAt = snap, now
	return snap, nil
}

// stale reports which parts of snap are missing or older than their limits at now.
func (s *Service) stale(snap Snapshot, now time.Time) (queueStale, totalsStale bool) {
	queueStale = s.d.Backlog != nil && (snap.Queue == nil || now.Sub(snap.Queue.At) >= QueueMaxAge)
	totalsStale = snap.Totals == nil || now.Sub(snap.Totals.At) >= TotalsMaxAge
	return queueStale, totalsStale
}

// refreshLocked computes the stale parts of snap when this process takes the lock, and
// otherwise returns snap as it is.
func (s *Service) refreshLocked(ctx context.Context, snap Snapshot, queueStale, totalsStale bool) Snapshot {
	n, err := s.d.KV.IncrBy(ctx, Namespace, lockKey, 1, LockTTL)
	if err != nil {
		s.d.Logger.WarnContext(ctx, "metrics: take the snapshot lock", "error", err)
		return snap
	}
	if n != 1 {
		return snap // another process computes it
	}
	defer func() {
		if err := s.d.KV.Delete(context.WithoutCancel(ctx), Namespace, lockKey); err != nil {
			s.d.Logger.WarnContext(ctx, "metrics: release the snapshot lock", "error", err)
		}
	}()
	fresh, err := s.compute(ctx, snap, queueStale, totalsStale)
	if err != nil {
		s.d.Logger.WarnContext(ctx, "metrics: compute the snapshot", "error", err)
	}
	if fresh.Queue == snap.Queue && fresh.Totals == snap.Totals {
		return snap // nothing computed
	}
	if err := s.write(ctx, fresh); err != nil {
		s.d.Logger.WarnContext(ctx, "metrics: write the snapshot", "error", err)
	}
	return fresh
}

// Refresh computes every part of the snapshot, writes it to the KeyValueStore, and returns
// it. The periodic job calls it without the lock: a concurrent computation writes the same
// values.
func (s *Service) Refresh(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.compute(ctx, Snapshot{}, s.d.Backlog != nil, true)
	if err != nil {
		return snap, err
	}
	if err := s.write(ctx, snap); err != nil {
		return snap, err
	}
	s.cached, s.readAt = snap, s.d.Clock.Now()
	return snap, nil
}

// compute returns snap with the requested parts computed again; a part that fails keeps its
// older values, and the error reports it.
func (s *Service) compute(ctx context.Context, snap Snapshot, queuePart, totalsPart bool) (Snapshot, error) {
	var errs []error
	if queuePart {
		now := s.d.Clock.Now()
		b, err := s.d.Backlog.Backlog(ctx, now)
		if err != nil {
			errs = append(errs, err)
		} else {
			snap.Queue = s.queuePart(b, now)
		}
	}
	if totalsPart {
		now := s.d.Clock.Now()
		t, err := s.d.Totals(ctx)
		if err != nil {
			errs = append(errs, err)
		} else {
			snap.Totals = &TotalsPart{At: now, Totals: t}
		}
	}
	return snap, errors.Join(errs...)
}

func (s *Service) queuePart(b queue.Backlog, now time.Time) *QueuePart {
	p := &QueuePart{At: now, Ready: make(map[string]int64, len(b.Ready)), Dead: b.Dead}
	for t, n := range b.Ready {
		if s.d.KnownJobType != nil && !s.d.KnownJobType(t) {
			t = queue.JobTypeOther
		}
		p.Ready[t] += n
	}
	if !b.OldestReadyAt.IsZero() {
		p.OldestReadyAge = max(now.Sub(b.OldestReadyAt), 0).Seconds()
	}
	return p
}

func (s *Service) read(ctx context.Context) (Snapshot, error) {
	v, err := s.d.KV.Get(ctx, Namespace, snapshotKey)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("metrics: read the snapshot: %w", err)
	}
	var snap Snapshot
	if json.Unmarshal(v, &snap) != nil {
		// A snapshot of another version of the server is a missing one: compute it again.
		return Snapshot{}, nil
	}
	return snap, nil
}

func (s *Service) write(ctx context.Context, snap Snapshot) error {
	v, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return s.d.KV.Set(ctx, Namespace, snapshotKey, v, SnapshotTTL)
}
