// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	stripe "github.com/stripe/stripe-go/v87"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The cursors of the lists that Stripe returns newest first, read oldest first: "seek:<id>"
// while the first listing walks back to the oldest object (its pages hand out nothing), then
// "after:<id>" for the objects newer than the last one handed out.
const (
	cursorSeek  = "seek:"
	cursorAfter = "after:"
)

// ListMoney implements supporters.PaymentProvider with the balance transactions of the account
// (ADR-0074 §2), oldest first: the first listing walks back from the newest page to the oldest
// one booked at or after since, each page one request; then the cursor names the newest
// transaction handed out, which stays valid as Stripe books more. Balance transactions do not
// change once booked, so they are all final. It reads the account's time zone again first when
// the last read is older than an hour.
func (p *Provider) ListMoney(ctx context.Context, since time.Time, cursor string) (supporters.MoneyPage, error) {
	p.refreshLocation(ctx)
	params := &stripe.BalanceTransactionListParams{
		ListParams: stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true},
		Expand:     expand("data.", []string{"source"}),
	}
	after, isAfter := strings.CutPrefix(cursor, cursorAfter)
	seek, isSeek := strings.CutPrefix(cursor, cursorSeek)
	switch {
	case isAfter && after != "":
		params.EndingBefore = stripe.String(after)
	case isSeek && seek != "":
		params.StartingAfter = stripe.String(seek)
	case cursor != "":
		return supporters.MoneyPage{}, fmt.Errorf("stripe: %w: unknown cursor %q", supporters.ErrPaymentInvalidRequest, cursor)
	}
	if !isAfter && !since.IsZero() {
		params.CreatedRange = &stripe.RangeQueryParams{GreaterThanOrEqual: since.Unix()}
	}
	var list *stripe.V1List[*stripe.BalanceTransaction]
	err := p.do(ctx, "list the balance transactions", func() error {
		list = p.sc.V1BalanceTransactions.List(ctx, params)
		return list.Err()
	})
	if err != nil {
		return supporters.MoneyPage{}, err
	}
	data := list.Data()
	more := list.Meta().HasMore
	if !isAfter {
		if more && len(data) > 0 {
			// Not at the oldest page yet.
			return supporters.MoneyPage{Next: cursorSeek + data[len(data)-1].ID, More: true}, nil
		}
		if len(data) == 0 && isSeek {
			p.log.WarnContext(ctx, "the walk back through the balance transactions ended on an empty page", slog.String("after", seek))
			return supporters.MoneyPage{Next: cursorAfter + seek}, nil
		}
		// The oldest page arrives newest first; the pages after it, oldest first.
		data = slices.Clone(data)
		slices.Reverse(data)
		more = isSeek
	}
	page := supporters.MoneyPage{Next: cursor, More: more}
	for _, bt := range data {
		m, ok, err := p.movement(ctx, bt)
		if err != nil {
			return supporters.MoneyPage{}, err
		}
		if ok {
			page.Movements = append(page.Movements, m)
		}
	}
	if len(data) > 0 {
		page.Next = cursorAfter + data[len(data)-1].ID
	}
	return page, nil
}

// GetMoney implements supporters.PaymentProvider.
func (p *Provider) GetMoney(ctx context.Context, id string) (supporters.MoneyMovement, error) {
	var bt *stripe.BalanceTransaction
	err := p.do(ctx, "read the balance transaction "+id, func() (err error) {
		bt, err = p.sc.V1BalanceTransactions.Retrieve(ctx, id, &stripe.BalanceTransactionRetrieveParams{Expand: expand("", []string{"source"})})
		return err
	})
	if err != nil {
		return supporters.MoneyMovement{}, err
	}
	m, ok, err := p.movement(ctx, bt)
	if err != nil {
		return supporters.MoneyMovement{}, err
	}
	if !ok {
		return supporters.MoneyMovement{}, fmt.Errorf("stripe: %w: %s is not a movement of the supporters", supporters.ErrPaymentNotFound, id)
	}
	return m, nil
}

// moneyKind maps a balance transaction to the kind of its movement (ADR-0074 §2); false for the
// ones that are not income (payouts, transfers, and the like).
func moneyKind(bt *stripe.BalanceTransaction) (supporters.MoneyKind, bool) {
	switch bt.ReportingCategory {
	case "charge":
		return supporters.MoneyPayment, true
	case "refund", "refund_failure":
		return supporters.MoneyRefund, true
	case "dispute":
		return supporters.MoneyChargeback, bt.Amount < 0
	case "dispute_reversal":
		return supporters.MoneyChargebackReversal, bt.Amount > 0
	case "fee":
		return supporters.MoneyFee, bt.Type == stripe.BalanceTransactionTypeStripeFee
	}
	return "", false
}

// movement maps a balance transaction read with its source expanded. A payment belongs to the
// subscription of its invoice and covers the invoice's period; a refund, a chargeback, or a
// reversal belongs to the payment of its charge.
func (p *Provider) movement(ctx context.Context, bt *stripe.BalanceTransaction) (supporters.MoneyMovement, bool, error) {
	kind, ok := moneyKind(bt)
	if !ok {
		return supporters.MoneyMovement{}, false, nil
	}
	if bt.Net != bt.Amount-bt.Fee {
		return supporters.MoneyMovement{}, false, fmt.Errorf("stripe: %w: balance transaction %s has net %d of %d - %d",
			supporters.ErrInvalidPaymentResponse, bt.ID, bt.Net, bt.Amount, bt.Fee)
	}
	cur := strings.ToUpper(string(bt.Currency))
	at := unixTime(bt.Created)
	m := supporters.MoneyMovement{
		ID: bt.ID, Kind: kind, OccurredAt: at, OccurredOn: supporters.DateOf(at.In(p.Location())),
		Gross: decimal(bt.Amount, cur), Fee: decimal(bt.Fee, cur), Net: decimal(bt.Net, cur), Currency: cur, Final: true,
	}
	src := bt.Source
	var pi string
	var charge *stripe.Charge
	switch {
	case kind == supporters.MoneyFee || src == nil:
	case kind == supporters.MoneyPayment:
		if src.Charge != nil {
			pi = paymentIntentID(src.Charge.PaymentIntent)
		}
	case src.Refund != nil:
		charge = src.Refund.Charge
		pi = paymentIntentID(src.Refund.PaymentIntent)
	case src.Dispute != nil:
		charge = src.Dispute.Charge
		pi = paymentIntentID(src.Dispute.PaymentIntent)
	}
	if kind.HasParent() {
		parent, err := p.chargePayment(ctx, charge)
		if err != nil {
			return supporters.MoneyMovement{}, false, err
		}
		if parent == nil {
			return supporters.MoneyMovement{}, false, fmt.Errorf("stripe: %w: balance transaction %s has no charge", supporters.ErrInvalidPaymentResponse, bt.ID)
		}
		m.ParentID, m.ParentGross = parent.ID, decimal(parent.Amount, string(parent.Currency))
		if pi == "" {
			pi = paymentIntentID(parent.chargePI)
		}
	}
	if pi != "" {
		l, err := p.link(ctx, pi)
		if err != nil {
			return supporters.MoneyMovement{}, false, err
		}
		m.Subscription = l.sub
		if kind == supporters.MoneyPayment {
			m.Covers = l.covers(p.Location())
		}
	}
	return m, true, nil
}

func paymentIntentID(pi *stripe.PaymentIntent) string {
	if pi == nil {
		return ""
	}
	return pi.ID
}

// parentPayment is the balance transaction of a charge, with the charge's payment intent.
type parentPayment struct {
	ID       string
	Amount   int64
	Currency stripe.Currency
	chargePI *stripe.PaymentIntent
}

// chargePayment returns the balance transaction of a refund's or a dispute's charge, reading the
// charge with it.
func (p *Provider) chargePayment(ctx context.Context, ch *stripe.Charge) (*parentPayment, error) {
	if ch == nil || ch.ID == "" {
		return nil, nil
	}
	err := p.do(ctx, "read the charge "+ch.ID, func() (err error) {
		ch, err = p.sc.V1Charges.Retrieve(ctx, ch.ID, &stripe.ChargeRetrieveParams{Expand: expand("", []string{"balance_transaction"})})
		return err
	})
	if err != nil {
		return nil, err
	}
	if ch.BalanceTransaction == nil || ch.BalanceTransaction.ID == "" {
		return nil, fmt.Errorf("stripe: %w: the charge %s has no balance transaction", supporters.ErrInvalidPaymentResponse, ch.ID)
	}
	bt := ch.BalanceTransaction
	return &parentPayment{ID: bt.ID, Amount: bt.Amount, Currency: stripe.Currency(strings.ToUpper(string(bt.Currency))), chargePI: ch.PaymentIntent}, nil
}

// subscriptionLink is the subscription of a payment intent, through its invoice.
type subscriptionLink struct {
	sub        supporters.SubscriptionRef
	start, end int64 // the invoice's period of service; 0 without one
}

// covers returns the days of the period, the day of its end excluded (ADR-0073 §1).
func (l subscriptionLink) covers(loc *time.Location) supporters.DateRange {
	if l.start == 0 || l.end <= l.start {
		return supporters.DateRange{}
	}
	return supporters.DateRange{
		Start: supporters.DateOf(time.Unix(l.start, 0).In(loc)),
		End:   supporters.DateOf(time.Unix(l.end, 0).In(loc).AddDate(0, 0, -1)),
	}
}

// link returns the subscription and the period of the invoice that a payment intent paid, from
// its invoice payments (API version basil and later: PaymentIntent -> invoice_payments ->
// invoice -> parent.subscription_details.subscription), cached.
func (p *Provider) link(ctx context.Context, pi string) (subscriptionLink, error) {
	if l, ok := p.links.get(pi); ok {
		return l, nil
	}
	var list *stripe.V1List[*stripe.InvoicePayment]
	err := p.do(ctx, "list the invoice payments of "+pi, func() error {
		list = p.sc.V1InvoicePayments.List(ctx, &stripe.InvoicePaymentListParams{
			ListParams: stripe.ListParams{Limit: stripe.Int64(10), Single: true},
			Payment:    &stripe.InvoicePaymentListPaymentParams{Type: stripe.String("payment_intent"), PaymentIntent: stripe.String(pi)},
			Expand:     expand("data.", []string{"invoice"}),
		})
		return list.Err()
	})
	if err != nil {
		return subscriptionLink{}, err
	}
	var l subscriptionLink
	for _, ip := range list.Data() {
		if ip.Invoice != nil {
			l = invoiceLink(ip.Invoice)
			break
		}
	}
	p.links.put(pi, l)
	return l, nil
}

// invoiceLink returns the subscription of an invoice and the period of its priced lines.
func invoiceLink(inv *stripe.Invoice) subscriptionLink {
	var l subscriptionLink
	if inv.Parent != nil && inv.Parent.SubscriptionDetails != nil && inv.Parent.SubscriptionDetails.Subscription != nil {
		l.sub = supporters.SubscriptionRef(inv.Parent.SubscriptionDetails.Subscription.ID)
	}
	if inv.Lines == nil {
		return l
	}
	for _, positive := range []bool{true, false} {
		for _, line := range inv.Lines.Data {
			if line.Period == nil || line.Period.Start == 0 || (positive && line.Amount <= 0) {
				continue
			}
			if l.start == 0 || line.Period.Start < l.start {
				l.start = line.Period.Start
			}
			l.end = max(l.end, line.Period.End)
		}
		if l.start != 0 {
			break
		}
	}
	return l
}

// linkCacheSize bounds the links kept; the cache starts over when it is full.
const linkCacheSize = 4096

type linkCache struct {
	mu   sync.Mutex
	size int
	m    map[string]subscriptionLink
}

func newLinkCache(size int) *linkCache {
	return &linkCache{size: size, m: map[string]subscriptionLink{}}
}

func (c *linkCache) get(k string) (subscriptionLink, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.m[k]
	return l, ok
}

func (c *linkCache) put(k string, l subscriptionLink) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= c.size {
		clear(c.m)
	}
	c.m[k] = l
}

// Resolve implements supporters.PaymentProvider. A subscription's or a checkout's notice names
// what to re-read. A notice of money re-reads the balance transactions of its object: of an
// invoice, those of the charges that paid it, with its subscription; of a charge, its payment
// and its refunds'; of a refund or a dispute, its own. The subscriptions of the movements are
// re-read too.
func (p *Provider) Resolve(ctx context.Context, n supporters.Notice) (supporters.Change, error) {
	var ch supporters.Change
	switch n.Kind {
	case supporters.NoticeSubscription:
		ch.Subscriptions = []supporters.SubscriptionRef{supporters.SubscriptionRef(n.ObjectID)}
	case supporters.NoticeCheckout:
		ch.Checkout = supporters.CheckoutRef(n.ObjectID)
	case supporters.NoticeMoney:
		var err error
		ch, err = p.resolveMoney(ctx, n.ObjectID)
		if err != nil {
			return supporters.Change{}, err
		}
		for _, m := range ch.Money {
			if m.Subscription != "" && !slices.Contains(ch.Subscriptions, m.Subscription) {
				ch.Subscriptions = append(ch.Subscriptions, m.Subscription)
			}
		}
	}
	return ch, nil
}

func (p *Provider) resolveMoney(ctx context.Context, object string) (supporters.Change, error) {
	var ch supporters.Change
	switch {
	case object == "":
		return ch, nil
	case strings.HasPrefix(object, "txn_"):
		m, err := p.GetMoney(ctx, object)
		if errors.Is(err, supporters.ErrPaymentNotFound) {
			return ch, nil
		}
		if err != nil {
			return ch, err
		}
		ch.Money = []supporters.MoneyMovement{m}
		return ch, nil
	case strings.HasPrefix(object, "in_"):
		return p.resolveInvoice(ctx, object)
	}
	ms, err := p.moneyOf(ctx, object)
	if err != nil {
		return ch, err
	}
	ch.Money = ms
	if strings.HasPrefix(object, "ch_") || strings.HasPrefix(object, "py_") {
		var refunds *stripe.V1List[*stripe.Refund]
		err := p.do(ctx, "list the refunds of "+object, func() error {
			refunds = p.sc.V1Refunds.List(ctx, &stripe.RefundListParams{
				ListParams: stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true}, Charge: stripe.String(object),
			})
			return refunds.Err()
		})
		if err != nil {
			return ch, err
		}
		for _, r := range refunds.Data() {
			ms, err := p.moneyOf(ctx, r.ID)
			if err != nil {
				return ch, err
			}
			ch.Money = append(ch.Money, ms...)
		}
	}
	return ch, nil
}

// moneyOf returns the movements of the balance transactions whose source is object.
func (p *Provider) moneyOf(ctx context.Context, object string) ([]supporters.MoneyMovement, error) {
	var list *stripe.V1List[*stripe.BalanceTransaction]
	err := p.do(ctx, "list the balance transactions of "+object, func() error {
		list = p.sc.V1BalanceTransactions.List(ctx, &stripe.BalanceTransactionListParams{
			ListParams: stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true},
			Source:     stripe.String(object), Expand: expand("data.", []string{"source"}),
		})
		return list.Err()
	})
	if err != nil {
		return nil, err
	}
	var out []supporters.MoneyMovement
	for _, bt := range list.Data() {
		m, ok, err := p.movement(ctx, bt)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// resolveInvoice returns the subscription of an invoice and the movements of its paid charges.
func (p *Provider) resolveInvoice(ctx context.Context, id string) (supporters.Change, error) {
	var ch supporters.Change
	var inv *stripe.Invoice
	err := p.do(ctx, "read the invoice "+id, func() (err error) {
		inv, err = p.sc.V1Invoices.Retrieve(ctx, id, &stripe.InvoiceRetrieveParams{})
		return err
	})
	if err != nil {
		return ch, err
	}
	l := invoiceLink(inv)
	if l.sub != "" {
		ch.Subscriptions = []supporters.SubscriptionRef{l.sub}
	}
	charges, err := p.paidCharges(ctx, id, l)
	if err != nil {
		return ch, err
	}
	for _, charge := range charges {
		ms, err := p.moneyOf(ctx, charge)
		if err != nil {
			return ch, err
		}
		ch.Money = append(ch.Money, ms...)
	}
	return ch, nil
}

// paidCharges returns the charges of an invoice's paid payments, and, when sub is known, remembers
// that their payment intents paid it.
func (p *Provider) paidCharges(ctx context.Context, invoice string, link subscriptionLink) ([]string, error) {
	var list *stripe.V1List[*stripe.InvoicePayment]
	err := p.do(ctx, "list the payments of the invoice "+invoice, func() error {
		list = p.sc.V1InvoicePayments.List(ctx, &stripe.InvoicePaymentListParams{
			ListParams: stripe.ListParams{Limit: stripe.Int64(10), Single: true},
			Invoice:    stripe.String(invoice), Expand: expand("data.", []string{"payment.payment_intent"}),
		})
		return list.Err()
	})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ip := range list.Data() {
		if ip.Status != "paid" || ip.Payment == nil {
			continue
		}
		switch {
		case ip.Payment.Charge != nil:
			out = append(out, ip.Payment.Charge.ID)
		case ip.Payment.PaymentIntent != nil && ip.Payment.PaymentIntent.LatestCharge != nil:
			if link.sub != "" {
				p.links.put(ip.Payment.PaymentIntent.ID, link)
			}
			out = append(out, ip.Payment.PaymentIntent.LatestCharge.ID)
		}
	}
	return out, nil
}

// ListChanges implements supporters.PaymentProvider with Stripe's events of the types of
// EventTypes, oldest first, from since (at most 30 days back) through the time of the listing:
// like ListMoney, the first listing walks back to the oldest page, then reads the newer ones.
// The subscription of an event's object fills Notice.Subscription when the object names it.
func (p *Provider) ListChanges(ctx context.Context, since time.Time, cursor string) (supporters.ChangePage, error) {
	now := p.o.Now()
	if since.Before(now.Add(-changesWindow)) {
		return supporters.ChangePage{}, fmt.Errorf("stripe: %w: %s", supporters.ErrChangesWindow, since.UTC().Format(time.RFC3339))
	}
	types := EventTypes()
	params := &stripe.EventListParams{
		ListParams:   stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true},
		CreatedRange: &stripe.RangeQueryParams{GreaterThanOrEqual: since.Unix()},
		Types:        make([]*string, len(types)),
	}
	for i, t := range types {
		params.Types[i] = stripe.String(t)
	}
	after, isAfter := strings.CutPrefix(cursor, cursorAfter)
	seek, isSeek := strings.CutPrefix(cursor, cursorSeek)
	switch {
	case isAfter && after != "":
		params.EndingBefore = stripe.String(after)
	case isSeek && seek != "":
		params.StartingAfter = stripe.String(seek)
	case cursor != "":
		return supporters.ChangePage{}, fmt.Errorf("stripe: %w: unknown cursor %q", supporters.ErrPaymentInvalidRequest, cursor)
	}
	var list *stripe.V1List[*stripe.Event]
	err := p.do(ctx, "list the events", func() error {
		list = p.sc.V1Events.List(ctx, params)
		return list.Err()
	})
	if err != nil {
		return supporters.ChangePage{}, err
	}
	data := list.Data()
	more := list.Meta().HasMore
	page := supporters.ChangePage{Until: now}
	if !isAfter {
		if more && len(data) > 0 {
			page.Next = cursorSeek + data[len(data)-1].ID
			return page, nil
		}
		data = slices.Clone(data)
		slices.Reverse(data)
		more = isSeek && len(data) > 0
	}
	for _, e := range data {
		object, sub := eventObject(e)
		page.Notices = append(page.Notices, notice(e.ID, string(e.Type), object, e.Created, sub))
	}
	if more && len(data) > 0 {
		page.Next = cursorAfter + data[len(data)-1].ID
	}
	return page, nil
}

// eventObject returns the ID of an event's object and the subscription it names, read from the
// fields that the API versions keep: the subscription itself, the subscription of a checkout,
// and the subscription of an invoice (parent.subscription_details since basil, subscription
// before).
func eventObject(e *stripe.Event) (id, sub string) {
	if e.Data == nil {
		return "", ""
	}
	var o struct {
		ID           string          `json:"id"`
		Object       string          `json:"object"`
		Subscription json.RawMessage `json:"subscription"`
		Parent       *struct {
			SubscriptionDetails *struct {
				Subscription json.RawMessage `json:"subscription"`
			} `json:"subscription_details"`
		} `json:"parent"`
	}
	if err := json.Unmarshal(e.Data.Raw, &o); err != nil {
		return "", ""
	}
	switch o.Object {
	case "subscription":
		sub = o.ID
	case "checkout.session":
		sub = refID(o.Subscription)
	case "invoice":
		if o.Parent != nil && o.Parent.SubscriptionDetails != nil {
			sub = refID(o.Parent.SubscriptionDetails.Subscription)
		}
		if sub == "" {
			sub = refID(o.Subscription)
		}
	}
	return o.ID, sub
}

// refID returns the ID of a field that is an ID or an expanded object.
func refID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var id string
	if json.Unmarshal(raw, &id) == nil {
		return id
	}
	var o struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.ID
	}
	return ""
}
