// SPDX-License-Identifier: AGPL-3.0-only

package cloudflare_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	billingcloudflare "github.com/mycroblog/my/internal/adapter/billing/cloudflare"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit/financetest"
)

const account = "023e105f4ecef8ad9ca31a8372d0c353"

// subscription is one subscription of the fake's info: its start and its first renewal.
type subscription struct {
	id            string
	start, anchor string // RFC 3339
}

// record is one daily record of the fake, shaped like the responses of a real account.
type record struct {
	day          string // YYYY-MM-DD
	days         int    // the record's length in days; 0 means 1
	family       string // ServiceFamilyName
	metric       string // ServiceName; empty means "<family> metric"
	cost         string // the JSON number, verbatim
	currency     string // empty means USD
	subscription string
}

// fakeAPI serves billable-usage/info and billable-usage of one account as a real account does: a
// range returns the records from the first day of a cycle inside it, and its `to` is excluded.
type fakeAPI struct {
	t       *testing.T
	covered bool
	subs    []subscription
	records []record
	// fail, when not 0, is the HTTP status of every answer; failCode is the code of its error.
	fail     int
	failCode int
	// garbled answers the usage with a body that is not JSON.
	garbled bool

	mu      sync.Mutex
	queries []string
	tokens  []string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append(f.tokens, r.Header.Get("Authorization"))
	w.Header().Set("Content-Type", "application/json")
	if f.fail != 0 {
		w.WriteHeader(f.fail)
		fmt.Fprintf(w, `{"success":false,"errors":[{"code":%d,"message":"failed"}],"messages":[],"result":null}`, f.failCode)
		return
	}
	switch r.URL.Path {
	case "/accounts/" + account + "/billable-usage/info":
		var subs []string
		for _, s := range f.subs {
			subs = append(subs, fmt.Sprintf(`{"id":%q,"billing_cycle_anchor_timestamp":%q,"start_timestamp":%q,"end_timestamp":null}`, s.id, s.anchor, s.start))
		}
		fmt.Fprintf(w, `{"success":true,"errors":[],"messages":[],"result":{"covered":%v,"subscriptions":[%s]}}`, f.covered, strings.Join(subs, ","))
	case "/accounts/" + account + "/billable-usage":
		if f.garbled {
			fmt.Fprint(w, `{"success":true,"result":[{"ContractedCost":`)
			return
		}
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		f.queries = append(f.queries, from+".."+to)
		if to <= from {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"success":false,"errors":[{"code":1182,"message":"gtfield"}],"messages":[],"result":null}`)
			return
		}
		var out []string
		for _, rec := range f.records {
			if rec.day >= from && rec.day < to && f.cycleStartsIn(from, to, rec.day) {
				out = append(out, rec.json())
			}
		}
		fmt.Fprintf(w, `{"success":true,"errors":[],"messages":[],"result":[%s]}`, strings.Join(out, ","))
	default:
		http.NotFound(w, r)
	}
}

// cycleStartsIn reports whether a cycle of a subscription starts in [from, day]: a real account
// returns nothing before the first cycle start inside the range.
func (f *fakeAPI) cycleStartsIn(from, to, day string) bool {
	for _, s := range f.subs {
		anchor, _ := time.Parse(time.RFC3339, s.anchor)
		start, _ := time.Parse(time.RFC3339, s.start)
		d, _ := time.Parse(time.DateOnly, from)
		for ; d.Format(time.DateOnly) < to; d = d.AddDate(0, 0, 1) {
			ds := d.Format(time.DateOnly)
			if ds > day {
				break
			}
			// The cycles start on the later of the start's and the anchor's day, or on the last
			// day of a shorter month.
			lastDay := time.Date(d.Year(), d.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
			if ds == start.Format(time.DateOnly) || d.Day() == min(max(start.Day(), anchor.Day()), lastDay) {
				return true
			}
		}
	}
	return false
}

func (r record) json() string {
	start, _ := time.Parse(time.DateOnly, r.day)
	end := start.AddDate(0, 0, max(1, r.days))
	metric := r.metric
	if metric == "" && r.family != "" {
		metric = r.family + " metric (first units included)"
	}
	currency := r.currency
	if currency == "" {
		currency = "USD"
	}
	sub := r.subscription
	if sub == "" {
		sub = "sub-1"
	}
	b, _ := json.Marshal(map[string]any{
		"BillingAccountId": account, "BillingCurrency": currency, "BillingPeriodStart": start.Format(time.RFC3339),
		"ChargeCategory": "Usage", "ChargeClass": nil, "ChargeDescription": metric + " usage measured in Count",
		"ChargePeriodStart": start.Format(time.RFC3339), "ChargePeriodEnd": end.Format(time.RFC3339),
		"ConsumedQuantity": 1, "ConsumedUnit": "Count", "PricingQuantity": 0, "PricingUnit": "Count",
		"ServiceName": metric, "ServiceFamilyName": r.family, "SubscriptionId": sub,
		"BilledCost": json.RawMessage(r.cost), "ContractedCost": json.RawMessage(r.cost),
		"EffectiveCost": json.RawMessage(r.cost), "ListCost": json.RawMessage(r.cost),
	})
	return string(b)
}

func newSource(t *testing.T, f *fakeAPI, now time.Time) *billingcloudflare.Source {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	src, err := billingcloudflare.New(billingcloudflare.Options{
		AccountID: account, Token: "billing-read-token", BaseURL: srv.URL, Client: srv.Client(),
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func months(t *testing.T, ss ...string) []finance.YearMonth {
	t.Helper()
	var out []finance.YearMonth
	for _, s := range ss {
		m, err := finance.ParseYearMonth(s)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func date(t *testing.T, s string) finance.Date {
	t.Helper()
	d, err := finance.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// summary is a line in a comparable form.
func summary(snap appfinance.Snapshot) []string {
	var out []string
	for _, l := range snap.Lines {
		out = append(out, fmt.Sprintf("%s %s %s %s %s %s final=%v", l.Month, l.Service, l.Category, l.Covered, l.Amount, l.Currency, l.Final))
	}
	return out
}

// TestBillingSourceContract runs the contract of a source of usage alone over a subscription whose
// cycles are calendar months, so that two months take one request each after the info.
func TestBillingSourceContract(t *testing.T) {
	financetest.BillingUsageSourceContract(t, func(t *testing.T, s financetest.BillingScenario) appfinance.BillingSource {
		f := &fakeAPI{t: t, covered: true, subs: []subscription{{id: "sub-1", start: "2025-01-01T08:00:00Z", anchor: "2025-02-01T00:00:00Z"}}}
		switch s.Failure {
		case "":
		case appfinance.ImportErrorPermission:
			f.fail, f.failCode = http.StatusForbidden, 10000
		case appfinance.ImportErrorNotEnabled:
			f.covered = false
		case appfinance.ImportErrorNotFound:
			f.fail, f.failCode = http.StatusNotFound, 7003
		case appfinance.ImportErrorThrottled:
			f.fail, f.failCode = http.StatusTooManyRequests, 971
		case appfinance.ImportErrorInvalidResponse:
			f.garbled = true
		case appfinance.ImportErrorUnavailable:
			f.fail, f.failCode = http.StatusServiceUnavailable, 1000
		default:
			t.Skipf("Cloudflare has no %s", s.Failure)
		}
		var last finance.Date
		for _, m := range s.Months {
			// The rows of a final month go on its 15th; those of the open month on the last day
			// with data, which the covered days of its lines must end on.
			day := m.Month.Range().Start.AddDays(14)
			if !m.Final {
				day = m.Through
			}
			if day.After(last) {
				last = day
			}
			// Each row is a metric of its own, as the records of one day are.
			for i, r := range m.Rows {
				f.records = append(f.records, record{day: day.String(), family: r.Service, metric: fmt.Sprintf("%s metric %d", r.Service, i), cost: r.Amount, currency: r.Currency})
			}
		}
		// The day after the open month's last day with data, as the import runs.
		return newSource(t, f, last.AddDays(1).Time().Add(9*time.Hour))
	})
}

// TestFetchSumsDailyRecordsByProductIntoCalendarMonths reads two cycles anchored on the 9th, as on
// a real account: the products are ServiceFamilyName, the amounts unrounded, the cycles fetched
// with `to` on the next cycle's first day, and the previous month final two days after the cycle
// over its last day ends.
func TestFetchSumsDailyRecordsByProductIntoCalendarMonths(t *testing.T) {
	t.Parallel()
	f := &fakeAPI{covered: true, subs: []subscription{{id: "sub-1", start: "2025-09-09T09:55:13Z", anchor: "2025-10-09T00:00:00Z"}}, records: []record{
		{day: "2026-09-30", family: "R2", cost: "9.99"}, // a month not asked for
		{day: "2026-10-09", family: "Workers", metric: "Workers Standard Requests (first 10M are included)", cost: "0.75"},
		{day: "2026-10-09", family: "Workers", metric: "Workers CPU ms (first 30M are included)", cost: "0.000005"},
		{day: "2026-10-31", family: "R2", metric: "R2 Data Storage (First 10GB-Month included)", cost: "0.015"},
		{day: "2026-11-08", family: "Workers", cost: "0.25"}, // the last day of the cycle
		{day: "2026-11-11", family: "D1", cost: "0"},         // free-tier usage
		{day: "2026-11-11", family: "Mystery", cost: "1e-3"},
	}}
	now := time.Date(2026, time.November, 12, 3, 0, 0, 0, time.UTC)
	snap, err := newSource(t, f, now).Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-10", "2026-11"), MaxRequests: 10})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026-09-09..2026-10-09", "2026-10-09..2026-11-09", "2026-11-09..2026-11-13"}; !slices.Equal(f.queries, want) || snap.Requests != 4 {
		t.Errorf("queries %v, %d requests; want %v and 4", f.queries, snap.Requests, want)
	}
	want := []string{
		"2026-10 R2 storage 2026-10-01/2026-10-31 0.015 USD final=true",
		"2026-10 Workers compute 2026-10-01/2026-10-31 0.750005 USD final=true",
		"2026-11 D1 database 2026-11-01/2026-11-11 0 USD final=false",
		"2026-11 Mystery other 2026-11-01/2026-11-11 0.001 USD final=false",
		"2026-11 Workers compute 2026-11-01/2026-11-11 0.25 USD final=false",
	}
	if got := summary(snap); !slices.Equal(got, want) {
		t.Errorf("lines\n got %q\nwant %q", got, want)
	}
	for _, auth := range f.tokens {
		if auth != "Bearer billing-read-token" {
			t.Errorf("Authorization = %q, want the configured token", auth)
		}
	}
}

// TestFetchClosesAMonth checks the rule of a final month: two days after the cycle over its last
// day ends, or after the end of the next month for a cycle late in the month.
func TestFetchClosesAMonth(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, start, anchor, now string
		final                    bool
	}{
		{"before the grace", "2025-09-09T00:00:00Z", "2025-10-09T00:00:00Z", "2026-11-10T23:00:00Z", false},
		{"after the grace", "2025-09-09T00:00:00Z", "2025-10-09T00:00:00Z", "2026-11-11T00:00:00Z", true},
		{"a cycle late in the month, during the next month", "2025-01-31T00:00:00Z", "2025-02-28T00:00:00Z", "2026-11-30T12:00:00Z", false},
		{"a cycle late in the month, after the next month", "2025-01-31T00:00:00Z", "2025-02-28T00:00:00Z", "2026-12-01T00:00:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeAPI{covered: true, subs: []subscription{{id: "sub-1", start: tc.start, anchor: tc.anchor}}, records: []record{
				{day: "2026-10-20", family: "R2", cost: "1.5"},
			}}
			now, _ := time.Parse(time.RFC3339, tc.now)
			snap, err := newSource(t, f, now).Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-10"), MaxRequests: 10})
			if err != nil || len(snap.Lines) != 1 {
				t.Fatalf("Fetch = %+v, %v", snap.Lines, err)
			}
			l := snap.Lines[0]
			wantCovered := "2026-10-01/2026-10-20"
			if tc.final {
				wantCovered = "2026-10-01/2026-10-31"
			}
			if l.Final != tc.final || l.Covered.String() != wantCovered {
				t.Errorf("final %v over %s, want %v over %s", l.Final, l.Covered, tc.final, wantCovered)
			}
		})
	}
}

func TestCycles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		start, anchor, from, until string
		want                       []string
	}{
		{"the 9th", "2025-09-09", "2025-10-09", "2026-10-01", "2026-11-30",
			[]string{"2026-09-09/2026-10-08", "2026-10-09/2026-11-08", "2026-11-09/2026-12-08"}},
		{"the first cycle starts on the subscription's start", "2026-10-29", "2026-11-29", "2026-10-01", "2026-11-30",
			[]string{"2026-10-29/2026-11-28", "2026-11-29/2026-12-28"}},
		{"the 31st in short months", "2025-01-31", "2025-02-28", "2026-02-01", "2026-03-31",
			[]string{"2026-01-31/2026-02-27", "2026-02-28/2026-03-30", "2026-03-31/2026-04-29"}},
		{"nothing before the start", "2026-12-05", "2027-01-05", "2026-10-01", "2026-11-30", nil},
		{"an empty range", "2025-09-09", "2025-10-09", "2026-11-01", "2026-10-31", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, c := range billingcloudflare.Cycles(date(t, tc.start), date(t, tc.anchor), date(t, tc.from), date(t, tc.until)) {
				got = append(got, c.String())
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Cycles = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFetchReadsEachDayOnce reads two subscriptions whose cycles overlap: a record answered by
// both requests counts once.
func TestFetchReadsEachDayOnce(t *testing.T) {
	t.Parallel()
	f := &fakeAPI{covered: true, subs: []subscription{
		{id: "sub-1", start: "2025-09-09T00:00:00Z", anchor: "2025-10-09T00:00:00Z"},
		{id: "sub-2", start: "2025-09-20T00:00:00Z", anchor: "2025-10-20T00:00:00Z"},
	}, records: []record{{day: "2026-11-21", family: "R2", cost: "0.5"}}}
	now := time.Date(2026, time.November, 25, 0, 0, 0, 0, time.UTC)
	snap, err := newSource(t, f, now).Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-11"), MaxRequests: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := summary(snap); len(got) != 1 || !strings.Contains(got[0], " 0.5 USD ") {
		t.Errorf("lines %q, want R2 once at 0.5", got)
	}
}

func TestFetchRefusesWhatItCannotPlace(t *testing.T) {
	t.Parallel()
	sub := []subscription{{id: "sub-1", start: "2025-09-09T00:00:00Z", anchor: "2025-10-09T00:00:00Z"}}
	now := time.Date(2026, time.November, 25, 0, 0, 0, 0, time.UTC)
	for name, recs := range map[string][]record{
		"a record over two days": {{day: "2026-11-10", days: 2, family: "R2", cost: "1"}},
		"two currencies":         {{day: "2026-11-10", family: "R2", cost: "1"}, {day: "2026-11-11", family: "R2", cost: "1", currency: "EUR"}},
		"no product":             {{day: "2026-11-10", cost: "1"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap, err := newSource(t, &fakeAPI{covered: true, subs: sub, records: recs}, now).
				Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-11"), MaxRequests: 10})
			if appfinance.ImportErrorKindOf(err) != appfinance.ImportErrorInvalidResponse || len(snap.Lines) != 0 {
				t.Errorf("Fetch = %d lines, %v; want invalid_response and none", len(snap.Lines), err)
			}
		})
	}
}

// TestFetchClassifiesAuthenticationFailures checks the answers of a real account: an invalid token
// is a 400 with code 9106, and an account the token cannot read a 403 with code 10000.
func TestFetchClassifiesAuthenticationFailures(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]*fakeAPI{
		"an invalid token":   {fail: http.StatusBadRequest, failCode: 9106},
		"an unknown account": {fail: http.StatusForbidden, failCode: 10000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := newSource(t, f, time.Now()).Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-11"), MaxRequests: 10})
			if got := appfinance.ImportErrorKindOf(err); got != appfinance.ImportErrorPermission {
				t.Errorf("Fetch = %v, classified %s, want permission", err, got)
			}
			if len(f.tokens) != 1 {
				t.Errorf("%d calls, want 1: the SDK must not retry", len(f.tokens))
			}
		})
	}
}

// TestNewReadsNoEnvironment sets the variables that the SDK's DefaultClientOptions read: the
// source still calls its own address with its own token.
func TestNewReadsNoEnvironment(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("the source called CLOUDFLARE_BASE_URL")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(other.Close)
	t.Setenv("CLOUDFLARE_BASE_URL", other.URL)
	t.Setenv("CLOUDFLARE_API_TOKEN", "a-token-that-edits-dns")
	t.Setenv("CLOUDFLARE_CUSTOM_HEADERS", "X-Leak: yes")
	f := &fakeAPI{covered: true, subs: []subscription{{id: "sub-1", start: "2025-09-09T00:00:00Z", anchor: "2025-10-09T00:00:00Z"}}}
	if _, err := newSource(t, f, time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC)).
		Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-11"), MaxRequests: 10}); err != nil {
		t.Fatal(err)
	}
	for _, auth := range f.tokens {
		if auth != "Bearer billing-read-token" {
			t.Errorf("Authorization = %q, want the configured token", auth)
		}
	}
}

func TestNewChecksItsOptions(t *testing.T) {
	t.Parallel()
	client := &http.Client{}
	for name, o := range map[string]billingcloudflare.Options{
		"no account":       {Token: "t", Client: client},
		"a bad account ID": {AccountID: "not-an-id", Token: "t", Client: client},
		"no token":         {AccountID: account, Client: client},
		"no client":        {AccountID: account, Token: "t"},
	} {
		if _, err := billingcloudflare.New(o); err == nil {
			t.Errorf("%s: New = nil error", name)
		}
	}
	if err := billingcloudflare.CheckAccountID(strings.ToUpper(account)); err == nil {
		t.Error("CheckAccountID accepted uppercase digits")
	}
}
