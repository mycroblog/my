// SPDX-License-Identifier: AGPL-3.0-only

package plugin_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	billingplugin "github.com/mycroblog/my/internal/adapter/billing/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit"
)

func newSource(t *testing.T, d kind.Destination, loc *time.Location) *billingplugin.Source {
	t.Helper()
	k, ok := kind.ByName(kind.Billing)
	if !ok {
		t.Fatal("no billing kind")
	}
	conn, err := client.New(client.Options{Kind: k, Destination: d})
	if err != nil {
		t.Fatal(err)
	}
	src, err := billingplugin.New(conn, loc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })
	return src
}

// failures are errors of the port by kind, which the fake's source fails with.
var failures = map[appfinance.ImportErrorKind]error{
	appfinance.ImportErrorPermission:      appfinance.ErrBillingPermission,
	appfinance.ImportErrorNotEnabled:      appfinance.ErrBillingNotEnabled,
	appfinance.ImportErrorNotFound:        appfinance.ErrBillingNotFound,
	appfinance.ImportErrorThrottled:       appfinance.ErrBillingThrottled,
	appfinance.ImportErrorBytesCap:        appfinance.ErrBytesCap,
	appfinance.ImportErrorInvalidResponse: appfinance.ErrInvalidBillingResponse,
	appfinance.ImportErrorUnavailable:     appfinance.ErrBillingUnavailable,
}

func providerOf(s testkit.BillingScenario) *testkit.FakeBillingSource {
	provider := testkit.NewFakeBillingSource(appfinance.SourcePlugin, s.Location)
	for _, m := range s.Months {
		provider.SetMonth(m.Month, testkit.FakeBillingMonth{Final: m.Final, Through: m.Through, Rows: m.Rows})
	}
	provider.SetPages(s.Pages)
	if s.Failure != "" {
		provider.SetFailure(failures[s.Failure])
	}
	return provider
}

// The adapter passes the contract of the port against both fakes, over each protocol they speak.
func TestContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		protocol string
		fake     func(fakeplugin.Options) (http.Handler, *fakeplugin.Recorder)
	}{
		{"http_and_json", kind.ProtocolJSON, fakeplugin.JSON},
		{"connect_json", kind.ProtocolJSON, fakeplugin.Connect},
		{"connect_proto", kind.ProtocolProto, fakeplugin.Connect},
		{"connect_grpc", kind.ProtocolGRPC, fakeplugin.Connect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.BillingSourceContract(t, func(t *testing.T, s testkit.BillingScenario) appfinance.BillingSource {
				t.Helper()
				h, _ := tc.fake(fakeplugin.Options{Billing: providerOf(s)})
				return newSource(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Protocol: tc.protocol}, time.UTC)
			})
		})
	}
}

func TestWhatThePortAsksWithNoCall(t *testing.T) {
	t.Parallel()
	h, rec := fakeplugin.JSON(fakeplugin.Options{})
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	src := newSource(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, tokyo)
	if src.Name() != "plugin" || src.Method() != "plugin" || src.Location() != tokyo {
		t.Errorf("Name, Method, Location = %s, %s, %s", src.Name(), src.Method(), src.Location())
	}
	if rec.Calls() != 0 {
		t.Errorf("%d calls, want none", rec.Calls())
	}
}

func TestNewRefusesNoTimeZone(t *testing.T) {
	t.Parallel()
	k, _ := kind.ByName(kind.Billing)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: "unix:///nowhere"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := billingplugin.New(conn, nil); err == nil {
		t.Error("New without a time zone succeeded")
	}
}

func fetch(t *testing.T, src *billingplugin.Source, maxRequests int) (appfinance.Snapshot, error) {
	t.Helper()
	month, err := finance.ParseYearMonth("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	return src.Fetch(t.Context(), appfinance.FetchRequest{Months: []finance.YearMonth{month}, MaxRequests: maxRequests})
}

// raw serves one fixed answer of a plugin that may be wrong in any way.
func raw(t *testing.T, status int, body string) *billingplugin.Source {
	t.Helper()
	addr := fakeplugin.ServeUnix(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	return newSource(t, kind.Destination{Address: addr}, time.UTC)
}

const goodLine = `"month":"2026-10","coveredStart":"2026-10-01","coveredEnd":"2026-10-20","service":"Hetzner Cloud","chargeType":"CHARGE_TYPE_USAGE","category":"CATEGORY_COMPUTE","amount":"1.25","currency":"EUR"`

// Each answer here must fail, record no line, and keep the requests it reported.
func TestUntrustedAnswers(t *testing.T) {
	t.Parallel()
	line := func(replace ...string) string {
		return `{"outcome":"OUTCOME_OK","requests":1,"lines":[{` + strings.NewReplacer(replace...).Replace(goodLine) + `}]}`
	}
	cases := []struct {
		name     string
		body     string
		kind     appfinance.ImportErrorKind
		requests int
	}{
		{"an empty object", `{}`, appfinance.ImportErrorInvalidResponse, 0},
		{"lines without an outcome", `{"requests":1,"lines":[{` + goodLine + `}]}`, appfinance.ImportErrorInvalidResponse, 1},
		{"an unknown name of an outcome", `{"outcome":"OUTCOME_FINE"}`, appfinance.ImportErrorInvalidResponse, 0},
		{"an unknown number of an outcome", `{"outcome":99}`, appfinance.ImportErrorInvalidResponse, 0},
		{"negative requests", `{"outcome":"OUTCOME_OK","requests":-1}`, appfinance.ImportErrorInvalidResponse, 0},
		{"more requests than allowed", `{"outcome":"OUTCOME_OK","requests":11}`, appfinance.ImportErrorInvalidResponse, 11},
		{"negative bytes", `{"outcome":"OUTCOME_OK","bytesBilled":"-1"}`, appfinance.ImportErrorInvalidResponse, 0},
		{"a month not asked for", line(`"month":"2026-10"`, `"month":"2026-09"`), appfinance.ImportErrorInvalidResponse, 1},
		{"a bad month", line(`"month":"2026-10"`, `"month":"October"`), appfinance.ImportErrorInvalidResponse, 1},
		{"no service", line(`"Hetzner Cloud"`, `""`), appfinance.ImportErrorInvalidResponse, 1},
		{"a long service", line(`Hetzner Cloud`, strings.Repeat("x", appfinance.MaxServiceLength+1)), appfinance.ImportErrorInvalidResponse, 1},
		{"a control character", line(`Hetzner Cloud`, `Hetzner\u0000Cloud`), appfinance.ImportErrorInvalidResponse, 1},
		{"no charge type", line(`"chargeType":"CHARGE_TYPE_USAGE",`, ``), appfinance.ImportErrorInvalidResponse, 1},
		{"a number outside the charge types", line(`"CHARGE_TYPE_USAGE"`, `99`), appfinance.ImportErrorInvalidResponse, 1},
		{"no category", line(`"category":"CATEGORY_COMPUTE",`, ``), appfinance.ImportErrorInvalidResponse, 1},
		{"a bad covered day", line(`"2026-10-20"`, `"20 October"`), appfinance.ImportErrorInvalidResponse, 1},
		{"days outside the month", line(`"2026-10-20"`, `"2026-11-02"`), appfinance.ImportErrorInvalidResponse, 1},
		{"days in the wrong order", line(`"2026-10-01"`, `"2026-10-21"`), appfinance.ImportErrorInvalidResponse, 1},
		{"a final month not covered whole", line(`"currency":"EUR"`, `"currency":"EUR","final":true`), appfinance.ImportErrorInvalidResponse, 1},
		{"an amount that is no decimal", line(`"1.25"`, `"1e3"`), appfinance.ImportErrorInvalidResponse, 1},
		{"an unknown currency", line(`"EUR"`, `"XXQ"`), appfinance.ImportErrorInvalidResponse, 1},
		{"a repeated key", `{"outcome":"OUTCOME_OK","requests":1,"lines":[{` + goodLine + `},{` + goodLine + `}]}`, appfinance.ImportErrorInvalidResponse, 1},
		{"a throttled answer that holds lines", `{"outcome":"OUTCOME_THROTTLED","requests":2,"lines":[{` + goodLine + `}]}`, appfinance.ImportErrorThrottled, 2},
		{"an HTTP error", `oops`, appfinance.ImportErrorUnavailable, 0},
		{"not JSON", `<html>`, appfinance.ImportErrorUnavailable, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status := http.StatusOK
			if tc.body == "oops" {
				status = http.StatusBadGateway
			}
			snap, err := fetch(t, raw(t, status, tc.body), 10)
			if got := appfinance.ImportErrorKindOf(err); got != tc.kind {
				t.Errorf("err = %v, classified %q, want %q", err, got, tc.kind)
			}
			if len(snap.Lines) != 0 || snap.Requests != tc.requests {
				t.Errorf("snapshot = %d lines, %d requests; want none and %d", len(snap.Lines), snap.Requests, tc.requests)
			}
		})
	}
}

func TestAWellFormedAnswer(t *testing.T) {
	t.Parallel()
	snap, err := fetch(t, raw(t, http.StatusOK, `{"outcome":"OUTCOME_OK","requests":1,"bytesBilled":"7","lines":[{`+goodLine+`}]}`), 10)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Requests != 1 || snap.BytesBilled != 7 || len(snap.Lines) != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
	l := snap.Lines[0]
	if l.Month.String() != "2026-10" || l.Covered.Start.String() != "2026-10-01" || l.Covered.End.String() != "2026-10-20" || l.Service != "Hetzner Cloud" ||
		l.ChargeType != finance.ChargeUsage || l.Category != finance.CategoryCompute || l.Amount != "1.25" || l.Currency != "EUR" || l.Final {
		t.Errorf("line = %+v", l)
	}
}

// What a plugin writes in an error reaches no error of the server (ADR-0146 §10).
func TestAConnectErrorLeavesItsMessage(t *testing.T) {
	t.Parallel()
	const secret = "the database password is hunter2"
	snap, err := fetch(t, raw(t, http.StatusServiceUnavailable, fmt.Sprintf(`{"code":"unavailable","message":%q}`, secret)), 10)
	if !errors.Is(err, appfinance.ErrBillingUnavailable) || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("err = %v", err)
	}
	if snap.Requests != 0 || len(snap.Lines) != 0 {
		t.Errorf("snapshot = %+v", snap)
	}
}

// The errors of a response name a line and hold nothing that the plugin wrote.
func TestAnInvalidLineLeavesItsText(t *testing.T) {
	t.Parallel()
	_, err := fetch(t, raw(t, http.StatusOK, `{"outcome":"OUTCOME_OK","lines":[{`+strings.ReplaceAll(goodLine, `"1.25"`, `"hunter2"`)+`}]}`), 10)
	if !errors.Is(err, appfinance.ErrInvalidBillingResponse) || strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "Hetzner") {
		t.Errorf("err = %v", err)
	}
}

func TestADeadlineIsAFailureToRetry(t *testing.T) {
	t.Parallel()
	h, _ := fakeplugin.JSON(fakeplugin.Options{Block: true, Billing: testkit.NewFakeBillingSource(appfinance.SourcePlugin, time.UTC)})
	src := newSource(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Timeout: 50 * time.Millisecond}, time.UTC)
	_, err := fetch(t, src, 10)
	if got := appfinance.ImportErrorKindOf(err); got != appfinance.ImportErrorUnavailable || !got.Retried() {
		t.Errorf("err = %v, classified %q", err, got)
	}
}

func TestACancelDuringTheCallIsTheContexts(t *testing.T) {
	t.Parallel()
	h, _ := fakeplugin.JSON(fakeplugin.Options{Block: true, Billing: testkit.NewFakeBillingSource(appfinance.SourcePlugin, time.UTC)})
	src := newSource(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, time.UTC)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	month, _ := finance.ParseYearMonth("2026-10")
	_, err := src.Fetch(ctx, appfinance.FetchRequest{Months: []finance.YearMonth{month}, MaxRequests: 10})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context's", err)
	}
}

// A source without caps allows math.MaxInt requests, which the protocol's int32 must not wrap into a cap of nothing.
func TestNoCapStaysNoCap(t *testing.T) {
	t.Parallel()
	var body atomic.Value
	addr := fakeplugin.ServeUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body.Store(string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"outcome":"OUTCOME_OK","requests":2147483647}`)
	}))
	snap, err := fetch(t, newSource(t, kind.Destination{Address: addr}, time.UTC), math.MaxInt)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Requests != math.MaxInt32 {
		t.Errorf("Requests = %d", snap.Requests)
	}
	if got, _ := body.Load().(string); !strings.Contains(got, `"maxRequests":2147483647`) {
		t.Errorf("request = %s", got)
	}
}

// No request allowed makes no call, whatever the plugin would do with it.
func TestNoRequestMakesNoCall(t *testing.T) {
	t.Parallel()
	h, rec := fakeplugin.JSON(fakeplugin.Options{Billing: testkit.NewFakeBillingSource(appfinance.SourcePlugin, time.UTC)})
	_, err := fetch(t, newSource(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, time.UTC), 0)
	if !errors.Is(err, appfinance.ErrRequestCap) || rec.Calls() != 0 {
		t.Errorf("err = %v, %d calls", err, rec.Calls())
	}
}
