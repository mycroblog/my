// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit/supporterstest"
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
func newOnFake(t *testing.T, s supporterstest.PaymentScenario) (*Provider, *fakeAPI, *sleeps) {
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
	supporterstest.PaymentProviderContract(t, func(t *testing.T, s supporterstest.PaymentScenario) supporterstest.PaymentUnderTest {
		p, f, _ := newOnFake(t, s)
		return supporterstest.PaymentUnderTest{
			Provider: p, Deliver: f.deliver(testSigningKey), CheckoutHosts: []string{CheckoutHost}, PortalHosts: []string{PortalHost},
			TimedSignature: true,
			DeclineChanges: func(sub supporters.SubscriptionRef, declined bool) {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.sub(string(sub)).payment = ""
				if declined {
					f.sub(string(sub)).payment = fakeCardDeclines
				}
			},
			ChangeCharges: func() int {
				f.mu.Lock()
				defer f.mu.Unlock()
				return f.changeCharges
			},
		}
	})
}

func scenario() supporterstest.PaymentScenario {
	return supporterstest.PaymentScenario{Mode: supporters.ModeTest, Location: time.UTC, Now: time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)}
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

// The values of a real subscription canceled in Stripe's customer portal: cancel_at at the
// period's end, cancel_at_period_end false, canceled_at the time of the cancellation, and active.
const (
	portalCancelAt   = int64(1825302070)
	portalCanceledAt = int64(1793766445)
)

// portalCanceled returns a Provider whose sub_1 has cancel_at at offset from its period's end.
func portalCanceled(t *testing.T, offset time.Duration) (*Provider, *fakeAPI) {
	t.Helper()
	p, f, _, _ := onPlan(t, "")
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sub("sub_1")
	f.item(s)["current_period_end"] = portalCancelAt
	s.o["cancel_at"] = portalCancelAt + int64(offset/time.Second)
	s.o["cancel_at_period_end"] = false
	s.o["canceled_at"] = portalCanceledAt
	return p, f
}

func TestPortalCancellationShows(t *testing.T) {
	end := time.Unix(portalCancelAt, 0).UTC()
	for name, tc := range map[string]struct {
		offset     time.Duration
		canceling  bool
		periodEnds time.Time
	}{
		"at the period's end": {0, true, end},
		// It ends sooner than the period, and the snapshot says when.
		"before the period's end": {-24 * time.Hour, true, end.Add(-24 * time.Hour)},
		// It renews first: not a cancellation yet.
		"after the period's end": {24 * time.Hour, false, end},
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := portalCanceled(t, tc.offset)
			snap, err := p.GetSubscription(t.Context(), "sub_1")
			if err != nil {
				t.Fatal(err)
			}
			if snap.CancelAtPeriodEnd != tc.canceling || !snap.PeriodEnd.Equal(tc.periodEnds) || snap.Status != supporters.StatusActive {
				t.Errorf("snapshot = %+v; want canceling %v, period end %s", snap, tc.canceling, tc.periodEnds)
			}
		})
	}
}

// A portal's cancellation is undone by clearing cancel_at, which cancel_at_period_end does not
// hold, and a second call changes nothing.
func TestResumePortalCancellation(t *testing.T) {
	p, f := portalCanceled(t, 0)
	before := f.countRequests("POST /v1/subscriptions/sub_1")
	for i := range 2 {
		if err := p.ResumeCancellation(t.Context(), "sub_1", fmt.Sprintf("resume:%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := p.GetSubscription(t.Context(), "sub_1")
	if err != nil || snap.CancelAtPeriodEnd {
		t.Errorf("after the resumption: %+v, %v", snap, err)
	}
	if n := f.countRequests("POST /v1/subscriptions/sub_1") - before; n != 1 {
		t.Errorf("%d updates, want 1", n)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if at := f.sub("sub_1").o["cancel_at"]; at != nil {
		t.Errorf("cancel_at = %v, want it cleared", at)
	}
}

// Our own cancellation, undone: cancel_at_period_end is cleared first, and then cancel_at whenever
// it stays.
func TestResumeOwnCancellationClearsBothFields(t *testing.T) {
	p, f, _, _ := onPlan(t, "")
	if err := p.CancelAtPeriodEnd(t.Context(), "sub_1", "cancel:1"); err != nil {
		t.Fatal(err)
	}
	if err := p.ResumeCancellation(t.Context(), "sub_1", "resume:1"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if o := f.sub("sub_1").o; o["cancel_at_period_end"] != false || o["cancel_at"] != nil {
		t.Errorf("cancel_at_period_end = %v, cancel_at = %v", o["cancel_at_period_end"], o["cancel_at"])
	}
}

// A cancel_at by the period's end is a cancellation already, and one after it moves to the
// period's end.
func TestCancelWithCancelAtSet(t *testing.T) {
	p, f := portalCanceled(t, 0)
	before := f.countRequests("POST /v1/subscriptions/sub_1")
	if err := p.CancelAtPeriodEnd(t.Context(), "sub_1", "cancel:1"); err != nil {
		t.Fatal(err)
	}
	if n := f.countRequests("POST /v1/subscriptions/sub_1") - before; n != 0 {
		t.Errorf("%d updates of a canceled subscription", n)
	}
	p, _ = portalCanceled(t, 24*time.Hour)
	if err := p.CancelAtPeriodEnd(t.Context(), "sub_1", "cancel:2"); err != nil {
		t.Fatal(err)
	}
	snap, err := p.GetSubscription(t.Context(), "sub_1")
	if err != nil || !snap.CancelAtPeriodEnd || !snap.PeriodEnd.Equal(time.Unix(portalCancelAt, 0)) {
		t.Errorf("after the cancellation: %+v, %v", snap, err)
	}
}

// A plan does not change while the portal's cancellation waits, in either timing.
func TestChangePriceRefusesACanceling(t *testing.T) {
	p, f, lower, higher := onPlan(t, "")
	f.mu.Lock()
	f.sub("sub_1").o["cancel_at"] = f.item(f.sub("sub_1"))["current_period_end"]
	f.mu.Unlock()
	before := f.countRequests("POST /v1/subscriptions/sub_1")
	for to, when := range map[supporters.PriceRef]supporters.ChangeTiming{higher: supporters.ChangeNowProrated, lower: supporters.ChangeAtPeriodEnd} {
		if err := p.ChangePrice(t.Context(), "sub_1", to, when, "change:"+string(when)); !errors.Is(err, supporters.ErrPaymentCanceling) {
			t.Errorf("ChangePrice(%s) = %v, want ErrPaymentCanceling", when, err)
		}
	}
	if n := f.countRequests("POST /v1/subscriptions/sub_1") - before; n != 0 || len(f.schedules) != 0 {
		t.Errorf("%d updates and %d schedules", n, len(f.schedules))
	}
}

// onPlan returns a Provider whose sub_1 pays price_1, scheduled to change to scheduled unless it
// is empty, with a lower and a higher price to change to.
func onPlan(t *testing.T, scheduled supporters.PriceRef) (p *Provider, f *fakeAPI, lower, higher supporters.PriceRef) {
	t.Helper()
	s := scenario()
	s.Subscriptions = []supporters.SubscriptionSnapshot{{
		Ref: "sub_1", Customer: "cus_1", LocalID: "01890a5d-ac96-7000-8000-000000000002", Price: "price_1", Status: supporters.StatusActive,
		PeriodStart: s.Now, PeriodEnd: s.Now.AddDate(0, 1, 0), ScheduledPrice: scheduled,
	}}
	p, f, _ = newOnFake(t, s)
	refs, err := p.EnsurePlan(t.Context(), supporters.PlanSpec{
		PlanID: "01890a5d-ac96-7000-8000-000000000001", Name: "Supporter", Prices: []supporters.PriceSpec{
			{Interval: supporters.IntervalMonth, AmountMinor: 500, Currency: "JPY"},
			{Interval: supporters.IntervalYear, AmountMinor: 5000, Currency: "JPY"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, f, refs.Prices[0].Ref, refs.Prices[1].Ref
}

// wantPrices fails unless sub_1 pays price and waits to change to scheduled.
func wantPrices(t *testing.T, p *Provider, price, scheduled supporters.PriceRef) {
	t.Helper()
	snap, err := p.GetSubscription(t.Context(), "sub_1")
	if err != nil || snap.Price != price || snap.ScheduledPrice != scheduled {
		t.Errorf("sub_1 = %+v, %v; want price %s, scheduled %q", snap, err, price, scheduled)
	}
}

// failOnce makes the next request of a method and path fail with status.
func failOnce(f *fakeAPI, method, path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.before = func(r *http.Request) int {
		if r.Method != method || r.URL.Path != path {
			return 0
		}
		f.before = nil
		return status
	}
}

// A change at the period's end can be withdrawn and made again, with a new schedule.
func TestChangeAtPeriodEndAgainAfterWithdrawal(t *testing.T) {
	p, f, lower, _ := onPlan(t, "")
	ctx := t.Context()
	if err := p.ChangePrice(ctx, "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:1"); err != nil {
		t.Fatal(err)
	}
	wantPrices(t, p, "price_1", lower)
	if err := p.CancelScheduledChange(ctx, "sub_1", "unschedule:1"); err != nil {
		t.Fatal(err)
	}
	wantPrices(t, p, "price_1", "")
	if err := p.ChangePrice(ctx, "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:2"); err != nil {
		t.Fatal(err)
	}
	wantPrices(t, p, "price_1", lower)
	if len(f.schedules) != 2 {
		t.Errorf("%d schedules, want 2", len(f.schedules))
	}
}

// A retry with a new key goes on from the schedule that the failed attempt left, which has no
// change yet, and makes no second one.
func TestChangeAtPeriodEndRetryReusesTheSchedule(t *testing.T) {
	p, f, lower, _ := onPlan(t, "")
	ctx := t.Context()
	f.mu.Lock()
	f.before = func(r *http.Request) int {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/subscription_schedules/") {
			f.before = nil
			return http.StatusInternalServerError
		}
		return 0
	}
	f.mu.Unlock()
	if err := p.ChangePrice(ctx, "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:1"); err == nil {
		t.Fatal("ChangePrice with the phases refused succeeded")
	}
	wantPrices(t, p, "price_1", "")
	if err := p.ChangePrice(ctx, "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:2"); err != nil {
		t.Fatal(err)
	}
	wantPrices(t, p, "price_1", lower)
	if len(f.schedules) != 1 {
		t.Errorf("%d schedules, want 1", len(f.schedules))
	}
}

// Two submissions at once read no schedule, and Stripe refuses the second to create one: it goes
// on with the schedule of the first.
func TestChangeAtPeriodEndDoubleSubmit(t *testing.T) {
	p, f, lower, _ := onPlan(t, "")
	f.mu.Lock()
	f.before = func(r *http.Request) int {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/subscription_schedules" {
			f.before = nil
			f.attachSchedule(f.sub("sub_1"))
		}
		return 0
	}
	f.mu.Unlock()
	if err := p.ChangePrice(t.Context(), "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:2"); err != nil {
		t.Fatal(err)
	}
	wantPrices(t, p, "price_1", lower)
	if len(f.schedules) != 1 {
		t.Errorf("%d schedules, want 1", len(f.schedules))
	}
}

// Any other refusal of a new schedule stays the failure of the change.
func TestChangeAtPeriodEndRefusedSchedule(t *testing.T) {
	p, f, lower, _ := onPlan(t, "")
	failOnce(f, http.MethodPost, "/v1/subscription_schedules", http.StatusBadRequest)
	err := p.ChangePrice(t.Context(), "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:1")
	if !errors.Is(err, supporters.ErrPaymentInvalidRequest) {
		t.Errorf("ChangePrice = %v, want an invalid request", err)
	}
}

// A change at once keeps its key through its retries, and its retry can meet a schedule that was
// made after the first attempt released another: releasing it is another request, which Stripe
// would refuse under the first release's key.
func TestChangeNowRetryReleasesASecondSchedule(t *testing.T) {
	p, f, lower, higher := onPlan(t, "price_2")
	ctx := t.Context()
	failOnce(f, http.MethodPost, "/v1/subscriptions/sub_1", http.StatusInternalServerError)
	if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); err == nil {
		t.Fatal("ChangePrice with the update refused succeeded")
	}
	wantPrices(t, p, "price_1", "")
	if err := p.ChangePrice(ctx, "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:later"); err != nil {
		t.Fatal(err)
	}
	wantPrices(t, p, "price_1", lower)
	if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); err != nil {
		t.Fatal(err)
	}
	wantPrices(t, p, higher, "")
}

// setCard sets what sub_1's card does with an invoice.
func setCard(f *fakeAPI, payment string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub("sub_1").payment = payment
}

// A card that fails in any way leaves the plan and a change waiting for the period's end as they
// were and nothing collectible, and a good card then pays once, under the same key.
func TestChangeNowFailedPaymentThenGoodCard(t *testing.T) {
	for name, card := range map[string]string{
		"declined": fakeCardDeclines, "needs authentication": fakeCardAuthenticate, "refused with a 402": fakeCardRefuses,
	} {
		t.Run(name, func(t *testing.T) {
			p, f, lower, higher := onPlan(t, "")
			ctx := t.Context()
			if err := p.ChangePrice(ctx, "sub_1", lower, supporters.ChangeAtPeriodEnd, "change:later"); err != nil {
				t.Fatal(err)
			}
			setCard(f, card)
			if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); !errors.Is(err, supporters.ErrPaymentDeclined) {
				t.Fatalf("ChangePrice = %v, want ErrPaymentDeclined", err)
			}
			wantPrices(t, p, "price_1", lower)
			snap, err := p.GetSubscription(ctx, "sub_1")
			if err != nil || snap.Status != supporters.StatusActive || !snap.LatestPaymentFailed.IsZero() || snap.UnpaidChange != "" {
				t.Errorf("after the failure: %+v, %v", snap, err)
			}
			f.mu.Lock()
			for id, inv := range f.invoices {
				if inv["status"] == "open" {
					t.Errorf("invoice %s stays collectible", id)
				}
			}
			f.mu.Unlock()
			// A second failure makes a new attempt, not a replay.
			before := f.countRequests("POST /v1/invoices/")
			if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); !errors.Is(err, supporters.ErrPaymentDeclined) {
				t.Fatalf("ChangePrice again = %v, want ErrPaymentDeclined", err)
			}
			if card != fakeCardRefuses && f.countRequests("POST /v1/invoices/") != before+1 {
				t.Error("the second attempt replayed the first")
			}
			setCard(f, "")
			for range 2 {
				if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); err != nil && card != fakeCardRefuses {
					t.Fatalf("ChangePrice with a good card = %v", err)
				}
			}
			if card == fakeCardRefuses {
				return // the same minute replays the refusal; TestChangeNowStoredFailure has the next one.
			}
			wantPrices(t, p, higher, "")
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.changeCharges != 1 {
				t.Errorf("%d charges, want 1", f.changeCharges)
			}
		})
	}
}

// A payment that has not settled stays: no void and no second update, and the plan moves if it
// succeeds.
func TestChangeNowProcessingPayment(t *testing.T) {
	p, f, _, higher := onPlan(t, "")
	ctx := t.Context()
	setCard(f, fakeCardProcesses)
	for range 2 {
		if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); !errors.Is(err, supporters.ErrPaymentProcessing) {
			t.Fatalf("ChangePrice = %v, want ErrPaymentProcessing", err)
		}
	}
	if n := f.countRequests("POST /v1/subscriptions/sub_1"); n != 1 {
		t.Errorf("%d updates, want 1", n)
	}
	if n := f.countRequests("POST /v1/invoices/"); n != 0 {
		t.Errorf("%d voids of a processing payment", n)
	}
	snap, err := p.GetSubscription(ctx, "sub_1")
	if err != nil || snap.Price != "price_1" || snap.UnpaidChange == "" || snap.Status != supporters.StatusActive {
		t.Errorf("while processing: %+v, %v", snap, err)
	}
	if err := p.DropUnpaidChange(ctx, "sub_1", "drop:1"); err != nil {
		t.Errorf("DropUnpaidChange = %v", err)
	}
	if n := f.countRequests("POST /v1/invoices/"); n != 0 {
		t.Errorf("DropUnpaidChange voided a processing payment")
	}
}

// The void fails after the decline: the snapshot does not show a late payment, whatever Stripe says
// of the status, it names the unpaid change, and DropUnpaidChange removes it.
func TestUnpaidChangeIsNotALatePayment(t *testing.T) {
	p, f, _, higher := onPlan(t, "")
	ctx := t.Context()
	setCard(f, fakeCardDeclines)
	f.mu.Lock()
	f.pastDueOnFailure = true
	f.before = func(r *http.Request) int {
		if strings.HasSuffix(r.URL.Path, "/void") {
			f.before = nil
			return http.StatusInternalServerError
		}
		return 0
	}
	f.mu.Unlock()
	if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); err == nil {
		t.Fatal("ChangePrice with the void refused succeeded")
	}
	snap, err := p.GetSubscription(ctx, "sub_1")
	if err != nil || snap.Status != supporters.StatusActive || !snap.LatestPaymentFailed.IsZero() || snap.UnpaidChange == "" || snap.Price != "price_1" {
		t.Fatalf("with the invoice open: %+v, %v", snap, err)
	}
	page, err := p.ListSubscriptions(ctx, "")
	if err != nil || len(page.Subscriptions) != 1 || page.Subscriptions[0].Status != supporters.StatusActive || page.Subscriptions[0].UnpaidChange == "" {
		t.Errorf("listed with the invoice open: %+v, %v", page, err)
	}
	if err := p.DropUnpaidChange(ctx, "sub_1", "drop:1"); err != nil {
		t.Fatal(err)
	}
	snap, err = p.GetSubscription(ctx, "sub_1")
	if err != nil || snap.UnpaidChange != "" || snap.Status != supporters.StatusActive {
		t.Errorf("after DropUnpaidChange: %+v, %v", snap, err)
	}
	if err := p.DropUnpaidChange(ctx, "sub_1", "drop:2"); err != nil {
		t.Errorf("DropUnpaidChange again = %v", err)
	}
}

// Stripe keeps a 500 under its key: a new attempt in the same minute gets it again, and one later
// goes through with a new key. A renewal's failed invoice stays a late payment.
func TestChangeNowStoredFailure(t *testing.T) {
	p, f, _, higher := onPlan(t, "")
	ctx := t.Context()
	at := p.o.Now()
	o := p.o
	o.Now = func() time.Time { return at }
	p, err := New(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.storeFailure = true
	f.mu.Unlock()
	for range 2 {
		if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); !errors.Is(err, supporters.ErrPaymentUnavailable) {
			t.Fatalf("ChangePrice = %v, want unavailable", err)
		}
	}
	at = at.Add(time.Minute)
	if err := p.ChangePrice(ctx, "sub_1", higher, supporters.ChangeNowProrated, "change:now"); err != nil {
		t.Fatalf("ChangePrice a minute later = %v", err)
	}
	wantPrices(t, p, higher, "")
}

// A failed renewal, without a pending update, is the late payment that it always was.
func TestFailedRenewalIsLate(t *testing.T) {
	s := scenario()
	failed := s.Now.Add(-time.Hour).Truncate(time.Second)
	s.Subscriptions = []supporters.SubscriptionSnapshot{{
		Ref: "sub_1", Customer: "cus_1", LocalID: "01890a5d-ac96-7000-8000-000000000002", Price: "price_1", Status: supporters.StatusPastDue,
		PeriodStart: s.Now, PeriodEnd: s.Now.AddDate(0, 1, 0), LatestPaymentFailed: failed,
	}}
	p, _, _ := newOnFake(t, s)
	snap, err := p.GetSubscription(t.Context(), "sub_1")
	if err != nil || snap.Status != supporters.StatusPastDue || !snap.LatestPaymentFailed.Equal(failed) || snap.UnpaidChange != "" {
		t.Errorf("failed renewal: %+v, %v", snap, err)
	}
}

// A notice of an invoice resolves to its subscription and the payment of its charge; the events
// of invoices name their subscriptions.
func TestResolveInvoice(t *testing.T) {
	s := scenario()
	s.Money = []supporterstest.PaymentScenarioMoney{{
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
		// Stripe's special cases: two decimal places, whatever ISO 4217 has.
		{500, "ISK", "5.00"}, {500, "UGX", "5.00"}, {17500, "HUF", "175.00"}, {80000, "TWD", "800.00"},
		// Zero decimal places at Stripe, two in ISO 4217.
		{500, "MGA", "500"}, {50, "KRW", "50"},
	} {
		if got := decimal(tc.minor, tc.cur); got != tc.want {
			t.Errorf("decimal(%d, %s) = %s, want %s", tc.minor, tc.cur, got, tc.want)
		}
	}
}

// stripeMinimums is the list of https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts
// as Stripe writes it, in major units.
var stripeMinimums = map[string]string{
	"USD": "0.50", "AED": "2.00", "ARS": "0.50", "AUD": "0.50", "BRL": "0.50", "CAD": "0.50", "CHF": "0.50",
	"COP": "0.50", "CZK": "15.00", "DKK": "2.50", "EUR": "0.50", "GBP": "0.30", "HKD": "4.00", "HUF": "175.00",
	"IDR": "0.50", "ILS": "0.50", "INR": "0.50", "JPY": "50", "KRW": "50", "MXN": "10", "MYR": "2.00",
	"NOK": "3.00", "NZD": "0.50", "PHP": "0.50", "PLN": "2.00", "RON": "2.00", "RUB": "0.50", "SEK": "3.00",
	"SGD": "0.50", "THB": "10", "ZAR": "0.50",
}

func TestMinimumAmount(t *testing.T) {
	p := &Provider{}
	for cur, major := range stripeMinimums {
		minor, ok := p.MinimumAmount(cur)
		if !ok {
			t.Errorf("MinimumAmount(%s) reports it as not taken", cur)
			continue
		}
		if got, err := finance.MinorUnitsOf(major, finance.Currency(cur)); err != nil || got != minor {
			t.Errorf("MinimumAmount(%s) = %d, want %d (%s), %v", cur, minor, got, major, err)
		}
	}
	if len(minimumAmounts) != len(stripeMinimums) {
		t.Errorf("%d minimums, want the %d of Stripe's list", len(minimumAmounts), len(stripeMinimums))
	}
	if n, ok := p.MinimumAmount("krw"); !ok || n != 50 {
		t.Errorf(`MinimumAmount("krw") = %d, %v, want any case to work`, n, ok)
	}
	for _, cur := range []string{"BGN", "ISK", "UGX", "TWD", "XXX"} {
		if n, ok := p.MinimumAmount(cur); ok {
			t.Errorf("MinimumAmount(%s) = %d, want it not taken", cur, n)
		}
	}
}

// A plan's amount reaches Stripe unconverted, so each currency it can use must have the same
// decimal places in ISO 4217 and at Stripe.
func TestMinimumAmountCurrenciesMatchISO(t *testing.T) {
	for cur := range minimumAmounts {
		iso, ok := finance.Currency(cur).MinorUnits()
		if !ok || iso != exponent(cur) {
			t.Errorf("%s has %d decimal places at Stripe, %d (%v) in ISO 4217", cur, exponent(cur), iso, ok)
		}
	}
}

func TestCheckPlanMinimum(t *testing.T) {
	p := &Provider{}
	for _, tc := range []struct {
		cur    string
		amount int64
		ok     bool
	}{
		{"KRW", 49, false}, {"KRW", 50, true}, {"ZAR", 49, false}, {"ZAR", 50, true}, {"MXN", 999, false}, {"MXN", 1000, true},
	} {
		err := p.checkPlan(supporters.PlanSpec{PlanID: "p", Name: "P", Prices: []supporters.PriceSpec{
			{Interval: supporters.IntervalMonth, Currency: tc.cur, AmountMinor: tc.amount},
		}})
		if (err == nil) != tc.ok {
			t.Errorf("checkPlan(%d %s) = %v, want ok = %v", tc.amount, tc.cur, err, tc.ok)
		}
		if err != nil && !errors.Is(err, supporters.ErrPaymentInvalidRequest) {
			t.Errorf("checkPlan(%d %s) = %v, want ErrPaymentInvalidRequest", tc.amount, tc.cur, err)
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

// oneCheckout returns a provider on the fake API, a request of a checkout, and its session.
func oneCheckout(t *testing.T) (*Provider, *fakeAPI, supporters.CheckoutRequest, supporters.Checkout) {
	t.Helper()
	p, f, _ := newOnFake(t, scenario())
	r, err := p.EnsurePlan(t.Context(), supporters.PlanSpec{PlanID: "plan", Name: "Plan", Prices: []supporters.PriceSpec{
		{Currency: "JPY", AmountMinor: 500, Interval: supporters.IntervalMonth},
		{Currency: "JPY", AmountMinor: 5000, Interval: supporters.IntervalYear},
	}})
	if err != nil {
		t.Fatal(err)
	}
	req := supporters.CheckoutRequest{
		LocalID: "local", Price: r.Prices[0].Ref, Email: "alice@example.com", Key: "first",
		SuccessURL: "https://my.example/ok?checkout=success", CancelURL: "https://my.example/ok?checkout=canceled",
	}
	c, err := p.StartCheckout(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Key = "second"
	return p, f, req, c
}

// A session is continued only for what a new session would be, and only while it is open.
func TestResumeCheckout(t *testing.T) {
	p, f, req, c := oneCheckout(t)
	res, page, err := p.ResumeCheckout(t.Context(), c.Ref, req)
	if err != nil || res.Status != supporters.CheckoutOpen || page == nil || page.RedirectURL != c.RedirectURL || !page.ExpiresAt.Equal(c.ExpiresAt) {
		t.Fatalf("ResumeCheckout = %+v, %+v, %v, want the page %+v", res, page, err, c)
	}
	for name, change := range map[string]func(*supporters.CheckoutRequest){
		"another price": func(r *supporters.CheckoutRequest) { r.Price = "price_other" },
		"a customer":    func(r *supporters.CheckoutRequest) { r.Customer, r.Email = "cus_alice", "" },
		"another email": func(r *supporters.CheckoutRequest) { r.Email = "bob@example.com" },
		"another return": func(r *supporters.CheckoutRequest) {
			r.SuccessURL = "https://other.example/ok"
		},
		"another cancel": func(r *supporters.CheckoutRequest) { r.CancelURL = "https://other.example/no" },
	} {
		other := req
		change(&other)
		if res, page, err := p.ResumeCheckout(t.Context(), c.Ref, other); err != nil || res.Status != supporters.CheckoutOpen || page != nil {
			t.Errorf("ResumeCheckout with %s = %+v, %+v, %v, want an open session without a page", name, res, page, err)
		}
	}
	f.paySession(t, string(c.Ref))
	if res, page, err := p.ResumeCheckout(t.Context(), c.Ref, req); err != nil || res.Status != supporters.CheckoutCompleted || page != nil {
		t.Errorf("ResumeCheckout of a paid session = %+v, %+v, %v, want completed without a page", res, page, err)
	}
}

// A tampered URL of an open session is not sent to the browser.
func TestResumeCheckoutChecksTheHost(t *testing.T) {
	p, f, req, c := oneCheckout(t)
	f.mu.Lock()
	f.checkouts[string(c.Ref)]["url"] = "https://evil.example/pay"
	f.mu.Unlock()
	if _, _, err := p.ResumeCheckout(t.Context(), c.Ref, req); !errors.Is(err, supporters.ErrInvalidPaymentResponse) {
		t.Errorf("ResumeCheckout with a URL elsewhere = %v, want ErrInvalidPaymentResponse", err)
	}
}

// Expiring makes a session unpayable; when Stripe refuses since the session just completed, the
// answer is the completed session, with its subscription.
func TestExpireCheckout(t *testing.T) {
	p, _, _, c := oneCheckout(t)
	if res, err := p.ExpireCheckout(t.Context(), c.Ref); err != nil || res.Status != supporters.CheckoutExpired {
		t.Fatalf("ExpireCheckout = %+v, %v, want expired", res, err)
	}
	if res, err := p.ExpireCheckout(t.Context(), c.Ref); err != nil || res.Status != supporters.CheckoutExpired {
		t.Errorf("ExpireCheckout again = %+v, %v, want expired", res, err)
	}
	if _, err := p.ExpireCheckout(t.Context(), "cs_unknown"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("ExpireCheckout of an unknown session = %v, want not_found", err)
	}

	p, f, _, c := oneCheckout(t)
	sub, _ := f.paySession(t, string(c.Ref))
	if res, err := p.ExpireCheckout(t.Context(), c.Ref); err != nil || res.Status != supporters.CheckoutCompleted || string(res.Subscription) != sub {
		t.Errorf("ExpireCheckout of a paid session = %+v, %v, want completed with %s", res, err, sub)
	}
}

// A refusal of Stripe for a session that stays open is an error: the caller must not make another page.
func TestExpireCheckoutRefusedWhileOpen(t *testing.T) {
	p, f, _, c := oneCheckout(t)
	f.mu.Lock()
	f.before = func(r *http.Request) int {
		if strings.HasSuffix(r.URL.Path, "/expire") {
			return http.StatusBadRequest
		}
		return 0
	}
	f.mu.Unlock()
	if res, err := p.ExpireCheckout(t.Context(), c.Ref); supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
		t.Errorf("ExpireCheckout refused while open = %+v, %v, want invalid_request", res, err)
	}
}

// Two sessions of one subscription paid: the refund takes the charge of the second alone, a repeat
// refunds nothing twice, and a key without the permission fails as permission.
func TestRefundSubscription(t *testing.T) {
	p, f, req, c := oneCheckout(t)
	second, err := p.StartCheckout(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	_, firstCharge := f.paySession(t, string(c.Ref))
	sub, charge := f.paySession(t, string(second.Ref))
	ref := supporters.SubscriptionRef(sub)
	if snap, err := p.GetSubscription(t.Context(), ref); err != nil || snap.LocalID != "local" {
		t.Fatalf("GetSubscription = %+v, %v, want our ID", snap, err)
	}
	f.mu.Lock()
	f.before = func(r *http.Request) int {
		if r.URL.Path == "/v1/refunds" && r.Method == http.MethodPost {
			return http.StatusForbidden
		}
		return 0
	}
	f.mu.Unlock()
	if err := p.RefundSubscription(t.Context(), ref, "k1"); supporters.ErrorKindOf(err) != supporters.ErrorPermission {
		t.Fatalf("RefundSubscription without the permission = %v, want permission", err)
	}
	f.mu.Lock()
	f.before = nil
	f.mu.Unlock()
	for _, key := range []string{"k2", "k3"} {
		if err := p.RefundSubscription(t.Context(), ref, key); err != nil {
			t.Fatalf("RefundSubscription(%s) = %v", key, err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.charges[charge]["refunded"] != true || f.charges[firstCharge]["refunded"] == true {
		t.Errorf("refunded: second %v, first %v, want only the second", f.charges[charge]["refunded"], f.charges[firstCharge]["refunded"])
	}
	if len(f.refunds) != 1 {
		t.Errorf("%d refunds, want 1", len(f.refunds))
	}
}

// A charge in a dispute cannot be refunded; the refund goes on without it.
func TestRefundSubscriptionLeavesADisputedCharge(t *testing.T) {
	p, f, _, c := oneCheckout(t)
	sub, charge := f.paySession(t, string(c.Ref))
	f.mu.Lock()
	f.charges[charge]["disputed"] = true
	f.mu.Unlock()
	if err := p.RefundSubscription(t.Context(), supporters.SubscriptionRef(sub), "k"); err != nil {
		t.Errorf("RefundSubscription = %v", err)
	}
}

// addPaidInvoice adds a paid invoice of a subscription, a renewal, and returns its charge.
func (f *fakeAPI) addPaidInvoice(sub string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, charge := f.nextID("in"), f.nextID("ch")
	f.invoices[inv] = obj{"id": inv, "object": "invoice", "status": "paid", "my_subscription": sub}
	f.charges[charge] = obj{"id": charge, "object": "charge"}
	f.invoicePays = append(f.invoicePays, obj{
		"id": "inpay_" + inv, "object": "invoice_payment", "status": "paid", "invoice": ref{id: inv},
		"payment": obj{"type": "charge", "charge": ref{id: charge}},
	})
	return charge
}

// A second subscription gave nothing for any period, so its renewals are refunded too, and a charge
// that fails does not stop the others: the failure comes back after them for another try.
func TestRefundSubscriptionTakesRenewalsAndGoesOnAfterAFailure(t *testing.T) {
	p, f, _, c := oneCheckout(t)
	sub, first := f.paySession(t, string(c.Ref))
	renewal := f.addPaidInvoice(sub)
	f.mu.Lock()
	f.before = func(r *http.Request) int {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/refunds" && r.Form.Get("charge") == first {
			return http.StatusInternalServerError
		}
		return 0
	}
	f.mu.Unlock()
	if err := p.RefundSubscription(t.Context(), supporters.SubscriptionRef(sub), "k1"); err == nil {
		t.Error("RefundSubscription with a failed charge succeeded")
	}
	f.mu.Lock()
	if f.charges[renewal]["refunded"] != true {
		t.Errorf("the renewal = %v, want refunded after the other charge failed", f.charges[renewal])
	}
	f.before = nil
	f.mu.Unlock()
	if err := p.RefundSubscription(t.Context(), supporters.SubscriptionRef(sub), "k2"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.charges[first]["refunded"] != true || len(f.refunds) != 2 {
		t.Errorf("first charge = %v with %d refunds, want refunded and two", f.charges[first], len(f.refunds))
	}
}
