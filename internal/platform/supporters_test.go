// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/supporters"
	domain "github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// newSupportersEnv is an App on SQLite with the inline queue, email (to a server that closes
// every connection at once), and the fake payment provider.
func newSupportersEnv(t *testing.T) (*accountsEnv, *fake.Provider) {
	t.Helper()
	return newSupportersEnvOn(t, "sqlite")
}

// newSupportersEnvOn is newSupportersEnv on a dialect: sqlite or postgres (pgmem in pull
// requests).
func newSupportersEnvOn(t *testing.T, dialect string) (*accountsEnv, *fake.Provider) {
	t.Helper()
	dbURL := newDatabase(t, true)
	if dialect == "postgres" {
		dbURL = newPostgresDatabase(t)
	}
	smtpPort := closingSMTPServer(t)
	cfg := loadConfig(t, map[string]string{
		"MY_DB": dialect, "MY_DATABASE_URL": dbURL, "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline",
		"MY_MAIL": "smtp", "MY_MAIL_FROM": "noreply@example.test", "MY_SMTP_HOST": "127.0.0.1", "MY_SMTP_PORT": smtpPort,
	})
	prov := fake.New(fake.Options{Name: "stripe"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), PaymentProvider: prov})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, prov
}

// closingSMTPServer listens on a local port and closes every connection at once, so that each
// email fails at once as a temporary error; a port that nobody listens on may instead drop the
// connection attempt until the send times out. It returns the port.
func closingSMTPServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

// TestSupporterProgram drives the supporter program on SQLite (ADR-0072): nothing shows while it
// is disabled; an administrator creates a plan on a role that then nobody grants by hand,
// enables the program, and sees the subscriptions; disabling it is refused while one goes on;
// the deletion of the subscriber's account ends the subscription at once, at the provider too;
// and every administrative step is in the audit log.
func TestSupporterProgram(t *testing.T) {
	t.Parallel()
	e, prov := newSupportersEnv(t)
	ctx := t.Context()
	admin := e.user("alice")

	var inst clientapi.Instance
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
	if inst.SupportersEnabled || inst.CommerceDisclosureUrl != nil {
		t.Errorf("a new instance: supporters_enabled %v, commerce_disclosure_url %v", inst.SupportersEnabled, inst.CommerceDisclosureUrl)
	}
	e.want(e.do(http.MethodGet, "/instance/supporter_plans", "", nil, nil), http.StatusNotFound, "the plans while disabled")
	var prog clientapi.AdminSupporterProgram
	e.want(e.do(http.MethodGet, "/admin/supporter_program", admin.token, nil, &prog), http.StatusOK, "GET /admin/supporter_program")
	if prog.Enabled || !prog.CanEnable || prog.ConfiguredProvider == nil || *prog.ConfiguredProvider != "stripe" ||
		!slices.Contains(prog.Warnings, clientapi.SupporterProgramWarningNoCommerceDisclosure) {
		t.Errorf("the program by default: %+v", prog)
	}

	e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
		http.StatusOK, "POST /admin/roles")
	plan := map[string]any{
		"role": "moderator", "name": "Supporter", "description": "Thank you", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
		"annual_discount_percent": 10, "emoji_limit": 5, "rank": 1, "visibility": "public",
	}
	res := e.do(http.MethodPost, "/admin/supporter_plans", admin.token, plan, nil)
	if typ := e.want(res, http.StatusUnprocessableEntity, "a plan on the moderator role"); typ != clientapi.ProblemTypeInvalidSupporterPlan {
		t.Errorf("problem %s", typ)
	}
	plan["role"] = "supporter"
	var created clientapi.AdminSupporterPlan
	e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, plan, &created), http.StatusOK, "POST /admin/supporter_plans")
	if created.Name != "Supporter" || created.YearlyPrice != (clientapi.Money{Amount: "5400", Currency: "JPY"}) || created.Role.Key != "supporter" {
		t.Errorf("created %+v", created)
	}
	var plans clientapi.AdminSupporterPlanList
	e.want(e.do(http.MethodGet, "/admin/supporter_plans", admin.token, nil, &plans), http.StatusOK, "GET /admin/supporter_plans")
	if len(plans.Items) != 1 || !plans.Items[0].PricesReady {
		t.Fatalf("plans %+v", plans)
	}
	// Nobody grants the plan's role by hand.
	res = e.do(http.MethodPut, "/admin/accounts/"+admin.id+"/roles/supporter", admin.token, nil, nil)
	if typ := e.want(res, http.StatusConflict, "a grant of the plan's role"); typ != clientapi.ProblemTypeRoleManagedByPlan {
		t.Errorf("problem %s", typ)
	}

	e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, &prog), http.StatusOK, "enable")
	if !prog.Enabled || prog.Provider == nil {
		t.Fatalf("enabled %+v", prog)
	}
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
	var offered clientapi.SupporterPlanList
	e.want(e.do(http.MethodGet, "/instance/supporter_plans", "", nil, &offered), http.StatusOK, "the plans")
	if !inst.SupportersEnabled || len(offered.Items) != 1 || offered.Items[0].MonthlyPrice.Amount != "500" {
		t.Fatalf("enabled: %v, offered %+v", inst.SupportersEnabled, offered)
	}

	// Bob subscribes; the provider completes the checkout.
	bob := e.user("bob")
	subID := subscribeOnFake(t, e.app, prov, bob.id, created.Id, "sub_1")
	var subs clientapi.AdminSupporterSubscriptionPage
	e.want(e.do(http.MethodGet, "/admin/supporter_subscriptions?state=active", admin.token, nil, &subs), http.StatusOK, "the subscriptions")
	if len(subs.Items) != 1 || subs.Items[0].Username == nil || *subs.Items[0].Username != "bob" || subs.Items[0].PeriodEndsAt == nil {
		t.Fatalf("subscriptions %+v", subs)
	}
	if roles := e.roleKeys(admin, bob); !slices.Contains(roles, "supporter") {
		t.Errorf("bob's roles %v", roles)
	}
	res = e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": false}, nil)
	if typ := e.want(res, http.StatusConflict, "disable with a subscription"); typ != clientapi.ProblemTypeLiveSubscriptions {
		t.Errorf("problem %s", typ)
	}

	// Bob deletes his account: the subscription ends at once, at the provider too.
	if err := e.app.Accounts.DeleteBySelf(ctx, bob.id); err != nil {
		t.Fatalf("DeleteBySelf = %v", err)
	}
	if v, err := e.app.Supporters.Subscription(ctx, subID); err != nil || v.Current.State != domain.StateExpired ||
		v.Current.Reason != domain.ReasonAccountClosed {
		t.Fatalf("after the deletion: %+v, %v", v.Current, err)
	}
	if s, _ := prov.Subscription("sub_1"); s.Status != supporters.StatusCanceled {
		t.Errorf("the provider's subscription after the deletion: %s", s.Status)
	}

	e.want(e.do(http.MethodPost, "/admin/supporter_plans/"+created.Id+"/retire", admin.token, nil, nil), http.StatusNoContent, "retire")
	res = e.do(http.MethodPost, "/admin/supporter_plans/"+created.Id+"/retire", admin.token, nil, nil)
	if typ := e.want(res, http.StatusConflict, "retire again"); typ != clientapi.ProblemTypeSupporterPlanRetired {
		t.Errorf("problem %s", typ)
	}
	e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": false}, &prog), http.StatusOK, "disable")
	e.want(e.do(http.MethodGet, "/instance/supporter_plans", "", nil, nil), http.StatusNotFound, "the plans after disabling")

	var log clientapi.AdminAuditEntryPage
	e.want(e.do(http.MethodGet, "/admin/audit_log?limit=40", admin.token, nil, &log), http.StatusOK, "GET /admin/audit_log")
	var actions []clientapi.AdminAuditAction
	for _, entry := range log.Items {
		if strings.HasPrefix(string(entry.Action), "supporter_") {
			actions = append(actions, entry.Action)
		}
	}
	want := []clientapi.AdminAuditAction{
		clientapi.AdminAuditActionSupporterProgramDisabled, clientapi.AdminAuditActionSupporterPlanRetired,
		clientapi.AdminAuditActionSupporterProgramEnabled, clientapi.AdminAuditActionSupporterPlanCreated,
	}
	if !slices.Equal(actions, want) {
		t.Errorf("audit log %v, want %v", actions, want)
	}
}

// TestSupporterPlanAmountComesBack changes a plan's monthly amount A -> B -> A on both dialects:
// each change makes the plan ready again with the new price as its current one, an amount the
// plan had before comes back as a new price beside the old one, the price heads stay what a
// rebuild derives, and the subscription on the first price keeps it (ADR-0072 §3).
func TestSupporterPlanAmountComesBack(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()
			e, prov := newSupportersEnvOn(t, dialect)
			ctx := t.Context()
			admin := e.user("alice")
			e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
				http.StatusOK, "POST /admin/roles")
			var plan clientapi.AdminSupporterPlan
			e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, map[string]any{
				"role": "supporter", "name": "Supporter", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
				"annual_discount_percent": 0, "emoji_limit": 0, "rank": 1, "visibility": "public",
			}, &plan), http.StatusOK, "POST /admin/supporter_plans")
			e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, nil), http.StatusOK, "enable")
			bob := e.user("bob")
			subID := subscribeOnFake(t, e.app, prov, bob.id, plan.Id, "sub_1")
			first, err := e.app.Supporters.Plan(ctx, plan.Id)
			if err != nil {
				t.Fatal(err)
			}
			firstPrice := first.Prices[domain.IntervalMonth]
			seen := map[string]bool{firstPrice.ID: true}

			for _, amount := range []string{"800", "500", "800"} {
				e.want(e.do(http.MethodPatch, "/admin/supporter_plans/"+plan.Id, admin.token,
					map[string]any{"monthly_price": map[string]string{"amount": amount, "currency": "JPY"}}, nil),
					http.StatusOK, "PATCH the monthly amount to "+amount)
				var plans clientapi.AdminSupporterPlanList
				e.want(e.do(http.MethodGet, "/admin/supporter_plans", admin.token, nil, &plans), http.StatusOK, "GET /admin/supporter_plans")
				if len(plans.Items) != 1 || !plans.Items[0].PricesReady || plans.Items[0].MonthlyPrice.Amount != amount {
					t.Fatalf("after %s: plans %+v", amount, plans.Items)
				}
				p, err := e.app.Supporters.Plan(ctx, plan.Id)
				if err != nil {
					t.Fatal(err)
				}
				price := p.Prices[domain.IntervalMonth]
				if !p.Offered() || price.Archived || seen[price.ID] || strconv.FormatInt(price.AmountMinor, 10) != amount {
					t.Fatalf("after %s: offered %v, the monthly price %+v (seen before: %v)", amount, p.Offered(), price, seen[price.ID])
				}
				seen[price.ID] = true
			}

			for _, pr := range e.app.Projections.List() {
				if pr.Name != "supporters" {
					continue
				}
				r, err := e.app.Projections.Verify(ctx, pr)
				if err != nil || r.Drifted() {
					t.Errorf("Verify(supporters) = %+v, %v; want no drift", r.Tables, err)
				}
			}
			v, err := e.app.Supporters.Subscription(ctx, subID)
			if err != nil {
				t.Fatal(err)
			}
			if v.Price.ID != firstPrice.ID || v.Price.AmountMinor != 500 || !v.Price.Archived {
				t.Errorf("the subscription's price %+v, want the first price %+v, archived", v.Price, firstPrice)
			}
			if s, _ := prov.Subscription("sub_1"); s.Status != supporters.StatusActive || s.Price != firstPrice.Ref {
				t.Errorf("the provider's subscription %+v", s)
			}
		})
	}
}

// subscribeOnFake subscribes the user of a local account to a plan, completes the checkout at
// the fake provider with its subscription ref, and applies it as the return from the checkout
// does. It returns our subscription.
func subscribeOnFake(t *testing.T, app *platform.App, prov *fake.Provider, accountID, planID string, ref supporters.SubscriptionRef) string {
	t.Helper()
	ctx := t.Context()
	co, err := app.Supporters.StartCheckout(ctx, supporters.CheckoutInput{UserID: userOfAccount(t, app, accountID), PlanID: planID, Interval: domain.IntervalMonth})
	if err != nil {
		t.Fatalf("StartCheckout = %v", err)
	}
	sub, err := app.Supporters.Subscription(ctx, co.SubscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := app.Supporters.Plan(ctx, planID)
	if err != nil {
		t.Fatal(err)
	}
	customer := supporters.CustomerRef("cus_" + string(ref))
	prov.PutCheckout(supporters.CheckoutResult{Ref: sub.Checkout.Ref, Status: supporters.CheckoutCompleted, LocalID: co.SubscriptionID, Customer: customer, Subscription: ref})
	prov.PutSubscription(supporters.SubscriptionSnapshot{
		Ref: ref, Customer: customer, LocalID: co.SubscriptionID, Price: p.Prices[domain.IntervalMonth].Ref, Status: supporters.StatusActive,
		PeriodStart: time.Now(), PeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	})
	if r := app.Supporters.Sync(ctx, co.SubscriptionID); !r.IsDone() {
		t.Fatalf("Sync = %v", r.Err())
	}
	return co.SubscriptionID
}

// TestSupporterSuspension cancels the subscription of a suspended account at the end of its
// period, keeping its role until then (ADR-0072 §10, S1).
func TestSupporterSuspension(t *testing.T) {
	t.Parallel()
	e, prov := newSupportersEnv(t)
	ctx := t.Context()
	admin := e.user("alice")
	e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
		http.StatusOK, "POST /admin/roles")
	var plan clientapi.AdminSupporterPlan
	e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, map[string]any{
		"role": "supporter", "name": "Supporter", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
		"annual_discount_percent": 0, "emoji_limit": 0, "rank": 1, "visibility": "public",
	}, &plan), http.StatusOK, "POST /admin/supporter_plans")
	e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, nil), http.StatusOK, "enable")
	carol := e.user("carol")
	subID := subscribeOnFake(t, e.app, prov, carol.id, plan.Id, "sub_9")
	e.want(e.do(http.MethodPost, "/admin/accounts/"+carol.id+"/actions", admin.token, map[string]any{"kind": "suspend", "public_reason": "rules"}, nil),
		http.StatusOK, "suspend")
	v, err := e.app.Supporters.Subscription(ctx, subID)
	if err != nil || v.Current.State != domain.StateCanceling || v.Current.Reason != domain.ReasonAccountSuspended {
		t.Fatalf("after the suspension: %+v, %v", v.Current, err)
	}
	if s, _ := prov.Subscription("sub_9"); !s.CancelAtPeriodEnd || s.Status != supporters.StatusActive {
		t.Errorf("the provider's subscription: %+v", s)
	}
	if roles := e.roleKeys(admin, carol); !slices.Contains(roles, "supporter") {
		t.Errorf("carol's roles %v", roles)
	}
}

// userOfAccount returns the user of a local account.
func userOfAccount(t *testing.T, app *platform.App, accountID string) string {
	t.Helper()
	a, err := app.Accounts.FindByID(t.Context(), accountID)
	if err != nil || a.Local == nil {
		t.Fatalf("account %s: %v", accountID, err)
	}
	return a.Local.UserID
}

// roleKeys returns the roles of a local account as an administrator reads them.
func (e *accountsEnv) roleKeys(admin, of testUser) []string {
	e.t.Helper()
	var list clientapi.AdminRoleList
	e.want(e.do(http.MethodGet, "/admin/accounts/"+of.id+"/roles", admin.token, nil, &list), http.StatusOK, "GET roles")
	var out []string
	for _, r := range list.Items {
		out = append(out, r.Key)
	}
	return out
}

// TestCommerceDisclosurePage serves the commerce disclosures at GET /instance/documents/commerce_disclosure once an
// administrator writes them, and links to them from GET /instance (ADR-0072 §13).
func TestCommerceDisclosurePage(t *testing.T) {
	t.Parallel()
	e, _ := newSupportersEnv(t)
	admin := e.user("alice")
	page := func() (int, clientapi.InstanceDocument) {
		var doc clientapi.InstanceDocument
		res := e.do(http.MethodGet, "/instance/documents/commerce_disclosure", "", nil, &doc)
		return res.StatusCode, doc
	}
	if code, _ := page(); code != http.StatusNotFound {
		t.Errorf("without disclosures: %d", code)
	}
	e.want(e.do(http.MethodPut, "/admin/instance_texts/commerce_disclosure", admin.token, map[string]any{"body": "# Seller\n\nExample Inc."}, nil),
		http.StatusOK, "PUT commerce_disclosure")
	code, doc := page()
	if code != http.StatusOK || !strings.Contains(doc.Html, "Example Inc.") {
		t.Errorf("with disclosures: %d %q", code, doc.Html)
	}
	var inst clientapi.Instance
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
	if inst.CommerceDisclosureUrl == nil || !strings.HasSuffix(*inst.CommerceDisclosureUrl, "/about/commerce") {
		t.Errorf("commerce_disclosure_url %v", inst.CommerceDisclosureUrl)
	}
}

// TestSupporterSubscriptionAndList drives a user's subscription and the supporter list on
// SQLite (ADR-0072 §6, §18, ADR-0071 §4): the user subscribes through the provider's checkout,
// which returns to the web client, and cannot subscribe to another plan while subscribed; the
// list shows only the members who chose to be listed, by plan, with their accounts and never an
// amount or a date; supporter_list_visibility decides who reads it; and the end of the
// subscription takes the member off it.
func TestSupporterSubscriptionAndList(t *testing.T) {
	t.Parallel()
	e, prov := newSupportersEnv(t)
	ctx := t.Context()
	admin := e.user("alice")
	bob, carol := e.user("bob"), e.user("carol")
	e.want(e.do(http.MethodGet, "/supporter/subscription", bob.token, nil, nil), http.StatusNotFound, "the subscription while disabled")

	e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
		http.StatusOK, "POST /admin/roles")
	var plan clientapi.AdminSupporterPlan
	e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, map[string]any{
		"role": "supporter", "name": "Supporter", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
		"annual_discount_percent": 0, "emoji_limit": 0, "rank": 1, "visibility": "public",
	}, &plan), http.StatusOK, "POST /admin/supporter_plans")
	e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, nil), http.StatusOK, "enable")

	var own clientapi.OwnSupporterSubscription
	e.want(e.do(http.MethodGet, "/supporter/subscription", bob.token, nil, &own), http.StatusOK, "the subscription before subscribing")
	if own.Subscription != nil || own.IneligibleReason != nil {
		t.Fatalf("before subscribing: %+v", own)
	}
	choice := map[string]any{"plan_id": plan.Id, "interval": "month"}
	var co clientapi.SupporterCheckout
	e.want(e.do(http.MethodPost, "/supporter/checkout", bob.token, choice, &co), http.StatusOK, "POST /supporter/checkout")
	sub, err := e.app.Supporters.Subscription(ctx, co.SubscriptionId)
	if err != nil || sub.Checkout == nil || co.RedirectUrl == "" {
		t.Fatalf("the checkout: %+v, %+v, %v", co, sub, err)
	}
	// The provider completes the checkout; its return asks for a re-read.
	prov.PutCheckout(supporters.CheckoutResult{Ref: sub.Checkout.Ref, Status: supporters.CheckoutCompleted, LocalID: co.SubscriptionId, Customer: "cus_b", Subscription: "sub_b"})
	prov.PutSubscription(supporters.SubscriptionSnapshot{
		Ref: "sub_b", Customer: "cus_b", LocalID: co.SubscriptionId, Price: sub.Price.Ref, Status: supporters.StatusActive,
		PeriodStart: time.Now(), PeriodEnd: time.Now().Add(30 * 24 * time.Hour),
	})
	e.want(e.do(http.MethodPost, "/supporter/subscription/sync", bob.token, nil, nil), http.StatusOK, "POST /supporter/subscription/sync")
	e.want(e.do(http.MethodGet, "/supporter/subscription", bob.token, nil, &own), http.StatusOK, "the subscription after the return")
	if own.Subscription == nil || own.Subscription.State != clientapi.SupporterSubscriptionStateActive || own.Subscription.PlanName != "Supporter" {
		t.Fatalf("after the return: %+v", own.Subscription)
	}
	// The payment reaches the income records with our subscription (ADR-0073 §1); the public
	// income shows the plan's members and never the payment.
	paid := time.Now().UTC()
	day := supporters.DateOf(paid)
	if err := e.app.Supporters.RecordMoney(ctx, "stripe", []supporters.MoneyMovement{{
		ID: "txn_b", Kind: supporters.MoneyPayment, Subscription: "sub_b", OccurredAt: paid, OccurredOn: day,
		Covers: supporters.DateRange{Start: day, End: day}, Gross: "500", Fee: "18", Net: "482", Currency: "JPY", Final: true,
	}}); err != nil {
		t.Fatalf("RecordMoney = %v", err)
	}
	var movements clientapi.IncomeMovementPage
	e.want(e.do(http.MethodGet, "/admin/income_movements", admin.token, nil, &movements), http.StatusOK, "GET /admin/income_movements")
	if len(movements.Items) != 1 || movements.Items[0].SubscriptionId == nil || *movements.Items[0].SubscriptionId != co.SubscriptionId ||
		movements.Items[0].AccountId == nil || movements.Items[0].Username == nil || *movements.Items[0].Username != "bob" ||
		movements.Items[0].Net.Amount != "482" {
		t.Errorf("income movements = %+v, want bob's payment", movements.Items)
	}
	e.want(e.do(http.MethodPatch, "/admin/settings", admin.token, map[string]any{"values": map[string]string{"income_visibility": "plans"}}, nil),
		http.StatusOK, "set income_visibility plans")
	var income clientapi.IncomeTotals
	q := "/instance/income_totals?granularity=year&from=" + day.String() + "&to=" + day.String()
	e.want(e.do(http.MethodGet, q, "", nil, &income), http.StatusOK, "GET /instance/income_totals")
	if len(income.Periods) != 1 || len(income.Periods[0].Net) != 1 || income.Periods[0].Net[0].Amount != "482" ||
		income.Plans == nil || len(*income.Plans) != 1 || (*income.Plans)[0].Members != 1 || income.Periods[0].Balance != nil {
		t.Errorf("public income = %+v, want 482 and one member, without a balance while the costs are private", income)
	}

	// One plan at a time.
	res := e.do(http.MethodPost, "/supporter/checkout", bob.token, choice, nil)
	if typ := e.want(res, http.StatusConflict, "a second checkout"); typ != clientapi.ProblemTypeSupporterSubscribed {
		t.Errorf("problem %s", typ)
	}
	// Carol, who does not subscribe, has no choice of the list.
	res = e.do(http.MethodPut, "/preferences/supporter_listing", carol.token, map[string]any{"listed": true}, nil)
	e.want(res, http.StatusNotFound, "carol's listing")

	setVisibility := func(v string) {
		t.Helper()
		e.want(e.do(http.MethodPatch, "/admin/settings", admin.token, map[string]any{"values": map[string]string{"supporter_list_visibility": v}}, nil),
			http.StatusOK, "set supporter_list_visibility "+v)
	}
	list := func(token string) clientapi.SupporterListPage {
		t.Helper()
		var page clientapi.SupporterListPage
		e.want(e.do(http.MethodGet, "/instance/supporters", token, nil, &page), http.StatusOK, "GET /instance/supporters")
		return page
	}
	e.want(e.do(http.MethodGet, "/instance/supporters", "", nil, nil), http.StatusNotFound, "the list while off")
	setVisibility("public")
	if page := list(""); len(page.Items) != 0 || len(page.Plans) != 1 || page.Plans[0].Name != "Supporter" {
		t.Errorf("nobody is listed before choosing to be: %+v", page)
	}
	var listing clientapi.SupporterListing
	e.want(e.do(http.MethodPut, "/preferences/supporter_listing", bob.token, map[string]any{"listed": true}, &listing), http.StatusOK, "bob's listing")
	if page := list(""); len(page.Items) != 1 || page.Items[0].Account.Username != "bob" || page.Items[0].PlanId != plan.Id {
		t.Errorf("the list after bob chose to be listed: %+v", page)
	}
	setVisibility("signed_in")
	e.want(e.do(http.MethodGet, "/instance/supporters", "", nil, nil), http.StatusUnauthorized, "the list of the signed-in users, anonymous")
	if page := list(carol.token); len(page.Items) != 1 {
		t.Errorf("the list of the signed-in users: %+v", page)
	}

	// The subscription ends: bob leaves the list.
	e.want(e.do(http.MethodPost, "/supporter/subscription/cancel", bob.token, nil, nil), http.StatusOK, "cancel")
	prov.PutSubscription(supporters.SubscriptionSnapshot{
		Ref: "sub_b", Customer: "cus_b", LocalID: co.SubscriptionId, Price: sub.Price.Ref, Status: supporters.StatusCanceled,
		PeriodStart: time.Now(), PeriodEnd: time.Now(),
	})
	if r := e.app.Supporters.Sync(ctx, co.SubscriptionId); !r.IsDone() {
		t.Fatalf("Sync = %v", r.Err())
	}
	if page := list(carol.token); len(page.Items) != 0 {
		t.Errorf("the list after bob's subscription ended: %+v", page)
	}
	e.want(e.do(http.MethodGet, "/preferences/supporter_listing", bob.token, nil, nil), http.StatusNotFound, "bob's listing after the end")
}
