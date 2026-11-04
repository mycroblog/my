// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	stripe "github.com/stripe/stripe-go/v87"

	"github.com/mycroblog/my/internal/app/supporters"
)

// subscriptionExpand are the objects that a snapshot reads with the subscription: the latest
// invoice (a failed payment) and the schedule (a change at the period's end).
var subscriptionExpand = []string{"latest_invoice", "schedule"}

func expand(prefix string, paths []string) []*string {
	out := make([]*string, len(paths))
	for i, p := range paths {
		out[i] = stripe.String(prefix + p)
	}
	return out
}

// statuses maps Stripe's states of a subscription to the port's (ADR-0074 §1). A trial is
// active: the plans have none.
var statuses = map[stripe.SubscriptionStatus]supporters.SubscriptionStatus{
	stripe.SubscriptionStatusIncomplete:        supporters.StatusIncomplete,
	stripe.SubscriptionStatusIncompleteExpired: supporters.StatusIncompleteExpired,
	stripe.SubscriptionStatusTrialing:          supporters.StatusActive,
	stripe.SubscriptionStatusActive:            supporters.StatusActive,
	stripe.SubscriptionStatusPastDue:           supporters.StatusPastDue,
	stripe.SubscriptionStatusUnpaid:            supporters.StatusUnpaid,
	stripe.SubscriptionStatusPaused:            supporters.StatusPaused,
	stripe.SubscriptionStatusCanceled:          supporters.StatusCanceled,
}

// snapshot returns the port's snapshot of a subscription read with subscriptionExpand.
func (p *Provider) snapshot(ctx context.Context, s *stripe.Subscription) (supporters.SubscriptionSnapshot, error) {
	if err := checkItems(s); err != nil {
		return supporters.SubscriptionSnapshot{}, err
	}
	item := s.Items.Data[0]
	snap := supporters.SubscriptionSnapshot{
		Ref: supporters.SubscriptionRef(s.ID), LocalID: s.Metadata[metaSubscriptionID], Price: supporters.PriceRef(item.Price.ID),
		PeriodStart: unixTime(item.CurrentPeriodStart), PeriodEnd: unixTime(item.CurrentPeriodEnd),
		CancelAtPeriodEnd: cancelsByPeriodEnd(s),
	}
	if s.CancelAt != 0 && s.CancelAt < item.CurrentPeriodEnd {
		// Set by hand before the period's end, the subscription ends then: the date on the screen
		// and the deadline of the re-read follow it.
		snap.PeriodEnd = unixTime(s.CancelAt)
	}
	if s.Customer != nil {
		snap.Customer = supporters.CustomerRef(s.Customer.ID)
	}
	status, ok := statuses[s.Status]
	if !ok {
		p.log.WarnContext(ctx, "unknown state of a Stripe subscription; it gets no role",
			slog.String("subscription", s.ID), slog.String("status", string(s.Status)))
		status = supporters.StatusUnpaid
	}
	snap.Status = status
	snap.ScheduledPrice = scheduledPrice(s, item.CurrentPeriodEnd)
	snap.LatestPaymentFailed = paymentFailed(s.LatestInvoice)
	if s.PendingUpdate != nil {
		// The latest invoice is the change's, not a renewal's: only an active subscription changes
		// at once, and a pending update expires by the period's end. Its failure is not a late
		// payment, whatever status Stripe gives it meanwhile, which its documentation does not say.
		snap.LatestPaymentFailed = time.Time{}
		if snap.Status == supporters.StatusPastDue {
			snap.Status = supporters.StatusActive
		}
		snap.UnpaidChange = s.ID
		if s.LatestInvoice != nil {
			snap.UnpaidChange = s.LatestInvoice.ID
		}
	}
	return snap, nil
}

// cancelsByPeriodEnd reports whether a subscription ends by the end of its current period. The
// customer portal sets cancel_at to that time and leaves cancel_at_period_end false, so either
// field can say it. A cancel_at after the period's end is not a cancellation yet: the subscription
// renews at least once, and it goes on as active until the period that holds that date.
func cancelsByPeriodEnd(s *stripe.Subscription) bool {
	if s.CancelAtPeriodEnd {
		return true
	}
	return s.CancelAt != 0 && s.CancelAt <= s.Items.Data[0].CurrentPeriodEnd
}

// pendingSchedule returns the subscription's schedule while it still waits to change it.
func pendingSchedule(s *stripe.Subscription) *stripe.SubscriptionSchedule {
	sch := s.Schedule
	if sch == nil || sch.ID == "" {
		return nil
	}
	if sch.Status != "" && sch.Status != stripe.SubscriptionScheduleStatusActive && sch.Status != stripe.SubscriptionScheduleStatusNotStarted {
		return nil
	}
	return sch
}

// scheduledPrice returns the price of the schedule's phase that starts at or after periodEnd,
// when it differs from the current one.
func scheduledPrice(s *stripe.Subscription, periodEnd int64) supporters.PriceRef {
	sch := pendingSchedule(s)
	if sch == nil {
		return ""
	}
	for _, ph := range sch.Phases {
		if ph.StartDate < periodEnd || len(ph.Items) == 0 || ph.Items[0].Price == nil {
			continue
		}
		if price := ph.Items[0].Price.ID; price != s.Items.Data[0].Price.ID {
			return supporters.PriceRef(price)
		}
		return ""
	}
	return ""
}

// paymentFailed returns when the payment of an invoice that is still open first failed: Stripe
// attempts an invoice of a subscription when it finalizes it, so the first failure is at the
// finalization. Zero when the invoice is paid, not attempted yet, or not read.
func paymentFailed(inv *stripe.Invoice) time.Time {
	if inv == nil || inv.Status != stripe.InvoiceStatusOpen || !inv.Attempted || inv.AttemptCount == 0 || inv.StatusTransitions == nil {
		return time.Time{}
	}
	return unixTime(inv.StatusTransitions.FinalizedAt)
}

// GetSubscription implements supporters.PaymentProvider.
func (p *Provider) GetSubscription(ctx context.Context, ref supporters.SubscriptionRef) (supporters.SubscriptionSnapshot, error) {
	s, err := p.subscription(ctx, ref)
	if err != nil {
		return supporters.SubscriptionSnapshot{}, err
	}
	return p.snapshot(ctx, s)
}

func (p *Provider) subscription(ctx context.Context, ref supporters.SubscriptionRef) (*stripe.Subscription, error) {
	if ref == "" {
		return nil, fmt.Errorf("stripe: %w: no subscription", supporters.ErrPaymentNotFound)
	}
	var s *stripe.Subscription
	err := p.do(ctx, "read the subscription "+string(ref), func() (err error) {
		s, err = p.sc.V1Subscriptions.Retrieve(ctx, string(ref), &stripe.SubscriptionRetrieveParams{Expand: expand("", subscriptionExpand)})
		return err
	})
	if err != nil {
		return nil, err
	}
	return s, checkItems(s)
}

// checkItems refuses a subscription without the one priced item of the plans.
func checkItems(s *stripe.Subscription) error {
	if s.Items == nil || len(s.Items.Data) == 0 || s.Items.Data[0].Price == nil {
		return fmt.Errorf("stripe: %w: subscription %s has no price", supporters.ErrInvalidPaymentResponse, s.ID)
	}
	return nil
}

// ListSubscriptions implements supporters.PaymentProvider: every subscription of the account
// (status=all), canceled ones included, newest first, with their latest invoices and schedules.
// The cursor is the ID of the last subscription of the previous page.
func (p *Provider) ListSubscriptions(ctx context.Context, cursor string) (supporters.SubscriptionPage, error) {
	params := &stripe.SubscriptionListParams{
		ListParams: stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true},
		Status:     stripe.String("all"),
		Expand:     expand("data.", subscriptionExpand),
	}
	if cursor != "" {
		params.StartingAfter = stripe.String(cursor)
	}
	var list *stripe.V1List[*stripe.Subscription]
	err := p.do(ctx, "list the subscriptions", func() error {
		list = p.sc.V1Subscriptions.List(ctx, params)
		return list.Err()
	})
	if err != nil {
		return supporters.SubscriptionPage{}, err
	}
	var page supporters.SubscriptionPage
	for _, s := range list.Data() {
		snap, err := p.snapshot(ctx, s)
		if err != nil {
			return supporters.SubscriptionPage{}, err
		}
		page.Subscriptions = append(page.Subscriptions, snap)
	}
	if data := list.Data(); list.Meta().HasMore && len(data) > 0 {
		page.Next = data[len(data)-1].ID
	}
	return page, nil
}

// ChangePrice implements supporters.PaymentProvider (ADR-0074 §2). ChangeNowProrated: see
// changeNow. ChangeAtPeriodEnd puts the subscription on a schedule whose next phase, from the end of the
// current period, has the new price; the schedule releases the subscription after that phase.
func (p *Provider) ChangePrice(ctx context.Context, ref supporters.SubscriptionRef, to supporters.PriceRef, when supporters.ChangeTiming, key string) error {
	if to == "" {
		return fmt.Errorf("stripe: %w: no price", supporters.ErrPaymentInvalidRequest)
	}
	s, err := p.subscription(ctx, ref)
	if err != nil {
		return err
	}
	if ended(s) {
		return fmt.Errorf("stripe: %w: subscription %s ended", supporters.ErrPaymentInvalidRequest, ref)
	}
	if cancelsByPeriodEnd(s) {
		// The portal's cancellation may not have reached our state yet; a change needs it undone.
		return fmt.Errorf("stripe: %w: subscription %s", supporters.ErrPaymentCanceling, ref)
	}
	item := s.Items.Data[0]
	current := supporters.PriceRef(item.Price.ID)
	switch when {
	case supporters.ChangeNowProrated:
		if current == to {
			// The same price: only a change waiting for the period's end may be left to drop.
			return p.release(ctx, s, key)
		}
		return p.changeNow(ctx, s, to, key)
	case supporters.ChangeAtPeriodEnd:
		if current == to {
			return p.release(ctx, s, key)
		}
		if scheduledPrice(s, item.CurrentPeriodEnd) == to {
			return nil
		}
		return p.scheduleChange(ctx, s, to, key)
	default:
		return fmt.Errorf("stripe: %w: unknown timing %q", supporters.ErrPaymentInvalidRequest, when)
	}
}

// changeNow replaces the price of the subscription's item and invoices the prorated difference at
// once (proration_behavior=always_invoice). With payment_behavior=pending_if_incomplete the new
// price applies only when that invoice is paid; otherwise Stripe keeps it in pending_update, and
// this voids the invoice, which also discards the update (ADR-0074 §2): nothing stays collectible.
//
// The key of the update names the latest invoice and the minute of the attempt. A new attempt after
// a failed payment follows another invoice, so it does not replay the failure, and a stored 500
// goes away in a minute; two submissions at once share the key. Each attempt reads the subscription
// first, so one after a success sends nothing and none charges twice.
//
// A change that waited for the period's end is released first, since a schedule's next phase would
// discard the update, and made again when the payment fails.
func (p *Provider) changeNow(ctx context.Context, s *stripe.Subscription, to supporters.PriceRef, key string) error {
	if s.PendingUpdate != nil {
		processing, err := p.invoiceProcessing(ctx, s.LatestInvoice)
		if err != nil {
			return err
		}
		if processing {
			return fmt.Errorf("stripe: %w: subscription %s", supporters.ErrPaymentProcessing, s.ID)
		}
	}
	item := s.Items.Data[0]
	scheduled := scheduledPrice(s, item.CurrentPeriodEnd)
	if err := p.release(ctx, s, key); err != nil {
		return err
	}
	latest := ""
	if s.LatestInvoice != nil {
		latest = s.LatestInvoice.ID
	}
	op := fmt.Sprintf("change_now:%s:%d", latest, p.o.Now().Unix()/60)
	refused := p.do(ctx, "change the price of "+s.ID, func() (err error) {
		_, err = p.sc.V1Subscriptions.Update(ctx, s.ID, &stripe.SubscriptionUpdateParams{
			Params:            idempotency(key, op),
			Items:             []*stripe.SubscriptionUpdateItemParams{{ID: stripe.String(item.ID), Price: stripe.String(string(to))}},
			ProrationBehavior: stripe.String("always_invoice"),
			PaymentBehavior:   stripe.String("pending_if_incomplete"),
		})
		return err
	})
	if refused != nil && !errors.Is(refused, supporters.ErrPaymentDeclined) {
		return refused
	}
	after, err := p.subscription(ctx, supporters.SubscriptionRef(s.ID))
	if err != nil {
		return err
	}
	if after.PendingUpdate == nil && supporters.PriceRef(after.Items.Data[0].Price.ID) == to {
		return nil
	}
	if after.PendingUpdate == nil && refused == nil {
		return fmt.Errorf("stripe: %w: subscription %s neither changed nor waits for its payment", supporters.ErrInvalidPaymentResponse, s.ID)
	}
	processing, err := p.dropUnpaid(ctx, after, key)
	if err != nil {
		return err
	}
	if processing {
		return fmt.Errorf("stripe: %w: subscription %s", supporters.ErrPaymentProcessing, s.ID)
	}
	if scheduled != "" {
		// The key names the schedule released here: each attempt makes a schedule of its own.
		if err := p.scheduleChange(ctx, after, scheduled, key+":restore:"+pendingSchedule(s).ID); err != nil {
			p.log.WarnContext(ctx, "could not make again the scheduled change that a failed change released",
				slog.String("subscription", s.ID), slog.Any("error", err))
		}
	}
	return fmt.Errorf("stripe: %w: subscription %s", supporters.ErrPaymentDeclined, s.ID)
}

// invoiceProcessing reports whether the payment of an open invoice has not settled yet. Voiding it
// then could lose a payment that still succeeds.
func (p *Provider) invoiceProcessing(ctx context.Context, inv *stripe.Invoice) (bool, error) {
	if inv == nil || inv.Status != stripe.InvoiceStatusOpen {
		return false, nil
	}
	var list *stripe.V1List[*stripe.InvoicePayment]
	err := p.do(ctx, "list the payments of the invoice "+inv.ID, func() error {
		list = p.sc.V1InvoicePayments.List(ctx, &stripe.InvoicePaymentListParams{
			ListParams: stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true},
			Invoice:    stripe.String(inv.ID), Expand: expand("data.", []string{"payment.payment_intent"}),
		})
		return list.Err()
	})
	if err != nil {
		return false, err
	}
	for _, ip := range list.Data() {
		if ip.Payment != nil && ip.Payment.PaymentIntent != nil && ip.Payment.PaymentIntent.Status == stripe.PaymentIntentStatusProcessing {
			return true, nil
		}
	}
	return false, nil
}

// dropUnpaid voids the open invoice of the pending update of s, read with subscriptionExpand, which
// is how Stripe documents canceling one, unless its payment is processing, which it reports. An
// invoice that it can no longer void was voided or paid meanwhile, which needs nothing more.
func (p *Provider) dropUnpaid(ctx context.Context, s *stripe.Subscription, key string) (processing bool, err error) {
	inv := s.LatestInvoice
	if s.PendingUpdate == nil || inv == nil || inv.Status != stripe.InvoiceStatusOpen {
		return false, nil
	}
	if processing, err = p.invoiceProcessing(ctx, inv); err != nil || processing {
		return processing, err
	}
	err = p.do(ctx, "void the invoice "+inv.ID, func() (err error) {
		_, err = p.sc.V1Invoices.VoidInvoice(ctx, inv.ID, &stripe.InvoiceVoidInvoiceParams{Params: idempotency(key, "void:"+inv.ID)})
		return err
	})
	if errors.Is(err, supporters.ErrPaymentInvalidRequest) {
		return false, nil
	}
	return false, err
}

// DropUnpaidChange implements supporters.PaymentProvider.
func (p *Provider) DropUnpaidChange(ctx context.Context, ref supporters.SubscriptionRef, key string) error {
	s, err := p.subscription(ctx, ref)
	if err != nil {
		return err
	}
	if ended(s) {
		return nil
	}
	_, err = p.dropUnpaid(ctx, s, key)
	return err
}

// scheduleChange sets the schedule of s to two phases: the current price until the period's
// end, then to for one interval of it, after which the schedule releases the subscription.
func (p *Provider) scheduleChange(ctx context.Context, s *stripe.Subscription, to supporters.PriceRef, key string) error {
	var target *stripe.Price
	err := p.do(ctx, "read the price "+string(to), func() (err error) {
		target, err = p.sc.V1Prices.Retrieve(ctx, string(to), &stripe.PriceRetrieveParams{})
		return err
	})
	if err != nil {
		return err
	}
	if target.Recurring == nil {
		return fmt.Errorf("stripe: %w: price %s does not recur", supporters.ErrPaymentInvalidRequest, to)
	}
	sch := pendingSchedule(s)
	if sch == nil {
		err := p.do(ctx, "create a schedule of "+s.ID, func() (err error) {
			sch, err = p.sc.V1SubscriptionSchedules.Create(ctx, &stripe.SubscriptionScheduleCreateParams{
				Params: idempotency(key, "schedule"), FromSubscription: stripe.String(s.ID),
			})
			return err
		})
		if err != nil {
			if sch = p.attachedSchedule(ctx, s, err); sch == nil {
				return err
			}
		}
	}
	item := s.Items.Data[0]
	start := item.CurrentPeriodStart
	if sch.CurrentPhase != nil && sch.CurrentPhase.StartDate != 0 {
		start = sch.CurrentPhase.StartDate
	} else if len(sch.Phases) > 0 && sch.Phases[0].StartDate != 0 {
		start = sch.Phases[0].StartDate
	}
	interval := string(target.Recurring.Interval)
	count := max(target.Recurring.IntervalCount, 1)
	return p.do(ctx, "schedule the change of "+s.ID, func() (err error) {
		_, err = p.sc.V1SubscriptionSchedules.Update(ctx, sch.ID, &stripe.SubscriptionScheduleUpdateParams{
			Params:            idempotency(key, "schedule_phases"),
			EndBehavior:       stripe.String("release"),
			ProrationBehavior: stripe.String("none"),
			Phases: []*stripe.SubscriptionScheduleUpdatePhaseParams{
				{
					Items:     []*stripe.SubscriptionScheduleUpdatePhaseItemParams{{Price: stripe.String(item.Price.ID), Quantity: stripe.Int64(max(item.Quantity, 1))}},
					StartDate: stripe.Int64(start), EndDate: stripe.Int64(item.CurrentPeriodEnd),
					ProrationBehavior: stripe.String("none"),
				},
				{
					Items:             []*stripe.SubscriptionScheduleUpdatePhaseItemParams{{Price: stripe.String(string(to)), Quantity: stripe.Int64(1)}},
					Duration:          &stripe.SubscriptionScheduleUpdatePhaseDurationParams{Interval: stripe.String(interval), IntervalCount: stripe.Int64(count)},
					ProrationBehavior: stripe.String("none"),
				},
			},
		})
		return err
	})
}

// attachedSchedule returns the schedule that a request running at the same time attached to s
// after it was read, which is why Stripe refused to create one with err; nil for any other failure.
func (p *Provider) attachedSchedule(ctx context.Context, s *stripe.Subscription, err error) *stripe.SubscriptionSchedule {
	if !errors.Is(err, supporters.ErrPaymentInvalidRequest) {
		return nil
	}
	again, rerr := p.subscription(ctx, supporters.SubscriptionRef(s.ID))
	if rerr != nil {
		return nil
	}
	return pendingSchedule(again)
}

// release releases the schedule of s that still waits to change it; nothing without one. The key
// names the schedule: a retried command can meet a second schedule, when a new change at the
// period's end was made after its first attempt released the first.
func (p *Provider) release(ctx context.Context, s *stripe.Subscription, key string) error {
	sch := pendingSchedule(s)
	if sch == nil {
		return nil
	}
	return p.do(ctx, "release the schedule of "+s.ID, func() (err error) {
		_, err = p.sc.V1SubscriptionSchedules.Release(ctx, sch.ID, &stripe.SubscriptionScheduleReleaseParams{
			Params: idempotency(key, "release:"+sch.ID), PreserveCancelDate: stripe.Bool(true),
		})
		return err
	})
}

// CancelScheduledChange implements supporters.PaymentProvider: it releases the schedule.
func (p *Provider) CancelScheduledChange(ctx context.Context, ref supporters.SubscriptionRef, key string) error {
	s, err := p.subscription(ctx, ref)
	if err != nil {
		return err
	}
	return p.release(ctx, s, key)
}

// CancelAtPeriodEnd implements supporters.PaymentProvider. Stripe refuses to change the
// cancellation of a subscription on a schedule, so a change waiting for the period's end is
// dropped first: the subscription ends before it would take effect. A subscription that the portal
// canceled is canceled already; a cancel_at in a later period moves to this period's end.
func (p *Provider) CancelAtPeriodEnd(ctx context.Context, ref supporters.SubscriptionRef, key string) error {
	s, err := p.subscription(ctx, ref)
	if err != nil {
		return err
	}
	if ended(s) || cancelsByPeriodEnd(s) {
		return nil
	}
	if err := p.release(ctx, s, key); err != nil {
		return err
	}
	return p.do(ctx, "cancel_at_period_end "+s.ID, func() (err error) {
		_, err = p.sc.V1Subscriptions.Update(ctx, s.ID, &stripe.SubscriptionUpdateParams{
			Params: idempotency(key, "cancel_at_period_end"), CancelAtPeriodEnd: stripe.Bool(true),
		})
		return err
	})
}

// ResumeCancellation implements supporters.PaymentProvider. It clears both fields that can hold a
// cancellation, cancel_at_period_end first and then whatever cancel_at it left, one request each:
// Stripe's documentation does not say that one clears the other or that one update may send both.
func (p *Provider) ResumeCancellation(ctx context.Context, ref supporters.SubscriptionRef, key string) error {
	s, err := p.subscription(ctx, ref)
	if err != nil {
		return err
	}
	if ended(s) || (!s.CancelAtPeriodEnd && s.CancelAt == 0) {
		return nil
	}
	if err := p.release(ctx, s, key); err != nil {
		return err
	}
	cancelAt := s.CancelAt
	if s.CancelAtPeriodEnd {
		err := p.do(ctx, "resume "+s.ID, func() error {
			after, err := p.sc.V1Subscriptions.Update(ctx, s.ID, &stripe.SubscriptionUpdateParams{
				Params: idempotency(key, "resume"), CancelAtPeriodEnd: stripe.Bool(false),
			})
			if err == nil {
				cancelAt = after.CancelAt
			}
			return err
		})
		if err != nil {
			return err
		}
	}
	if cancelAt == 0 {
		return nil
	}
	return p.do(ctx, "clear cancel_at of "+s.ID, func() error {
		params := &stripe.SubscriptionUpdateParams{Params: idempotency(key, "resume_cancel_at")}
		params.AddUnsetField(stripe.SubscriptionUpdateParamsUnsetFieldCancelAt)
		_, err := p.sc.V1Subscriptions.Update(ctx, s.ID, params)
		return err
	})
}

// CancelNow implements supporters.PaymentProvider: the subscription ends now, without
// prorating or invoicing what is left (prorate=false, invoice_now=false).
func (p *Provider) CancelNow(ctx context.Context, ref supporters.SubscriptionRef, key string) error {
	s, err := p.subscription(ctx, ref)
	if err != nil {
		return err
	}
	if ended(s) {
		return nil
	}
	return p.do(ctx, "cancel "+s.ID, func() (err error) {
		_, err = p.sc.V1Subscriptions.Cancel(ctx, s.ID, &stripe.SubscriptionCancelParams{
			Params: idempotency(key, "cancel_now"), Prorate: stripe.Bool(false), InvoiceNow: stripe.Bool(false),
		})
		return err
	})
}

// RefundSubscription implements supporters.PaymentProvider: a refund of the whole of each charge of
// the subscription's paid invoices. Without an amount a refund takes what is left of its charge, and
// Stripe refuses a charge with nothing left, so a repeat refunds nothing twice. The refund goes to the
// card charged and takes Stripe's fee of the charge from nobody: Stripe keeps it.
func (p *Provider) RefundSubscription(ctx context.Context, ref supporters.SubscriptionRef, key string) error {
	params := &stripe.InvoiceListParams{
		ListParams:   stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true},
		Subscription: stripe.String(string(ref)), Status: stripe.String(string(stripe.InvoiceStatusPaid)),
	}
	for {
		var list *stripe.V1List[*stripe.Invoice]
		err := p.do(ctx, "list the paid invoices of "+string(ref), func() error {
			list = p.sc.V1Invoices.List(ctx, params)
			return list.Err()
		})
		if err != nil {
			return err
		}
		data := list.Data()
		for _, inv := range data {
			charges, err := p.paidCharges(ctx, inv.ID, subscriptionLink{})
			if err != nil {
				return err
			}
			for _, charge := range charges {
				if err := p.refundCharge(ctx, charge, key); err != nil {
					return err
				}
			}
		}
		if !list.Meta().HasMore || len(data) == 0 {
			return nil
		}
		params.StartingAfter = stripe.String(data[len(data)-1].ID)
	}
}

// refundCharge refunds what is left of a charge. A charge with nothing left is done. A charge in a
// dispute cannot be refunded, and the dispute decides where its money goes.
func (p *Provider) refundCharge(ctx context.Context, charge, key string) error {
	return p.do(ctx, "refund the charge "+charge, func() error {
		_, err := p.sc.V1Refunds.Create(ctx, &stripe.RefundCreateParams{
			Params: idempotency(key, "refund:"+charge), Charge: stripe.String(charge), Reason: stripe.String(string(stripe.RefundReasonDuplicate)),
		})
		var se *stripe.Error
		if errors.As(err, &se) {
			switch se.Code {
			case stripe.ErrorCodeChargeAlreadyRefunded:
				return nil
			case stripe.ErrorCodeChargeDisputed, stripe.ErrorCodeRefundDisputedPayment:
				p.log.WarnContext(ctx, "a charge in a dispute cannot be refunded", slog.String("charge", charge))
				return nil
			}
		}
		return err
	})
}

// ended reports whether a subscription can no longer change.
func ended(s *stripe.Subscription) bool {
	return s.Status == stripe.SubscriptionStatusCanceled || s.Status == stripe.SubscriptionStatusIncompleteExpired
}
