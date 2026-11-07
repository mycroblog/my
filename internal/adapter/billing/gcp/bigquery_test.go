// SPDX-License-Identifier: AGPL-3.0-only

package gcp_test

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/option"

	billinggcp "github.com/mycroblog/my/internal/adapter/billing/gcp"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit/financetest"
)

const table = "my-billing.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01"

var pacific = mustLocation()

func mustLocation() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return loc
}

// now is the time of the tests: 2026-10-21 in Pacific time, when September is final by the rule
// of the 5th and October is open.
var now = time.Date(2026, time.October, 21, 9, 30, 0, 0, pacific)

// queryRequest is the part of a jobs.query request that the fake reads.
type queryRequest struct {
	Query              string
	UseLegacySQL       *bool  `json:"useLegacySql"`
	MaximumBytesBilled string `json:"maximumBytesBilled"`
	ParameterMode      string `json:"parameterMode"`
	QueryParameters    []struct {
		Name          string
		ParameterType struct {
			Type      string
			ArrayType *struct{ Type string }
		}
		ParameterValue struct {
			Value       string
			ArrayValues []struct{ Value string }
		}
	}
}

func (r queryRequest) param(name string) []string {
	for _, p := range r.QueryParameters {
		if p.Name != name {
			continue
		}
		if p.ParameterType.Type == "ARRAY" {
			out := make([]string, 0, len(p.ParameterValue.ArrayValues))
			for _, v := range p.ParameterValue.ArrayValues {
				out = append(out, v.Value)
			}
			return out
		}
		return []string{p.ParameterValue.Value}
	}
	return nil
}

// fakeBigQuery is the BigQuery REST API of jobs.query, jobs.getQueryResults, and jobs.get over
// HTTP: it runs the adapter's query on the rows of a scenario (the GROUP BY of the export, in
// Go), splits the result over the scenario's pages, fails a query that would bill more than its
// maximumBytesBilled, or answers with the scenario's failure.
type fakeBigQuery struct {
	t        *testing.T
	scenario financetest.BillingScenario
	// bytes is what a query bills; 10 MB, BigQuery's minimum, when 0.
	bytes int64
	// incomplete makes jobs.query answer before the job completes.
	incomplete bool

	mu      sync.Mutex
	queries []queryRequest
	paths   []string
	pages   [][][]string
}

func (f *fakeBigQuery) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	rest, ok := strings.CutPrefix(r.URL.Path, "/bigquery/v2/projects/")
	if !ok {
		apiError(w, http.StatusNotFound, "notFound")
		return
	}
	project, rest, _ := strings.Cut(rest, "/")
	switch {
	case r.Method == http.MethodPost && rest == "queries":
		f.query(w, r, project)
	case r.Method == http.MethodGet && strings.HasPrefix(rest, "queries/"):
		f.results(w, r, project, strings.TrimPrefix(rest, "queries/"))
	case r.Method == http.MethodGet && strings.HasPrefix(rest, "jobs/"):
		writeJSON(w, map[string]any{
			"jobReference": map[string]string{"projectId": project, "jobId": strings.TrimPrefix(rest, "jobs/")},
			"statistics":   map[string]any{"query": map[string]string{"totalBytesBilled": strconv.FormatInt(f.billed(), 10)}},
		})
	default:
		apiError(w, http.StatusNotFound, "notFound")
	}
}

func (f *fakeBigQuery) billed() int64 { return cmp.Or(f.bytes, 10<<20) }

func apiError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":"fake %s","errors":[{"reason":%q,"message":"fake %s"}]}}`, status, reason, reason, reason)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeBigQuery) query(w http.ResponseWriter, r *http.Request, project string) {
	body, _ := io.ReadAll(r.Body)
	var req queryRequest
	if err := json.Unmarshal(body, &req); err != nil {
		apiError(w, http.StatusBadRequest, "invalid")
		return
	}
	f.mu.Lock()
	f.queries = append(f.queries, req)
	f.mu.Unlock()
	switch f.scenario.Failure {
	case "":
	case appfinance.ImportErrorPermission:
		apiError(w, http.StatusForbidden, "accessDenied")
		return
	case appfinance.ImportErrorNotEnabled:
		apiError(w, http.StatusForbidden, "accessNotConfigured")
		return
	case appfinance.ImportErrorNotFound:
		apiError(w, http.StatusNotFound, "notFound")
		return
	case appfinance.ImportErrorThrottled:
		apiError(w, http.StatusForbidden, "rateLimitExceeded")
		return
	case appfinance.ImportErrorBytesCap:
		apiError(w, http.StatusBadRequest, "bytesBilledLimitExceeded")
		return
	case appfinance.ImportErrorUnavailable:
		apiError(w, http.StatusServiceUnavailable, "backendError")
		return
	case appfinance.ImportErrorInvalidResponse:
		writeJSON(w, map[string]any{"jobComplete": true, "rows": []any{row("202609", "Tax", "tax", "USD", "1e3", "0", "")}})
		return
	}
	if req.UseLegacySQL == nil || *req.UseLegacySQL || req.ParameterMode != "NAMED" ||
		!strings.Contains(req.Query, "FROM `"+table+"`") || req.MaximumBytesBilled == "" {
		apiError(w, http.StatusBadRequest, "invalidQuery")
		return
	}
	if limit, _ := strconv.ParseInt(req.MaximumBytesBilled, 10, 64); limit < f.billed() {
		apiError(w, http.StatusBadRequest, "bytesBilledLimitExceeded")
		return
	}
	f.mu.Lock()
	f.pages = f.split(f.run(req.param("months")))
	f.mu.Unlock()
	job := map[string]string{"projectId": project, "jobId": "job-1", "location": "US"}
	if f.incomplete {
		writeJSON(w, map[string]any{"jobComplete": false, "jobReference": job})
		return
	}
	out := f.page(0)
	out["jobReference"] = job
	out["totalBytesBilled"] = strconv.FormatInt(f.billed(), 10)
	writeJSON(w, out)
}

func (f *fakeBigQuery) results(w http.ResponseWriter, r *http.Request, project, job string) {
	if job != "job-1" || project == "" {
		apiError(w, http.StatusNotFound, "notFound")
		return
	}
	n := 0
	if token := r.URL.Query().Get("pageToken"); token != "" {
		var err error
		if n, err = strconv.Atoi(strings.TrimPrefix(token, "page-")); err != nil {
			apiError(w, http.StatusBadRequest, "invalid")
			return
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	writeJSON(w, f.page(n))
}

// page returns page n of the result: its rows and the token of the next page.
func (f *fakeBigQuery) page(n int) map[string]any {
	out := map[string]any{"jobComplete": true}
	if n < len(f.pages) {
		rows := make([]any, 0, len(f.pages[n]))
		for _, r := range f.pages[n] {
			rows = append(rows, row(r...))
		}
		out["rows"] = rows
	}
	if n+1 < len(f.pages) {
		out["pageToken"] = "page-" + strconv.Itoa(n+1)
	}
	return out
}

// row is a TableRow of string cells; "" is NULL.
func row(cells ...string) map[string]any {
	f := make([]map[string]any, 0, len(cells))
	for _, c := range cells {
		if c == "" {
			f = append(f, map[string]any{"v": nil})
		} else {
			f = append(f, map[string]any{"v": c})
		}
	}
	return map[string]any{"f": f}
}

// split spreads the result rows over the scenario's pages, at least one row per page.
func (f *fakeBigQuery) split(rows [][]string) [][][]string {
	pages := make([][][]string, max(1, f.scenario.Pages))
	for i, r := range rows {
		pages[i%len(pages)] = append(pages[i%len(pages)], r)
	}
	return pages
}

// costTypes are the cost_type of the export of each charge type; the credits are credits[].
var costTypes = map[finance.ChargeType]string{
	finance.ChargeUsage:      "regular",
	finance.ChargeTax:        "tax",
	finance.ChargeAdjustment: "rounding error",
}

// run is the adapter's query on the scenario: the sums by invoice month, service, cost type, and
// currency of the costs and the credits, and the end of the last usage (the day after Through,
// at midnight Pacific, for an open month; the end of the month for a final one).
func (f *fakeBigQuery) run(months []string) [][]string {
	type key struct{ month, service, costType, currency string }
	type sums struct {
		cost, credits *big.Rat
		scale         int
		end           int64
	}
	var order []key
	groups := map[key]*sums{}
	for _, m := range f.scenario.Months {
		invoice := billinggcp.InvoiceMonth(m.Month)
		if !slices.Contains(months, invoice) {
			continue
		}
		last := m.Month.Range().End
		if !m.Final && m.Through != (finance.Date{}) {
			last = m.Through
		}
		y, mo, d := last.AddDays(1).Date()
		end := time.Date(y, mo, d, 0, 0, 0, 0, pacific).UnixMicro()
		for _, r := range m.Rows {
			costType := "regular"
			if r.ChargeType != finance.ChargeCredit {
				var ok bool
				if costType, ok = costTypes[r.ChargeType]; !ok {
					f.t.Fatalf("the export has no charge type %s", r.ChargeType)
				}
			}
			k := key{invoice, r.Service, costType, r.Currency}
			g, ok := groups[k]
			if !ok {
				g = &sums{cost: new(big.Rat), credits: new(big.Rat)}
				groups[k] = g
				order = append(order, k)
			}
			amount, _ := new(big.Rat).SetString(r.Amount)
			if r.ChargeType == finance.ChargeCredit {
				g.credits.Add(g.credits, amount)
			} else {
				g.cost.Add(g.cost, amount)
			}
			if _, frac, ok := strings.Cut(r.Amount, "."); ok {
				g.scale = max(g.scale, len(frac))
			}
			g.end = max(g.end, end)
		}
	}
	out := make([][]string, 0, len(order))
	for _, k := range order {
		g := groups[k]
		out = append(out, []string{k.month, k.service, k.costType, k.currency,
			g.cost.FloatString(g.scale), g.credits.FloatString(g.scale), strconv.FormatInt(g.end, 10)})
	}
	return out
}

func newFake(t *testing.T, s financetest.BillingScenario, o billinggcp.BigQueryOptions) (*billinggcp.BigQuerySource, *fakeBigQuery) {
	t.Helper()
	fake := &fakeBigQuery{t: t, scenario: s}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	o.Table = cmp.Or(o.Table, table)
	if o.Now == nil {
		o.Now = func() time.Time { return now }
	}
	src, err := billinggcp.NewBigQuery(t.Context(), o,
		option.WithEndpoint(srv.URL+"/bigquery/v2/"), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return src, fake
}

func TestBillingSourceContract(t *testing.T) {
	financetest.BillingSourceContract(t, func(t *testing.T, s financetest.BillingScenario) appfinance.BillingSource {
		src, _ := newFake(t, s, billinggcp.BigQueryOptions{})
		return src
	})
}

func month(t *testing.T, s string) finance.YearMonth {
	t.Helper()
	m, err := finance.ParseYearMonth(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func date(t *testing.T, s string) finance.Date {
	t.Helper()
	d, err := finance.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func oneMonth(t *testing.T) financetest.BillingScenario {
	return financetest.BillingScenario{Location: time.UTC, Pages: 1, Months: []financetest.BillingScenarioMonth{
		{Month: month(t, "2026-10"), Through: date(t, "2026-10-20"), Rows: []financetest.FakeBillingRow{
			{Service: "Cloud Run", ChargeType: finance.ChargeUsage, Amount: "0.10", Currency: "JPY"},
		}},
	}}
}

func fetch(t *testing.T, src *billinggcp.BigQuerySource, maxRequests int, months ...string) (appfinance.Snapshot, error) {
	t.Helper()
	req := appfinance.FetchRequest{MaxRequests: maxRequests}
	for _, m := range months {
		req.Months = append(req.Months, month(t, m))
	}
	return src.Fetch(t.Context(), req)
}

func TestFetchRequest(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     billinggcp.BigQueryOptions
		project  string
		bytes    string
		filters  []string
		projects []string
		label    []string
	}{
		{name: "default", project: "my-billing", bytes: "1073741824"},
		{
			name:    "query project and cap",
			opts:    billinggcp.BigQueryOptions{QueryProject: "my-queries", MaxBytesBilled: 100 << 20},
			project: "my-queries", bytes: "104857600",
		},
		{
			name:    "projects and label",
			opts:    billinggcp.BigQueryOptions{Projects: []string{"my-social", "my-media"}, Label: "app = my"},
			project: "my-billing", bytes: "1073741824",
			filters:  []string{"project.id IN UNNEST(@projects)", "l.key = @label_key AND l.value = @label_value"},
			projects: []string{"my-social", "my-media"}, label: []string{"app", "my"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, fake := newFake(t, oneMonth(t), tc.opts)
			snap, err := fetch(t, src, 4, "2026-09", "2026-10")
			if err != nil {
				t.Fatal(err)
			}
			if snap.Requests != 1 || snap.BytesBilled != 10<<20 {
				t.Errorf("Requests = %d, BytesBilled = %d; want 1 and 10 MB", snap.Requests, snap.BytesBilled)
			}
			if len(fake.queries) != 1 || fake.paths[0] != "POST /bigquery/v2/projects/"+tc.project+"/queries" {
				t.Fatalf("calls = %q, want one query in %s", fake.paths, tc.project)
			}
			q := fake.queries[0]
			if q.MaximumBytesBilled != tc.bytes {
				t.Errorf("maximumBytesBilled = %s, want %s", q.MaximumBytesBilled, tc.bytes)
			}
			if got := q.param("months"); !slices.Equal(got, []string{"202609", "202610"}) {
				t.Errorf("@months = %q, want the invoice months 202609 and 202610", got)
			}
			for _, want := range append([]string{"SUM(CAST(cost AS NUMERIC))", "UNNEST(credits)", "invoice.month IN UNNEST(@months)"}, tc.filters...) {
				if !strings.Contains(q.Query, want) {
					t.Errorf("the query has no %q:\n%s", want, q.Query)
				}
			}
			if len(tc.filters) == 0 && (strings.Contains(q.Query, "@projects") || strings.Contains(q.Query, "@label")) {
				t.Errorf("a filter without its setting:\n%s", q.Query)
			}
			if got := q.param("projects"); !slices.Equal(got, tc.projects) {
				t.Errorf("@projects = %q, want %q", got, tc.projects)
			}
			if got := append(q.param("label_key"), q.param("label_value")...); !slices.Equal(got, tc.label) {
				t.Errorf("@label_key, @label_value = %q, want %q", got, tc.label)
			}
			if len(snap.Lines) != 1 || snap.Lines[0].Currency != "JPY" || snap.Lines[0].Category != finance.CategoryCompute {
				t.Errorf("lines = %+v, want Cloud Run in JPY", snap.Lines)
			}
		})
	}
}

func TestFetchStopsAtTheBytesCap(t *testing.T) {
	src, fake := newFake(t, oneMonth(t), billinggcp.BigQueryOptions{MaxBytesBilled: 1 << 20})
	fake.bytes = 20 << 20
	snap, err := fetch(t, src, 4, "2026-10")
	if !errors.Is(err, appfinance.ErrBytesCap) || appfinance.ImportErrorKindOf(err) != appfinance.ImportErrorBytesCap {
		t.Errorf("Fetch over the cap = %v, want ErrBytesCap", err)
	}
	if snap.Requests != 1 || len(snap.Lines) != 0 || snap.BytesBilled != 0 {
		t.Errorf("Fetch over the cap = %d requests, %d lines, %d bytes; want 1, none, 0", snap.Requests, len(snap.Lines), snap.BytesBilled)
	}
}

func TestFetchWaitsForTheJob(t *testing.T) {
	src, fake := newFake(t, oneMonth(t), billinggcp.BigQueryOptions{})
	fake.incomplete = true
	snap, err := fetch(t, src, 4, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /bigquery/v2/projects/my-billing/queries",
		"GET /bigquery/v2/projects/my-billing/queries/job-1",
		"GET /bigquery/v2/projects/my-billing/jobs/job-1",
	}
	if !slices.Equal(fake.paths, want) || snap.Requests != 3 || snap.BytesBilled != 10<<20 || len(snap.Lines) != 1 {
		t.Errorf("calls %q, %d requests, %d bytes, %d lines; want %q, 3, 10 MB, 1", fake.paths, snap.Requests, snap.BytesBilled, len(snap.Lines), want)
	}
	if _, err := fetch(t, src, 2, "2026-10"); !errors.Is(err, appfinance.ErrRequestCap) {
		t.Errorf("Fetch within 2 requests = %v, want ErrRequestCap", err)
	}
}

func TestFetchKeepsPacificMonths(t *testing.T) {
	// 2026-11-01 02:00 UTC is still October 31 in Pacific time: the open month is October, through
	// the day of the last usage, and October is not final.
	at := time.Date(2026, time.November, 1, 2, 0, 0, 0, time.UTC)
	s := oneMonth(t)
	s.Months[0].Through = date(t, "2026-10-30")
	src, _ := newFake(t, s, billinggcp.BigQueryOptions{Now: func() time.Time { return at }})
	snap, err := fetch(t, src, 4, "2026-10", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Lines) != 1 {
		t.Fatalf("lines = %+v, want October's", snap.Lines)
	}
	l := snap.Lines[0]
	if l.Month.String() != "2026-10" || l.Final || l.Covered.String() != "2026-10-01/2026-10-30" {
		t.Errorf("line = %s final %v covers %s, want an estimate of 2026-10 over 2026-10-01/2026-10-30", l.Month, l.Final, l.Covered)
	}
	if src.Location().String() != "America/Los_Angeles" {
		t.Errorf("Location() = %s", src.Location())
	}
}

func TestFetchFinalByTheFifth(t *testing.T) {
	s := oneMonth(t)
	for _, tc := range []struct {
		at    time.Time
		final bool
	}{
		{time.Date(2026, time.November, 4, 23, 59, 0, 0, pacific), false},
		{time.Date(2026, time.November, 5, 0, 0, 0, 0, pacific), true},
		// 2026-11-05 07:00 UTC is still November 4 in Pacific time.
		{time.Date(2026, time.November, 5, 7, 0, 0, 0, time.UTC), false},
	} {
		src, _ := newFake(t, s, billinggcp.BigQueryOptions{Now: func() time.Time { return tc.at }})
		snap, err := fetch(t, src, 4, "2026-10", "2026-11")
		if err != nil || len(snap.Lines) != 1 {
			t.Fatalf("Fetch at %s = %+v, %v", tc.at, snap.Lines, err)
		}
		l := snap.Lines[0]
		if l.Final != tc.final {
			t.Errorf("at %s: final %v, want %v", tc.at, l.Final, tc.final)
		}
		if tc.final && l.Covered != l.Month.Range() {
			t.Errorf("at %s: the final month covers %s, want the whole month", tc.at, l.Covered)
		}
	}
}

func TestFetchSumsCostTypesAndRejectsMixedCurrencies(t *testing.T) {
	s := oneMonth(t)
	s.Months[0].Rows = []financetest.FakeBillingRow{
		{Service: "Cloud SQL", ChargeType: finance.ChargeUsage, Amount: "12.000000001", Currency: "JPY"},
		{Service: "Cloud SQL", ChargeType: finance.ChargeAdjustment, Amount: "-0.000000001", Currency: "JPY"},
		{Service: "Cloud SQL", ChargeType: finance.ChargeCredit, Amount: "-12", Currency: "JPY"},
		{Service: "Cloud SQL", ChargeType: finance.ChargeCredit, Amount: "12", Currency: "JPY"},
	}
	src, _ := newFake(t, s, billinggcp.BigQueryOptions{})
	snap, err := fetch(t, src, 4, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Lines) != 2 {
		t.Fatalf("lines = %+v, want usage and adjustment, and no credit line of 0", snap.Lines)
	}
	if l := snap.Lines[1]; l.ChargeType != finance.ChargeUsage || l.Amount != "12.000000001" || l.Category != finance.CategoryDatabase {
		t.Errorf("usage = %+v", l)
	}
	if l := snap.Lines[0]; l.ChargeType != finance.ChargeAdjustment || l.Amount != "-0.000000001" {
		t.Errorf("adjustment = %+v", l)
	}

	s.Months[0].Rows = append(s.Months[0].Rows, financetest.FakeBillingRow{Service: "Cloud Storage", ChargeType: finance.ChargeUsage, Amount: "1", Currency: "USD"})
	src, _ = newFake(t, s, billinggcp.BigQueryOptions{})
	if _, err := fetch(t, src, 4, "2026-10"); !errors.Is(err, appfinance.ErrInvalidBillingResponse) {
		t.Errorf("Fetch of a month in two currencies = %v, want ErrInvalidBillingResponse", err)
	}
}

func TestFetchOfFutureMonthsCallsNothing(t *testing.T) {
	src, fake := newFake(t, oneMonth(t), billinggcp.BigQueryOptions{})
	snap, err := fetch(t, src, 4, "2026-11")
	if err != nil || snap.Requests != 0 || len(fake.paths) != 0 {
		t.Errorf("Fetch of a future month = %d requests, %d calls, %v; want none", snap.Requests, len(fake.paths), err)
	}
}

func TestClassifiesThrottlingAndAuthentication(t *testing.T) {
	for _, tc := range []struct {
		status int
		reason string
		want   error
	}{
		{http.StatusTooManyRequests, "tooManyRequests", appfinance.ErrBillingThrottled},
		{http.StatusForbidden, "quotaExceeded", appfinance.ErrBillingThrottled},
		{http.StatusUnauthorized, "authError", appfinance.ErrBillingPermission},
		{http.StatusForbidden, "billingNotEnabled", appfinance.ErrBillingNotEnabled},
		{http.StatusBadRequest, "invalidQuery", appfinance.ErrInvalidBillingResponse},
		{http.StatusInternalServerError, "internalError", appfinance.ErrBillingUnavailable},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { apiError(w, tc.status, tc.reason) }))
		src, err := billinggcp.NewBigQuery(t.Context(), billinggcp.BigQueryOptions{Table: table, Now: func() time.Time { return now }},
			option.WithEndpoint(srv.URL+"/bigquery/v2/"), option.WithHTTPClient(srv.Client()))
		if err != nil {
			t.Fatal(err)
		}
		snap, err := fetch(t, src, 4, "2026-10")
		if !errors.Is(err, tc.want) || snap.Requests != 1 {
			t.Errorf("%d %s: Fetch = %v after %d requests, want %v after 1 (no retry in the client)", tc.status, tc.reason, err, snap.Requests, tc.want)
		}
		srv.Close()
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	for name, o := range map[string]billinggcp.BigQueryOptions{
		"no table":        {},
		"table":           {Table: "billing_export.table"},
		"query project":   {Table: table, QueryProject: "My_Project"},
		"projects":        {Table: table, Projects: []string{"ok-project", "ok-project"}},
		"label":           {Table: table, Label: "=my"},
		"negative cap":    {Table: table, MaxBytesBilled: -1},
		"project of a ID": {Table: "x.billing_export.table"},
	} {
		if _, err := billinggcp.NewBigQuery(t.Context(), o, option.WithoutAuthentication()); err == nil {
			t.Errorf("%s: NewBigQuery(%+v) = nil error", name, o)
		}
	}
}

func TestChecks(t *testing.T) {
	for _, tc := range []struct {
		check func(string) error
		ok    []string
		bad   []string
	}{
		{billinggcp.CheckTable, []string{table, "example.com:my-project.billing.gcp_billing_export_v1_X"}, []string{"", "a.b", "my-project.bad-dataset.t", "my-project.d.t t"}},
		{billinggcp.CheckProject, []string{"my-project", "example.com:my-project"}, []string{"", "short", "My-Project", "my-project-"}},
		{billinggcp.CheckProjects, []string{"my-project", "my-project, my-media"}, []string{"", "my-project,my-project", "my-project,x"}},
		{billinggcp.CheckLabel, []string{"app=my", "env="}, []string{"app", "=my"}},
	} {
		for _, v := range tc.ok {
			if err := tc.check(v); err != nil {
				t.Errorf("check(%q) = %v", v, err)
			}
		}
		for _, v := range tc.bad {
			if err := tc.check(v); err == nil {
				t.Errorf("check(%q) = nil, want an error", v)
			}
		}
	}
}

func TestChargeTypeOf(t *testing.T) {
	for costType, want := range map[string]finance.ChargeType{
		"regular": finance.ChargeUsage, "tax": finance.ChargeTax, "adjustment": finance.ChargeAdjustment,
		"rounding error": finance.ChargeAdjustment, "commitment": finance.ChargeOther,
	} {
		if got := billinggcp.ChargeTypeOf(costType); got != want {
			t.Errorf("ChargeTypeOf(%q) = %s, want %s", costType, got, want)
		}
	}
}
