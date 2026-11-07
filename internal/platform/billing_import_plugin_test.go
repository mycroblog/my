// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit/financetest"
)

// TestBillingImportsFromAPlugin runs billing_sources = plugin with the real adapter against a fake plugin, on SQLite with
// the inline queue (ADR-0146 §11, §15): the API shows the plugin with the settings' name and time zone, and a plugin that
// fails, or stops answering, or answers nonsense fails as a built-in source does and leaves the earlier import alone.
func TestBillingImportsFromAPlugin(t *testing.T) {
	t.Parallel()
	const zone = "Asia/Tokyo"
	tokyo, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	provider := financetest.NewFakeBillingSource(appfinance.SourcePlugin, tokyo)
	month := finance.YearMonthOf(finance.DateOf(time.Now().In(tokyo)))
	provider.SetMonth(month, financetest.FakeBillingMonth{Rows: []financetest.FakeBillingRow{
		{Service: "Hetzner Cloud", ChargeType: finance.ChargeUsage, Amount: "1.25", Currency: "USD"},
	}})

	// The handler follows what the test wants of the plugin.
	var plugin atomic.Pointer[http.Handler]
	healthy, _ := fakeplugin.JSON(fakeplugin.Options{Billing: provider})
	broken, _ := fakeplugin.JSON(fakeplugin.Options{Billing: provider, Broken: true})
	down := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) })
	use := func(h http.Handler) { plugin.Store(&h) }
	use(healthy)
	address := fakeplugin.ServeUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { (*plugin.Load()).ServeHTTP(w, r) }))

	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline", "MY_BILLING_SOURCES": "plugin",
		"MY_PLUGIN_BILLING_ADDRESS": address, "MY_PLUGIN_BILLING_LOCATION": zone, "MY_PLUGIN_BILLING_PROVIDER": "Hetzner",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

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
		res := spec.Serve(t, h, req) // checks the answer against the specification, the source and the method included
		if out != nil {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
		}
		return res.StatusCode
	}
	status := func() clientapi.BillingImport {
		t.Helper()
		var list clientapi.BillingImportList
		if code := api(http.MethodGet, "/admin/billing_imports", &list); code != http.StatusOK || len(list.Items) != 1 {
			t.Fatalf("GET /admin/billing_imports = %d %+v", code, list)
		}
		return list.Items[0]
	}

	if code := api(http.MethodPost, "/admin/billing_imports/plugin/runs", nil); code != http.StatusAccepted {
		t.Fatalf("POST /admin/billing_imports/plugin/runs = %d", code)
	}
	st := status()
	if st.Source != "plugin" || st.Method != "plugin" || st.Provider != "Hetzner" || st.TimeZone != zone ||
		st.LastRun == nil || st.LastRun.Outcome != clientapi.BillingImportRunOutcomeSucceeded || st.RequestsToday != 1 ||
		st.MaxRequestsPerDay != nil || st.MaxRequestsPerMonth != nil || len(st.Months) != 1 {
		t.Fatalf("status after the import = %+v", st)
	}
	imported := st.Months[0]
	if imported.Totals[0] != (clientapi.Money{Amount: "1.25", Currency: "USD"}) {
		t.Errorf("month = %+v", imported)
	}

	var audit clientapi.AdminAuditEntryPage
	if code := api(http.MethodGet, "/admin/audit_log", &audit); code != http.StatusOK {
		t.Fatalf("GET /admin/audit_log = %d", code)
	}
	labelled := false
	for _, e := range audit.Items {
		labelled = labelled || e.TargetId == "plugin" && e.TargetLabel == "Hetzner"
	}
	if !labelled {
		t.Errorf("audit log = %+v, want an entry of the plugin with its provider's name", audit.Items)
	}

	failing := func(name string, set func(), wantKind clientapi.BillingImportRunErrorKind, retried bool) {
		t.Helper()
		set()
		r := app.Finance.Import(ctx, appfinance.SourcePlugin, "run:"+name)
		if r.IsRetry() != retried {
			t.Errorf("%s: the job retries = %v, want %v", name, r.IsRetry(), retried)
		}
		st := status()
		if st.LastRun == nil || st.LastRun.Outcome != clientapi.BillingImportRunOutcomeFailed || st.LastRun.ErrorKind != wantKind {
			t.Errorf("%s: last run = %+v, want failed with %s", name, st.LastRun, wantKind)
		}
		if len(st.Months) != 1 || st.Months[0].Kind != imported.Kind || st.Months[0].Totals[0] != imported.Totals[0] {
			t.Errorf("%s: the earlier import changed: %+v, was %+v", name, st.Months, imported)
		}
		if st.LastSuccessAt == nil {
			t.Errorf("%s: the last success is gone", name)
		}
	}
	failing("down", func() { use(down) }, clientapi.BillingImportRunErrorKindUnavailable, true)
	failing("broken", func() { use(broken) }, clientapi.BillingImportRunErrorKindInvalidResponse, false)
	failing("refused", func() {
		use(healthy)
		provider.SetFailure(appfinance.ErrBillingPermission)
	}, clientapi.BillingImportRunErrorKindPermission, false)

	if rules := app.Finance.Rules(); len(rules.Sources) != 1 || rules.Sources[0].Provider != "Hetzner" || rules.Sources[0].Location.String() != zone {
		t.Errorf("rules = %+v", rules)
	}
	var totals clientapi.CostTotals
	if code := api(http.MethodGet, "/admin/cost_totals?granularity=month&from="+month.Range().Start.String()+"&to="+month.Range().End.String(), &totals); code != http.StatusOK {
		t.Fatalf("GET /admin/cost_totals = %d", code)
	}
	named := false
	for _, p := range totals.Periods {
		for _, line := range p.Breakdown {
			named = named || line.Provider == "Hetzner"
		}
	}
	if !named {
		t.Errorf("the totals do not name the provider: %+v", totals.Periods)
	}
}

func TestPluginBillingSourceStartsWithThePluginDown(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline", "MY_BILLING_SOURCES": "plugin",
		"MY_PLUGIN_BILLING_ADDRESS":  "unix://" + filepath.Join(t.TempDir(), "nobody.sock"),
		"MY_PLUGIN_BILLING_LOCATION": "UTC", "MY_PLUGIN_BILLING_PROVIDER": "Hetzner",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp with the plugin down: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if r := app.Finance.Import(t.Context(), appfinance.SourcePlugin, "run:down"); !r.IsRetry() {
		t.Errorf("an import with the plugin down is not retried: %+v", r)
	}
}
