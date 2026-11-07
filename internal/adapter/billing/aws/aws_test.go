// SPDX-License-Identifier: AGPL-3.0-only

package aws_test

import (
	"context"
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

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	billingaws "github.com/mycroblog/my/internal/adapter/billing/aws"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit/financetest"
)

// now is the time of the tests: the data of October 2026 is through the 20th.
var now = time.Date(2026, time.October, 21, 9, 30, 0, 0, time.UTC)

// recordTypes are the RECORD_TYPE values that the fake gives the rows of a charge type, in turn,
// so that the adapter sums several groups into one line.
var recordTypes = map[finance.ChargeType][]string{
	finance.ChargeUsage:      {"Usage", "DiscountedUsage", "SavingsPlanCoveredUsage"},
	finance.ChargeCredit:     {"Credit"},
	finance.ChargeRefund:     {"Refund"},
	finance.ChargeTax:        {"Tax"},
	finance.ChargeSupport:    {"Support"},
	finance.ChargeFee:        {"RIFee", "Fee", "SavingsPlanRecurringFee"},
	finance.ChargeAdjustment: {"BundledDiscount"},
	finance.ChargeOther:      {"SavingsPlanNegation"},
}

type ceRequest struct {
	TimePeriod struct {
		Start string
		End   string
	}
	Granularity   string
	Metrics       []string
	GroupBy       []struct{ Type, Key string }
	NextPageToken string
	Filter        json.RawMessage
}

type ceGroup struct {
	Keys    []string
	Metrics map[string]map[string]string
}

type ceResult struct {
	TimePeriod map[string]string
	Total      map[string]any
	Groups     []ceGroup
	Estimated  bool
}

// fakeCostExplorer is Cost Explorer's GetCostAndUsage over HTTP (the JSON 1.1 protocol): it
// serves the months of a scenario, MONTHLY and grouped by SERVICE and RECORD_TYPE, spread over
// the scenario's pages, or answers with the scenario's failure.
type fakeCostExplorer struct {
	t        *testing.T
	scenario financetest.BillingScenario
	metric   string

	mu       sync.Mutex
	requests []ceRequest
	auth     []string
}

func (f *fakeCostExplorer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if got := r.Header.Get("X-Amz-Target"); got != "AWSInsightsIndexService.GetCostAndUsage" {
		http.Error(w, "unexpected target "+got, http.StatusBadRequest)
		return
	}
	var req ceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.mu.Unlock()
	switch f.scenario.Failure {
	case "":
	case appfinance.ImportErrorPermission:
		apiError(w, http.StatusBadRequest, "AccessDeniedException")
		return
	case appfinance.ImportErrorNotEnabled:
		apiError(w, http.StatusBadRequest, "DataUnavailableException")
		return
	case appfinance.ImportErrorNotFound:
		apiError(w, http.StatusBadRequest, "BillExpirationException")
		return
	case appfinance.ImportErrorThrottled:
		apiError(w, http.StatusBadRequest, "LimitExceededException")
		return
	case appfinance.ImportErrorUnavailable:
		apiError(w, http.StatusServiceUnavailable, "ServiceUnavailableException")
		return
	case appfinance.ImportErrorInvalidResponse:
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = io.WriteString(w, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-01","End":"2026-10-01"},"Groups":[{"Keys":["Tax","Tax"],"Metrics":{"`+f.metric+`":{"Amount":"1e3","Unit":"USD"}}}],"Estimated":false}]}`)
		return
	}
	if req.Granularity != "MONTHLY" || !slices.Equal(req.Metrics, []string{f.metric}) || len(req.GroupBy) != 2 ||
		req.GroupBy[0].Key != "SERVICE" || req.GroupBy[1].Key != "RECORD_TYPE" {
		apiError(w, http.StatusBadRequest, "ValidationException")
		return
	}
	page := 0
	if req.NextPageToken != "" {
		n, err := strconv.Atoi(strings.TrimPrefix(req.NextPageToken, "page-"))
		if err != nil {
			apiError(w, http.StatusBadRequest, "InvalidNextTokenException")
			return
		}
		page = n
	}
	out := map[string]any{"ResultsByTime": f.page(req, page)}
	if pages := max(1, f.scenario.Pages); page+1 < pages {
		out["NextPageToken"] = "page-" + strconv.Itoa(page+1)
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	_ = json.NewEncoder(w).Encode(out)
}

func apiError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"__type":%q,"Message":"fake %s"}`, code, code)
}

// page returns the results of the months in the request's period, with the groups of page n.
func (f *fakeCostExplorer) page(req ceRequest, n int) []ceResult {
	start, err := finance.ParseDate(req.TimePeriod.Start)
	if err != nil {
		f.t.Errorf("TimePeriod.Start %q", req.TimePeriod.Start)
	}
	end, err := finance.ParseDate(req.TimePeriod.End)
	if err != nil {
		f.t.Errorf("TimePeriod.End %q", req.TimePeriod.End)
	}
	pages := max(1, f.scenario.Pages)
	var out []ceResult
	i := 0
	for _, m := range f.scenario.Months {
		r := m.Month.Range()
		if r.Start.Before(start) || !r.Start.Before(end) {
			continue
		}
		periodEnd := r.End.AddDays(1)
		if end.Before(periodEnd) {
			periodEnd = end
		}
		res := ceResult{TimePeriod: map[string]string{"Start": r.Start.String(), "End": periodEnd.String()}, Total: map[string]any{}, Estimated: !m.Final}
		for _, g := range groups(m.Rows, f.metric) {
			if i%pages == n {
				res.Groups = append(res.Groups, g)
			}
			i++
		}
		out = append(out, res)
	}
	return out
}

// groups turns the rows of a month into the groups of Cost Explorer: one per (SERVICE,
// RECORD_TYPE), the rows of a charge type taking its record types in turn and the rows beyond
// them summed into the last, as Cost Explorer sums the line items of a group.
func groups(rows []financetest.FakeBillingRow, metric string) []ceGroup {
	type key struct{ service, record string }
	var order []key
	sums := map[key]*big.Rat{}
	scales := map[key]int{}
	used := map[string]int{}
	for _, row := range rows {
		types := recordTypes[row.ChargeType]
		turn := row.Service + "|" + string(row.ChargeType)
		k := key{row.Service, types[min(used[turn], len(types)-1)]}
		used[turn]++
		amount, _ := new(big.Rat).SetString(row.Amount)
		if sums[k] == nil {
			sums[k] = new(big.Rat)
			order = append(order, k)
		}
		sums[k].Add(sums[k], amount)
		if _, frac, ok := strings.Cut(row.Amount, "."); ok {
			scales[k] = max(scales[k], len(frac))
		}
	}
	out := make([]ceGroup, 0, len(order))
	for _, k := range order {
		out = append(out, ceGroup{
			Keys:    []string{k.service, k.record},
			Metrics: map[string]map[string]string{metric: {"Amount": sums[k].FloatString(scales[k]), "Unit": "USD"}},
		})
	}
	return out
}

func newFake(t *testing.T, s financetest.BillingScenario, o billingaws.Options) (*billingaws.Source, *fakeCostExplorer) {
	t.Helper()
	fake := &fakeCostExplorer{t: t, scenario: s, metric: o.Metric}
	if fake.metric == "" {
		fake.metric = billingaws.MetricNetUnblendedCost
	}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	if o.Now == nil {
		o.Now = func() time.Time { return now }
	}
	cfg := awssdk.Config{
		Region:       "eu-west-1", // the adapter uses us-east-1 whatever the configuration says
		BaseEndpoint: awssdk.String(srv.URL),
		HTTPClient:   srv.Client(),
		Credentials: awssdk.CredentialsProviderFunc(func(context.Context) (awssdk.Credentials, error) {
			return awssdk.Credentials{AccessKeyID: "test", SecretAccessKey: "test", Source: "test"}, nil
		}),
	}
	src, err := billingaws.NewFromConfig(cfg, o)
	if err != nil {
		t.Fatal(err)
	}
	return src, fake
}

func TestBillingSourceContract(t *testing.T) {
	financetest.BillingSourceContract(t, func(t *testing.T, s financetest.BillingScenario) appfinance.BillingSource {
		if s.Failure == appfinance.ImportErrorBytesCap {
			t.Skip("Cost Explorer bills requests, not bytes")
		}
		src, _ := newFake(t, s, billingaws.Options{})
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

func oneMonth(t *testing.T) financetest.BillingScenario {
	return financetest.BillingScenario{Location: time.UTC, Pages: 1, Months: []financetest.BillingScenarioMonth{
		{Month: month(t, "2026-10"), Rows: []financetest.FakeBillingRow{
			{Service: "Amazon Simple Storage Service", ChargeType: finance.ChargeUsage, Amount: "0.10", Currency: "USD"},
		}},
	}}
}

func TestFetchRequest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opts   billingaws.Options
		metric string
		filter string
	}{
		{name: "default", metric: "NetUnblendedCost", filter: ""},
		{name: "unblended", opts: billingaws.Options{Metric: billingaws.MetricUnblendedCost}, metric: "UnblendedCost"},
		{
			name:   "linked accounts",
			opts:   billingaws.Options{Filter: billingaws.FilterLinkedAccount, LinkedAccounts: []string{"111122223333", "444455556666"}},
			metric: "NetUnblendedCost",
			filter: `{"Dimensions":{"Key":"LINKED_ACCOUNT","Values":["111122223333","444455556666"]}}`,
		},
		{
			name:   "tag",
			opts:   billingaws.Options{Filter: billingaws.FilterTag, Tag: "app = my"},
			metric: "NetUnblendedCost",
			filter: `{"Tags":{"Key":"app","MatchOptions":["EQUALS"],"Values":["my"]}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, fake := newFake(t, oneMonth(t), tc.opts)
			if _, err := src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-09"), month(t, "2026-10")}, MaxRequests: 4}); err != nil {
				t.Fatal(err)
			}
			if len(fake.requests) != 1 {
				t.Fatalf("%d requests, want 1", len(fake.requests))
			}
			req := fake.requests[0]
			if req.TimePeriod.Start != "2026-09-01" || req.TimePeriod.End != "2026-10-22" {
				t.Errorf("TimePeriod = %+v, want 2026-09-01 to tomorrow 2026-10-22", req.TimePeriod)
			}
			if !slices.Equal(req.Metrics, []string{tc.metric}) {
				t.Errorf("Metrics = %v, want %s", req.Metrics, tc.metric)
			}
			if got := compactJSON(t, req.Filter); got != tc.filter {
				t.Errorf("Filter = %s, want %s", got, tc.filter)
			}
			if !strings.Contains(fake.auth[0], "/us-east-1/ce/aws4_request") {
				t.Errorf("Authorization = %q, want a signature for us-east-1", fake.auth[0])
			}
		})
	}
}

func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestFetchEndsWithThePastMonths(t *testing.T) {
	src, fake := newFake(t, oneMonth(t), billingaws.Options{Now: func() time.Time { return time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC) }})
	if _, err := src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-09"), month(t, "2026-10")}, MaxRequests: 4}); err != nil {
		t.Fatal(err)
	}
	if req := fake.requests[0]; req.TimePeriod.End != "2026-11-01" {
		t.Errorf("TimePeriod.End = %s, want 2026-11-01, the end of the last month asked for", req.TimePeriod.End)
	}
}

func TestFetchOnTheFirstDayOfAMonth(t *testing.T) {
	src, _ := newFake(t, oneMonth(t), billingaws.Options{Now: func() time.Time { return time.Date(2026, time.October, 1, 3, 0, 0, 0, time.UTC) }})
	snap, err := src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-10")}, MaxRequests: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Lines) != 1 || !snap.Lines[0].Covered.Valid() || snap.Lines[0].Covered.Start.String() != "2026-10-01" {
		t.Errorf("lines = %+v, want one covering 2026-10-01", snap.Lines)
	}
}

func TestFetchDoesNotRetryInTheSDK(t *testing.T) {
	s := oneMonth(t)
	s.Failure = appfinance.ImportErrorThrottled
	src, fake := newFake(t, s, billingaws.Options{})
	snap, err := src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month(t, "2026-10")}, MaxRequests: 4})
	if !errors.Is(err, appfinance.ErrBillingThrottled) {
		t.Errorf("Fetch = %v, want ErrBillingThrottled", err)
	}
	if len(fake.requests) != 1 || snap.Requests != 1 {
		t.Errorf("%d requests sent, %d counted; want 1: the job retries, counting each", len(fake.requests), snap.Requests)
	}
}

func TestChargeTypeOf(t *testing.T) {
	for record, want := range map[string]finance.ChargeType{
		"Usage": finance.ChargeUsage, "DiscountedUsage": finance.ChargeUsage, "SavingsPlanCoveredUsage": finance.ChargeUsage,
		"Credit": finance.ChargeCredit, "Refund": finance.ChargeRefund, "Tax": finance.ChargeTax,
		"Support": finance.ChargeSupport, "Support fee": finance.ChargeSupport,
		"RIFee": finance.ChargeFee, "Recurring reservation fee": finance.ChargeFee, "SavingsPlanRecurringFee": finance.ChargeFee,
		"SavingsPlanUpfrontFee": finance.ChargeFee, "EdpDiscount": finance.ChargeOther, "": finance.ChargeOther,
	} {
		if got := billingaws.ChargeTypeOf(record); got != want {
			t.Errorf("ChargeTypeOf(%q) = %s, want %s", record, got, want)
		}
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	for name, o := range map[string]billingaws.Options{
		"metric":          {Metric: "BlendedCost"},
		"filter":          {Filter: "project"},
		"no accounts":     {Filter: billingaws.FilterLinkedAccount},
		"bad account":     {Filter: billingaws.FilterLinkedAccount, LinkedAccounts: []string{"1234"}},
		"tag without key": {Filter: billingaws.FilterTag, Tag: "=my"},
	} {
		if _, err := billingaws.NewFromConfig(awssdk.Config{}, o); err == nil {
			t.Errorf("%s: NewFromConfig(%+v) = nil error", name, o)
		}
	}
}

func TestChecks(t *testing.T) {
	for _, tc := range []struct {
		check func(string) error
		ok    []string
		bad   []string
	}{
		{billingaws.CheckMetric, []string{"NetUnblendedCost", "UnblendedCost"}, []string{"", "AmortizedCost"}},
		{billingaws.CheckLinkedAccounts, []string{"111122223333", "111122223333, 444455556666"}, []string{"", "abc", "111122223333,111122223333"}},
		{billingaws.CheckTag, []string{"app=my", "env="}, []string{"app", "=my"}},
		{billingaws.CheckRoleARN, []string{"arn:aws:iam::123456789012:role/my-billing", "arn:aws:iam::123456789012:role/path/to/r"}, []string{"", "arn:aws:iam::123:role/x", "arn:aws:iam::123456789012:user/x"}},
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

type fakeMetadata struct {
	suffix string
	token  string
}

func (m *fakeMetadata) GetWithContext(_ context.Context, suffix string) (string, error) {
	m.suffix = suffix
	return m.token, nil
}

func TestGoogleIDToken(t *testing.T) {
	md := &fakeMetadata{token: "header.payload.signature\n"}
	got, err := billingaws.GoogleIDToken{Client: md, Audience: "arn:aws:iam::123456789012:role/my-billing"}.GetIdentityToken()
	if err != nil || string(got) != "header.payload.signature" {
		t.Errorf("GetIdentityToken = %q, %v", got, err)
	}
	want := "instance/service-accounts/default/identity?audience=arn%3Aaws%3Aiam%3A%3A123456789012%3Arole%2Fmy-billing&format=full"
	if md.suffix != want {
		t.Errorf("metadata path = %q, want %q", md.suffix, want)
	}
	if _, err := (billingaws.GoogleIDToken{Client: &fakeMetadata{}, Audience: "x"}).GetIdentityToken(); err == nil {
		t.Error("an empty token: nil error")
	}
}
