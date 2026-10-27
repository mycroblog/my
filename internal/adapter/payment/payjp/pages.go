// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/i18n"
)

// The pages stand in for the hosted checkout and customer portal that PAY.JP API v1 lacks
// (ADR-0099 §3). They take the state that StartCheckout and PortalURL signed into their URL,
// load payjp.js, which takes the card in PAY.JP's own iframe and runs its 3-D Secure in an
// iframe, and receive only the token of the card: never its number, expiry, or code. The
// templates are embedded, the only scripts are payjp.js and the page's own with a nonce, and
// every response carries the pages' Content-Security-Policy (PCI DSS 6.4.3, 11.6.1; the
// decider's answer 4 on PR #1303).

//go:embed templates/*.html
var templateFS embed.FS

var pageTemplates = map[string]*template.Template{
	pageCheckout: template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/checkout.html")),
	pagePortal:   template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/portal.html")),
	"error":      template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/error.html")),
}

// pagesCSP is the Content-Security-Policy of the pages, with the nonce of their scripts: payjp.js
// from js.pay.jp, its iframes from js.pay.jp and api.pay.jp (the 3-D Secure), its requests to
// api.pay.jp, the server pages' stylesheet, and forms to this origin only.
const pagesCSP = "default-src 'none'; script-src 'nonce-%s' https://js.pay.jp; style-src 'self'; " +
	"frame-src https://js.pay.jp https://api.pay.jp; connect-src https://api.pay.jp; img-src 'self' data:; " +
	"form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// maxPageForm bounds the body of a page's form.
const maxPageForm = 16 << 10

// The 3-D Secure results that verify a card.
var verifiedThreeDSecure = map[string]bool{"verified": true, "attempted": true}

// The results of a card change that the portal reports after its redirect.
const (
	doneChanged      = "changed"
	doneResumed      = "resumed"
	doneResumeFailed = "resume_failed"
)

var _ supporters.PageServer = (*Provider)(nil)

// pages serves the pages with one environment.
type pages struct {
	p   *Provider
	env supporters.PageEnv
	cop *http.CrossOriginProtection
	// checkouts serializes the posts of the checkout form of one subscription, so that two at
	// the same time (a double submit, two tabs) do not both find no subscription and create two.
	checkouts keyedMutex
}

// keyedMutex is a set of mutexes by key, each kept while it is held or awaited.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

// lock locks key and returns its unlock.
func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedLock{}
	}
	l := k.locks[key]
	if l == nil {
		l = &keyedLock{}
		k.locks[key] = l
	}
	l.refs++
	k.mu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		k.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}

// Pages implements supporters.PageServer.
func (p *Provider) Pages(env supporters.PageEnv) http.Handler {
	h := &pages{p: p, env: env, cop: http.NewCrossOriginProtection()}
	h.cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.renderError(w, r, http.StatusForbidden, (*i18n.Localizer).ServerPaymentsErrorState, "")
	}))
	base := supporters.PagesPath(Name)
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+base+pageCheckout, h.checkoutPage)
	mux.Handle("POST "+base+pageCheckout, h.cop.Handler(http.HandlerFunc(h.checkout)))
	mux.HandleFunc("GET "+base+pagePortal, h.portalPage)
	mux.Handle("POST "+base+pagePortal, h.cop.Handler(http.HandlerFunc(h.portal)))
	return mux
}

// view is the data of a template.
type view struct {
	L         *i18n.Localizer
	Lang, Dir string
	Title     string
	// Stylesheet is the server pages' stylesheet; Nonce is the nonce of the scripts.
	Stylesheet, Nonce string
	TestMode          bool
	Error, Notice     string
	// Back is where the page's link back goes.
	Back string
	// Form is the card form; nil renders no script.
	Form *cardForm

	// Price is the plan's price on the checkout page.
	Price string
	// Card, Paused, and Charges are the customer's card, whether a subscription is paused, and
	// the recent payments, on the portal page.
	Card    string
	Paused  bool
	Charges []chargeRow
}

type cardForm struct {
	Action, PublicKey, State string
	Email, Name, Submit      string
}

type chargeRow struct {
	Date, Amount, Refunded string
}

func (h *pages) newView(r *http.Request, st *pageState) view {
	var l *i18n.Localizer
	if st != nil && st.Locale != "" {
		l = h.env.Locales.Localizer(r.Context(), st.Locale)
	} else {
		l = h.env.Locales.RequestLocalizer(r)
	}
	v := view{L: l, Lang: l.Locale(), Dir: l.Direction(), TestMode: h.p.mode == supporters.ModeTest}
	if h.env.Stylesheet != nil {
		v.Stylesheet = h.env.Stylesheet()
	}
	return v
}

func (h *pages) render(w http.ResponseWriter, r *http.Request, status int, name string, v view) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	v.Nonce = base64.RawURLEncoding.EncodeToString(b)
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Language", v.Lang)
	hd.Set("Content-Security-Policy", strings.Replace(pagesCSP, "%s", v.Nonce, 1))
	hd.Set("Cache-Control", "no-store")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Add("Vary", "Accept-Language")
	w.WriteHeader(status)
	if err := pageTemplates[name].ExecuteTemplate(w, "layout", v); err != nil {
		h.log().ErrorContext(r.Context(), "render a PAY.JP page", slog.String("page", name), slog.Any("error", err))
	}
}

func (h *pages) log() *slog.Logger {
	if h.env.Logger != nil {
		return h.env.Logger
	}
	return h.p.log
}

// renderError renders the error page with a link back to back ("" for none).
func (h *pages) renderError(w http.ResponseWriter, r *http.Request, status int, message func(*i18n.Localizer) string, back string) {
	v := h.newView(r, nil)
	v.Title, v.Error, v.Back = v.L.ServerPaymentsErrorTitle(), message(v.L), back
	h.render(w, r, status, "error", v)
}

// failed renders the error page of a request of PAY.JP that failed.
func (h *pages) failed(w http.ResponseWriter, r *http.Request, back string, err error) {
	h.log().WarnContext(r.Context(), "a PAY.JP page failed", slog.Any("error", err))
	if supporters.ErrorKindOf(err) == supporters.ErrorNotFound {
		h.renderError(w, r, http.StatusNotFound, (*i18n.Localizer).ServerPaymentsErrorState, back)
		return
	}
	h.renderError(w, r, http.StatusBadGateway, (*i18n.Localizer).ServerPaymentsErrorUnavailable, back)
}

// state reads and verifies the state of a request.
func (h *pages) state(w http.ResponseWriter, r *http.Request, page, s string) (pageState, bool) {
	st, err := h.p.verify(s, page, h.p.o.Now())
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, (*i18n.Localizer).ServerPaymentsErrorState, "")
		return pageState{}, false
	}
	return st, true
}

// cardInput is a card form as the browser posted it.
type cardInput struct {
	Token, Email, Name string
}

// readCardForm parses the form of a POST: its state and its card input, nil when a field is not
// valid.
func readCardForm(w http.ResponseWriter, r *http.Request) (state string, in *cardInput) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPageForm)
	if err := r.ParseForm(); err != nil {
		return "", nil
	}
	c := cardInput{Token: r.PostForm.Get("token"), Email: strings.TrimSpace(r.PostForm.Get("email")), Name: strings.TrimSpace(r.PostForm.Get("name"))}
	if !validToken(c.Token) || !validEmail(c.Email) || !validCardName(c.Name) {
		return r.PostForm.Get("s"), nil
	}
	return r.PostForm.Get("s"), &c
}

func validToken(s string) bool {
	return strings.HasPrefix(s, "tok_") && len(s) <= 100 && !strings.ContainsAny(s, "/?#")
}

func validEmail(s string) bool {
	at := strings.LastIndexByte(s, '@')
	return at > 0 && at < len(s)-1 && len(s) <= 254 && !strings.ContainsAny(s, " \t\r\n")
}

// validCardName reports a card holder's name as PAY.JP takes it: 2 to 45 letters, digits,
// spaces, periods, and hyphens.
func validCardName(s string) bool {
	if len(s) < 2 || len(s) > 45 {
		return false
	}
	for _, c := range s {
		ok := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == ' ' || c == '.' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}

// verifiedToken reads a token and reports whether its card passed 3-D Secure and the token is
// unused.
func (h *pages) verifiedToken(ctx context.Context, id string) (bool, error) {
	var tk token
	err := h.p.get(ctx, "read a token", "/tokens/"+url.PathEscape(id), nil, &tk)
	if errors.Is(err, supporters.ErrPaymentNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !tk.Used && verifiedThreeDSecure[tk.Card.ThreeDSecureStatus], nil
}

// successURL returns the success URL of a checkout with its ref as checkout_id, a key apart from
// the checkout=success of the client API's return URLs.
func successURL(st pageState) string {
	sep := "?"
	if strings.Contains(st.Return, "?") {
		sep = "&"
	}
	return st.Return + sep + "checkout_id=" + url.QueryEscape(string(checkoutRef(st.Customer, st.LocalID, st.Exp)))
}

// checkoutView returns the view of the checkout form of st.
func (h *pages) checkoutView(ctx context.Context, r *http.Request, st pageState, s string, in cardInput) (view, error) {
	pl, err := h.p.getPlan(ctx, supporters.PriceRef(st.Plan))
	if err != nil {
		return view{}, err
	}
	v := h.newView(r, &st)
	v.Title, v.Back = v.L.ServerPaymentsCheckoutTitle(pl.Name), st.Cancel
	if pl.Interval == string(supporters.IntervalYear) {
		v.Price = v.L.ServerPaymentsCheckoutPriceYear(float64(pl.Amount))
	} else {
		v.Price = v.L.ServerPaymentsCheckoutPriceMonth(float64(pl.Amount))
	}
	email := in.Email
	if email == "" {
		email = st.Email
	}
	v.Form = &cardForm{
		Action: supporters.PagesPath(Name) + pageCheckout, PublicKey: h.p.o.PublicKey, State: s,
		Email: email, Name: in.Name, Submit: v.L.ServerPaymentsCheckoutSubmit(),
	}
	return v, nil
}

// checkoutPage shows the checkout form, or goes on to the success URL once the checkout made
// its subscription.
func (h *pages) checkoutPage(w http.ResponseWriter, r *http.Request) {
	s := r.URL.Query().Get("s")
	st, ok := h.state(w, r, pageCheckout, s)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, found, err := h.p.findSubscription(ctx, st.Customer, st.LocalID); err != nil {
		h.failed(w, r, st.Cancel, err)
		return
	} else if found {
		http.Redirect(w, r, successURL(st), http.StatusSeeOther) //nolint:gosec // G710: the URL is from the state that this server signed
		return
	}
	v, err := h.checkoutView(ctx, r, st, s, cardInput{})
	if err != nil {
		h.failed(w, r, st.Cancel, err)
		return
	}
	h.render(w, r, http.StatusOK, pageCheckout, v)
}

// checkout takes the checkout form: it checks the card's 3-D Secure, prepares the customer,
// makes the card the customer's default, and creates the subscription with our ID, once: the
// posts of one checkout run one at a time, in the process and across the server's processes.
func (h *pages) checkout(w http.ResponseWriter, r *http.Request) {
	s, in := readCardForm(w, r)
	st, ok := h.state(w, r, pageCheckout, s)
	if !ok {
		return
	}
	ctx := r.Context()
	again := func(status int, message func(*i18n.Localizer) string, in cardInput) {
		v, err := h.checkoutView(ctx, r, st, s, in)
		if err != nil {
			h.failed(w, r, st.Cancel, err)
			return
		}
		v.Error = message(v.L)
		h.render(w, r, status, pageCheckout, v)
	}
	if in == nil {
		again(http.StatusBadRequest, (*i18n.Localizer).ServerPaymentsErrorForm, cardInput{Email: r.PostForm.Get("email"), Name: r.PostForm.Get("name")})
		return
	}
	// PAY.JP v1 takes no idempotency key, so the subscription is looked for and created under
	// the lock of the checkout in the process and its claim across the processes: a second post
	// waits and then finds the first one's subscription.
	defer h.checkouts.lock(st.LocalID)()
	release, err := h.claimCheckout(ctx, st.LocalID)
	if err != nil {
		h.failed(w, r, st.Cancel, err)
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, supporters.CheckoutCallBudget)
	defer cancel()
	if _, found, err := h.p.findSubscription(ctx, st.Customer, st.LocalID); err != nil {
		h.failed(w, r, st.Cancel, err)
		return
	} else if found {
		http.Redirect(w, r, successURL(st), http.StatusSeeOther)
		return
	}
	switch ok, err := h.verifiedToken(ctx, in.Token); {
	case err != nil:
		h.failed(w, r, st.Cancel, err)
		return
	case !ok:
		again(http.StatusBadRequest, (*i18n.Localizer).ServerPaymentsErrorThreeDSecure, *in)
		return
	}
	cus, err := h.ensureCustomer(ctx, st, in.Email)
	if err != nil {
		h.failed(w, r, st.Cancel, err)
		return
	}
	newCard, err := h.addDefaultCard(ctx, cus.ID, in.Token)
	if err != nil {
		if supporters.ErrorKindOf(err) == supporters.ErrorInvalidRequest {
			again(http.StatusBadRequest, (*i18n.Localizer).ServerPaymentsErrorDeclined, *in)
			return
		}
		h.failed(w, r, st.Cancel, err)
		return
	}
	form := url.Values{"customer": {cus.ID}, "plan": {st.Plan}, "metadata[" + metaSubscriptionID + "]": {st.LocalID}}
	if err := h.p.post(ctx, "create a subscription", "/subscriptions", form, nil); err != nil {
		h.restoreCard(ctx, cus, newCard)
		if supporters.ErrorKindOf(err) == supporters.ErrorInvalidRequest {
			again(http.StatusPaymentRequired, (*i18n.Localizer).ServerPaymentsErrorDeclined, *in)
			return
		}
		h.failed(w, r, st.Cancel, err)
		return
	}
	http.Redirect(w, r, successURL(st), http.StatusSeeOther)
}

// checkoutClaimWait is how long a post of a checkout waits for the claim that another process
// holds, and checkoutClaimPoll how often it tries again meanwhile.
var (
	checkoutClaimWait = 30 * time.Second
	checkoutClaimPoll = 250 * time.Millisecond
)

// errCheckoutBusy reports a checkout whose claim another process held for all of
// checkoutClaimWait.
var errCheckoutBusy = errors.New("payjp: another post of the checkout is running")

// claimCheckout claims the checkout localID across the server's processes (PageEnv.Checkouts),
// waiting up to checkoutClaimWait while another post holds it, and returns its release. Without
// Checkouts it claims nothing.
func (h *pages) claimCheckout(ctx context.Context, localID string) (func(), error) {
	if h.env.Checkouts == nil {
		return func() {}, nil
	}
	deadline := time.NewTimer(checkoutClaimWait)
	defer deadline.Stop()
	for {
		release, claimed, err := h.env.Checkouts.ClaimCheckout(ctx, localID)
		if err != nil {
			return nil, err
		}
		if claimed {
			return release, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errCheckoutBusy
		case <-time.After(checkoutClaimPoll):
		}
	}
}

// ensureCustomer reads the customer of a checkout, creating the one that the checkout names
// with our ID when it is new.
func (h *pages) ensureCustomer(ctx context.Context, st pageState, email string) (customer, error) {
	var cus customer
	err := h.p.get(ctx, "read a customer", "/customers/"+url.PathEscape(st.Customer), nil, &cus)
	if err == nil || !errors.Is(err, supporters.ErrPaymentNotFound) || !st.NewCustomer {
		return cus, err
	}
	form := url.Values{"id": {st.Customer}, "email": {email}, "metadata[" + metaSubscriptionID + "]": {st.LocalID}}
	err = h.p.post(ctx, "create a customer", "/customers", form, &cus)
	return cus, err
}

// addDefaultCard adds the card of a token to a customer as the default.
func (h *pages) addDefaultCard(ctx context.Context, cus, tok string) (card, error) {
	var c card
	err := h.p.post(ctx, "add a card", "/customers/"+url.PathEscape(cus)+"/cards", url.Values{"card": {tok}, "default": {"true"}}, &c)
	return c, err
}

// restoreCard takes a card added for a payment that failed off the customer, giving back the
// default card it had before.
func (h *pages) restoreCard(ctx context.Context, before customer, added card) {
	if before.DefaultCard != "" {
		if err := h.p.post(ctx, "restore the default card", "/customers/"+url.PathEscape(before.ID), url.Values{"default_card": {before.DefaultCard}}, nil); err != nil {
			h.log().WarnContext(ctx, "the default card of a PAY.JP customer could not be restored", slog.Any("error", err))
		}
	}
	if err := h.p.del(ctx, "remove a card", "/customers/"+url.PathEscape(before.ID)+"/cards/"+url.PathEscape(added.ID), nil); err != nil {
		h.log().WarnContext(ctx, "a declined card of a PAY.JP customer could not be removed", slog.Any("error", err))
	}
}

// portalView returns the view of the portal of st.
func (h *pages) portalView(ctx context.Context, r *http.Request, st pageState, s string) (view, error) {
	var cus customer
	if err := h.p.get(ctx, "read a customer", "/customers/"+url.PathEscape(st.Customer), nil, &cus); err != nil {
		return view{}, err
	}
	v := h.newView(r, &st)
	v.Title, v.Back = v.L.ServerPaymentsPortalTitle(), st.Return
	for _, c := range cus.Cards.Data {
		if c.ID == cus.DefaultCard {
			v.Card = v.L.ServerPaymentsPortalCardValue(c.Brand, c.Last4, strconv.Itoa(c.ExpMonth), strconv.Itoa(c.ExpYear))
		}
	}
	subs, err := h.customerSubscriptions(ctx, cus.ID)
	if err != nil {
		return view{}, err
	}
	for _, sub := range subs {
		v.Paused = v.Paused || sub.Status == subPaused
	}
	var charges list[charge]
	if err := h.p.get(ctx, "list the charges of a customer", "/charges", url.Values{"customer": {cus.ID}, "limit": {"20"}}, &charges); err != nil {
		return view{}, err
	}
	for _, c := range charges.Data {
		if !c.Captured {
			continue
		}
		row := chargeRow{Date: unixTime(c.Created).In(tokyo).Format(time.DateOnly), Amount: v.L.ServerPaymentsPortalAmountValue(float64(c.Amount))}
		if c.AmountRefunded > 0 {
			row.Refunded = v.L.ServerPaymentsPortalAmountValue(float64(c.AmountRefunded))
		}
		v.Charges = append(v.Charges, row)
	}
	v.Form = &cardForm{Action: supporters.PagesPath(Name) + pagePortal, PublicKey: h.p.o.PublicKey, State: s, Submit: v.L.ServerPaymentsPortalChangeCard()}
	switch r.URL.Query().Get("done") {
	case doneChanged:
		v.Notice = v.L.ServerPaymentsPortalChanged()
	case doneResumed:
		v.Notice = v.L.ServerPaymentsPortalResumed()
	case doneResumeFailed:
		v.Error = v.L.ServerPaymentsPortalResumeFailed()
	}
	return v, nil
}

// customerSubscriptions reads the subscriptions of a customer.
func (h *pages) customerSubscriptions(ctx context.Context, cus string) ([]subscription, error) {
	var out []subscription
	for offset := 0; ; offset += supporters.PageSize {
		var page list[subscription]
		q := url.Values{"customer": {cus}, "limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)}}
		if err := h.p.get(ctx, "list the subscriptions of a customer", "/subscriptions", q, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Data...)
		if !page.HasMore || len(page.Data) == 0 {
			return out, nil
		}
	}
}

// portalPage shows the customer's card, its recent payments, and the form of a card change.
func (h *pages) portalPage(w http.ResponseWriter, r *http.Request) {
	s := r.URL.Query().Get("s")
	st, ok := h.state(w, r, pagePortal, s)
	if !ok {
		return
	}
	v, err := h.portalView(r.Context(), r, st, s)
	if err != nil {
		h.failed(w, r, st.Return, err)
		return
	}
	h.render(w, r, http.StatusOK, pagePortal, v)
}

// portal takes a card change: the verified card becomes the default and the others are removed,
// and the paused subscriptions resume, which charges them.
func (h *pages) portal(w http.ResponseWriter, r *http.Request) {
	s, in := readCardForm(w, r)
	st, ok := h.state(w, r, pagePortal, s)
	if !ok {
		return
	}
	ctx := r.Context()
	again := func(status int, message func(*i18n.Localizer) string) {
		v, err := h.portalView(ctx, r, st, s)
		if err != nil {
			h.failed(w, r, st.Return, err)
			return
		}
		v.Notice, v.Error = "", message(v.L)
		if in != nil {
			v.Form.Email, v.Form.Name = in.Email, in.Name
		}
		h.render(w, r, status, pagePortal, v)
	}
	if in == nil {
		again(http.StatusBadRequest, (*i18n.Localizer).ServerPaymentsErrorForm)
		return
	}
	switch ok, err := h.verifiedToken(ctx, in.Token); {
	case err != nil:
		h.failed(w, r, st.Return, err)
		return
	case !ok:
		again(http.StatusBadRequest, (*i18n.Localizer).ServerPaymentsErrorThreeDSecure)
		return
	}
	var cus customer
	if err := h.p.get(ctx, "read a customer", "/customers/"+url.PathEscape(st.Customer), nil, &cus); err != nil {
		h.failed(w, r, st.Return, err)
		return
	}
	added, err := h.addDefaultCard(ctx, cus.ID, in.Token)
	if err != nil {
		if supporters.ErrorKindOf(err) == supporters.ErrorInvalidRequest {
			again(http.StatusBadRequest, (*i18n.Localizer).ServerPaymentsErrorDeclined)
			return
		}
		h.failed(w, r, st.Return, err)
		return
	}
	for _, c := range cus.Cards.Data {
		if c.ID != added.ID {
			if err := h.p.del(ctx, "remove a card", "/customers/"+url.PathEscape(cus.ID)+"/cards/"+url.PathEscape(c.ID), nil); err != nil {
				h.log().WarnContext(ctx, "an old card of a PAY.JP customer could not be removed", slog.Any("error", err))
			}
		}
	}
	done := doneChanged
	subs, err := h.customerSubscriptions(ctx, cus.ID)
	if err != nil {
		h.failed(w, r, st.Return, err)
		return
	}
	for _, sub := range subs {
		if sub.Status != subPaused {
			continue
		}
		err := h.p.post(ctx, "resume a subscription", "/subscriptions/"+url.PathEscape(sub.ID)+"/resume", url.Values{}, nil)
		switch {
		case err == nil:
			if done == doneChanged {
				done = doneResumed
			}
		case supporters.ErrorKindOf(err) == supporters.ErrorInvalidRequest:
			done = doneResumeFailed
		default:
			h.failed(w, r, st.Return, err)
			return
		}
	}
	u := url.URL{Path: supporters.PagesPath(Name) + pagePortal, RawQuery: url.Values{"s": {s}, "done": {done}}.Encode()}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}
