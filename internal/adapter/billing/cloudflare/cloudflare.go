// SPDX-License-Identifier: AGPL-3.0-only

// Package cloudflare is the finance.BillingSource adapter of Cloudflare (ADR-0068 §11). It reads the
// usage charges of a pay-as-you-go account from the billable usage API (Version 1) through the
// official SDK, and sums its daily records by product into UTC calendar months. Fixed-fee
// subscriptions, taxes, and credits are not in that API, and an Enterprise contract account is
// not covered.
//
// Cloudflare bills in cycles that start on the same day of every month, from the first purchase
// of the account, in UTC. The API answers one cycle at a time, so a fetch asks for each cycle that
// overlaps its months, and a month is final once every cycle over its last day ended FinalGrace
// days ago.
package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	cloudflaresdk "github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/billing"
	"github.com/cloudflare/cloudflare-go/v7/option"
	"github.com/cloudflare/cloudflare-go/v7/shared"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// Name is the source's identifier of billing_sources.
const Name = "cloudflare"

// BaseURL is Cloudflare's API.
const BaseURL = "https://api.cloudflare.com/client/v4/"

// FinalGrace is how many days after a billing cycle ends a calendar month whose last day the cycle
// holds is final. Cloudflare issues the invoice when the cycle ends and its records carry no mark
// of their own; 2 days is provisional (ADR-0068 §11).
const FinalGrace = 2

// The error codes of Cloudflare's API that the adapter tells apart.
const (
	// codeInvalidToken comes with HTTP 400 when the token is not valid.
	codeInvalidToken = 9106
)

// accountIDRe is an account ID of Cloudflare: 32 hexadecimal digits.
var accountIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// CheckAccountID reports whether v is an account ID of Cloudflare (billing_cloudflare_account_id).
func CheckAccountID(v string) error {
	if !accountIDRe.MatchString(v) {
		return errors.New("not a Cloudflare account ID (32 lowercase hexadecimal digits, from the account's overview in the dashboard)")
	}
	return nil
}

// Options configure a Source.
type Options struct {
	// AccountID is the account whose charges are read (billing_cloudflare_account_id).
	AccountID string
	// Token is an API token of the account with the permission Billing Read
	// (billing_cloudflare_api_token).
	Token string
	// Client is the HTTP client of the API calls: the dedicated client of internal/platform that
	// follows no redirects.
	Client *http.Client
	// BaseURL replaces the API's address in tests; empty means BaseURL.
	BaseURL string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Source is the BillingSource of billing_sources = cloudflare.
type Source struct {
	accountID string
	usage     *billing.UsageService
	now       func() time.Time
}

// New returns the Source of o. The SDK's service is built from o alone, never from
// DefaultClientOptions, which would read CLOUDFLARE_API_TOKEN, CLOUDFLARE_BASE_URL, and the other
// CLOUDFLARE_* variables of the environment.
func New(o Options) (*Source, error) {
	if err := CheckAccountID(o.AccountID); err != nil {
		return nil, fmt.Errorf("billing cloudflare: %w", err)
	}
	if o.Token == "" {
		return nil, errors.New("billing cloudflare: an API token is required")
	}
	if o.Client == nil {
		return nil, errors.New("billing cloudflare: an HTTP client is required")
	}
	base := o.BaseURL
	if base == "" {
		base = BaseURL
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	usage := billing.NewUsageService(
		option.WithBaseURL(base),
		option.WithHTTPClient(o.Client),
		option.WithAPIToken(o.Token),
		// The job retries throttling and unavailability (ADR-0068 §9).
		option.WithMaxRetries(0),
	)
	return &Source{accountID: o.AccountID, usage: usage, now: now}, nil
}

// Name implements finance.BillingSource.
func (s *Source) Name() string { return Name }

// Location implements finance.BillingSource: Cloudflare's billing dates are in UTC.
func (s *Source) Location() *time.Location { return time.UTC }

// Method implements finance.BillingSource.
func (s *Source) Method() string { return appfinance.MethodBillableUsage }

// lineKey is the key of a line: a month and a product.
type lineKey struct {
	month   finance.YearMonth
	product string
}

// Fetch implements finance.BillingSource. Cloudflare does not bill its API: Requests counts the
// calls for the record, and the import gives the source no cap of its own.
func (s *Source) Fetch(ctx context.Context, req appfinance.FetchRequest) (appfinance.Snapshot, error) {
	var snap appfinance.Snapshot
	if req.MaxRequests <= 0 {
		return snap, appfinance.ErrRequestCap
	}
	if err := ctx.Err(); err != nil {
		return snap, err
	}
	if len(req.Months) == 0 {
		return snap, nil
	}
	months := slices.Clone(req.Months)
	slices.SortFunc(months, finance.YearMonth.Compare)
	today := finance.DateOf(s.now().UTC())

	if err := s.spend(&snap, req.MaxRequests); err != nil {
		return snap, err
	}
	info, err := s.usage.GetAccountUsageInfoV1(ctx, billing.UsageGetAccountUsageInfoV1Params{AccountID: cloudflaresdk.F(s.accountID)})
	if err != nil {
		return failed(snap), classify(ctx, err)
	}
	if !info.Covered {
		return failed(snap), fmt.Errorf("%w: the account's billable usage is not covered (an Enterprise contract account, or no pay-as-you-go subscription)", appfinance.ErrBillingNotEnabled)
	}

	from := months[0].Range().Start
	until := months[len(months)-1].Range().End
	if today.Before(until) {
		until = today
	}
	sums := map[lineKey]*big.Rat{}
	through := map[finance.YearMonth]finance.Date{}
	closed := map[finance.YearMonth]bool{}
	seen := map[string]bool{}
	currency := ""
	for _, sub := range info.Subscriptions {
		for _, c := range Cycles(finance.DateOf(sub.StartTimestamp.UTC()), finance.DateOf(sub.BillingCycleAnchorTimestamp.UTC()), from, until) {
			for _, m := range months {
				if c.Contains(m.Range().End) {
					was, ok := closed[m]
					closed[m] = (was || !ok) && cycleClosed(c, m, today)
				}
			}
			if err := s.spend(&snap, req.MaxRequests); err != nil {
				return failed(snap), err
			}
			// `to` is excluded: the next cycle's first day, or tomorrow during the cycle.
			to := c.End.AddDays(1)
			if today.Before(c.End) {
				to = today.AddDays(1)
			}
			records, err := s.usage.GetAccountUsageV1(ctx, billing.UsageGetAccountUsageV1Params{
				AccountID: cloudflaresdk.F(s.accountID),
				From:      cloudflaresdk.F(c.Start.Time()),
				To:        cloudflaresdk.F(to.Time()),
			})
			if err != nil {
				return failed(snap), classify(ctx, err)
			}
			for _, r := range *records {
				if err := add(r, months, sums, through, seen, &currency); err != nil {
					return failed(snap), err
				}
			}
		}
	}
	for _, m := range months {
		if _, ok := closed[m]; !ok {
			// No cycle holds the month's last day yet: the month is open.
			closed[m] = false
		}
	}
	for k, sum := range sums {
		covered := k.month.Range()
		final := closed[k.month]
		if !final {
			covered.End = through[k.month]
		}
		snap.Lines = append(snap.Lines, appfinance.CostLine{
			Month: k.month, Covered: covered, Service: k.product,
			ChargeType: finance.ChargeUsage, Category: categoryOf(k.product),
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

// spend counts one request against limit, or fails with ErrRequestCap when none is left.
func (s *Source) spend(snap *appfinance.Snapshot, limit int) error {
	if snap.Requests >= limit {
		return appfinance.ErrRequestCap
	}
	snap.Requests++
	return nil
}

// failed is a snapshot of a failed fetch: the requests made, and no line.
func failed(snap appfinance.Snapshot) appfinance.Snapshot {
	return appfinance.Snapshot{Requests: snap.Requests}
}

// add sums one record into its month and product. The SDK decodes the cost into a float64, so the
// amount comes from the response's own JSON, unrounded (ADR-0068 §1).
func add(r billing.UsageGetAccountUsageV1Response, months []finance.YearMonth, sums map[lineKey]*big.Rat,
	through map[finance.YearMonth]finance.Date, seen map[string]bool, currency *string,
) error {
	if r.JSON.ChargePeriodStart.IsMissing() || r.JSON.ChargePeriodEnd.IsMissing() || r.JSON.ContractedCost.IsNull() {
		return fmt.Errorf("%w: a record without its charge period or cost", appfinance.ErrInvalidBillingResponse)
	}
	day := finance.DateOf(r.ChargePeriodStart.UTC())
	// Each record is one metric of one day, 24 hours; one over more days has no single month.
	if days := finance.DateOf(r.ChargePeriodEnd.UTC()).Sub(day); days > 1 {
		return fmt.Errorf("%w: a record covers %d days", appfinance.ErrInvalidBillingResponse, days)
	}
	// ServiceFamilyName is the product (R2, D1, Workers); ServiceName is the metric, such as
	// "R2 Data Storage (First 10GB-Month included)".
	product := strings.TrimSpace(r.ServiceFamilyName)
	if product == "" {
		product = strings.TrimSpace(r.ServiceName)
	}
	if product == "" {
		return fmt.Errorf("%w: a record without a product", appfinance.ErrInvalidBillingResponse)
	}
	id := strings.Join([]string{r.SubscriptionID, r.ServiceName, r.ChargeDescription, day.String()}, "|")
	if seen[id] {
		return nil // the cycles of two subscriptions overlap
	}
	seen[id] = true
	m := finance.YearMonthOf(day)
	if !slices.Contains(months, m) {
		return nil
	}
	cost := r.JSON.ContractedCost.Raw()
	amount, ok := new(big.Rat).SetString(cost)
	code := strings.ToUpper(strings.TrimSpace(r.BillingCurrency))
	if !ok || len(code) != 3 {
		return fmt.Errorf("%w: cost %q in %q", appfinance.ErrInvalidBillingResponse, cost, r.BillingCurrency)
	}
	if *currency != "" && *currency != code {
		return fmt.Errorf("%w: the currencies %s and %s", appfinance.ErrInvalidBillingResponse, *currency, code)
	}
	*currency = code
	k := lineKey{m, product}
	if sums[k] == nil {
		sums[k] = new(big.Rat)
	}
	sums[k].Add(sums[k], amount)
	if day.After(through[m]) {
		through[m] = day
	}
	return nil
}

// cycleClosed reports whether month m, whose last day cycle c holds, is final on today: FinalGrace
// days after the cycle ends, or after the end of the next month, whichever comes first, so that a
// cycle late in the month still closes the month before the import stops fetching it (ADR-0068
// §2, §11).
func cycleClosed(c finance.DateRange, m finance.YearMonth, today finance.Date) bool {
	return !today.Before(c.End.AddDays(1+FinalGrace)) || today.After(m.AddMonths(1).Range().End)
}

// Cycles returns the monthly billing cycles of a subscription that overlap [from, until]. The first
// starts on the subscription's start; each later one on the day of the month of the anchor
// (billing_cycle_anchor_timestamp, the first renewal), or on the month's last day when the month is
// shorter. A start on the 31st has its first renewal on the 28th of February, so the day is the
// later of the start's and the anchor's.
func Cycles(start, anchor, from, until finance.Date) []finance.DateRange {
	if until.Before(from) {
		return nil
	}
	_, _, startDay := start.Date()
	_, _, anchorDay := anchor.Date()
	day := max(startDay, anchorDay)
	first := func(m finance.YearMonth) finance.Date {
		r := m.Range()
		if d := r.Start.AddDays(day - 1); !d.After(r.End) {
			return d
		}
		return r.End
	}
	var out []finance.DateRange
	for m := finance.YearMonthOf(from).AddMonths(-1); !first(m).After(until); m = m.AddMonths(1) {
		c := finance.DateRange{Start: first(m), End: first(m.AddMonths(1)).AddDays(-1)}
		if c.End.Before(start) || c.End.Before(from) {
			continue
		}
		if c.Start.Before(start) {
			c.Start = start
		}
		out = append(out, c)
	}
	return out
}

// classify maps an error of the SDK onto the failure kinds of ADR-0068 §9.
func classify(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var apiErr *cloudflaresdk.Error
	if !errors.As(err, &apiErr) {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return fmt.Errorf("%w: %w", appfinance.ErrBillingUnavailable, err)
		}
		// The SDK could not read the response.
		return fmt.Errorf("%w: %w", appfinance.ErrInvalidBillingResponse, err)
	}
	invalidToken := slices.ContainsFunc(apiErr.Errors, func(e shared.ErrorData) bool { return e.Code == codeInvalidToken })
	switch code := apiErr.StatusCode; {
	// An invalid token is a 400 with code 9106; an account the token cannot read, or that does not
	// exist, is a 403 with code 10000.
	case invalidToken || code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fmt.Errorf("%w: %w", appfinance.ErrBillingPermission, err)
	case code == http.StatusNotFound:
		return fmt.Errorf("%w: %w", appfinance.ErrBillingNotFound, err)
	case code == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %w", appfinance.ErrBillingThrottled, err)
	case code >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %w", appfinance.ErrBillingUnavailable, err)
	default:
		return fmt.Errorf("%w: %w", appfinance.ErrInvalidBillingResponse, err)
	}
}

// decimal formats r in decimal without rounding: the costs are decimals, so r has a finite
// expansion.
func decimal(r *big.Rat) string {
	for prec := 0; prec <= 40; prec++ {
		s := r.FloatString(prec)
		if back, ok := new(big.Rat).SetString(s); ok && back.Cmp(r) == 0 {
			return s
		}
	}
	return r.FloatString(40)
}

// categories maps the products of ServiceFamilyName onto the categories of ADR-0067; the others are
// finance.CategoryOther. R2, D1, and Workers are the names a real account returns; Cloudflare
// publishes no full list.
var categories = map[string]finance.Category{
	"workers":            finance.CategoryCompute,
	"durable objects":    finance.CategoryCompute,
	"queues":             finance.CategoryCompute,
	"workers ai":         finance.CategoryCompute,
	"browser rendering":  finance.CategoryCompute,
	"r2":                 finance.CategoryStorage,
	"workers kv":         finance.CategoryStorage,
	"kv":                 finance.CategoryStorage,
	"images":             finance.CategoryStorage,
	"stream":             finance.CategoryStorage,
	"d1":                 finance.CategoryDatabase,
	"hyperdrive":         finance.CategoryDatabase,
	"argo":               finance.CategoryNetwork,
	"argo smart routing": finance.CategoryNetwork,
	"load balancing":     finance.CategoryNetwork,
	"spectrum":           finance.CategoryNetwork,
}

func categoryOf(product string) finance.Category {
	if c, ok := categories[strings.ToLower(product)]; ok {
		return c
	}
	return finance.CategoryOther
}
