// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/pkg/testkit"
)

const pagesHost = "my.example"

// A PageProvider passes the contract with its pages on the origin of the
// success and return URLs.
func TestPaymentProviderContractWithPages(t *testing.T) {
	testkit.PaymentProviderContract(t, func(t *testing.T, s testkit.PaymentScenario) testkit.PaymentUnderTest {
		u := newOnScenario(t, s)
		p := u.Provider.(*Provider)
		p.o.Pages = true
		u.Provider = PageProvider{p}
		u.CheckoutHosts, u.PortalHosts = []string{pagesHost}, []string{pagesHost}
		return u
	})
}

// pagesEnv is a provider with pages and a plan of 1,200 yen a month.
type pagesEnv struct {
	p     PageProvider
	h     http.Handler
	price supporters.PriceRef
	now   time.Time
}

func newPagesEnv(t *testing.T) *pagesEnv {
	t.Helper()
	e := &pagesEnv{now: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	e.p = NewPages(Options{Now: func() time.Time { return e.now }})
	refs, err := e.p.EnsurePlan(t.Context(), supporters.PlanSpec{
		PlanID: "plan-1", Name: "Supporter", Prices: []supporters.PriceSpec{{Interval: supporters.IntervalMonth, AmountMinor: 1200, Currency: "JPY"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.price = refs.Prices[0].Ref
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	e.h = e.p.Pages(supporters.PageEnv{Locales: &i18n.Resolver{Bundle: b}, Stylesheet: func() string { return "/assets/server.css" }})
	return e
}

func (e *pagesEnv) start(t *testing.T) supporters.Checkout {
	t.Helper()
	c, err := e.p.StartCheckout(t.Context(), supporters.CheckoutRequest{
		LocalID: "local-1", Price: e.price, Locale: "en", Key: "k",
		SuccessURL: "https://" + pagesHost + "/settings/supporter?checkout=success", CancelURL: "https://" + pagesHost + "/settings/supporter?checkout=canceled",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *pagesEnv) get(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://"+pagesHost+path, nil))
	return rec
}

func (e *pagesEnv) post(path, action, site string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "https://"+pagesHost+path, strings.NewReader(url.Values{"action": {action}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", site)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func pathOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Path
}

// The checkout page shows the plan, its price, and that no money moves; fail shows it again
// with the failure, and pay records the subscription and its payment and returns to the success
// URL, once.
func TestCheckoutPage(t *testing.T) {
	e := newPagesEnv(t)
	c := e.start(t)
	if !strings.HasPrefix(c.RedirectURL, "https://"+pagesHost+"/payments/pages/fake/checkout/cs_") || len(c.Ref) != len("cs_")+32 {
		t.Fatalf("StartCheckout = %+v, want a page of an unguessable checkout on the origin of the success URL", c)
	}
	path := pathOf(t, c.RedirectURL)

	rec := e.get(path)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Subscribe to Supporter") || !strings.Contains(body, "1200 JPY a month") ||
		!strings.Contains(body, "no money moves") || !strings.Contains(body, `value="pay"`) {
		t.Fatalf("GET the checkout = %d %s", rec.Code, body)
	}
	for k, want := range map[string]string{"Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "Content-Security-Policy": pagesCSP} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	if rec := e.post(path, actionPay, "cross-site"); rec.Code != http.StatusForbidden {
		t.Errorf("a cross-site post = %d, want 403", rec.Code)
	}
	rec = e.post(path, actionFail, "same-origin")
	if rec.Code != http.StatusPaymentRequired || !strings.Contains(rec.Body.String(), "The payment failed") {
		t.Errorf("fail = %d %s, want 402 with the failure", rec.Code, rec.Body.String())
	}
	if res, _ := e.p.GetCheckout(t.Context(), c.Ref); res.Status != supporters.CheckoutOpen {
		t.Errorf("after fail, the checkout is %s, want open", res.Status)
	}
	if rec := e.post(path, "other", "same-origin"); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown action = %d, want 400", rec.Code)
	}

	rec = e.post(path, actionPay, "same-origin")
	want := "https://" + pagesHost + "/settings/supporter?checkout=success&checkout_id=" + string(c.Ref)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Fatalf("pay = %d to %q, want 303 to %q", rec.Code, rec.Header().Get("Location"), want)
	}
	res, err := e.p.GetCheckout(t.Context(), c.Ref)
	if err != nil || res.Status != supporters.CheckoutCompleted || res.LocalID != "local-1" || res.Customer == "" || res.Subscription == "" {
		t.Fatalf("GetCheckout after pay = %+v, %v", res, err)
	}
	sub, err := e.p.GetSubscription(t.Context(), res.Subscription)
	if err != nil || sub.Status != supporters.StatusActive || sub.LocalID != "local-1" || sub.Price != e.price ||
		!sub.PeriodStart.Equal(e.now) || !sub.PeriodEnd.Equal(e.now.AddDate(0, 1, 0)) {
		t.Errorf("the subscription = %+v, %v", sub, err)
	}
	money, err := e.p.ListMoney(t.Context(), time.Time{}, "")
	if err != nil || len(money.Movements) != 1 {
		t.Fatalf("ListMoney = %+v, %v, want the first payment", money, err)
	}
	if m := money.Movements[0]; m.Kind != supporters.MoneyPayment || m.Subscription != res.Subscription || m.Gross != "1200" ||
		m.Net != "1200" || m.Currency != "JPY" || !m.Final || m.Covers.Start != supporters.DateOf(e.now) {
		t.Errorf("the payment = %+v", m)
	}
	changes, err := e.p.ListChanges(t.Context(), e.now.Add(-time.Hour), "")
	if err != nil || len(changes.Notices) != 3 {
		t.Errorf("ListChanges = %+v, %v, want the events of the checkout, the subscription, and the payment", changes, err)
	}

	// Paying again, or opening the page again, goes to the success URL without another subscription.
	if rec := e.post(path, actionPay, "same-origin"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Errorf("pay again = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := e.get(path); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Errorf("GET after pay = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if page, _ := e.p.ListSubscriptions(t.Context(), ""); len(page.Subscriptions) != 1 {
		t.Errorf("subscriptions %+v, want one", page.Subscriptions)
	}
}

// Cancel returns to the cancel URL; an unknown or expired checkout is not served.
func TestCheckoutPageCancelAndExpiry(t *testing.T) {
	e := newPagesEnv(t)
	path := pathOf(t, e.start(t).RedirectURL)
	rec := e.post(path, actionCancel, "same-origin")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "https://"+pagesHost+"/settings/supporter?checkout=canceled" {
		t.Errorf("cancel = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := e.get("/payments/pages/fake/checkout/cs_unknown"); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown checkout = %d, want 404", rec.Code)
	}
	e.now = e.now.Add(time.Hour)
	if rec := e.get(path); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "expired") {
		t.Errorf("an expired checkout = %d %s, want 404", rec.Code, rec.Body.String())
	}
	if rec := e.post(path, actionPay, "same-origin"); rec.Code != http.StatusNotFound {
		t.Errorf("pay an expired checkout = %d, want 404", rec.Code)
	}
}

// The portal page links back; a checkout needs its return URLs on one origin; without
// Options.Pages, the URLs are on the fake hosts, and a plain Provider serves no pages.
func TestPortalPageAndOptions(t *testing.T) {
	e := newPagesEnv(t)
	u, err := e.p.PortalURL(t.Context(), "cus_1", "https://"+pagesHost+"/settings/supporter?portal=returned")
	if err != nil {
		t.Fatal(err)
	}
	rec := e.get(pathOf(t, u))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `href="https://my.example/settings/supporter?portal=returned"`) {
		t.Errorf("GET the portal = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := e.p.StartCheckout(t.Context(), supporters.CheckoutRequest{
		LocalID: "local-2", Price: e.price, SuccessURL: "https://my.example/ok", CancelURL: "https://other.example/no",
	}); supporters.ErrorKindOf(err) != supporters.ErrorInvalidRequest {
		t.Errorf("StartCheckout across origins = %v, want an invalid request", err)
	}

	plain := New(Options{})
	if _, ok := any(plain).(supporters.PageServer); ok {
		t.Error("a plain Provider is a PageServer")
	}
	if u, err := plain.PortalURL(t.Context(), "cus_1", "https://my.example/back"); err != nil || !strings.HasPrefix(u, "https://"+PortalHost+"/") {
		t.Errorf("PortalURL without Options.Pages = %q, %v, want a URL on %s", u, err, PortalHost)
	}
}
