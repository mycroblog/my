// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/pkg/testkit"
)

const testSigningKey = "whsec_test_secret"

// sleeps records the waits of a Provider without waiting.
type sleeps struct {
	mu sync.Mutex
	d  []time.Duration
}

func (s *sleeps) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.d = append(s.d, d)
	s.mu.Unlock()
	return ctx.Err()
}

// newOnFake returns a Provider on a fake API whose account holds s.
func newOnFake(t *testing.T, s testkit.PaymentScenario) (*Provider, *fakeAPI, *sleeps) {
	t.Helper()
	now := func() time.Time { return s.Now }
	f := newFakeAPI(t, now)
	f.load(s)
	key := "sk_test_fake"
	if s.Mode == supporters.ModeLive {
		key = "rk_live_fake"
	}
	sl := &sleeps{}
	p, err := New(t.Context(), Options{
		SecretKey: key, WebhookSecret: testSigningKey, URL: f.srv.URL, HTTPClient: f.srv.Client(),
		Now: now, Sleep: sl.sleep, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, f, sl
}

func TestPaymentProviderContract(t *testing.T) {
	testkit.PaymentProviderContract(t, func(t *testing.T, s testkit.PaymentScenario) testkit.PaymentUnderTest {
		p, f, _ := newOnFake(t, s)
		return testkit.PaymentUnderTest{
			Provider: p, Deliver: f.deliver(testSigningKey), CheckoutHosts: []string{CheckoutHost}, PortalHosts: []string{PortalHost},
			TimedSignature: true,
		}
	})
}

func scenario() testkit.PaymentScenario {
	return testkit.PaymentScenario{Mode: supporters.ModeTest, Location: time.UTC, Now: time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)}
}

func TestNewRefusesKeys(t *testing.T) {
	for _, key := range []string{"", "pk_test_x", "sk_x", "whsec_x"} {
		if _, err := New(t.Context(), Options{SecretKey: key, WebhookSecret: "whsec_x"}); err == nil {
			t.Errorf("New(%q) accepted the key", key)
		}
	}
	if _, err := New(t.Context(), Options{SecretKey: "sk_test_x"}); err == nil {
		t.Error("New without a webhook secret succeeded")
	}
}

// The account's time zone falls back to UTC when Stripe does not answer, and is read again an
// hour later.
func TestLocationFallsBackToUTC(t *testing.T) {
	s := scenario()
	s.Location = time.FixedZone("x", 0)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	now := s.Now
	f := newFakeAPI(t, func() time.Time { return now })
	f.failure = 500
	p, err := New(t.Context(), Options{
		SecretKey: "sk_test_x", WebhookSecret: "whsec_x", URL: f.srv.URL, HTTPClient: f.srv.Client(),
		Now: func() time.Time { return now }, Sleep: (&sleeps{}).sleep, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Location() != time.UTC {
		t.Fatalf("Location = %v, want UTC", p.Location())
	}
	f.mu.Lock()
	f.failure, f.timezone = 0, "Asia/Tokyo"
	f.mu.Unlock()
	if _, err := p.ListMoney(t.Context(), time.Time{}, ""); err != nil {
		t.Fatal(err)
	}
	if p.Location() != time.UTC {
		t.Error("the time zone was read again within the hour")
	}
	now = now.Add(locationTTL)
	if _, err := p.ListMoney(t.Context(), time.Time{}, ""); err != nil {
		t.Fatal(err)
	}
	if p.Location().String() != tokyo.String() {
		t.Errorf("Location after an hour = %v, want Asia/Tokyo", p.Location())
	}
}

// A 429 waits as long as Retry-After asks and holds the requests, then succeeds; the requests are
// spaced by the mode's rate.
func TestRateLimit(t *testing.T) {
	s := scenario()
	s.Subscriptions = []supporters.SubscriptionSnapshot{{
		Ref: "sub_1", Customer: "cus_1", Price: "price_1", Status: supporters.StatusActive,
		PeriodStart: s.Now, PeriodEnd: s.Now.AddDate(0, 1, 0),
	}}
	p, f, sl := newOnFake(t, s)
	f.mu.Lock()
	f.throttle, f.retryAfter = 1, "3"
	f.mu.Unlock()
	if _, err := p.GetSubscription(t.Context(), "sub_1"); err != nil {
		t.Fatalf("GetSubscription after a 429 = %v", err)
	}
	if !slices.ContainsFunc(sl.d, func(d time.Duration) bool { return d >= 3*time.Second }) {
		t.Errorf("waits %v, want one of Retry-After's 3s", sl.d)
	}
	// Retry-After beyond maxRetryAfter is not waited for.
	f.mu.Lock()
	f.throttle, f.retryAfter = 1, "120"
	f.mu.Unlock()
	if _, err := p.GetSubscription(t.Context(), "sub_1"); supporters.ErrorKindOf(err) != supporters.ErrorThrottled {
		t.Errorf("GetSubscription after a long Retry-After = %v, want throttled", err)
	}
	sl.d = nil
	for range 3 {
		if _, err := p.GetSubscription(t.Context(), "sub_1"); err != nil {
			t.Fatal(err)
		}
	}
	// The clock stands still: each request waits one interval more than the one before.
	interval := time.Second / TestRequestsPerSecond
	if len(sl.d) != 3 || sl.d[1]-sl.d[0] != interval || sl.d[2]-sl.d[1] != interval {
		t.Errorf("waits %v, want them spaced by %s", sl.d, interval)
	}
}

func TestLiveRate(t *testing.T) {
	s := scenario()
	s.Mode = supporters.ModeLive
	p, _, _ := newOnFake(t, s)
	if p.limiter.interval != time.Second/LiveRequestsPerSecond {
		t.Errorf("live interval = %s", p.limiter.interval)
	}
}

// A checkout URL off Stripe's host is refused.
func TestCheckoutURLHost(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	refs, err := p.EnsurePlan(t.Context(), supporters.PlanSpec{PlanID: "01890a5d-ac96-7000-8000-000000000001", Name: "P", Prices: []supporters.PriceSpec{
		{Interval: supporters.IntervalMonth, AmountMinor: 500, Currency: "JPY"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	f.checkoutURL = "https://checkout.stripe.com.evil.example/c/pay/x"
	_, err = p.StartCheckout(t.Context(), supporters.CheckoutRequest{
		LocalID: "01890a5d-ac96-7000-8000-000000000002", Price: refs.Prices[0].Ref, SuccessURL: "https://my.example/ok?x=1", CancelURL: "https://my.example/", Key: "k1",
	})
	if !errors.Is(err, supporters.ErrInvalidPaymentResponse) {
		t.Errorf("StartCheckout with a URL elsewhere = %v, want ErrInvalidPaymentResponse", err)
	}
	f.mu.Lock()
	var cs obj
	for _, c := range f.checkouts {
		cs = c
	}
	f.mu.Unlock()
	if cs["success_url"] != "https://my.example/ok?x=1&checkout_id={CHECKOUT_SESSION_ID}" || cs["locale"] != "auto" {
		t.Errorf("checkout success_url %v, locale %v", cs["success_url"], cs["locale"])
	}
}

// The portal's configuration is created once and reused.
func TestPortalConfiguration(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	for range 2 {
		if _, err := p.PortalURL(t.Context(), "cus_1", "https://my.example/"); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.countRequests("POST /v1/billing_portal/configurations"); n != 1 {
		t.Errorf("created %d configurations, want 1", n)
	}
	// Another process finds it.
	other, err := New(t.Context(), p.o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.PortalURL(t.Context(), "cus_1", "https://my.example/"); err != nil {
		t.Fatal(err)
	}
	if n := f.countRequests("POST /v1/billing_portal/configurations"); n != 1 {
		t.Errorf("created %d configurations, want 1", n)
	}
}

// A cancellation at the period's end drops the change that waited for it: Stripe refuses to
// change the cancellation of a subscription on a schedule.
func TestCancelAtPeriodEndReleasesSchedule(t *testing.T) {
	s := scenario()
	s.Subscriptions = []supporters.SubscriptionSnapshot{{
		Ref: "sub_1", Customer: "cus_1", LocalID: "01890a5d-ac96-7000-8000-000000000002", Price: "price_1", Status: supporters.StatusActive,
		PeriodStart: s.Now, PeriodEnd: s.Now.AddDate(0, 1, 0), ScheduledPrice: "price_2",
	}}
	p, _, _ := newOnFake(t, s)
	if err := p.CancelAtPeriodEnd(t.Context(), "sub_1", "job:cancel"); err != nil {
		t.Fatal(err)
	}
	snap, err := p.GetSubscription(t.Context(), "sub_1")
	if err != nil || !snap.CancelAtPeriodEnd || snap.ScheduledPrice != "" {
		t.Errorf("after CancelAtPeriodEnd: %+v, %v", snap, err)
	}
}

// A notice of an invoice resolves to its subscription and the payment of its charge; the events
// of invoices name their subscriptions.
func TestResolveInvoice(t *testing.T) {
	s := scenario()
	s.Money = []testkit.PaymentScenarioMoney{{
		ID: "txn_1", Object: "ch_1", Kind: supporters.MoneyPayment, Subscription: "sub_1", OccurredAt: s.Now.Add(-time.Hour),
		Covers:     supporters.DateRange{Start: supporters.Date{Year: 2026, Month: 10, Day: 22}, End: supporters.Date{Year: 2026, Month: 11, Day: 21}},
		GrossMinor: 1250, FeeMinor: 66, Currency: "USD",
	}}
	p, f, _ := newOnFake(t, s)
	ch, err := p.Resolve(t.Context(), supporters.Notice{EventID: "evt_1", Type: "invoice.paid", Kind: supporters.NoticeMoney, ObjectID: "in_ch_1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ch.Subscriptions, []supporters.SubscriptionRef{"sub_1"}) || len(ch.Money) != 1 {
		t.Fatalf("Resolve(invoice) = %+v", ch)
	}
	m := ch.Money[0]
	if m.Gross != "12.50" || m.Fee != "0.66" || m.Net != "11.84" || m.Currency != "USD" || m.Covers != s.Money[0].Covers {
		t.Errorf("payment of the invoice = %+v", m)
	}
	f.mu.Lock()
	f.events = append(f.events, obj{
		"id": "evt_inv", "object": "event", "type": "invoice.paid", "livemode": false, "created": s.Now.Add(-time.Minute).Unix(),
		"data": obj{"object": obj{"id": "in_ch_1", "object": "invoice",
			"parent": obj{"subscription_details": obj{"subscription": "sub_1"}}}},
	}, obj{
		"id": "evt_ignored", "object": "event", "type": "customer.updated", "livemode": false, "created": s.Now.Add(-time.Minute).Unix(),
		"data": obj{"object": obj{"id": "cus_1", "object": "customer"}},
	})
	f.mu.Unlock()
	page, err := p.ListChanges(t.Context(), s.Now.Add(-time.Hour), "")
	if err != nil || len(page.Notices) != 1 || page.Notices[0].Subscription != "sub_1" || page.Notices[0].Kind != supporters.NoticeMoney {
		t.Errorf("ListChanges = %+v, %v, want the invoice's event with its subscription", page, err)
	}
}

func TestDecimal(t *testing.T) {
	for _, tc := range []struct {
		minor int64
		cur   string
		want  string
	}{
		{1000, "JPY", "1000"}, {-1250, "USD", "-12.50"}, {5, "usd", "0.05"}, {-5, "EUR", "-0.05"}, {1234, "KWD", "1.234"},
		{0, "USD", "0.00"}, {-9223372036854775808, "JPY", "-9223372036854775808"},
	} {
		if got := decimal(tc.minor, tc.cur); got != tc.want {
			t.Errorf("decimal(%d, %s) = %s, want %s", tc.minor, tc.cur, got, tc.want)
		}
	}
}

func TestCheckoutLocale(t *testing.T) {
	for tag, want := range map[string]string{
		"ja": "ja", "ja-JP": "ja", "en-GB": "en-GB", "en-us": "en", "pt-br": "pt-BR", "zh-Hant-TW": "zh-TW", "": "auto", "xx": "auto",
	} {
		if got := checkoutLocale(tag); got != want {
			t.Errorf("checkoutLocale(%q) = %q, want %q", tag, got, want)
		}
	}
}

// The verifier accepts an event of any API version and ignores the fields it does not read.
func TestParseWebhookOfOtherAPIVersion(t *testing.T) {
	p, _, _ := newOnFake(t, scenario())
	body := []byte(`{"id":"evt_1","object":"event","api_version":"2020-08-27","type":"invoice.payment_failed","livemode":false,` +
		`"created":1792630800,"data":{"object":{"id":"in_1","object":"invoice","subscription":"sub_1","lines":{"data":[]}}}}`)
	now := time.Unix(1792630800, 0)
	n, err := p.ParseWebhook(signedHeader(body, now, testSigningKey), body, now)
	if err != nil || n.Kind != supporters.NoticeMoney || n.ObjectID != "in_1" || n.Type != "invoice.payment_failed" || !n.OccurredAt.Equal(now) {
		t.Errorf("ParseWebhook = %+v, %v", n, err)
	}
	if _, err := p.ParseWebhook(signedHeader([]byte("not json"), now, testSigningKey), []byte("not json"), now); err == nil {
		t.Error("ParseWebhook accepted a body that is not an event")
	}
}

func TestEventTypes(t *testing.T) {
	if got := len(EventTypes()); got != 16 {
		t.Errorf("EventTypes = %d types, want the 16 of ADR-0074 §3", got)
	}
}
