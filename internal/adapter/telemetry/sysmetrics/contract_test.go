// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics_test

import (
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/kv/memory"
	"github.com/mycroblog/my/internal/adapter/queue/inline"
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit"
)

// The decorators stand in for the adapters they wrap, so each passes the contract of its port
// on an in-memory adapter, with a Meter that records and reports misuse.

func recordingMeter(t *testing.T) *testkit.Meter {
	t.Helper()
	m := testkit.NewMeter()
	t.Cleanup(func() { checkMeter(t, m) })
	return m
}

func TestKVContract(t *testing.T) {
	testkit.KeyValueStoreContract(t, func(t *testing.T, clock *testkit.Clock) kernel.KeyValueStore {
		return sysmetrics.KV(memory.New(clock), recordingMeter(t), "kv.memory")
	})
}

func TestBlobsContract(t *testing.T) {
	testkit.BlobStoreContract(t, func(t *testing.T) blob.Store {
		store := testkit.NewMemoryBlobStore(testkit.NewClock(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
		return sysmetrics.Blobs(store, recordingMeter(t), "blob.memory")
	})
}

func TestJobsAndSourceContract(t *testing.T) {
	testkit.QueueContract(t, func(t *testing.T, clock *testkit.Clock) testkit.QueueHarness {
		q := inline.New(inline.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)})
		m := recordingMeter(t)
		return testkit.QueueHarness{
			Queue:    sysmetrics.Jobs(q, m, "queue.inline"),
			Source:   sysmetrics.Source(q, m, "queue.inline"),
			DeadJobs: q,
		}
	})
}

func TestBusContract(t *testing.T) {
	testkit.StreamBusContract(t, func(t *testing.T) testkit.StreamBus {
		broker := stream.NewMemoryBroker()
		m := recordingMeter(t)
		return testkit.StreamBus{
			NewBus: func(*testing.T) stream.Bus { return sysmetrics.Bus(broker.Bus(), m, "stream.memory", nil) },
			Unreachable: func(*testing.T) stream.Bus {
				unreachable := stream.NewMemoryBroker()
				unreachable.SetUnreachable(true)
				return sysmetrics.Bus(unreachable.Bus(), m, "stream.memory", nil)
			},
			Break:     func(*testing.T) { broker.Break() },
			Resources: func(*testing.T) int { return broker.Listeners() },
		}
	})
}
