// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// Label keys of the snapshot gauges.
const (
	LabelMediaOrigin   = "my.media.origin"
	LabelSnapshotPart  = "my.metrics.snapshot.part"
	LabelInstanceState = "my.federation.instance.state"
	LabelBillingSource = "my.billing.source"
)

// The values of my.media.origin, my.metrics.snapshot.part, and my.federation.instance.state.
const (
	MediaOriginLocal       = "local"
	MediaOriginRemoteCache = "remote_cache"
	PartQueue              = "queue"
	PartTotals             = "totals"
	// InstanceStateAvailable is a remote server that deliveries still go to and that failed on a
	// day since it last answered; InstanceStateUnavailable one whose deliveries are paused.
	InstanceStateAvailable   = "available"
	InstanceStateUnavailable = "unavailable"
)

// The instance-wide gauges of ADR-0055 §3, reported from the snapshot.
var (
	QueueReady = kernel.Instrument{
		Name: "my.queue.ready", Unit: "{job}", Description: "Jobs of the database queue that are due and that no worker holds.",
		LabelKeys: []string{queue.LabelJobType},
	}
	QueueOldestReadyAge = kernel.Instrument{
		Name: "my.queue.oldest_ready_age", Unit: "s", Description: "Time the oldest ready job of the database queue has waited.",
	}
	QueueDead = kernel.Instrument{
		Name: "my.queue.dead", Unit: "{job}", Description: "Jobs that failed for good.",
	}
	InstanceUsers = kernel.Instrument{
		Name: "my.instance.users", Unit: "{account}", Description: "Local users, as NodeInfo reports them.",
	}
	InstanceStatuses = kernel.Instrument{
		Name: "my.instance.statuses", Unit: "{status}", Description: "Local posts, as NodeInfo reports them.",
	}
	MediaStoredBytes = kernel.Instrument{
		Name: "my.media.stored_bytes", Unit: "By", Description: "Bytes of the media files in blob storage, by origin.",
		LabelKeys: []string{LabelMediaOrigin},
	}
	DBSize = kernel.Instrument{
		Name: "my.db.size", Unit: "By", Description: "Bytes that the database takes in its storage.",
	}
	FederationInstances = kernel.Instrument{
		Name: "my.federation.instances", Unit: "{server}",
		Description: "Remote servers that deliveries failed to reach, by whether deliveries to them are paused.",
		LabelKeys:   []string{LabelInstanceState},
	}
	BillingImportFailing = kernel.Instrument{
		Name: "my.billing.import.failing", Unit: "{source}",
		Description: "1 when the newest import of the usage charges of a billing source failed, 0 otherwise.",
		LabelKeys:   []string{LabelBillingSource},
	}
	SnapshotAge = kernel.Instrument{
		Name: "my.metrics.snapshot.age", Unit: "s", Description: "Age of the parts of the snapshot of the instance-wide gauges.",
		LabelKeys: []string{LabelSnapshotPart},
	}
)

// RegisterGauges registers the gauges of the snapshot on m.
func (s *Service) RegisterGauges(m kernel.Meter) {
	m.Gauge(QueueReady, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Queue != nil {
			for t, n := range snap.Queue.Ready {
				o.Observe(float64(n), kernel.NewLabels(queue.LabelJobType, t))
			}
		}
	}))
	m.Gauge(QueueOldestReadyAge, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Queue != nil {
			o.Observe(snap.Queue.OldestReadyAge, kernel.Labels{})
		}
	}))
	m.Gauge(QueueDead, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Queue != nil {
			o.Observe(float64(snap.Queue.Dead), kernel.Labels{})
		}
	}))
	m.Gauge(InstanceUsers, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Totals != nil {
			o.Observe(float64(snap.Totals.Users), kernel.Labels{})
		}
	}))
	m.Gauge(InstanceStatuses, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Totals != nil {
			o.Observe(float64(snap.Totals.Statuses), kernel.Labels{})
		}
	}))
	m.Gauge(MediaStoredBytes, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Totals != nil {
			o.Observe(float64(snap.Totals.MediaLocalBytes), kernel.NewLabels(LabelMediaOrigin, MediaOriginLocal))
			o.Observe(float64(snap.Totals.MediaRemoteCacheBytes), kernel.NewLabels(LabelMediaOrigin, MediaOriginRemoteCache))
		}
	}))
	m.Gauge(DBSize, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Totals != nil && snap.Totals.DatabaseBytes != nil {
			o.Observe(float64(*snap.Totals.DatabaseBytes), kernel.Labels{})
		}
	}))
	m.Gauge(FederationInstances, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Totals != nil {
			o.Observe(float64(snap.Totals.InstancesFailing), kernel.NewLabels(LabelInstanceState, InstanceStateAvailable))
			o.Observe(float64(snap.Totals.InstancesUnavailable), kernel.NewLabels(LabelInstanceState, InstanceStateUnavailable))
		}
	}))
	m.Gauge(BillingImportFailing, s.gauge(func(snap Snapshot, o kernel.GaugeObserver) {
		if snap.Totals != nil {
			for src, failing := range snap.Totals.BillingImportFailing {
				v := 0.0
				if failing {
					v = 1
				}
				o.Observe(v, kernel.NewLabels(LabelBillingSource, src))
			}
		}
	}))
	m.Gauge(SnapshotAge, func(ctx context.Context, o kernel.GaugeObserver) error {
		snap, err := s.Current(ctx)
		if err != nil {
			return err
		}
		now := s.d.Clock.Now()
		if snap.Queue != nil {
			o.Observe(max(now.Sub(snap.Queue.At), 0).Seconds(), kernel.NewLabels(LabelSnapshotPart, PartQueue))
		}
		if snap.Totals != nil {
			o.Observe(max(now.Sub(snap.Totals.At), 0).Seconds(), kernel.NewLabels(LabelSnapshotPart, PartTotals))
		}
		return nil
	})
}

// gauge returns the GaugeFunc that reports the current snapshot through observe.
func (s *Service) gauge(observe func(Snapshot, kernel.GaugeObserver)) kernel.GaugeFunc {
	return func(ctx context.Context, o kernel.GaugeObserver) error {
		snap, err := s.Current(ctx)
		if err != nil {
			return err
		}
		observe(snap, o)
		return nil
	}
}

// RegisterJobs registers JobSnapshot and its schedule. The job computes the snapshot, writes it
// to the KeyValueStore, and exports it once: through m when it writes the gauges on request
// (kernel.GaugeWriter, emf), and as one structured log line otherwise, from which the platforms
// without a scrape make log-based metrics (ADR-0055 §4).
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler, m kernel.Meter) {
	reg.Register(JobSnapshot, func(ctx context.Context, _ queue.Job) queue.Result {
		snap, err := s.Refresh(ctx)
		if err != nil {
			return queue.Retry(0, err)
		}
		if w, ok := m.(kernel.GaugeWriter); ok {
			if err := w.WriteGauges(ctx); err != nil {
				return queue.Retry(0, err)
			}
			return queue.Done()
		}
		s.d.Logger.InfoContext(ctx, "metrics snapshot", snapshotAttrs(snap)...)
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobSnapshot, Every: SnapshotInterval, Job: queue.NewJob{Type: JobSnapshot}})
}

// snapshotAttrs are the values of snap as log attributes named after the gauges.
func snapshotAttrs(snap Snapshot) []any {
	var attrs []any
	if q := snap.Queue; q != nil {
		ready := make([]any, 0, len(q.Ready))
		for _, t := range slices.Sorted(maps.Keys(q.Ready)) {
			ready = append(ready, slog.Int64(t, q.Ready[t]))
		}
		attrs = append(attrs,
			slog.Group(QueueReady.Name, ready...),
			slog.Float64(QueueOldestReadyAge.Name, q.OldestReadyAge),
			slog.Int64(QueueDead.Name, q.Dead),
		)
	}
	if t := snap.Totals; t != nil {
		attrs = append(attrs,
			slog.Int64(InstanceUsers.Name, t.Users),
			slog.Int64(InstanceStatuses.Name, t.Statuses),
			slog.Group(MediaStoredBytes.Name,
				slog.Int64(MediaOriginLocal, t.MediaLocalBytes), slog.Int64(MediaOriginRemoteCache, t.MediaRemoteCacheBytes)),
			slog.Group(FederationInstances.Name,
				slog.Int64(InstanceStateAvailable, t.InstancesFailing), slog.Int64(InstanceStateUnavailable, t.InstancesUnavailable)),
		)
		if t.DatabaseBytes != nil {
			attrs = append(attrs, slog.Int64(DBSize.Name, *t.DatabaseBytes))
		}
		if len(t.BillingImportFailing) > 0 {
			failing := make([]any, 0, len(t.BillingImportFailing))
			for _, src := range slices.Sorted(maps.Keys(t.BillingImportFailing)) {
				failing = append(failing, slog.Bool(src, t.BillingImportFailing[src]))
			}
			attrs = append(attrs, slog.Group(BillingImportFailing.Name, failing...))
		}
	}
	return attrs
}
