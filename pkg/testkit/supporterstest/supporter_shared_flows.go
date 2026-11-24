// SPDX-License-Identifier: AGPL-3.0-only

package supporterstest

import (
	"testing"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	domain "github.com/mycroblog/my/internal/domain/supporters"
)

// The income of a provider's account that other instances may share (payments_account_shared).

// accountMoney is the money of an account that this instance may share: ours, the movements of a
// subscription and a tip of ours, and the others, those that no subscription or tip of ours
// accounts for.
type accountMoney struct {
	e *tipEnv
	// subID is our subscription, and tipID our tip.
	subID, tipID string
	// subPay and subRefund are our subscription's payment and its partial refund; tipPay and
	// tipChargeback our tip's payment and its chargeback.
	subPay, subRefund, tipPay, tipChargeback supporters.MoneyMovement
	// foreignPay and foreignRefund are the payment of another instance's subscription and its
	// refund; stray a payment of no subscription that is no tip of ours; fee a fee of the account.
	foreignPay, foreignRefund, stray, fee supporters.MoneyMovement
}

func (a *accountMoney) ours() []supporters.MoneyMovement {
	return []supporters.MoneyMovement{a.subPay, a.subRefund, a.tipPay, a.tipChargeback}
}

func (a *accountMoney) others() []supporters.MoneyMovement {
	return []supporters.MoneyMovement{a.foreignPay, a.foreignRefund, a.stray, a.fee}
}

// recordAccountMoney builds a service whose account is shared or not, subscribes a user and has
// another tip, and hands every movement of the account to RecordMoney, one at a time, as the
// webhooks and the reconciliation do.
func recordAccountMoney(t *testing.T, se *supporterEnv, shared bool) *accountMoney {
	t.Helper()
	ctx := t.Context()
	se.deps.SharedAccount = shared
	e := newTipEnv(t, se)
	plan := e.plan(t, "supporter", 1, 10, 500)
	subID, ref := e.subscribe(t, e.user("alice"), plan, domain.IntervalMonth)
	now := e.Clock.Now()
	a := &accountMoney{e: e, subID: subID}
	a.subPay = supporters.MoneyMovement{
		ID: "txn_our_pay", Kind: supporters.MoneyPayment, Subscription: ref, Payment: "pi_our_pay", OccurredAt: now,
		OccurredOn: supporters.DateOf(now), Covers: supporters.DateRange{Start: supporters.DateOf(now), End: supporters.DateOf(now.AddDate(0, 1, 0))},
		Gross: "500", Fee: "18", Net: "482", Currency: "JPY", Final: true,
	}
	a.subRefund = supporters.MoneyMovement{
		ID: "txn_our_refund", Kind: supporters.MoneyRefund, ParentID: a.subPay.ID, ParentGross: a.subPay.Gross, Subscription: ref,
		Payment: a.subPay.Payment, OccurredAt: now, OccurredOn: supporters.DateOf(now), Gross: "-100", Fee: "0", Net: "-100", Currency: "JPY", Final: true,
	}
	a.foreignPay = a.subPay
	a.foreignPay.ID, a.foreignPay.Subscription, a.foreignPay.Payment = "txn_foreign_pay", "sub_elsewhere", "pi_elsewhere"
	a.foreignRefund = a.subRefund
	a.foreignRefund.ID, a.foreignRefund.ParentID, a.foreignRefund.Subscription, a.foreignRefund.Payment = "txn_foreign_refund", a.foreignPay.ID, "sub_elsewhere", "pi_elsewhere"
	bob := e.user("bob")
	st, tipRef := e.tip(t, bob, 1000)
	a.tipID = st.TipID
	a.tipPay = e.pay(t, bob, st.TipID, tipRef)
	a.tipChargeback = child("txn_tip_cb", supporters.MoneyChargeback, a.tipPay, "-1000", now)
	a.stray = a.tipPay
	a.stray.ID, a.stray.Payment = "txn_stray", "pi_stray"
	a.fee = supporters.MoneyMovement{
		ID: "txn_account_fee", Kind: supporters.MoneyFee, OccurredAt: now, OccurredOn: supporters.DateOf(now),
		Gross: "-300", Fee: "0", Net: "-300", Currency: "JPY", Final: true,
	}
	for _, m := range []supporters.MoneyMovement{a.subPay, a.foreignPay, a.subRefund, a.foreignRefund, a.tipPay, a.stray, a.tipChargeback, a.fee} {
		if err := e.svc.RecordMoney(ctx, paymentProvider, []supporters.MoneyMovement{m}); err != nil {
			t.Fatalf("RecordMoney(%s) = %v", m.ID, err)
		}
	}
	e.run(t)
	return a
}

// checkOurs checks that the income holds our movements with the source, the subscription or the
// tip, and the fee of each.
func (a *accountMoney) checkOurs(t *testing.T) {
	t.Helper()
	for _, m := range []supporters.MoneyMovement{a.subPay, a.subRefund} {
		got, ok := a.e.income.get(m.ID)
		if !ok || got.Source != string(finance.SourceSubscription) || got.SubscriptionID != a.subID || got.Fee != m.Fee || got.Gross != m.Gross {
			t.Errorf("income of our subscription's %s = %+v, %v, want it with its subscription and fee", m.ID, got, ok)
		}
	}
	for _, m := range []supporters.MoneyMovement{a.tipPay, a.tipChargeback} {
		got, ok := a.e.income.get(m.ID)
		if !ok || got.Source != string(finance.SourceOneTime) || got.Fee != m.Fee || got.Gross != m.Gross {
			t.Errorf("income of our tip's %s = %+v, %v, want it one_time with its fee", m.ID, got, ok)
		}
	}
	if got, _ := a.e.income.get(a.tipPay.ID); got.TipID != a.tipID {
		t.Errorf("income of our tip's payment = %+v, want the tip %s", got, a.tipID)
	}
}

// On an account that other instances share, the income holds only the movements of our
// subscriptions and tips, with their refunds and chargebacks and their fees: those of another
// instance's subscription, a payment that is no tip of ours, and a fee of the account are dropped.
func supporterSharedAccountIncome(t *testing.T, se *supporterEnv) {
	a := recordAccountMoney(t, se, true)
	a.checkOurs(t)
	for _, m := range a.others() {
		if got, ok := a.e.income.get(m.ID); ok {
			t.Errorf("income of %s (%s) on a shared account = %+v, want none", m.ID, m.Kind, got)
		}
	}
	if n, want := a.e.income.count(), len(a.ours()); n != want {
		t.Errorf("income rows = %d, want %d, ours alone", n, want)
	}
}

// On an account of its own, the income holds every movement of the account: ours with their
// sources, and the others of an unknown source.
func supporterOwnAccountIncome(t *testing.T, se *supporterEnv) {
	a := recordAccountMoney(t, se, false)
	a.checkOurs(t)
	for _, m := range a.others() {
		got, ok := a.e.income.get(m.ID)
		if !ok || got.Source != string(finance.SourceUnknown) || got.SubscriptionID != "" || got.TipID != "" {
			t.Errorf("income of %s (%s) on an account of its own = %+v, %v, want it of an unknown source", m.ID, m.Kind, got, ok)
		}
	}
	if n, want := a.e.income.count(), len(a.ours())+len(a.others()); n != want {
		t.Errorf("income rows = %d, want %d, every movement", n, want)
	}
}
