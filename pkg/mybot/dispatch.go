// SPDX-License-Identifier: MIT

package mybot

import (
	"slices"
	"sync"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// job is one event to handle: a notification, a post of the home timeline, or a run of a scheduled job.
type job struct {
	notification *mykit.Notification
	status       *mykit.Status
	// every and at are a run of a scheduled job.
	every *everyJob
	at    time.Time
	// sender is the key of the sender's queue.
	sender string
}

// senderOf is the key of the account that acted, "" for an event without one.
func senderOf(a *mykit.Account) string {
	if a == nil {
		return ""
	}
	return a.Id
}

// dispatcher runs the jobs: up to its limit at once, those of one sender one after the other in the order they
// came (ADR-0094 §4). A notification that is queued or running is not queued again, so that the catch-up and the
// event stream may both bring it.
type dispatcher struct {
	run func(job)
	sem chan struct{}

	mu       sync.Mutex
	queues   map[string][]job
	queued   map[string]struct{}
	stopping bool
	wg       sync.WaitGroup
}

func newDispatcher(concurrency int, run func(job)) *dispatcher {
	return &dispatcher{
		run:    run,
		sem:    make(chan struct{}, concurrency),
		queues: map[string][]job{},
		queued: map[string]struct{}{},
	}
}

// submit queues j and reports whether it did: not after stop, and not for a notification already queued.
func (d *dispatcher) submit(j job) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopping {
		return false
	}
	if j.notification != nil {
		if _, ok := d.queued[j.notification.Id]; ok {
			return false
		}
		d.queued[j.notification.Id] = struct{}{}
	}
	d.queues[j.sender] = append(d.queues[j.sender], j)
	if len(d.queues[j.sender]) == 1 {
		d.wg.Add(1)
		go d.drain(j.sender)
	}
	return true
}

// drain runs the queue of sender until it is empty, or the dispatcher stops.
func (d *dispatcher) drain(sender string) {
	defer d.wg.Done()
	for {
		d.mu.Lock()
		if d.stopping {
			// The jobs that never started are left to the next catch-up.
			delete(d.queues, sender)
			d.mu.Unlock()
			return
		}
		j := d.queues[sender][0]
		d.mu.Unlock()

		d.sem <- struct{}{}
		d.mu.Lock()
		stopping := d.stopping
		d.mu.Unlock()
		if !stopping {
			d.run(j)
		}
		<-d.sem

		d.mu.Lock()
		if j.notification != nil {
			delete(d.queued, j.notification.Id)
		}
		q := d.queues[sender]
		if len(q) <= 1 || stopping {
			delete(d.queues, sender)
			d.mu.Unlock()
			return
		}
		d.queues[sender] = q[1:]
		d.mu.Unlock()
	}
}

// stop makes the dispatcher take no more jobs and start none of those queued.
func (d *dispatcher) stop() {
	d.mu.Lock()
	d.stopping = true
	d.mu.Unlock()
}

// done is closed when every running job has returned.
func (d *dispatcher) done() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(ch)
	}()
	return ch
}

// tracker computes the position of the catch-up (ADR-0094 §3): the newest notification up to which every
// notification handed to the bot was processed. Handlers end out of order, so a processed notification moves the
// position only when no older one is still pending; while a catch-up hands out notifications, the position holds
// still, so that a restart in the middle of it reads them again.
type tracker struct {
	mu      sync.Mutex
	pending map[string]struct{}
	// done are the processed notifications above the position, in order.
	done  []string
	holds int
}

func newTracker() *tracker { return &tracker{pending: map[string]struct{}{}} }

// add marks the notification id pending.
func (t *tracker) add(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[id] = struct{}{}
}

// forget drops a pending notification that was not handed out after all.
func (t *tracker) forget(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, id)
}

// complete marks the notification id processed and returns the new position, "" when it does not move.
func (t *tracker) complete(id string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, id)
	if i, found := slices.BinarySearch(t.done, id); !found {
		t.done = slices.Insert(t.done, i, id)
	}
	return t.advance()
}

// hold keeps the position where it is until release.
func (t *tracker) hold() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.holds++
}

// release ends a hold and returns the new position, "" when it does not move.
func (t *tracker) release() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.holds--
	return t.advance()
}

// advance moves the position past the processed notifications older than every pending one; t.mu is held.
// Notification IDs are UUIDv7 strings, whose order is the order of time.
func (t *tracker) advance() string {
	if t.holds > 0 || len(t.done) == 0 {
		return ""
	}
	oldest := ""
	for id := range t.pending {
		if oldest == "" || id < oldest {
			oldest = id
		}
	}
	n := len(t.done)
	if oldest != "" {
		n, _ = slices.BinarySearch(t.done, oldest)
	}
	if n == 0 {
		return ""
	}
	pos := t.done[n-1]
	t.done = slices.Delete(t.done, 0, n)
	return pos
}
