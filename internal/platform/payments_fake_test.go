// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestE2EFakesPaymentsCheckout starts the test build with payments_provider = fake and profile =
// development, as the demonstration does (ADR-0117 §2): the start warns that this is a test
// build, and a user subscribes to a plan through the fake's checkout page, paying after a failed
// attempt, and gets the plan's role. The same configuration in the production profile does not
// start.
func TestE2EFakesPaymentsCheckout(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline",
		"MY_PROFILE": "development", "MY_PAYMENTS_PROVIDER": "fake",
		// The program needs email; each one fails at once.
		"MY_MAIL": "smtp", "MY_MAIL_FROM": "noreply@example.test", "MY_SMTP_HOST": "127.0.0.1", "MY_SMTP_PORT": closingSMTPServer(t),
	}
	cfg := loadConfig(t, env)
	logs := &syncBuffer{}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.NewTextHandler(logs, nil))})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "this is a test build") {
		t.Errorf("the start logged no warning of the test build:\n%s", logs)
	}
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}

	admin := e.user("alice")
	e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
		http.StatusOK, "POST /admin/roles")
	var plan clientapi.AdminSupporterPlan
	e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, map[string]any{
		"role": "supporter", "name": "Supporter", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
		"annual_discount_percent": 0, "emoji_limit": 0, "rank": 1, "visibility": "public",
	}, &plan), http.StatusOK, "POST /admin/supporter_plans")
	var prog clientapi.AdminSupporterProgram
	if res := e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, &prog); res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("enable = %d %s", res.StatusCode, b)
	}
	if prog.Provider == nil || *prog.Provider != "fake" {
		t.Fatalf("the program's provider: %v", prog.Provider)
	}
	var st clientapi.PaymentStatus
	e.want(e.do(http.MethodGet, "/admin/payment_status", admin.token, nil, &st), http.StatusOK, "GET /admin/payment_status")
	if st.Provider != "fake" || st.Mode != clientapi.PaymentStatusModeTest {
		t.Errorf("the payment status: %+v, want fake in test mode", st)
	}

	bob := e.user("bob")
	var co clientapi.SupporterCheckout
	e.want(e.do(http.MethodPost, "/supporter/checkout", bob.token, map[string]any{"plan_id": plan.Id, "interval": "month"}, &co),
		http.StatusOK, "POST /supporter/checkout")
	page, err := url.Parse(co.RedirectUrl)
	if err != nil || page.Scheme+"://"+page.Host != webOrigin || !strings.HasPrefix(page.Path, "/payments/pages/fake/checkout/") {
		t.Fatalf("redirect_url = %q, want the fake's checkout page on %s", co.RedirectUrl, webOrigin)
	}
	serve := func(method, action string) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if method == http.MethodGet {
			req = httptest.NewRequest(method, page.Path, nil)
		} else {
			req = httptest.NewRequest(method, page.Path, strings.NewReader(url.Values{"action": {action}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		req.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec
	}
	if rec := serve(http.MethodGet, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Subscribe to Supporter") ||
		!strings.Contains(rec.Body.String(), "500 JPY a month") || !strings.Contains(rec.Body.String(), "no money moves") {
		t.Fatalf("GET the checkout page = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(http.MethodPost, "fail"); rec.Code != http.StatusPaymentRequired {
		t.Errorf("fail = %d, want 402", rec.Code)
	}
	rec := serve(http.MethodPost, "pay")
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc, webOrigin+"/settings/supporter?checkout=success&checkout_id=") {
		t.Fatalf("pay = %d to %q, want 303 to the success URL", rec.Code, loc)
	}

	// The web client syncs the subscription when the checkout returns.
	var own clientapi.OwnSupporterSubscription
	e.want(e.do(http.MethodPost, "/supporter/subscription/sync", bob.token, nil, &own), http.StatusOK, "POST /supporter/subscription/sync")
	e.want(e.do(http.MethodGet, "/supporter/subscription", bob.token, nil, &own), http.StatusOK, "GET /supporter/subscription")
	if own.Subscription == nil || own.Subscription.State != clientapi.SupporterSubscriptionStateActive {
		t.Fatalf("bob's subscription after paying: %+v", own.Subscription)
	}
	if roles := e.roleKeys(admin, bob); !slices.Contains(roles, "supporter") {
		t.Errorf("bob's roles %v, want supporter", roles)
	}

	// The platform refuses the fake outside development even with a configuration that passed
	// its own check.
	prod := *cfg
	prod.Profile = "production"
	if _, err := platform.NewApp(t.Context(), &prod, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)}); err == nil ||
		!strings.Contains(err.Error(), "fake needs profile = development") {
		t.Errorf("NewApp in the production profile = %v, want a refusal", err)
	}
}
