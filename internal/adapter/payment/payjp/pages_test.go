// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/i18n"
)

// pageEnv is the provider and its pages on a fake PAY.JP with a plan "p" of 1,200 yen a month.
type pageEnv struct {
	p   *Provider
	f   *fakeAPI
	srv http.Handler
}

func newPageEnv(t *testing.T) *pageEnv {
	t.Helper()
	p, f := newOnFake(t, supporters.ModeTest, unitNow)
	f.addPlan("p", 1200, "month", nil)
	f.plans["p"].Name = "Supporter"
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	h := p.Pages(supporters.PageEnv{
		Locales: &i18n.Resolver{Bundle: b}, Stylesheet: func() string { return "/assets/server.css" },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return &pageEnv{p: p, f: f, srv: h}
}

func (e *pageEnv) checkout(t *testing.T) (supporters.Checkout, string) {
	t.Helper()
	c, err := e.p.StartCheckout(t.Context(), supporters.CheckoutRequest{
		LocalID: "local-1", Price: "p", Email: "alice@example.com", Locale: "ja",
		SuccessURL: "https://my.example/supporter/done", CancelURL: "https://my.example/supporter", Key: "k",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, query(t, c.RedirectURL).Get("s")
}

func (e *pageEnv) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://my.example"+path, nil))
	return rec
}

func (e *pageEnv) post(t *testing.T, page string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "https://my.example"+supporters.PagesPath(Name)+page, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func postedCard(s, tok string) url.Values {
	return url.Values{"s": {s}, "token": {tok}, "email": {"alice@example.com"}, "name": {"ALICE EXAMPLE"}}
}

func (e *pageEnv) subscriptionsOf(cus string) []*fakeSub {
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	var out []*fakeSub
	for _, s := range e.f.subs {
		if s.Customer == cus {
			out = append(out, s)
		}
	}
	return out
}

func TestPagesHeaders(t *testing.T) {
	e := newPageEnv(t)
	c, _ := e.checkout(t)
	u, _ := url.Parse(c.RedirectURL)
	rec := e.get(t, u.RequestURI())
	if rec.Code != http.StatusOK {
		t.Fatalf("checkout page = %d %s", rec.Code, rec.Body)
	}
	h := rec.Header()
	csp := h.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'nonce-", "https://js.pay.jp", "frame-ancestors 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("Content-Security-Policy %q lacks %q", csp, want)
		}
	}
	nonce := strings.TrimSuffix(strings.SplitN(strings.SplitN(csp, "'nonce-", 2)[1], "'", 2)[0], "'")
	body := rec.Body.String()
	if strings.Count(body, `nonce="`+nonce+`"`) != 2 {
		t.Errorf("the page's scripts do not carry the nonce %q of its policy", nonce)
	}
	for k, want := range map[string]string{"Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff", "Content-Language": "ja"} {
		if got := h.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	for _, want := range []string{"Supporter", "月額 1,200 円", `data-public-key="pk_test_public"`, "テストのモード", `value="alice@example.com"`, "/assets/server.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("checkout page lacks %q", want)
		}
	}
	for _, field := range []string{`name="number"`, `name="cvc"`, `name="exp`} {
		if strings.Contains(body, field) {
			t.Errorf("checkout page has a card field %s", field)
		}
	}
}

func TestPagesRefuseBadState(t *testing.T) {
	e := newPageEnv(t)
	_, s := e.checkout(t)
	expired, _ := New(Options{
		SecretKey: "sk_test_secret", PublicKey: "pk_test_public", WebhookToken: testToken,
		Now: func() time.Time { return unitNow.Add(-3 * time.Hour) },
	})
	old := expired.sign(pageState{Page: pageCheckout, LocalID: "local-1", Plan: "p", Customer: "my_local-1", Exp: unitNow.Add(-time.Minute).Unix()})
	for name, path := range map[string]string{
		"none":     supporters.PagesPath(Name) + pageCheckout,
		"altered":  supporters.PagesPath(Name) + pageCheckout + "?s=" + url.QueryEscape(s+"x"),
		"expired":  supporters.PagesPath(Name) + pageCheckout + "?s=" + url.QueryEscape(old),
		"of other": supporters.PagesPath(Name) + pagePortal + "?s=" + url.QueryEscape(s),
	} {
		if rec := e.get(t, path); rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "card-form") {
			t.Errorf("%s state: %d, want 400 without the form", name, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "https://my.example"+supporters.PagesPath(Name)+pageCheckout, strings.NewReader(postedCard(s, "tok_1").Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site POST = %d, want 403", rec.Code)
	}
}

func TestCheckoutSubscribesOnce(t *testing.T) {
	e := newPageEnv(t)
	c, s := e.checkout(t)
	e.f.addToken("tok_ok", "4242", "verified")
	rec := e.post(t, pageCheckout, postedCard(s, "tok_ok"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("checkout = %d %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "https://my.example/supporter/done?checkout_id="+url.QueryEscape(string(c.Ref)) {
		t.Errorf("Location = %q, want the success URL with the checkout", loc)
	}
	subs := e.subscriptionsOf("my_local-1")
	if len(subs) != 1 || subs[0].Metadata[metaSubscriptionID] != "local-1" || subs[0].Plan.ID != "p" {
		t.Fatalf("subscriptions = %+v, want one of local-1 on p", subs)
	}
	if got, err := e.p.GetCheckout(t.Context(), c.Ref); err != nil || got.Status != supporters.CheckoutCompleted || got.Subscription != supporters.SubscriptionRef(subs[0].ID) {
		t.Errorf("GetCheckout = %+v, %v, want completed", got, err)
	}
	// The browser posting again, or coming back to the page, goes on without a second one.
	if rec := e.post(t, pageCheckout, postedCard(s, "tok_ok")); rec.Code != http.StatusSeeOther {
		t.Errorf("second POST = %d, want 303", rec.Code)
	}
	u, _ := url.Parse(c.RedirectURL)
	if rec := e.get(t, u.RequestURI()); rec.Code != http.StatusSeeOther {
		t.Errorf("page after the checkout = %d, want 303", rec.Code)
	}
	if n := len(e.subscriptionsOf("my_local-1")); n != 1 {
		t.Errorf("%d subscriptions after the repeats, want 1", n)
	}
}

// findBarrier holds the lookups of the subscriptions of a customer until two arrive or a short
// wait passes, so that two checkouts at the same time both look before either creates.
type findBarrier struct {
	base    http.RoundTripper
	arrived chan struct{}
	once    sync.Once
	release chan struct{}
}

func (b *findBarrier) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodGet && r.URL.Path == "/subscriptions" && r.URL.Query().Get("customer") != "" {
		select {
		case b.arrived <- struct{}{}:
		default:
			b.once.Do(func() { close(b.release) })
		}
		select {
		case <-b.release:
		case <-time.After(200 * time.Millisecond):
		}
	}
	return b.base.RoundTrip(r)
}

func TestCheckoutAtTheSameTimeSubscribesOnce(t *testing.T) {
	e := newPageEnv(t)
	_, s := e.checkout(t)
	e.f.addToken("tok_ok", "4242", "verified")
	base := e.p.o.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	e.p.o.HTTPClient = &http.Client{Transport: &findBarrier{base: base, arrived: make(chan struct{}, 1), release: make(chan struct{})}}
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range codes {
		wg.Go(func() { codes[i] = e.post(t, pageCheckout, postedCard(s, "tok_ok")).Code })
	}
	wg.Wait()
	for _, c := range codes {
		if c != http.StatusSeeOther {
			t.Errorf("checkout = %d, want 303", c)
		}
	}
	if n := len(e.subscriptionsOf("my_local-1")); n != 1 {
		t.Errorf("%d subscriptions after two checkouts at the same time, want 1", n)
	}
}

func TestCheckoutRefusals(t *testing.T) {
	e := newPageEnv(t)
	_, s := e.checkout(t)
	e.f.addToken("tok_unverified", "4242", "unverified")
	e.f.addToken("tok_declined", declinedLast4, "verified")
	for _, tc := range []struct {
		name   string
		form   url.Values
		status int
		says   string
	}{
		{"no token", postedCard(s, ""), http.StatusBadRequest, "メールアドレス・カードの名義・カードを確かめてください"},
		{"bad name", url.Values{"s": {s}, "token": {"tok_x"}, "email": {"alice@example.com"}, "name": {"アリス"}}, http.StatusBadRequest, "カードの名義"},
		{"3-D Secure", postedCard(s, "tok_unverified"), http.StatusBadRequest, "3D セキュア"},
		{"unknown token", postedCard(s, "tok_unknown"), http.StatusBadRequest, "3D セキュア"},
		{"declined", postedCard(s, "tok_declined"), http.StatusPaymentRequired, "別のカード"},
	} {
		rec := e.post(t, pageCheckout, tc.form)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.says) || !strings.Contains(rec.Body.String(), `role="alert"`) {
			t.Errorf("%s: %d, want %d saying %q", tc.name, rec.Code, tc.status, tc.says)
		}
	}
	if n := len(e.subscriptionsOf("my_local-1")); n != 0 {
		t.Errorf("%d subscriptions after the refusals, want 0", n)
	}
	if c := e.f.customers["my_local-1"]; c == nil || len(c.Cards.Data) != 0 || c.DefaultCard != "" {
		t.Errorf("customer after a declined card = %+v, want no card left", c)
	}
}

func TestPortalChangesTheCardAndResumes(t *testing.T) {
	e := newPageEnv(t)
	e.f.addToken("tok_old", declinedLast4, "verified")
	e.f.addToken("tok_new", "4242", "verified")
	e.f.addSub(subscription{ID: "sub_1", Customer: "cus_1", Plan: e.f.plans["p"].plan, Status: subPaused, PausedAt: unitNow.Add(-24 * time.Hour).Unix()}, unitNow.Unix())
	e.f.customers["cus_1"].Cards.Data = []card{{ID: "car_old", Brand: "Visa", Last4: declinedLast4, ExpMonth: 1, ExpYear: 2027}}
	e.f.customers["cus_1"].DefaultCard = "car_old"
	e.f.addCharge(charge{ID: "ch_1", Customer: "cus_1", Amount: 1200, AmountRefunded: 200, Captured: true, Created: unitNow.Add(-40 * 24 * time.Hour).Unix(), FeeRate: "3.00", Subscription: "sub_1"}, "")
	portal, err := e.p.PortalURL(t.Context(), "cus_1", "https://my.example/supporter")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(portal)
	req := httptest.NewRequest(http.MethodGet, "https://my.example"+u.RequestURI(), nil)
	req.Header.Set("Accept-Language", "ja")
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"Visa（末尾 0002、有効期限 1/2027）", "加入が止まっています", "1,200 円", "200 円", "2026-09-12", "card-form"} {
		if !strings.Contains(body, want) {
			t.Errorf("portal page lacks %q", want)
		}
	}
	rec = e.post(t, pagePortal, postedCard(u.Query().Get("s"), "tok_new"))
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "done=resumed") {
		t.Fatalf("card change = %d %s, want 303 to the portal resumed", rec.Code, rec.Header().Get("Location"))
	}
	c := e.f.customers["cus_1"]
	if len(c.Cards.Data) != 1 || c.Cards.Data[0].Last4 != "4242" || c.DefaultCard != c.Cards.Data[0].ID {
		t.Errorf("cards after the change = %+v (default %s), want only the new one", c.Cards.Data, c.DefaultCard)
	}
	if s := e.f.subs["sub_1"]; s.Status != subActive {
		t.Errorf("subscription after the change = %s, want active", s.Status)
	}
}
