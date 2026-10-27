// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// ContinuationKey returns the idempotency key of the job that continues the reconciliation
// attempt runID.
func ContinuationKey(runID string) string { return "continue:" + runID }

// Reconcile runs one attempt of the reconciliation for the job runKey (ADR-0074 §4, C1). It
// sends the provider at most ReconcileMaxRequests requests, outside any transaction, in four
// stages, each from the cursors of the latest succeeded or partial attempt:
//
//  1. the changes since ChangesOverlap before the end of the previous listing, whose
//     subscriptions and checkouts it re-reads (skipped on ErrChangesWindow, which the sweep
//     makes up for);
//  2. the checkouts that our subscriptions wait for;
//  3. the money movements after the money cursor;
//  4. a sweep of every subscription of the account from the sweep cursor, which starts over
//     after the last page.
//
// Everything it re-reads goes to the Syncer. It records the attempt in one transaction with,
// when it used its requests, the continuation job (ContinuationKey, ContinuationDelay later)
// that picks up from its cursors. A job whose attempt is recorded does nothing. A failure of
// the provider is recorded and waits for the next reconciliation; a failure of the Syncer or
// the database is retried.
func (p *Payments) Reconcile(ctx context.Context, runKey string) queue.Result {
	name := p.d.Provider.Name()
	used, err := p.d.Repository.ReconcileRunKeyUsed(ctx, name, runKey)
	if err != nil {
		return queue.Retry(0, err)
	}
	if used {
		return queue.Done()
	}
	now := p.now()
	run := ReconcileRun{ID: p.d.IDs.NewID(), Provider: name, RunKey: runKey, OccurredAt: now}
	prev, err := p.d.Repository.LastReconcileRun(ctx, name, true)
	switch {
	case err == nil:
		run.ChangesUntil, run.ChangesCursor = prev.ChangesUntil, prev.ChangesCursor
		run.MoneyCursor, run.SweepCursor = prev.MoneyCursor, prev.SweepCursor
	case errors.Is(err, ErrNotFound):
		// The first reconciliation reads the changes of one period; the sweep reads the rest.
		run.ChangesUntil = now.Add(-p.d.ReconcileInterval)
	default:
		return queue.Retry(0, err)
	}
	r := &reconciliation{p: p, run: &run, now: now, synced: map[SubscriptionRef]bool{}, checkouts: map[CheckoutRef]bool{}}
	stageErr := r.stages(ctx)
	switch {
	case stageErr == nil:
		run.Outcome, run.ErrorKind = ReconcileSucceeded, ErrorNone
	case errors.Is(stageErr, errRequestCap):
		run.Outcome, run.ErrorKind = ReconcilePartial, ErrorRequestCap
	case errors.Is(stageErr, errSyncer):
		return queue.Retry(0, stageErr)
	default:
		run.Outcome, run.ErrorKind = ReconcileFailed, ErrorKindOf(stageErr)
	}
	// The attempt is recorded even when ctx ended, so that its requests and cursors count.
	rec := context.WithoutCancel(ctx)
	err = p.d.UnitOfWork.Tx(rec, func(tx uow.Tx) error {
		inserted, err := p.d.Repository.InsertReconcileRun(rec, tx, run)
		if err != nil || !inserted || run.Outcome != ReconcilePartial {
			return err
		}
		return p.d.Outbox.EnqueueTx(rec, tx, p.reconcileJob(ContinuationKey(run.ID), now.Add(ContinuationDelay)))
	})
	if err != nil {
		return queue.Retry(0, fmt.Errorf("supporters: record the reconciliation of %s: %w", name, err))
	}
	attrs := []any{
		slog.String("provider", name), slog.String("outcome", string(run.Outcome)), slog.String("error_kind", string(run.ErrorKind)),
		slog.Int("requests", run.Requests), slog.Int("checked", run.CheckedSubscriptions), slog.Int("corrected", run.CorrectedSubscriptions),
	}
	if run.Outcome == ReconcileFailed {
		p.d.Logger.WarnContext(ctx, "payment reconciliation failed", append(attrs, slog.Any("error", stageErr))...)
	} else {
		p.d.Logger.InfoContext(ctx, "payment reconciliation recorded", attrs...)
	}
	return queue.Done()
}

// reconciliation is the state of one attempt.
type reconciliation struct {
	p   *Payments
	run *ReconcileRun
	now time.Time
	// synced and checkouts are what the attempt re-read already.
	synced    map[SubscriptionRef]bool
	checkouts map[CheckoutRef]bool
}

// take counts one more request, and reports false when the attempt used them all.
func (r *reconciliation) take() bool {
	if r.run.Requests >= r.p.d.ReconcileMaxRequests {
		return false
	}
	r.run.Requests++
	return true
}

func (r *reconciliation) stages(ctx context.Context) error {
	for _, stage := range []func(context.Context) error{r.changes, r.openCheckouts, r.money, r.sweep} {
		if err := stage(ctx); err != nil {
			return err
		}
	}
	return nil
}

// sync hands a snapshot to the Syncer and counts it.
func (r *reconciliation) sync(ctx context.Context, snap SubscriptionSnapshot) error {
	changed, err := r.p.d.Syncer.SyncSubscription(ctx, r.p.d.Provider.Name(), snap)
	if err != nil {
		return fmt.Errorf("%w: %w", errSyncer, err)
	}
	r.synced[snap.Ref] = true
	r.run.CheckedSubscriptions++
	if changed {
		r.run.CorrectedSubscriptions++
	}
	return nil
}

// checkout re-reads a checkout once per attempt and hands it to the Syncer; it returns the
// subscription a completed checkout created.
func (r *reconciliation) checkout(ctx context.Context, ref CheckoutRef) (SubscriptionRef, error) {
	if r.checkouts[ref] {
		return "", nil
	}
	if !r.take() {
		return "", errRequestCap
	}
	res, err := r.p.d.Provider.GetCheckout(ctx, ref)
	if err != nil {
		return "", err
	}
	if _, err := r.p.d.Syncer.SyncCheckout(ctx, r.p.d.Provider.Name(), res); err != nil {
		return "", fmt.Errorf("%w: %w", errSyncer, err)
	}
	r.checkouts[ref] = true
	return res.Subscription, nil
}

// refetch re-reads a subscription once per attempt and hands it to the Syncer.
func (r *reconciliation) refetch(ctx context.Context, ref SubscriptionRef) error {
	if ref == "" || r.synced[ref] {
		return nil
	}
	if !r.take() {
		return errRequestCap
	}
	snap, err := readSubscription(ctx, r.p.d.Provider, r.p.d.Clock, ref)
	if errors.Is(err, ErrPaymentNotFound) {
		return nil // deleted on the provider's side; the sweep no longer lists it
	}
	if err != nil {
		return err
	}
	return r.sync(ctx, snap)
}

// changes is stage 1. A page whose objects were not all re-read is read again next time.
func (r *reconciliation) changes(ctx context.Context) error {
	since := r.run.ChangesUntil.Add(-ChangesOverlap)
	for {
		if !r.take() {
			return errRequestCap
		}
		page, err := r.p.d.Provider.ListChanges(ctx, since, r.run.ChangesCursor)
		if errors.Is(err, ErrChangesWindow) {
			r.p.d.Logger.WarnContext(ctx, "payment changes older than the provider keeps; the sweep makes up for them",
				slog.String("provider", r.p.d.Provider.Name()), slog.Time("since", since))
			r.run.ChangesUntil, r.run.ChangesCursor = r.now, ""
			return nil
		}
		if err != nil {
			return err
		}
		for _, n := range page.Notices {
			if err := r.notice(ctx, n); err != nil {
				return err
			}
		}
		if page.Next == "" {
			until := page.Until
			if until.IsZero() || until.After(r.now) {
				until = r.now
			}
			r.run.ChangesUntil, r.run.ChangesCursor = until, ""
			return nil
		}
		r.run.ChangesCursor = page.Next
	}
}

// notice re-reads what a notice of stage 1 points at. Money waits for stage 3.
func (r *reconciliation) notice(ctx context.Context, n Notice) error {
	if err := r.refetch(ctx, n.Subscription); err != nil {
		return err
	}
	switch n.Kind {
	case NoticeSubscription:
		return r.refetch(ctx, SubscriptionRef(n.ObjectID))
	case NoticeCheckout:
		sub, err := r.checkout(ctx, CheckoutRef(n.ObjectID))
		if err != nil {
			return err
		}
		return r.refetch(ctx, sub)
	}
	return nil
}

// openCheckouts is stage 2.
func (r *reconciliation) openCheckouts(ctx context.Context) error {
	refs, err := r.p.d.Syncer.OpenCheckouts(ctx, r.p.d.Provider.Name(), r.now)
	if err != nil {
		return fmt.Errorf("%w: %w", errSyncer, err)
	}
	for _, ref := range refs {
		sub, err := r.checkout(ctx, ref)
		if err != nil {
			return err
		}
		if err := r.refetch(ctx, sub); err != nil {
			return err
		}
	}
	return nil
}

// money is stage 3: the movements after the money cursor, from the start of the account's
// history the first time.
func (r *reconciliation) money(ctx context.Context) error {
	for {
		if !r.take() {
			return errRequestCap
		}
		page, err := r.p.d.Provider.ListMoney(ctx, time.Time{}, r.run.MoneyCursor)
		if err != nil {
			return err
		}
		if len(page.Movements) > 0 {
			if err := r.p.recordMoney(ctx, page.Movements, r.take); err != nil {
				return err
			}
		}
		if page.Next != "" {
			r.run.MoneyCursor = page.Next
		}
		if !page.More {
			return nil
		}
	}
}

// sweep is stage 4: the pages of every subscription from the sweep cursor, with the requests
// that are left. After the last page the cursor goes back to the start.
func (r *reconciliation) sweep(ctx context.Context) error {
	for {
		if !r.take() {
			return errRequestCap
		}
		readAt := kernel.Normalize(r.p.d.Clock.Now())
		page, err := r.p.d.Provider.ListSubscriptions(ctx, r.run.SweepCursor)
		if err != nil {
			return err
		}
		for _, snap := range page.Subscriptions {
			if snap.LocalID == "" || r.synced[snap.Ref] {
				continue
			}
			snap.ReadAt = readAt
			if err := r.sync(ctx, snap); err != nil {
				return err
			}
		}
		r.run.SweepCursor = page.Next
		if page.Next == "" {
			return nil
		}
	}
}
