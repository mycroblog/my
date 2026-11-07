// SPDX-License-Identifier: AGPL-3.0-only

package testkit

// The recorders and stand-ins that the contracts and flows of several modules share.

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

// ContractHost is the host of the instance that the contracts and flows run on.
const ContractHost = "social.example"

// EventLog records the graph events that commands emit (committed or not).
type EventLog struct {
	Mu     sync.Mutex
	Events []uow.Event
}

// Record is a projector that logs e.
func (l *EventLog) Record(_ context.Context, _ uow.Tx, e uow.Event) error {
	l.Mu.Lock()
	defer l.Mu.Unlock()
	l.Events = append(l.Events, e)
	return nil
}

// Take returns the events logged since the last Take and forgets them.
func (l *EventLog) Take() []uow.Event {
	l.Mu.Lock()
	defer l.Mu.Unlock()
	out := l.Events
	l.Events = nil
	return out
}

// RecordingOutbox is a queue.Outbox that keeps the jobs it is given.
type RecordingOutbox struct {
	mu   sync.Mutex
	Jobs []queue.NewJob
}

// EnqueueTx keeps jobs.
func (o *RecordingOutbox) EnqueueTx(_ context.Context, _ uow.Tx, jobs ...queue.NewJob) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Jobs = append(o.Jobs, jobs...)
	return nil
}

// Take returns the jobs kept since the last Take and forgets them.
func (o *RecordingOutbox) Take() []queue.NewJob {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.Jobs
	o.Jobs = nil
	return out
}

// RemoteSanitizer stands in for the HTML sanitizer: it drops script elements and records
// version 7, so that the contract sees that remote HTML went through it.
type RemoteSanitizer struct{}

var (
	remoteScripts = regexp.MustCompile(`(?s)<script.*?</script>`)
	remoteTags    = regexp.MustCompile(`<[^>]*>`)
)

// SanitizeHTML drops the script elements of s.
func (RemoteSanitizer) SanitizeHTML(s string) string { return remoteScripts.ReplaceAllString(s, "") }

// PlainText drops the tags of s.
func (RemoteSanitizer) PlainText(s string) string { return remoteTags.ReplaceAllString(s, "") }

// PolicyVersion is 7.
func (RemoteSanitizer) PolicyVersion() int { return 7 }

// SortedIDs sorts ids in place and returns them.
func SortedIDs(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

// Rendezvous lets each of n transactions wait, inside the transaction, until all n reach it or
// patience runs out. A transaction that waits for a lock that another one holds never reaches
// it, so patience bounds the wait; the transactions then go on in the order the database allows.
type Rendezvous struct {
	arrived  chan struct{}
	n        int
	patience time.Duration
}

// NewRendezvous returns a rendezvous of n transactions.
func NewRendezvous(n int) *Rendezvous {
	return &Rendezvous{arrived: make(chan struct{}, n), n: n, patience: 300 * time.Millisecond}
}

// Wait blocks until all n transactions reach the rendezvous or patience runs out.
func (r *Rendezvous) Wait() {
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

// InTransactionOf registers on projectors a projector of eventType that runs check in the
// transaction of each such event, and returns how many times it ran.
func InTransactionOf(projectors *uow.Projectors, eventType string, check func(ctx context.Context, tx uow.Tx, ev uow.Event)) *int {
	n := new(int)
	projectors.Register(eventType, func(ctx context.Context, tx uow.Tx, ev uow.Event) error {
		*n++
		check(ctx, tx, ev)
		return nil
	})
	return n
}

// WantRan fails t when the reads counted by n, from InTransactionOf, never ran.
func WantRan(t *testing.T, what string, n *int) {
	t.Helper()
	if *n == 0 {
		t.Errorf("%s: the reads in the transaction did not run", what)
	}
}
