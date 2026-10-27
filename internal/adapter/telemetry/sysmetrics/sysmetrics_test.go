// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics_test

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/kv/memory"
	"github.com/mycroblog/my/internal/adapter/system"
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
)

func checkMeter(t *testing.T, m *testkit.Meter) {
	t.Helper()
	if err := m.Err(); err != nil {
		t.Error(err)
	}
}

func TestDecoratorsReturnTheAdapterWithoutAMeter(t *testing.T) {
	s := memory.New(system.Clock{})
	if got := sysmetrics.KV(s, kernel.NopMeter{}, "kv.memory"); got != kernel.KeyValueStore(s) {
		t.Error("KV wraps the store with NopMeter")
	}
	if sysmetrics.NewConnCounter(nil, "valkey") != nil {
		t.Error("NewConnCounter with a nil Meter is not nil")
	}
}

func TestKV(t *testing.T) {
	m := testkit.NewMeter()
	s := sysmetrics.KV(memory.New(system.Clock{}), m, "kv.memory")
	ctx := t.Context()
	if _, err := s.Get(ctx, "ns", "missing"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Fatalf("Get = %v", err)
	}
	if err := s.Set(ctx, "ns", "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	name := sysmetrics.AdapterOperationDuration.Name
	if got := len(m.Measurements(name, map[string]string{"my.adapter": "kv.memory", "my.operation": "get", "my.outcome": "success"})); got != 1 {
		t.Errorf("a Get of a missing key: %d successes, want 1", got)
	}
	if got := len(m.Measurements(name, map[string]string{"my.operation": "set", "my.outcome": "success"})); got != 1 {
		t.Errorf("Set: %d successes, want 1", got)
	}
	checkMeter(t, m)
}

type failingKV struct{ kernel.KeyValueStore }

func (failingKV) Delete(context.Context, string, string) error { return errors.New("down") }

func TestKVError(t *testing.T) {
	m := testkit.NewMeter()
	s := sysmetrics.KV(failingKV{}, m, "kv.test")
	if err := s.Delete(t.Context(), "ns", "k"); err == nil {
		t.Fatal("Delete succeeded")
	}
	if got := len(m.Measurements(sysmetrics.AdapterOperationDuration.Name, map[string]string{"my.operation": "delete", "my.outcome": "error"})); got != 1 {
		t.Errorf("%d errors, want 1", got)
	}
	checkMeter(t, m)
}

type idleSource struct{ queue.Source }

func (idleSource) Claim(context.Context, string, time.Time, time.Duration) (queue.Job, bool, error) {
	return queue.Job{}, false, nil
}

func TestSourceSkipsIdleClaims(t *testing.T) {
	m := testkit.NewMeter()
	s := sysmetrics.Source(idleSource{}, m, "queue.test")
	if _, ok, err := s.Claim(t.Context(), "w", time.Now(), time.Minute); ok || err != nil {
		t.Fatalf("Claim = %v, %v", ok, err)
	}
	if got := m.Measurements(sysmetrics.AdapterOperationDuration.Name, nil); len(got) != 0 {
		t.Errorf("an idle claim is measured: %v", got)
	}
	checkMeter(t, m)
}

func TestBusCountsTheListener(t *testing.T) {
	m := testkit.NewMeter()
	broker := stream.NewMemoryBroker()
	b := sysmetrics.Bus(broker.Bus(), m, "stream.memory", nil)
	ctx := t.Context()
	conns := func() float64 {
		return m.Sum(sysmetrics.AdapterConnections.Name, map[string]string{"my.adapter": "stream.memory"})
	}
	l, err := b.Listen(ctx, func([]byte) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Send(ctx, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if got := conns(); got != 1 {
		t.Errorf("connections = %v, want 1", got)
	}
	if err := l.Close(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return conns() == 0 })
	l, err = b.Listen(ctx, func([]byte) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close(ctx) }()
	// A broken listener is no longer an open connection.
	broker.Break()
	waitFor(t, func() bool { return conns() == 0 })
	if got := m.Sum(sysmetrics.AdapterReconnects.Name, nil); got != 1 {
		t.Errorf("reconnects = %v, want 1", got)
	}
	if got := len(m.Measurements(sysmetrics.AdapterOperationDuration.Name, map[string]string{"my.operation": "send", "my.outcome": "success"})); got != 1 {
		t.Errorf("%d sends, want 1", got)
	}
	checkMeter(t, m)
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestConnCounter(t *testing.T) {
	m := testkit.NewMeter()
	cc := sysmetrics.NewConnCounter(m, "valkey")
	open := func() float64 {
		return m.Sum(sysmetrics.AdapterConnections.Name, map[string]string{"my.adapter": "valkey"})
	}
	a, b := net.Pipe()
	defer func() { _ = b.Close() }()
	c := cc.Track(t.Context(), a)
	if got := open(); got != 1 {
		t.Errorf("open = %v, want 1", got)
	}
	_ = c.Close()
	_ = c.Close()
	if got := open(); got != 0 {
		t.Errorf("open after Close = %v, want 0", got)
	}
	x, y := net.Pipe()
	defer func() { _ = y.Close() }()
	_ = cc.Track(t.Context(), x).Close()
	if got := m.Sum(sysmetrics.AdapterReconnects.Name, nil); got != 1 {
		t.Errorf("reconnects = %v, want 1", got)
	}
	checkMeter(t, m)
}

func TestRegisterPool(t *testing.T) {
	m := testkit.NewMeter()
	sysmetrics.RegisterPool(m, "main", func() sysmetrics.PoolStats {
		return sysmetrics.PoolStats{Idle: 2, Used: 3, Max: 10, Waits: 4, WaitTime: 1500 * time.Millisecond}
	})
	if err := m.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		match map[string]string
		want  float64
	}{
		{sysmetrics.DBConnectionCount.Name, map[string]string{"db.client.connection.state": "idle"}, 2},
		{sysmetrics.DBConnectionCount.Name, map[string]string{"db.client.connection.state": "used"}, 3},
		{sysmetrics.DBConnectionMax.Name, nil, 10},
		{sysmetrics.DBConnectionWaits.Name, nil, 4},
		{sysmetrics.DBConnectionWaitTime.Name, nil, 1.5},
	} {
		if got := m.Sum(c.name, c.match); got != c.want {
			t.Errorf("%s %v = %v, want %v", c.name, c.match, got, c.want)
		}
	}
	if got := m.Measurements(sysmetrics.DBConnectionTimeouts.Name, nil); len(got) != 0 {
		t.Errorf("timeouts reported for a pool that does not count them: %v", got)
	}
	checkMeter(t, m)
}

func TestRegisterProcess(t *testing.T) {
	m := testkit.NewMeter()
	sysmetrics.RegisterProcess(m, time.Now().Add(-time.Minute))
	if err := m.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := m.Sum(sysmetrics.ProcessUptime.Name, nil); got < 60 {
		t.Errorf("uptime = %v, want >= 60", got)
	}
	if runtime.GOOS == "linux" {
		for _, name := range []string{
			sysmetrics.ProcessMemoryUsage.Name, sysmetrics.ProcessFileDescriptors.Name, sysmetrics.ProcessFileDescriptorLimit.Name,
		} {
			if got := m.Sum(name, nil); got <= 0 {
				t.Errorf("%s = %v, want > 0", name, got)
			}
		}
		if got := m.Measurements(sysmetrics.ProcessCPUTime.Name, nil); len(got) != 2 || !strings.Contains(got[0].Labels["cpu.mode"]+got[1].Labels["cpu.mode"], "user") {
			t.Errorf("cpu time = %v", got)
		}
	}
	checkMeter(t, m)
}
