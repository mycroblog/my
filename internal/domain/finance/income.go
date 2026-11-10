// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

// ErrInvalidIncome reports a money movement that breaks the rules of the income records
// (ADR-0067 §12): an unknown kind, amounts in different currencies, a net that is not the gross
// less the fee, or a covered period on a movement that is not a payment.
var ErrInvalidIncome = errors.New("finance: invalid income movement")

// IncomeKind is the kind of a money movement of the payment provider (ADR-0067 §12).
type IncomeKind string

// Income kinds.
const (
	// IncomePayment is a payment of a subscription: the gross that the subscriber paid less the
	// provider's fee of the payment.
	IncomePayment IncomeKind = "payment"
	// IncomeRefund is money given back of a payment: a negative gross, and a fee that is not
	// given back.
	IncomeRefund IncomeKind = "refund"
	// IncomeChargeback is money that a dispute took back of a payment, with the dispute's fee.
	IncomeChargeback IncomeKind = "chargeback"
	// IncomeChargebackReversal is the money of a chargeback that came back.
	IncomeChargebackReversal IncomeKind = "chargeback_reversal"
	// IncomeFee is a fee of the provider that belongs to no payment (Stripe Billing, Tax, Radar).
	IncomeFee IncomeKind = "fee"
)

// IncomeKinds returns every kind, in a fixed order.
func IncomeKinds() []IncomeKind {
	return []IncomeKind{IncomePayment, IncomeRefund, IncomeChargeback, IncomeChargebackReversal, IncomeFee}
}

// Validate fails with ErrInvalidIncome for an unknown kind.
func (k IncomeKind) Validate() error {
	if !slices.Contains(IncomeKinds(), k) {
		return fmt.Errorf("%w: kind %q", ErrInvalidIncome, k)
	}
	return nil
}

// HasParent reports whether movements of the kind belong to a payment: refunds, chargebacks,
// and their reversals.
func (k IncomeKind) HasParent() bool {
	return k == IncomeRefund || k == IncomeChargeback || k == IncomeChargebackReversal
}

// IncomeSource is where a money movement comes from (ADR-0067 §12, ADR-0162 §8).
type IncomeSource string

// Income sources.
const (
	// SourceSubscription is a movement of a subscription of ours.
	SourceSubscription IncomeSource = "subscription"
	// SourceOneTime is a movement of a tip of ours (ADR-0162).
	SourceOneTime IncomeSource = "one_time"
	// SourceUnknown is a movement of neither: a payment taken outside my, or a fee of no
	// subscription.
	SourceUnknown IncomeSource = "unknown"
)

// IncomeSources returns every source, in a fixed order.
func IncomeSources() []IncomeSource {
	return []IncomeSource{SourceSubscription, SourceOneTime, SourceUnknown}
}

// Validate fails with ErrInvalidIncome for an unknown source.
func (s IncomeSource) Validate() error {
	if !slices.Contains(IncomeSources(), s) {
		return fmt.Errorf("%w: source %q", ErrInvalidIncome, s)
	}
	return nil
}

// IncomeMovement is a recorded money movement as the income totals read it (income_movements):
// its kind, its source, the date it falls on in the provider's reporting time zone, the billing period that
// a payment pays for, and its gross, fee, and net in one currency.
type IncomeMovement struct {
	Kind IncomeKind
	// Source is where the movement comes from; "" counts as SourceUnknown in the totals.
	Source     IncomeSource
	OccurredOn Date
	// Covers is the billing period of a payment; nil for the other kinds and for a payment
	// without one.
	Covers *DateRange
	// ParentCovers is the Covers of the payment that a refund, a chargeback, or a reversal
	// belongs to; nil otherwise.
	ParentCovers *DateRange
	Gross, Fee   Money
	Net          Money
}

// Validate fails with ErrInvalidIncome for a movement that breaks the rules of ADR-0067 §12, and
// with the errors of ParseCurrency for an unknown currency.
func (m IncomeMovement) Validate() error {
	if err := m.Kind.Validate(); err != nil {
		return err
	}
	if m.Source != "" {
		if err := m.Source.Validate(); err != nil {
			return err
		}
	}
	if _, err := ParseCurrency(string(m.Net.Currency)); err != nil {
		return err
	}
	if m.Gross.Currency != m.Net.Currency || m.Fee.Currency != m.Net.Currency {
		return fmt.Errorf("%w: amounts in %s, %s, and %s", ErrInvalidIncome, m.Gross.Currency, m.Fee.Currency, m.Net.Currency)
	}
	if m.Net.Minor != m.Gross.Minor-m.Fee.Minor {
		return fmt.Errorf("%w: net %d is not %d - %d", ErrInvalidIncome, m.Net.Minor, m.Gross.Minor, m.Fee.Minor)
	}
	if m.Covers != nil && (m.Kind != IncomePayment || !m.Covers.Valid()) {
		return fmt.Errorf("%w: a %s covers %s", ErrInvalidIncome, m.Kind, m.Covers)
	}
	return nil
}

// IncomeAmortizeAfterDays is the most days that a payment covers and still stays on its date
// under the amortized basis: a month at most.
const IncomeAmortizeAfterDays = 31

// AmortizesIncome reports whether a payment that covers r spreads over it under the amortized
// basis: a period longer than a month (more than IncomeAmortizeAfterDays days, the yearly
// plans), as the cost items of ADR-0067 §4. A monthly payment and a payment without a period
// stay on their dates.
func AmortizesIncome(r *DateRange) bool { return r != nil && r.Days() > IncomeAmortizeAfterDays }

// incomeEntries returns the entries that place amount, a part of m in m's currency, under basis
// (ADR-0067 §15). Cash places it on m's date. Amortized spreads an amortized payment by day over
// its period; a refund, a chargeback, or a reversal of an amortized payment puts the part of
// the payment's days before its own date on its date and spreads the rest by the payment's days,
// so that it cancels the payment's later days and leaves the past periods as they were; the
// other movements stay on their dates.
func incomeEntries(m IncomeMovement, basis Basis, amount Money) []Entry {
	on := Entry{Range: DateRange{Start: m.OccurredOn, End: m.OccurredOn}, Amount: amount}
	if basis != BasisAmortized {
		return []Entry{on}
	}
	switch {
	case m.Kind == IncomePayment && AmortizesIncome(m.Covers):
		return []Entry{{Range: *m.Covers, Amount: amount}}
	case m.Kind.HasParent() && AmortizesIncome(m.ParentCovers):
		span := *m.ParentCovers
		on.Amount.Minor = Spread(amount.Minor, span, DateRange{Start: span.Start, End: m.OccurredOn.AddDays(-1)})
		out := []Entry{on}
		rest := DateRange{Start: maxDate(span.Start, m.OccurredOn), End: span.End}
		if rest.Valid() {
			out = append(out, Entry{Range: rest, Span: &span, Amount: amount})
		}
		return out
	}
	return []Entry{on}
}

// IncomeTotal is what a period of the income totals carries: the net, the gross, and the fee of
// each currency, sorted by currency code. A currency appears when a movement meets the period,
// even with a zero amount. Fee is the gross less the net of the period, so that the three agree
// however the days round. NetBySource is the net of each source that a movement of the period
// has, in the order of IncomeSources: what the administrators see (ADR-0162 §16).
type IncomeTotal struct {
	Period      Period
	Net         []Money
	Gross       []Money
	Fee         []Money
	NetBySource []SourceNet
}

// SourceNet is the net of one source over a period, by currency.
type SourceNet struct {
	Source IncomeSource
	Net    []Money
}

// IncomeTotals places ms under basis (ADR-0067 §15) and totals each period by currency, never
// converting between currencies.
func IncomeTotals(ms []IncomeMovement, basis Basis, periods []Period) []IncomeTotal {
	if len(periods) == 0 {
		return nil
	}
	var net, gross []Entry
	bySource := map[IncomeSource][]Entry{}
	for _, m := range ms {
		e := incomeEntries(m, basis, m.Net)
		net = append(net, e...)
		gross = append(gross, incomeEntries(m, basis, m.Gross)...)
		src := m.Source
		if src == "" {
			src = SourceUnknown
		}
		bySource[src] = append(bySource[src], e...)
	}
	netTotals := Summarize(net, periods)
	grossTotals := Summarize(gross, periods)
	sourceTotals := map[IncomeSource][]PeriodTotal{}
	for src, e := range bySource {
		sourceTotals[src] = Summarize(e, periods)
	}
	out := make([]IncomeTotal, len(periods))
	for i, p := range periods {
		t := IncomeTotal{Period: p, Net: netTotals[i].Totals, Gross: grossTotals[i].Totals, Fee: make([]Money, 0, len(netTotals[i].Totals))}
		for _, g := range t.Gross {
			fee := g
			for _, n := range t.Net {
				if n.Currency == g.Currency {
					fee.Minor -= n.Minor
				}
			}
			t.Fee = append(t.Fee, fee)
		}
		t.NetBySource = []SourceNet{}
		for _, src := range IncomeSources() {
			if st, ok := sourceTotals[src]; ok && len(st[i].Totals) > 0 {
				t.NetBySource = append(t.NetBySource, SourceNet{Source: src, Net: st[i].Totals})
			}
		}
		out[i] = t
	}
	return out
}

// Balance returns the income less the costs of each currency that either has, sorted by
// currency code (ADR-0067 §17): a currency of only one side shows that side's amount, never
// converted.
func Balance(income, costs []Money) []Money {
	sums := map[Currency]int64{}
	for _, m := range income {
		sums[m.Currency] += m.Minor
	}
	for _, m := range costs {
		sums[m.Currency] -= m.Minor
	}
	out := make([]Money, 0, len(sums))
	for c, n := range sums {
		out = append(out, Money{Minor: n, Currency: c})
	}
	slices.SortFunc(out, func(a, b Money) int { return cmp.Compare(a.Currency, b.Currency) })
	return out
}
