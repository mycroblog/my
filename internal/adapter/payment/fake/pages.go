// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/i18n"
)

// The pages of a PageProvider stand in for a provider's hosted checkout and
// portal in the test builds (build tag e2e_fakes, ADR-0117 §2): the checkout shows the plan, its
// price, and that no money moves, and offers to pay, to make the payment fail, or to cancel.
// They are server-rendered like the PAY.JP pages and run no script.

//go:embed templates/*.html
var templateFS embed.FS

var pageTemplates = map[string]*template.Template{
	"checkout": template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/checkout.html")),
	"portal":   template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/portal.html")),
	"error":    template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/error.html")),
}

// pagesCSP is the Content-Security-Policy of the pages: no script, the server pages' stylesheet,
// and forms to this origin only.
const pagesCSP = "default-src 'none'; style-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// The pages under supporters.PagesPath, each followed by its token.
const (
	pageCheckout = "checkout/"
	pagePortal   = "portal/"
)

// The actions of the checkout form.
const (
	actionPay    = "pay"
	actionFail   = "fail"
	actionCancel = "cancel"
)

// maxPageForm bounds the body of the checkout form.
const maxPageForm = 4 << 10

// portal is a portal page that PortalURL made.
type portal struct {
	customer  supporters.CustomerRef
	returnURL string
	expiresAt time.Time
}

// PageProvider is a Provider with its own checkout and portal pages: a supporters.PageServer.
// A plain Provider is not one, as the hosted checkout of Stripe is not.
type PageProvider struct{ *Provider }

var _ supporters.PageServer = PageProvider{}

// NewPages returns a PageProvider with nothing on its side; it sets o.Pages.
func NewPages(o Options) PageProvider {
	o.Pages = true
	return PageProvider{New(o)}
}

// randomToken returns 32 hex digits from crypto/rand.
func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// pageURL returns the URL of page (a page and its token) on the origin of base.
func (p *Provider) pageURL(base, page string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("%w: %q is not an absolute URL", supporters.ErrPaymentInvalidRequest, base)
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host, Path: supporters.PagesPath(p.o.Name) + page}
	return out.String(), nil
}

// sameOrigin reports whether two absolute URLs share their origin.
func sameOrigin(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	return err1 == nil && err2 == nil && ua.Scheme == ub.Scheme && ua.Host == ub.Host
}

// pages serves the pages with one environment.
type pages struct {
	p   *Provider
	env supporters.PageEnv
}

// Pages implements supporters.PageServer.
func (pp PageProvider) Pages(env supporters.PageEnv) http.Handler {
	p := pp.Provider
	h := &pages{p: p, env: env}
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.renderError(w, r, http.StatusForbidden, "")
	}))
	base := supporters.PagesPath(p.o.Name)
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+base+pageCheckout+"{token}", h.checkoutPage)
	mux.Handle("POST "+base+pageCheckout+"{token}", cop.Handler(http.HandlerFunc(h.checkout)))
	mux.HandleFunc("GET "+base+pagePortal+"{token}", h.portalPage)
	return mux
}

// view is the data of a template.
type view struct {
	L         *i18n.Localizer
	Lang, Dir string
	Title     string
	// Stylesheet is the server pages' stylesheet.
	Stylesheet string
	Error      string
	// Back is where the page's link back goes.
	Back string
	// Price is the plan's price on the checkout page.
	Price string
}

func (h *pages) newView(r *http.Request, locale string) view {
	var l *i18n.Localizer
	if locale != "" {
		l = h.env.Locales.Localizer(r.Context(), locale)
	} else {
		l = h.env.Locales.RequestLocalizer(r)
	}
	v := view{L: l, Lang: l.Locale(), Dir: l.Direction()}
	if h.env.Stylesheet != nil {
		v.Stylesheet = h.env.Stylesheet()
	}
	return v
}

func (h *pages) render(w http.ResponseWriter, r *http.Request, status int, name string, v view) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Language", v.Lang)
	hd.Set("Content-Security-Policy", pagesCSP)
	hd.Set("Cache-Control", "no-store")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Add("Vary", "Accept-Language")
	w.WriteHeader(status)
	if err := pageTemplates[name].ExecuteTemplate(w, "layout", v); err != nil && h.env.Logger != nil {
		h.env.Logger.ErrorContext(r.Context(), "render a page of the fake payment provider", slog.String("page", name), slog.Any("error", err))
	}
}

// renderError renders the page of an expired or unknown link, with a link back to back ("" for
// none).
func (h *pages) renderError(w http.ResponseWriter, r *http.Request, status int, back string) {
	v := h.newView(r, "")
	v.Title, v.Error, v.Back = v.L.ServerPaymentsErrorTitle(), v.L.ServerPaymentsErrorState(), back
	h.render(w, r, status, "error", v)
}

// openCheckout returns the request of the checkout of token while it can be paid: false for an
// unknown, expired, or completed one, and done for a completed one.
func (p *Provider) openCheckout(token string) (req supporters.CheckoutRequest, ref supporters.CheckoutRef, done, ok bool) {
	ref = supporters.CheckoutRef(token)
	c, found := p.checkouts[ref]
	switch {
	case !found || c.req == nil:
		return supporters.CheckoutRequest{}, ref, false, false
	case c.result.Status == supporters.CheckoutCompleted:
		return *c.req, ref, true, false
	case !p.o.Now().Before(c.expiresAt):
		return *c.req, ref, false, false
	}
	return *c.req, ref, false, true
}

// successURL returns the success URL of a checkout with its ref as checkout_id, as the other
// providers' checkouts return.
func successURL(req supporters.CheckoutRequest, ref supporters.CheckoutRef) string {
	sep := "?"
	if strings.Contains(req.SuccessURL, "?") {
		sep = "&"
	}
	return req.SuccessURL + sep + "checkout_id=" + url.QueryEscape(string(ref))
}

// checkoutView returns the view of the checkout page of req.
func (h *pages) checkoutView(r *http.Request, req supporters.CheckoutRequest) view {
	h.p.mu.Lock()
	var spec supporters.PriceSpec
	var name string
	for _, pr := range h.p.prices {
		if pr.ref == req.Price {
			spec, name = pr.spec, h.p.planNames[pr.planID]
		}
	}
	h.p.mu.Unlock()
	v := h.newView(r, req.Locale)
	v.Title, v.Back = v.L.ServerPaymentsCheckoutTitle(name), req.CancelURL
	amount := formatMinor(spec.AmountMinor, spec.Currency)
	if spec.Interval == supporters.IntervalYear {
		v.Price = v.L.ServerPaymentsFakePriceYear(i18n.ServerPaymentsFakePriceYearArgs{Amount: amount, Currency: spec.Currency})
	} else {
		v.Price = v.L.ServerPaymentsFakePriceMonth(i18n.ServerPaymentsFakePriceMonthArgs{Amount: amount, Currency: spec.Currency})
	}
	return v
}

// checkoutPage shows the checkout, or goes on to the success URL once it is paid.
func (h *pages) checkoutPage(w http.ResponseWriter, r *http.Request) {
	h.p.mu.Lock()
	req, ref, done, ok := h.p.openCheckout(r.PathValue("token"))
	h.p.mu.Unlock()
	switch {
	case done:
		http.Redirect(w, r, successURL(req, ref), http.StatusSeeOther) //nolint:gosec // G710: the URL is the checkout's own, which our server gave
		return
	case !ok:
		h.renderError(w, r, http.StatusNotFound, req.CancelURL)
		return
	}
	h.render(w, r, http.StatusOK, "checkout", h.checkoutView(r, req))
}

// checkout takes the checkout form: pay completes the checkout with a new subscription and its
// first payment, fail shows the page again with the payment failed, and cancel goes back.
func (h *pages) checkout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPageForm)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "")
		return
	}
	h.p.mu.Lock()
	req, ref, done, ok := h.p.openCheckout(r.PathValue("token"))
	action := r.PostForm.Get("action")
	if ok && action == actionPay {
		h.p.pay(ref, req)
		done, ok = true, false
	}
	h.p.mu.Unlock()
	switch {
	case done:
		http.Redirect(w, r, successURL(req, ref), http.StatusSeeOther) //nolint:gosec // G710: the URL is the checkout's own, which our server gave
	case !ok:
		h.renderError(w, r, http.StatusNotFound, req.CancelURL)
	case action == actionCancel:
		http.Redirect(w, r, req.CancelURL, http.StatusSeeOther)
	case action == actionFail:
		v := h.checkoutView(r, req)
		v.Error = v.L.ServerPaymentsFakeFailed()
		h.render(w, r, http.StatusPaymentRequired, "checkout", v)
	default:
		h.renderError(w, r, http.StatusBadRequest, req.CancelURL)
	}
}

// pay completes the checkout ref of req with p.mu held, as a provider whose buyer paid: a
// customer (req.Customer or a new one), an active subscription on req.Price for one interval
// from now, its final first payment, and the events of the three.
func (p *Provider) pay(ref supporters.CheckoutRef, req supporters.CheckoutRequest) {
	now := p.o.Now().UTC()
	var spec supporters.PriceSpec
	for _, pr := range p.prices {
		if pr.ref == req.Price {
			spec = pr.spec
		}
	}
	end := now.AddDate(0, 1, 0)
	if spec.Interval == supporters.IntervalYear {
		end = now.AddDate(1, 0, 0)
	}
	customer := req.Customer
	if customer == "" {
		customer = supporters.CustomerRef(p.newRef("cus"))
	}
	sub := supporters.SubscriptionRef(p.newRef("sub"))
	p.subs = append(p.subs, supporters.SubscriptionSnapshot{
		Ref: sub, Customer: customer, LocalID: req.LocalID, Price: req.Price, Status: supporters.StatusActive,
		PeriodStart: now, PeriodEnd: end,
	})
	c := p.checkouts[ref]
	c.result.Status, c.result.Customer, c.result.Subscription = supporters.CheckoutCompleted, customer, sub
	gross, fee := formatMinor(spec.AmountMinor, spec.Currency), formatMinor(0, spec.Currency)
	txn := p.newRef("txn")
	p.putMoney(txn, supporters.MoneyMovement{
		ID: txn, Kind: supporters.MoneyPayment, Subscription: sub, OccurredAt: now,
		Covers: supporters.DateRange{Start: supporters.DateOf(now.In(p.o.Location)), End: supporters.DateOf(end.In(p.o.Location))},
		Gross:  gross, Fee: fee, Net: gross, Currency: spec.Currency, Final: true,
	})
	for _, n := range []supporters.Notice{
		{Type: "checkout.completed", Kind: supporters.NoticeCheckout, ObjectID: string(ref), Subscription: sub},
		{Type: "subscription.created", Kind: supporters.NoticeSubscription, ObjectID: string(sub)},
		{Type: "money.booked", Kind: supporters.NoticeMoney, ObjectID: txn, Subscription: sub},
	} {
		n.EventID, n.OccurredAt = p.newRef("evt"), now
		p.changes = append(p.changes, n)
	}
}

// formatMinor formats an amount in minor units of currency as a decimal in major units: no
// decimals for JPY, two for the others that the fake takes.
func formatMinor(amount int64, currency string) string {
	if currency == "JPY" {
		return strconv.FormatInt(amount, 10)
	}
	sign := ""
	if amount < 0 {
		sign, amount = "-", -amount
	}
	return fmt.Sprintf("%s%d.%02d", sign, amount/100, amount%100)
}

// portalPage shows that the test build has no portal, with the link back.
func (h *pages) portalPage(w http.ResponseWriter, r *http.Request) {
	h.p.mu.Lock()
	pt, found := h.p.portals[r.PathValue("token")]
	h.p.mu.Unlock()
	if !found || !h.p.o.Now().Before(pt.expiresAt) {
		h.renderError(w, r, http.StatusNotFound, "")
		return
	}
	v := h.newView(r, "")
	v.Title, v.Back = v.L.ServerPaymentsPortalTitle(), pt.returnURL
	h.render(w, r, http.StatusOK, "portal", v)
}
