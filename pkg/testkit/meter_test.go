// SPDX-License-Identifier: AGPL-3.0-only

package testkit_test

import (
	"context"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
)

func TestMeterContract(t *testing.T) {
	kerneltest.MeterContract(t, func(*testing.T) kernel.Meter { return testkit.NewMeter() })
}

func TestMeterReportsProgrammingErrors(t *testing.T) {
	m := testkit.NewMeter()
	c := m.Counter(kernel.Instrument{Name: "my.events", Unit: "{event}", LabelKeys: []string{"my.outcome"}})
	c.Add(t.Context(), 2, kernel.NewLabels("my.outcome", "success"))
	if err := m.Err(); err != nil {
		t.Fatalf("Err() = %v after a declared label", err)
	}
	if got := m.Sum("my.events", map[string]string{"my.outcome": "success"}); got != 2 {
		t.Errorf("Sum() = %v, want 2", got)
	}
	c.Add(t.Context(), 1, kernel.NewLabels("account_id", "x"))
	if m.Err() == nil {
		t.Error("Err() = nil after an undeclared label")
	}

	m = testkit.NewMeter()
	m.Counter(kernel.Instrument{Name: "my.events", Unit: "{event}"}).Add(t.Context(), -1, kernel.Labels{})
	if m.Err() == nil {
		t.Error("Err() = nil after a negative counter increment")
	}

	m = testkit.NewMeter()
	m.Gauge(kernel.Instrument{Name: "my.level", Unit: "1"}, func(_ context.Context, o kernel.GaugeObserver) error {
		o.Observe(5, kernel.Labels{})
		return nil
	})
	if err := m.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := m.Sum("my.level", nil); got != 5 {
		t.Errorf("gauge = %v, want 5", got)
	}

	m = testkit.NewMeter()
	total := 3.0
	m.ObservableCounter(kernel.Instrument{Name: "my.cpu", Unit: "s"}, func(_ context.Context, o kernel.GaugeObserver) error {
		o.Observe(total, kernel.Labels{})
		return nil
	})
	if err := m.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	total = 4
	if err := m.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Err(); err != nil {
		t.Fatalf("Err() = %v after a growing total", err)
	}
	if got := m.Measurements("my.cpu", nil); len(got) != 2 || got[1].Value != 4 {
		t.Errorf("observable counter = %v, want 3 then 4", got)
	}
	total = 2
	if err := m.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.Err() == nil {
		t.Error("Err() = nil after an observable counter went down")
	}
}
