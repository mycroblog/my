// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/pkg/testkit"
)

const testToken = "whook_test_token"

// testHost is the origin of the scenarios' return URLs, which the pages are on.
const testHost = "my.example"

func keysOf(mode supporters.Mode) (secret, public string) {
	if mode == supporters.ModeLive {
		return "sk_live_secret", "pk_live_public"
	}
	return "sk_test_secret", "pk_test_public"
}

func noSleep(context.Context, time.Duration) error { return nil }

// newOnFake returns a provider on a fake PAY.JP whose clock is now.
func newOnFake(t *testing.T, mode supporters.Mode, now time.Time) (*Provider, *fakeAPI) {
	t.Helper()
	secret, public := keysOf(mode)
	f := newFakeAPI(t, secret, mode == supporters.ModeLive, func() time.Time { return now })
	p, err := New(Options{
		SecretKey: secret, PublicKey: public, WebhookToken: testToken, URL: f.srv.URL,
		RequestsPerSecond: 1e6, Now: func() time.Time { return now }, Sleep: noSleep,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, f
}

// statusOfFailure is the status of PAY.JP that fails a request with err.
func statusOfFailure(err error) int {
	switch {
	case errors.Is(err, supporters.ErrPaymentPermission):
		return http.StatusUnauthorized
	case errors.Is(err, supporters.ErrPaymentThrottled):
		return http.StatusTooManyRequests
	}
	return http.StatusServiceUnavailable
}

// rateFor returns a fee rate in percent whose fee of gross is fee, rounded down.
func rateFor(t *testing.T, gross, fee int64) string {
	t.Helper()
	bp := (fee*10000 + gross - 1) / gross
	got, _ := feeOf(gross, fmtRate(bp))
	if got != fee {
		t.Fatalf("no rate makes a fee of %d of %d", fee, gross)
	}
	return fmtRate(bp)
}

func fmtRate(bp int64) string {
	return strings.TrimSpace(strings.Join([]string{itoa(bp / 100), pad2(bp % 100)}, "."))
}

func itoa(n int64) string { return yen(n) }

func pad2(n int64) string {
	s := yen(n)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

// onScenario builds a fake PAY.JP that holds what it can of s, and the expectations of what the
// adapter reports of it (testkit.PaymentExpected).
func onScenario(t *testing.T, s testkit.PaymentScenario) testkit.PaymentUnderTest {
	p, f := newOnFake(t, s.Mode, s.Now)
	f.addPlan("price_base", 500, "month", nil)
	f.addPlan("price_lower", 300, "month", nil)
	exp := &testkit.PaymentExpected{
		Currencies: []string{"JPY"}, Snapshots: map[supporters.SubscriptionRef]*supporters.SubscriptionSnapshot{},
		Gone: func(ref supporters.SubscriptionRef) supporters.SubscriptionSnapshot {
			return supporters.SubscriptionSnapshot{Ref: ref, Status: supporters.StatusCanceled}
		},
		Notices:         []supporters.NoticeKind{supporters.NoticeSubscription, supporters.NoticeMoney, supporters.NoticeIgnored},
		NoChangesWindow: true,
	}
	for i, snap := range s.Subscriptions {
		sub := subscription{
			ID: string(snap.Ref), Customer: string(snap.Customer), Status: subActive,
			CurrentPeriodStart: snap.PeriodStart.Unix(), CurrentPeriodEnd: snap.PeriodEnd.Unix(), Metadata: metadata{},
		}
		if snap.LocalID != "" {
			sub.Metadata[metaSubscriptionID] = snap.LocalID
		}
		if pl, ok := f.plans[string(snap.Price)]; ok {
			sub.Plan = pl.plan
		}
		if pl, ok := f.plans[string(snap.ScheduledPrice)]; ok {
			np := pl.plan
			sub.NextCyclePlan = &np
		}
		want := snap
		switch snap.Status {
		case supporters.StatusActive:
			if snap.CancelAtPeriodEnd {
				sub.Status = subCanceled
			}
		case supporters.StatusPastDue:
			failed := snap.LatestPaymentFailed
			if failed.IsZero() {
				failed = snap.PeriodStart
				want.LatestPaymentFailed = failed
			}
			sub.Status, sub.PausedAt = subPaused, failed.Unix()
		case supporters.StatusCanceled:
			exp.Snapshots[snap.Ref] = &supporters.SubscriptionSnapshot{Ref: snap.Ref, Status: supporters.StatusCanceled}
			continue
		default:
			exp.Snapshots[snap.Ref] = nil
			continue
		}
		exp.Snapshots[snap.Ref] = &want
		f.addSub(sub, snap.PeriodStart.Unix()-int64(i))
	}
	for _, c := range s.Checkouts {
		want := c
		switch c.Status {
		case supporters.CheckoutCompleted:
			f.addSub(subscription{
				ID: string(c.Subscription), Customer: string(c.Customer), Plan: f.plans["price_base"].plan, Status: subActive,
				CurrentPeriodStart: s.Now.Unix(), CurrentPeriodEnd: s.Now.AddDate(0, 1, 0).Unix(),
				Metadata: metadata{metaSubscriptionID: c.LocalID},
			}, s.Now.Unix())
			want.Ref = checkoutRef(string(c.Customer), c.LocalID, s.Now.Add(time.Hour).Unix())
		case supporters.CheckoutExpired:
			want.Ref = checkoutRef(newCustomerPrefix+c.LocalID, c.LocalID, s.Now.Add(-time.Hour).Unix())
		default:
			want.Ref = checkoutRef(newCustomerPrefix+c.LocalID, c.LocalID, s.Now.Add(time.Hour).Unix())
		}
		exp.Checkouts = append(exp.Checkouts, want)
	}
	exp.Money = moneyOnFake(t, f, s)
	for _, n := range s.Changes {
		f.addEvent(n.EventID, n.Type, n.OccurredAt.Unix(), n.ObjectID, "subscription", map[string]any{"id": n.ObjectID, "object": "subscription"})
	}
	if s.Failure != nil {
		f.fail(statusOfFailure(s.Failure))
	}
	return testkit.PaymentUnderTest{
		Provider: p, Deliver: deliver(s.Now), CheckoutHosts: []string{testHost}, PortalHosts: []string{testHost}, Expected: exp,
	}
}

// moneyOnFake puts the JPY payments, refunds, and chargebacks of s on f as charges and events,
// and returns what the adapter reports of each of s.Money: none for a payment in another
// currency, a reversal of a chargeback, or a fee of its own, which PAY.JP does not have.
func moneyOnFake(t *testing.T, f *fakeAPI, s testkit.PaymentScenario) []supporters.MoneyMovement {
	out := make([]supporters.MoneyMovement, len(s.Money))
	rates := map[string]string{}
	charges := map[string]string{} // scenario ID → charge ID
	gross := map[string]int64{}
	for i, m := range s.Money {
		id := m.Object
		if id == "" {
			id = m.ID
		}
		if m.Currency != "JPY" {
			continue
		}
		at := m.OccurredAt
		switch m.Kind {
		case supporters.MoneyPayment:
			if _, ok := f.subs[string(m.Subscription)]; !ok {
				f.addSub(subscription{
					ID: string(m.Subscription), Customer: "cus_" + string(m.Subscription), Plan: f.plans["price_base"].plan,
					Status: subActive, CurrentPeriodStart: at.Unix(), CurrentPeriodEnd: at.AddDate(0, 1, 0).Unix(),
				}, at.Unix())
			}
			rate := rateFor(t, m.GrossMinor, m.FeeMinor)
			rates[m.ID], charges[m.ID], gross[m.ID] = rate, id, m.GrossMinor
			f.addCharge(charge{
				ID: id, Amount: m.GrossMinor, Captured: !m.Pending, Created: at.Unix(), Currency: currency,
				FeeRate: rate, Paid: true, Subscription: string(m.Subscription), CapturedAt: capturedAt(m),
			}, "evnt_"+id)
			want := newMovement(id, supporters.MoneyPayment, at, m.GrossMinor, m.FeeMinor)
			start := supporters.DateOf(at.In(tokyo))
			want.Subscription = m.Subscription
			want.Covers = supporters.DateRange{Start: start, End: supporters.DateOf(at.In(tokyo).AddDate(0, 1, -1))}
			want.Final = !m.Pending
			out[i] = want
		case supporters.MoneyRefund, supporters.MoneyChargeback:
			parent, ok := charges[m.ParentID]
			if !ok {
				continue
			}
			if m.Kind == supporters.MoneyRefund {
				f.refund(parent, -m.GrossMinor, at.Unix(), id)
			} else {
				f.dispute(parent, -m.GrossMinor, at.Unix(), id)
			}
			fee, _ := feeOf(m.GrossMinor, rates[m.ParentID])
			want := newMovement(id, m.Kind, at, m.GrossMinor, fee)
			want.ParentID, want.ParentGross, want.Subscription = parent, yen(gross[m.ParentID]), m.Subscription
			out[i] = want
		}
	}
	return out
}

func capturedAt(m testkit.PaymentScenarioMoney) int64 {
	if m.Pending {
		return 0
	}
	return m.OccurredAt.Unix()
}

// deliver returns the webhook deliveries of notices (testkit.PaymentDeliver): the token is the
// account's with no key or a key "", and the first other key otherwise.
func deliver(now time.Time) testkit.PaymentDeliver {
	return func(n supporters.Notice, live bool, _ time.Time, keys ...string) (http.Header, []byte) {
		typ, object := "customer.updated", "customer"
		switch n.Kind {
		case supporters.NoticeSubscription:
			typ, object = "subscription.updated", "subscription"
		case supporters.NoticeMoney:
			typ, object = "charge.succeeded", "charge"
		}
		token := testToken
		if len(keys) > 0 && keys[0] != "" {
			token = keys[0]
		}
		for _, k := range keys {
			if k == "" {
				token = testToken
			}
		}
		body, _ := json.Marshal(map[string]any{
			"id": n.EventID, "object": "event", "livemode": live, "type": typ, "created": now.Unix(),
			"data": map[string]any{"id": n.ObjectID, "object": object},
		})
		h := http.Header{}
		h.Set(TokenHeader, token)
		h.Set("Content-Type", "application/json; charset=utf-8")
		return h, body
	}
}

func TestPaymentProviderContract(t *testing.T) {
	testkit.PaymentProviderContract(t, onScenario)
}
