// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// A start leaves at most one page of a checkout payable, but two payments can still meet: both
// pages submitted at once, a page completed after its expiry failed, or a page that the provider
// cannot withdraw paid after our subscription closed. Each makes a provider's subscription that
// buys nothing, which the server cancels and refunds (ADR-0072 §6). The record stays unsettled
// until both are done, shows as a warning to the administrators, and each reconciliation tries it
// again.

// recordDuplicate records a provider's subscription to cancel and refund, inside tx, and queues
// the job that does it, once per subscription of the provider.
func (s *Service) recordDuplicate(ctx context.Context, tx uow.Tx, rec SubscriptionRecord, provider string, ref SubscriptionRef, now time.Time) error {
	inserted, err := s.d.Repository.InsertDuplicate(ctx, tx, Duplicate{
		ID: s.d.IDs.NewID(), SubscriptionID: rec.ID, Provider: provider, Ref: ref, DetectedAt: now,
	})
	if err != nil || !inserted {
		return err
	}
	return s.enqueueSettle(ctx, tx, rec.ID, ref, string(ref))
}

func (s *Service) enqueueSettle(ctx context.Context, tx uow.Tx, subscriptionID string, ref SubscriptionRef, cause string) error {
	return s.enqueue(ctx, tx, commandPayload{SubscriptionID: subscriptionID, Command: CommandSettleDuplicate, Ref: ref}, cause)
}

// RetryDuplicates implements Syncer.
func (s *Service) RetryDuplicates(ctx context.Context, provider, cause string) error {
	ds, err := s.d.Repository.UnsettledDuplicates(ctx, provider)
	if err != nil || len(ds) == 0 {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, d := range ds {
			if err := s.enqueueSettle(ctx, tx, d.SubscriptionID, d.Ref, string(d.Ref)+":"+cause); err != nil {
				return err
			}
		}
		return nil
	})
}

// subscriptionEnded reports whether a provider's subscription can no longer bill.
func subscriptionEnded(status SubscriptionStatus) bool {
	return status == StatusCanceled || status == StatusIncompleteExpired
}

// settleDuplicate is the work of CommandSettleDuplicate. The first attempt re-reads the provider's
// subscription: one that already ended was ended by someone else, and what they did about its charge
// stays, and one that is not what the record says is left alone until someone ends it. Otherwise it
// records that it takes the subscription on, since the provider may no longer report it after the
// cancellation (PAY.JP deletes it), and every later attempt goes on from there. A failure that a retry cannot
// help leaves the record unsettled for the administrators and the next reconciliation.
func (s *Service) settleDuplicate(ctx context.Context, rec SubscriptionRecord, ref SubscriptionRef, key string) queue.Result {
	prov, name := s.d.Provider, s.d.Provider.Name()
	d, err := s.d.Repository.FindDuplicate(ctx, nil, name, ref)
	switch {
	case errors.Is(err, ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case d.Settled || d.SubscriptionID != rec.ID:
		return queue.Done()
	}
	attrs := []any{slog.String("subscription", rec.ID), slog.String("provider_subscription", string(ref))}
	failed := func(what string, err error) queue.Result {
		kind := ErrorKindOf(err)
		if kind.Retried() {
			return queue.Retry(0, fmt.Errorf("supporters: %s the second subscription %s: %w", what, ref, err))
		}
		s.d.Logger.ErrorContext(ctx, "a second subscription could not be canceled and refunded; check the provider's dashboard",
			append(attrs, slog.String("call", what), slog.String("error_kind", string(kind)), slog.Any("error", err))...)
		return queue.Done()
	}
	settle := func() queue.Result {
		now := s.now()
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.SettleDuplicate(ctx, tx, d.ID, now) })
		if err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	}
	if !d.Cancelling {
		snap, err := prov.GetSubscription(ctx, ref)
		switch {
		case ErrorKindOf(err) == ErrorNotFound || (err == nil && subscriptionEnded(snap.Status)):
			s.d.Logger.WarnContext(ctx, "a second subscription had already ended at the provider; the server leaves its charges as they are", attrs...)
			return settle()
		case err != nil:
			return failed("read", err)
		case snap.LocalID != rec.ID || (rec.Link == ref && !rec.Current.State.Final()):
			s.d.Logger.ErrorContext(ctx, "a second subscription is not what it was recorded as; the server leaves it alone", attrs...)
			return queue.Done()
		}
		now := s.now()
		err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.CancelDuplicate(ctx, tx, d.ID, now) })
		if err != nil {
			return queue.Retry(0, err)
		}
	}
	// The refund comes first, since a provider that deletes what it cancels (PAY.JP) may no longer
	// list its charges. A failure that a retry may cure waits with the subscription as it is; any
	// other still stops the billing, as a key without the permission to refund does. A new key for
	// every attempt: Stripe replays a stored failure under a key, and a refund of a whole charge
	// cannot run twice.
	refundErr := prov.RefundSubscription(ctx, ref, key+":"+s.d.IDs.NewID())
	if refundErr != nil && ErrorKindOf(refundErr).Retried() {
		return failed("refund", refundErr)
	}
	if err := prov.CancelNow(ctx, ref, key); err != nil && ErrorKindOf(err) != ErrorNotFound {
		return failed("cancel", err)
	}
	if refundErr != nil {
		return failed("refund", refundErr)
	}
	if r := settle(); !r.IsDone() {
		return r
	}
	s.d.Logger.WarnContext(ctx, "a second subscription was canceled and refunded", attrs...)
	return queue.Done()
}

// expireCheckouts is the work of CommandExpireCheckouts. A page paid before its expiry is linked
// here, which records it as a duplicate.
func (s *Service) expireCheckouts(ctx context.Context, rec SubscriptionRecord) queue.Result {
	refs, err := s.d.Repository.LiveCheckouts(ctx, nil, rec.ID, s.now())
	if err != nil {
		return queue.Retry(0, err)
	}
	name := s.d.Provider.Name()
	for _, ref := range refs {
		res, err := s.d.Provider.ExpireCheckout(ctx, ref)
		if ErrorKindOf(err) == ErrorNotFound {
			continue
		}
		if err != nil {
			return s.providerFailure(ctx, "expire a checkout of "+rec.ID, err)
		}
		if res.Status == CheckoutCompleted {
			if _, err := s.SyncCheckout(ctx, name, res); err != nil {
				return queue.Retry(0, err)
			}
		}
	}
	return queue.Done()
}
