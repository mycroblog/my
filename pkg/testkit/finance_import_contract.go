// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"fmt"
	"io"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// The billing imports of FinanceRepositoryContract (ADR-0068): FinanceHarness's service imports
// from e.aws, a FakeBillingSource in UTC, from 2026-10-20 12:00 UTC on.

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

const (
	fakeEC2 = "Amazon Elastic Compute Cloud - Compute"
	fakeS3  = "Amazon Simple Storage Service"
)

func usd(service string, amount string) FakeBillingRow {
	return FakeBillingRow{Service: service, ChargeType: finance.ChargeUsage, Amount: amount, Currency: "USD"}
}

// importNow runs an attempt of the job runKey and checks that it ends as want: "done" or "retry".
func (e *financeEnv) importNow(t *testing.T, runKey, want string) {
	t.Helper()
	r := e.svc.Import(t.Context(), "aws", runKey)
	got := "done"
	switch {
	case r.IsRetry():
		got = "retry"
	case r.IsFail():
		got = "fail"
	}
	if got != want {
		t.Fatalf("Import(%s) = %s (%v), want %s", runKey, got, r.Err(), want)
	}
}

func (e *financeEnv) monthTotals(t *testing.T, from, to string) (map[string]int64, map[string]bool) {
	t.Helper()
	got, err := e.svc.Totals(t.Context(), appfinance.TotalsQuery{
		Granularity: finance.GranularityMonth, From: financeDate(t, from), To: financeDate(t, to), Basis: finance.BasisAmortized,
	})
	if err != nil {
		t.Fatalf("Totals = %v", err)
	}
	amounts, estimated := map[string]int64{}, map[string]bool{}
	for _, p := range got {
		for _, m := range p.Totals {
			if m.Currency == "USD" {
				amounts[p.Period.Label] = m.Minor
			}
		}
		if p.Estimated {
			estimated[p.Period.Label] = true
		}
	}
	return amounts, estimated
}

// heads returns the newest revision of each key as "month service charge: amount kind".
func (e *financeEnv) heads(t *testing.T) map[string]string {
	t.Helper()
	list, err := e.h.Finance.ImportedHeads(t.Context(), "aws", nil)
	if err != nil {
		t.Fatalf("ImportedHeads = %v", err)
	}
	out := map[string]string{}
	for _, h := range list {
		out[h.Key.Month.String()+" "+h.Key.Service+" "+string(h.Key.ChargeType)] = fmt.Sprintf("%d %s", h.Amount.Minor, h.Kind)
	}
	return out
}

func (e *financeEnv) status(t *testing.T) appfinance.ImportStatus {
	t.Helper()
	list, err := e.svc.ImportStatuses(t.Context())
	if err != nil || len(list) != 1 {
		t.Fatalf("ImportStatuses = %+v, %v; want one source", list, err)
	}
	return list[0]
}

// Importing the same billing periods again and again, in retries of a job and in later jobs,
// counts every cost once.
func financeImportIdempotent(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	e.aws.SetMonth(billingMonth(t, "2026-09"), FakeBillingMonth{Final: true, Rows: []FakeBillingRow{usd(fakeEC2, "60.00")}})
	e.aws.SetMonth(billingMonth(t, "2026-10"), FakeBillingMonth{
		Through: financeDate(t, "2026-10-19"), Rows: []FakeBillingRow{usd(fakeEC2, "30.004"), usd(fakeS3, "1.235")},
	})
	e.importNow(t, "schedule:finance.billing_import.aws:1", "done")
	e.importNow(t, "schedule:finance.billing_import.aws:1", "done") // a retry of the job
	e.clock.Advance(time.Hour)
	e.importNow(t, "schedule:finance.billing_import.aws:2", "done")
	e.importNow(t, appfinance.ManualRunKey("aws", e.clock.Now()), "done")

	amounts, estimated := e.monthTotals(t, "2026-08-01", "2026-10-31")
	checkTotals(t, "monthly totals", amounts, map[string]int64{"2026-09": 6000, "2026-10": 3000 + 124})
	if !reflect.DeepEqual(estimated, map[string]bool{"2026-10": true}) {
		t.Errorf("estimated months = %v, want 2026-10 only", estimated)
	}
	if got, want := e.heads(t), map[string]string{
		"2026-09 " + fakeEC2 + " usage": "6000 final",
		"2026-10 " + fakeEC2 + " usage": "3000 estimate",
		"2026-10 " + fakeS3 + " usage":  "124 estimate",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("heads = %v, want %v", got, want)
	}
	// The estimate of October spreads over the days with data: nothing falls after the 19th.
	weeks, err := e.svc.Totals(ctx, appfinance.TotalsQuery{
		Granularity: finance.GranularityWeek, From: financeDate(t, "2026-10-19"), To: financeDate(t, "2026-10-31"), Basis: finance.BasisCash,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range weeks {
		for _, m := range w.Totals {
			if w.Period.Range.Start.After(financeDate(t, "2026-10-19")) && m.Minor != 0 {
				t.Errorf("%s carries %d of an estimate through 2026-10-19", w.Period.Label, m.Minor)
			}
		}
	}

	st := e.status(t)
	if st.Source != "aws" || st.Provider != "AWS" || st.LastRun == nil || !st.LastRun.Succeeded() || st.LastSuccess == nil ||
		st.RequestsToday != 4 || st.RequestsThisMonth != 4 || st.Caps != nil {
		t.Errorf("status = %+v, want 4 successful requests today", st)
	}
	if len(st.Months) != 2 || st.Months[0].Month.String() != "2026-10" || st.Months[0].Kind != finance.KindEstimate ||
		st.Months[1].Kind != finance.KindFinal || !slices.Equal(st.Months[0].Totals, []finance.Money{{Minor: 3124, Currency: "USD"}}) {
		t.Errorf("months = %+v, want 2026-10 estimated at 31.24 USD and 2026-09 final", st.Months)
	}

	before := e.heads(t)
	if err := appfinance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Finance); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if after := e.heads(t); !reflect.DeepEqual(after, before) {
		t.Errorf("heads after the rebuild = %v, want %v", after, before)
	}
}

// An estimate becomes final when the month closes, a later change is a correction, and a key that
// the source no longer reports gets a revision of 0; the totals follow the newest revision.
func financeImportRevisions(t *testing.T, e *financeEnv) {
	october, november := billingMonth(t, "2026-10"), billingMonth(t, "2026-11")
	e.aws.SetMonth(october, FakeBillingMonth{Through: financeDate(t, "2026-10-19"), Rows: []FakeBillingRow{usd(fakeEC2, "10.00"), usd(fakeS3, "2.00")}})
	e.importNow(t, "run:1", "done")
	if req := e.aws.Requests(); len(req) != 1 || !slices.Equal(req[0].Months, []finance.YearMonth{billingMonth(t, "2026-09"), october}) {
		t.Errorf("the first import asked for %+v, want 2026-09 and 2026-10", req)
	}

	e.clock.Set(time.Date(2026, 11, 3, 1, 0, 0, 0, time.UTC))
	e.aws.SetMonth(october, FakeBillingMonth{Final: true, Rows: []FakeBillingRow{usd(fakeEC2, "12.00"), usd(fakeS3, "2.00")}})
	e.aws.SetMonth(november, FakeBillingMonth{Through: financeDate(t, "2026-11-02"), Rows: []FakeBillingRow{usd(fakeEC2, "1.00")}})
	e.importNow(t, "run:2", "done")
	amounts, estimated := e.monthTotals(t, "2026-10-01", "2026-11-30")
	checkTotals(t, "totals after October closed", amounts, map[string]int64{"2026-10": 1400, "2026-11": 100})
	if !reflect.DeepEqual(estimated, map[string]bool{"2026-11": true}) {
		t.Errorf("estimated months = %v, want 2026-11 only", estimated)
	}

	e.clock.Set(time.Date(2026, 11, 10, 1, 0, 0, 0, time.UTC))
	e.aws.SetMonth(october, FakeBillingMonth{Final: true, Rows: []FakeBillingRow{
		usd(fakeEC2, "12.00"), {Service: fakeEC2, ChargeType: finance.ChargeUsage, Amount: "-0.50", Currency: "USD"},
	}})
	e.importNow(t, "run:3", "done")
	if got, want := e.heads(t), map[string]string{
		"2026-10 " + fakeEC2 + " usage": "1150 correction",
		"2026-10 " + fakeS3 + " usage":  "0 correction",
		"2026-11 " + fakeEC2 + " usage": "100 estimate",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("heads after the correction = %v, want %v", got, want)
	}
	amounts, _ = e.monthTotals(t, "2026-10-01", "2026-11-30")
	checkTotals(t, "totals after the correction", amounts, map[string]int64{"2026-10": 1150, "2026-11": 100})
	if st := e.status(t); len(st.Months) != 2 || st.Months[1].Kind != finance.KindCorrection {
		t.Errorf("months = %+v, want October corrected", st.Months)
	}

	// After the 15th, October is final and no longer fetched: what the source says of it is not
	// read any more.
	e.clock.Set(time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC))
	e.aws.SetMonth(october, FakeBillingMonth{Final: true, Rows: []FakeBillingRow{usd(fakeEC2, "99.00")}})
	e.importNow(t, "run:4", "done")
	req := e.aws.Requests()
	if last := req[len(req)-1]; !slices.Equal(last.Months, []finance.YearMonth{november}) {
		t.Errorf("the import of 2026-11-16 asked for %v, want 2026-11 only", last.Months)
	}
	amounts, _ = e.monthTotals(t, "2026-10-01", "2026-11-30")
	checkTotals(t, "totals after the 15th", amounts, map[string]int64{"2026-10": 1150, "2026-11": 100})
}

// The caps count every attempt's requests by UTC day and month: an attempt with none left calls
// nothing, is recorded as request_cap, and is not retried; one that reaches the cap within its
// pages records the failure and nothing else.
func financeImportCaps(t *testing.T, e *financeEnv) {
	october := billingMonth(t, "2026-10")
	e.aws.SetMonth(october, FakeBillingMonth{Through: financeDate(t, "2026-10-19"), Rows: []FakeBillingRow{usd(fakeS3, "1.00")}})
	e.caps["aws"] = appfinance.RequestCaps{PerDay: 2, PerMonth: 3}
	e.importNow(t, "run:1", "done")
	e.importNow(t, "run:2", "done")
	e.importNow(t, "run:3", "done")
	if n := e.aws.Calls(); n != 2 {
		t.Errorf("the source was called %d times, want 2 under the daily cap", n)
	}
	st := e.status(t)
	if st.LastRun == nil || st.LastRun.ErrorKind != appfinance.ImportErrorRequestCap || st.LastRun.Requests != 0 ||
		st.RequestsToday != 2 || st.Caps == nil || st.Caps.PerDay != 2 {
		t.Errorf("status at the daily cap = %+v, last run %+v", st, st.LastRun)
	}
	e.clock.Set(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)) // the next UTC day
	e.importNow(t, "run:4", "done")
	e.importNow(t, "run:5", "done")
	if n := e.aws.Calls(); n != 3 {
		t.Errorf("the source was called %d times, want 3 under the monthly cap", n)
	}
	if st := e.status(t); st.LastRun.ErrorKind != appfinance.ImportErrorRequestCap || st.RequestsThisMonth != 3 || st.RequestsToday != 1 {
		t.Errorf("status at the monthly cap = %+v", st)
	}
	e.clock.Set(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) // the next UTC month
	e.importNow(t, "run:6", "done")
	if n := e.aws.Calls(); n != 4 {
		t.Errorf("the source was called %d times in a new month, want 4", n)
	}

	// Three pages with one request left: the attempt stops at the cap and records nothing but
	// its failure.
	e.caps["aws"] = appfinance.RequestCaps{PerDay: 2, PerMonth: 120}
	e.aws.SetPages(3)
	before := e.heads(t)
	e.aws.SetMonth(october, FakeBillingMonth{Final: true, Rows: []FakeBillingRow{usd(fakeS3, "5.00")}})
	e.importNow(t, "run:7", "done")
	if got := e.heads(t); !reflect.DeepEqual(got, before) {
		t.Errorf("heads after an attempt cut at the cap = %v, want %v", got, before)
	}
	st = e.status(t)
	if st.LastRun.ErrorKind != appfinance.ImportErrorRequestCap || st.LastRun.Requests != 1 || st.RequestsToday != 2 {
		t.Errorf("status after an attempt cut at the cap = %+v, last run %+v", st, st.LastRun)
	}
}

// Every failed attempt is recorded alone and counted in my.billing.imports with its kind;
// throttling and unavailability are retried, the others wait for the next import.
func financeImportFailures(t *testing.T, e *financeEnv) {
	e.aws.SetMonth(billingMonth(t, "2026-10"), FakeBillingMonth{Rows: []FakeBillingRow{usd(fakeS3, "1.00")}})
	failing := func(want bool) {
		t.Helper()
		if got, err := e.svc.FailingSources(t.Context()); err != nil || !maps.Equal(got, map[string]bool{"aws": want}) {
			t.Errorf("FailingSources = %v, %v; want aws %v", got, err, want)
		}
	}
	failing(false) // no attempt yet
	for _, tc := range []struct {
		err  error
		kind appfinance.ImportErrorKind
		want string
	}{
		{appfinance.ErrBillingThrottled, appfinance.ImportErrorThrottled, "retry"},
		{appfinance.ErrBillingUnavailable, appfinance.ImportErrorUnavailable, "retry"},
		{appfinance.ErrBillingPermission, appfinance.ImportErrorPermission, "done"},
		{appfinance.ErrBillingNotEnabled, appfinance.ImportErrorNotEnabled, "done"},
		{fmt.Errorf("%w: a bad answer", appfinance.ErrInvalidBillingResponse), appfinance.ImportErrorInvalidResponse, "done"},
	} {
		e.aws.SetFailure(tc.err)
		e.importNow(t, "run:"+string(tc.kind), tc.want)
		if st := e.status(t); st.LastRun == nil || st.LastRun.ErrorKind != tc.kind || st.LastRun.Requests != 1 || st.LastSuccess != nil {
			t.Errorf("after %v: last run %+v, last success %+v", tc.err, st.LastRun, st.LastSuccess)
		}
		failing(true)
		e.clock.Advance(time.Second)
	}
	if got := e.heads(t); len(got) != 0 {
		t.Errorf("failed attempts recorded %v", got)
	}
	e.aws.SetFailure(nil)
	e.importNow(t, "run:ok", "done")
	if st := e.status(t); st.LastSuccess == nil || st.LastRun.ID != st.LastSuccess.ID || st.RequestsToday != 6 {
		t.Errorf("after a success: %+v", st)
	}
	failing(false)
	for _, kind := range []appfinance.ImportErrorKind{
		appfinance.ImportErrorThrottled, appfinance.ImportErrorUnavailable, appfinance.ImportErrorPermission,
		appfinance.ImportErrorNotEnabled, appfinance.ImportErrorInvalidResponse,
	} {
		match := map[string]string{
			appfinance.LabelBillingSource: "aws", appfinance.LabelOutcome: appfinance.ImportOutcomeFailed, appfinance.LabelErrorType: string(kind),
		}
		if got := e.meter.Sum(appfinance.BillingImports.Name, match); got != 1 {
			t.Errorf("%s%v = %v, want 1", appfinance.BillingImports.Name, match, got)
		}
	}
	success := map[string]string{appfinance.LabelBillingSource: "aws", appfinance.LabelOutcome: appfinance.ImportOutcomeSuccess}
	if got := e.meter.Sum(appfinance.BillingImports.Name, success); got != 1 {
		t.Errorf("%s%v = %v, want 1", appfinance.BillingImports.Name, success, got)
	}
	if err := e.meter.Err(); err != nil {
		t.Error(err)
	}
}
