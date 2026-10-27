// SPDX-License-Identifier: AGPL-3.0-only

package emf_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/adapter/telemetry/emf"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

type failingWriter struct {
	buffer
	fail bool
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.fail {
		return 0, errors.New("write failed")
	}
	return f.buffer.Write(p)
}

func TestKeepsTheTotalsOfObservableCountersWhenTheWriteFails(t *testing.T) {
	w := &failingWriter{}
	m := emf.New(emf.Options{Writer: w, Namespace: "my", Clock: testkit.NewClock(start)})
	total := 5.0
	m.ObservableCounter(kernel.Instrument{Name: "process.cpu.time", Unit: "s"},
		func(_ context.Context, o kernel.GaugeObserver) error {
			o.Observe(total, kernel.NewLabels())
			return nil
		})
	if err := m.WriteGauges(t.Context()); err != nil {
		t.Fatal(err)
	}
	w.buffer.reset()
	total = 8
	w.fail = true
	if err := m.WriteGauges(t.Context()); err == nil {
		t.Fatal("WriteGauges succeeded although the write failed")
	}
	w.fail = false
	if err := m.WriteGauges(t.Context()); err != nil {
		t.Fatal(err)
	}
	lines := w.buffer.lines(t)
	if len(lines) != 1 || lines[0]["process.cpu.time"] != 3.0 {
		t.Errorf("lines = %v, want the increase of 3 once", lines)
	}
}
