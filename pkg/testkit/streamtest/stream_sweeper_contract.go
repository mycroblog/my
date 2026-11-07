// SPDX-License-Identifier: AGPL-3.0-only

package streamtest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
)

// SweepingBus is a stream.Bus adapter that also sweeps what processes leave on its broker.
type SweepingBus interface {
	stream.Bus
	stream.Sweeper
}

// StreamSweeper is what StreamSweeperContract needs of a stream.Sweeper adapter on one broker.
type StreamSweeper struct {
	// NewBus returns a new Bus on the broker, standing for one process, and the ID of that
	// process. It registers the cleanup of what it creates with t.Cleanup. Required.
	NewBus func(t *testing.T) (SweepingBus, string)
	// Resources returns the number of the per-process resources on the broker (listening
	// queues, subscriptions) of the process. Required.
	Resources func(t *testing.T, processID string) int
	// Advance, when not nil, moves the time that the broker's resources are created at
	// forward. Without it the contract sweeps with a minAge of zero and does not check that
	// young resources are kept.
	Advance func(d time.Duration)
	// Timeout bounds each wait for Broken; zero means 5 seconds.
	Timeout time.Duration
}

// StreamSweeperFactory returns the harness of a new broker, or of a part of a shared one that no
// other test uses.
type StreamSweeperFactory func(t *testing.T) StreamSweeper

// StreamSweeperContract verifies a stream.Sweeper adapter: Sweep deletes the resources of the
// processes that are not alive, reports how many it deleted, and cuts their listeners; it
// keeps the resources of live processes, of processes whose liveness cannot be told (and
// returns that error), and of resources younger than minAge; a second sweep deletes nothing;
// and two sweeps may run at once.
func StreamSweeperContract(t *testing.T, newSweeper StreamSweeperFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, h StreamSweeper)
	}{
		{"SweepsEndedProcesses", sweeperSweepsEndedProcesses},
		{"SweepsConcurrently", sweeperSweepsConcurrently},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newSweeper(t)
			if h.Timeout <= 0 {
				h.Timeout = defaultStreamTimeout
			}
			tc.run(t, h)
		})
	}
}

// sweptProcess is one process of a sweeper case.
type sweptProcess struct {
	bus      SweepingBus
	id       string
	listener stream.Listener
}

func (h StreamSweeper) process(t *testing.T, listen bool) sweptProcess {
	t.Helper()
	bus, id := h.NewBus(t)
	p := sweptProcess{bus: bus, id: id}
	if listen {
		p.listener = busListen(t, StreamBus{Timeout: h.Timeout}, bus, newBusCollector())
		if h.Resources(t, id) == 0 {
			t.Fatalf("a listening process %s has no resources", id)
		}
	}
	return p
}

func (h StreamSweeper) sweep(t *testing.T, s SweepingBus, minAge time.Duration, alive func(context.Context, string) (bool, error)) (int, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 4*h.Timeout)
	defer cancel()
	return s.Sweep(ctx, minAge, alive)
}

func sweeperSweepsEndedProcesses(t *testing.T, h StreamSweeper) {
	live, dead, unknown := h.process(t, true), h.process(t, true), h.process(t, true)
	minAge := time.Duration(0)
	var young *sweptProcess
	if h.Advance != nil {
		minAge = 5 * time.Minute
		h.Advance(minAge)
		p := h.process(t, true)
		young = &p
		h.Advance(time.Minute)
	}
	errUnknown := errors.New("liveness store unavailable")
	alive := func(_ context.Context, id string) (bool, error) {
		switch id {
		case live.id:
			return true, nil
		case unknown.id:
			return false, errUnknown
		}
		return false, nil
	}
	kept := []sweptProcess{live, unknown}
	if young != nil {
		kept = append(kept, *young)
	}
	before := map[string]int{}
	for _, p := range kept {
		before[p.id] = h.Resources(t, p.id)
	}
	deadResources := h.Resources(t, dead.id)

	sweeper := h.process(t, false)
	n, err := h.sweep(t, sweeper.bus, minAge, alive)
	if !errors.Is(err, errUnknown) {
		t.Errorf("Sweep error = %v, want the error of alive", err)
	}
	if n != deadResources {
		t.Errorf("Sweep deleted %d, want the %d resources of the ended process", n, deadResources)
	}
	if got := h.Resources(t, dead.id); got != 0 {
		t.Errorf("the ended process has %d resources after the sweep, want 0", got)
	}
	for _, p := range kept {
		if got := h.Resources(t, p.id); got != before[p.id] {
			t.Errorf("process %s has %d resources after the sweep, want %d", p.id, got, before[p.id])
		}
	}
	select {
	case <-dead.listener.Broken():
	case <-time.After(h.Timeout):
		t.Error("the listener whose resources were swept did not break")
	}
	for _, p := range kept {
		select {
		case <-p.listener.Broken():
			t.Errorf("the listener of the kept process %s broke", p.id)
		default:
		}
	}

	// Sweeping again deletes nothing more.
	if n, _ := h.sweep(t, sweeper.bus, minAge, alive); n != 0 {
		t.Errorf("a second Sweep deleted %d, want 0", n)
	}
}

func sweeperSweepsConcurrently(t *testing.T, h StreamSweeper) {
	dead := []sweptProcess{h.process(t, true), h.process(t, true), h.process(t, true)}
	live := h.process(t, true)
	liveResources := h.Resources(t, live.id)
	if h.Advance != nil {
		h.Advance(time.Minute)
	}
	alive := func(_ context.Context, id string) (bool, error) { return id == live.id, nil }
	sweepers := []sweptProcess{h.process(t, false), h.process(t, false)}
	var wg sync.WaitGroup
	errs := make([]error, len(sweepers))
	for i, s := range sweepers {
		wg.Go(func() { _, errs[i] = h.sweep(t, s.bus, 0, alive) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent Sweep %d = %v, want no error for resources already gone", i, err)
		}
	}
	for _, p := range dead {
		if got := h.Resources(t, p.id); got != 0 {
			t.Errorf("the ended process %s has %d resources after the sweeps, want 0", p.id, got)
		}
	}
	if got := h.Resources(t, live.id); got != liveResources {
		t.Errorf("the live process has %d resources after the sweeps, want %d", got, liveResources)
	}
}
