// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// ErrorSender sends one error report to an external service (ADR-0103 §4 (3)): the client of an
// exporter adapter of errors_exporter, such as internal/adapter/errorreport/sentry. ExportQueue
// calls it one event at a time from its own goroutine, so it may block until the service
// answers within ctx. It returns nil when the service took the event, and an error otherwise; an
// error whose chain has a Dropped() bool method that reports true means the event was dropped on
// purpose, as under a rate limit of the service, and counts as dropped rather than failed. The
// adapters implement it without importing this package.
type ErrorSender interface {
	SendError(ctx context.Context, ev kernel.ErrorEvent) error
}

// ErrorsExported counts the outcomes of the error reports for the exporter (ADR-0103 §4 (3)).
var ErrorsExported = kernel.Instrument{
	Name: "my.errors.exported", Unit: "{report}",
	Description: "Error reports for the external exporter, by whether they were sent, dropped, or failed.",
	LabelKeys:   []string{"my.outcome"},
}

// The outcomes of ErrorsExported.
const (
	exportSuccess = "success"
	exportDropped = "dropped"
	exportFailed  = "failed"
)

// The send queue of the exporters (ADR-0103 §4 (3)).
const (
	// exportQueueSize is how many reports wait to be sent; the reports beyond it are dropped.
	exportQueueSize = 1000
	// exportDrainTimeout is how long Close waits for the queue to be sent.
	exportDrainTimeout = 2 * time.Second
	// exportSendTimeout bounds the send of one report.
	exportSendTimeout = 10 * time.Second
)

// ExportQueue is the ErrorSinkCloser that every exporter of errors_exporter shares (ADR-0103
// §4 (3)): ReceiveError puts the report in a queue of 1,000 without blocking and drops it when
// the queue is full, one goroutine hands the reports to the ErrorSender one at a time outside
// any transaction, and Close waits up to 2 seconds for the queue to be sent. Each report counts
// once in my.errors.exported as success, dropped, or failed.
type ExportQueue struct {
	sender  ErrorSender
	counter kernel.Counter

	mu     sync.Mutex
	closed bool
	queue  chan kernel.ErrorEvent
	done   chan struct{}
	ctx    context.Context //nolint:containedctx // the sends outlive the requests that report
	cancel context.CancelFunc
}

// NewExportQueue starts the queue that sends through sender and counts with meter.
func NewExportQueue(sender ErrorSender, meter kernel.Meter) *ExportQueue {
	if meter == nil {
		meter = kernel.NopMeter{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &ExportQueue{
		sender: sender, counter: meter.Counter(ErrorsExported),
		queue: make(chan kernel.ErrorEvent, exportQueueSize), done: make(chan struct{}),
		ctx: ctx, cancel: cancel,
	}
	// A nil reporter: the queue never reports its own failures, and send recovers the sender's panics.
	kernel.Go(ctx, "errors.export", nil, q.run)
	return q
}

var _ ErrorSinkCloser = (*ExportQueue)(nil)

// ReceiveError queues ev, or drops it when the queue is full or closed.
func (q *ExportQueue) ReceiveError(ctx context.Context, ev kernel.ErrorEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		select {
		case q.queue <- ev:
			return
		default:
		}
	}
	q.count(ctx, exportDropped)
}

// CountDropped counts a report that never reached the queue, such as one that the limits of the
// exports skipped (exportLimiter), as dropped in my.errors.exported.
func (q *ExportQueue) CountDropped(ctx context.Context) { q.count(ctx, exportDropped) }

// Close stops taking reports and waits until the queue is sent, at most 2 seconds or until ctx
// ends; the reports still waiting then are lost.
func (q *ExportQueue) Close(ctx context.Context) error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.queue)
	}
	q.mu.Unlock()
	timer := time.NewTimer(exportDrainTimeout)
	defer timer.Stop()
	select {
	case <-q.done:
	case <-timer.C:
	case <-ctx.Done():
	}
	// Cancel the send in flight and wait for the goroutine, so that nothing is sent after Close
	// returns; the sender's context ends with the cancel, and ctx bounds the wait.
	q.cancel()
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// closingExportQueue is an ExportQueue whose sender holds resources that shutdown releases
// after the queue has drained, such as the LoggerProvider of the otlp exporter.
type closingExportQueue struct {
	*ExportQueue
	shutdown func(ctx context.Context) error
}

// Close drains the queue, then shuts the sender down within ctx.
func (q *closingExportQueue) Close(ctx context.Context) error {
	_ = q.ExportQueue.Close(ctx)
	return q.shutdown(ctx)
}

func (q *ExportQueue) run(ctx context.Context) {
	defer close(q.done)
	for ev := range q.queue {
		if ctx.Err() != nil {
			q.count(ctx, exportDropped)
			continue
		}
		q.count(ctx, q.send(ctx, ev))
	}
}

// send hands ev to the sender and returns the outcome; a panic of the sender is a failure.
func (q *ExportQueue) send(ctx context.Context, ev kernel.ErrorEvent) (outcome string) {
	defer func() {
		if recover() != nil {
			outcome = exportFailed
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, exportSendTimeout)
	defer cancel()
	err := q.sender.SendError(ctx, ev)
	switch {
	case err == nil:
		return exportSuccess
	case isDropped(err):
		return exportDropped
	default:
		return exportFailed
	}
}

func (q *ExportQueue) count(ctx context.Context, outcome string) {
	q.counter.Add(ctx, 1, kernel.NewLabels("my.outcome", outcome))
}

// isDropped reports whether an error of err's chain says that the event was dropped on purpose.
func isDropped(err error) bool {
	var d interface{ Dropped() bool }
	return errors.As(err, &d) && d.Dropped()
}
