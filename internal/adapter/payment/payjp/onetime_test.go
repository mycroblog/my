// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

func tipRequest() supporters.OneTimeRequest {
	return supporters.OneTimeRequest{
		LocalID: "01890a5d-ac96-7000-8000-000000000009", AmountMinor: 500, Currency: "JPY", Description: "Tip for my.example",
		Email: "alice@example.com", Locale: "ja-JP", SuccessURL: "https://my.example/settings/supporter/tip/1",
		CancelURL: "https://my.example/settings/supporter/tip", Key: "tip:1",
	}
}

// The session of a tip is one line of the amount, by card, for an hour, with our ID on the session
// and its PaymentFlow, and the same key returns it again without a second session.
func TestStartOneTimeCheckoutRequest(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	c, err := p.StartOneTimeCheckout(t.Context(), tipRequest())
	if err != nil {
		t.Fatal(err)
	}
	again, err := p.StartOneTimeCheckout(t.Context(), tipRequest())
	if err != nil || again != c {
		t.Errorf("StartOneTimeCheckout again = %+v, %v, want %+v", again, err, c)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.v2.sessions) != 1 || f.v2.requests["POST /checkout/sessions"] != 2 {
		t.Errorf("sessions = %d after %d requests, want 1 after 2", len(f.v2.sessions), f.v2.requests["POST /checkout/sessions"])
	}
	body := f.v2.sessionBodies[string(c.Ref)]
	b, _ := json.Marshal(body)
	var got struct {
		Mode      string `json:"mode"`
		LineItems []struct {
			Quantity  int `json:"quantity"`
			PriceData struct {
				Currency    string `json:"currency"`
				UnitAmount  int64  `json:"unit_amount"`
				ProductData struct {
					Name string `json:"name"`
				} `json:"product_data"`
			} `json:"price_data"`
		} `json:"line_items"`
		ClientReferenceID string            `json:"client_reference_id"`
		Metadata          map[string]string `json:"metadata"`
		PaymentFlowData   struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"payment_flow_data"`
		Methods   []string  `json:"payment_method_types"`
		Email     string    `json:"customer_email"`
		Locale    string    `json:"locale"`
		ExpiresAt time.Time `json:"expires_at"`
		Success   string    `json:"success_url"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	id := tipRequest().LocalID
	if got.Mode != "payment" || len(got.LineItems) != 1 || got.LineItems[0].Quantity != 1 || got.LineItems[0].PriceData.UnitAmount != 500 ||
		got.LineItems[0].PriceData.Currency != "jpy" || got.LineItems[0].PriceData.ProductData.Name != "Tip for my.example" ||
		got.ClientReferenceID != id || got.Metadata[metaTipID] != id || got.PaymentFlowData.Metadata[metaTipID] != id ||
		!slices.Equal(got.Methods, []string{"card"}) || got.Email != "alice@example.com" || got.Locale != "ja" ||
		!got.ExpiresAt.Equal(unitNow.Add(time.Hour)) || got.Success != tipRequest().SuccessURL {
		t.Errorf("the session's creation = %s", b)
	}
	if !c.ExpiresAt.Equal(unitNow.Add(time.Hour)) {
		t.Errorf("ExpiresAt = %s, want an hour from now", c.ExpiresAt)
	}
}

// A session whose page is not on PAY.JP is refused.
func TestStartOneTimeCheckoutChecksTheHost(t *testing.T) {
	for _, u := range []string{"https://pay.jp.evil.example/c/1", "http://checkout.pay.jp/c/1", "https://user@checkout.pay.jp/c/1"} {
		if hostedURL(u) {
			t.Errorf("hostedURL(%s) = true", u)
		}
	}
	for _, u := range []string{"https://checkout.pay.jp/c/1", "https://pay.jp/c/1"} {
		if !hostedURL(u) {
			t.Errorf("hostedURL(%s) = false", u)
		}
	}
}

// A completed session whose PaymentFlow has not succeeded is open; once it has, it is paid.
func TestOneTimeCheckoutPaidOnlyWhenTheFlowSucceeded(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addFlow(paymentFlow{ID: "pfw_1", Amount: 500, Currency: currency, Status: "processing"})
	f.addSession(checkoutSession{ID: "cs_1", Status: sessionComplete, PaymentFlowID: "pfw_1", Metadata: metadata{metaTipID: "tip"}})
	if got, err := p.GetOneTimeCheckout(t.Context(), "cs_1"); err != nil || got.Status != supporters.OneTimeOpen || got.Payment != "" {
		t.Errorf("GetOneTimeCheckout(processing) = %+v, %v, want open", got, err)
	}
	f.mu.Lock()
	f.v2.flows["pfw_1"].Status, f.v2.flows["pfw_1"].AmountReceived = flowSucceeded, 500
	f.mu.Unlock()
	want := supporters.OneTimeResult{Ref: "cs_1", Status: supporters.OneTimePaid, LocalID: "tip", Payment: "pfw_1", AmountMinor: 500, Currency: "JPY"}
	if got, err := p.GetOneTimeCheckout(t.Context(), "cs_1"); err != nil || got != want {
		t.Errorf("GetOneTimeCheckout(succeeded) = %+v, %v, want %+v", got, err, want)
	}
}

// An expiry that PAY.JP refuses because the session completed meanwhile reports the payment.
func TestExpireOneTimeCheckoutAfterThePayment(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addFlow(paymentFlow{ID: "pfw_1", Amount: 500, AmountReceived: 500, Currency: currency, Status: flowSucceeded})
	f.addSession(checkoutSession{ID: "cs_1", Status: sessionOpen, Metadata: metadata{metaTipID: "tip"}})
	// The customer pays between the adapter's read and its expiry.
	p.o.HTTPClient.Transport = roundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			f.mu.Lock()
			f.v2.sessions["cs_1"].Status, f.v2.sessions["cs_1"].PaymentFlowID = sessionComplete, "pfw_1"
			f.mu.Unlock()
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	got, err := p.ExpireOneTimeCheckout(t.Context(), "cs_1")
	if err != nil || got.Status != supporters.OneTimePaid || got.Payment != "pfw_1" {
		t.Errorf("ExpireOneTimeCheckout = %+v, %v, want paid", got, err)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A refund carries its key, and a repeat with the key refunds nothing twice.
func TestRefundPaymentIsIdempotent(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addFlow(paymentFlow{ID: "pfw_1", Amount: 500, AmountReceived: 500, Currency: currency, Status: flowSucceeded})
	var keys []string
	p.o.HTTPClient.Transport = roundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			keys = append(keys, r.Header.Get("Idempotency-Key"))
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	for range 2 {
		if err := p.RefundPayment(t.Context(), "pfw_1", "refund:tip"); err != nil {
			t.Fatal(err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.v2.refundsCreated != 1 || !slices.Equal(keys, []string{"refund:tip", "refund:tip"}) {
		t.Errorf("refunds = %d with keys %q, want 1 with the key twice", f.v2.refundsCreated, keys)
	}
	if err := p.RefundPayment(t.Context(), "pfw_1", ""); !errors.Is(err, supporters.ErrPaymentInvalidRequest) {
		t.Errorf("RefundPayment without a key = %v, want invalid_request", err)
	}
}

// The webhook tells the events of API v1 and v2 apart, and maps those of v2.
func TestParseWebhookV2Kinds(t *testing.T) {
	p, _ := newOnFake(t, supporters.ModeTest, unitNow)
	h := http.Header{}
	h.Set(TokenHeader, testToken)
	for _, tc := range []struct {
		typ    string
		data   map[string]any
		kind   supporters.NoticeKind
		object string
	}{
		{"checkout.session.completed", map[string]any{"id": "cs_1", "object": "checkout.session"}, supporters.NoticeCheckout, "cs_1"},
		{"checkout.session.expired", map[string]any{"id": "cs_1", "object": "checkout.session"}, supporters.NoticeCheckout, "cs_1"},
		{"payment_flow.succeeded", map[string]any{"id": "pfw_1", "object": "payment_flow"}, supporters.NoticeMoney, "pfw_1"},
		{"payment_flow.payment_failed", map[string]any{"id": "pfw_1", "object": "payment_flow"}, supporters.NoticeIgnored, "pfw_1"},
		{"refund.created", map[string]any{"id": "pr_1", "object": "payment_refund"}, supporters.NoticeMoney, "pr_1"},
		{"refund.updated", map[string]any{"id": "pr_1", "object": "payment_refund"}, supporters.NoticeMoney, "pr_1"},
		{"dispute.created", map[string]any{"id": "pd_1", "object": "payment_dispute"}, supporters.NoticeMoney, "pd_1"},
		{"customer.created", map[string]any{"id": "cus_1", "object": "customer"}, supporters.NoticeIgnored, "cus_1"},
	} {
		body, _ := json.Marshal(map[string]any{"id": "evt_1", "object": "event", "type": tc.typ, "livemode": false, "created_at": unitNow.Format(time.RFC3339), "data": tc.data})
		n, err := p.ParseWebhook(h, body, unitNow)
		if err != nil || n.Kind != tc.kind || n.ObjectID != tc.object || !n.OccurredAt.Equal(unitNow) {
			t.Errorf("ParseWebhook(v2 %s) = %+v, %v, want %s of %s", tc.typ, n, err, tc.kind, tc.object)
		}
	}
	live, _ := json.Marshal(map[string]any{"id": "evt_1", "type": "dispute.created", "livemode": true, "created_at": unitNow.Format(time.RFC3339),
		"data": map[string]any{"id": "pd_1", "object": "payment_dispute"}})
	if _, err := p.ParseWebhook(h, live, unitNow); supporters.ErrorKindOf(err) != supporters.ErrorModeMismatch {
		t.Errorf("ParseWebhook(live v2) = %v, want mode_mismatch", err)
	}
	bad := http.Header{}
	bad.Set(TokenHeader, "whook_other")
	if _, err := p.ParseWebhook(bad, live, unitNow); supporters.ErrorKindOf(err) != supporters.ErrorInvalidSignature {
		t.Errorf("ParseWebhook(v2, another token) = %v, want invalid_signature", err)
	}
}

// A notice of a tip's money resolves to its movement each time it comes, and a second delivery
// changes nothing.
func TestResolveV2MoneyAgain(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	at := unitNow.Add(-time.Hour)
	f.addFlow(paymentFlow{ID: "pfw_1", Amount: 1000, AmountReceived: 1000, Currency: currency, Status: flowSucceeded})
	f.addTxn(paymentTransaction{ID: "pt_1", ResourceID: "pfw_1", Amount: 1000, Fee: 36, Type: txnPayment, CreatedAt: at})
	f.addRefund(paymentRefund{ID: "pr_1", PaymentFlowID: "pfw_1", Amount: 400})
	f.addTxn(paymentTransaction{ID: "pt_2", ResourceID: "pr_1", Amount: -400, Fee: -14, Type: txnRefund, CreatedAt: at.Add(time.Minute)})
	for _, n := range []supporters.Notice{
		{EventID: "evt_1", Type: "payment_flow.succeeded", Kind: supporters.NoticeMoney, ObjectID: "pfw_1"},
		{EventID: "evt_2", Type: "refund.created", Kind: supporters.NoticeMoney, ObjectID: "pr_1"},
	} {
		first, err := p.Resolve(t.Context(), n)
		if err != nil || len(first.Money) != 1 || first.Money[0].ID != n.ObjectID || first.Money[0].Payment != "pfw_1" {
			t.Fatalf("Resolve(%s) = %+v, %v, want the movement of %s", n.Type, first, err, n.ObjectID)
		}
		again, err := p.Resolve(t.Context(), n)
		if err != nil || !slices.Equal(again.Money, first.Money) {
			t.Errorf("Resolve(%s) again = %+v, %v, want %+v", n.Type, again, err, first)
		}
	}
	m, err := p.GetMoney(t.Context(), "pr_1")
	if err != nil || m.Kind != supporters.MoneyRefund || m.Gross != "-400" || m.Fee != "-14" || m.ParentID != "pfw_1" || m.ParentGross != "1000" {
		t.Errorf("GetMoney(refund) = %+v, %v", m, err)
	}
	if ch, err := p.Resolve(t.Context(), supporters.Notice{EventID: "evt_3", Type: "checkout.session.completed", Kind: supporters.NoticeCheckout, ObjectID: "cs_none"}); err != nil || ch.Checkout != "cs_none" {
		t.Errorf("Resolve(unknown checkout) = %+v, %v, want the subscription's checkout", ch, err)
	}
}

// listAll pages through ListMoney from cursor and returns the movements and the last cursor.
func listAll(t *testing.T, p *Provider, cursor string) ([]supporters.MoneyMovement, string) {
	t.Helper()
	var out []supporters.MoneyMovement
	for range 100 {
		page, err := p.ListMoney(t.Context(), time.Time{}, cursor)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, page.Movements...)
		cursor = page.Next
		if !page.More {
			return out, cursor
		}
	}
	t.Fatal("ListMoney never ends")
	return nil, ""
}

// The reconciliation reads the disputes: one that ended in our favour after a chargeback, which
// the ledger does not report as cancelled, is a reversal once; an inquiry that PAY.JP closed
// without a chargeback moves no money.
func TestDisputeOutcomesFromTheDisputes(t *testing.T) {
	now := unitNow
	var clock atomic.Int64
	clock.Store(now.Unix())
	p, f := newOnFakeClock(t, supporters.ModeTest, func() time.Time { return time.Unix(clock.Load(), 0).UTC() })
	at := now.Add(-48 * time.Hour)
	for _, id := range []string{"pfw_1", "pfw_2"} {
		f.addFlow(paymentFlow{ID: id, Amount: 3000, AmountReceived: 3000, Currency: currency, Status: flowSucceeded})
		f.addTxn(paymentTransaction{ID: "pt_" + id, ResourceID: id, Amount: 3000, Fee: 108, Type: txnPayment, CreatedAt: at})
	}
	f.addDispute(paymentDispute{ID: "pd_1", PaymentFlowID: "pfw_1", Amount: 3000, Status: "needs_response", CreatedAt: at.Add(time.Hour), UpdatedAt: at.Add(time.Hour)})
	f.addTxn(paymentTransaction{ID: "pt_cb", ResourceID: "pd_1", Amount: -3000, Fee: -108, Type: txnChargeback, CreatedAt: at.Add(time.Hour)})
	f.addDispute(paymentDispute{ID: "pd_2", PaymentFlowID: "pfw_2", Amount: 3000, Status: disputeCanceled, CreatedAt: at.Add(time.Hour), UpdatedAt: at.Add(2 * time.Hour)})
	ms, cursor := listAll(t, p, "")
	kinds := func(ms []supporters.MoneyMovement) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.ID+":"+string(m.Kind))
		}
		return out
	}
	if want := []string{"pfw_1:payment", "pfw_2:payment", "pd_1:chargeback"}; !slices.Equal(kinds(ms), want) {
		t.Fatalf("ListMoney = %v, want %v", kinds(ms), want)
	}
	// An hour later, PAY.JP decides for us: the dispute's state changes, and no cancellation is in
	// the ledger.
	decided := now.Add(30 * time.Minute)
	clock.Store(now.Add(time.Hour).Unix())
	f.mu.Lock()
	f.v2.disputes["pd_1"].Status, f.v2.disputes["pd_1"].UpdatedAt = disputeCanceled, decided
	f.mu.Unlock()
	ms, cursor = listAll(t, p, cursor)
	want := newMovement("pd_1"+cancelSuffix, supporters.MoneyChargebackReversal, decided, 3000, 108)
	want.ParentID, want.ParentGross, want.Payment = "pfw_1", "3000", "pfw_1"
	if len(ms) != 1 || ms[0] != want {
		t.Fatalf("ListMoney after the dispute = %+v, want %+v", ms, want)
	}
	if ms, _ := listAll(t, p, cursor); len(ms) != 0 {
		t.Errorf("ListMoney again = %v, want nothing", kinds(ms))
	}
}

// A cursor of API v1 alone, from before the tips, reads the transactions of v2 from the start.
func TestMoneyCursorOfV1Alone(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addFlow(paymentFlow{ID: "pfw_1", Amount: 500, AmountReceived: 500, Currency: currency, Status: flowSucceeded})
	f.addTxn(paymentTransaction{ID: "pt_1", ResourceID: "pfw_1", Amount: 500, Fee: 18, Type: txnPayment, CreatedAt: unitNow.Add(-time.Hour)})
	ms, _ := listAll(t, p, "1792630800")
	if len(ms) != 1 || ms[0].ID != "pfw_1" {
		t.Errorf("ListMoney from a cursor of v1 = %+v, want the payment of v2", ms)
	}
}

// The fees of a term's transactions of API v2 are among the fees that its statement is compared
// with.
func TestStatementCountsTheFeesOfV2(t *testing.T) {
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	start, end := unitNow.Add(-48*time.Hour), unitNow.Add(-time.Hour)
	f.addCharge(charge{ID: "ch_1", Amount: 1000, Captured: true, CapturedAt: start.Unix() + 60, Created: start.Unix() + 60, FeeRate: "3.60", TermID: "tm_1"}, "")
	f.addTxn(paymentTransaction{ID: "pt_1", ResourceID: "pfw_1", Amount: 500, Fee: 18, Type: txnPayment, TermID: "tm_1", CreatedAt: start.Add(time.Hour)})
	f.addTxn(paymentTransaction{ID: "pt_2", ResourceID: "pfw_2", Amount: 500, Fee: 18, Type: txnPayment, TermID: "tm_2", CreatedAt: start.Add(time.Hour)})
	f.statements["st_1"] = statement{
		ID: "st_1", Created: end.Unix(), Type: "sales", Term: &term{ID: "tm_1", StartAt: start.Unix(), EndAt: end.Unix()},
		Items: []statementItem{{Subject: "fee", Amount: -36 - 18 - 2}},
	}
	m, err := p.GetMoney(t.Context(), "st_1")
	if err != nil || m.Kind != supporters.MoneyFee || m.Gross != "-2" {
		t.Errorf("GetMoney(statement) = %+v, %v, want a fee of -2", m, err)
	}
}
