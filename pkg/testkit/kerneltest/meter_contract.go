// SPDX-License-Identifier: AGPL-3.0-only

package kerneltest

import (
	"context"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
)

// MeterContract verifies a kernel.Meter adapter: it declares every kind of instrument, records
// from many goroutines inside and outside units of work (Begin), survives a label that the
// instrument did not declare, a measurement after its unit of work ended, and a gauge or
// observable counter function that fails, all without a panic or a data race. What the adapter exports is checked by its own
// tests, since every exporter has its own format.
func MeterContract(t *testing.T, newMeter func(t *testing.T) kernel.Meter) {
	t.Helper()
	t.Run("RecordsEveryKind", func(t *testing.T) {
		m := newMeter(t)
		keys := []string{"my.outcome", "my.surface"}
		c := m.Counter(kernel.Instrument{Name: "my.contract.events", Unit: "{event}", Description: "Events.", LabelKeys: keys})
		u := m.UpDownCounter(kernel.Instrument{Name: "my.contract.active", Unit: "{request}", Description: "Active.", LabelKeys: keys})
		h := m.Histogram(kernel.Instrument{
			Name: "my.contract.duration", Unit: "s", Description: "Duration.", LabelKeys: keys, Buckets: kernel.DurationBuckets,
		})
		m.Gauge(kernel.Instrument{Name: "my.contract.level", Unit: "{job}", Description: "Level.", LabelKeys: keys},
			func(_ context.Context, o kernel.GaugeObserver) error {
				o.Observe(3, kernel.NewLabels("my.outcome", "success"))
				return nil
			})
		m.ObservableCounter(kernel.Instrument{Name: "my.contract.cpu", Unit: "s", Description: "CPU.", LabelKeys: keys},
			func(_ context.Context, o kernel.GaugeObserver) error {
				o.Observe(1.5, kernel.NewLabels("my.outcome", "success"))
				return nil
			})
		l := kernel.NewLabels("my.outcome", "success", "my.surface", "client_api")

		var wg sync.WaitGroup
		for n := range 8 {
			wg.Go(func() {
				ctx := t.Context()
				end := func() {}
				if n%2 == 0 {
					ctx, end = m.Begin(ctx)
				}
				for range 50 {
					c.Add(ctx, 1, l)
					u.Add(ctx, 1, l)
					h.Record(ctx, 0.25, l)
					u.Add(ctx, -1, l)
				}
				end()
			})
		}
		wg.Wait()
		collect(t, m)
	})
	t.Run("SurvivesProgrammingErrors", func(t *testing.T) {
		m := newMeter(t)
		i := kernel.Instrument{Name: "my.contract.errors", Unit: "{event}", Description: "Events.", LabelKeys: []string{"my.outcome"}}
		c := m.Counter(i)
		h := m.Histogram(kernel.Instrument{Name: "my.contract.latency", Unit: "s", Description: "Latency."})
		m.Gauge(kernel.Instrument{Name: "my.contract.failing", Unit: "1", Description: "Fails."},
			func(context.Context, kernel.GaugeObserver) error { return context.Canceled })
		m.ObservableCounter(kernel.Instrument{Name: "my.contract.failing_total", Unit: "1", Description: "Fails."},
			func(context.Context, kernel.GaugeObserver) error { return context.Canceled })
		ctx, end := m.Begin(t.Context())
		c.Add(ctx, 1, kernel.NewLabels("my.undeclared", "x"))
		c.Add(ctx, -1, kernel.Labels{})
		h.Record(ctx, 0.5, kernel.Labels{})
		end()
		// After its unit of work ended, a context still records.
		c.Add(ctx, 1, kernel.NewLabels("my.outcome", "success"))
		h.Record(ctx, 0.1, kernel.Labels{})
		collect(t, m)
	})
}

// collect asks a Meter whose exporter does not collect (kernel.GaugeWriter) to write its
// gauges and observable counters, twice, as the periodic job does; the errors of the
// functions are theirs, not the adapter's.
func collect(t *testing.T, m kernel.Meter) {
	t.Helper()
	if w, ok := m.(kernel.GaugeWriter); ok {
		for range 2 {
			_ = w.WriteGauges(t.Context())
		}
	}
}
