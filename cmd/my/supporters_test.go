// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/payment/fake"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// TestAdminSupportersCommands enables the supporter program, lists its plans and subscriptions,
// cancels a subscription, and disables the program from the CLI, every change audited as the
// CLI's. The CLI runs with the Stripe adapter of a test key, which nothing here calls; the
// subscription comes from an App on the fake provider of the same name.
func TestAdminSupportersCommands(t *testing.T) {
	newAdminEnv(t)
	t.Setenv(config.EnvPrefix+"MAIL", "smtp")
	t.Setenv(config.EnvPrefix+"MAIL_FROM", "noreply@social.example")
	t.Setenv(config.EnvPrefix+"MAIL_SMTP_HOST", "127.0.0.1")
	t.Setenv(config.EnvPrefix+"MAIL_SMTP_PORT", "1")
	t.Setenv(config.EnvPrefix+"PAYMENTS", "stripe")
	t.Setenv(config.EnvPrefix+"PAYMENTS_STRIPE_SECRET_KEY", "sk_test_cli")
	t.Setenv(config.EnvPrefix+"PAYMENTS_STRIPE_WEBHOOK_SECRET", "whsec_cli")

	if code, stdout, stderr := runCmd(t, adminCmd, "supporters", "status"); code != 0 ||
		!strings.Contains(stdout, "program: disabled\n") || !strings.Contains(stdout, "payments: stripe\n") {
		t.Fatalf("supporters status = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runCmd(t, adminCmd, "supporters", "enable"); code != 0 || stdout != "enabled the supporter program with stripe\n" {
		t.Fatalf("supporters enable = %d %q %q", code, stdout, stderr)
	}

	// A plan and a subscription to it, on the fake provider, with the jobs run inline.
	cfg, _, err := config.Load(config.Source{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Queue = "inline"
	prov := fake.New(fake.Options{Name: "stripe"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), PaymentProvider: prov})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Close() }()
	ctx := t.Context()
	reg, err := app.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "carol", Email: "carol@example.com", Password: adminPassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Identity.CreateRole(ctx, appidentity.CreateRoleInput{Key: "supporter", Position: 10}); err != nil {
		t.Fatal(err)
	}
	plan, err := app.Supporters.CreatePlan(ctx, appsupporters.PlanInput{RoleKey: "supporter", Currency: "JPY", Terms: supporters.PlanTerms{
		Name: "Gold", MonthlyAmountMinor: 500, Rank: 1, Visibility: supporters.VisibilityPublic,
	}})
	if err != nil {
		t.Fatalf("CreatePlan = %v", err)
	}
	if plan, err = app.Supporters.Plan(ctx, plan.ID); err != nil || !plan.Ready() {
		t.Fatalf("the plan is not ready: %+v, %v", plan, err)
	}
	co, err := app.Supporters.StartCheckout(ctx, appsupporters.CheckoutInput{UserID: reg.User.ID, PlanID: plan.ID, Interval: supporters.IntervalMonth})
	if err != nil {
		t.Fatalf("StartCheckout = %v", err)
	}
	sub, err := app.Supporters.Subscription(ctx, co.SubscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	prov.PutCheckout(appsupporters.CheckoutResult{Ref: sub.Checkout.Ref, Status: appsupporters.CheckoutCompleted, LocalID: sub.ID, Customer: "cus_1", Subscription: "sub_1"})
	prov.PutSubscription(appsupporters.SubscriptionSnapshot{
		Ref: "sub_1", Customer: "cus_1", LocalID: sub.ID, Price: plan.Prices[supporters.IntervalMonth].Ref, Status: appsupporters.StatusActive,
		PeriodStart: time.Now(), PeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	})
	if r := app.Supporters.Sync(ctx, sub.ID); !r.IsDone() {
		t.Fatalf("Sync = %v", r.Err())
	}

	if code, stdout, _ := runCmd(t, adminCmd, "supporters", "plans"); code != 0 ||
		!regexp.MustCompile(plan.ID+` +Gold +1 +500 JPY +0% +public +ready +1 +supporter\n`).MatchString(stdout) {
		t.Errorf("supporters plans = %d\n%s", code, stdout)
	}
	if code, stdout, _ := runCmd(t, adminCmd, "supporters", "subscriptions", "-state", "active"); code != 0 ||
		!regexp.MustCompile(sub.ID+` +`+reg.User.ID+` +Gold +month +active +\S+ +\S+\n`).MatchString(stdout) {
		t.Errorf("supporters subscriptions = %d\n%s", code, stdout)
	}
	if code, _, stderr := runCmd(t, adminCmd, "supporters", "disable"); code != 1 || !strings.Contains(stderr, "subscriptions go on") {
		t.Errorf("supporters disable with a subscription = %d %q", code, stderr)
	}
	if code, stdout, stderr := runCmd(t, adminCmd, "supporters", "cancel", sub.ID); code != 0 || stdout != "canceled subscription "+sub.ID+"\n" {
		t.Fatalf("supporters cancel = %d %q %q", code, stdout, stderr)
	}
	if _, stdout, _ := runCmd(t, adminCmd, "supporters", "subscriptions", "-plan", plan.ID); !regexp.MustCompile(sub.ID + `.* expired `).MatchString(stdout) {
		t.Errorf("supporters subscriptions after the cancellation:\n%s", stdout)
	}
	if code, stdout, stderr := runCmd(t, adminCmd, "supporters", "disable"); code != 0 || stdout != "disabled the supporter program\n" {
		t.Errorf("supporters disable = %d %q %q", code, stdout, stderr)
	}
	code, stdout, _ := runCmd(t, adminCmd, "audit", "list")
	for _, want := range []string{
		` cli +supporter_program_enabled +supporter_program +stripe `,
		` cli +supporter_subscription_ended +supporter_subscription +Gold @carol `,
		` cli +supporter_program_disabled +supporter_program +stripe `,
	} {
		if code != 0 || !regexp.MustCompile(want).MatchString(stdout) {
			t.Errorf("audit list = %d, want %q:\n%s", code, want, stdout)
		}
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"supporters", "cancel", sub.ID}, 1, "already ended"},
		{[]string{"supporters", "cancel", "01890a5d-ac96-774b-bcce-b302099a0000"}, 1, "no subscription has the ID"},
		{[]string{"supporters", "cancel"}, 2, "want 1 argument(s)"},
		{[]string{"supporters", "subscriptions", "-state", "gone"}, 2, `unknown -state "gone"`},
		{[]string{"supporters", "subscriptions", "-limit", "0"}, 2, "-limit must be at least 1"},
		{[]string{"supporters", "retire"}, 2, `unknown subcommand "retire"`},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
