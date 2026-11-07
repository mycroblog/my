// SPDX-License-Identifier: AGPL-3.0-only

package financetest

import (
	"cmp"
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// FakeBillingSource is the fake adapter of the appfinance.BillingSource port (ADR-0068 §1): an
// in-memory provider whose billing data the test sets month by month, with the number of billed
// pages that a fetch takes and an optional failure. It passes BillingSourceContract and stands in
// for the aws and gcp adapters in the tests of the imports.
type FakeBillingSource struct {
	name   string
	loc    *time.Location
	method string

	mu       sync.Mutex
	months   map[finance.YearMonth]FakeBillingMonth
	pages    int
	failure  error
	calls    int
	requests []appfinance.FetchRequest
}

// FakeBillingMonth is the provider's data of a billing month: open (an estimate through Through,
// the last day with data; the zero Date means the end of the month) or Final, and its raw rows,
// which may offset each other.
type FakeBillingMonth struct {
	Final   bool
	Through finance.Date
	Rows    []FakeBillingRow
}

// FakeBillingRow is a raw row of the provider: an unrounded decimal amount in major units.
type FakeBillingRow struct {
	Service    string
	ChargeType finance.ChargeType
	Amount     string
	Currency   string
}

// NewFakeBillingSource returns a source named name whose billing months are in loc, with no data
// and one billed page per fetch. Its method is the default of the source: MethodCostExplorer for
// aws, MethodBigQuery otherwise; NewFakeBillingSourceMethod sets another.
func NewFakeBillingSource(name string, loc *time.Location) *FakeBillingSource {
	method := appfinance.MethodBigQuery
	if name == "aws" {
		method = appfinance.MethodCostExplorer
	}
	return NewFakeBillingSourceMethod(name, loc, method)
}

// NewFakeBillingSourceMethod returns NewFakeBillingSource with the given method.
func NewFakeBillingSourceMethod(name string, loc *time.Location, method string) *FakeBillingSource {
	return &FakeBillingSource{name: name, loc: loc, method: method, months: map[finance.YearMonth]FakeBillingMonth{}, pages: 1}
}

// SetMonth sets the provider's data of month m.
func (f *FakeBillingSource) SetMonth(m finance.YearMonth, data FakeBillingMonth) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.months[m] = data
}

// SetPages sets the billed pages that a fetch takes (at least 1).
func (f *FakeBillingSource) SetPages(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages = max(1, n)
}

// SetFailure makes every fetch fail with err after its first billed page; nil clears it.
func (f *FakeBillingSource) SetFailure(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failure = err
}

// Calls returns the billed pages fetched so far.
func (f *FakeBillingSource) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// Requests returns the requests that Fetch received, oldest first.
func (f *FakeBillingSource) Requests() []appfinance.FetchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

// Name implements appfinance.BillingSource.
func (f *FakeBillingSource) Name() string { return f.name }

// Location implements appfinance.BillingSource.
func (f *FakeBillingSource) Location() *time.Location { return f.loc }

// Method implements appfinance.BillingSource.
func (f *FakeBillingSource) Method() string { return f.method }

// fakeServiceCategories is the fake provider's table of known services.
var fakeServiceCategories = map[string]finance.Category{
	"Amazon Elastic Compute Cloud - Compute": finance.CategoryCompute,
	"Amazon Relational Database Service":     finance.CategoryDatabase,
	"Amazon Simple Storage Service":          finance.CategoryStorage,
	"Amazon CloudFront":                      finance.CategoryNetwork,
	"Compute Engine":                         finance.CategoryCompute,
	"Cloud Storage":                          finance.CategoryStorage,
}

// Fetch implements appfinance.BillingSource. Each billed page counts one request; the lines come
// with the last page, so a fetch cut short by the cap returns none.
func (f *FakeBillingSource) Fetch(ctx context.Context, req appfinance.FetchRequest) (appfinance.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, appfinance.FetchRequest{Months: slices.Clone(req.Months), MaxRequests: req.MaxRequests})
	var snap appfinance.Snapshot
	if err := ctx.Err(); err != nil {
		return snap, err
	}
	for page := range f.pages {
		if snap.Requests >= req.MaxRequests {
			return appfinance.Snapshot{Requests: snap.Requests}, fmt.Errorf("%w: %d of %d pages", appfinance.ErrRequestCap, page, f.pages)
		}
		if err := ctx.Err(); err != nil {
			return appfinance.Snapshot{Requests: snap.Requests}, err
		}
		snap.Requests++
		f.calls++
		if f.failure != nil {
			return appfinance.Snapshot{Requests: snap.Requests}, f.failure
		}
	}
	for _, m := range req.Months {
		data, ok := f.months[m]
		if !ok {
			continue
		}
		lines, err := fakeLines(m, data)
		if err != nil {
			return appfinance.Snapshot{Requests: snap.Requests}, err
		}
		snap.Lines = append(snap.Lines, lines...)
	}
	return snap, nil
}

// fakeLines sums the rows of a month by service and charge type, exactly: the sum of decimals of
// at most n fraction digits has at most n.
func fakeLines(m finance.YearMonth, data FakeBillingMonth) ([]appfinance.CostLine, error) {
	type key struct {
		service string
		charge  finance.ChargeType
	}
	sums := map[key]*big.Rat{}
	scales := map[key]int{}
	currency := map[key]string{}
	var order []key
	for _, r := range data.Rows {
		k := key{r.Service, r.ChargeType}
		amount, err := finance.ParseDecimal(r.Amount)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", appfinance.ErrInvalidBillingResponse, err)
		}
		if _, ok := sums[k]; !ok {
			sums[k] = new(big.Rat)
			order = append(order, k)
		}
		sums[k].Add(sums[k], amount)
		if _, frac, ok := strings.Cut(r.Amount, "."); ok {
			scales[k] = max(scales[k], len(frac))
		}
		if c, ok := currency[k]; ok && c != r.Currency {
			return nil, fmt.Errorf("%w: %s %s mixes currencies", appfinance.ErrInvalidBillingResponse, r.Service, r.ChargeType)
		}
		currency[k] = r.Currency
	}
	covered := m.Range()
	if !data.Final && data.Through != (finance.Date{}) {
		covered.End = data.Through
	}
	out := make([]appfinance.CostLine, 0, len(order))
	for _, k := range order {
		category, ok := fakeServiceCategories[k.service]
		if !ok || k.charge == finance.ChargeTax {
			category = finance.CategoryOther
		}
		out = append(out, appfinance.CostLine{
			Month: m, Covered: covered, Service: k.service, ChargeType: k.charge, Category: category,
			Amount: sums[k].FloatString(scales[k]), Currency: currency[k], Final: data.Final,
		})
	}
	slices.SortFunc(out, func(a, b appfinance.CostLine) int {
		return cmp.Or(cmp.Compare(a.Service, b.Service), cmp.Compare(a.ChargeType, b.ChargeType))
	})
	return out, nil
}
