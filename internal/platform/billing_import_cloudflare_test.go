// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_billing_cloudflare

package platform_test

import (
	"log/slog"
	"testing"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/platform"
)

// TestCloudflareBillingSource builds billing_sources = cloudflare from its settings alone, without
// calling Cloudflare: the source shows as Cloudflare's billable usage, in UTC, without a cap.
func TestCloudflareBillingSource(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "a-token-the-server-must-not-read")
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_BILLING_SOURCES": "cloudflare",
		"MY_BILLING_CLOUDFLARE_ACCOUNT_ID": "023e105f4ecef8ad9ca31a8372d0c353", "MY_BILLING_CLOUDFLARE_API_TOKEN": "billing-read-token",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	statuses, err := app.Finance.ImportStatuses(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("ImportStatuses = %+v, want the Cloudflare source", statuses)
	}
	st := statuses[0]
	if st.Source != "cloudflare" || st.Provider != "Cloudflare" || st.Method != appfinance.MethodBillableUsage || st.Location.String() != "UTC" || st.Caps != nil {
		t.Errorf("status = %s %s %s %s caps %v; want cloudflare, Cloudflare, billable_usage, UTC, no caps", st.Source, st.Provider, st.Method, st.Location, st.Caps)
	}
}
