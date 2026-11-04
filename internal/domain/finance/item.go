// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Category is the fixed category of a cost item (cost_item_label_revisions.category); the
// catalogs translate its name (ADR-0067 §2).
type Category string

// Categories, in the order of the schema's CHECK.
const (
	CategoryCompute  Category = "compute"
	CategoryDatabase Category = "database"
	CategoryStorage  Category = "storage"
	CategoryNetwork  Category = "network"
	CategoryDomain   Category = "domain"
	CategoryEmail    Category = "email"
	CategorySoftware Category = "software"
	CategoryOther    Category = "other"
)

var categories = []Category{
	CategoryCompute, CategoryDatabase, CategoryStorage, CategoryNetwork, CategoryDomain, CategoryEmail,
	CategorySoftware, CategoryOther,
}

// Categories returns the categories in a stable order.
func Categories() []Category { return slices.Clone(categories) }

// Valid reports whether c is a known category.
func (c Category) Valid() bool { return slices.Contains(categories, c) }

// Limits of a cost item's label.
const (
	// MaxName and MaxProvider are the number of characters of a name and a provider.
	MaxName     = 100
	MaxProvider = 100
	// MaxMemo is the number of characters of a memo.
	MaxMemo = 10000
)

var (
	// ErrInvalidName reports an empty name, one that is too long, or one with control characters.
	ErrInvalidName = errors.New("finance: invalid name")
	// ErrInvalidProvider reports an empty provider, one that is too long, or one with control
	// characters.
	ErrInvalidProvider = errors.New("finance: invalid provider")
	// ErrInvalidCategory reports an unknown category.
	ErrInvalidCategory = errors.New("finance: invalid category")
	// ErrInvalidMemo reports a memo that is too long, invalid UTF-8, or holds a NUL.
	ErrInvalidMemo = errors.New("finance: invalid memo")
	// ErrInvalidAmount reports a negative amount of a recorded price.
	ErrInvalidAmount = errors.New("finance: invalid amount")
	// ErrBeforeStart reports a price revision or an end before the item's start date.
	ErrBeforeStart = errors.New("finance: before the start date")
)

// Label is what a cost item is called and grouped by: one revision of it (ADR-0067 §2). The
// provider groups the breakdown in its normalized form, which the application derives.
type Label struct {
	Name     string
	Provider string
	Category Category
	// Memo is the operator's note, never published; "" for none.
	Memo string
}

// Normalize trims the spaces around the name, the provider, and the memo.
func (l Label) Normalize() Label {
	l.Name = strings.TrimSpace(l.Name)
	l.Provider = strings.TrimSpace(l.Provider)
	l.Memo = strings.TrimSpace(l.Memo)
	return l
}

// Validate checks a normalized label.
func (l Label) Validate() error {
	if !validLine(l.Name, MaxName) {
		return ErrInvalidName
	}
	if !validLine(l.Provider, MaxProvider) {
		return ErrInvalidProvider
	}
	if !l.Category.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidCategory, l.Category)
	}
	if !utf8.ValidString(l.Memo) || strings.ContainsRune(l.Memo, 0) || utf8.RuneCountInString(l.Memo) > MaxMemo {
		return ErrInvalidMemo
	}
	return nil
}

// validLine reports whether s is valid UTF-8 of 1 to limit characters without control characters.
func validLine(s string, limit int) bool {
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > limit {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

// ValidatePrice fails with ErrInvalidAmount for a negative amount and with ErrInvalidCurrency for
// an unknown currency: a recorded cost is never negative (ADR-0067 §2).
func ValidatePrice(m Money) error {
	if m.Minor < 0 {
		return fmt.Errorf("%w: %d", ErrInvalidAmount, m.Minor)
	}
	if !m.Currency.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidCurrency, m.Currency)
	}
	return nil
}

// PriceRevision is a price of a cost item from a date on (cost_item_price_revisions): the price
// of every payment on or after EffectiveOn until a later revision. Revisions with the same date
// apply in the order they were recorded, the last one winning.
type PriceRevision struct {
	ID          string
	EffectiveOn Date
	Price       Money
	RevisedAt   time.Time
}

// CostItem is a cost of the instance that is not canceled, with its current label and end and
// every price revision (ADR-0067 §2).
type CostItem struct {
	ID         string
	Recurrence Recurrence
	// StartsOn is the first payment date, the payment date of once.
	StartsOn Date
	// EndsOn is the last date that a payment can fall on (included); nil for no end.
	EndsOn *Date
	Label  Label
	// ProviderNormalized is the provider in the form that groups the breakdown (NFKC, lower case).
	ProviderNormalized string
	// LabelRevisionID and LabelRevisedAt are the ID and the time of the current label's revision,
	// which choose the provider's spelling of a breakdown line (ADR-0067).
	LabelRevisionID string
	LabelRevisedAt  time.Time
	// Prices are the price revisions in the order they were recorded; the first one starts on
	// StartsOn.
	Prices          []PriceRevision
	CreatedByUserID string // "" for the CLI
	CreatedAt       time.Time
}

// ValidateDates checks the dates that a cost item records against its start date: an effective
// date of a price revision and an end are never before it.
func ValidateDates(startsOn, d Date) error {
	if d.Before(startsOn) {
		return fmt.Errorf("%w: %s before %s", ErrBeforeStart, d, startsOn)
	}
	return nil
}

// Payment is one payment of a cost item: its date, price, and the dates it covers, from its date
// to the day before the next payment (its own date for once).
type Payment struct {
	On     Date
	Price  Money
	Covers DateRange
}

// PriceOn returns the price of a payment on d: the revision with the latest effective date on
// or before d, the last recorded among those of the same date; false when no revision is in effect.
func (c CostItem) PriceOn(d Date) (Money, bool) {
	var (
		best  PriceRevision
		found bool
	)
	for _, p := range c.Prices {
		if p.EffectiveOn.After(d) {
			continue
		}
		if !found || !p.EffectiveOn.Before(best.EffectiveOn) {
			best, found = p, true
		}
	}
	return best.Price, found
}

// Payments returns, in date order, the payments of the item whose covered dates meet window:
// the payments counted from the start date (Recurrence.Occurrence) up to the end date included
// (ADR-0067 §3). The last payment before the end still covers the dates up to the next payment
// date, after the end (§4).
func (c CostItem) Payments(window DateRange) []Payment {
	if !window.Valid() {
		return nil
	}
	var out []Payment
	for k := c.Recurrence.firstCovering(c.StartsOn, window.Start); ; k++ {
		on, ok := c.Recurrence.Occurrence(c.StartsOn, k)
		if !ok || on.After(window.End) || (c.EndsOn != nil && on.After(*c.EndsOn)) {
			break
		}
		covers := DateRange{Start: on, End: on}
		if next, ok := c.Recurrence.Occurrence(c.StartsOn, k+1); ok {
			covers.End = next.AddDays(-1)
		} else if c.Recurrence.Schedule != ScheduleOnce {
			covers.End = MaxDate
		}
		if covers.End.Before(window.Start) {
			continue
		}
		price, ok := c.PriceOn(on)
		if !ok {
			continue
		}
		out = append(out, Payment{On: on, Price: price, Covers: covers})
		if c.Recurrence.Schedule == ScheduleOnce {
			break
		}
	}
	return out
}
