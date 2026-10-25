// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The money movements of PAY.JP (ADR-0099 §5). A payment is a captured charge of a subscription
// or of the difference of a change, by the charge's ID; a refund is an event charge.refunded and
// a chargeback an event dispute.created, by the event's ID; and the fees of a term that differ
// from the ones the adapter computed are a fee by the ID of the term's statement. PAY.JP returns
// the fee of a refunded or charged back amount, so their fees are negative.

// The money's event types.
const (
	evChargeSucceeded  = "charge.succeeded"
	evChargeRefunded   = "charge.refunded"
	evDisputeCreated   = "dispute.created"
	evStatementCreated = "statement.created"
)

// The subjects of a statement's items that are fees and their offsets.
var feeSubjects = map[string]bool{"fee": true, "refund_fee_offset": true, "chargeback_fee_offset": true}

// feeOf is the fee of amount at a rate in percent with two decimals, such as "3.60", rounded
// down (ADR-0099 §5).
func feeOf(amount int64, rate string) (int64, error) {
	whole, frac, _ := strings.Cut(strings.TrimSpace(rate), ".")
	if len(frac) > 2 {
		frac = frac[:2]
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err1 := strconv.ParseInt(whole, 10, 64)
	f, err2 := strconv.ParseInt(frac, 10, 64)
	if err1 != nil || err2 != nil || w < 0 || f < 0 {
		return 0, fmt.Errorf("payjp: %w: fee rate %q", supporters.ErrInvalidPaymentResponse, rate)
	}
	neg := amount < 0
	if neg {
		amount = -amount
	}
	fee := amount * (w*100 + f) / 10000
	if neg {
		fee = -fee
	}
	return fee, nil
}

// yen formats an amount in yen as the decimal of a MoneyMovement.
func yen(n int64) string { return strconv.FormatInt(n, 10) }

// newMovement returns a final movement of gross and fee in yen at at.
func newMovement(id string, kind supporters.MoneyKind, at time.Time, gross, fee int64) supporters.MoneyMovement {
	return supporters.MoneyMovement{
		ID: id, Kind: kind, OccurredAt: at, OccurredOn: supporters.DateOf(at.In(tokyo)),
		Gross: yen(gross), Fee: yen(fee), Net: yen(gross - fee), Currency: "JPY", Final: true,
	}
}

// subscriptionOf returns the subscription that a charge belongs to; "" for a charge that is not
// of this server's subscriptions.
func subscriptionOf(c charge) string {
	if c.Subscription != "" {
		return c.Subscription
	}
	return c.Metadata[metaSubscription]
}

// intervals caches the intervals of the subscriptions within one call.
type intervals map[string]string

// intervalOf returns the interval of a subscription's plan: from the subscription, or from its
// latest event once PAY.JP deleted it; a month when neither tells.
func (p *Provider) intervalOf(ctx context.Context, cache intervals, sub string) (string, error) {
	if iv, ok := cache[sub]; ok {
		return iv, nil
	}
	s, err := p.getSubscription(ctx, supporters.SubscriptionRef(sub))
	switch {
	case err == nil:
	case errors.Is(err, supporters.ErrPaymentNotFound):
		var page list[event]
		q := url.Values{"resource_id": {sub}, "object": {"subscription"}, "limit": {"1"}}
		if err := p.get(ctx, "list the events of a subscription", "/events", q, &page); err != nil {
			return "", err
		}
		if len(page.Data) > 0 {
			_ = json.Unmarshal(page.Data[0].Data, &s)
		}
	default:
		return "", err
	}
	iv := s.Plan.Interval
	if iv == "" {
		iv = string(supporters.IntervalMonth)
	}
	cache[sub] = iv
	return iv, nil
}

// paymentOf returns the movement of a charge: none for a charge that is not captured or not of
// a subscription.
func (p *Provider) paymentOf(ctx context.Context, cache intervals, c charge) ([]supporters.MoneyMovement, error) {
	sub := subscriptionOf(c)
	if !c.Captured || sub == "" {
		return nil, nil
	}
	fee, err := feeOf(c.Amount, c.FeeRate)
	if err != nil {
		return nil, err
	}
	at := unixTime(c.CapturedAt)
	if at.IsZero() {
		at = unixTime(c.Created)
	}
	created := unixTime(c.Created)
	start := supporters.DateOf(created.In(tokyo))
	var end time.Time
	if pe, err := strconv.ParseInt(c.Metadata[metaPeriodEnd], 10, 64); err == nil && pe > 0 {
		end = time.Unix(pe, 0)
	} else {
		iv, err := p.intervalOf(ctx, cache, sub)
		if err != nil {
			return nil, err
		}
		end = addInterval(created.In(tokyo), iv)
	}
	last := supporters.DateOf(end.In(tokyo).AddDate(0, 0, -1))
	if dateBefore(last, start) {
		last = start
	}
	m := newMovement(c.ID, supporters.MoneyPayment, at, c.Amount, fee)
	m.Subscription, m.Covers = supporters.SubscriptionRef(sub), supporters.DateRange{Start: start, End: last}
	return []supporters.MoneyMovement{m}, nil
}

func dateBefore(a, b supporters.Date) bool {
	return time.Date(a.Year, a.Month, a.Day, 0, 0, 0, 0, time.UTC).Before(time.Date(b.Year, b.Month, b.Day, 0, 0, 0, 0, time.UTC))
}

// refundedBefore returns the amount refunded of a charge as of the charge.refunded event before
// ev; 0 without one.
func (p *Provider) refundedBefore(ctx context.Context, chargeID string, ev event) (int64, error) {
	evs, err := p.allEvents(ctx, url.Values{"resource_id": {chargeID}, "type": {evChargeRefunded}, "until": {strconv.FormatInt(ev.Created, 10)}})
	if err != nil {
		return 0, err
	}
	var prev int64
	for _, e := range evs {
		if e.ID == ev.ID || e.Created > ev.Created || (e.Created == ev.Created && e.ID > ev.ID) {
			continue
		}
		var c charge
		if json.Unmarshal(e.Data, &c) == nil && c.AmountRefunded > prev {
			prev = c.AmountRefunded
		}
	}
	return prev, nil
}

// refundOf returns the movement of an event charge.refunded: the amount it added to the charge's
// refunds.
func (p *Provider) refundOf(ctx context.Context, ev event) ([]supporters.MoneyMovement, error) {
	var c charge
	if err := json.Unmarshal(ev.Data, &c); err != nil {
		return nil, fmt.Errorf("payjp: %w: %w", supporters.ErrInvalidPaymentResponse, err)
	}
	sub := subscriptionOf(c)
	if sub == "" {
		return nil, nil
	}
	prev, err := p.refundedBefore(ctx, c.ID, ev)
	if err != nil {
		return nil, err
	}
	amount := c.AmountRefunded - prev
	if amount <= 0 {
		return nil, nil
	}
	fee, err := feeOf(-amount, c.FeeRate)
	if err != nil {
		return nil, err
	}
	m := newMovement(ev.ID, supporters.MoneyRefund, unixTime(ev.Created), -amount, fee)
	m.ParentID, m.ParentGross, m.Subscription = c.ID, yen(c.Amount), supporters.SubscriptionRef(sub)
	return []supporters.MoneyMovement{m}, nil
}

// disputeOf returns the movement of an event dispute.created: the charge it names (its charge,
// or the charge itself) charged back by its amount.
func (p *Provider) disputeOf(ctx context.Context, ev event) ([]supporters.MoneyMovement, error) {
	var obj eventObject
	if err := json.Unmarshal(ev.Data, &obj); err != nil {
		return nil, fmt.Errorf("payjp: %w: %w", supporters.ErrInvalidPaymentResponse, err)
	}
	id := obj.Charge
	if id == "" && obj.Object == "charge" {
		id = obj.ID
	}
	if id == "" {
		return nil, fmt.Errorf("payjp: %w: dispute %s names no charge", supporters.ErrInvalidPaymentResponse, ev.ID)
	}
	var c charge
	if err := p.get(ctx, "read a charge", "/charges/"+url.PathEscape(id), nil, &c); err != nil {
		return nil, err
	}
	sub := subscriptionOf(c)
	if sub == "" {
		return nil, nil
	}
	amount := obj.Amount
	if amount <= 0 || amount > c.Amount {
		amount = c.Amount
	}
	fee, err := feeOf(-amount, c.FeeRate)
	if err != nil {
		return nil, err
	}
	m := newMovement(ev.ID, supporters.MoneyChargeback, unixTime(ev.Created), -amount, fee)
	m.ParentID, m.ParentGross, m.Subscription = c.ID, yen(c.Amount), supporters.SubscriptionRef(sub)
	return []supporters.MoneyMovement{m}, nil
}

// adjustmentOf returns the fee of a sales statement that the adapter's fees of its term miss:
// the statement's fees and offsets less the fees of the term's charges, refunds, and chargebacks
// as the adapter computes them (ADR-0099 §5).
func (p *Provider) adjustmentOf(ctx context.Context, st statement) ([]supporters.MoneyMovement, error) {
	if st.Type != "sales" || st.Term == nil || st.Term.ID == "" || st.Term.EndAt == 0 {
		return nil, nil
	}
	var reported int64
	for _, it := range st.Items {
		if feeSubjects[it.Subject] {
			reported -= it.Amount
		}
	}
	var computed int64
	for offset := 0; ; offset += supporters.PageSize {
		var page list[charge]
		q := url.Values{"term": {st.Term.ID}, "limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)}}
		if err := p.get(ctx, "list the charges of a term", "/charges", q, &page); err != nil {
			return nil, err
		}
		for _, c := range page.Data {
			if !c.Captured {
				continue
			}
			fee, err := feeOf(c.Amount, c.FeeRate)
			if err != nil {
				return nil, err
			}
			computed += fee
		}
		if !page.HasMore || len(page.Data) == 0 {
			break
		}
	}
	span := url.Values{"since": {strconv.FormatInt(st.Term.StartAt, 10)}, "until": {strconv.FormatInt(st.Term.EndAt-1, 10)}}
	for _, typ := range []string{evChargeRefunded, evDisputeCreated} {
		q := url.Values{"type": {typ}}
		for k, v := range span {
			q[k] = v
		}
		evs, err := p.allEvents(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, ev := range evs {
			fee, err := p.returnedFee(ctx, ev)
			if err != nil {
				return nil, err
			}
			computed -= fee
		}
	}
	diff := reported - computed
	if diff == 0 {
		return nil, nil
	}
	return []supporters.MoneyMovement{newMovement(st.ID, supporters.MoneyFee, unixTime(st.Created), -diff, 0)}, nil
}

// returnedFee is the fee that a refund or a chargeback of any charge returns, as the adapter
// computes it.
func (p *Provider) returnedFee(ctx context.Context, ev event) (int64, error) {
	var c charge
	var amount int64
	switch ev.Type {
	case evChargeRefunded:
		if err := json.Unmarshal(ev.Data, &c); err != nil {
			return 0, fmt.Errorf("payjp: %w: %w", supporters.ErrInvalidPaymentResponse, err)
		}
		prev, err := p.refundedBefore(ctx, c.ID, ev)
		if err != nil {
			return 0, err
		}
		amount = c.AmountRefunded - prev
	case evDisputeCreated:
		var obj eventObject
		if err := json.Unmarshal(ev.Data, &obj); err != nil {
			return 0, fmt.Errorf("payjp: %w: %w", supporters.ErrInvalidPaymentResponse, err)
		}
		id := obj.Charge
		if id == "" && obj.Object == "charge" {
			id = obj.ID
		}
		if err := p.get(ctx, "read a charge", "/charges/"+url.PathEscape(id), nil, &c); err != nil {
			return 0, err
		}
		amount = obj.Amount
		if amount <= 0 || amount > c.Amount {
			amount = c.Amount
		}
	}
	if amount <= 0 {
		return 0, nil
	}
	return feeOf(amount, c.FeeRate)
}

// allEvents reads every event of a query, newest first.
func (p *Provider) allEvents(ctx context.Context, q url.Values) ([]event, error) {
	var out []event
	for offset := 0; ; offset += supporters.PageSize {
		page := url.Values{"limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)}}
		for k, v := range q {
			page[k] = v
		}
		var l list[event]
		if err := p.get(ctx, "list the events", "/events", page, &l); err != nil {
			return nil, err
		}
		out = append(out, l.Data...)
		if !l.HasMore || len(l.Data) == 0 {
			return out, nil
		}
	}
}

// moneyOfEvent returns the movements of an event.
func (p *Provider) moneyOfEvent(ctx context.Context, cache intervals, ev event) ([]supporters.MoneyMovement, error) {
	switch ev.Type {
	case evChargeSucceeded:
		var c charge
		if err := json.Unmarshal(ev.Data, &c); err != nil {
			return nil, fmt.Errorf("payjp: %w: %w", supporters.ErrInvalidPaymentResponse, err)
		}
		return p.paymentOf(ctx, cache, c)
	case evChargeRefunded:
		return p.refundOf(ctx, ev)
	case evDisputeCreated:
		return p.disputeOf(ctx, ev)
	case evStatementCreated:
		var st statement
		if err := json.Unmarshal(ev.Data, &st); err != nil {
			return nil, fmt.Errorf("payjp: %w: %w", supporters.ErrInvalidPaymentResponse, err)
		}
		return p.adjustmentOf(ctx, st)
	}
	return nil, nil
}

// moneyOf returns the movements of an ID: a charge, an event, or a statement; none for an ID
// that PAY.JP does not have.
func (p *Provider) moneyOf(ctx context.Context, id string) ([]supporters.MoneyMovement, error) {
	if id == "" {
		return nil, nil
	}
	cache := intervals{}
	var c charge
	err := p.get(ctx, "read a charge", "/charges/"+url.PathEscape(id), nil, &c)
	if err == nil {
		return p.paymentOf(ctx, cache, c)
	}
	if !errors.Is(err, supporters.ErrPaymentNotFound) {
		return nil, err
	}
	var ev event
	err = p.get(ctx, "read an event", "/events/"+url.PathEscape(id), nil, &ev)
	if err == nil {
		return p.moneyOfEvent(ctx, cache, ev)
	}
	if !errors.Is(err, supporters.ErrPaymentNotFound) {
		return nil, err
	}
	var st statement
	err = p.get(ctx, "read a statement", "/statements/"+url.PathEscape(id), nil, &st)
	if err == nil {
		return p.adjustmentOf(ctx, st)
	}
	if errors.Is(err, supporters.ErrPaymentNotFound) {
		return nil, nil
	}
	return nil, err
}

// GetMoney implements supporters.PaymentProvider.
func (p *Provider) GetMoney(ctx context.Context, id string) (supporters.MoneyMovement, error) {
	ms, err := p.moneyOf(ctx, id)
	if err != nil {
		return supporters.MoneyMovement{}, err
	}
	for _, m := range ms {
		if m.ID == id {
			return m, nil
		}
	}
	return supporters.MoneyMovement{}, fmt.Errorf("payjp: %w: money %q", supporters.ErrPaymentNotFound, id)
}

// ListMoney implements supporters.PaymentProvider: the movements of the money's events, oldest
// first, from the cursor's time (ADR-0099 §5).
func (p *Provider) ListMoney(ctx context.Context, since time.Time, cursor string) (supporters.MoneyPage, error) {
	from, err := timeCursor(cursor, since)
	if err != nil {
		return supporters.MoneyPage{}, err
	}
	w, err := p.walkEvents(ctx, from)
	if err != nil {
		return supporters.MoneyPage{}, err
	}
	out := supporters.MoneyPage{Next: strconv.FormatInt(w.Next.Unix(), 10), More: !w.Next.After(w.Until)}
	cache := intervals{}
	for _, ev := range w.Events {
		ms, err := p.moneyOfEvent(ctx, cache, ev)
		if err != nil {
			return supporters.MoneyPage{}, err
		}
		out.Movements = append(out.Movements, ms...)
	}
	return out, nil
}
