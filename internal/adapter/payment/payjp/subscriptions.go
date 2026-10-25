// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// changeLookback is how far back a command looks for the charge of the difference that an
// earlier try of it made (ADR-0099 §2).
const changeLookback = 48 * time.Hour

// snapshot maps a subscription of PAY.JP to the port's (ADR-0099 §2): a trial is active (the
// adapter makes one only to keep the period of a change), canceled is a cancellation at the
// period's end, and paused is a failed payment since paused_at, since the adapter never pauses.
func (p *Provider) snapshot(ctx context.Context, s subscription) supporters.SubscriptionSnapshot {
	snap := supporters.SubscriptionSnapshot{
		Ref: supporters.SubscriptionRef(s.ID), Customer: supporters.CustomerRef(s.Customer), LocalID: s.Metadata[metaSubscriptionID],
		Price: supporters.PriceRef(s.Plan.ID), PeriodStart: unixTime(s.CurrentPeriodStart), PeriodEnd: unixTime(s.CurrentPeriodEnd),
	}
	if s.NextCyclePlan != nil {
		snap.ScheduledPrice = supporters.PriceRef(s.NextCyclePlan.ID)
	}
	switch s.Status {
	case subActive, subTrial:
		snap.Status = supporters.StatusActive
	case subCanceled:
		snap.Status, snap.CancelAtPeriodEnd = supporters.StatusActive, true
	case subPaused:
		snap.Status, snap.LatestPaymentFailed = supporters.StatusPastDue, unixTime(s.PausedAt)
	default:
		p.log.WarnContext(ctx, "unknown status of a PAY.JP subscription", slog.String("subscription", s.ID), slog.String("status", s.Status))
		snap.Status = supporters.StatusUnpaid
	}
	return snap
}

// getSubscription reads a subscription; ErrPaymentNotFound once it ended or was deleted.
func (p *Provider) getSubscription(ctx context.Context, ref supporters.SubscriptionRef) (subscription, error) {
	var s subscription
	err := p.get(ctx, "read a subscription", "/subscriptions/"+url.PathEscape(string(ref)), nil, &s)
	return s, err
}

// GetSubscription implements supporters.PaymentProvider. PAY.JP deletes a subscription when its
// cancellation takes effect, so one that it no longer has is canceled (ADR-0099 §2): the adapter
// only re-reads the subscriptions that it reported.
func (p *Provider) GetSubscription(ctx context.Context, ref supporters.SubscriptionRef) (supporters.SubscriptionSnapshot, error) {
	s, err := p.getSubscription(ctx, ref)
	if errors.Is(err, supporters.ErrPaymentNotFound) {
		return supporters.SubscriptionSnapshot{Ref: ref, Status: supporters.StatusCanceled}, nil
	}
	if err != nil {
		return supporters.SubscriptionSnapshot{}, err
	}
	return p.snapshot(ctx, s), nil
}

// ListSubscriptions implements supporters.PaymentProvider. PAY.JP lists the newest first by an
// offset, so the cursor fixes the time of the first page (until) for the offsets not to move
// when subscriptions are made meanwhile: "<until>:<offset>".
func (p *Provider) ListSubscriptions(ctx context.Context, cursor string) (supporters.SubscriptionPage, error) {
	until, offset, err := parseOffsetCursor(cursor, p.o.Now())
	if err != nil {
		return supporters.SubscriptionPage{}, err
	}
	q := url.Values{
		"limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)}, "until": {strconv.FormatInt(until, 10)},
	}
	var page list[subscription]
	if err := p.get(ctx, "list the subscriptions", "/subscriptions", q, &page); err != nil {
		return supporters.SubscriptionPage{}, err
	}
	out := supporters.SubscriptionPage{}
	for _, s := range page.Data {
		out.Subscriptions = append(out.Subscriptions, p.snapshot(ctx, s))
	}
	if page.HasMore && len(page.Data) > 0 {
		out.Next = fmt.Sprintf("%d:%d", until, offset+len(page.Data))
	}
	return out, nil
}

// parseOffsetCursor reads "<until>:<offset>"; "" is now and 0.
func parseOffsetCursor(cursor string, now time.Time) (until int64, offset int, err error) {
	if cursor == "" {
		return now.Unix(), 0, nil
	}
	u, o, ok := strings.Cut(cursor, ":")
	until, err1 := strconv.ParseInt(u, 10, 64)
	offset, err2 := strconv.Atoi(o)
	if !ok || err1 != nil || err2 != nil || offset < 0 {
		return 0, 0, fmt.Errorf("payjp: %w: cursor %q", supporters.ErrPaymentInvalidRequest, cursor)
	}
	return until, offset, nil
}

// ChangePrice implements supporters.PaymentProvider (ADR-0099 §2). Now: it charges the prorated
// difference of the rest of the period (U1) once, by the key in the charge's metadata, and
// switches the plan with trial_end at the period's end, so that PAY.JP charges the new plan's
// full amount then rather than now. At the period's end: next_cycle_plan.
func (p *Provider) ChangePrice(ctx context.Context, ref supporters.SubscriptionRef, to supporters.PriceRef, when supporters.ChangeTiming, key string) error {
	s, err := p.getSubscription(ctx, ref)
	if err != nil {
		return err
	}
	path := "/subscriptions/" + url.PathEscape(s.ID)
	switch when {
	case supporters.ChangeAtPeriodEnd:
		if s.Plan.ID == string(to) {
			return p.CancelScheduledChange(ctx, ref, key)
		}
		if s.NextCyclePlan != nil && s.NextCyclePlan.ID == string(to) {
			return nil
		}
		return p.post(ctx, "schedule a change of plan", path, url.Values{"next_cycle_plan": {string(to)}}, nil)
	case supporters.ChangeNowProrated:
	default:
		return fmt.Errorf("payjp: %w: change timing %q", supporters.ErrPaymentInvalidRequest, when)
	}
	if s.Plan.ID == string(to) {
		if s.NextCyclePlan != nil {
			return p.CancelScheduledChange(ctx, ref, key)
		}
		return nil
	}
	next, err := p.getPlan(ctx, to)
	if err != nil {
		return err
	}
	now := p.o.Now()
	start, end := unixTime(s.CurrentPeriodStart), unixTime(s.CurrentPeriodEnd)
	if diff := proratedDifference(s.Plan, next, start, end, now); diff >= MinimumAmount {
		if err := p.chargeDifference(ctx, s, diff, key, end); err != nil {
			return err
		}
	}
	form := url.Values{"plan": {string(to)}}
	if end.After(now.Add(time.Minute)) {
		form.Set("trial_end", strconv.FormatInt(end.Unix(), 10))
	}
	if s.NextCyclePlan != nil {
		form.Set("next_cycle_plan", "")
	}
	return p.post(ctx, "change the plan", path, form, nil)
}

// proratedDifference is the difference of the prices per second of two plans over the rest of
// the period from now, rounded down, in yen (ADR-0099 §2, U1). The price per second of a plan is
// its amount over one period of its interval from the period's start. It is not positive when
// the new plan costs less per second, such as a year at a discount after a month.
func proratedDifference(from, to plan, start, end, now time.Time) int64 {
	if !end.After(now) || start.IsZero() {
		return 0
	}
	perSecond := func(pl plan) float64 {
		d := addInterval(start, pl.Interval).Sub(start).Seconds()
		if d <= 0 {
			return 0
		}
		return float64(pl.Amount) / d
	}
	return int64(math.Floor((perSecond(to) - perSecond(from)) * end.Sub(now).Seconds()))
}

// addInterval returns t one period of interval later.
func addInterval(t time.Time, interval string) time.Time {
	if interval == string(supporters.IntervalYear) {
		return t.AddDate(1, 0, 0)
	}
	return t.AddDate(0, 1, 0)
}

// chargeDifference charges the customer of s the difference of a change, unless a charge of key
// is there already.
func (p *Provider) chargeDifference(ctx context.Context, s subscription, amount int64, key string, end time.Time) error {
	if key != "" {
		since := p.o.Now().Add(-changeLookback).Unix()
		for offset := 0; ; offset += supporters.PageSize {
			var page list[charge]
			q := url.Values{
				"customer": {s.Customer}, "since": {strconv.FormatInt(since, 10)},
				"limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)},
			}
			if err := p.get(ctx, "list the charges of a customer", "/charges", q, &page); err != nil {
				return err
			}
			for _, c := range page.Data {
				if c.Metadata[metaChange] == key {
					return nil
				}
			}
			if !page.HasMore || len(page.Data) == 0 {
				break
			}
		}
	}
	form := url.Values{
		"customer": {s.Customer}, "amount": {strconv.FormatInt(amount, 10)}, "currency": {currency},
		"metadata[" + metaSubscriptionID + "]": {s.Metadata[metaSubscriptionID]},
		"metadata[" + metaSubscription + "]":   {s.ID},
		"metadata[" + metaChange + "]":         {key},
		"metadata[" + metaPeriodEnd + "]":      {strconv.FormatInt(end.Unix(), 10)},
	}
	return p.post(ctx, "charge the difference of a change", "/charges", form, nil)
}

// CancelScheduledChange implements supporters.PaymentProvider.
func (p *Provider) CancelScheduledChange(ctx context.Context, ref supporters.SubscriptionRef, _ string) error {
	s, err := p.getSubscription(ctx, ref)
	if err != nil || s.NextCyclePlan == nil {
		return err
	}
	return p.post(ctx, "withdraw a change of plan", "/subscriptions/"+url.PathEscape(s.ID), url.Values{"next_cycle_plan": {""}}, nil)
}

// CancelAtPeriodEnd implements supporters.PaymentProvider: PAY.JP's cancel ends the subscription
// at the period's end.
func (p *Provider) CancelAtPeriodEnd(ctx context.Context, ref supporters.SubscriptionRef, _ string) error {
	s, err := p.getSubscription(ctx, ref)
	if err != nil || s.Status == subCanceled {
		return err
	}
	return p.post(ctx, "cancel a subscription", "/subscriptions/"+url.PathEscape(s.ID)+"/cancel", url.Values{}, nil)
}

// ResumeCancellation implements supporters.PaymentProvider: within the period, PAY.JP's resume
// withdraws the cancellation without a charge.
func (p *Provider) ResumeCancellation(ctx context.Context, ref supporters.SubscriptionRef, _ string) error {
	s, err := p.getSubscription(ctx, ref)
	if err != nil || s.Status != subCanceled || !unixTime(s.CurrentPeriodEnd).After(p.o.Now()) {
		return err
	}
	return p.post(ctx, "resume a subscription", "/subscriptions/"+url.PathEscape(s.ID)+"/resume", url.Values{}, nil)
}

// CancelNow implements supporters.PaymentProvider: it deletes the subscription without a refund.
func (p *Provider) CancelNow(ctx context.Context, ref supporters.SubscriptionRef, _ string) error {
	s, err := p.getSubscription(ctx, ref)
	if errors.Is(err, supporters.ErrPaymentNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	err = p.del(ctx, "delete a subscription", "/subscriptions/"+url.PathEscape(s.ID), url.Values{"prorate": {"false"}})
	if errors.Is(err, supporters.ErrPaymentNotFound) {
		return nil
	}
	return err
}
