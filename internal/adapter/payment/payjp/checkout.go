// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The pages of the adapter under supporters.PagesPath(Name) (ADR-0099 §3).
const (
	pageCheckout = "checkout"
	pagePortal   = "portal"
)

// checkoutLifetime is how long the URL of a checkout lasts at least; it ends at the hour after
// next, so that the same key returns the same checkout within the hour.
const checkoutLifetime = 2 * time.Hour

// portalLifetime is how long the URL of the portal lasts.
const portalLifetime = time.Hour

// newCustomerPrefix starts the ID of a customer that a checkout creates, followed by our
// subscription's ID.
const newCustomerPrefix = "my_"

// pageState is what a page needs, which the adapter signs into its URL instead of keeping it
// (ADR-0099 §3).
type pageState struct {
	Page     string `json:"pg"`
	LocalID  string `json:"l,omitempty"`
	Plan     string `json:"p,omitempty"`
	Customer string `json:"c"`
	// NewCustomer reports that the checkout creates Customer.
	NewCustomer bool   `json:"n,omitempty"`
	Email       string `json:"e,omitempty"`
	Locale      string `json:"lc,omitempty"`
	// Return is where the browser goes after the page: the success URL of a checkout, the
	// return URL of the portal.
	Return string `json:"r"`
	Cancel string `json:"x,omitempty"`
	Exp    int64  `json:"exp"`
}

// deriveKey derives the key of one purpose from the secret key.
func deriveKey(secret, purpose string) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(purpose))
	return m.Sum(nil)
}

// sign returns st as base64url(JSON).base64url(HMAC-SHA256).
func (p *Provider) sign(st pageState) string {
	b, _ := json.Marshal(st)
	body := base64.RawURLEncoding.EncodeToString(b)
	m := hmac.New(sha256.New, p.stateKey)
	m.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// errBadState is a state that is not ours, altered, or expired.
var errBadState = errors.New("payjp: invalid or expired page state")

// verify returns the state of s when it is ours, of page, and not expired at now.
func (p *Provider) verify(s, page string, now time.Time) (pageState, error) {
	body, sig, ok := strings.Cut(s, ".")
	if !ok {
		return pageState{}, errBadState
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return pageState{}, errBadState
	}
	m := hmac.New(sha256.New, p.stateKey)
	m.Write([]byte(body))
	if !hmac.Equal(got, m.Sum(nil)) {
		return pageState{}, errBadState
	}
	b, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return pageState{}, errBadState
	}
	var st pageState
	if err := json.Unmarshal(b, &st); err != nil || st.Page != page || !now.Before(time.Unix(st.Exp, 0)) {
		return pageState{}, errBadState
	}
	return st, nil
}

// pageURL returns the URL of a page on the origin of base.
func pageURL(base, page, state string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("payjp: %w: %q is not an absolute URL", supporters.ErrPaymentInvalidRequest, base)
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host, Path: supporters.PagesPath(Name) + page, RawQuery: url.Values{"s": {state}}.Encode()}
	return out.String(), nil
}

// sameOrigin reports whether two absolute URLs share their origin.
func sameOrigin(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	return err1 == nil && err2 == nil && ua.Scheme == ub.Scheme && ua.Host == ub.Host
}

// checkoutRef is the ref of a checkout: "<customer>/<our subscription>/<expiry>", which
// GetCheckout reads back without any record of the adapter.
func checkoutRef(customer, localID string, exp int64) supporters.CheckoutRef {
	return supporters.CheckoutRef(customer + "/" + localID + "/" + strconv.FormatInt(exp, 10))
}

func parseCheckoutRef(ref supporters.CheckoutRef) (customer, localID string, exp int64, ok bool) {
	s := string(ref)
	i, j := strings.Index(s, "/"), strings.LastIndex(s, "/")
	if i <= 0 || j <= i+1 {
		return "", "", 0, false
	}
	exp, err := strconv.ParseInt(s[j+1:], 10, 64)
	if err != nil {
		return "", "", 0, false
	}
	return s[:i], s[i+1 : j], exp, true
}

// StartCheckout implements supporters.PaymentProvider: the URL of the adapter's checkout page on
// the origin of SuccessURL, with the state of the checkout signed into it (ADR-0099 §3).
func (p *Provider) StartCheckout(ctx context.Context, req supporters.CheckoutRequest) (supporters.Checkout, error) {
	if req.LocalID == "" || req.Price == "" || !sameOrigin(req.SuccessURL, req.CancelURL) {
		return supporters.Checkout{}, fmt.Errorf("payjp: %w: a checkout needs our ID, a price, and return URLs on one origin", supporters.ErrPaymentInvalidRequest)
	}
	pl, err := p.getPlan(ctx, req.Price)
	if err != nil {
		return supporters.Checkout{}, err
	}
	if !strings.EqualFold(pl.Currency, currency) {
		return supporters.Checkout{}, fmt.Errorf("payjp: %w: plan %s is in %s", supporters.ErrPaymentInvalidRequest, pl.ID, pl.Currency)
	}
	exp := p.o.Now().Truncate(time.Hour).Add(checkoutLifetime)
	st := pageState{
		Page: pageCheckout, LocalID: req.LocalID, Plan: pl.ID, Customer: string(req.Customer), Email: req.Email,
		Locale: req.Locale, Return: req.SuccessURL, Cancel: req.CancelURL, Exp: exp.Unix(),
	}
	if st.Customer == "" {
		st.Customer, st.NewCustomer = newCustomerPrefix+req.LocalID, true
	}
	u, err := pageURL(req.SuccessURL, pageCheckout, p.sign(st))
	if err != nil {
		return supporters.Checkout{}, err
	}
	return supporters.Checkout{Ref: checkoutRef(st.Customer, req.LocalID, st.Exp), RedirectURL: u, ExpiresAt: exp.UTC()}, nil
}

// GetCheckout implements supporters.PaymentProvider: completed once the customer has a
// subscription with our ID, expired after the ref's expiry without one, open otherwise.
func (p *Provider) GetCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.CheckoutResult, error) {
	cus, localID, exp, ok := parseCheckoutRef(ref)
	if !ok {
		return supporters.CheckoutResult{}, fmt.Errorf("payjp: %w: checkout %q", supporters.ErrPaymentNotFound, ref)
	}
	res := supporters.CheckoutResult{Ref: ref, Status: supporters.CheckoutOpen, LocalID: localID}
	s, found, err := p.findSubscription(ctx, cus, localID)
	switch {
	case err != nil:
		return supporters.CheckoutResult{}, err
	case found:
		res.Status, res.Customer, res.Subscription = supporters.CheckoutCompleted, supporters.CustomerRef(cus), supporters.SubscriptionRef(s.ID)
	case !p.o.Now().Before(time.Unix(exp, 0)):
		res.Status = supporters.CheckoutExpired
	}
	return res, nil
}

// findSubscription looks for the subscription of a customer with our ID.
func (p *Provider) findSubscription(ctx context.Context, cus, localID string) (subscription, bool, error) {
	for offset := 0; ; offset += supporters.PageSize {
		var page list[subscription]
		q := url.Values{"customer": {cus}, "limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)}}
		err := p.get(ctx, "list the subscriptions of a customer", "/subscriptions", q, &page)
		if errors.Is(err, supporters.ErrPaymentNotFound) {
			return subscription{}, false, nil
		}
		if err != nil {
			return subscription{}, false, err
		}
		for _, s := range page.Data {
			if s.Metadata[metaSubscriptionID] == localID {
				return s, true, nil
			}
		}
		if !page.HasMore || len(page.Data) == 0 {
			return subscription{}, false, nil
		}
	}
}

// PortalURL implements supporters.PaymentProvider: the URL of the adapter's portal page on the
// origin of returnURL (ADR-0099 §3).
func (p *Provider) PortalURL(_ context.Context, cus supporters.CustomerRef, returnURL string) (string, error) {
	if cus == "" {
		return "", fmt.Errorf("payjp: %w: no customer", supporters.ErrPaymentInvalidRequest)
	}
	st := pageState{Page: pagePortal, Customer: string(cus), Return: returnURL, Exp: p.o.Now().Add(portalLifetime).Unix()}
	return pageURL(returnURL, pagePortal, p.sign(st))
}
