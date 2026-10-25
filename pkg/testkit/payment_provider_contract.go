// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/supporters"
)

// PaymentScenario is the provider's side that PaymentProviderContract hands a factory: what the
// provider's account holds, which the PaymentProvider under test must report (ADR-0074 §1).
type PaymentScenario struct {
	Mode     supporters.Mode
	Location *time.Location
	// Now is the provider's clock.
	Now time.Time
	// Subscriptions are the account's subscriptions, in the order the provider lists them.
	Subscriptions []supporters.SubscriptionSnapshot
	// Checkouts are hosted checkouts, open, completed, or expired.
	Checkouts []supporters.CheckoutResult
	// Money are the account's money movements, oldest first.
	Money []PaymentScenarioMoney
	// Changes are the account's events that ListChanges returns, oldest first.
	Changes []supporters.Notice
	// Failure, when not nil, makes every request fail with it: ErrPaymentPermission,
	// ErrPaymentThrottled, or ErrPaymentUnavailable.
	Failure error
}

// PaymentScenarioMoney is a money movement of a PaymentScenario as the provider books it, in
// minor units signed as the provider reports them (negative for money out).
type PaymentScenarioMoney struct {
	ID string
	// Object is what a webhook of the movement names (Stripe: the charge or the refund); "" is ID.
	Object       string
	Kind         supporters.MoneyKind
	ParentID     string
	Subscription supporters.SubscriptionRef
	OccurredAt   time.Time
	Covers       supporters.DateRange
	GrossMinor   int64
	FeeMinor     int64
	Currency     string
	// Pending reports a movement whose amounts are not final yet: the port does not return it.
	Pending bool
}

// object returns what a webhook of the movement names.
func (m PaymentScenarioMoney) object() string { return cmp.Or(m.Object, m.ID) }

// paymentExponents are the minor units of the currencies of the scenarios.
var paymentExponents = map[string]int{"JPY": 0, "USD": 2, "EUR": 2}

// PaymentDecimal formats an amount in minor units of currency as a decimal in major units, the
// form of MoneyMovement: -1250 USD is "-12.50", 1000 JPY is "1000".
func PaymentDecimal(minor int64, currency string) string {
	exp, ok := paymentExponents[currency]
	if !ok {
		panic("testkit: no minor units of " + currency)
	}
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	s := strconv.FormatInt(minor, 10)
	if exp == 0 {
		return sign + s
	}
	for len(s) <= exp {
		s = "0" + s
	}
	return sign + s[:len(s)-exp] + "." + s[len(s)-exp:]
}

// PaymentMovements returns the movements of s as the port reports them, pending ones included
// with Final false: the decimals, Net = Gross - Fee, OccurredOn in s.Location, and the
// ParentGross of the parent.
func PaymentMovements(s PaymentScenario) []supporters.MoneyMovement {
	gross := map[string]string{}
	for _, m := range s.Money {
		gross[m.ID] = PaymentDecimal(m.GrossMinor, m.Currency)
	}
	out := make([]supporters.MoneyMovement, len(s.Money))
	for i, m := range s.Money {
		out[i] = supporters.MoneyMovement{
			ID: m.ID, Kind: m.Kind, ParentID: m.ParentID, Subscription: m.Subscription,
			OccurredAt: m.OccurredAt, OccurredOn: supporters.DateOf(m.OccurredAt.In(s.Location)), Covers: m.Covers,
			Gross: PaymentDecimal(m.GrossMinor, m.Currency), Fee: PaymentDecimal(m.FeeMinor, m.Currency),
			Net: PaymentDecimal(m.GrossMinor-m.FeeMinor, m.Currency), Currency: m.Currency, Final: !m.Pending,
		}
		if m.ParentID != "" {
			out[i].ParentGross = gross[m.ParentID]
		}
	}
	return out
}

// PaymentDeliver returns the header and body of a webhook delivery of n signed at at, with one
// signature per key: "" signs with the endpoint's secret, any other key with itself as the
// secret. live marks a live event.
type PaymentDeliver func(n supporters.Notice, live bool, at time.Time, keys ...string) (http.Header, []byte)

// PaymentUnderTest is what a PaymentProviderFactory returns: the provider on the scenario, its
// webhook signer, and the hosts its checkout and portal URLs may be on (ADR-0074 §7).
type PaymentUnderTest struct {
	Provider      supporters.PaymentProvider
	Deliver       PaymentDeliver
	CheckoutHosts []string
	PortalHosts   []string
	// TimedSignature reports that the provider signs each delivery's body and time, with several
	// signatures while its secret rotates (Stripe). A provider that authenticates deliveries by a
	// fixed token (PAY.JP, ADR-0099 §4) leaves it false: its Deliver ignores at, a key other than
	// "" sends a wrong token, and the contract skips the cases of a tampered body, the time, and
	// several signatures.
	TimedSignature bool
}

// PaymentProviderFactory returns a provider whose account holds s. The factory registers the
// cleanup of everything it creates with t.Cleanup.
type PaymentProviderFactory func(t *testing.T, s PaymentScenario) PaymentUnderTest

// PaymentProviderContract verifies a PaymentProvider adapter (ADR-0074 §1): its mode, time
// zone, and minimum amounts; EnsurePlan reusing its prices and archiving the ones it replaces;
// checkouts on the allowed hosts that report our ID, completion, and expiry, and portal URLs;
// commands that end the same when repeated; webhooks verified for their signature, time, and
// mode without I/O, with several signatures and unknown types (the signature only, for a
// provider without TimedSignature); the snapshots of every state;
// money movements with their signs, Net = Gross - Fee, parents, periods, and dates in the
// provider's time zone, final ones only; pages without gaps or repeats; the window of the
// changes; and the kinds of failure.
func PaymentProviderContract(t *testing.T, newProvider PaymentProviderFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, newProvider PaymentProviderFactory)
	}{
		{"ReportsItsAccount", paymentAccount},
		{"EnsuresPlans", paymentPlans},
		{"StartsCheckouts", paymentCheckouts},
		{"RepeatsCommands", paymentCommands},
		{"VerifiesWebhooks", paymentWebhooks},
		{"ReportsSnapshots", paymentSnapshots},
		{"ReportsMoney", paymentMoney},
		{"PagesWithoutGaps", paymentPages},
		{"ClassifiesFailures", paymentFailures},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newProvider) })
	}
}

var paymentNow = time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)

func paymentTokyo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func paymentScenario(t *testing.T) PaymentScenario {
	return PaymentScenario{Mode: supporters.ModeTest, Location: paymentTokyo(t), Now: paymentNow}
}

func paymentSub(ref, local string, status supporters.SubscriptionStatus) supporters.SubscriptionSnapshot {
	return supporters.SubscriptionSnapshot{
		Ref: supporters.SubscriptionRef(ref), Customer: "cus_" + supporters.CustomerRef(ref), LocalID: local, Price: "price_base",
		Status: status, PeriodStart: paymentNow.Add(-10 * 24 * time.Hour), PeriodEnd: paymentNow.Add(20 * 24 * time.Hour),
	}
}

func paymentAccount(t *testing.T, newProvider PaymentProviderFactory) {
	for _, mode := range []supporters.Mode{supporters.ModeTest, supporters.ModeLive} {
		s := paymentScenario(t)
		s.Mode = mode
		p := newProvider(t, s).Provider
		if p.Name() == "" || strings.ContainsAny(p.Name(), " /:") {
			t.Errorf("Name = %q, want an identifier", p.Name())
		}
		if p.Mode() != mode {
			t.Errorf("Mode = %q, want %q", p.Mode(), mode)
		}
		if got := p.Location(); got == nil || got.String() != s.Location.String() {
			t.Errorf("Location = %v, want %v", got, s.Location)
		}
	}
	p := newProvider(t, paymentScenario(t)).Provider
	for _, c := range []string{"JPY", "USD"} {
		if n, ok := p.MinimumAmount(c); !ok || n <= 0 {
			t.Errorf("MinimumAmount(%s) = %d, %v, want a positive minimum", c, n, ok)
		}
	}
	if _, ok := p.MinimumAmount("XXX"); ok {
		t.Error("MinimumAmount(XXX) reports a minimum of a currency that does not exist")
	}
}

func paymentPlanSpec(month int64) supporters.PlanSpec {
	return supporters.PlanSpec{
		PlanID: NamedID("plan-1"), Name: "Supporter", Description: "Thanks",
		Prices: []supporters.PriceSpec{
			{Interval: supporters.IntervalMonth, AmountMinor: month, Currency: "JPY"},
			{Interval: supporters.IntervalYear, AmountMinor: 5000, Currency: "JPY"},
		},
	}
}

func ensurePlan(t *testing.T, p supporters.PaymentProvider, spec supporters.PlanSpec) supporters.PlanRefs {
	t.Helper()
	refs, err := p.EnsurePlan(t.Context(), spec)
	if err != nil {
		t.Fatalf("EnsurePlan(%+v) = %v", spec, err)
	}
	if refs.Product == "" || len(refs.Prices) != len(spec.Prices) {
		t.Fatalf("EnsurePlan(%+v) = %+v, want a product and one price per spec", spec, refs)
	}
	for i, pr := range refs.Prices {
		if pr.Spec != spec.Prices[i] || pr.Ref == "" {
			t.Fatalf("EnsurePlan price %d = %+v, want a ref of %+v", i, pr, spec.Prices[i])
		}
	}
	return refs
}

func paymentPlans(t *testing.T, newProvider PaymentProviderFactory) {
	s := paymentScenario(t)
	s.Subscriptions = []supporters.SubscriptionSnapshot{paymentSub("sub_on_old", NamedID("local-1"), supporters.StatusActive)}
	p := newProvider(t, s).Provider
	first := ensurePlan(t, p, paymentPlanSpec(500))
	if len(first.Archived) != 0 {
		t.Errorf("first EnsurePlan archived %v", first.Archived)
	}
	again := ensurePlan(t, p, paymentPlanSpec(500))
	if again.Product != first.Product || !slices.Equal(again.Prices, first.Prices) || len(again.Archived) != 0 {
		t.Errorf("repeated EnsurePlan = %+v, want %+v unchanged", again, first)
	}
	raised := ensurePlan(t, p, paymentPlanSpec(600))
	if raised.Product != first.Product {
		t.Errorf("EnsurePlan with a new amount made product %s, want %s", raised.Product, first.Product)
	}
	if raised.Prices[0].Ref == first.Prices[0].Ref || raised.Prices[1].Ref != first.Prices[1].Ref {
		t.Errorf("EnsurePlan with a new monthly amount = %+v, want a new monthly price and the same yearly one", raised.Prices)
	}
	if !slices.Equal(raised.Archived, []supporters.PriceRef{first.Prices[0].Ref}) {
		t.Errorf("EnsurePlan with a new amount archived %v, want %v", raised.Archived, first.Prices[0].Ref)
	}
	if again := ensurePlan(t, p, paymentPlanSpec(600)); len(again.Archived) != 0 || !slices.Equal(again.Prices, raised.Prices) {
		t.Errorf("repeated EnsurePlan after the change = %+v, want %+v unchanged", again, raised)
	}
	if snap, err := p.GetSubscription(t.Context(), "sub_on_old"); err != nil || snap.Price != "price_base" {
		t.Errorf("subscription after the archival = %+v, %v, want it on its price", snap, err)
	}
	bad := paymentPlanSpec(500)
	bad.Prices[0].AmountMinor = 0
	if _, err := p.EnsurePlan(t.Context(), bad); supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
		t.Errorf("EnsurePlan of a price of 0 = %v, want invalid_request", err)
	}
}

// allowedURL reports whether u is https on one of hosts.
func allowedURL(u string, hosts []string) bool {
	parsed, err := url.Parse(u)
	return err == nil && parsed.Scheme == "https" && slices.Contains(hosts, parsed.Hostname())
}

func paymentCheckouts(t *testing.T, newProvider PaymentProviderFactory) {
	ctx := t.Context()
	s := paymentScenario(t)
	s.Checkouts = []supporters.CheckoutResult{
		{Ref: "cs_done", Status: supporters.CheckoutCompleted, LocalID: NamedID("local-done"), Customer: "cus_done", Subscription: "sub_done"},
		{Ref: "cs_gone", Status: supporters.CheckoutExpired, LocalID: NamedID("local-gone")},
	}
	u := newProvider(t, s)
	p := u.Provider
	refs := ensurePlan(t, p, paymentPlanSpec(500))
	req := supporters.CheckoutRequest{
		LocalID: NamedID("local-new"), Price: refs.Prices[0].Ref, Email: "alice@example.com", Locale: "ja",
		SuccessURL: "https://my.example/supporters/done", CancelURL: "https://my.example/supporters", Key: "checkout:local-new",
	}
	c, err := p.StartCheckout(ctx, req)
	if err != nil {
		t.Fatalf("StartCheckout = %v", err)
	}
	if c.Ref == "" || !allowedURL(c.RedirectURL, u.CheckoutHosts) || !c.ExpiresAt.After(s.Now) {
		t.Errorf("StartCheckout = %+v, want a checkout on %v that expires after %s", c, u.CheckoutHosts, s.Now)
	}
	if again, err := p.StartCheckout(ctx, req); err != nil || again.Ref != c.Ref {
		t.Errorf("StartCheckout with the same key = %+v, %v, want %s", again, err, c.Ref)
	}
	got, err := p.GetCheckout(ctx, c.Ref)
	if err != nil || got.Status != supporters.CheckoutOpen || got.LocalID != NamedID("local-new") {
		t.Errorf("GetCheckout(new) = %+v, %v, want open with our ID", got, err)
	}
	for _, want := range s.Checkouts {
		got, err := p.GetCheckout(ctx, want.Ref)
		if err != nil || got != want {
			t.Errorf("GetCheckout(%s) = %+v, %v, want %+v", want.Ref, got, err, want)
		}
	}
	if _, err := p.GetCheckout(ctx, "cs_unknown"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("GetCheckout(unknown) = %v, want not_found", err)
	}
	portal, err := p.PortalURL(ctx, "cus_done", "https://my.example/supporters")
	if err != nil || !allowedURL(portal, u.PortalHosts) {
		t.Errorf("PortalURL = %q, %v, want a URL on %v", portal, err, u.PortalHosts)
	}
}

func paymentCommands(t *testing.T, newProvider PaymentProviderFactory) {
	ctx := t.Context()
	s := paymentScenario(t)
	s.Subscriptions = []supporters.SubscriptionSnapshot{paymentSub("sub_cmd", NamedID("local-cmd"), supporters.StatusActive)}
	p := newProvider(t, s).Provider
	refs := ensurePlan(t, p, paymentPlanSpec(500))
	up, down := refs.Prices[1].Ref, refs.Prices[0].Ref
	twice := func(name string, f func(key string) error, check func(supporters.SubscriptionSnapshot) bool) {
		t.Helper()
		for i := range 2 {
			if err := f("job-1:" + name); err != nil {
				t.Fatalf("%s (call %d) = %v", name, i+1, err)
			}
			snap, err := p.GetSubscription(ctx, "sub_cmd")
			if err != nil {
				t.Fatalf("GetSubscription after %s = %v", name, err)
			}
			if !check(snap) {
				t.Errorf("after %s (call %d): %+v", name, i+1, snap)
			}
		}
	}
	twice("change_now", func(k string) error { return p.ChangePrice(ctx, "sub_cmd", up, supporters.ChangeNowProrated, k) },
		func(s supporters.SubscriptionSnapshot) bool { return s.Price == up && s.ScheduledPrice == "" })
	twice("change_at_end", func(k string) error { return p.ChangePrice(ctx, "sub_cmd", down, supporters.ChangeAtPeriodEnd, k) },
		func(s supporters.SubscriptionSnapshot) bool { return s.Price == up && s.ScheduledPrice == down })
	twice("cancel_change", func(k string) error { return p.CancelScheduledChange(ctx, "sub_cmd", k) },
		func(s supporters.SubscriptionSnapshot) bool { return s.Price == up && s.ScheduledPrice == "" })
	twice("cancel_at_end", func(k string) error { return p.CancelAtPeriodEnd(ctx, "sub_cmd", k) },
		func(s supporters.SubscriptionSnapshot) bool {
			return s.CancelAtPeriodEnd && s.Status == supporters.StatusActive
		})
	twice("resume", func(k string) error { return p.ResumeCancellation(ctx, "sub_cmd", k) },
		func(s supporters.SubscriptionSnapshot) bool {
			return !s.CancelAtPeriodEnd && s.Status == supporters.StatusActive
		})
	twice("cancel_now", func(k string) error { return p.CancelNow(ctx, "sub_cmd", k) },
		func(s supporters.SubscriptionSnapshot) bool { return s.Status == supporters.StatusCanceled })
	if err := p.CancelAtPeriodEnd(ctx, "sub_unknown", "job-1:unknown"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("CancelAtPeriodEnd(unknown) = %v, want not_found", err)
	}
}

func paymentWebhooks(t *testing.T, newProvider PaymentProviderFactory) {
	s := paymentScenario(t)
	u := newProvider(t, s)
	p := u.Provider
	notices := []supporters.Notice{
		{EventID: "evt_sub", Kind: supporters.NoticeSubscription, ObjectID: "sub_1"},
		{EventID: "evt_cs", Kind: supporters.NoticeCheckout, ObjectID: "cs_1"},
		{EventID: "evt_money", Kind: supporters.NoticeMoney, ObjectID: "ch_1"},
		{EventID: "evt_other", Kind: supporters.NoticeIgnored, ObjectID: "cus_1"},
	}
	for _, n := range notices {
		h, body := u.Deliver(n, false, s.Now)
		got, err := p.ParseWebhook(h, body, s.Now.Add(time.Minute))
		if err != nil || got.EventID != n.EventID || got.Kind != n.Kind || got.Type == "" {
			t.Errorf("ParseWebhook(%s) = %+v, %v, want its ID and kind", n.EventID, got, err)
			continue
		}
		if n.Kind != supporters.NoticeIgnored && got.ObjectID != n.ObjectID {
			t.Errorf("ParseWebhook(%s).ObjectID = %q, want %q", n.EventID, got.ObjectID, n.ObjectID)
		}
	}
	n := notices[0]
	rejects := []struct {
		name string
		at   time.Time
		live bool
		keys []string
		edit func(http.Header, []byte) (http.Header, []byte)
		want supporters.ErrorKind
	}{
		{name: "tampered body", at: s.Now, want: supporters.ErrorInvalidSignature, edit: func(h http.Header, b []byte) (http.Header, []byte) {
			return h, []byte(strings.Replace(string(b), "sub_1", "sub_2", 1))
		}},
		{name: "no signature", at: s.Now, want: supporters.ErrorInvalidSignature, edit: func(_ http.Header, b []byte) (http.Header, []byte) {
			return http.Header{}, b
		}},
		{name: "another secret", at: s.Now, keys: []string{"whsec_other"}, want: supporters.ErrorInvalidSignature},
		{name: "too old", at: s.Now.Add(-supporters.WebhookTolerance - time.Minute), want: supporters.ErrorStaleSignature},
		{name: "too new", at: s.Now.Add(supporters.WebhookTolerance + time.Minute), want: supporters.ErrorStaleSignature},
		{name: "live event on test credentials", at: s.Now, live: true, want: supporters.ErrorModeMismatch},
	}
	for _, r := range rejects {
		if !u.TimedSignature && (r.want == supporters.ErrorStaleSignature || r.name == "tampered body") {
			continue
		}
		h, body := u.Deliver(n, r.live, r.at, r.keys...)
		if r.edit != nil {
			h, body = r.edit(h, body)
		}
		if _, err := p.ParseWebhook(h, body, s.Now); supporters.ErrorKindOf(err) != r.want {
			t.Errorf("ParseWebhook(%s) = %v, want %s", r.name, err, r.want)
		}
	}
	if u.TimedSignature {
		h, body := u.Deliver(n, false, s.Now, "whsec_rotated", "")
		if got, err := p.ParseWebhook(h, body, s.Now); err != nil || got.EventID != n.EventID {
			t.Errorf("ParseWebhook(two signatures, one valid) = %+v, %v, want it accepted", got, err)
		}
	}
	// A provider that fails every request still verifies webhooks: ParseWebhook does no I/O.
	s.Failure = supporters.ErrPaymentUnavailable
	u = newProvider(t, s)
	h, body := u.Deliver(n, false, s.Now)
	if _, err := u.Provider.ParseWebhook(h, body, s.Now); err != nil {
		t.Errorf("ParseWebhook on an unavailable provider = %v, want no I/O", err)
	}
}

func paymentSnapshots(t *testing.T, newProvider PaymentProviderFactory) {
	s := paymentScenario(t)
	for i, st := range []supporters.SubscriptionStatus{
		supporters.StatusIncomplete, supporters.StatusIncompleteExpired, supporters.StatusActive, supporters.StatusPastDue,
		supporters.StatusUnpaid, supporters.StatusPaused, supporters.StatusCanceled,
	} {
		s.Subscriptions = append(s.Subscriptions, paymentSub(fmt.Sprintf("sub_%d", i), NamedID(fmt.Sprintf("local-%d", i)), st))
	}
	canceling := paymentSub("sub_canceling", NamedID("local-canceling"), supporters.StatusActive)
	canceling.CancelAtPeriodEnd, canceling.ScheduledPrice = true, "price_lower"
	failed := paymentSub("sub_failed", NamedID("local-failed"), supporters.StatusPastDue)
	failed.LatestPaymentFailed = paymentNow.Add(-2 * 24 * time.Hour)
	s.Subscriptions = append(s.Subscriptions, canceling, failed, paymentSub("sub_not_ours", "", supporters.StatusActive))
	p := newProvider(t, s).Provider
	for _, want := range s.Subscriptions {
		got, err := p.GetSubscription(t.Context(), want.Ref)
		if err != nil {
			t.Errorf("GetSubscription(%s) = %v", want.Ref, err)
			continue
		}
		if !sameSnapshot(got, want) {
			t.Errorf("GetSubscription(%s) = %+v, want %+v", want.Ref, got, want)
		}
	}
	if _, err := p.GetSubscription(t.Context(), "sub_unknown"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("GetSubscription(unknown) = %v, want not_found", err)
	}
}

func sameSnapshot(a, b supporters.SubscriptionSnapshot) bool {
	return a.Ref == b.Ref && a.Customer == b.Customer && a.LocalID == b.LocalID && a.Price == b.Price && a.Status == b.Status &&
		a.PeriodStart.Equal(b.PeriodStart) && a.PeriodEnd.Equal(b.PeriodEnd) && a.CancelAtPeriodEnd == b.CancelAtPeriodEnd &&
		a.ScheduledPrice == b.ScheduledPrice && a.LatestPaymentFailed.Equal(b.LatestPaymentFailed)
}

func sameMovement(a, b supporters.MoneyMovement) bool {
	a.OccurredAt, b.OccurredAt = a.OccurredAt.UTC(), b.OccurredAt.UTC()
	return a == b
}

// paymentMoneyScenario has a payment of each period, refunds partial and full, a chargeback with
// its reversal, a fee of its own, a payment in USD, and a pending payment, around midnight in
// Tokyo.
func paymentMoneyScenario(t *testing.T) PaymentScenario {
	s := paymentScenario(t)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 14, 30, 0, 0, time.UTC) }  // 23:30 in Tokyo
	late := func(d int) time.Time { return time.Date(2026, 10, d, 15, 30, 0, 0, time.UTC) } // 00:30 of the next day in Tokyo
	period := supporters.DateRange{Start: supporters.Date{Year: 2026, Month: 10, Day: 1}, End: supporters.Date{Year: 2026, Month: 10, Day: 31}}
	s.Money = []PaymentScenarioMoney{
		{ID: "txn_p1", Object: "ch_p1", Kind: supporters.MoneyPayment, Subscription: "sub_1", OccurredAt: day(1), Covers: period, GrossMinor: 1000, FeeMinor: 43, Currency: "JPY"},
		{ID: "txn_p2", Object: "ch_p2", Kind: supporters.MoneyPayment, Subscription: "sub_2", OccurredAt: late(1), Covers: period, GrossMinor: 500, FeeMinor: 22, Currency: "JPY"},
		{ID: "txn_p3", Object: "ch_p3", Kind: supporters.MoneyPayment, Subscription: "sub_3", OccurredAt: day(2), Covers: period, GrossMinor: 1000, FeeMinor: 43, Currency: "JPY"},
		{ID: "txn_usd", Object: "ch_usd", Kind: supporters.MoneyPayment, Subscription: "sub_4", OccurredAt: day(3), Covers: period, GrossMinor: 1250, FeeMinor: 66, Currency: "USD"},
		{ID: "txn_r1", Object: "re_r1", Kind: supporters.MoneyRefund, ParentID: "txn_p1", Subscription: "sub_1", OccurredAt: day(4), GrossMinor: -300, Currency: "JPY"},
		{ID: "txn_r2", Object: "re_r2", Kind: supporters.MoneyRefund, ParentID: "txn_p2", Subscription: "sub_2", OccurredAt: late(4), GrossMinor: -500, Currency: "JPY"},
		{ID: "txn_c1", Object: "dp_c1", Kind: supporters.MoneyChargeback, ParentID: "txn_p3", Subscription: "sub_3", OccurredAt: day(5), GrossMinor: -1000, FeeMinor: 1500, Currency: "JPY"},
		{ID: "txn_c1r", Object: "dp_c1r", Kind: supporters.MoneyChargebackReversal, ParentID: "txn_p3", Subscription: "sub_3", OccurredAt: day(6), GrossMinor: 1000, Currency: "JPY"},
		{ID: "txn_fee", Object: "fee_1", Kind: supporters.MoneyFee, OccurredAt: day(7), GrossMinor: -70, Currency: "JPY"},
		{ID: "txn_pending", Object: "ch_pending", Kind: supporters.MoneyPayment, Subscription: "sub_5", OccurredAt: day(8), Covers: period, GrossMinor: 1000, FeeMinor: 43, Currency: "JPY", Pending: true},
	}
	return s
}

func paymentMoney(t *testing.T, newProvider PaymentProviderFactory) {
	ctx := t.Context()
	s := paymentMoneyScenario(t)
	p := newProvider(t, s).Provider
	all := PaymentMovements(s)
	var want []supporters.MoneyMovement
	for _, m := range all {
		if m.Final {
			want = append(want, m)
		}
	}
	got := listMoney(t, p, time.Time{})
	if len(got) != len(want) {
		t.Fatalf("ListMoney = %d movements, want %d (the final ones)", len(got), len(want))
	}
	for i := range want {
		if !sameMovement(got[i], want[i]) {
			t.Errorf("ListMoney[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	signs := map[supporters.MoneyKind]int{
		supporters.MoneyPayment: 1, supporters.MoneyRefund: -1, supporters.MoneyChargeback: -1,
		supporters.MoneyChargebackReversal: 1, supporters.MoneyFee: -1,
	}
	for _, m := range got {
		if neg := strings.HasPrefix(m.Gross, "-"); neg != (signs[m.Kind] < 0) {
			t.Errorf("%s (%s): Gross %s has the wrong sign", m.ID, m.Kind, m.Gross)
		}
		if !m.Final {
			t.Errorf("%s: not final", m.ID)
		}
	}
	// Each movement maps to an income movement that the income records hold (ADR-0073 §1): Net =
	// Gross - Fee in the currency's minor unit, a period on the payments only, and the parent
	// on the refunds, chargebacks, and reversals.
	for _, m := range got {
		in := supporters.IncomeOf(p.Name(), m, "")
		rec, err := appfinance.ParseIncome(in)
		if err != nil {
			t.Errorf("ParseIncome(%s) = %v", m.ID, err)
			continue
		}
		if rec.OccurredOn.String() != m.OccurredOn.String() || (rec.Covers != nil) != (m.Kind == supporters.MoneyPayment) ||
			(in.ParentID != "") != m.Kind.HasParent() {
			t.Errorf("income of %s = %+v, want its date, the period of a payment, and the parent of a reversal", m.ID, rec)
		}
	}
	if late := got[1]; late.OccurredOn != (supporters.Date{Year: 2026, Month: 10, Day: 2}) {
		t.Errorf("OccurredOn of a movement at 00:30 in Tokyo = %s, want 2026-10-02", late.OccurredOn)
	}
	since := listMoney(t, p, s.Money[3].OccurredAt)
	if len(since) == 0 || since[0].ID != "txn_usd" {
		t.Errorf("ListMoney(since the USD payment) starts with %v, want txn_usd", since)
	}
	if m, err := p.GetMoney(ctx, "txn_p1"); err != nil || !sameMovement(m, want[0]) {
		t.Errorf("GetMoney(txn_p1) = %+v, %v, want %+v", m, err, want[0])
	}
	if _, err := p.GetMoney(ctx, "txn_pending"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("GetMoney(pending) = %v, want not_found", err)
	}
	for i, m := range s.Money {
		ch, err := p.Resolve(ctx, supporters.Notice{EventID: "evt_" + m.ID, Type: "money", Kind: supporters.NoticeMoney, ObjectID: m.object()})
		if err != nil {
			t.Errorf("Resolve(%s) = %v", m.ID, err)
			continue
		}
		if m.Pending {
			if len(ch.Money) != 0 {
				t.Errorf("Resolve(pending %s) = %+v, want no money", m.ID, ch.Money)
			}
			continue
		}
		if !slices.ContainsFunc(ch.Money, func(got supporters.MoneyMovement) bool { return sameMovement(got, all[i]) }) {
			t.Errorf("Resolve(%s).Money = %+v, want %+v", m.ID, ch.Money, all[i])
		}
	}
	ch, err := p.Resolve(ctx, supporters.Notice{EventID: "evt_s", Type: "subscription", Kind: supporters.NoticeSubscription, ObjectID: "sub_9"})
	if err != nil || !slices.Contains(ch.Subscriptions, "sub_9") {
		t.Errorf("Resolve(subscription) = %+v, %v, want sub_9", ch, err)
	}
	ch, err = p.Resolve(ctx, supporters.Notice{EventID: "evt_c", Type: "checkout", Kind: supporters.NoticeCheckout, ObjectID: "cs_9"})
	if err != nil || ch.Checkout != "cs_9" {
		t.Errorf("Resolve(checkout) = %+v, %v, want cs_9", ch, err)
	}
}

// listMoney reads every page of ListMoney since since, checks the cursor that ends it, and
// returns the movements.
func listMoney(t *testing.T, p supporters.PaymentProvider, since time.Time) []supporters.MoneyMovement {
	t.Helper()
	var out []supporters.MoneyMovement
	cursor := ""
	for range 1000 {
		page, err := p.ListMoney(t.Context(), since, cursor)
		if err != nil {
			t.Fatalf("ListMoney(%q) = %v", cursor, err)
		}
		if len(page.Movements) > supporters.PageSize {
			t.Fatalf("ListMoney page of %d movements, want at most %d", len(page.Movements), supporters.PageSize)
		}
		out = append(out, page.Movements...)
		if page.Next == "" && len(page.Movements) > 0 {
			t.Fatal("ListMoney returned movements without a cursor")
		}
		cursor = page.Next
		if !page.More {
			after, err := p.ListMoney(t.Context(), since, cursor)
			if err != nil || len(after.Movements) != 0 || after.More {
				t.Fatalf("ListMoney after the last page = %+v, %v, want nothing", after, err)
			}
			return out
		}
	}
	t.Fatal("ListMoney never ends")
	return nil
}

func paymentPages(t *testing.T, newProvider PaymentProviderFactory) {
	ctx := t.Context()
	s := paymentScenario(t)
	for i := range 250 {
		s.Subscriptions = append(s.Subscriptions, paymentSub(fmt.Sprintf("sub_%03d", i), NamedID(fmt.Sprintf("local-%03d", i)), supporters.StatusActive))
	}
	for i := range 230 {
		s.Money = append(s.Money, PaymentScenarioMoney{
			ID: fmt.Sprintf("txn_%03d", i), Kind: supporters.MoneyPayment, Subscription: s.Subscriptions[i].Ref,
			OccurredAt: s.Now.Add(time.Duration(i-300) * time.Minute), GrossMinor: 500, FeeMinor: 22, Currency: "JPY",
		})
	}
	for i := range 150 {
		s.Changes = append(s.Changes, supporters.Notice{
			EventID: fmt.Sprintf("evt_%03d", i), Type: "subscription.updated", Kind: supporters.NoticeSubscription,
			ObjectID: string(s.Subscriptions[i].Ref), OccurredAt: s.Now.Add(time.Duration(i-200) * time.Minute),
		})
	}
	p := newProvider(t, s).Provider
	var subs []supporters.SubscriptionRef
	cursor, pages := "", 0
	for {
		page, err := p.ListSubscriptions(ctx, cursor)
		if err != nil {
			t.Fatalf("ListSubscriptions(%q) = %v", cursor, err)
		}
		if len(page.Subscriptions) > supporters.PageSize {
			t.Fatalf("ListSubscriptions page of %d, want at most %d", len(page.Subscriptions), supporters.PageSize)
		}
		pages++
		for _, sub := range page.Subscriptions {
			subs = append(subs, sub.Ref)
		}
		if cursor = page.Next; cursor == "" || pages > 100 {
			break
		}
	}
	slices.Sort(subs)
	if len(subs) != 250 || len(slices.Compact(slices.Clone(subs))) != 250 || pages < 3 {
		t.Errorf("ListSubscriptions listed %d subscriptions (%d distinct) on %d pages, want 250 on at least 3", len(subs), len(slices.Compact(subs)), pages)
	}
	if got := listMoney(t, p, time.Time{}); len(got) != 230 {
		t.Errorf("ListMoney listed %d movements, want 230", len(got))
	}
	since := s.Now.Add(-100 * time.Minute)
	var changes []supporters.Notice
	cursor = ""
	var page supporters.ChangePage
	for range 100 {
		var err error
		page, err = p.ListChanges(ctx, since, cursor)
		if err != nil {
			t.Fatalf("ListChanges(%q) = %v", cursor, err)
		}
		changes = append(changes, page.Notices...)
		if cursor = page.Next; cursor == "" {
			break
		}
	}
	if len(changes) != 50 || changes[0].EventID != "evt_100" || changes[49].EventID != "evt_149" {
		t.Errorf("ListChanges(since 100 minutes ago) = %d notices, want evt_100 through evt_149 in order", len(changes))
	}
	if page.Until.Before(changes[len(changes)-1].OccurredAt) {
		t.Errorf("ListChanges Until = %s, before its last notice", page.Until)
	}
	if _, err := p.ListChanges(ctx, s.Now.Add(-40*24*time.Hour), ""); !errors.Is(err, supporters.ErrChangesWindow) {
		t.Errorf("ListChanges(40 days ago) = %v, want ErrChangesWindow", err)
	}
}

func paymentFailures(t *testing.T, newProvider PaymentProviderFactory) {
	for _, tc := range []struct {
		err  error
		want supporters.ErrorKind
	}{
		{supporters.ErrPaymentPermission, supporters.ErrorPermission},
		{supporters.ErrPaymentThrottled, supporters.ErrorThrottled},
		{supporters.ErrPaymentUnavailable, supporters.ErrorUnavailable},
	} {
		s := paymentScenario(t)
		s.Subscriptions = []supporters.SubscriptionSnapshot{paymentSub("sub_1", NamedID("local-1"), supporters.StatusActive)}
		s.Failure = tc.err
		p := newProvider(t, s).Provider
		calls := map[string]error{}
		_, calls["GetSubscription"] = p.GetSubscription(t.Context(), "sub_1")
		_, calls["ListSubscriptions"] = p.ListSubscriptions(t.Context(), "")
		_, calls["ListMoney"] = p.ListMoney(t.Context(), time.Time{}, "")
		calls["CancelAtPeriodEnd"] = p.CancelAtPeriodEnd(t.Context(), "sub_1", "job-1:cancel")
		for name, err := range calls {
			if got := supporters.ErrorKindOf(err); got != tc.want {
				t.Errorf("%s with the provider failing with %v = %v (%s), want %s", name, tc.err, err, got, tc.want)
			}
		}
	}
	s := paymentScenario(t)
	s.Subscriptions = []supporters.SubscriptionSnapshot{paymentSub("sub_1", NamedID("local-1"), supporters.StatusActive)}
	p := newProvider(t, s).Provider
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.GetSubscription(ctx, "sub_1"); err == nil {
		t.Error("GetSubscription with a canceled context succeeded")
	}
}
