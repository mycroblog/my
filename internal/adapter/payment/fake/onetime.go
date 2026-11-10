// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The one-time payments of the fake (supporters.OneTimePayments, ADR-0162 §9): hosted checkouts
// of one payment on CheckoutHost, which tests pay with PayOneTime or put with PutOneTimeCheckout,
// and whose money they book with PutMoney, naming the payment in MoneyMovement.Payment.

var _ supporters.OneTimePayments = (*Provider)(nil)

// oneTimeMaximum is the largest one-time charge of the fake, in minor units of any currency.
const oneTimeMaximum = 99_999_999

// exponents are the minor units of the currencies of the fake.
var exponents = map[string]int{"JPY": 0, "USD": 2, "EUR": 2}

// oneTime is a one-time checkout.
type oneTime struct {
	result    supporters.OneTimeResult
	expiresAt time.Time
	req       *supporters.OneTimeRequest
}

// OneTimeLimits implements supporters.OneTimePayments.
func (p *Provider) OneTimeLimits(currency string) (int64, int64, bool) {
	n, ok := minimums[currency]
	if !ok {
		return 0, 0, false
	}
	return n, oneTimeMaximum, true
}

// StartOneTimeCheckout implements supporters.OneTimePayments.
func (p *Provider) StartOneTimeCheckout(ctx context.Context, req supporters.OneTimeRequest) (supporters.Checkout, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.Checkout{}, err
	}
	p.initOneTime()
	if req.Key != "" {
		if ref, ok := p.oneTimeKeys[req.Key]; ok {
			return p.oneTimeCheckoutOf(ref), nil
		}
	}
	minimum, maximum, ok := p.OneTimeLimits(req.Currency)
	if req.LocalID == "" || !ok || req.AmountMinor < minimum || req.AmountMinor > maximum {
		return supporters.Checkout{}, fmt.Errorf("%w: a one-time checkout needs our ID and an amount the provider takes", supporters.ErrPaymentInvalidRequest)
	}
	ref := supporters.CheckoutRef(p.newRef("cs_tip"))
	if p.o.Pages {
		if _, err := p.pageURL(req.SuccessURL, ""); err != nil || !sameOrigin(req.SuccessURL, req.CancelURL) {
			return supporters.Checkout{}, fmt.Errorf("%w: a checkout needs return URLs on one origin", supporters.ErrPaymentInvalidRequest)
		}
		ref = supporters.CheckoutRef("cs_tip_" + randomToken())
	}
	r := req
	p.oneTimes[ref] = &oneTime{
		result:    supporters.OneTimeResult{Ref: ref, Status: supporters.OneTimeOpen, LocalID: req.LocalID},
		expiresAt: p.o.Now().Add(time.Hour), req: &r,
	}
	if req.Key != "" {
		p.oneTimeKeys[req.Key] = ref
	}
	return p.oneTimeCheckoutOf(ref), nil
}

func (p *Provider) initOneTime() {
	if p.oneTimes == nil {
		p.oneTimes = map[supporters.CheckoutRef]*oneTime{}
		p.oneTimeKeys = map[string]supporters.CheckoutRef{}
	}
}

func (p *Provider) oneTimeCheckoutOf(ref supporters.CheckoutRef) supporters.Checkout {
	c := p.oneTimes[ref]
	redirect := "https://" + CheckoutHost + "/t/" + url.PathEscape(string(ref))
	if p.o.Pages && c.req != nil {
		redirect, _ = p.pageURL(c.req.SuccessURL, pageTip+url.PathEscape(string(ref))) // StartOneTimeCheckout checked the URL
	}
	return supporters.Checkout{Ref: ref, RedirectURL: redirect, ExpiresAt: c.expiresAt}
}

// GetOneTimeCheckout implements supporters.OneTimePayments.
func (p *Provider) GetOneTimeCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.OneTimeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.OneTimeResult{}, err
	}
	return p.oneTimeResult(ref)
}

func (p *Provider) oneTimeResult(ref supporters.CheckoutRef) (supporters.OneTimeResult, error) {
	c, ok := p.oneTimes[ref]
	if !ok {
		return supporters.OneTimeResult{}, fmt.Errorf("%w: one-time checkout %s", supporters.ErrPaymentNotFound, ref)
	}
	if c.result.Status == supporters.OneTimeOpen && !p.o.Now().Before(c.expiresAt) {
		c.result.Status = supporters.OneTimeExpired
	}
	return c.result, nil
}

// ExpireOneTimeCheckout implements supporters.OneTimePayments.
func (p *Provider) ExpireOneTimeCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.OneTimeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.OneTimeResult{}, err
	}
	res, err := p.oneTimeResult(ref)
	if err != nil || res.Status != supporters.OneTimeOpen {
		return res, err
	}
	p.oneTimes[ref].result.Status = supporters.OneTimeExpired
	return p.oneTimes[ref].result, nil
}

// PutOneTimeCheckout creates or replaces a one-time checkout, as a buyer paying it or its expiry
// does. A checkout created so expires an hour after now.
func (p *Provider) PutOneTimeCheckout(res supporters.OneTimeResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.initOneTime()
	if c, ok := p.oneTimes[res.Ref]; ok {
		c.result = res
		return
	}
	p.oneTimes[res.Ref] = &oneTime{result: res, expiresAt: p.o.Now().Add(time.Hour)}
}

// PayOneTime pays an open one-time checkout that StartOneTimeCheckout made, as the buyer does,
// and books its final payment, with a fee of 3.6% rounded down, at now. It returns the paid
// checkout and the ID of the money movement; it fails for a checkout that is not open.
func (p *Provider) PayOneTime(ref supporters.CheckoutRef) (supporters.OneTimeResult, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.payOneTime(ref)
}

// payOneTime is PayOneTime with p.mu held.
func (p *Provider) payOneTime(ref supporters.CheckoutRef) (supporters.OneTimeResult, string, error) {
	p.initOneTime()
	res, err := p.oneTimeResult(ref)
	if err != nil {
		return supporters.OneTimeResult{}, "", err
	}
	c := p.oneTimes[ref]
	if res.Status != supporters.OneTimeOpen || c.req == nil {
		return supporters.OneTimeResult{}, "", fmt.Errorf("fake: one-time checkout %s is %s", ref, res.Status)
	}
	payment := supporters.PaymentRef(p.newRef("pi"))
	c.result.Status, c.result.Payment = supporters.OneTimePaid, payment
	c.result.AmountMinor, c.result.Currency = c.req.AmountMinor, c.req.Currency
	id := p.newRef("txn")
	fee := c.req.AmountMinor * 36 / 1000
	now := p.o.Now()
	p.putMoney(id, supporters.MoneyMovement{
		ID: id, Kind: supporters.MoneyPayment, Payment: payment, OccurredAt: now,
		Gross: decimal(c.req.AmountMinor, c.req.Currency), Fee: decimal(fee, c.req.Currency),
		Net: decimal(c.req.AmountMinor-fee, c.req.Currency), Currency: c.req.Currency, Final: true,
	})
	return c.result, id, nil
}

// OneTimeRequest returns the request of a one-time checkout that StartOneTimeCheckout made.
func (p *Provider) OneTimeRequest(ref supporters.CheckoutRef) (supporters.OneTimeRequest, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.oneTimes[ref]
	if !ok || c.req == nil {
		return supporters.OneTimeRequest{}, false
	}
	return *c.req, true
}

// isOneTime reports with p.mu held whether ref is a one-time checkout.
func (p *Provider) isOneTime(ref supporters.CheckoutRef) bool {
	_, ok := p.oneTimes[ref]
	return ok
}

// decimal formats minor units of currency in major units, as MoneyMovement does.
func decimal(minor int64, currency string) string {
	exp := exponents[currency]
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	s := strconv.FormatInt(minor, 10)
	if exp == 0 {
		return sign + s
	}
	for len(s) <= exp {
		s = "0" + s
	}
	return sign + s[:len(s)-exp] + "." + s[len(s)-exp:]
}
