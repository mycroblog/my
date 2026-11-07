// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"regexp"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

const contractHost = "social.example"

// eventLog records the graph events that commands emit (committed or not).
type eventLog struct {
	mu     sync.Mutex
	events []uow.Event
}

// record is a projector that logs e.
func (l *eventLog) record(_ context.Context, _ uow.Tx, e uow.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
	return nil
}

func (l *eventLog) take() []uow.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.events
	l.events = nil
	return out
}

// recordingOutbox is a queue.Outbox that keeps the jobs it is given.
type recordingOutbox struct {
	mu   sync.Mutex
	jobs []queue.NewJob
}

func (o *recordingOutbox) EnqueueTx(_ context.Context, _ uow.Tx, jobs ...queue.NewJob) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.jobs = append(o.jobs, jobs...)
	return nil
}

func (o *recordingOutbox) take() []queue.NewJob {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.jobs
	o.jobs = nil
	return out
}

// remoteSanitizer stands in for the HTML sanitizer: it drops script elements and records
// version 7, so that the contract sees that remote HTML went through it.
type remoteSanitizer struct{}

var (
	remoteScripts = regexp.MustCompile(`(?s)<script.*?</script>`)
	remoteTags    = regexp.MustCompile(`<[^>]*>`)
)

func (remoteSanitizer) SanitizeHTML(s string) string { return remoteScripts.ReplaceAllString(s, "") }

func (remoteSanitizer) PlainText(s string) string { return remoteTags.ReplaceAllString(s, "") }

func (remoteSanitizer) PolicyVersion() int { return 7 }

func sortedIDs(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

// rendezvous lets each of n transactions wait, inside the transaction, until all n reach it or
// patience runs out. A transaction that waits for a lock that another one holds never reaches
// it, so patience bounds the wait; the transactions then go on in the order the database allows.
type rendezvous struct {
	arrived  chan struct{}
	n        int
	patience time.Duration
}

func newRendezvous(n int) *rendezvous {
	return &rendezvous{arrived: make(chan struct{}, n), n: n, patience: 300 * time.Millisecond}
}

func (r *rendezvous) wait() {
	r.arrived <- struct{}{}
	deadline := time.After(r.patience)
	for {
		if len(r.arrived) == r.n {
			return
		}
		select {
		case <-deadline:
			return
		case <-time.After(time.Millisecond):
		}
	}
}

// inTransactionOf registers on projectors a projector of eventType that runs check in the
// transaction of each such event, and returns how many times it ran.
func inTransactionOf(projectors *uow.Projectors, eventType string, check func(ctx context.Context, tx uow.Tx, ev uow.Event)) *int {
	n := new(int)
	projectors.Register(eventType, func(ctx context.Context, tx uow.Tx, ev uow.Event) error {
		*n++
		check(ctx, tx, ev)
		return nil
	})
	return n
}

func wantRan(t *testing.T, what string, n *int) {
	t.Helper()
	if *n == 0 {
		t.Errorf("%s: the reads in the transaction did not run", what)
	}
}
