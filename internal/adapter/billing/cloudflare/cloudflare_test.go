// SPDX-License-Identifier: AGPL-3.0-only

package cloudflare_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/billing/cloudflare"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// A pay-as-you-go account whose cycle is anchored on the 10th, read on 2026-11-20: the months
// asked for are 2026-10 and 2026-11. The cycle 2026-10-10..2026-11-09 holds 2026-10-31 and ended
// on 2026-11-09, so 2026-10 is closed; 2026-11 is open through the 19th.
var now = time.Date(2026, time.November, 20, 3, 0, 0, 0, time.UTC)

// record is shaped like the 200 example of "Get Account Billable Usage (Version 1, Alpha)".
func record(day, service, cost string) string {
	start, _ := time.Parse(time.DateOnly, day)
	return fmt.Sprintf(`{"BilledCost":%[3]s,"BillingAccountId":"023e105f4ecef8ad9ca31a8372d0c353","BillingCurrency":"USD",
"BillingPeriodStart":"2026-10-10T00:00:00Z","ChargeCategory":"Usage","ChargeClass":null,
"ChargeDescription":"%[2]s usage","ChargePeriodStart":"%[1]s","ChargePeriodEnd":"%[4]s",
"ConsumedQuantity":1,"ConsumedUnit":"Count","ContractedCost":%[3]s,"EffectiveCost":%[3]s,"ListCost":%[3]s,
"ServiceName":"%[2]s","SubscriptionId":"3F3CD4CQ6N7FXO7IK6NVFJBOYA"}`,
		start.Format(time.RFC3339), service, cost, start.AddDate(0, 0, 1).Format(time.RFC3339))
}

type fake struct {
	mu      sync.Mutex
	queries []string
	status  int
	covered bool
	days    map[string][]string // date -> records
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != 0 {
		w.WriteHeader(f.status)
		fmt.Fprint(w, `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}],"result":null}`)
		return
	}
	switch r.URL.Path {
	case "/accounts/acc/billable-usage/info":
		fmt.Fprintf(w, `{"success":true,"errors":[],"messages":[],"result":{"covered":%v,"subscriptions":[
{"id":"3F3CD4CQ6N7FXO7IK6NVFJBOYA","billing_cycle_anchor_timestamp":"2025-01-10T00:00:00Z","start_timestamp":"2025-01-10T00:00:00Z"}]}}`, f.covered)
	case "/accounts/acc/billable-usage":
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		f.queries = append(f.queries, from+".."+to)
		var out []string
		for day, recs := range f.days {
			if day >= from && day <= to {
				out = append(out, recs...)
			}
		}
		fmt.Fprintf(w, `{"success":true,"errors":[],"messages":[],"result":[%s]}`, strings.Join(out, ","))
	default:
		http.NotFound(w, r)
	}
}

func newSource(t *testing.T, f *fake) *cloudflare.Source {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	src, err := cloudflare.New(cloudflare.Options{
		AccountID: "acc", Token: "token", BaseURL: srv.URL, Client: srv.Client(),
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

func TestFetchBucketsDaysIntoCalendarMonths(t *testing.T) {
	f := &fake{covered: true, days: map[string][]string{
		"2026-09-30": {record("2026-09-30", "R2 Storage", "9.99")}, // not asked for
		"2026-10-10": {record("2026-10-10", "Workers Standard", "0.75")},
		"2026-10-31": {record("2026-10-31", "Workers Standard", "0.000005"), record("2026-10-31", "R2 Storage", "0.015")},
		"2026-11-09": {record("2026-11-09", "Workers Standard", "0.25")},
		"2026-11-19": {record("2026-11-19", "R2 Storage", "0.36")},
	}}
	snap, err := newSource(t, f).Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-10", "2026-11"), MaxRequests: 10})
	if err != nil {
		t.Fatal(err)
	}
	// One info request and one per cycle: 09-10..10-09, 10-10..11-09, 11-10..11-20.
	if snap.Requests != 4 || len(f.queries) != 3 {
		t.Errorf("Requests = %d, queries %v", snap.Requests, f.queries)
	}
	got := map[string]string{}
	for _, l := range snap.Lines {
		got[fmt.Sprintf("%s|%s|%s|%s", l.Month, l.Service, l.Category, l.Covered)] = fmt.Sprintf("%s %s final=%v", l.Amount, l.Currency, l.Final)
	}
	want := map[string]string{
		"2026-10|R2 Storage|storage|2026-10-01/2026-10-31":       "0.015 USD final=true",
		"2026-10|Workers Standard|compute|2026-10-01/2026-10-31": "0.750005 USD final=true",
		"2026-11|R2 Storage|storage|2026-11-01/2026-11-19":       "0.36 USD final=false",
		"2026-11|Workers Standard|compute|2026-11-01/2026-11-19": "0.25 USD final=false",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("lines\n got %v\nwant %v", got, want)
	}
}

func TestFetchClassifiesFailures(t *testing.T) {
	for status, want := range map[int]appfinance.ImportErrorKind{
		http.StatusForbidden:          appfinance.ImportErrorPermission,
		http.StatusTooManyRequests:    appfinance.ImportErrorThrottled,
		http.StatusServiceUnavailable: appfinance.ImportErrorUnavailable,
	} {
		_, err := newSource(t, &fake{status: status}).Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-11"), MaxRequests: 10})
		if got := appfinance.ImportErrorKindOf(err); got != want {
			t.Errorf("%d: %v classified %s, want %s", status, err, got, want)
		}
	}
	_, err := newSource(t, &fake{}).Fetch(t.Context(), appfinance.FetchRequest{Months: months(t, "2026-11"), MaxRequests: 10})
	if got := appfinance.ImportErrorKindOf(err); got != appfinance.ImportErrorNotEnabled {
		t.Errorf("an account not covered: %v classified %s", err, got)
	}
}
