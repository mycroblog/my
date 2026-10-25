// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

var unitNow = time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)

func TestCheckKeys(t *testing.T) {
	for _, tc := range []struct {
		secret, public string
		ok             bool
	}{
		{"sk_test_a", "pk_test_b", true},
		{"sk_live_a", "pk_live_b", true},
		{"sk_test_a", "pk_live_b", false},
		{"rk_test_a", "pk_test_b", false},
		{"sk_test_a", "", false},
	} {
		if err := CheckKeys(tc.secret, tc.public); (err == nil) != tc.ok {
			t.Errorf("CheckKeys(%s, %s) = %v, want ok %v", tc.secret, tc.public, err, tc.ok)
		}
	}
}

func TestFeeOf(t *testing.T) {
	for _, tc := range []struct {
		amount int64
		rate   string
		want   int64
	}{
		{1000, "3.00", 30}, {500, "3.6", 18}, {333, "3.60", 11}, {-300, "3.60", -10}, {1000, "0", 0},
	} {
		if got, err := feeOf(tc.amount, tc.rate); err != nil || got != tc.want {
			t.Errorf("feeOf(%d, %s) = %d, %v, want %d", tc.amount, tc.rate, got, err, tc.want)
		}
	}
	if _, err := feeOf(1000, "x"); supporters.ErrorKindOf(err) != supporters.ErrorInvalidResponse {
		t.Errorf("feeOf(rate x) = %v, want invalid_response", err)
	}
}

func TestProratedDifference(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	month500 := plan{Amount: 500, Interval: "month"}
	month1000 := plan{Amount: 1000, Interval: "month"}
	year5000 := plan{Amount: 5000, Interval: "year"}
	half := start.Add(end.Sub(start) / 2)
	if got := proratedDifference(month500, month1000, start, end, half); got != 250 {
		t.Errorf("half of a month from 500 to 1000 = %d, want 250", got)
	}
	if got := proratedDifference(month500, year5000, start, end, half); got > 0 {
		t.Errorf("a month of 500 to a year of 5000 = %d, want none (cheaper per second)", got)
	}
	if got := proratedDifference(month1000, month500, start, end, half); got > 0 {
		t.Errorf("a lower plan = %d, want none", got)
	}
	if got := proratedDifference(month500, month1000, start, end, end.Add(time.Hour)); got != 0 {
		t.Errorf("after the period = %d, want 0", got)
	}
}

// A change now charges the prorated difference once, however often it is repeated, and keeps
// the period by a trial to its end.
func TestChangeNowChargesTheDifferenceOnce(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addPlan("low", 500, "month", nil)
	f.addPlan("high", 3000, "month", nil)
	start, end := unitNow.Add(-15*24*time.Hour), unitNow.Add(15*24*time.Hour)
	f.addSub(subscription{
		ID: "sub_1", Customer: "cus_1", Plan: f.plans["low"].plan, Status: subActive,
		CurrentPeriodStart: start.Unix(), CurrentPeriodEnd: end.Unix(), Metadata: metadata{metaSubscriptionID: "local-1"},
	}, start.Unix())
	f.customers["cus_1"].DefaultCard = "car_1"
	for range 2 {
		if err := p.ChangePrice(t.Context(), "sub_1", "high", supporters.ChangeNowProrated, "job-1:change"); err != nil {
			t.Fatal(err)
		}
		// A repeat after the plan changed charges nothing either.
		f.subs["sub_1"].Plan = f.plans["low"].plan
	}
	var charges []charge
	for _, c := range f.charges {
		charges = append(charges, c.charge)
	}
	if len(charges) != 1 {
		t.Fatalf("charges = %+v, want one charge of the difference", charges)
	}
	c := charges[0]
	want := proratedDifference(f.plans["low"].plan, f.plans["high"].plan, start, end, unitNow)
	if c.Amount != want || c.Metadata[metaChange] != "job-1:change" || c.Metadata[metaSubscription] != "sub_1" ||
		c.Metadata[metaSubscriptionID] != "local-1" || c.Metadata[metaPeriodEnd] != yen(end.Unix()) {
		t.Errorf("charge = %+v, want %d of the change with its metadata", c, want)
	}
	if s := f.subs["sub_1"]; s.Status != subTrial {
		t.Errorf("subscription status = %s, want trial to the period's end", s.Status)
	}
	// The difference's payment covers the rest of the period.
	ms, err := p.moneyOf(t.Context(), c.ID)
	if err != nil || len(ms) != 1 || ms[0].Subscription != "sub_1" ||
		ms[0].Covers != (supporters.DateRange{Start: supporters.DateOf(unitNow.In(tokyo)), End: supporters.DateOf(end.In(tokyo).AddDate(0, 0, -1))}) {
		t.Errorf("money of the difference = %+v, %v", ms, err)
	}
}

func TestChangeNowRefusedCardKeepsThePlan(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addPlan("low", 500, "month", nil)
	f.addPlan("high", 3000, "month", nil)
	f.addSub(subscription{
		ID: "sub_1", Customer: "cus_1", Plan: f.plans["low"].plan, Status: subActive,
		CurrentPeriodStart: unitNow.Add(-24 * time.Hour).Unix(), CurrentPeriodEnd: unitNow.Add(29 * 24 * time.Hour).Unix(),
	}, unitNow.Unix())
	f.customers["cus_1"].DefaultCard = "declined"
	err := p.ChangePrice(t.Context(), "sub_1", "high", supporters.ChangeNowProrated, "job-1:change")
	if supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
		t.Errorf("ChangePrice with a declined card = %v, want invalid_request", err)
	}
	if got := f.subs["sub_1"].Plan.ID; got != "low" {
		t.Errorf("plan after a declined difference = %s, want low", got)
	}
}

// Two partial refunds of a charge are two movements of what each added.
func TestRefundsAreTheirDifferences(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addPlan("p", 1000, "month", nil)
	f.addSub(subscription{ID: "sub_1", Customer: "cus_1", Plan: f.plans["p"].plan, Status: subActive}, unitNow.Unix())
	at := unitNow.Add(-3 * time.Hour)
	f.addCharge(charge{ID: "ch_1", Amount: 1000, Captured: true, CapturedAt: at.Unix(), Created: at.Unix(), FeeRate: "3.60", Subscription: "sub_1"}, "evnt_c")
	f.refund("ch_1", 300, at.Add(time.Hour).Unix(), "evnt_r1")
	f.refund("ch_1", 200, at.Add(2*time.Hour).Unix(), "evnt_r2")
	page, err := p.ListMoney(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range page.Movements {
		got = append(got, m.ID+":"+m.Gross+"/"+m.Fee+"/"+m.ParentGross)
	}
	if want := "ch_1:1000/36/,evnt_r1:-300/-10/1000,evnt_r2:-200/-7/1000"; strings.Join(got, ",") != want {
		t.Errorf("movements = %s, want %s", strings.Join(got, ","), want)
	}
}

// A sales statement whose fees differ from the adapter's is a fee of the difference.
func TestStatementAdjustsTheFees(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	start, end := unitNow.Add(-48*time.Hour), unitNow.Add(-time.Hour)
	for i, amount := range []int64{333, 333, 333} {
		at := start.Add(time.Duration(i+1) * time.Hour)
		f.addCharge(charge{ID: "ch_" + yen(int64(i)), Amount: amount, Captured: true, CapturedAt: at.Unix(), Created: at.Unix(), FeeRate: "3.60", TermID: "tm_1"}, "")
	}
	// 3 × floor(333 × 3.6%) = 33; on the total, 999 × 3.6% = 35.96, which PAY.JP charges as 36.
	st := statement{
		ID: "st_1", Created: end.Unix(), Type: "sales", Term: &term{ID: "tm_1", StartAt: start.Unix(), EndAt: end.Unix()},
		Items: []statementItem{{Subject: "gross_sales", Amount: 999}, {Subject: "fee", Amount: -36}},
	}
	f.statements["st_1"] = st
	f.addEvent("evnt_st", "statement.created", end.Unix(), "st_1", "statement", st)
	m, err := p.GetMoney(t.Context(), "st_1")
	if err != nil || m.Kind != supporters.MoneyFee || m.Gross != "-3" || m.Net != "-3" || m.Subscription != "" {
		t.Errorf("GetMoney(statement) = %+v, %v, want a fee of -3", m, err)
	}
	f.statements["st_1"] = statement{ID: "st_1", Created: end.Unix(), Type: "sales", Term: st.Term, Items: []statementItem{{Subject: "fee", Amount: -33}}}
	if _, err := p.GetMoney(t.Context(), "st_1"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("GetMoney(statement without a difference) = %v, want not_found", err)
	}
}

func TestParseWebhookKinds(t *testing.T) {
	p, _ := newOnFake(t, supporters.ModeTest, unitNow)
	h := http.Header{}
	h.Set(TokenHeader, testToken)
	for _, tc := range []struct {
		typ    string
		data   map[string]any
		kind   supporters.NoticeKind
		object string
	}{
		{"subscription.renewed", map[string]any{"id": "sub_1", "object": "subscription"}, supporters.NoticeSubscription, "sub_1"},
		{"charge.failed", map[string]any{"id": "ch_1", "object": "charge", "subscription": "sub_1"}, supporters.NoticeSubscription, "sub_1"},
		{"charge.failed", map[string]any{"id": "ch_1", "object": "charge"}, supporters.NoticeIgnored, "ch_1"},
		{"charge.succeeded", map[string]any{"id": "ch_1", "object": "charge", "subscription": "sub_1"}, supporters.NoticeMoney, "ch_1"},
		{"charge.refunded", map[string]any{"id": "ch_1", "object": "charge"}, supporters.NoticeMoney, "evnt_1"},
		{"dispute.created", map[string]any{"id": "dp_1", "object": "dispute"}, supporters.NoticeMoney, "evnt_1"},
		{"statement.created", map[string]any{"id": "st_1", "object": "statement"}, supporters.NoticeMoney, "evnt_1"},
		{"customer.card.created", map[string]any{"id": "car_1", "object": "card"}, supporters.NoticeIgnored, "car_1"},
	} {
		body, _ := json.Marshal(map[string]any{"id": "evnt_1", "type": tc.typ, "livemode": false, "created": unitNow.Unix(), "data": tc.data})
		n, err := p.ParseWebhook(h, body, unitNow)
		if err != nil || n.Kind != tc.kind || n.ObjectID != tc.object {
			t.Errorf("ParseWebhook(%s %v) = %+v, %v, want %s of %s", tc.typ, tc.data, n, err, tc.kind, tc.object)
		}
	}
	if _, err := p.ParseWebhook(h, []byte("not json"), unitNow); supporters.ErrorKindOf(err) != supporters.ErrorInvalidResponse {
		t.Errorf("ParseWebhook(not an event) = %v, want invalid_response", err)
	}
}

func TestPageState(t *testing.T) {
	p, _ := newOnFake(t, supporters.ModeTest, unitNow)
	st := pageState{Page: pageCheckout, LocalID: "local-1", Customer: "cus_1", Return: "https://my.example/done", Exp: unitNow.Add(time.Hour).Unix()}
	s := p.sign(st)
	if got, err := p.verify(s, pageCheckout, unitNow); err != nil || got != st {
		t.Errorf("verify = %+v, %v, want %+v", got, err, st)
	}
	for name, tc := range map[string]struct {
		s, page string
		at      time.Time
	}{
		"expired":    {s, pageCheckout, unitNow.Add(2 * time.Hour)},
		"other page": {s, pagePortal, unitNow},
		"altered":    {strings.Replace(s, s[:4], "eyJw", 1) + "x", pageCheckout, unitNow},
		"no mac":     {strings.Split(s, ".")[0], pageCheckout, unitNow},
	} {
		if _, err := p.verify(tc.s, tc.page, tc.at); err == nil {
			t.Errorf("verify(%s) passed", name)
		}
	}
	other, _ := New(Options{SecretKey: "sk_test_other", PublicKey: "pk_test_other", WebhookToken: testToken})
	if _, err := other.verify(s, pageCheckout, unitNow); err == nil {
		t.Error("verify with another secret key passed")
	}
}

func TestStartCheckoutNeedsOneOrigin(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addPlan("p", 500, "month", nil)
	req := supporters.CheckoutRequest{
		LocalID: "local-1", Price: "p", SuccessURL: "https://my.example/done", CancelURL: "https://evil.example/", Key: "k",
	}
	if _, err := p.StartCheckout(t.Context(), req); supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
		t.Errorf("StartCheckout with return URLs on two origins = %v, want invalid_request", err)
	}
	req.CancelURL = "https://my.example/"
	c, err := p.StartCheckout(t.Context(), req)
	if err != nil || !strings.HasPrefix(c.RedirectURL, "https://my.example/payments/pages/payjp/checkout?s=") {
		t.Errorf("StartCheckout = %+v, %v, want a page on the success URL's origin", c, err)
	}
	st, err := p.verify(query(t, c.RedirectURL).Get("s"), pageCheckout, unitNow)
	if err != nil || st.Customer != "my_local-1" || !st.NewCustomer || st.Plan != "p" {
		t.Errorf("state = %+v, %v, want a new customer of our ID on plan p", st, err)
	}
}
