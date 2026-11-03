// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeBillingImports has the aws source, imported once and then capped, and the gcp source by
// its budget, never imported; it records the requested imports; a second request is too soon.
type fakeBillingImports struct{ requested []string }

func (f *fakeBillingImports) ImportStatuses(context.Context) ([]appfinance.ImportStatus, error) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	return []appfinance.ImportStatus{{
		Source: "aws", Provider: "AWS", Location: time.UTC, Method: appfinance.MethodCostExplorer,
		LastRun:       &appfinance.Run{ErrorKind: appfinance.ImportErrorRequestCap, OccurredAt: at.Add(time.Hour)},
		LastSuccess:   &appfinance.Run{ErrorKind: appfinance.ImportErrorNone, Requests: 1, OccurredAt: at},
		RequestsToday: 4, RequestsThisMonth: 40, Caps: &appfinance.RequestCaps{PerDay: 4, PerMonth: 120},
		Months: []appfinance.MonthStatus{
			{Month: finance.YearMonth{Year: 2026, Month: time.October}, Kind: finance.KindEstimate, Totals: []finance.Money{{Minor: 3124, Currency: "USD"}}},
			{Month: finance.YearMonth{Year: 2026, Month: time.September}, Kind: finance.KindCorrection, Totals: []finance.Money{{Minor: -5, Currency: "USD"}}},
		},
	}, {
		Source: "gcp", Provider: "Google Cloud", Location: time.UTC, Method: appfinance.MethodBudget,
	}}, nil
}

func (f *fakeBillingImports) RequestImport(_ context.Context, source, byUserID string) error {
	switch {
	case source != "aws":
		return fmt.Errorf("%w: %s", appfinance.ErrUnknownSource, source)
	case len(f.requested) > 0:
		return appfinance.ErrImportTooSoon
	}
	f.requested = append(f.requested, source+" by "+byUserID)
	return nil
}

func TestBillingImports(t *testing.T) {
	imports := &fakeBillingImports{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{
			userOf(aliceAccountID): {identity.PermManageFinance},
			userOf(bobAccountID):   {identity.PermManageSettings},
		}
		o.BillingImports = imports
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)

	res := e.admin(t, http.MethodGet, "/admin/billing_imports", "admin-read", nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET /admin/billing_imports = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	list := decodeJSON[clientapi.BillingImportList](t, res)
	if len(list.Items) != 2 {
		t.Fatalf("items = %+v", list.Items)
	}
	aws := list.Items[0]
	if aws.Source != "aws" || aws.Provider != "AWS" || aws.TimeZone != "UTC" || aws.Method != clientapi.BillingImportMethodCostExplorer || aws.LastRun == nil ||
		aws.LastRun.Outcome != clientapi.BillingImportRunOutcomeFailed || aws.LastRun.ErrorKind != "request_cap" ||
		aws.LastSuccessAt == nil || aws.RequestsToday != 4 || aws.RequestsThisMonth != 40 ||
		aws.MaxRequestsPerDay == nil || *aws.MaxRequestsPerDay != 4 || aws.MaxRequestsPerMonth == nil || *aws.MaxRequestsPerMonth != 120 {
		t.Errorf("aws = %+v", aws)
	}
	if len(aws.Months) != 2 || aws.Months[0].Month != "2026-10" || aws.Months[0].Kind != "estimate" ||
		aws.Months[0].Totals[0] != (clientapi.Money{Amount: "31.24", Currency: "USD"}) ||
		aws.Months[1].Kind != "correction" || aws.Months[1].Totals[0].Amount != "-0.05" {
		t.Errorf("months = %+v", aws.Months)
	}
	if gcp := list.Items[1]; gcp.Source != "gcp" || gcp.Method != clientapi.BillingImportMethodBudget || gcp.LastRun != nil || len(gcp.Months) != 0 {
		t.Errorf("gcp = %+v", gcp)
	}

	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{http.MethodPost, "/admin/billing_imports/aws/runs", "admin", http.StatusAccepted},
		{http.MethodPost, "/admin/billing_imports/aws/runs", "admin", http.StatusTooManyRequests},
		{http.MethodPost, "/admin/billing_imports/gcp/runs", "admin", http.StatusNotFound},
		{http.MethodPost, "/admin/billing_imports/azure/runs", "admin", http.StatusNotFound},
		{http.MethodPost, "/admin/billing_imports/aws/runs", "admin-read", http.StatusForbidden},
		{http.MethodPost, "/admin/billing_imports/aws/runs", "bob-admin", http.StatusForbidden},
		{http.MethodGet, "/admin/billing_imports", "bob-admin", http.StatusForbidden},
	} {
		res := e.admin(t, tc.method, tc.path, tc.token, nil)
		if res.StatusCode != tc.status {
			t.Errorf("%s %s with %s = %d, want %d", tc.method, tc.path, tc.token, res.StatusCode, tc.status)
		}
		if got := res.Header.Get("Retry-After"); tc.status == http.StatusTooManyRequests && (got == "" || got == "0") {
			t.Errorf("%s %s = 429 with Retry-After %q", tc.method, tc.path, got)
		}
	}
	if want := "aws by " + userOf(aliceAccountID); len(imports.requested) != 1 || imports.requested[0] != want {
		t.Errorf("requested = %v, want %s", imports.requested, want)
	}
}
