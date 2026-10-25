// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/testkit"
)

// newOnScenario returns a Provider whose side holds s.
func newOnScenario(_ *testing.T, s testkit.PaymentScenario) testkit.PaymentUnderTest {
	p := New(Options{Mode: s.Mode, Location: s.Location, Now: func() time.Time { return s.Now }})
	for _, sub := range s.Subscriptions {
		p.PutSubscription(sub)
	}
	for _, c := range s.Checkouts {
		p.PutCheckout(c)
	}
	for i, m := range testkit.PaymentMovements(s) {
		object := s.Money[i].Object
		if object == "" {
			object = m.ID
		}
		p.PutMoneyObject(object, m)
	}
	for _, n := range s.Changes {
		p.PutChange(n)
	}
	p.Fail(s.Failure)
	return testkit.PaymentUnderTest{
		Provider: p, Deliver: p.Deliver, CheckoutHosts: []string{CheckoutHost}, PortalHosts: []string{PortalHost}, TimedSignature: true,
	}
}

func TestPaymentProviderContract(t *testing.T) {
	testkit.PaymentProviderContract(t, newOnScenario)
	t.Run("WithoutTimedSignature", func(t *testing.T) {
		testkit.PaymentProviderContract(t, func(t *testing.T, s testkit.PaymentScenario) testkit.PaymentUnderTest {
			u := newOnScenario(t, s)
			u.TimedSignature = false
			return u
		})
	})
}

// A pending movement that becomes final is listed after the cursor that passed its time.
func TestPendingMoneyBecomesFinal(t *testing.T) {
	p := New(Options{})
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	m := testkit.PaymentMovements(testkit.PaymentScenario{Location: time.UTC, Money: []testkit.PaymentScenarioMoney{
		{ID: "txn_1", Kind: "payment", OccurredAt: at, GrossMinor: 1000, Currency: "JPY", Pending: true},
		{ID: "txn_2", Kind: "payment", OccurredAt: at.Add(time.Hour), GrossMinor: 1000, Currency: "JPY"},
	}})
	p.PutMoney(m[0])
	p.PutMoney(m[1])
	page, err := p.ListMoney(t.Context(), time.Time{}, "")
	if err != nil || len(page.Movements) != 1 || page.Movements[0].ID != "txn_2" {
		t.Fatalf("ListMoney = %+v, %v, want txn_2 alone", page, err)
	}
	m[0].Final = true
	p.PutMoney(m[0])
	next, err := p.ListMoney(t.Context(), time.Time{}, page.Next)
	if err != nil || len(next.Movements) != 1 || next.Movements[0].ID != "txn_1" {
		t.Fatalf("ListMoney after the cursor = %+v, %v, want txn_1", next, err)
	}
}
