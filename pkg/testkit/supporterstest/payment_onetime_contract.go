// SPDX-License-Identifier: AGPL-3.0-only

package supporterstest

import (
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/pkg/testkit"
)

// paymentOneTimeScenario has a paid tip and an expired one, and the money of two paid tips: a
// full refund of the first, a partial refund and a chargeback with its reversal of the second.
func paymentOneTimeScenario(t *testing.T) PaymentScenario {
	s := paymentScenario(t)
	at := func(h int) time.Time { return paymentNow.Add(-time.Duration(48-h) * time.Hour) }
	s.OneTimeCheckouts = []supporters.OneTimeResult{
		{Ref: "cs_tip_paid", Status: supporters.OneTimePaid, LocalID: testkit.NamedID("tip-paid"), Payment: "pi_tip_1", AmountMinor: 1000, Currency: "JPY"},
		{Ref: "cs_tip_gone", Status: supporters.OneTimeExpired, LocalID: testkit.NamedID("tip-gone")},
	}
	s.Money = []PaymentScenarioMoney{
		{ID: "txn_tip_1", Object: "ch_tip_1", Kind: supporters.MoneyPayment, Payment: "pi_tip_1", OccurredAt: at(0), GrossMinor: 1000, FeeMinor: 36, Currency: "JPY"},
		{ID: "txn_tip_2", Object: "ch_tip_2", Kind: supporters.MoneyPayment, Payment: "pi_tip_2", OccurredAt: at(1), GrossMinor: 3000, FeeMinor: 108, Currency: "JPY"},
		{ID: "txn_tip_1r", Object: "re_tip_1", Kind: supporters.MoneyRefund, ParentID: "txn_tip_1", Payment: "pi_tip_1", OccurredAt: at(2), GrossMinor: -1000, Currency: "JPY"},
		{ID: "txn_tip_2r", Object: "re_tip_2", Kind: supporters.MoneyRefund, ParentID: "txn_tip_2", Payment: "pi_tip_2", OccurredAt: at(3), GrossMinor: -500, Currency: "JPY"},
		{ID: "txn_tip_2c", Object: "dp_tip_2", Kind: supporters.MoneyChargeback, ParentID: "txn_tip_2", Payment: "pi_tip_2", OccurredAt: at(4), GrossMinor: -2500, FeeMinor: 1500, Currency: "JPY"},
		{ID: "txn_tip_2cr", Object: "dp_tip_2r", Kind: supporters.MoneyChargebackReversal, ParentID: "txn_tip_2", Payment: "pi_tip_2", OccurredAt: at(5), GrossMinor: 2500, Currency: "JPY"},
	}
	return s
}

// paymentOneTime verifies the one-time payments of a provider that takes them (ADR-0162 §9): the
// limits and the refusal of an amount outside them, a checkout on the allowed hosts that reports
// our ID, its expiry and an expiry that comes after the payment, the paid checkouts with what they
// charged, the notices of their checkouts, and the movements of their money, whose refunds,
// chargebacks, and reversals name the payment of their parent. A movement that the provider
// cannot hold (an entry of PaymentExpected.Money without an ID, such as a reversal that a provider
// does not report) is skipped.
func paymentOneTime(t *testing.T, newProvider PaymentProviderFactory) {
	ctx := t.Context()
	s := paymentOneTimeScenario(t)
	u := newProvider(t, s)
	p, ok := u.Provider.(supporters.OneTimePayments)
	if !ok {
		t.Skip("the provider takes no one-time payments")
	}
	minimum, maximum, ok := p.OneTimeLimits("JPY")
	if !ok || minimum <= 0 || maximum < minimum {
		t.Fatalf("OneTimeLimits(JPY) = %d, %d, %v, want a positive range", minimum, maximum, ok)
	}
	if _, _, ok := p.OneTimeLimits("XXX"); ok {
		t.Error("OneTimeLimits(XXX) reports the limits of a currency that does not exist")
	}
	req := supporters.OneTimeRequest{
		LocalID: testkit.NamedID("tip-new"), AmountMinor: max(minimum, 500), Currency: "JPY", Description: "Tip for my.example",
		Email: "alice@example.com", Locale: "ja", SuccessURL: "https://my.example/settings/supporter/tip",
		CancelURL: "https://my.example/settings/supporter/tip", Key: "tip:new",
	}
	c, err := p.StartOneTimeCheckout(ctx, req)
	if err != nil {
		t.Fatalf("StartOneTimeCheckout = %v", err)
	}
	if c.Ref == "" || !allowedURL(c.RedirectURL, u.CheckoutHosts) || !c.ExpiresAt.After(s.Now) {
		t.Errorf("StartOneTimeCheckout = %+v, want a checkout on %v that expires after %s", c, u.CheckoutHosts, s.Now)
	}
	if again, err := p.StartOneTimeCheckout(ctx, req); err != nil || again.Ref != c.Ref {
		t.Errorf("StartOneTimeCheckout with the same key = %+v, %v, want %s", again, err, c.Ref)
	}
	if got, err := p.GetOneTimeCheckout(ctx, c.Ref); err != nil || got.Status != supporters.OneTimeOpen || got.LocalID != req.LocalID {
		t.Errorf("GetOneTimeCheckout(new) = %+v, %v, want open with our ID", got, err)
	}
	for range 2 {
		if res, err := p.ExpireOneTimeCheckout(ctx, c.Ref); err != nil || res.Status != supporters.OneTimeExpired {
			t.Errorf("ExpireOneTimeCheckout = %+v, %v, want expired", res, err)
		}
	}
	for _, amount := range []int64{minimum - 1, maximum + 1} {
		bad := req
		bad.AmountMinor, bad.Key = amount, "tip:bad"
		if _, err := p.StartOneTimeCheckout(ctx, bad); supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
			t.Errorf("StartOneTimeCheckout of %d JPY = %v, want invalid_request", amount, err)
		}
	}
	for _, want := range s.OneTimeCheckouts {
		if got, err := p.GetOneTimeCheckout(ctx, want.Ref); err != nil || got != want {
			t.Errorf("GetOneTimeCheckout(%s) = %+v, %v, want %+v", want.Ref, got, err, want)
		}
		// An expiry that comes after the payment reports the payment.
		if got, err := p.ExpireOneTimeCheckout(ctx, want.Ref); err != nil || got != want {
			t.Errorf("ExpireOneTimeCheckout(%s) = %+v, %v, want %+v", want.Ref, got, err, want)
		}
	}
	if _, err := p.GetOneTimeCheckout(ctx, "cs_unknown"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("GetOneTimeCheckout(unknown) = %v, want not_found", err)
	}
	ch, err := u.Provider.Resolve(ctx, supporters.Notice{EventID: "evt_tip", Type: "checkout", Kind: supporters.NoticeCheckout, ObjectID: "cs_tip_paid"})
	if err != nil || ch.OneTimeCheckout != "cs_tip_paid" || ch.Checkout != "" {
		t.Errorf("Resolve(one-time checkout) = %+v, %v, want the one-time checkout cs_tip_paid alone", ch, err)
	}
	all := PaymentMovements(s)
	if e := u.expected(); e.Money != nil {
		all = e.Money
	}
	got := listMoney(t, u.Provider, time.Time{})
	payments := map[string]supporters.PaymentRef{}
	for _, m := range got {
		payments[m.ID] = m.Payment
	}
	for _, want := range all {
		if want.ID == "" {
			continue
		}
		i := slices.IndexFunc(got, func(m supporters.MoneyMovement) bool { return m.ID == want.ID })
		switch {
		case i < 0:
			t.Errorf("ListMoney lacks %s", want.ID)
		case !sameMovement(got[i], want):
			t.Errorf("ListMoney has %+v, want %+v", got[i], want)
		case want.Kind.HasParent() && (got[i].Payment == "" || got[i].Payment != payments[want.ParentID]):
			t.Errorf("%s (%s) names the payment %q, want %q of its parent", want.ID, want.Kind, got[i].Payment, payments[want.ParentID])
		}
	}
	for i, m := range s.Money {
		if all[i].ID == "" {
			continue
		}
		ch, err := u.Provider.Resolve(ctx, supporters.Notice{EventID: "evt_" + m.ID, Type: "money", Kind: supporters.NoticeMoney, ObjectID: m.object()})
		if err != nil || !slices.ContainsFunc(ch.Money, func(got supporters.MoneyMovement) bool { return sameMovement(got, all[i]) }) {
			t.Errorf("Resolve(%s) = %+v, %v, want %+v", m.ID, ch.Money, err, all[i])
		}
	}
}
