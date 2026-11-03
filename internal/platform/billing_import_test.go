// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestBillingImports enables billing_sources = aws on SQLite with the inline queue and a fake
// source (ADR-0068): the daily schedule is registered, an administrator imports now through the
// admin API, which is audited and runs once an hour, and the status shows the imported month.
func TestBillingImports(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline", "MY_BILLING_SOURCES": "aws",
		"MY_BILLING_AWS_MAX_REQUESTS_PER_DAY": "3",
	})
	aws := testkit.NewFakeBillingSource("aws", time.UTC)
	month := finance.YearMonthOf(finance.DateOf(time.Now().UTC()))
	aws.SetMonth(month, testkit.FakeBillingMonth{Rows: []testkit.FakeBillingRow{
		{Service: "Amazon Simple Storage Service", ChargeType: finance.ChargeUsage, Amount: "1.25", Currency: "USD"},
	}})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{
		Logger: slog.New(slog.DiscardHandler), BillingSources: map[string]appfinance.BillingSource{"aws": aws},
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !slices.ContainsFunc(app.Scheduler.Schedules(), func(s queue.Schedule) bool {
		return s.Name == "finance.billing_import.aws" && s.Every == 24*time.Hour && s.Job.Type == appfinance.JobBillingImport
	}) {
		t.Error("no daily schedule finance.billing_import.aws")
	}
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()
	admin, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	token := issueAccessToken(t, app, admin.User.ID, "read write admin")
	api := func(method, path string, out any) int {
		t.Helper()
		req := httptest.NewRequest(method, clientapi.BasePath+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := spec.Serve(t, h, req)
		if out != nil {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
		}
		return res.StatusCode
	}

	if code := api(http.MethodPost, "/admin/billing_imports/aws/runs", nil); code != http.StatusAccepted {
		t.Fatalf("POST /admin/billing_imports/aws/runs = %d, want 204", code)
	}
	if code := api(http.MethodPost, "/admin/billing_imports/aws/runs", nil); code != http.StatusTooManyRequests {
		t.Errorf("a second import in the hour = %d, want 429", code)
	}
	if code := api(http.MethodPost, "/admin/billing_imports/gcp/runs", nil); code != http.StatusNotFound {
		t.Errorf("an import of gcp, which is not enabled = %d, want 404", code)
	}
	if n := aws.Calls(); n != 1 {
		t.Errorf("the source was called %d times, want once", n)
	}

	var list clientapi.BillingImportList
	if code := api(http.MethodGet, "/admin/billing_imports", &list); code != http.StatusOK || len(list.Items) != 1 {
		t.Fatalf("GET /admin/billing_imports = %d %+v", code, list)
	}
	st := list.Items[0]
	if st.Source != "aws" || st.LastRun == nil || st.LastRun.Outcome != clientapi.BillingImportRunOutcomeSucceeded || st.RequestsToday != 1 ||
		st.MaxRequestsPerDay == nil || *st.MaxRequestsPerDay != 3 || st.MaxRequestsPerMonth == nil || *st.MaxRequestsPerMonth != 120 ||
		len(st.Months) != 1 || st.Months[0].Kind != "estimate" || st.Months[0].Totals[0] != (clientapi.Money{Amount: "1.25", Currency: "USD"}) {
		t.Errorf("status = %+v", st)
	}

	var audit clientapi.AdminAuditEntryPage
	if code := api(http.MethodGet, "/admin/audit_log", &audit); code != http.StatusOK {
		t.Fatalf("GET /admin/audit_log = %d", code)
	}
	found := false
	for _, e := range audit.Items {
		if e.Action == "billing_import_queued" && e.TargetKind == "billing_source" && e.TargetId == "aws" && e.TargetLabel == "AWS" {
			found = true
		}
	}
	if !found {
		var actions []string
		for _, e := range audit.Items {
			actions = append(actions, string(e.Action))
		}
		t.Errorf("audit log = %s, want billing_import_queued on aws", strings.Join(actions, ", "))
	}
}

// A billing source whose adapter cannot be built stops the start.
func TestBillingSourceThatCannotBeBuilt(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_BILLING_SOURCES": "gcp",
		"MY_BILLING_GCP_TABLE": "my-billing.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01",
	})
	cfg.BillingGCPTable = "" // past the validation of the configuration: the adapter refuses it
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err == nil {
		_ = app.Close()
		t.Fatal("NewApp with billing_sources = gcp and no table succeeded")
	}
	if !strings.Contains(err.Error(), "billing_sources = gcp") {
		t.Errorf("NewApp = %v, want the billing source named", err)
	}
}
