// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// The subscription core (ADR-0072 §7, T1, G1): every signal (a webhook, the return from the
// checkout, a deadline of the Scheduler, the reconciliation) comes down to re-reading the
// provider's subscription and deriving our state from the snapshot and our own facts
// (supporters.Derive). A transition is recorded only when the state or what the role depends on
// changes, in the transaction that grants, revokes, or replaces the plan's role and that removes
// the listing of a member who loses it. The commands to the provider run in jobs after the
// commit (JobCommand), and the deadlines of the Scheduler queue JobSync.

// The jobs of the subscriptions.
const (
	// JobSync re-reads one subscription from the provider and applies it; its idempotency key
	// names the subscription and what queued it.
	JobSync = "supporters.sync"
	// JobDeadlines queues JobSync for the subscriptions whose deadline passed, every
	// DeadlineInterval.
	JobDeadlines = "supporters.deadlines"
	// JobCommand sends a command to the provider for one subscription and applies the snapshot
	// that follows.
	JobCommand = "supporters.command"

	// DeadlineInterval is the period of JobDeadlines, and DeadlineBatch the deadlines one run
	// queues.
	DeadlineInterval = 5 * time.Minute
	DeadlineBatch    = 500
)

// Command is a command of JobCommand.
type Command string

// The commands.
const (
	// CommandCancelNow ends the subscription at the provider at once, without a refund.
	CommandCancelNow Command = "cancel_now"
	// CommandCancelAtPeriodEnd withdraws a scheduled change and cancels at the end of the period.
	CommandCancelAtPeriodEnd Command = "cancel_at_period_end"
)

// snapshotOf resolves the provider's prices of snap to ours: a price that is not ours is "".
func (s *Service) snapshotOf(ctx context.Context, tx uow.Tx, provider string, snap SubscriptionSnapshot) (supporters.Snapshot, error) {
	out := supporters.Snapshot{
		Status: supporters.ProviderStatus(snap.Status), PeriodEnd: kernelTime(snap.PeriodEnd), CancelAtPeriodEnd: snap.CancelAtPeriodEnd,
		PaymentFailedAt: kernelTime(snap.LatestPaymentFailed),
	}
	if snap.Price != "" {
		p, err := s.d.Repository.FindPriceByRef(ctx, tx, provider, snap.Price)
		switch {
		case err == nil:
			out.PlanID, out.PriceID = p.PlanID, p.ID
		case !errors.Is(err, ErrNotFound):
			return supporters.Snapshot{}, err
		}
	}
	if snap.ScheduledPrice != "" {
		p, err := s.d.Repository.FindPriceByRef(ctx, tx, provider, snap.ScheduledPrice)
		switch {
		case err == nil:
			out.ScheduledPriceID = p.ID
		case !errors.Is(err, ErrNotFound):
			return supporters.Snapshot{}, err
		}
	}
	return out, nil
}

// kernelTime truncates a provider's time to what the database keeps.
func kernelTime(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Millisecond)
}

// SyncSubscription implements Syncer: it applies a snapshot just re-read from the provider. A
// snapshot that names none of our subscriptions, or another provider's subscription than the
// one linked to ours, is ignored.
func (s *Service) SyncSubscription(ctx context.Context, provider string, snap SubscriptionSnapshot) (bool, error) {
	return s.applySnapshot(ctx, provider, snap, "")
}

// applySnapshot applies a snapshot as SyncSubscription does; reason, when not "", names the
// transition that it records instead of the derived one (a command of ours caused it).
func (s *Service) applySnapshot(ctx context.Context, provider string, snap SubscriptionSnapshot, reason supporters.Reason) (bool, error) {
	id := snap.LocalID
	if id == "" && snap.Ref != "" {
		var err error
		id, err = s.d.Repository.FindSubscriptionIDByRef(ctx, nil, provider, snap.Ref)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	if id == "" {
		return false, nil
	}
	now := s.now()
	changed, orphan := false, false
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		changed, orphan = false, false
		rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if rec.Provider != provider {
			return nil
		}
		switch {
		case rec.Link == "" && snap.Ref != "":
			if err := s.d.Repository.InsertLink(ctx, tx, rec.ID, provider, snap.Ref, now); err != nil {
				return err
			}
			rec.Link = snap.Ref
		case snap.Ref != "" && rec.Link != snap.Ref:
			s.d.Logger.WarnContext(ctx, "a provider's subscription names a subscription linked to another one",
				slog.String("subscription", rec.ID), slog.String("provider_subscription", string(snap.Ref)))
			return nil
		}
		if snap.Customer != "" {
			if err := s.d.Repository.InsertCustomer(ctx, tx, Customer{
				ID: s.d.IDs.NewID(), UserID: rec.UserID, Provider: provider, Ref: snap.Customer, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if rec.Current.State.Final() {
			// The provider still bills a subscription that ended here: stop it.
			orphan = snap.Status != StatusCanceled && snap.Status != StatusIncompleteExpired
			if orphan {
				return s.enqueueCommand(ctx, tx, rec.ID, CommandCancelNow, "", "orphan")
			}
			return nil
		}
		ds, err := s.snapshotOf(ctx, tx, provider, snap)
		if err != nil {
			return err
		}
		reversed, err := s.d.Repository.HasRevokingReversal(ctx, tx, rec.ID)
		if err != nil {
			return err
		}
		next, why := supporters.Derive(rec.Current.Transition, &ds, supporters.Facts{Now: now, StartedAt: rec.CreatedAt, Reversed: reversed})
		if why == "" {
			return nil
		}
		if reason != "" {
			why = reason
		}
		changed = true
		return s.transition(ctx, tx, rec, next, why, now)
	})
	if err != nil {
		return false, fmt.Errorf("supporters: apply the snapshot of %s: %w", id, err)
	}
	return changed, nil
}

// transition records the move of rec to next inside tx with its effects (ADR-0072 §7): the
// role granted, revoked, or replaced (as the system), the listing removed with the role, the
// deadline of the Scheduler, the provider's subscription canceled at once when it is revoked,
// the role of a retired plan made ordinary once nobody holds it, and SubscriptionTransitioned,
// which notifies the subscriber of a failed payment and of the end (ADR-0072 §16).
func (s *Service) transition(ctx context.Context, tx uow.Tx, rec SubscriptionRecord, next supporters.Transition, reason supporters.Reason, now time.Time) error {
	tr := TransitionRecord{ID: s.d.IDs.NewID(), SubscriptionID: rec.ID, Transition: next, Reason: reason, OccurredAt: now}
	var dl *Deadline
	if kind, at, ok := supporters.DeadlineOf(next, rec.CreatedAt); ok {
		dl = &Deadline{SubscriptionID: rec.ID, Kind: kind, DueAt: at}
	}
	if err := s.d.Repository.InsertTransition(ctx, tx, tr, rec.UserID, dl); err != nil {
		return err
	}
	accountID, err := s.accountOf(ctx, tx, rec.UserID)
	if err != nil {
		return err
	}
	ev := SubscriptionTransitioned{
		SubscriptionID: rec.ID, TransitionID: tr.ID, UserID: rec.UserID, AccountID: accountID,
		From: rec.Current.State, To: next, Reason: reason, At: now,
	}
	if ev.PaymentFailed() || ev.Ended() {
		p, err := s.d.Repository.FindPlan(ctx, tx, next.PlanID)
		if err != nil {
			return err
		}
		ev.PlanName = p.Revision.Name
	}
	if err := tx.Emit(ctx, ev); err != nil {
		return err
	}
	change := supporters.RoleChangeOf(rec.Current.Transition, next)
	roleOf := func(planID string) (string, bool, error) {
		p, err := s.d.Repository.FindPlan(ctx, tx, planID)
		if err != nil {
			return "", false, err
		}
		return p.RoleID, p.Retired, nil
	}
	switch {
	case change.Revoke != "" && change.Grant != "":
		from, retired, err := roleOf(change.Revoke)
		if err != nil {
			return err
		}
		to, _, err := roleOf(change.Grant)
		if err != nil {
			return err
		}
		if err := s.d.Roles.ReplaceManagedRole(ctx, tx, rec.UserID, from, to, now); err != nil {
			return err
		}
		if retired {
			if _, err := s.d.Roles.UnmanageRole(ctx, tx, from); err != nil {
				return err
			}
		}
	case change.Revoke != "":
		from, retired, err := roleOf(change.Revoke)
		if err != nil {
			return err
		}
		if _, err := s.d.Roles.RevokeManagedRole(ctx, tx, rec.UserID, from, now); err != nil {
			return err
		}
		if err := s.d.Repository.DeleteListing(ctx, tx, rec.UserID); err != nil {
			return err
		}
		if retired {
			if _, err := s.d.Roles.UnmanageRole(ctx, tx, from); err != nil {
				return err
			}
		}
	case change.Grant != "":
		to, _, err := roleOf(change.Grant)
		if err != nil {
			return err
		}
		if _, err := s.d.Roles.GrantManagedRole(ctx, tx, rec.UserID, to, now); err != nil {
			return err
		}
	}
	if next.State == supporters.StateRevoked && rec.Current.State != supporters.StateRevoked && rec.Link != "" {
		return s.enqueueCommand(ctx, tx, rec.ID, CommandCancelNow, "", "revoked")
	}
	return nil
}

// close ends rec inside tx for one of our own reasons (supporters.Close) with its effects.
func (s *Service) close(ctx context.Context, tx uow.Tx, rec SubscriptionRecord, reason supporters.Reason, now time.Time) error {
	next, ok := supporters.Close(rec.Current.Transition, now)
	if !ok {
		return nil
	}
	return s.transition(ctx, tx, rec, next, reason, now)
}

// SyncCheckout implements Syncer: a completed checkout records the user's customer and links
// our subscription to the provider's one; the snapshot that follows sets its state.
func (s *Service) SyncCheckout(ctx context.Context, provider string, res CheckoutResult) (bool, error) {
	if res.Status != CheckoutCompleted {
		return false, nil
	}
	id := res.LocalID
	if id == "" {
		var err error
		id, err = s.d.Repository.FindSubscriptionIDByCheckout(ctx, nil, provider, res.Ref)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	now := s.now()
	changed := false
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		changed = false
		rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil || rec.Provider != provider {
			return err
		}
		if res.Customer != "" {
			if err := s.d.Repository.InsertCustomer(ctx, tx, Customer{
				ID: s.d.IDs.NewID(), UserID: rec.UserID, Provider: provider, Ref: res.Customer, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if rec.Link == "" && res.Subscription != "" {
			changed = true
			return s.d.Repository.InsertLink(ctx, tx, rec.ID, provider, res.Subscription, now)
		}
		return nil
	})
	return changed, err
}

// RecordMoney implements Syncer: it records the refunds and chargebacks of our subscriptions,
// revokes a subscription at a full refund or a chargeback (and cancels it at the provider; a
// partial refund keeps it, and a won dispute gives nothing back, ADR-0072 §7), and hands every
// movement to the income records.
func (s *Service) RecordMoney(ctx context.Context, provider string, ms []MoneyMovement) error {
	for _, m := range ms {
		if !m.Final || m.Subscription == "" || (m.Kind != MoneyRefund && m.Kind != MoneyChargeback) {
			continue
		}
		if err := s.recordReversal(ctx, provider, m); err != nil {
			return err
		}
	}
	if s.d.Income != nil {
		return s.recordIncome(ctx, provider, ms)
	}
	return nil
}

// reversalKind returns the kind of a refund or a chargeback: a refund of the whole payment is a
// full refund.
func reversalKind(m MoneyMovement) ReversalKind {
	if m.Kind == MoneyChargeback {
		return ReversalChargeback
	}
	gross, ok1 := new(big.Rat).SetString(strings.TrimPrefix(m.Gross, "-"))
	parent, ok2 := new(big.Rat).SetString(strings.TrimPrefix(m.ParentGross, "-"))
	if ok1 && ok2 && parent.Sign() > 0 && gross.Cmp(parent) >= 0 {
		return ReversalFullRefund
	}
	return ReversalPartialRefund
}

func (s *Service) recordReversal(ctx context.Context, provider string, m MoneyMovement) error {
	id, err := s.d.Repository.FindSubscriptionIDByRef(ctx, nil, provider, m.Subscription)
	if errors.Is(err, ErrNotFound) {
		s.d.Logger.WarnContext(ctx, "a refund or chargeback of a subscription that is not ours", slog.String("movement", m.ID))
		return nil
	}
	if err != nil {
		return err
	}
	now := s.now()
	kind := reversalKind(m)
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
		if err != nil {
			return err
		}
		inserted, err := s.d.Repository.InsertReversal(ctx, tx, Reversal{
			ID: s.d.IDs.NewID(), SubscriptionID: id, Kind: kind, Provider: provider, ProviderID: m.ID, OccurredAt: kernelTime(m.OccurredAt),
		})
		if err != nil || !inserted || !kind.Revokes() {
			return err
		}
		next, why := supporters.Derive(rec.Current.Transition, nil, supporters.Facts{Now: now, StartedAt: rec.CreatedAt, Reversed: true})
		if why == "" {
			return nil
		}
		return s.transition(ctx, tx, rec, next, why, now)
	})
}

// KnownMoney implements Syncer: the refunds and chargebacks recorded here, and the movements
// that the income records know.
func (s *Service) KnownMoney(ctx context.Context, provider string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if s.d.Income != nil {
		known, err := s.d.Income.KnownIncome(ctx, provider, ids)
		if err != nil {
			return nil, err
		}
		for id, ok := range known {
			out[id] = ok
		}
	}
	rev, err := s.d.Repository.KnownReversals(ctx, provider, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range rev {
		out[id] = true
	}
	return out, nil
}

// OpenCheckouts implements Syncer.
func (s *Service) OpenCheckouts(ctx context.Context, provider string, now time.Time) ([]CheckoutRef, error) {
	return s.d.Repository.OpenCheckouts(ctx, provider, now)
}

// syncPayload is the payload of JobSync.
type syncPayload struct {
	SubscriptionID string `json:"subscription_id"`
}

// commandPayload is the payload of JobCommand.
type commandPayload struct {
	SubscriptionID string            `json:"subscription_id"`
	Command        Command           `json:"command"`
	Reason         supporters.Reason `json:"reason,omitempty"`
}

// SyncJob returns the JobSync of a subscription; cause makes its idempotency key.
func SyncJob(subscriptionID, cause string) queue.NewJob {
	b, err := json.Marshal(syncPayload{SubscriptionID: subscriptionID})
	if err != nil {
		panic(err) // a string always marshals
	}
	return queue.NewJob{
		Type: JobSync, Payload: b, IdempotencyKey: "sync:" + subscriptionID + ":" + cause, OrderingKey: "supporter_subscription:" + subscriptionID,
	}
}

// enqueueCommand queues a JobCommand inside tx; cause makes its idempotency key.
func (s *Service) enqueueCommand(ctx context.Context, tx uow.Tx, subscriptionID string, c Command, reason supporters.Reason, cause string) error {
	b, err := json.Marshal(commandPayload{SubscriptionID: subscriptionID, Command: c, Reason: reason})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobCommand, Payload: b, IdempotencyKey: "command:" + subscriptionID + ":" + string(c) + ":" + cause,
		OrderingKey: "supporter_subscription:" + subscriptionID,
	})
}

// Sync is the work of JobSync (ADR-0072 §6, §7): it re-reads the provider's subscription of
// ours, or, before the provider has one, the latest checkout, and applies what it read; a
// subscription that is still pending a day after its start, without anything at the provider,
// is abandoned.
func (s *Service) Sync(ctx context.Context, id string) queue.Result {
	if s.d.Provider == nil {
		return queue.Done()
	}
	prov, name := s.d.Provider, s.d.Provider.Name()
	rec, err := s.d.Repository.FindSubscription(ctx, nil, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case rec.Provider != name:
		return queue.Done()
	}
	link := rec.Link
	if link == "" && rec.Checkout != nil {
		res, err := prov.GetCheckout(ctx, rec.Checkout.Ref)
		if err != nil && ErrorKindOf(err) != ErrorNotFound {
			return s.providerFailure(ctx, "get checkout", err)
		}
		if err == nil {
			if _, err := s.SyncCheckout(ctx, name, res); err != nil {
				return queue.Retry(0, err)
			}
			link = res.Subscription
		}
	}
	if link == "" {
		if err := s.expirePending(ctx, id); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	}
	snap, err := prov.GetSubscription(ctx, link)
	if err != nil {
		return s.providerFailure(ctx, "get subscription", err)
	}
	if snap.LocalID == "" {
		snap.LocalID = id
	}
	if _, err := s.SyncSubscription(ctx, name, snap); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// expirePending abandons a pending subscription that the provider has nothing of a day after
// its start.
func (s *Service) expirePending(ctx context.Context, id string) error {
	now := s.now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
		if err != nil || rec.Link != "" {
			return err
		}
		next, why := supporters.Derive(rec.Current.Transition, nil, supporters.Facts{Now: now, StartedAt: rec.CreatedAt})
		if why == "" {
			return nil
		}
		return s.transition(ctx, tx, rec, next, why, now)
	})
}

// RunCommand is the work of JobCommand: it sends the command for the provider's subscription
// (idempotent with key), then re-reads and applies it with reason. A pending subscription
// without the provider's one is closed here instead.
func (s *Service) RunCommand(ctx context.Context, p commandPayload, key string) queue.Result {
	if s.d.Provider == nil {
		return queue.Done()
	}
	prov, name := s.d.Provider, s.d.Provider.Name()
	rec, err := s.d.Repository.FindSubscription(ctx, nil, p.SubscriptionID)
	switch {
	case errors.Is(err, ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case rec.Provider != name:
		return queue.Done()
	}
	if rec.Link == "" {
		if !rec.Current.State.Final() && p.Reason != "" {
			now := s.now()
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				rec, err := s.d.Repository.LockSubscription(ctx, tx, p.SubscriptionID)
				if err != nil || rec.Link != "" {
					return err
				}
				return s.close(ctx, tx, rec, p.Reason, now)
			})
			if err != nil {
				return queue.Retry(0, err)
			}
		}
		return queue.Done()
	}
	switch p.Command {
	case CommandCancelNow:
		err = prov.CancelNow(ctx, rec.Link, key)
	case CommandCancelAtPeriodEnd:
		if rec.Current.State.Final() {
			return queue.Done()
		}
		if err = prov.CancelScheduledChange(ctx, rec.Link, key+":scheduled"); err == nil {
			err = prov.CancelAtPeriodEnd(ctx, rec.Link, key)
		}
	default:
		return queue.Fail(fmt.Errorf("supporters: unknown command %q", p.Command))
	}
	if err != nil {
		return s.providerFailure(ctx, string(p.Command)+" "+rec.ID, err)
	}
	snap, err := prov.GetSubscription(ctx, rec.Link)
	if err != nil {
		return s.providerFailure(ctx, "get subscription", err)
	}
	if snap.LocalID == "" {
		snap.LocalID = rec.ID
	}
	if _, err := s.applySnapshot(ctx, name, snap, p.Reason); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// QueueDeadlines is the work of JobDeadlines: it queues JobSync for the subscriptions whose
// deadline passed (ADR-0072 §7), each once per deadline.
func (s *Service) QueueDeadlines(ctx context.Context) error {
	due, err := s.d.Repository.DueDeadlines(ctx, s.now(), DeadlineBatch)
	if err != nil || len(due) == 0 {
		return err
	}
	jobs := make([]queue.NewJob, len(due))
	for i, d := range due {
		jobs[i] = SyncJob(d.SubscriptionID, fmt.Sprintf("%s:%d", d.Kind, d.DueAt.UnixMilli()))
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, jobs...) })
}

// RegisterJobs registers the jobs of the plans and the subscriptions and, when sc is not nil,
// the schedule of JobDeadlines.
func (s *Service) RegisterJobs(h *queue.JobHandlerRegistry, sc *queue.Scheduler) {
	h.Register(JobEnsurePrices, func(ctx context.Context, j queue.Job) queue.Result {
		var p ensurePricesPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.PlanID == "" {
			return queue.Fail(fmt.Errorf("supporters: the payload of %s: %w", j.ID, err))
		}
		return s.EnsurePrices(ctx, p.PlanID)
	})
	h.Register(JobSync, func(ctx context.Context, j queue.Job) queue.Result {
		var p syncPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.SubscriptionID == "" {
			return queue.Fail(fmt.Errorf("supporters: the payload of %s: %w", j.ID, err))
		}
		return s.Sync(ctx, p.SubscriptionID)
	})
	h.Register(JobCommand, func(ctx context.Context, j queue.Job) queue.Result {
		var p commandPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.SubscriptionID == "" {
			return queue.Fail(fmt.Errorf("supporters: the payload of %s: %w", j.ID, err))
		}
		key := j.IdempotencyKey
		if key == "" {
			key = "job:" + j.ID
		}
		return s.RunCommand(ctx, p, key)
	})
	s.registerClosureJobs(h)
	h.Register(JobDeadlines, func(ctx context.Context, _ queue.Job) queue.Result {
		if err := s.QueueDeadlines(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	if sc != nil {
		sc.Add(queue.Schedule{Name: JobDeadlines, Every: DeadlineInterval, Job: queue.NewJob{Type: JobDeadlines}})
	}
}
