// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrInvalidBasis reports an unknown basis.
var ErrInvalidBasis = errors.New("finance: invalid basis")

// Basis is how the totals place a payment (ADR-0067 §4).
type Basis string

// Bases.
const (
	// BasisAmortized spreads each payment of a schedule longer than a month by day over the dates
	// it covers, and keeps the others on their payment dates. It is the default.
	BasisAmortized Basis = "amortized"
	// BasisCash places each payment on its payment date.
	BasisCash Basis = "cash"
)

// Validate fails with ErrInvalidBasis for an unknown basis.
func (b Basis) Validate() error {
	if b != BasisAmortized && b != BasisCash {
		return fmt.Errorf("%w: %q", ErrInvalidBasis, b)
	}
	return nil
}

// Entry is an amount to spread by day over a range of dates (Spread): a payment of a cost item
// on its date or over the dates it covers, or an imported cost over its billing period (§5).
type Entry struct {
	Range  DateRange
	Amount Money
	// Provider is the normalized provider that groups the breakdown, and Category its category.
	Provider string
	Category Category
	// Spelling is the provider as entered, which a breakdown line of Provider may show (ADR-0067).
	Spelling Spelling
	// Estimated marks an imported cost of a month that its provider has not closed (ADR-0068 §3).
	Estimated bool
	// Item is what the entry is a payment of: a cost item or a service of an imported cost. The
	// totals ignore it; the item lines of ItemizedTotals group by it (ADR-0070 §5).
	Item ItemRef
	// Span, when set, is the range that Amount spreads over by day, of which only the dates of
	// Range carry their part: the rest of a refund of an amortized payment, which cancels the
	// payment's later days with the payment's own rounding (ADR-0067 §15). Nil spreads Amount over
	// Range.
	Span *DateRange
}

// amountIn returns the part of the entry's amount that falls on the dates of part (Spread).
func (e Entry) amountIn(part DateRange) int64 {
	if e.Span == nil {
		return Spread(e.Amount.Minor, e.Range, part)
	}
	in, ok := e.Range.Intersect(part)
	if !ok {
		return 0
	}
	return Spread(e.Amount.Minor, *e.Span, in)
}

// Spelling is a provider as entered, which the lines of its normalized provider in a period may
// show (ADR-0067): the provider of a cost item's current label revision, recorded at RevisedAt as
// the revision RevisionID, or the provider's name of an imported cost, with no revision.
type Spelling struct {
	Name       string
	RevisedAt  time.Time
	RevisionID string
}

// newer reports whether s wins over o as the spelling of a period's lines: a cost item's spelling
// over an imported cost's, then the newer label revision, then the larger revision ID.
func (s Spelling) newer(o Spelling) bool {
	if (s.RevisionID != "") != (o.RevisionID != "") {
		return s.RevisionID != ""
	}
	if !s.RevisedAt.Equal(o.RevisedAt) {
		return s.RevisedAt.After(o.RevisedAt)
	}
	return s.RevisionID > o.RevisionID
}

// spellings returns the spelling of each normalized provider among the entries that meet part:
// the newest one (Spelling.newer). A provider whose entries carry no spelling has none.
func spellings(entries []Entry, part DateRange) map[string]string {
	best := map[string]Spelling{}
	for _, e := range entries {
		if _, ok := e.Range.Intersect(part); !ok || e.Spelling.Name == "" {
			continue
		}
		if cur, ok := best[e.Provider]; !ok || e.Spelling.newer(cur) {
			best[e.Provider] = e.Spelling
		}
	}
	out := make(map[string]string, len(best))
	for k, v := range best {
		out[k] = v.Name
	}
	return out
}

// spellingOf returns the spelling of provider in names, or provider itself without one.
func spellingOf(names map[string]string, provider string) string {
	if name, ok := names[provider]; ok {
		return name
	}
	return provider
}

// ItemRef is what an entry is a payment of: Key tells the cost item or the service of an
// imported cost apart from every other one, Name is the item's name or the service, and Memo is
// the item's memo ("" for an imported cost).
type ItemRef struct {
	Key  string
	Name string
	Memo string
}

// ItemLine is the amount of an item in a currency over a period: the cost item or the service
// of an imported cost of Item, with the provider and category that group its breakdown, and the
// spelling of the provider that the period's lines show (ADR-0067).
type ItemLine struct {
	Item         ItemRef
	Provider     string
	ProviderName string
	Category     Category
	Amount       Money
}

// Entries returns the entries of the item's payments that meet window under basis: on the
// payment date, or over the covered dates when basis is amortized and the schedule amortizes.
func (c CostItem) Entries(basis Basis, window DateRange) []Entry {
	amortize := basis == BasisAmortized && c.Recurrence.Amortizes()
	var out []Entry
	for _, p := range c.Payments(window) {
		r := DateRange{Start: p.On, End: p.On}
		if amortize {
			r = p.Covers
		}
		if _, ok := r.Intersect(window); !ok {
			continue
		}
		out = append(out, Entry{
			Range: r, Amount: p.Price, Provider: c.ProviderNormalized, Category: c.Label.Category,
			Spelling: Spelling{Name: c.Label.Provider, RevisedAt: c.LabelRevisedAt, RevisionID: c.LabelRevisionID},
			Item:     ItemRef{Key: "item:" + c.ID, Name: c.Label.Name, Memo: c.Label.Memo},
		})
	}
	return out
}

// BreakdownKey groups the breakdown of a period: a provider, a category, and a currency.
type BreakdownKey struct {
	Provider string
	Category Category
	Currency Currency
}

// Line is an amount of a breakdown key, with the spelling of its provider that the period's lines
// of the key's provider show (ADR-0067).
type Line struct {
	Key          BreakdownKey
	ProviderName string
	Amount       int64
}

// PeriodTotal is what a period carries: the total of each currency, by currency code, and the
// breakdown by provider, category, and currency, sorted by provider, category, and currency.
// A currency appears when an entry meets the period, even with a zero amount. Estimated reports
// that an estimated entry meets the period (ADR-0068 §3).
type PeriodTotal struct {
	Period    Period
	Totals    []Money
	Breakdown []Line
	Estimated bool
	// Items are the item lines of the period, which only ItemizedTotals fills (ADR-0070 §5).
	Items []ItemLine
}

// Summarize spreads entries over periods (Spread) and totals each period by currency, never
// converting between currencies (ADR-0067 §6).
func Summarize(entries []Entry, periods []Period) []PeriodTotal {
	out := make([]PeriodTotal, len(periods))
	for i, p := range periods {
		totals := map[Currency]int64{}
		lines := map[BreakdownKey]int64{}
		estimated := false
		for _, e := range entries {
			if _, ok := e.Range.Intersect(p.Range); !ok {
				continue
			}
			estimated = estimated || e.Estimated
			amount := e.amountIn(p.Range)
			totals[e.Amount.Currency] += amount
			lines[BreakdownKey{Provider: e.Provider, Category: e.Category, Currency: e.Amount.Currency}] += amount
		}
		t := PeriodTotal{Period: p, Totals: make([]Money, 0, len(totals)), Breakdown: make([]Line, 0, len(lines)), Estimated: estimated}
		for c, amount := range totals {
			t.Totals = append(t.Totals, Money{Minor: amount, Currency: c})
		}
		slices.SortFunc(t.Totals, func(a, b Money) int { return cmp.Compare(a.Currency, b.Currency) })
		names := spellings(entries, p.Range)
		for k, amount := range lines {
			t.Breakdown = append(t.Breakdown, Line{Key: k, ProviderName: spellingOf(names, k.Provider), Amount: amount})
		}
		slices.SortFunc(t.Breakdown, func(a, b Line) int {
			return cmp.Or(cmp.Compare(a.Key.Provider, b.Key.Provider), cmp.Compare(a.Key.Category, b.Key.Category),
				cmp.Compare(a.Key.Currency, b.Key.Currency))
		})
		out[i] = t
	}
	return out
}

// Totals returns the totals of items under basis over periods: the entries of the items' payments
// that meet the periods, summarized with the entries of the imported costs, which every basis
// spreads by day over their billing periods (ADR-0067 §5).
func Totals(items []CostItem, imported []Entry, basis Basis, periods []Period) []PeriodTotal {
	if len(periods) == 0 {
		return nil
	}
	return Summarize(windowEntries(items, imported, basis, periods), periods)
}

// ItemizedTotals returns the Totals of items, imported, basis, and periods with the item lines
// of each period: the amount of each cost item and each service of the imported costs, by
// currency, for the items whose entries meet the period, sorted by provider, category, name, key,
// and currency (ADR-0070 §5).
func ItemizedTotals(items []CostItem, imported []Entry, basis Basis, periods []Period) []PeriodTotal {
	if len(periods) == 0 {
		return nil
	}
	entries := windowEntries(items, imported, basis, periods)
	out := Summarize(entries, periods)
	type key struct {
		item     string
		currency Currency
	}
	for i, p := range periods {
		names := spellings(entries, p.Range)
		lines := map[key]*ItemLine{}
		for _, e := range entries {
			if _, ok := e.Range.Intersect(p.Range); !ok {
				continue
			}
			k := key{item: e.Item.Key, currency: e.Amount.Currency}
			line, ok := lines[k]
			if !ok {
				line = &ItemLine{
					Item: e.Item, Provider: e.Provider, ProviderName: spellingOf(names, e.Provider), Category: e.Category,
					Amount: Money{Currency: e.Amount.Currency},
				}
				lines[k] = line
			}
			line.Amount.Minor += e.amountIn(p.Range)
		}
		items := make([]ItemLine, 0, len(lines))
		for _, line := range lines {
			items = append(items, *line)
		}
		slices.SortFunc(items, func(a, b ItemLine) int {
			return cmp.Or(cmp.Compare(a.Provider, b.Provider), cmp.Compare(a.Category, b.Category), cmp.Compare(a.Item.Name, b.Item.Name),
				cmp.Compare(a.Item.Key, b.Item.Key), cmp.Compare(a.Amount.Currency, b.Amount.Currency))
		})
		out[i].Items = items
	}
	return out
}

// windowEntries returns the entries of items under basis and of imported that meet periods.
func windowEntries(items []CostItem, imported []Entry, basis Basis, periods []Period) []Entry {
	window := DateRange{Start: periods[0].Range.Start, End: periods[len(periods)-1].Range.End}
	var entries []Entry
	for _, c := range items {
		entries = append(entries, c.Entries(basis, window)...)
	}
	for _, e := range imported {
		if _, ok := e.Range.Intersect(window); ok {
			entries = append(entries, e)
		}
	}
	return entries
}
