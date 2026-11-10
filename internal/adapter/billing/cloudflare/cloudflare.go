// SPDX-License-Identifier: AGPL-3.0-only

// Package cloudflare is a spike (#2664) of a BillingSource over Cloudflare's billable usage API
// (GET /accounts/{account_id}/billable-usage, Version 1, Alpha). It is not merged: it checks how
// the API's records map onto the port of ADR-0068 §1 through the official SDK
// (github.com/cloudflare/cloudflare-go/v7), built without its DefaultClientOptions so that it reads
// no CLOUDFLARE_* environment variable, with the dedicated client and no retries of its own.
//
// What it assumes and a real account must confirm is marked SPIKE.
package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"time"

	cloudflaresdk "github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/billing"
	"github.com/cloudflare/cloudflare-go/v7/option"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// DefaultBaseURL is Cloudflare's API.
const DefaultBaseURL = "https://api.cloudflare.com/client/v4"

// finalGrace is how long after a billing cycle ends the adapter waits before it treats the
// calendar months the cycle covers as closed. SPIKE: Cloudflare generates the invoice at the end of
// the cycle; when the billable usage of a closed cycle stops changing is not documented.
const finalGrace = 2 // days

// Options configure a Source.
type Options struct {
	AccountID string
	Token     string // an API token with Account > Billing > Read
	BaseURL   string // DefaultBaseURL when empty
	// Client is the dedicated client of internal/platform that follows no redirects.
	Client *http.Client
	Now    func() time.Time
}

// Source reads the usage charges of a Cloudflare pay-as-you-go account.
type Source struct {
	o     Options
	usage *billing.UsageService
}

// New returns a Source.
func New(o Options) (*Source, error) {
	if o.AccountID == "" || o.Token == "" || o.Client == nil {
		return nil, errors.New("cloudflare: an account ID, a token, and a client are required")
	}
	if o.BaseURL == "" {
		o.BaseURL = DefaultBaseURL
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	usage := billing.NewUsageService(
		option.WithBaseURL(o.BaseURL),
		option.WithHTTPClient(o.Client),
		option.WithAPIToken(o.Token),
		option.WithMaxRetries(0), // the job retries throttling and unavailability (ADR-0068 §9)
	)
	return &Source{o: o, usage: usage}, nil
}

// Name implements appfinance.BillingSource.
func (s *Source) Name() string { return "cloudflare" }

// Location implements appfinance.BillingSource: Cloudflare's billing dates are in UTC.
func (s *Source) Location() *time.Location { return time.UTC }

// Method implements appfinance.BillingSource.
func (s *Source) Method() string { return "billable_usage" }

// Fetch implements appfinance.BillingSource. Cloudflare does not bill API requests: Requests
// counts them for the record only, and the import gives the source no cap.
func (s *Source) Fetch(ctx context.Context, req appfinance.FetchRequest) (appfinance.Snapshot, error) {
	var snap appfinance.Snapshot
	if req.MaxRequests <= 0 {
		return snap, appfinance.ErrRequestCap
	}
	if len(req.Months) == 0 {
		return snap, nil
	}
	today := finance.DateOf(s.o.Now().UTC())
	if snap.Requests >= req.MaxRequests {
		return snap, appfinance.ErrRequestCap
	}
	snap.Requests++
	info, err := s.usage.GetAccountUsageInfoV1(ctx, billing.UsageGetAccountUsageInfoV1Params{AccountID: cloudflaresdk.F(s.o.AccountID)})
	if err != nil {
		return appfinance.Snapshot{Requests: snap.Requests}, classify(ctx, err)
	}
	if !info.Covered {
		// SPIKE: what "covered" means is not documented; an Enterprise contract account is the
		// likely case, which the billable usage data does not support.
		return snap, fmt.Errorf("%w: the account is not covered by billable usage", appfinance.ErrBillingNotEnabled)
	}
	from := req.Months[0].Range().Start
	until := req.Months[len(req.Months)-1].Range().End
	if today.Before(until) {
		until = today
	}
	type key struct {
		month   finance.YearMonth
		service string
	}
	sums := map[key]*big.Rat{}
	currency := ""
	through := map[finance.YearMonth]finance.Date{}
	closedThrough := map[finance.YearMonth]bool{}
	seen := map[string]bool{}
	for _, sub := range info.Subscriptions {
		for _, c := range cycles(finance.DateOf(sub.BillingCycleAnchorTimestamp.UTC()), from, until) {
			// A month is closed once every cycle over its last day ended finalGrace days ago.
			for _, m := range req.Months {
				if c.Contains(m.Range().End) {
					closed, ok := closedThrough[m]
					closedThrough[m] = (closed || !ok) && !today.Before(c.End.AddDays(1+finalGrace))
				}
			}
			to := c.End
			if today.Before(to) {
				to = today
			}
			if snap.Requests >= req.MaxRequests {
				return appfinance.Snapshot{Requests: snap.Requests}, appfinance.ErrRequestCap
			}
			snap.Requests++
			records, err := s.usage.GetAccountUsageV1(ctx, billing.UsageGetAccountUsageV1Params{
				AccountID: cloudflaresdk.F(s.o.AccountID),
				From:      cloudflaresdk.F(c.Start.Time()),
				To:        cloudflaresdk.F(to.Time()),
			})
			if err != nil {
				return appfinance.Snapshot{Requests: snap.Requests}, classify(ctx, err)
			}
			for _, r := range *records {
				day := finance.DateOf(r.ChargePeriodStart.UTC())
				// SPIKE: the records are assumed to be one per metric and day, as the dashboard's
				// daily chart and the Version 2 endpoint are. A record over more days cannot be
				// placed in a calendar month without splitting its amount.
				if days := finance.DateOf(r.ChargePeriodEnd.UTC()).Sub(day); days > 1 {
					return appfinance.Snapshot{Requests: snap.Requests}, fmt.Errorf("%w: a record covers %d days", appfinance.ErrInvalidBillingResponse, days)
				}
				id := strings.Join([]string{r.SubscriptionID, r.ServiceName, r.ChargeDescription, r.ZoneID, day.String()}, "|")
				// The SDK decodes the cost into a float64; the raw JSON keeps the decimal unrounded.
				cost := r.JSON.ContractedCost.Raw()
				if seen[id] {
					continue // the cycles of two subscriptions overlap
				}
				seen[id] = true
				m := finance.YearMonthOf(day)
				if !slices.Contains(req.Months, m) {
					continue
				}
				amount, ok := new(big.Rat).SetString(cost)
				if !ok || r.BillingCurrency == "" {
					return appfinance.Snapshot{Requests: snap.Requests}, fmt.Errorf("%w: cost %q %q", appfinance.ErrInvalidBillingResponse, cost, r.BillingCurrency)
				}
				if currency != "" && currency != r.BillingCurrency {
					return appfinance.Snapshot{Requests: snap.Requests}, fmt.Errorf("%w: currencies %s and %s", appfinance.ErrInvalidBillingResponse, currency, r.BillingCurrency)
				}
				currency = r.BillingCurrency
				k := key{m, r.ServiceName}
				if sums[k] == nil {
					sums[k] = new(big.Rat)
				}
				sums[k].Add(sums[k], amount)
				if day.After(through[m]) {
					through[m] = day
				}
			}
		}
	}
	for k, sum := range sums {
		covered := k.month.Range()
		final := closedThrough[k.month]
		if !final {
			covered.End = through[k.month]
		}
		snap.Lines = append(snap.Lines, appfinance.CostLine{
			Month: k.month, Covered: covered, Service: k.service,
			ChargeType: finance.ChargeUsage, Category: categoryOf(k.service),
			Amount: decimal(sum), Currency: currency, Final: final,
		})
	}
	slices.SortFunc(snap.Lines, func(a, b appfinance.CostLine) int {
		if c := a.Month.Compare(b.Month); c != 0 {
			return c
		}
		return strings.Compare(a.Service, b.Service)
	})
	return snap, nil
}

// cycles returns the monthly billing cycles of an anchor date that overlap [from, until]: each
// starts on the anchor's day of the month, or the month's last day when the month is shorter.
// SPIKE: the billing docs say both "every 30 days" and "on the 10th going forward".
func cycles(anchor, from, until finance.Date) []finance.DateRange {
	_, _, day := anchor.Date()
	start := func(m finance.YearMonth) finance.Date {
		r := m.Range()
		if d := r.Start.AddDays(day - 1); !d.After(r.End) {
			return d
		}
		return r.End
	}
	var out []finance.DateRange
	for m := finance.YearMonthOf(from).AddMonths(-1); !start(m).After(until); m = m.AddMonths(1) {
		c := finance.DateRange{Start: start(m), End: start(m.AddMonths(1)).AddDays(-1)}
		if c.Start.Before(anchor) || c.End.Before(from) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// classify maps an error of the SDK onto the failure kinds of ADR-0068 §9.
func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var apiErr *cloudflaresdk.Error
	if !errors.As(err, &apiErr) {
		// A network failure, or a body the SDK could not decode.
		if strings.Contains(err.Error(), "unmarshal") {
			return fmt.Errorf("%w: %v", appfinance.ErrInvalidBillingResponse, err)
		}
		return fmt.Errorf("%w: %v", appfinance.ErrBillingUnavailable, err)
	}
	switch code := apiErr.StatusCode; {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fmt.Errorf("%w: %v", appfinance.ErrBillingPermission, err)
	case code == http.StatusNotFound:
		return fmt.Errorf("%w: %v", appfinance.ErrBillingNotFound, err)
	case code == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %v", appfinance.ErrBillingThrottled, err)
	case code >= 500:
		return fmt.Errorf("%w: %v", appfinance.ErrBillingUnavailable, err)
	default:
		return fmt.Errorf("%w: %v", appfinance.ErrInvalidBillingResponse, err)
	}
}

// decimal formats r in decimal without rounding: the costs are decimals, so r has a finite
// expansion.
func decimal(r *big.Rat) string {
	for prec := 0; prec <= 40; prec++ {
		s := r.FloatString(prec)
		if back, _ := new(big.Rat).SetString(s); back.Cmp(r) == 0 {
			return s
		}
	}
	return r.FloatString(40)
}

// categories maps the ServiceName values seen in the documentation onto ADR-0067 categories.
// SPIKE: the full list of ServiceName values is not published.
var categories = map[string]finance.Category{
	"Workers Standard": finance.CategoryCompute,
	"Workers":          finance.CategoryCompute,
	"Durable Objects":  finance.CategoryCompute,
	"Queues":           finance.CategoryCompute,
	"R2":               finance.CategoryStorage,
	"R2 Storage":       finance.CategoryStorage,
	"Workers KV":       finance.CategoryStorage,
	"Images":           finance.CategoryStorage,
	"Stream":           finance.CategoryStorage,
	"D1":               finance.CategoryDatabase,
	"Hyperdrive":       finance.CategoryDatabase,
	"Argo":             finance.CategoryNetwork,
	"Load Balancing":   finance.CategoryNetwork,
	"Spectrum":         finance.CategoryNetwork,
}

func categoryOf(service string) finance.Category {
	if c, ok := categories[service]; ok {
		return c
	}
	for prefix, c := range categories {
		if strings.HasPrefix(service, prefix+" ") {
			return c
		}
	}
	return finance.CategoryOther
}
