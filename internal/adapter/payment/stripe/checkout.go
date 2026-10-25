// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v87"

	"github.com/mycroblog/my/internal/app/supporters"
)

// checkoutLocales are the locales of Stripe's hosted Checkout
// (https://docs.stripe.com/api/checkout/sessions/create#create_checkout_session-locale).
var checkoutLocales = map[string]bool{
	"bg": true, "cs": true, "da": true, "de": true, "el": true, "en": true, "en-GB": true, "es": true,
	"es-419": true, "et": true, "fi": true, "fil": true, "fr": true, "fr-CA": true, "hr": true, "hu": true,
	"id": true, "it": true, "ja": true, "ko": true, "lt": true, "lv": true, "ms": true, "mt": true,
	"nb": true, "nl": true, "pl": true, "pt": true, "pt-BR": true, "ro": true, "ru": true, "sk": true,
	"sl": true, "sv": true, "th": true, "tr": true, "vi": true, "zh": true, "zh-HK": true, "zh-TW": true,
}

// checkoutLocale returns the locale of Checkout for a BCP 47 tag: the tag, its language and
// region, its language, or "auto" (the browser's).
func checkoutLocale(tag string) string {
	tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	parts := strings.Split(tag, "-")
	if len(parts) == 0 || parts[0] == "" {
		return "auto"
	}
	lang := strings.ToLower(parts[0])
	if len(parts) > 1 {
		region := parts[len(parts)-1]
		if len(region) == 2 {
			region = strings.ToUpper(region)
		}
		if c := lang + "-" + region; checkoutLocales[c] {
			return c
		}
	}
	if checkoutLocales[lang] {
		return lang
	}
	return "auto"
}

// successURL appends the placeholder of the checkout's ID that Stripe fills in, as checkout_id:
// the return URLs of the client API already carry checkout=success, and the web client reads a
// repeated key as a list and so misses the return.
func successURL(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("stripe: %w: the success URL %q is not absolute", supporters.ErrPaymentInvalidRequest, u)
	}
	sep := "?"
	if parsed.RawQuery != "" {
		sep = "&"
	}
	// The placeholder must stay unescaped for Stripe to replace it.
	return u + sep + "checkout_id={CHECKOUT_SESSION_ID}", nil
}

// checkHostedURL refuses a URL of a hosted page that is not https on host (ADR-0074 §7): an
// answer that was tampered with must not send the browser elsewhere.
func checkHostedURL(u, host string) error {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != host || parsed.User != nil {
		return fmt.Errorf("stripe: %w: a hosted page outside https://%s", supporters.ErrInvalidPaymentResponse, host)
	}
	return nil
}

// StartCheckout implements supporters.PaymentProvider: a hosted Checkout Session in subscription
// mode, redirect only, that carries LocalID as its client_reference_id and in the metadata of
// the session and of the subscription it creates (ADR-0074 §2). It expires an hour after the
// minute it was asked in, so that a repeat with the same Key sends the same parameters, which
// Stripe's idempotency requires.
func (p *Provider) StartCheckout(ctx context.Context, req supporters.CheckoutRequest) (supporters.Checkout, error) {
	if req.LocalID == "" || req.Price == "" || req.CancelURL == "" {
		return supporters.Checkout{}, fmt.Errorf("stripe: %w: a checkout needs our ID, a price, and the URLs", supporters.ErrPaymentInvalidRequest)
	}
	success, err := successURL(req.SuccessURL)
	if err != nil {
		return supporters.Checkout{}, err
	}
	meta := map[string]string{metaSubscriptionID: req.LocalID}
	params := &stripe.CheckoutSessionCreateParams{
		Params:            idempotency(req.Key, "checkout"),
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems:         []*stripe.CheckoutSessionCreateLineItemParams{{Price: stripe.String(string(req.Price)), Quantity: stripe.Int64(1)}},
		ClientReferenceID: stripe.String(req.LocalID),
		Metadata:          meta,
		SubscriptionData:  &stripe.CheckoutSessionCreateSubscriptionDataParams{Metadata: meta},
		Locale:            stripe.String(checkoutLocale(req.Locale)),
		SuccessURL:        stripe.String(success),
		CancelURL:         stripe.String(req.CancelURL),
		ExpiresAt:         stripe.Int64(p.o.Now().Truncate(time.Minute).Add(checkoutLifetime).Unix()),
	}
	if req.Customer != "" {
		params.Customer = stripe.String(string(req.Customer))
	} else if req.Email != "" {
		params.CustomerEmail = stripe.String(req.Email)
	}
	var cs *stripe.CheckoutSession
	err = p.do(ctx, "create a checkout", func() (err error) {
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

// GetCheckout implements supporters.PaymentProvider.
func (p *Provider) GetCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.CheckoutResult, error) {
	var cs *stripe.CheckoutSession
	err := p.do(ctx, "read the checkout "+string(ref), func() (err error) {
		cs, err = p.sc.V1CheckoutSessions.Retrieve(ctx, string(ref), &stripe.CheckoutSessionRetrieveParams{})
		return err
	})
	if err != nil {
		return supporters.CheckoutResult{}, err
	}
	res := supporters.CheckoutResult{Ref: supporters.CheckoutRef(cs.ID), LocalID: cs.ClientReferenceID}
	if res.LocalID == "" {
		res.LocalID = cs.Metadata[metaSubscriptionID]
	}
	switch cs.Status {
	case stripe.CheckoutSessionStatusComplete:
		res.Status = supporters.CheckoutCompleted
		if cs.Customer != nil {
			res.Customer = supporters.CustomerRef(cs.Customer.ID)
		}
		if cs.Subscription != nil {
			res.Subscription = supporters.SubscriptionRef(cs.Subscription.ID)
		}
	case stripe.CheckoutSessionStatusExpired:
		res.Status = supporters.CheckoutExpired
	default:
		res.Status = supporters.CheckoutOpen
	}
	return res, nil
}

// PortalURL implements supporters.PaymentProvider: a session of the customer portal on the
// configuration of this server, which updates payment methods, shows the invoices, and cancels
// at the period's end, but never changes the plan (ADR-0074 §2, P1).
func (p *Provider) PortalURL(ctx context.Context, customer supporters.CustomerRef, returnURL string) (string, error) {
	if customer == "" {
		return "", fmt.Errorf("stripe: %w: no customer", supporters.ErrPaymentInvalidRequest)
	}
	config, err := p.portalConfiguration(ctx)
	if err != nil {
		return "", err
	}
	var s *stripe.BillingPortalSession
	err = p.do(ctx, "create a portal session", func() (err error) {
		s, err = p.sc.V1BillingPortalSessions.Create(ctx, &stripe.BillingPortalSessionCreateParams{
			Customer: stripe.String(string(customer)), ReturnURL: stripe.String(returnURL), Configuration: stripe.String(config),
		})
		return err
	})
	if err != nil {
		return "", err
	}
	if err := checkHostedURL(s.URL, PortalHost); err != nil {
		return "", err
	}
	return s.URL, nil
}

// portalConfiguration returns the ID of the portal's configuration of this server, which carries
// metadata my=1: found among the active ones or created, once per process.
func (p *Provider) portalConfiguration(ctx context.Context) (string, error) {
	p.portalMu.Lock()
	defer p.portalMu.Unlock()
	if p.portalConfig != "" {
		return p.portalConfig, nil
	}
	var list *stripe.V1List[*stripe.BillingPortalConfiguration]
	err := p.do(ctx, "list the portal configurations", func() error {
		list = p.sc.V1BillingPortalConfigurations.List(ctx, &stripe.BillingPortalConfigurationListParams{
			ListParams: stripe.ListParams{Limit: stripe.Int64(supporters.PageSize), Single: true}, Active: stripe.Bool(true),
		})
		return list.Err()
	})
	if err != nil {
		return "", err
	}
	for _, c := range list.Data() {
		if c.Metadata[metaPortal] == "1" {
			p.portalConfig = c.ID
			return c.ID, nil
		}
	}
	var c *stripe.BillingPortalConfiguration
	err = p.do(ctx, "create the portal configuration", func() (err error) {
		c, err = p.sc.V1BillingPortalConfigurations.Create(ctx, &stripe.BillingPortalConfigurationCreateParams{
			Name:     stripe.String("my supporters"),
			Metadata: map[string]string{metaPortal: "1"},
			Features: &stripe.BillingPortalConfigurationCreateFeaturesParams{
				PaymentMethodUpdate: &stripe.BillingPortalConfigurationCreateFeaturesPaymentMethodUpdateParams{Enabled: stripe.Bool(true)},
				InvoiceHistory:      &stripe.BillingPortalConfigurationCreateFeaturesInvoiceHistoryParams{Enabled: stripe.Bool(true)},
				SubscriptionCancel: &stripe.BillingPortalConfigurationCreateFeaturesSubscriptionCancelParams{
					Enabled: stripe.Bool(true), Mode: stripe.String("at_period_end"), ProrationBehavior: stripe.String("none"),
				},
				SubscriptionUpdate: &stripe.BillingPortalConfigurationCreateFeaturesSubscriptionUpdateParams{Enabled: stripe.Bool(false)},
			},
		})
		return err
	})
	if err != nil {
		return "", err
	}
	p.portalConfig = c.ID
	return c.ID, nil
}
