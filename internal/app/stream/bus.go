// SPDX-License-Identifier: AGPL-3.0-only

package stream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// MaxPayloadSize is the largest payload a Relay sends on a Bus, in bytes. It fits the payload
// limits of every broker (PostgreSQL NOTIFY's 8,000 bytes, SNS's 256 KiB, Pub/Sub's 10 MB).
const MaxPayloadSize = 1024

// Bus broadcasts payloads to every process that listens on it: the port of the cross-process
// brokers (ADR-0050 §1). Relay uses it to deliver the signals of one process to the streams of
// every other. Every adapter passes testkit.StreamBusContract; MemoryBroker is the in-process
// one, for tests.
//
// A payload is UTF-8 text without NUL bytes, at most MaxPayloadSize bytes; a Bus carries it
// unchanged and never looks into it. Delivery is best effort and at least once: a payload may
// be lost while a listener is broken and may arrive twice, and payloads of different senders
// arrive in any order. A Bus is safe for concurrent use.
//
// Every adapter closes Listener.Broken when its listener may have missed payloads, and the
// Relay then ends the subscriptions of the process, whose clients reconnect and read again: the
// payloads lost or repeated around a break and that reconnect are the accepted semantics, not
// a defect. While a listener is open, the adapters guarantee:
//
//   - postgres (LISTEN / NOTIFY): every payload once; a payload sent while the listening
//     connection is lost is lost, and the lost connection breaks the listener.
//   - valkey (Pub/Sub): at most once; a payload published while the listener is disconnected
//     is lost, and the lost connection breaks the listener.
//   - sns (SNS to one SQS standard queue per listener): at least once, so a payload may arrive
//     twice; the queue keeps payloads for 60 seconds while receives fail, and the listener
//     breaks after 30 seconds of failures or when its queue is gone.
//   - pubsub (one Pub/Sub subscription per listener): at least once, so a payload may arrive
//     twice; a streaming pull that the service closes and the client reopens loses nothing,
//     and the listener breaks when its subscription is gone.
//   - MemoryBroker: every payload once, for tests.
//
// testkit.StreamBusContract checks these with bursts of payloads, cut by a break and not.
type Bus interface {
	// Send hands payload to the broker for every listening process, including this one, and
	// returns once the broker has it. It returns no later than ctx's deadline, with an error,
	// when the broker cannot be reached.
	Send(ctx context.Context, payload []byte) error
	// Listen starts receiving and returns once the listener is ready: a payload sent after
	// Listen returns reaches receive, unless the listener breaks. ctx bounds only the start;
	// the listener lives until Close or until it breaks. Listen creates the per-process
	// resources it needs on the broker (a connection, a queue, a subscription) and returns an
	// error, no later than ctx's deadline, when it cannot. The caller holds at most one
	// listener of a Bus at a time.
	//
	// receive is called from one goroutine at a time and keeps the payload; it must return
	// quickly (Relay only decodes and hands the signal to its Hub) and must not call Close.
	Listen(ctx context.Context, receive func(payload []byte)) (Listener, error)
}

// Listener is one process's open listener on a Bus.
type Listener interface {
	// Broken is closed when the listener may have missed payloads (the connection was lost, the
	// queue or subscription was deleted) and stops receiving. A broken listener still needs
	// Close, to delete what is left of its resources. Close may close Broken, or not.
	Broken() <-chan struct{}
	// Close stops receiving and deletes the per-process resources (queue, subscription,
	// connection): once it returns, receive is not called again. It returns no later than
	// ctx's deadline. Calling it again does nothing and returns nil.
	Close(ctx context.Context) error
}

// NewProcessID returns a new random process ID: 32 lowercase hexadecimal characters. A process
// takes a new one each time it starts and shares it between its Relay (the origin of its
// signals and its liveness marker) and its Bus adapter (the names of its per-process
// resources), so that a sweep can tell the resources of live processes from the ones that
// processes left behind.
func NewProcessID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails
	return hex.EncodeToString(b[:])
}

// Sweeper deletes the per-process resources that processes left behind on a broker: the
// processes that ended without closing their listener (a crash, a frozen and discarded Lambda
// execution environment). Brokers whose per-process resources go away with the process (a
// connection) need none. The job JobSweep drives it (RegisterSweepJob).
type Sweeper interface {
	// Sweep deletes the per-process resources created at least minAge ago whose process is not
	// alive, and returns how many it deleted. alive reports whether a process ID has a live
	// marker; a resource whose process cannot be told is kept, and the error of alive is
	// returned with the count. A resource that is already gone is not an error, so that two
	// sweeps may run at once. Adapters pass testkit.StreamSweeperContract.
	Sweep(ctx context.Context, minAge time.Duration, alive func(ctx context.Context, processID string) (bool, error)) (int, error)
}
