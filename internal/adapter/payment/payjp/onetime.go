// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The one-time payments of PAY.JP (supporters.OneTimePayments, ADR-0162 §11) on API v2: a
// hosted Checkout Session with mode=payment, whose PaymentFlow is the payment. The adapter's own
// pages of API v1 take no part in them.

var _ supporters.OneTimePayments = (*Provider)(nil)

// metaTipID is the metadata key of our tip's ID on a Checkout Session and its PaymentFlow.
const metaTipID = "my_tip_id"

// oneTimeLifetime is how long a one-time checkout stays payable (ADR-0162 §13). PAY.JP takes
// up to 24 hours.
const oneTimeLifetime = time.Hour

// OneTimeLimits implements supporters.OneTimePayments: 50 to 9,999,999 yen, by card; PAY.JP takes
// no other currency.
func (p *Provider) OneTimeLimits(c string) (int64, int64, bool) {
	if !strings.EqualFold(c, currency) {
		return 0, 0, false
	}
	return MinimumAmount, MaximumAmount, true
}

// StartOneTimeCheckout implements supporters.OneTimePayments: a Checkout Session of one line of
// the amount, by card, that expires in an hour, with our ID as its client_reference_id and in
// the metadata of the session and of its PaymentFlow. The request carries req.Key as its
// Idempotency-Key, so the same key returns the same session. SuccessURL is passed as it is: it
// names our tip already.
func (p *Provider) StartOneTimeCheckout(ctx context.Context, req supporters.OneTimeRequest) (supporters.Checkout, error) {
	minimum, maximum, ok := p.OneTimeLimits(req.Currency)
	if req.LocalID == "" || !ok || req.AmountMinor < minimum || req.AmountMinor > maximum {
		return supporters.Checkout{}, fmt.Errorf("payjp: %w: a one-time checkout needs our ID and %d to %d yen", supporters.ErrPaymentInvalidRequest, MinimumAmount, MaximumAmount)
	}
	product := map[string]any{"name": req.Description}
	body := map[string]any{
		"mode":     sessionModePayment,
		"currency": currency,
		"line_items": []map[string]any{{
			"quantity":   1,
			"price_data": map[string]any{"currency": currency, "unit_amount": req.AmountMinor, "product_data": product},
		}},
		"client_reference_id":  req.LocalID,
		"metadata":             map[string]string{metaTipID: req.LocalID},
		"payment_flow_data":    map[string]any{"metadata": map[string]string{metaTipID: req.LocalID}},
		"payment_method_types": []string{"card"},
		"success_url":          req.SuccessURL,
		"cancel_url":           req.CancelURL,
		"expires_at":           p.o.Now().Add(oneTimeLifetime).UTC().Truncate(time.Second),
	}
	if req.Email != "" {
		body["customer_email"] = req.Email
	}
	// The hosted page speaks Japanese only; another language leaves PAY.JP's default.
	if strings.HasPrefix(strings.ToLower(req.Locale), "ja") {
		body["locale"] = "ja"
	}
	var s checkoutSession
	if err := p.postV2(ctx, "create a one-time checkout", "/checkout/sessions", body, req.Key, &s); err != nil {
		return supporters.Checkout{}, err
	}
	if s.ID == "" || !hostedURL(s.URL) {
		return supporters.Checkout{}, fmt.Errorf("payjp: %w: the checkout's URL is not on PAY.JP", supporters.ErrInvalidPaymentResponse)
	}
	return supporters.Checkout{Ref: supporters.CheckoutRef(s.ID), RedirectURL: s.URL, ExpiresAt: s.ExpiresAt}, nil
}

// hostedURL reports whether u is a page of PAY.JP: https on pay.jp or one of its subdomains
// (ADR-0074 §7).
func hostedURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "pay.jp" || strings.HasSuffix(host, ".pay.jp")
}

// getSession reads a one-time checkout; a session of another mode is not one.
func (p *Provider) getSession(ctx context.Context, ref supporters.CheckoutRef) (checkoutSession, error) {
	var s checkoutSession
	if err := p.getV2(ctx, "read a one-time checkout", "/checkout/sessions/"+url.PathEscape(string(ref)), nil, &s); err != nil {
		return checkoutSession{}, err
	}
	if s.Mode != sessionModePayment {
		return checkoutSession{}, fmt.Errorf("payjp: %w: checkout %s is not of one payment", supporters.ErrPaymentNotFound, ref)
	}
	return s, nil
}

// GetOneTimeCheckout implements supporters.OneTimePayments.
func (p *Provider) GetOneTimeCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.OneTimeResult, error) {
	s, err := p.getSession(ctx, ref)
	if err != nil {
		return supporters.OneTimeResult{}, err
	}
	return p.resultOf(ctx, s)
}

// resultOf is what a session reports. A completed session is paid only once its PaymentFlow
// succeeded, since PAY.JP counts a payment as done when its result arrives; until then it is open.
func (p *Provider) resultOf(ctx context.Context, s checkoutSession) (supporters.OneTimeResult, error) {
	res := supporters.OneTimeResult{Ref: supporters.CheckoutRef(s.ID), Status: supporters.OneTimeOpen, LocalID: s.Metadata[metaTipID]}
	if res.LocalID == "" {
		res.LocalID = s.ClientReferenceID
	}
	switch s.Status {
	case sessionExpired:
		res.Status = supporters.OneTimeExpired
	case sessionComplete:
		if s.PaymentFlowID == "" {
			return res, nil
		}
		f, err := p.getFlow(ctx, s.PaymentFlowID)
		if err != nil {
			return supporters.OneTimeResult{}, err
		}
		if f.Status != flowSucceeded {
			return res, nil
		}
		res.Status, res.Payment = supporters.OneTimePaid, supporters.PaymentRef(f.ID)
		res.AmountMinor, res.Currency = f.received(), strings.ToUpper(f.Currency)
	}
	return res, nil
}

// received is the amount that a PaymentFlow took.
func (f paymentFlow) received() int64 {
	if f.AmountReceived > 0 {
		return f.AmountReceived
	}
	return f.Amount
}

func (p *Provider) getFlow(ctx context.Context, id string) (paymentFlow, error) {
	var f paymentFlow
	err := p.getV2(ctx, "read a payment flow", "/payment_flows/"+url.PathEscape(id), nil, &f)
	return f, err
}

// ExpireOneTimeCheckout implements supporters.OneTimePayments: an open session is expired, and
// one that is not open any more reports what it is. When PAY.JP refuses the expiry because the
// customer got there first, the session read again says so.
func (p *Provider) ExpireOneTimeCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.OneTimeResult, error) {
	s, err := p.getSession(ctx, ref)
	if err != nil {
		return supporters.OneTimeResult{}, err
	}
	if s.Status != sessionOpen {
		return p.resultOf(ctx, s)
	}
	var after checkoutSession
	path := "/checkout/sessions/" + url.PathEscape(string(ref)) + "/expire"
	if err := p.postV2(ctx, "expire a one-time checkout", path, struct{}{}, "expire:"+string(ref), &after); err != nil {
		if !errors.Is(err, supporters.ErrPaymentInvalidRequest) {
			return supporters.OneTimeResult{}, err
		}
		again, gerr := p.getSession(ctx, ref)
		if gerr != nil || again.Status == sessionOpen {
			return supporters.OneTimeResult{}, err
		}
		return p.resultOf(ctx, again)
	}
	if after.ID == "" {
		after = s
		after.Status = sessionExpired
	}
	return p.resultOf(ctx, after)
}

// RefundPayment refunds a PaymentFlow of API v2 in full, with key as the Idempotency-Key of the
// refund, so a repeat within PAY.JP's 24 hours refunds nothing twice. PAY.JP refunds a card
// payment for 180 days. The port has no refund of a one-time payment (operators refund tips in
// PAY.JP's dashboard); this is the call that one would make.
func (p *Provider) RefundPayment(ctx context.Context, payment supporters.PaymentRef, key string) error {
	if payment == "" || key == "" {
		return fmt.Errorf("payjp: %w: a refund needs its payment and a key", supporters.ErrPaymentInvalidRequest)
	}
	var r paymentRefund
	return p.postV2(ctx, "refund a payment", "/payment_refunds", map[string]any{"payment_flow_id": string(payment)}, key, &r)
}
