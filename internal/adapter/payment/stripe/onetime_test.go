// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// tipRequest is a request of a one-time checkout of 500 JPY.
func tipRequest() supporters.OneTimeRequest {
	return supporters.OneTimeRequest{
		LocalID: "01890a5d-ac96-7000-8000-0000000000a1", AmountMinor: 500, Currency: "JPY", Description: "Tip for my.example",
		Email: "alice@example.com", Locale: "ja-JP", SuccessURL: "https://my.example/settings/supporter/tip?tip=1",
		CancelURL: "https://my.example/settings/supporter/tip", Key: "tip:01890a5d-ac96-7000-8000-0000000000a1",
	}
}

// A paid tip reports its PaymentIntent and what it charged; its money names the PaymentIntent and
// no subscription; and its notice resolves to the one-time checkout, while a subscription's
// checkout still resolves to Change.Checkout.
func TestOneTimeCheckoutPaid(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	ctx := t.Context()
	req := tipRequest()
	c, err := p.StartOneTimeCheckout(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	sess := f.checkouts[string(c.Ref)]
	f.mu.Unlock()
	if sess["locale"] != "ja" || sess["customer_email"] != req.Email || sess["success_url"] != req.SuccessURL+"&checkout_id={CHECKOUT_SESSION_ID}" {
		t.Errorf("session = %v, want the locale ja, the email, and the success URL with the checkout's ID", sess)
	}
	pi := f.payOneTime(t, string(c.Ref))
	want := supporters.OneTimeResult{Ref: c.Ref, Status: supporters.OneTimePaid, LocalID: req.LocalID, Payment: supporters.PaymentRef(pi), AmountMinor: 500, Currency: "JPY"}
	if got, err := p.GetOneTimeCheckout(ctx, c.Ref); err != nil || got != want {
		t.Errorf("GetOneTimeCheckout = %+v, %v, want %+v", got, err, want)
	}
	page, err := p.ListMoney(ctx, time.Time{}, "")
	if err != nil || len(page.Movements) != 1 {
		t.Fatalf("ListMoney = %+v, %v, want the tip's payment", page, err)
	}
	if m := page.Movements[0]; m.Kind != supporters.MoneyPayment || m.Payment != supporters.PaymentRef(pi) || m.Subscription != "" || !m.Covers.IsZero() || m.Gross != "500" {
		t.Errorf("the tip's movement = %+v, want a payment of 500 JPY by %s without a subscription or a period", m, pi)
	}
	ch, err := p.Resolve(ctx, supporters.Notice{EventID: "evt_1", Type: "checkout.session.completed", Kind: supporters.NoticeCheckout, ObjectID: string(c.Ref)})
	if err != nil || ch.OneTimeCheckout != c.Ref || ch.Checkout != "" {
		t.Errorf("Resolve(the tip's checkout) = %+v, %v, want the one-time checkout alone", ch, err)
	}

	sp, _, _, sc := oneCheckout(t)
	ch, err = sp.Resolve(ctx, supporters.Notice{EventID: "evt_2", Type: "checkout.session.completed", Kind: supporters.NoticeCheckout, ObjectID: string(sc.Ref)})
	if err != nil || ch.Checkout != sc.Ref || ch.OneTimeCheckout != "" {
		t.Errorf("Resolve(a subscription's checkout) = %+v, %v, want Change.Checkout alone", ch, err)
	}
	if _, err := sp.GetOneTimeCheckout(ctx, sc.Ref); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("GetOneTimeCheckout(a subscription's checkout) = %v, want not_found", err)
	}
}

// A request that Stripe would refuse is refused before it is sent.
func TestStartOneTimeCheckoutRefusesBadRequests(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	for name, change := range map[string]func(*supporters.OneTimeRequest){
		"no ID":                 func(r *supporters.OneTimeRequest) { r.LocalID = "" },
		"no description":        func(r *supporters.OneTimeRequest) { r.Description = "" },
		"no cancel URL":         func(r *supporters.OneTimeRequest) { r.CancelURL = "" },
		"a relative return":     func(r *supporters.OneTimeRequest) { r.SuccessURL = "/settings" },
		"an unknown currency":   func(r *supporters.OneTimeRequest) { r.Currency = "XXX" },
		"below the minimum":     func(r *supporters.OneTimeRequest) { r.AmountMinor = 49 },
		"above the maximum":     func(r *supporters.OneTimeRequest) { r.AmountMinor = maxOneTimeAmount + 1 },
		"a negative amount":     func(r *supporters.OneTimeRequest) { r.AmountMinor = -500 },
		"a USD amount too low":  func(r *supporters.OneTimeRequest) { r.Currency, r.AmountMinor = "USD", 49 },
		"a lowercase too small": func(r *supporters.OneTimeRequest) { r.Currency, r.AmountMinor = "jpy", 10 },
	} {
		req := tipRequest()
		change(&req)
		if _, err := p.StartOneTimeCheckout(t.Context(), req); supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
			t.Errorf("StartOneTimeCheckout with %s = %v, want invalid_request", name, err)
		}
	}
	if n := f.countRequests("POST /v1/checkout"); n != 0 {
		t.Errorf("%d checkouts were asked of Stripe, want none", n)
	}
	if lo, hi, ok := p.OneTimeLimits("usd"); !ok || lo != 50 || hi != maxOneTimeAmount {
		t.Errorf("OneTimeLimits(usd) = %d, %d, %v, want 50 to %d", lo, hi, ok, maxOneTimeAmount)
	}
}

// A one-time checkout URL off Stripe's host is refused.
func TestOneTimeCheckoutURLHost(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	f.checkoutURL = "https://checkout.stripe.com.evil.example/c/pay/x"
	if _, err := p.StartOneTimeCheckout(t.Context(), tipRequest()); !errors.Is(err, supporters.ErrInvalidPaymentResponse) {
		t.Errorf("StartOneTimeCheckout with a URL elsewhere = %v, want ErrInvalidPaymentResponse", err)
	}
}

// Expiring a tip's page that was paid in the meantime reports the payment; a refusal for a session
// that stays open is an error, so that the caller does not open another page.
func TestExpireOneTimeCheckout(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	ctx := t.Context()
	c, err := p.StartOneTimeCheckout(ctx, tipRequest())
	if err != nil {
		t.Fatal(err)
	}
	pi := f.payOneTime(t, string(c.Ref))
	if res, err := p.ExpireOneTimeCheckout(ctx, c.Ref); err != nil || res.Status != supporters.OneTimePaid || res.Payment != supporters.PaymentRef(pi) {
		t.Errorf("ExpireOneTimeCheckout of a paid page = %+v, %v, want paid by %s", res, err, pi)
	}
	if _, err := p.ExpireOneTimeCheckout(ctx, "cs_unknown"); supporters.ErrorKindOf(err) != supporters.ErrorNotFound {
		t.Errorf("ExpireOneTimeCheckout of an unknown session = %v, want not_found", err)
	}

	req := tipRequest()
	req.Key += ":again"
	open, err := p.StartOneTimeCheckout(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.before = func(r *http.Request) int {
		if strings.HasSuffix(r.URL.Path, "/expire") {
			return http.StatusBadRequest
		}
		return 0
	}
	f.mu.Unlock()
	if res, err := p.ExpireOneTimeCheckout(ctx, open.Ref); supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
		t.Errorf("ExpireOneTimeCheckout refused while open = %+v, %v, want invalid_request", res, err)
	}
}

// A session that completed unpaid, which a card does not leave, stays open rather than paid.
func TestOneTimeCheckoutCompletedUnpaid(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	c, err := p.StartOneTimeCheckout(t.Context(), tipRequest())
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.checkouts[string(c.Ref)]["status"] = "complete"
	f.mu.Unlock()
	if res, err := p.GetOneTimeCheckout(t.Context(), c.Ref); err != nil || res.Status != supporters.OneTimeOpen || res.Payment != "" {
		t.Errorf("GetOneTimeCheckout of a session completed unpaid = %+v, %v, want open", res, err)
	}
}

// recordedTipSession is a Checkout Session in payment mode as Stripe's API reference shows one
// (https://docs.stripe.com/api/checkout/sessions/object), complete and paid, with the fields that
// the adapter does not read.
const recordedTipSession = `{
  "id": "cs_test_a1YS1URlnyQCN5fUUduORoQ7Pw41PJqDWkIVQCpJPqkfIhd6tVY8XB1OLY",
  "object": "checkout.session",
  "adaptive_pricing": {"enabled": false},
  "after_expiration": null,
  "allow_promotion_codes": null,
  "amount_subtotal": 1000,
  "amount_total": 1000,
  "automatic_tax": {"enabled": false, "liability": null, "status": null},
  "billing_address_collection": null,
  "cancel_url": "https://my.example/settings/supporter/tip",
  "client_reference_id": "01890a5d-ac96-7000-8000-0000000000a2",
  "client_secret": null,
  "consent": null,
  "created": 1792630000,
  "currency": "jpy",
  "custom_fields": [],
  "custom_text": {"after_submit": null, "shipping_address": null, "submit": null, "terms_of_service_acceptance": null},
  "customer": null,
  "customer_creation": "if_required",
  "customer_details": {"address": null, "email": "alice@example.com", "name": null, "phone": null, "tax_exempt": "none", "tax_ids": []},
  "customer_email": "alice@example.com",
  "expires_at": 1792633600,
  "invoice": null,
  "invoice_creation": {"enabled": false, "invoice_data": {"account_tax_ids": null, "custom_fields": null, "description": null, "footer": null, "issuer": null, "metadata": {}, "rendering_options": null}},
  "livemode": false,
  "locale": "ja",
  "metadata": {"my_tip_id": "01890a5d-ac96-7000-8000-0000000000a2"},
  "mode": "payment",
  "payment_intent": "pi_3MtwBwLkdIwHu7ix28a3tqPa",
  "payment_link": null,
  "payment_method_collection": "if_required",
  "payment_method_options": {},
  "payment_method_types": ["card"],
  "payment_status": "paid",
  "phone_number_collection": {"enabled": false},
  "recovered_from": null,
  "setup_intent": null,
  "shipping_address_collection": null,
  "shipping_cost": null,
  "shipping_details": null,
  "shipping_options": [],
  "status": "complete",
  "submit_type": null,
  "subscription": null,
  "success_url": "https://my.example/settings/supporter/tip?checkout_id={CHECKOUT_SESSION_ID}",
  "total_details": {"amount_discount": 0, "amount_shipping": 0, "amount_tax": 0},
  "ui_mode": "hosted",
  "url": null
}`

// The adapter reads a session in the shape of Stripe's reference.
func TestOneTimeCheckoutOfRecordedSession(t *testing.T) {
	p, f, _ := newOnFake(t, scenario())
	var o obj
	if err := json.Unmarshal([]byte(recordedTipSession), &o); err != nil {
		t.Fatal(err)
	}
	ref := supporters.CheckoutRef(o["id"].(string))
	f.mu.Lock()
	f.checkouts[string(ref)] = o
	f.mu.Unlock()
	want := supporters.OneTimeResult{
		Ref: ref, Status: supporters.OneTimePaid, LocalID: "01890a5d-ac96-7000-8000-0000000000a2",
		Payment: "pi_3MtwBwLkdIwHu7ix28a3tqPa", AmountMinor: 1000, Currency: "JPY",
	}
	if got, err := p.GetOneTimeCheckout(t.Context(), ref); err != nil || got != want {
		t.Errorf("GetOneTimeCheckout = %+v, %v, want %+v", got, err, want)
	}
}

// The webhook of a tip's checkout is verified and read like any other: a checkout's notice, by
// the fields that no API version moves.
func TestParseWebhookOfOneTimeCheckout(t *testing.T) {
	p, _, _ := newOnFake(t, scenario())
	now := time.Unix(1792630800, 0)
	for _, typ := range []string{"checkout.session.completed", "checkout.session.expired"} {
		body := []byte(`{"id":"evt_1Tip","object":"event","api_version":"2026-09-30.endive","type":"` + typ + `","livemode":false,` +
			`"created":1792630800,"pending_webhooks":1,"request":{"id":null,"idempotency_key":null},"data":{"object":` + recordedTipSession + `}}`)
		n, err := p.ParseWebhook(signedHeader(body, now, testSigningKey), body, now)
		if err != nil || n.Kind != supporters.NoticeCheckout || n.ObjectID != "cs_test_a1YS1URlnyQCN5fUUduORoQ7Pw41PJqDWkIVQCpJPqkfIhd6tVY8XB1OLY" || n.Type != typ {
			t.Errorf("ParseWebhook(%s) = %+v, %v", typ, n, err)
		}
	}
	// No event type is added for the tips: the checkout's and the money's cover them.
	for _, typ := range []string{"checkout.session.async_payment_succeeded", "payment_intent.succeeded"} {
		if slices.Contains(EventTypes(), typ) {
			t.Errorf("EventTypes has %s", typ)
		}
	}
}
