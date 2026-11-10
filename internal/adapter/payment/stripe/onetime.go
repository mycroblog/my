// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v87"

	"github.com/mycroblog/my/internal/app/supporters"
)

// metaTipID is the metadata key that carries our tip's ID on a one-time checkout and its payment.
const metaTipID = "my_tip_id"

// maxOneTimeAmount is the largest amount that Stripe takes in one charge: eight digits of minor
// units (https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts). Stripe takes
// twelve digits in a few currencies; eight holds in all of them.
const maxOneTimeAmount = 99_999_999

var _ supporters.OneTimePayments = (*Provider)(nil)

// OneTimeLimits implements supporters.OneTimePayments: Stripe's smallest charge of the currencies
// of MinimumAmount, which a one-time payment shares with the plans, up to maxOneTimeAmount.
func (p *Provider) OneTimeLimits(currency string) (minimum, maximum int64, ok bool) {
	minimum, ok = p.MinimumAmount(currency)
	if !ok {
		return 0, 0, false
	}
	return minimum, maxOneTimeAmount, true
}

// StartOneTimeCheckout implements supporters.OneTimePayments (ADR-0162 §10): a hosted Checkout
// Session in payment mode, cards only, with one line item priced inline (no Product or Price is
// created), that carries LocalID as its client_reference_id and in the metadata of the session
// and of its PaymentIntent. It creates no customer: the email only prefills the page. Adaptive
// Pricing is off, so that the customer pays the amount and currency asked. Like StartCheckout,
// it expires an hour after the minute it was asked in, so that a repeat with the same Key sends
// the same parameters.
func (p *Provider) StartOneTimeCheckout(ctx context.Context, req supporters.OneTimeRequest) (supporters.Checkout, error) {
	if req.LocalID == "" || req.Description == "" || req.CancelURL == "" {
		return supporters.Checkout{}, fmt.Errorf("stripe: %w: a one-time checkout needs our ID, a description, and the URLs", supporters.ErrPaymentInvalidRequest)
	}
	minimum, maximum, ok := p.OneTimeLimits(req.Currency)
	if !ok || req.AmountMinor < minimum || req.AmountMinor > maximum {
		return supporters.Checkout{}, fmt.Errorf("stripe: %w: %d %s is outside the one-time charges that Stripe takes",
			supporters.ErrPaymentInvalidRequest, req.AmountMinor, req.Currency)
	}
	success, err := successURL(req.SuccessURL)
	if err != nil {
		return supporters.Checkout{}, err
	}
	meta := map[string]string{metaTipID: req.LocalID}
	params := &stripe.CheckoutSessionCreateParams{
		Params: idempotency(req.Key, "checkout"),
		Mode:   stripe.String(string(stripe.CheckoutSessionModePayment)),
		// A payment method that settles later completes the session unpaid (ADR-0162 §10).
		AllowedPaymentMethodTypes: []*string{stripe.String("card")},
		AdaptivePricing:           &stripe.CheckoutSessionCreateAdaptivePricingParams{Enabled: stripe.Bool(false)},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:    stripe.String(strings.ToLower(req.Currency)),
				UnitAmount:  stripe.Int64(req.AmountMinor),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{Name: stripe.String(req.Description)},
			},
			Quantity: stripe.Int64(1),
		}},
		ClientReferenceID: stripe.String(req.LocalID),
		Metadata:          meta,
		PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{Metadata: meta},
		Locale:            stripe.String(checkoutLocale(req.Locale)),
		SuccessURL:        stripe.String(success),
		CancelURL:         stripe.String(req.CancelURL),
		ExpiresAt:         stripe.Int64(p.o.Now().Truncate(time.Minute).Add(checkoutLifetime).Unix()),
	}
	if req.Email != "" {
		params.CustomerEmail = stripe.String(req.Email)
	}
	var cs *stripe.CheckoutSession
	err = p.do(ctx, "create a one-time checkout", func() (err error) {
		cs, err = p.sc.V1CheckoutSessions.Create(ctx, params)
		return err
	})
	if err != nil {
		return supporters.Checkout{}, err
	}
	if err := checkHostedURL(cs.URL, CheckoutHost); err != nil {
		return supporters.Checkout{}, err
	}
	return supporters.Checkout{Ref: supporters.CheckoutRef(cs.ID), RedirectURL: cs.URL, ExpiresAt: unixTime(cs.ExpiresAt)}, nil
}

// GetOneTimeCheckout implements supporters.OneTimePayments. A session in another mode is not a
// one-time checkout, and fails with ErrPaymentNotFound.
func (p *Provider) GetOneTimeCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.OneTimeResult, error) {
	cs, err := p.session(ctx, ref)
	if err != nil {
		return supporters.OneTimeResult{}, err
	}
	return p.oneTimeResult(ctx, cs)
}

// ExpireOneTimeCheckout implements supporters.OneTimePayments. Like ExpireCheckout, a refusal of
// Stripe, which expires open sessions only, is settled by what the session is now.
func (p *Provider) ExpireOneTimeCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.OneTimeResult, error) {
	var cs *stripe.CheckoutSession
	err := p.do(ctx, "expire the one-time checkout "+string(ref), func() (err error) {
		cs, err = p.sc.V1CheckoutSessions.Expire(ctx, string(ref), &stripe.CheckoutSessionExpireParams{})
		return err
	})
	if err == nil {
		return p.oneTimeResult(ctx, cs)
	}
	if supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
		return supporters.OneTimeResult{}, err
	}
	res, readErr := p.GetOneTimeCheckout(ctx, ref)
	if readErr != nil {
		return supporters.OneTimeResult{}, readErr
	}
	if res.Status == supporters.OneTimeOpen {
		return supporters.OneTimeResult{}, err
	}
	return res, nil
}

// session reads a Checkout Session.
func (p *Provider) session(ctx context.Context, ref supporters.CheckoutRef) (*stripe.CheckoutSession, error) {
	var cs *stripe.CheckoutSession
	err := p.do(ctx, "read the checkout "+string(ref), func() (err error) {
		cs, err = p.sc.V1CheckoutSessions.Retrieve(ctx, string(ref), &stripe.CheckoutSessionRetrieveParams{})
		return err
	})
	return cs, err
}

// oneTimeResult returns the port's result of a session in payment mode: paid when it is complete
// and its payment_status is paid, with its PaymentIntent and the amount it charged. A session that
// completed unpaid, which cards do not leave, stays open, and is logged.
func (p *Provider) oneTimeResult(ctx context.Context, cs *stripe.CheckoutSession) (supporters.OneTimeResult, error) {
	if cs.Mode != stripe.CheckoutSessionModePayment {
		return supporters.OneTimeResult{}, fmt.Errorf("stripe: %w: the checkout %s is not of one payment", supporters.ErrPaymentNotFound, cs.ID)
	}
	res := supporters.OneTimeResult{Ref: supporters.CheckoutRef(cs.ID), LocalID: cs.ClientReferenceID, Status: supporters.OneTimeOpen}
	if res.LocalID == "" {
		res.LocalID = cs.Metadata[metaTipID]
	}
	switch cs.Status {
	case stripe.CheckoutSessionStatusComplete:
		pi := paymentIntentID(cs.PaymentIntent)
		if cs.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid || pi == "" {
			p.log.WarnContext(ctx, "a one-time checkout completed without its payment",
				slog.String("checkout", cs.ID), slog.String("payment_status", string(cs.PaymentStatus)))
			return res, nil
		}
		res.Status = supporters.OneTimePaid
		res.Payment = supporters.PaymentRef(pi)
		res.AmountMinor = cs.AmountTotal
		res.Currency = strings.ToUpper(string(cs.Currency))
	case stripe.CheckoutSessionStatusExpired:
		res.Status = supporters.OneTimeExpired
	}
	return res, nil
}

// resolveCheckout tells a one-time checkout from a subscription's by the session's mode. A session
// that Stripe does not know goes to the subscriptions' checkouts, whose re-read reports it.
func (p *Provider) resolveCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.Change, error) {
	cs, err := p.session(ctx, ref)
	switch {
	case supporters.ErrorKindOf(err) == supporters.ErrorNotFound:
		return supporters.Change{Checkout: ref}, nil
	case err != nil:
		return supporters.Change{}, err
	case cs.Mode == stripe.CheckoutSessionModePayment:
		return supporters.Change{OneTimeCheckout: ref}, nil
	}
	return supporters.Change{Checkout: ref}, nil
}
