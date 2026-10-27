// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/uow"
)

// PaymentWorld is a PaymentProvider whose side a test drives: the fake adapter
// (internal/adapter/payment/fake) is one.
type PaymentWorld interface {
	supporters.PaymentProvider
	// Deliver returns a webhook delivery of n signed at at (PaymentDeliver).
	Deliver(n supporters.Notice, live bool, at time.Time, keys ...string) (http.Header, []byte)
	// PutSubscription, PutCheckout, PutMoney, and PutChange change the provider's side.
	PutSubscription(s supporters.SubscriptionSnapshot)
	PutCheckout(c supporters.CheckoutResult)
	PutMoney(m supporters.MoneyMovement)
	PutChange(n supporters.Notice)
	// Fail makes every request fail with err; nil ends it.
	Fail(err error)
	// Requests returns how many requests the port's methods sent.
	Requests() int
}

// PaymentWorldOptions configure a PaymentWorld.
type PaymentWorldOptions struct {
	Name     string
	Mode     supporters.Mode
	Now      func() time.Time
	PageSize int
}

// PaymentHarness is what PaymentRepositoryContract needs from a database adapter: its unit of
// work, the payment repository, and a PaymentWorld for the provider's side.
type PaymentHarness struct {
	UnitOfWork uow.UnitOfWork
	Payments   supporters.PaymentRepository
	NewWorld   func(o PaymentWorldOptions) PaymentWorld
}

// PaymentFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type PaymentFactory func(t *testing.T, opts uow.Options) PaymentHarness

// paymentProvider is the provider of the contract: the one name the schema's CHECK takes.
const paymentProvider = "stripe"

// PaymentRepositoryContract verifies the payment records of a database adapter through the
// use cases that carry the provider's facts to the Syncer (ADR-0074 §3, §4): a webhook delivery
// is recorded once with its job however often it comes, deliveries in any order end in the
// provider's current state, deliveries with a wrong signature, time, or mode, of another
// provider, of an ignored type, or beyond the rate limit record nothing, money comes with the
// payments it refers to, the receipts are purged after their retention, and the
// reconciliation catches up on what the webhooks missed, stays within its requests, resumes
// from its cursors in continuation jobs until its sweep ends, records failures without losing
// its cursors, and skips changes older than the provider keeps. One post of a checkout at a time
// holds its claim across the processes on the database, until it releases the claim or the claim
// expires.
func PaymentRepositoryContract(t *testing.T, newHarness PaymentFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, newHarness PaymentFactory)
	}{
		{"RecordsWebhooksOnce", paymentDuplicateWebhooks},
		{"EndsInTheProvidersState", paymentOutOfOrderWebhooks},
		{"RejectsBadWebhooks", paymentBadWebhooks},
		{"RecordsMoneyWithItsPayment", paymentEventMoney},
		{"PurgesOldReceipts", paymentPurge},
		{"RecordsReconcileRuns", paymentRuns},
		{"ReconcilesWhatWebhooksMissed", paymentReconcileCatchesUp},
		{"ReconcilesWithinTheRequestCap", paymentReconcileCap},
		{"RecordsFailedReconciliations", paymentReconcileFailure},
		{"SkipsChangesOutsideTheWindow", paymentReconcileWindow},
		{"ServesThePagesOfAPageServer", paymentPageServer},
		{"ClaimsACheckoutAcrossProcesses", paymentCheckoutClaim},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newHarness) })
	}
}

// paymentEnvOptions are what a case changes of its environment.
type paymentEnvOptions struct {
	pageSize    int
	maxRequests int
	limiter     bool
	limit       kernel.Limit
	// pages, when not nil, makes the provider a supporters.PageServer of it, under pageLimit.
	pages     http.Handler
	pageLimit kernel.Limit
}

// pageWorld is a PaymentWorld that serves pages (supporters.PageServer).
type pageWorld struct {
	PaymentWorld
	pages http.Handler
}

func (w pageWorld) Pages(supporters.PageEnv) http.Handler { return w.pages }

type paymentEnv struct {
	h        PaymentHarness
	clock    *Clock
	world    PaymentWorld
	syncer   *recordingSyncer
	jobs     *paymentJobs
	handlers *queue.JobHandlerRegistry
	svc      *supporters.Payments
}

func newPaymentEnv(t *testing.T, newHarness PaymentFactory, o paymentEnvOptions) *paymentEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := NewClock(paymentNow)
	e := &paymentEnv{
		h: h, clock: clock, syncer: newRecordingSyncer(), jobs: &paymentJobs{pending: map[string]bool{}},
		handlers: &queue.JobHandlerRegistry{},
	}
	e.world = h.NewWorld(PaymentWorldOptions{Name: paymentProvider, Mode: supporters.ModeTest, Now: clock.Now, PageSize: o.pageSize})
	d := supporters.PaymentsDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Payments, Provider: e.world, Syncer: e.syncer,
		Outbox: queue.AfterCommit(projectors, e.jobs, nil), Clock: clock, IDs: NewIDGenerator(clock),
		ReconcileMaxRequests: o.maxRequests, WebhookLimit: o.limit, PageLimit: o.pageLimit, Logger: discardLogger(),
	}
	if o.pages != nil {
		d.Provider = pageWorld{PaymentWorld: e.world, pages: o.pages}
	}
	if o.limiter {
		d.RateLimiter = kernel.NewRateLimiter(NewKeyValueStore(clock), clock)
	}
	e.svc = supporters.NewPayments(d)
	e.svc.RegisterJobs(e.handlers, nil)
	return e
}

// deliver sends a webhook delivery of n, signed now, and returns the outcome.
func (e *paymentEnv) deliver(t *testing.T, n supporters.Notice) supporters.WebhookOutcome {
	t.Helper()
	h, body := e.world.Deliver(n, false, e.clock.Now())
	res, err := e.svc.ReceiveWebhook(t.Context(), supporters.WebhookRequest{Provider: paymentProvider, Header: h, Body: body, ClientIP: "192.0.2.1"})
	if err != nil {
		t.Fatalf("ReceiveWebhook(%s) = %v", n.EventID, err)
	}
	return res.Outcome
}

// run runs the queued jobs, the ones they queue included, in the order they were queued, and
// returns how many ran. A job that does not end done fails the test.
func (e *paymentEnv) run(t *testing.T) int {
	t.Helper()
	ran := 0
	for job, ok := e.jobs.next(); ok; job, ok = e.jobs.next() {
		h, _, found := e.handlers.Lookup(job.Type)
		if !found {
			t.Fatalf("no handler of %s", job.Type)
		}
		r := h(t.Context(), queue.Job{
			ID: fmt.Sprintf("job-%d", ran), Type: job.Type, Payload: job.Payload, IdempotencyKey: job.IdempotencyKey,
			OrderingKey: job.OrderingKey, Attempt: 1, RunAt: job.RunAt, CreatedAt: e.clock.Now(),
		})
		if !r.IsDone() {
			t.Fatalf("job %s (%s) did not end done: %v", job.Type, job.IdempotencyKey, r.Err())
		}
		e.jobs.done(job)
		ran++
	}
	return ran
}

// paymentJobs is a queue.JobQueue that keeps the jobs for paymentEnv.run, skipping a job whose
// idempotency key is pending, as the queues do.
type paymentJobs struct {
	mu      sync.Mutex
	queued  []queue.NewJob
	pending map[string]bool
	all     []queue.NewJob
}

func (q *paymentJobs) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range jobs {
		if j.IdempotencyKey != "" && q.pending[j.IdempotencyKey] {
			continue
		}
		q.pending[j.IdempotencyKey] = true
		q.queued = append(q.queued, j)
		q.all = append(q.all, j)
	}
	return nil
}

func (q *paymentJobs) next() (queue.NewJob, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queued) == 0 {
		return queue.NewJob{}, false
	}
	j := q.queued[0]
	q.queued = q.queued[1:]
	return j, true
}

func (q *paymentJobs) done(j queue.NewJob) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.pending, j.IdempotencyKey)
}

// queuedOf returns the jobs of type typ queued so far.
func (q *paymentJobs) queuedOf(typ string) []queue.NewJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []queue.NewJob
	for _, j := range q.all {
		if j.Type == typ {
			out = append(out, j)
		}
	}
	return out
}

// recordingSyncer is a supporters.Syncer that keeps the latest snapshot of each of our
// subscriptions and the money it was given, and checks that a movement's parent comes first.
type recordingSyncer struct {
	mu        sync.Mutex
	subs      map[string]supporters.SubscriptionSnapshot
	syncs     map[string]int
	checkouts map[supporters.CheckoutRef]supporters.CheckoutResult
	open      []supporters.CheckoutRef
	money     map[string]supporters.MoneyMovement
	moneyLog  []string
}

func newRecordingSyncer() *recordingSyncer {
	return &recordingSyncer{
		subs: map[string]supporters.SubscriptionSnapshot{}, syncs: map[string]int{},
		checkouts: map[supporters.CheckoutRef]supporters.CheckoutResult{}, money: map[string]supporters.MoneyMovement{},
	}
}

func (s *recordingSyncer) SyncSubscription(_ context.Context, provider string, snap supporters.SubscriptionSnapshot) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if provider != paymentProvider || snap.LocalID == "" {
		return false, nil
	}
	s.syncs[snap.LocalID]++
	prev, ok := s.subs[snap.LocalID]
	s.subs[snap.LocalID] = snap
	return !ok || !sameSnapshot(prev, snap), nil
}

func (s *recordingSyncer) SyncCheckout(_ context.Context, _ string, res supporters.CheckoutResult) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.checkouts[res.Ref]
	s.checkouts[res.Ref] = res
	if res.Status != supporters.CheckoutOpen {
		s.open = slices.DeleteFunc(s.open, func(r supporters.CheckoutRef) bool { return r == res.Ref })
	}
	return !ok || prev != res, nil
}

func (s *recordingSyncer) RecordMoney(_ context.Context, _ string, ms []supporters.MoneyMovement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range ms {
		if m.ParentID != "" {
			if _, ok := s.money[m.ParentID]; !ok {
				return fmt.Errorf("money %s before its payment %s", m.ID, m.ParentID)
			}
		}
		if _, ok := s.money[m.ID]; ok {
			continue // recorded once by its ID, as finance.RecordIncome does
		}
		s.money[m.ID] = m
		s.moneyLog = append(s.moneyLog, m.ID)
	}
	return nil
}

func (s *recordingSyncer) KnownMoney(_ context.Context, _ string, ids []string) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		_, out[id] = s.money[id]
	}
	return out, nil
}

func (s *recordingSyncer) OpenCheckouts(context.Context, string, time.Time) ([]supporters.CheckoutRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.open), nil
}

func (s *recordingSyncer) snapshot(local string) (supporters.SubscriptionSnapshot, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subs[local], s.syncs[local]
}

func (s *recordingSyncer) recordedMoney() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.moneyLog)
}

func subNotice(event, sub string, at time.Time) supporters.Notice {
	return supporters.Notice{EventID: event, Kind: supporters.NoticeSubscription, ObjectID: sub, OccurredAt: at}
}

func paymentDuplicateWebhooks(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	e.world.PutSubscription(paymentSub("sub_1", NamedID("local-1"), supporters.StatusActive))
	n := subNotice("evt_1", "sub_1", e.clock.Now())
	want := []supporters.WebhookOutcome{supporters.WebhookRecorded, supporters.WebhookDuplicate, supporters.WebhookDuplicate}
	for i, w := range want {
		if got := e.deliver(t, n); got != w {
			t.Fatalf("delivery %d = %s, want %s", i+1, got, w)
		}
	}
	jobs := e.jobs.queuedOf(supporters.JobPaymentEvent)
	if len(jobs) != 1 || jobs[0].IdempotencyKey != supporters.EventJobKey(paymentProvider, "evt_1") ||
		jobs[0].OrderingKey == "" || !strings.Contains(jobs[0].OrderingKey, "sub_1") {
		t.Fatalf("queued %+v, want one job of evt_1 ordered by sub_1", jobs)
	}
	if ran := e.run(t); ran != 1 {
		t.Errorf("ran %d jobs, want 1", ran)
	}
	// A delivery after its job ran is still a duplicate, and queues nothing.
	if got := e.deliver(t, n); got != supporters.WebhookDuplicate {
		t.Errorf("delivery after the job = %s, want duplicate", got)
	}
	if ran := e.run(t); ran != 0 {
		t.Errorf("ran %d jobs after a duplicate, want 0", ran)
	}
	snap, syncs := e.syncer.snapshot(NamedID("local-1"))
	if syncs != 1 || snap.Status != supporters.StatusActive {
		t.Errorf("synced %d times to %+v, want once to active", syncs, snap)
	}
	got, err := e.h.Payments.LastWebhookReceipt(t.Context(), paymentProvider)
	if err != nil || got.EventID != "evt_1" || got.ObjectID != "sub_1" || got.EventType == "" || !got.ReceivedAt.Equal(paymentNow) {
		t.Errorf("LastWebhookReceipt = %+v, %v, want evt_1 received now", got, err)
	}
	// The job is idempotent too: running it again leaves the same state.
	if r := e.svc.HandleNotice(t.Context(), n); !r.IsDone() {
		t.Fatalf("HandleNotice again = %v", r.Err())
	}
	if again, _ := e.syncer.snapshot(NamedID("local-1")); !sameSnapshot(again, snap) {
		t.Errorf("state after the job ran again = %+v, want %+v", again, snap)
	}
}

func paymentOutOfOrderWebhooks(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	ctx := t.Context()
	sub := paymentSub("sub_1", NamedID("local-1"), supporters.StatusIncomplete)
	e.world.PutSubscription(sub)
	created := subNotice("evt_created", "sub_1", e.clock.Now())
	if got := e.deliver(t, created); got != supporters.WebhookRecorded {
		t.Fatalf("created = %s", got)
	}
	// The provider moves on before the first job runs: paid, then failed, then paid again.
	e.clock.Advance(time.Minute)
	sub.Status = supporters.StatusActive
	e.world.PutSubscription(sub)
	paid := subNotice("evt_paid", "sub_1", e.clock.Now())
	e.clock.Advance(time.Minute)
	sub.Status, sub.LatestPaymentFailed = supporters.StatusPastDue, e.clock.Now()
	e.world.PutSubscription(sub)
	failed := subNotice("evt_failed", "sub_1", e.clock.Now())
	e.clock.Advance(time.Minute)
	sub.Status, sub.LatestPaymentFailed, sub.PeriodEnd = supporters.StatusActive, time.Time{}, sub.PeriodEnd.Add(30*24*time.Hour)
	e.world.PutSubscription(sub)
	repaid := subNotice("evt_repaid", "sub_1", e.clock.Now())
	// The deliveries come newest first.
	for _, n := range []supporters.Notice{repaid, failed, paid} {
		if got := e.deliver(t, n); got != supporters.WebhookRecorded {
			t.Fatalf("%s = %s", n.EventID, got)
		}
	}
	if ran := e.run(t); ran != 4 {
		t.Fatalf("ran %d jobs, want 4", ran)
	}
	got, _ := e.syncer.snapshot(NamedID("local-1"))
	if !sameSnapshot(got, sub) {
		t.Errorf("state after the deliveries = %+v, want the provider's %+v", got, sub)
	}
	// A late delivery of an old event re-reads the current state too.
	if got := e.deliver(t, subNotice("evt_old", "sub_1", paymentNow)); got != supporters.WebhookRecorded {
		t.Fatalf("old event = %s", got)
	}
	e.run(t)
	if got, _ := e.syncer.snapshot(NamedID("local-1")); !sameSnapshot(got, sub) {
		t.Errorf("state after an old event = %+v, want %+v", got, sub)
	}
	// A completed checkout links the subscription it created.
	e.world.PutCheckout(supporters.CheckoutResult{Ref: "cs_1", Status: supporters.CheckoutCompleted, LocalID: NamedID("local-2"), Customer: "cus_2", Subscription: "sub_2"})
	e.world.PutSubscription(paymentSub("sub_2", NamedID("local-2"), supporters.StatusActive))
	if got := e.deliver(t, supporters.Notice{EventID: "evt_cs", Kind: supporters.NoticeCheckout, ObjectID: "cs_1"}); got != supporters.WebhookRecorded {
		t.Fatalf("checkout = %s", got)
	}
	e.run(t)
	if snap, n := e.syncer.snapshot(NamedID("local-2")); n != 1 || snap.Ref != "sub_2" {
		t.Errorf("after the checkout: %+v (%d syncs), want sub_2", snap, n)
	}
	if _, err := e.h.Payments.LastWebhookReceipt(ctx, paymentProvider); err != nil {
		t.Errorf("LastWebhookReceipt = %v", err)
	}
}

func paymentBadWebhooks(t *testing.T, newHarness PaymentFactory) {
	// The limit lets the 7 deliveries of the client IP below through: the 5 rejected, the ignored
	// one, and the valid one. A delivery to another provider's path is refused before it.
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{limiter: true, limit: kernel.Limit{Name: "test.webhook", Max: 7, Window: time.Hour}})
	ctx := t.Context()
	now := e.clock.Now()
	n := subNotice("evt_1", "sub_1", now)
	receive := func(name, provider string, h http.Header, body []byte) supporters.WebhookResult {
		t.Helper()
		res, err := e.svc.ReceiveWebhook(ctx, supporters.WebhookRequest{Provider: provider, Header: h, Body: body, ClientIP: "198.51.100.7"})
		if err != nil {
			t.Fatalf("%s: ReceiveWebhook = %v", name, err)
		}
		return res
	}
	h, body := e.world.Deliver(n, false, now)
	tampered := []byte(strings.Replace(string(body), "sub_1", "sub_9", 1))
	staleH, staleBody := e.world.Deliver(n, false, now.Add(-time.Hour))
	otherH, otherBody := e.world.Deliver(n, false, now, "whsec_other")
	liveH, liveBody := e.world.Deliver(n, true, now)
	cases := []struct {
		name      string
		provider  string
		h         http.Header
		body      []byte
		outcome   supporters.WebhookOutcome
		errorKind supporters.ErrorKind
	}{
		{"tampered body", paymentProvider, h, tampered, supporters.WebhookRejected, supporters.ErrorInvalidSignature},
		{"stale signature", paymentProvider, staleH, staleBody, supporters.WebhookRejected, supporters.ErrorStaleSignature},
		{"another secret", paymentProvider, otherH, otherBody, supporters.WebhookRejected, supporters.ErrorInvalidSignature},
		{"live event", paymentProvider, liveH, liveBody, supporters.WebhookRejected, supporters.ErrorModeMismatch},
		{"no signature", paymentProvider, http.Header{}, body, supporters.WebhookRejected, supporters.ErrorInvalidSignature},
		{"another provider", "other", h, body, supporters.WebhookUnknownProvider, ""},
	}
	for _, c := range cases {
		res := receive(c.name, c.provider, c.h, c.body)
		if res.Outcome != c.outcome || (c.errorKind != "" && res.ErrorKind != c.errorKind) {
			t.Errorf("%s: %s (%s), want %s (%s)", c.name, res.Outcome, res.ErrorKind, c.outcome, c.errorKind)
		}
	}
	ignoredH, ignoredBody := e.world.Deliver(supporters.Notice{EventID: "evt_other", Kind: supporters.NoticeIgnored, ObjectID: "cus_1"}, false, now)
	if res := receive("ignored", paymentProvider, ignoredH, ignoredBody); res.Outcome != supporters.WebhookIgnored {
		t.Errorf("ignored type: %s, want ignored", res.Outcome)
	}
	if _, err := e.h.Payments.LastWebhookReceipt(ctx, paymentProvider); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("LastWebhookReceipt after the refused deliveries = %v, want ErrNotFound", err)
	}
	if jobs := e.jobs.queuedOf(supporters.JobPaymentEvent); len(jobs) != 0 {
		t.Errorf("refused deliveries queued %+v", jobs)
	}
	// The rate limit counts every delivery of the client IP, refused or not.
	if res := receive("valid", paymentProvider, h, body); res.Outcome != supporters.WebhookRecorded {
		t.Errorf("valid delivery: %s, want recorded", res.Outcome)
	}
	if res := receive("beyond the limit", paymentProvider, h, body); res.Outcome != supporters.WebhookRateLimited || res.RetryAfter <= 0 {
		t.Errorf("delivery beyond the limit: %+v, want rate_limited with a Retry-After", res)
	}
}

func paymentEventMoney(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	s := paymentMoneyScenario(t)
	movements := PaymentMovements(s)
	byID := map[string]supporters.MoneyMovement{}
	for _, m := range movements {
		byID[m.ID] = m
	}
	for _, id := range []string{"txn_p1", "txn_r1", "txn_pending"} {
		e.world.PutMoney(byID[id])
	}
	e.world.PutSubscription(paymentSub("sub_1", NamedID("local-1"), supporters.StatusActive))
	// The refund comes first: its job reads the payment it refunds before recording it.
	for _, id := range []string{"txn_r1", "txn_r1", "txn_pending"} {
		e.deliver(t, supporters.Notice{EventID: "evt_" + id, Kind: supporters.NoticeMoney, ObjectID: id})
	}
	e.run(t)
	if got := e.syncer.recordedMoney(); !slices.Equal(got, []string{"txn_p1", "txn_r1"}) {
		t.Errorf("recorded money %v, want the payment and then its refund", got)
	}
	if _, n := e.syncer.snapshot(NamedID("local-1")); n != 1 {
		t.Errorf("the refund synced its subscription %d times, want 1", n)
	}
	// The payment's own delivery records nothing new.
	e.deliver(t, supporters.Notice{EventID: "evt_txn_p1", Kind: supporters.NoticeMoney, ObjectID: "txn_p1"})
	e.run(t)
	if got := e.syncer.recordedMoney(); len(got) != 2 {
		t.Errorf("recorded money %v after the payment's delivery, want 2", got)
	}
}

func paymentPurge(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	e.deliver(t, subNotice("evt_old", "sub_1", e.clock.Now()))
	e.clock.Advance(supporters.ReceiptRetention - time.Hour)
	e.deliver(t, subNotice("evt_new", "sub_1", e.clock.Now()))
	e.clock.Advance(2 * time.Hour)
	n, err := e.svc.PurgeReceipts(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("PurgeReceipts = %d, %v, want 1", n, err)
	}
	if got, err := e.h.Payments.LastWebhookReceipt(t.Context(), paymentProvider); err != nil || got.EventID != "evt_new" {
		t.Errorf("LastWebhookReceipt after the purge = %+v, %v, want evt_new", got, err)
	}
	// A purged event that comes again is recorded again.
	if got := e.deliver(t, subNotice("evt_old", "sub_1", e.clock.Now())); got != supporters.WebhookRecorded {
		t.Errorf("purged event again = %s, want recorded", got)
	}
}

func paymentRuns(t *testing.T, newHarness PaymentFactory) {
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: &uow.Projectors{}})
	ctx := t.Context()
	ids := NewIDGenerator(NewClock(paymentNow))
	insert := func(r supporters.ReconcileRun) bool {
		t.Helper()
		var ok bool
		if err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			ok, err = h.Payments.InsertReconcileRun(ctx, tx, r)
			return err
		}); err != nil {
			t.Fatalf("InsertReconcileRun(%s) = %v", r.RunKey, err)
		}
		return ok
	}
	if _, err := h.Payments.LastReconcileRun(ctx, paymentProvider, false); !errors.Is(err, supporters.ErrNotFound) {
		t.Errorf("LastReconcileRun of none = %v, want ErrNotFound", err)
	}
	partial := supporters.ReconcileRun{
		ID: ids.NewID(), Provider: paymentProvider, RunKey: "schedule:supporters.payment_reconcile:1", Outcome: supporters.ReconcilePartial,
		ErrorKind: supporters.ErrorRequestCap, ChangesUntil: paymentNow.Add(-time.Hour), ChangesCursor: "chg:100", MoneyCursor: "txn:5",
		SweepCursor: "sub:200", Requests: 1000, CheckedSubscriptions: 950, CorrectedSubscriptions: 3, OccurredAt: paymentNow,
	}
	if !insert(partial) {
		t.Fatal("InsertReconcileRun did not record the first attempt")
	}
	again := partial
	again.ID = ids.NewID()
	if insert(again) {
		t.Error("InsertReconcileRun recorded a second attempt of the same job")
	}
	failed := supporters.ReconcileRun{
		ID: ids.NewID(), Provider: paymentProvider, RunKey: "continue:" + partial.ID, Outcome: supporters.ReconcileFailed,
		ErrorKind: supporters.ErrorThrottled, ChangesUntil: paymentNow, Requests: 2, OccurredAt: paymentNow.Add(time.Minute),
	}
	insert(failed)
	if used, err := h.Payments.ReconcileRunKeyUsed(ctx, paymentProvider, partial.RunKey); err != nil || !used {
		t.Errorf("ReconcileRunKeyUsed(recorded) = %v, %v", used, err)
	}
	if used, err := h.Payments.ReconcileRunKeyUsed(ctx, paymentProvider, "manual:x"); err != nil || used {
		t.Errorf("ReconcileRunKeyUsed(unknown) = %v, %v", used, err)
	}
	if got, err := h.Payments.LastReconcileRun(ctx, paymentProvider, true); err != nil || !reflect.DeepEqual(got, partial) {
		t.Errorf("LastReconcileRun(resumable) = %+v, %v, want %+v", got, err, partial)
	}
	if got, err := h.Payments.LastReconcileRun(ctx, paymentProvider, false); err != nil || got.ID != failed.ID {
		t.Errorf("LastReconcileRun = %+v, %v, want the failed attempt", got, err)
	}
	// The schema keeps the outcome and the kind of failure together.
	bad := failed
	bad.ID, bad.RunKey, bad.ErrorKind = ids.NewID(), "bad", supporters.ErrorNone
	if err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := h.Payments.InsertReconcileRun(ctx, tx, bad)
		return err
	}); err == nil {
		t.Error("InsertReconcileRun recorded a failure of kind none")
	}
}

// reconcile runs the reconciliation job of key and the continuations it queues, and returns
// the newest attempt.
func (e *paymentEnv) reconcile(t *testing.T, key string) supporters.ReconcileRun {
	t.Helper()
	if r := e.svc.Reconcile(t.Context(), key); !r.IsDone() {
		t.Fatalf("Reconcile(%s) = %v", key, r.Err())
	}
	run, err := e.h.Payments.LastReconcileRun(t.Context(), paymentProvider, false)
	if err != nil {
		t.Fatalf("LastReconcileRun = %v", err)
	}
	return run
}

func paymentReconcileCatchesUp(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	ctx := t.Context()
	sub := paymentSub("sub_1", NamedID("local-1"), supporters.StatusActive)
	e.world.PutSubscription(sub)
	e.world.PutSubscription(paymentSub("sub_not_ours", "", supporters.StatusActive))
	first := e.reconcile(t, "schedule:1")
	if first.Outcome != supporters.ReconcileSucceeded || first.CheckedSubscriptions != 1 || first.CorrectedSubscriptions != 1 || first.SweepCursor != "" {
		t.Errorf("first reconciliation = %+v, want a sweep that found local-1", first)
	}
	// The provider cancels the subscription and the webhook never arrives.
	e.clock.Advance(time.Hour)
	sub.Status = supporters.StatusCanceled
	e.world.PutSubscription(sub)
	e.world.PutChange(subNotice("evt_deleted", "sub_1", e.clock.Now()))
	e.world.PutCheckout(supporters.CheckoutResult{Ref: "cs_open", Status: supporters.CheckoutCompleted, LocalID: NamedID("local-3"), Customer: "cus_3", Subscription: "sub_3"})
	e.world.PutSubscription(paymentSub("sub_3", NamedID("local-3"), supporters.StatusActive))
	e.syncer.open = []supporters.CheckoutRef{"cs_open"}
	movements := PaymentMovements(paymentMoneyScenario(t))
	for _, m := range movements {
		e.world.PutMoney(m)
	}
	e.clock.Advance(time.Hour)
	before := e.world.Requests()
	run := e.reconcile(t, "schedule:2")
	if run.Outcome != supporters.ReconcileSucceeded || run.ErrorKind != supporters.ErrorNone {
		t.Fatalf("reconciliation = %+v, want succeeded", run)
	}
	if got, _ := e.syncer.snapshot(NamedID("local-1")); got.Status != supporters.StatusCanceled {
		t.Errorf("local-1 after the reconciliation = %s, want canceled", got.Status)
	}
	if _, n := e.syncer.snapshot(NamedID("local-3")); n != 1 {
		t.Errorf("local-3 synced %d times, want once from its checkout", n)
	}
	if got := e.syncer.recordedMoney(); len(got) != 9 {
		t.Errorf("recorded %d movements, want the 9 final ones", len(got))
	}
	if sent := e.world.Requests() - before; sent != run.Requests {
		t.Errorf("the reconciliation counted %d requests and sent %d", run.Requests, sent)
	}
	if !run.ChangesUntil.Equal(e.clock.Now()) || run.ChangesCursor != "" || run.MoneyCursor == "" {
		t.Errorf("cursors = %+v, want the changes read through now and a money cursor", run)
	}
	// The next run reads the money after its cursor only.
	e.clock.Advance(time.Hour)
	third := e.reconcile(t, "schedule:3")
	if got := e.syncer.recordedMoney(); third.Outcome != supporters.ReconcileSucceeded || len(got) != 9 || third.MoneyCursor != run.MoneyCursor {
		t.Errorf("third reconciliation = %+v with %d movements, want the same money cursor", third, len(got))
	}
	// A job whose attempt is recorded does nothing.
	before = e.world.Requests()
	if r := e.svc.Reconcile(ctx, "schedule:3"); !r.IsDone() || e.world.Requests() != before {
		t.Errorf("Reconcile of a recorded job sent %d requests", e.world.Requests()-before)
	}
}

func paymentReconcileCap(t *testing.T, newHarness PaymentFactory) {
	const perRun = 3
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{pageSize: 10, maxRequests: perRun})
	for i := range 35 {
		e.world.PutSubscription(paymentSub(fmt.Sprintf("sub_%02d", i), NamedID(fmt.Sprintf("local-%02d", i)), supporters.StatusActive))
	}
	before := e.world.Requests()
	run := e.reconcile(t, "schedule:1")
	if run.Outcome != supporters.ReconcilePartial || run.ErrorKind != supporters.ErrorRequestCap || run.Requests != perRun || run.SweepCursor == "" {
		t.Fatalf("first attempt = %+v, want partial after %d requests with a sweep cursor", run, perRun)
	}
	if sent := e.world.Requests() - before; sent != perRun {
		t.Errorf("the first attempt sent %d requests, want %d", sent, perRun)
	}
	conts := e.jobs.queuedOf(supporters.JobPaymentReconcile)
	if len(conts) != 1 || conts[0].IdempotencyKey != supporters.ContinuationKey(run.ID) || !conts[0].RunAt.After(e.clock.Now()) {
		t.Fatalf("continuations = %+v, want one of %s later", conts, run.ID)
	}
	// The continuations take over from the cursors until the sweep ends.
	e.clock.Advance(supporters.ContinuationDelay)
	before = e.world.Requests()
	if ran := e.run(t); ran < 2 || ran > 5 {
		t.Errorf("ran %d continuations, want the 3 that finish the sweep", ran)
	}
	last, err := e.h.Payments.LastReconcileRun(t.Context(), paymentProvider, false)
	if err != nil || last.Outcome != supporters.ReconcileSucceeded || last.SweepCursor != "" {
		t.Fatalf("last attempt = %+v, %v, want a finished sweep", last, err)
	}
	conts = e.jobs.queuedOf(supporters.JobPaymentReconcile)
	if sent := e.world.Requests() - before; sent > perRun*len(conts) {
		t.Errorf("the continuations sent %d requests, more than %d each", sent, perRun)
	}
	for i := range 35 {
		if _, n := e.syncer.snapshot(NamedID(fmt.Sprintf("local-%02d", i))); n != 1 {
			t.Errorf("local-%02d synced %d times, want once", i, n)
		}
	}
}

func paymentReconcileFailure(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{pageSize: 10, maxRequests: 3})
	for i := range 25 {
		e.world.PutSubscription(paymentSub(fmt.Sprintf("sub_%02d", i), NamedID(fmt.Sprintf("local-%02d", i)), supporters.StatusActive))
	}
	partial := e.reconcile(t, "schedule:1")
	if partial.Outcome != supporters.ReconcilePartial {
		t.Fatalf("first attempt = %+v, want partial", partial)
	}
	e.world.Fail(fmt.Errorf("%w: 429", supporters.ErrPaymentThrottled))
	failed := e.reconcile(t, "schedule:2")
	if failed.Outcome != supporters.ReconcileFailed || failed.ErrorKind != supporters.ErrorThrottled {
		t.Fatalf("attempt on a throttling provider = %+v, want failed as throttled", failed)
	}
	st, err := e.svc.Status(t.Context())
	if err != nil || st.LastRun == nil || st.LastRun.ID != failed.ID || st.Provider != paymentProvider || st.Mode != supporters.ModeTest {
		t.Errorf("Status = %+v, %v, want the failed attempt", st, err)
	}
	// The next attempt starts from the cursors of the partial one.
	e.world.Fail(nil)
	next := e.reconcile(t, "schedule:3")
	if next.Outcome == supporters.ReconcileFailed || next.SweepCursor == partial.SweepCursor {
		t.Errorf("attempt after the failure = %+v, want it past the sweep cursor %q", next, partial.SweepCursor)
	}
	if _, n := e.syncer.snapshot(NamedID("local-10")); n != 1 {
		t.Errorf("local-10 synced %d times, want once", n)
	}
}

func paymentReconcileWindow(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	e.world.PutSubscription(paymentSub("sub_1", NamedID("local-1"), supporters.StatusActive))
	first := e.reconcile(t, "schedule:1")
	// The reconciliations stop for 40 days; the provider no longer keeps the changes since.
	e.clock.Advance(40 * 24 * time.Hour)
	run := e.reconcile(t, "schedule:2")
	if run.Outcome != supporters.ReconcileSucceeded || !run.ChangesUntil.After(first.ChangesUntil) {
		t.Errorf("reconciliation after 40 days = %+v, want succeeded with the changes skipped", run)
	}
	if _, n := e.syncer.snapshot(NamedID("local-1")); n != 2 {
		t.Errorf("local-1 synced %d times, want once per sweep", n)
	}
}

func paymentPageServer(t *testing.T, newHarness PaymentFactory) {
	ctx := t.Context()
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	if res, err := e.svc.Pages(ctx, paymentProvider, "198.51.100.7"); err != nil || res.Handler != nil {
		t.Errorf("Pages of a provider without pages = %+v, %v, want no handler", res, err)
	}
	page := http.NotFoundHandler()
	e = newPaymentEnv(t, newHarness, paymentEnvOptions{
		pages: page, limiter: true, pageLimit: kernel.Limit{Name: "test.pages", Max: 2, Window: time.Hour},
	})
	if res, err := e.svc.Pages(ctx, "other", "198.51.100.7"); err != nil || res.Handler != nil || res.RateLimited {
		t.Errorf("Pages of another provider = %+v, %v, want no handler", res, err)
	}
	for i := range 2 {
		if res, err := e.svc.Pages(ctx, paymentProvider, "198.51.100.7"); err != nil || res.Handler == nil {
			t.Errorf("Pages #%d = %+v, %v, want the provider's handler", i+1, res, err)
		}
	}
	if res, err := e.svc.Pages(ctx, paymentProvider, "198.51.100.7"); err != nil || res.Handler != nil || !res.RateLimited || res.RetryAfter <= 0 {
		t.Errorf("Pages beyond the limit = %+v, %v, want rate limited", res, err)
	}
	if res, err := e.svc.Pages(ctx, paymentProvider, "198.51.100.8"); err != nil || res.Handler == nil {
		t.Errorf("Pages of another client IP = %+v, %v, want the provider's handler", res, err)
	}
}

func paymentCheckoutClaim(t *testing.T, newHarness PaymentFactory) {
	e := newPaymentEnv(t, newHarness, paymentEnvOptions{})
	ctx := t.Context()
	// Another process: its own Payments on the same database.
	other := supporters.NewPayments(supporters.PaymentsDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Payments, Provider: e.world, Syncer: e.syncer,
		Outbox: queue.AfterCommit(&uow.Projectors{}, e.jobs, nil), Clock: e.clock, IDs: NewIDGenerator(e.clock), Logger: discardLogger(),
	})
	claim := func(p *supporters.Payments, id string) (func(), bool) {
		t.Helper()
		release, ok, err := p.ClaimCheckout(ctx, id)
		if err != nil {
			t.Fatalf("ClaimCheckout(%s) = %v", id, err)
		}
		if ok != (release != nil) {
			t.Fatalf("ClaimCheckout(%s) = %v with release %v", id, ok, release != nil)
		}
		return release, ok
	}
	release, ok := claim(e.svc, "local-1")
	if !ok {
		t.Fatal("first claim refused")
	}
	if _, ok := claim(other, "local-1"); ok {
		t.Error("claim of another process while the first holds it was taken")
	}
	if _, ok := claim(e.svc, "local-1"); ok {
		t.Error("second claim in the same process was taken")
	}
	if r, ok := claim(other, "local-2"); !ok {
		t.Error("claim of another checkout refused")
	} else {
		r()
	}
	release()
	release2, ok := claim(other, "local-1")
	if !ok {
		t.Fatal("claim after the release refused")
	}
	// A release by a holder whose claim was taken over deletes nothing of the new holder.
	release()
	if _, ok := claim(e.svc, "local-1"); ok {
		t.Error("claim taken after a stale release")
	}
	// A claim that its process never releases expires.
	e.clock.Advance(supporters.CheckoutClaimHold - time.Second)
	if _, ok := claim(e.svc, "local-1"); ok {
		t.Error("claim taken before the other expired")
	}
	e.clock.Advance(time.Second)
	release3, ok := claim(e.svc, "local-1")
	if !ok {
		t.Fatal("claim after the other expired refused")
	}
	release2()
	if _, ok := claim(other, "local-1"); ok {
		t.Error("claim taken after the expired holder released")
	}
	release3()
	// The purge deletes the expired claims only.
	if _, ok := claim(e.svc, "local-3"); !ok {
		t.Fatal("claim of local-3 refused")
	}
	e.clock.Advance(supporters.CheckoutClaimHold + time.Second)
	if _, ok := claim(other, "local-4"); !ok {
		t.Fatal("claim of local-4 refused")
	}
	var purged int64
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		purged, err = e.h.Payments.PurgeCheckoutClaims(ctx, tx, e.clock.Now())
		return err
	}); err != nil || purged != 1 {
		t.Errorf("PurgeCheckoutClaims = %d, %v, want 1", purged, err)
	}
	if _, ok := claim(e.svc, "local-4"); ok {
		t.Error("the purge deleted a claim that has not expired")
	}
}
