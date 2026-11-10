// SPDX-License-Identifier: AGPL-3.0-only

// Package cloudflare is a spike (#2664) of a BillingSource over Cloudflare's billable usage API
// (GET /accounts/{account_id}/billable-usage, Version 1, Alpha). It is not merged: it checks how
// the API's records map onto the port of ADR-0068 §1 with a small REST client and no SDK.
//
// What it assumes and a real account must confirm is marked SPIKE.
package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

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
type Source struct{ o Options }

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
	return &Source{o: o}, nil
}

// Name implements appfinance.BillingSource.
func (s *Source) Name() string { return "cloudflare" }

// Location implements appfinance.BillingSource: Cloudflare's billing dates are in UTC.
func (s *Source) Location() *time.Location { return time.UTC }

// Method implements appfinance.BillingSource.
func (s *Source) Method() string { return "billable_usage" }

type envelope[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result T `json:"result"`
}

type usageInfo struct {
	Covered       bool `json:"covered"`
	Subscriptions []struct {
		ID     string    `json:"id"`
		Anchor time.Time `json:"billing_cycle_anchor_timestamp"`
		Start  time.Time `json:"start_timestamp"`
		End    time.Time `json:"end_timestamp"`
	} `json:"subscriptions"`
}

type usageRecord struct {
	ServiceName       string      `json:"ServiceName"`
	ServiceFamilyName string      `json:"ServiceFamilyName"`
	ChargeDescription string      `json:"ChargeDescription"`
	SubscriptionID    string      `json:"SubscriptionId"`
	ZoneID            string      `json:"ZoneId"`
	ChargePeriodStart time.Time   `json:"ChargePeriodStart"`
	ChargePeriodEnd   time.Time   `json:"ChargePeriodEnd"`
	ContractedCost    json.Number `json:"ContractedCost"`
	BillingCurrency   string      `json:"BillingCurrency"`
}

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
	var info usageInfo
	if err := s.get(ctx, &snap, req.MaxRequests, "billable-usage/info", nil, &info); err != nil {
		return appfinance.Snapshot{Requests: snap.Requests}, err
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
		for _, c := range cycles(finance.DateOf(sub.Anchor.UTC()), from, until) {
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
			q := url.Values{"from": {c.Start.String()}, "to": {to.String()}}
			var records []usageRecord
			if err := s.get(ctx, &snap, req.MaxRequests, "billable-usage", q, &records); err != nil {
				return appfinance.Snapshot{Requests: snap.Requests}, err
			}
			for _, r := range records {
				day := finance.DateOf(r.ChargePeriodStart.UTC())
				// SPIKE: the records are assumed to be one per metric and day, as the dashboard's
				// daily chart and the Version 2 endpoint are. A record over more days cannot be
				// placed in a calendar month without splitting its amount.
				if days := finance.DateOf(r.ChargePeriodEnd.UTC()).Sub(day); days > 1 {
					return appfinance.Snapshot{Requests: snap.Requests}, fmt.Errorf("%w: a record covers %d days", appfinance.ErrInvalidBillingResponse, days)
				}
				id := strings.Join([]string{r.SubscriptionID, r.ServiceName, r.ChargeDescription, r.ZoneID, day.String()}, "|")
				if seen[id] {
					continue // the cycles of two subscriptions overlap
				}
				seen[id] = true
				m := finance.YearMonthOf(day)
				if !slices.Contains(req.Months, m) {
					continue
				}
				amount, ok := new(big.Rat).SetString(r.ContractedCost.String())
				if !ok || r.BillingCurrency == "" {
					return appfinance.Snapshot{Requests: snap.Requests}, fmt.Errorf("%w: cost %q %q", appfinance.ErrInvalidBillingResponse, r.ContractedCost, r.BillingCurrency)
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

// get calls one endpoint of the account, counting it against max, and decodes its result.
func (s *Source) get(ctx context.Context, snap *appfinance.Snapshot, max int, path string, q url.Values, out any) error {
	if snap.Requests >= max {
		return appfinance.ErrRequestCap
	}
	u := s.o.BaseURL + "/accounts/" + url.PathEscape(s.o.AccountID) + "/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.o.Token)
	snap.Requests++
	res, err := s.o.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", appfinance.ErrBillingUnavailable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", appfinance.ErrBillingUnavailable, err)
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s: %s", appfinance.ErrBillingPermission, res.Status, snippet(body))
	case res.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s: %s", appfinance.ErrBillingNotFound, res.Status, snippet(body))
	case res.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", appfinance.ErrBillingThrottled, res.Status)
	case res.StatusCode >= 500:
		return fmt.Errorf("%w: %s", appfinance.ErrBillingUnavailable, res.Status)
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: %s: %s", appfinance.ErrInvalidBillingResponse, res.Status, snippet(body))
	}
	env := envelope[json.RawMessage]{}
	if err := json.Unmarshal(body, &env); err != nil || !env.Success {
		return fmt.Errorf("%w: %s", appfinance.ErrInvalidBillingResponse, snippet(body))
	}
	dec := json.NewDecoder(strings.NewReader(string(env.Result)))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%w: %v", appfinance.ErrInvalidBillingResponse, err)
	}
	return nil
}

func snippet(b []byte) string { return string(b[:min(len(b), 200)]) }

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
