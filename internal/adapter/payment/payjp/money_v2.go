// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The money movements of API v2 (ADR-0162 §11): the account's payment transactions, the
// ledger of the PaymentFlows. Every movement is named by the object that made it, so that a
// refund and a chargeback name their payment without a search: a payment by its PaymentFlow, a
// refund by the refund, a chargeback by the dispute, and the cancellation of a chargeback by the
// dispute and cancelSuffix. Payment is the PaymentFlow, and ParentID too, since the payment's ID
// is its PaymentFlow. The signs of Gross and Fee follow the kind, as on API v1: a refund and a
// chargeback return the fee of their amount.
//
// PAY.JP sends only dispute.created of a dispute, so the reconciliation also reads the disputes
// that ended in the merchant's favour (/v2/payment_disputes, status cancel) and reports the
// reversal of a chargeback that the ledger does not report itself.

// cancelSuffix makes the ID of a chargeback's cancellation from the ID of its dispute.
const cancelSuffix = "_cancel"

// resolvePages bounds the pages of the newest transactions that a notice's object is looked for
// in: the transaction of a webhook's object is among the newest.
const resolvePages = 3

// cacheV2 keeps the objects that one call reads more than once.
type cacheV2 struct {
	flows    map[string]paymentFlow
	refunds  map[string]paymentRefund
	disputes map[string]paymentDispute
}

func newCacheV2() *cacheV2 {
	return &cacheV2{flows: map[string]paymentFlow{}, refunds: map[string]paymentRefund{}, disputes: map[string]paymentDispute{}}
}

func (p *Provider) cachedFlow(ctx context.Context, c *cacheV2, id string) (paymentFlow, error) {
	if f, ok := c.flows[id]; ok {
		return f, nil
	}
	f, err := p.getFlow(ctx, id)
	if err != nil {
		return paymentFlow{}, err
	}
	c.flows[id] = f
	return f, nil
}

func (p *Provider) cachedRefund(ctx context.Context, c *cacheV2, id string) (paymentRefund, error) {
	if r, ok := c.refunds[id]; ok {
		return r, nil
	}
	var r paymentRefund
	if err := p.getV2(ctx, "read a refund", "/payment_refunds/"+url.PathEscape(id), nil, &r); err != nil {
		return paymentRefund{}, err
	}
	c.refunds[id] = r
	return r, nil
}

func (p *Provider) cachedDispute(ctx context.Context, c *cacheV2, id string) (paymentDispute, error) {
	if d, ok := c.disputes[id]; ok {
		return d, nil
	}
	var d paymentDispute
	if err := p.getV2(ctx, "read a dispute", "/payment_disputes/"+url.PathEscape(id), nil, &d); err != nil {
		return paymentDispute{}, err
	}
	c.disputes[id] = d
	return d, nil
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// signOf is the sign of the gross of a transaction's type; 0 for a type that is no movement.
func signOf(typ string) int64 {
	switch typ {
	case txnPayment, txnChargebackCancel:
		return 1
	case txnRefund, txnChargeback:
		return -1
	}
	return 0
}

// signedFee is the fee of a transaction as a movement has it.
func signedFee(t paymentTransaction) int64 { return signOf(t.Type) * abs(t.Fee) }

// movementOfTxn returns the movement of a transaction: none for one in another currency or of
// another type.
func (p *Provider) movementOfTxn(ctx context.Context, c *cacheV2, t paymentTransaction) ([]supporters.MoneyMovement, error) {
	sign := signOf(t.Type)
	if sign == 0 || !strings.EqualFold(t.Currency, currency) || t.ResourceID == "" {
		return nil, nil
	}
	m := newMovement(t.ResourceID, supporters.MoneyPayment, t.CreatedAt.UTC(), sign*abs(t.Amount), signedFee(t))
	if t.Type == txnPayment {
		m.Payment = supporters.PaymentRef(t.ResourceID)
		return []supporters.MoneyMovement{m}, nil
	}
	flow, err := p.flowOfTxn(ctx, c, t)
	if err != nil {
		return nil, err
	}
	if flow == "" {
		// The resource is the PaymentFlow itself: the transaction names the movement.
		flow, m.ID = t.ResourceID, t.ID
	}
	switch t.Type {
	case txnRefund:
		m.Kind = supporters.MoneyRefund
	case txnChargeback:
		m.Kind = supporters.MoneyChargeback
	case txnChargebackCancel:
		m.Kind = supporters.MoneyChargebackReversal
		if m.ID == t.ResourceID {
			m.ID += cancelSuffix
		}
	}
	f, err := p.cachedFlow(ctx, c, flow)
	if err != nil {
		return nil, err
	}
	m.ParentID, m.ParentGross, m.Payment = flow, yen(f.received()), supporters.PaymentRef(flow)
	return []supporters.MoneyMovement{m}, nil
}

// flowOfTxn returns the PaymentFlow of a refund's or a chargeback's transaction, from its refund
// or its dispute; "" when PAY.JP has no such object, for a transaction whose resource is the
// PaymentFlow itself.
func (p *Provider) flowOfTxn(ctx context.Context, c *cacheV2, t paymentTransaction) (string, error) {
	var flow string
	var err error
	if t.Type == txnRefund {
		var r paymentRefund
		r, err = p.cachedRefund(ctx, c, t.ResourceID)
		flow = r.PaymentFlowID
	} else {
		var d paymentDispute
		d, err = p.cachedDispute(ctx, c, t.ResourceID)
		flow = d.PaymentFlowID
	}
	if errors.Is(err, supporters.ErrPaymentNotFound) {
		return "", nil
	}
	if err == nil && flow == "" {
		return "", fmt.Errorf("payjp: %w: %s %s names no payment", supporters.ErrInvalidPaymentResponse, t.Type, t.ResourceID)
	}
	return flow, err
}

// listV2Page reads one page of a list of API v2.
func listV2Page[T any](ctx context.Context, p *Provider, op, path string, q url.Values) (listV2[T], error) {
	var l listV2[T]
	err := p.getV2(ctx, op, path, q, &l)
	return l, err
}

// listAllV2 reads every page of a list of API v2, newest first, within maxWindowRequests
// requests; complete is false when the bound stopped it.
func listAllV2[T any](ctx context.Context, p *Provider, op, path string, q url.Values, idOf func(T) string) (out []T, complete bool, err error) {
	after := ""
	for range maxWindowRequests {
		page := url.Values{"limit": {strconv.Itoa(supporters.PageSize)}}
		for k, v := range q {
			page[k] = v
		}
		if after != "" {
			page.Set("starting_after", after)
		}
		l, err := listV2Page[T](ctx, p, op, path, page)
		if err != nil {
			return nil, false, err
		}
		out = append(out, l.Data...)
		if !l.HasMore || len(l.Data) == 0 {
			return out, true, nil
		}
		after = idOf(l.Data[len(l.Data)-1])
	}
	return out, false, nil
}

func txnID(t paymentTransaction) string { return t.ID }

// walkTransactions reads the transactions after the transaction last, oldest first, a page at
// most, and returns the last one read and whether more follow. Without last, it reads back from
// the newest to since and starts from the oldest of them.
func (p *Provider) walkTransactions(ctx context.Context, since time.Time, last string) ([]paymentTransaction, string, bool, error) {
	if last != "" {
		q := url.Values{"limit": {strconv.Itoa(supporters.PageSize)}, "ending_before": {last}}
		l, err := listV2Page[paymentTransaction](ctx, p, "list the payment transactions", "/payment_transactions", q)
		if err != nil {
			return nil, "", false, err
		}
		slices.Reverse(l.Data)
		if len(l.Data) > 0 {
			last = l.Data[len(l.Data)-1].ID
		}
		return l.Data, last, l.HasMore, nil
	}
	var back []paymentTransaction
	after, complete := "", false
	for range maxWindowRequests {
		q := url.Values{"limit": {strconv.Itoa(supporters.PageSize)}}
		if after != "" {
			q.Set("starting_after", after)
		}
		l, err := listV2Page[paymentTransaction](ctx, p, "list the payment transactions", "/payment_transactions", q)
		if err != nil {
			return nil, "", false, err
		}
		done := !l.HasMore || len(l.Data) == 0
		for _, t := range l.Data {
			if t.CreatedAt.Before(since) {
				done = true
				break
			}
			back = append(back, t)
		}
		if done {
			complete = true
			break
		}
		after = l.Data[len(l.Data)-1].ID
	}
	if !complete {
		p.log.WarnContext(ctx, "the payment transactions since the reconciliation's start are too many to read back; the older ones are left out",
			slog.Int("transactions", len(back)))
	}
	slices.Reverse(back)
	if len(back) > supporters.PageSize {
		page := back[:supporters.PageSize]
		return page, page[len(page)-1].ID, true, nil
	}
	if len(back) == 0 {
		return nil, "", false, nil
	}
	return back, back[len(back)-1].ID, false, nil
}

// disputeOutcomes returns the reversals of the chargebacks whose disputes ended in the merchant's
// favour after after and by until, which the ledger does not report as a cancellation: a
// dispute without a chargeback (an inquiry that PAY.JP closed) moved no money.
func (p *Provider) disputeOutcomes(ctx context.Context, after, until time.Time) ([]supporters.MoneyMovement, error) {
	canceled, _, err := listAllV2(ctx, p, "list the disputes", "/payment_disputes", url.Values{"status": {disputeCanceled}},
		func(d paymentDispute) string { return d.ID })
	if err != nil {
		return nil, err
	}
	canceled = slices.DeleteFunc(canceled, func(d paymentDispute) bool { return !d.UpdatedAt.After(after) || d.UpdatedAt.After(until) })
	if len(canceled) == 0 {
		return nil, nil
	}
	ledger := map[string]map[string]paymentTransaction{}
	for _, typ := range []string{txnChargeback, txnChargebackCancel} {
		ts, _, err := listAllV2(ctx, p, "list the payment transactions", "/payment_transactions", url.Values{"type": {typ}}, txnID)
		if err != nil {
			return nil, err
		}
		ledger[typ] = map[string]paymentTransaction{}
		for _, t := range ts {
			ledger[typ][t.ResourceID] = t
		}
	}
	c := newCacheV2()
	var out []supporters.MoneyMovement
	slices.SortFunc(canceled, func(a, b paymentDispute) int { return a.UpdatedAt.Compare(b.UpdatedAt) })
	for _, d := range canceled {
		cb, charged := ledger[txnChargeback][d.ID]
		if _, reported := ledger[txnChargebackCancel][d.ID]; reported || !charged || d.PaymentFlowID == "" {
			continue
		}
		f, err := p.cachedFlow(ctx, c, d.PaymentFlowID)
		if err != nil {
			return nil, err
		}
		m := newMovement(d.ID+cancelSuffix, supporters.MoneyChargebackReversal, d.UpdatedAt.UTC(), abs(cb.Amount), abs(cb.Fee))
		m.ParentID, m.ParentGross, m.Payment = d.PaymentFlowID, yen(f.received()), supporters.PaymentRef(d.PaymentFlowID)
		out = append(out, m)
	}
	return out, nil
}

// moneyOfV2 returns the movements of an object of API v2: a transaction by its ID, or the
// transactions among the newest that an object made (a PaymentFlow, a refund, a dispute, or the
// ID of a movement). It returns none for an object that has none yet; the reconciliation reads
// it later.
func (p *Provider) moneyOfV2(ctx context.Context, id string) ([]supporters.MoneyMovement, error) {
	c := newCacheV2()
	var t paymentTransaction
	err := p.getV2(ctx, "read a payment transaction", "/payment_transactions/"+url.PathEscape(id), nil, &t)
	if err == nil {
		return p.movementOfTxn(ctx, c, t)
	}
	if !errors.Is(err, supporters.ErrPaymentNotFound) {
		return nil, err
	}
	resource := strings.TrimSuffix(id, cancelSuffix)
	var out []supporters.MoneyMovement
	after := ""
	for range resolvePages {
		q := url.Values{"limit": {strconv.Itoa(supporters.PageSize)}}
		if after != "" {
			q.Set("starting_after", after)
		}
		l, err := listV2Page[paymentTransaction](ctx, p, "list the payment transactions", "/payment_transactions", q)
		if err != nil {
			return nil, err
		}
		for _, t := range l.Data {
			if t.ResourceID != resource {
				continue
			}
			ms, err := p.movementOfTxn(ctx, c, t)
			if err != nil {
				return nil, err
			}
			out = append(out, ms...)
		}
		if len(out) > 0 || !l.HasMore || len(l.Data) == 0 {
			break
		}
		after = l.Data[len(l.Data)-1].ID
	}
	slices.Reverse(out)
	return out, nil
}

// feesOfTermV2 is the sum of the fees of a term's transactions of API v2, as their movements
// have them. A statement of the account covers the fees of both versions.
func (p *Provider) feesOfTermV2(ctx context.Context, term string) (int64, error) {
	ts, _, err := listAllV2(ctx, p, "list the payment transactions of a term", "/payment_transactions", url.Values{"term_id": {term}}, txnID)
	if err != nil {
		return 0, err
	}
	var sum int64
	for _, t := range ts {
		if strings.EqualFold(t.Currency, currency) {
			sum += signedFee(t)
		}
	}
	return sum, nil
}

// moneyCursor is the position of ListMoney: the time of the next event of API v1, the last
// transaction of API v2 read, and the time through which the disputes were read. Its text is
// "<v1>~<v2>~<disputes>"; a cursor of v1 alone is one of v1 with v2 to read from the start.
type moneyCursor struct {
	v1       string
	v2       string
	disputes string
}

func parseMoneyCursor(s string) moneyCursor {
	parts := strings.SplitN(s, "~", 3)
	c := moneyCursor{v1: parts[0]}
	if len(parts) > 1 {
		c.v2 = parts[1]
	}
	if len(parts) > 2 {
		c.disputes = parts[2]
	}
	return c
}

func (c moneyCursor) String() string { return c.v1 + "~" + c.v2 + "~" + c.disputes }

// disputesAfter is the time after which the disputes are read: the cursor's, or since.
func (c moneyCursor) disputesAfter(since time.Time) (time.Time, error) {
	if c.disputes == "" {
		if since.IsZero() {
			return time.Unix(0, 0).UTC(), nil
		}
		return since.Add(-time.Second), nil
	}
	n, err := strconv.ParseInt(c.disputes, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("payjp: %w: cursor %q", supporters.ErrPaymentInvalidRequest, c.String())
	}
	return time.Unix(n, 0).UTC(), nil
}
