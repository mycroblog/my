// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeAPI is an httptest server that answers the parts of Stripe's REST API that the adapter
// calls, in the shapes of Stripe's documentation for the API version of the SDK
// (2026-09-30.endive): form-encoded requests, objects whose expandable fields are IDs unless
// expanded, lists newest first with starting_after and ending_before, idempotency keys that
// replay the first answer and refuse other parameters, and the error object.
type fakeAPI struct {
	t   *testing.T
	srv *httptest.Server
	now func() time.Time

	mu       sync.Mutex
	seq      int
	timezone string
	// failure answers every request with this status (0: none).
	failure int
	// before sees each request before it is answered, holding the lock; a status other than 0
	// fails the request as one that never reached the API, so its key is not kept.
	before func(r *http.Request) int
	// throttle answers the next requests with 429 and Retry-After.
	throttle   int
	retryAfter string
	requests   []string
	idem       map[string]idemEntry
	products   map[string]obj
	prices     []obj
	subs       []*fakeSub
	schedules  map[string]*fakeSchedule
	checkouts  map[string]obj
	// sessionPrices are the prices of the checkouts that the API created, by session.
	sessionPrices map[string]string
	portalConfigs []obj
	bts           []*fakeBT
	charges       map[string]obj
	refunds       []obj
	disputes      map[string]obj
	invoices      map[string]obj
	invoicePays   []obj
	intents       map[string]obj
	events        []obj
	// checkoutURL replaces the URL of the checkouts it creates, to test a tampered answer.
	checkoutURL string
	// storeFailure makes the next update of a price end in a 500, which is kept under its key.
	storeFailure bool
	// pastDueOnFailure marks a subscription past due while its update's payment fails, which
	// Stripe's documentation does not rule out.
	pastDueOnFailure bool
	changeCharges    int
}

// obj is a Stripe object. A ref value is an expandable field.
type obj = map[string]any

// ref is an expandable field: its ID unless the request expands its path.
type ref struct {
	id  string
	get func() obj
}

type idemEntry struct {
	fingerprint string
	status      int
	body        []byte
}

type fakeSub struct {
	o        obj // id, customer, metadata, status, cancel_at_period_end, cancel_at, pending_update, items
	schedule string
	invoice  obj
	// payment is what the customer's card does with the invoice of an update (fakeCard*).
	payment string
}

// What a card does with the invoice of an update: decline it, ask for an authentication, leave it
// processing, or refuse the request itself with a 402 and no invoice.
const (
	fakeCardDeclines     = "requires_payment_method"
	fakeCardAuthenticate = "requires_action"
	fakeCardProcesses    = "processing"
	fakeCardRefuses      = "402"
)

type fakeSchedule struct {
	id       string
	sub      string
	status   string
	phases   []fakePhase
	behavior string
}

type fakePhase struct {
	start, end int64
	price      string
}

type fakeBT struct {
	o       obj
	source  string
	created int64
	seq     int
}

func newFakeAPI(t *testing.T, now func() time.Time) *fakeAPI {
	f := &fakeAPI{
		t: t, now: now, timezone: "UTC", retryAfter: "1",
		idem: map[string]idemEntry{}, products: map[string]obj{}, schedules: map[string]*fakeSchedule{},
		checkouts: map[string]obj{}, sessionPrices: map[string]string{}, charges: map[string]obj{}, disputes: map[string]obj{}, invoices: map[string]obj{},
		intents: map[string]obj{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) nextID(prefix string) string {
	f.seq++
	return fmt.Sprintf("%s_fake%04d", prefix, f.seq)
}

// load fills the fake with the account of a scenario.
func (f *fakeAPI) load(s testkit.PaymentScenario) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timezone = s.Location.String()
	f.failure = map[error]int{
		supporters.ErrPaymentPermission: http.StatusUnauthorized, supporters.ErrPaymentThrottled: http.StatusTooManyRequests,
		supporters.ErrPaymentUnavailable: http.StatusInternalServerError,
	}[s.Failure]
	for _, snap := range s.Subscriptions {
		f.putSubscription(snap)
	}
	for _, c := range s.Checkouts {
		status := map[supporters.CheckoutStatus]string{
			supporters.CheckoutOpen: "open", supporters.CheckoutCompleted: "complete", supporters.CheckoutExpired: "expired",
		}[c.Status]
		f.checkouts[string(c.Ref)] = obj{
			"id": string(c.Ref), "object": "checkout.session", "mode": "subscription", "status": status,
			"client_reference_id": c.LocalID, "metadata": obj{metaSubscriptionID: c.LocalID},
			"customer": nullable(string(c.Customer)), "subscription": nullable(string(c.Subscription)),
			"url": nil, "expires_at": s.Now.Add(-time.Hour).Unix(),
		}
	}
	objects := map[string]string{}
	for _, m := range s.Money {
		objects[m.ID] = cmp.Or(m.Object, m.ID)
	}
	for i, m := range s.Money {
		f.putMoney(s, i, m, objects)
	}
	for _, n := range s.Changes {
		f.events = append(f.events, f.event(n, false, n.OccurredAt))
	}
}

func nullable(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// putSubscription adds a subscription in the state of a snapshot.
func (f *fakeAPI) putSubscription(snap supporters.SubscriptionSnapshot) {
	id := string(snap.Ref)
	meta := obj{}
	if snap.LocalID != "" {
		meta[metaSubscriptionID] = snap.LocalID
	}
	sub := &fakeSub{o: obj{
		"id": id, "object": "subscription", "customer": string(snap.Customer), "metadata": meta, "status": string(snap.Status),
		"cancel_at_period_end": snap.CancelAtPeriodEnd, "cancel_at": nil, "livemode": false,
		"items": obj{"object": "list", "has_more": false, "data": []any{obj{
			"id": "si_" + id, "object": "subscription_item", "price": string(snap.Price), "quantity": 1,
			"current_period_start": snap.PeriodStart.Unix(), "current_period_end": snap.PeriodEnd.Unix(),
		}}},
	}}
	inv := obj{"id": "in_latest_" + id, "object": "invoice", "status": "paid", "attempted": true, "attempt_count": 1,
		"status_transitions": obj{"finalized_at": snap.PeriodStart.Unix(), "paid_at": snap.PeriodStart.Unix()}}
	if !snap.LatestPaymentFailed.IsZero() {
		inv = obj{"id": "in_latest_" + id, "object": "invoice", "status": "open", "attempted": true, "attempt_count": 2,
			"status_transitions": obj{"finalized_at": snap.LatestPaymentFailed.Unix()}}
	}
	sub.invoice = inv
	if snap.CancelAtPeriodEnd {
		sub.o["cancel_at"] = snap.PeriodEnd.Unix()
	}
	if snap.ScheduledPrice != "" {
		sch := &fakeSchedule{id: "sub_sched_" + id, sub: id, status: "active", behavior: "release", phases: []fakePhase{
			{start: snap.PeriodStart.Unix(), end: snap.PeriodEnd.Unix(), price: string(snap.Price)},
			{start: snap.PeriodEnd.Unix(), end: snap.PeriodEnd.AddDate(0, 1, 0).Unix(), price: string(snap.ScheduledPrice)},
		}}
		f.schedules[sch.id] = sch
		sub.schedule = sch.id
	}
	f.subs = append(f.subs, sub)
}

// putMoney adds a movement as Stripe books it: a balance transaction whose source is a charge
// (with its payment intent, invoice, and invoice payment), a refund, a dispute, or, for a fee,
// nothing that expands. A pending payment is a charge without a balance transaction yet.
func (f *fakeAPI) putMoney(s testkit.PaymentScenario, i int, m testkit.PaymentScenarioMoney, objects map[string]string) {
	object := objects[m.ID]
	cur := strings.ToLower(m.Currency)
	btID := m.ID
	pi := ""
	switch m.Kind {
	case supporters.MoneyPayment:
		pi = "pi_" + object
		f.charges[object] = obj{
			"id": object, "object": "charge", "amount": m.GrossMinor, "currency": cur, "status": "succeeded",
			"balance_transaction": ref{id: btID, get: f.btGetter(btID)}, "payment_intent": pi,
		}
		if m.Pending {
			f.charges[object]["balance_transaction"] = nil
			f.charges[object]["status"] = "pending"
		}
		f.intents[pi] = obj{"id": pi, "object": "payment_intent", "latest_charge": object, "status": "succeeded"}
		if m.Subscription != "" {
			invID := "in_" + object
			line := obj{"id": "il_" + object, "object": "line_item", "amount": m.GrossMinor, "currency": cur}
			if !m.Covers.IsZero() {
				start := time.Date(m.Covers.Start.Year, m.Covers.Start.Month, m.Covers.Start.Day, 0, 0, 0, 0, s.Location)
				end := time.Date(m.Covers.End.Year, m.Covers.End.Month, m.Covers.End.Day, 0, 0, 0, 0, s.Location).AddDate(0, 0, 1)
				line["period"] = obj{"start": start.Unix(), "end": end.Unix()}
			}
			f.invoices[invID] = obj{
				"id": invID, "object": "invoice", "status": "paid",
				"parent":   obj{"type": "subscription_details", "subscription_details": obj{"subscription": string(m.Subscription)}},
				"lines":    obj{"object": "list", "has_more": false, "data": []any{line}},
				"payments": nil,
			}
			f.invoicePays = append(f.invoicePays, obj{
				"id": "inpay_" + object, "object": "invoice_payment", "status": "paid", "invoice": ref{id: invID, get: f.getter(f.invoices, invID)},
				"payment": obj{"type": "payment_intent", "payment_intent": ref{id: pi, get: f.getter(f.intents, pi)}},
			})
		}
	case supporters.MoneyRefund:
		parent := objects[m.ParentID]
		f.refunds = append(f.refunds, obj{
			"id": object, "object": "refund", "amount": -m.GrossMinor, "currency": cur, "status": "succeeded",
			"charge": ref{id: parent, get: f.getter(f.charges, parent)}, "payment_intent": "pi_" + parent,
			"balance_transaction": btID,
		})
	case supporters.MoneyChargeback, supporters.MoneyChargebackReversal:
		parent := objects[m.ParentID]
		f.disputes[object] = obj{
			"id": object, "object": "dispute", "amount": -m.GrossMinor, "currency": cur, "status": "needs_response",
			"charge": ref{id: parent, get: f.getter(f.charges, parent)}, "payment_intent": "pi_" + parent,
		}
	}
	if m.Pending {
		return
	}
	ct := map[supporters.MoneyKind][2]string{
		supporters.MoneyPayment: {"charge", "charge"}, supporters.MoneyRefund: {"refund", "refund"},
		supporters.MoneyChargeback: {"dispute", "adjustment"}, supporters.MoneyChargebackReversal: {"dispute_reversal", "adjustment"},
		supporters.MoneyFee: {"fee", "stripe_fee"},
	}[m.Kind]
	category, typ := ct[0], ct[1]
	var source any
	switch m.Kind {
	case supporters.MoneyPayment:
		source = ref{id: object, get: f.getter(f.charges, object)}
	case supporters.MoneyRefund:
		source = ref{id: object, get: func() obj { return f.find(f.refunds, object) }}
	case supporters.MoneyChargeback, supporters.MoneyChargebackReversal:
		source = ref{id: object, get: f.getter(f.disputes, object)}
	}
	f.bts = append(f.bts, &fakeBT{
		source: object, created: m.OccurredAt.Unix(), seq: i,
		o: obj{
			"id": btID, "object": "balance_transaction", "amount": m.GrossMinor, "fee": m.FeeMinor, "net": m.GrossMinor - m.FeeMinor,
			"currency": cur, "created": m.OccurredAt.Unix(), "reporting_category": category, "type": typ, "status": "available",
			"source": source,
		},
	})
}

func (f *fakeAPI) getter(m map[string]obj, id string) func() obj { return func() obj { return m[id] } }

func (f *fakeAPI) btGetter(id string) func() obj {
	return func() obj {
		for _, b := range f.bts {
			if b.o["id"] == id {
				return b.o
			}
		}
		return nil
	}
}

func (f *fakeAPI) find(list []obj, id string) obj {
	for _, o := range list {
		if o["id"] == id {
			return o
		}
	}
	return nil
}

// eventType is the type of Stripe's event of a notice's kind.
func eventType(k supporters.NoticeKind) (typ, object string) {
	switch k {
	case supporters.NoticeSubscription:
		return "customer.subscription.updated", "subscription"
	case supporters.NoticeCheckout:
		return "checkout.session.completed", "checkout.session"
	case supporters.NoticeMoney:
		return "charge.refunded", "charge"
	}
	return "customer.updated", "customer"
}

func (f *fakeAPI) event(n supporters.Notice, live bool, created time.Time) obj {
	typ, object := eventType(n.Kind)
	data := obj{"id": n.ObjectID, "object": object}
	if object == "subscription" {
		data["status"] = "active"
	}
	return obj{
		"id": n.EventID, "object": "event", "type": typ, "livemode": live, "created": created.Unix(),
		"api_version": "2019-12-03", "pending_webhooks": 1, "data": obj{"object": data},
	}
}

// deliver signs a webhook delivery of n as Stripe does (testkit.PaymentDeliver).
func (f *fakeAPI) deliver(secret string) testkit.PaymentDeliver {
	return func(n supporters.Notice, live bool, at time.Time, keys ...string) (http.Header, []byte) {
		created := n.OccurredAt
		if created.IsZero() {
			created = at
		}
		body, err := json.Marshal(f.event(n, live, created))
		if err != nil {
			panic(err)
		}
		return signedHeader(body, at, secret, keys...), body
	}
}

// signedHeader returns the Stripe-Signature header of body signed at at, one v1 per key ("" is
// secret).
func signedHeader(body []byte, at time.Time, secret string, keys ...string) http.Header {
	if len(keys) == 0 {
		keys = []string{""}
	}
	t := strconv.FormatInt(at.Unix(), 10)
	parts := []string{"t=" + t}
	for _, k := range keys {
		mac := hmac.New(sha256.New, []byte(cmp.Or(k, secret)))
		mac.Write([]byte(t + "."))
		mac.Write(body)
		parts = append(parts, "v1="+hex.EncodeToString(mac.Sum(nil)), "v0=ignored")
	}
	h := http.Header{}
	h.Set(SignatureHeader, strings.Join(parts, ","))
	return h
}

// serve answers one request.
func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := r.ParseForm(); err != nil {
		f.fail(w, http.StatusBadRequest, "invalid_request_error", "", err.Error())
		return
	}
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer sk_") && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer rk_") {
		f.fail(w, http.StatusUnauthorized, "invalid_request_error", "", "no key")
		return
	}
	if f.throttle > 0 {
		f.throttle--
		w.Header().Set("Retry-After", f.retryAfter)
		f.fail(w, http.StatusTooManyRequests, "invalid_request_error", "rate_limit", "too many requests")
		return
	}
	if f.failure != 0 {
		typ := "api_error"
		if f.failure < 500 {
			typ = "invalid_request_error"
		}
		f.fail(w, f.failure, typ, "", "failing")
		return
	}
	if f.before != nil {
		if status := f.before(r); status != 0 {
			f.fail(w, status, "api_error", "", "failing")
			return
		}
	}
	key := r.Header.Get("Idempotency-Key")
	if r.Method != http.MethodGet && key != "" {
		fp := r.Method + " " + r.URL.Path + " " + r.PostForm.Encode()
		if e, ok := f.idem[key]; ok {
			if e.fingerprint != fp {
				f.fail(w, http.StatusBadRequest, "idempotency_error", "", "Keys for idempotent requests can only be used with the same parameters they were first used with.")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(e.status)
			_, _ = w.Write(e.body)
			return
		}
		rec := httptest.NewRecorder()
		f.route(rec, r)
		f.idem[key] = idemEntry{fingerprint: fp, status: rec.Code, body: rec.Body.Bytes()}
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}
	f.route(w, r)
}

func (f *fakeAPI) fail(w http.ResponseWriter, status int, typ, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Request-Id", "req_fake")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(obj{"error": obj{"type": typ, "code": code, "message": msg}})
}

func (f *fakeAPI) missing(w http.ResponseWriter, what string) {
	f.fail(w, http.StatusNotFound, "invalid_request_error", "resource_missing", "No such "+what)
}

func (f *fakeAPI) bad(w http.ResponseWriter, msg string) {
	f.fail(w, http.StatusBadRequest, "invalid_request_error", "parameter_invalid", msg)
}

// respond writes o with the expansions of the request.
func (f *fakeAPI) respond(w http.ResponseWriter, r *http.Request, o obj) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(render(o, "", expansions(r.Form)))
}

// expansions returns the paths of expand[N].
func expansions(form url.Values) map[string]bool {
	out := map[string]bool{}
	for _, v := range formList(form, "expand") {
		out[v] = true
	}
	return out
}

var indexed = regexp.MustCompile(`^([a-z_]+)\[(\d*)\]$`)

// formList returns the values of name[0], name[1], ... (or name[]) in order.
func formList(form url.Values, name string) []string {
	type kv struct {
		i int
		v string
	}
	var vals []kv
	for k, vs := range form {
		m := indexed.FindStringSubmatch(k)
		if m == nil || m[1] != name {
			continue
		}
		i, _ := strconv.Atoi(m[2])
		for _, v := range vs {
			vals = append(vals, kv{i, v})
		}
	}
	sort.SliceStable(vals, func(a, b int) bool { return vals[a].i < vals[b].i })
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = v.v
	}
	return out
}

// render returns v with each ref expanded when its path is in exp, as its ID otherwise.
func render(v any, path string, exp map[string]bool) any {
	join := func(k string) string {
		if path == "" {
			return k
		}
		return path + "." + k
	}
	switch x := v.(type) {
	case ref:
		if exp[path] && x.get != nil {
			if o := x.get(); o != nil {
				return render(o, path, exp)
			}
		}
		return x.id
	case obj:
		out := obj{}
		for k, e := range x {
			out[k] = render(e, join(k), exp)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = render(e, path, exp)
		}
		return out
	}
	return v
}

// page returns the items of a list (newest first) after starting_after or before
// ending_before, at most limit, and whether more follow in that direction.
func page[T any](r *http.Request, items []T, id func(T) string) ([]T, bool) {
	limit := 10
	if l, err := strconv.Atoi(r.Form.Get("limit")); err == nil && l > 0 {
		limit = min(l, 100)
	}
	if before := r.Form.Get("ending_before"); before != "" {
		i := slices.IndexFunc(items, func(x T) bool { return id(x) == before })
		if i < 0 {
			return nil, false
		}
		from := max(0, i-limit)
		return items[from:i], from > 0
	}
	if after := r.Form.Get("starting_after"); after != "" {
		i := slices.IndexFunc(items, func(x T) bool { return id(x) == after })
		if i < 0 {
			return nil, false
		}
		items = items[i+1:]
	}
	if len(items) > limit {
		return items[:limit], true
	}
	return items, false
}

func (f *fakeAPI) list(w http.ResponseWriter, r *http.Request, data []any, more bool) {
	exp := expansions(r.Form)
	out := make([]any, len(data))
	for i, d := range data {
		out[i] = render(d, "data", exp)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(obj{"object": "list", "url": r.URL.Path, "has_more": more, "data": out})
}

func toAny[T any](xs []T, f func(T) any) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

// route dispatches a request.
func (f *fakeAPI) route(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/"), "/")
	m := r.Method
	switch {
	case m == "GET" && r.URL.Path == "/v1/account":
		f.respond(w, r, obj{"id": "acct_fake", "object": "account", "settings": obj{"dashboard": obj{"display_name": "my", "timezone": f.timezone}}})
	case parts[0] == "products":
		f.serveProducts(w, r, parts)
	case parts[0] == "prices":
		f.servePrices(w, r, parts)
	case parts[0] == "checkout":
		f.serveCheckouts(w, r, parts)
	case parts[0] == "billing_portal":
		f.portal(w, r, parts)
	case parts[0] == "subscriptions":
		f.subscriptions(w, r, parts)
	case parts[0] == "subscription_schedules":
		f.serveSchedules(w, r, parts)
	case parts[0] == "balance_transactions":
		f.balance(w, r, parts)
	case m == "GET" && parts[0] == "charges" && len(parts) == 2:
		if c, ok := f.charges[parts[1]]; ok {
			f.respond(w, r, c)
		} else {
			f.missing(w, "charge")
		}
	case m == "GET" && parts[0] == "refunds" && len(parts) == 1:
		var out []any
		for _, re := range f.refunds {
			if c, _ := re["charge"].(ref); c.id == r.Form.Get("charge") {
				out = append(out, re)
			}
		}
		f.list(w, r, out, false)
	case m == "POST" && parts[0] == "refunds" && len(parts) == 1:
		f.createRefund(w, r)
	case m == "GET" && parts[0] == "invoices" && len(parts) == 1:
		var out []any
		for _, id := range slices.Sorted(maps.Keys(f.invoices)) {
			inv := f.invoices[id]
			if inv["status"] == r.Form.Get("status") && inv["my_subscription"] == r.Form.Get("subscription") {
				out = append(out, inv)
			}
		}
		f.list(w, r, out, false)
	case m == "GET" && parts[0] == "invoices" && len(parts) == 2:
		if inv, ok := f.invoices[parts[1]]; ok {
			f.respond(w, r, inv)
		} else {
			f.missing(w, "invoice")
		}
	case m == "POST" && parts[0] == "invoices" && len(parts) == 3 && parts[2] == "void":
		f.voidInvoice(w, r, parts[1])
	case m == "GET" && parts[0] == "invoice_payments" && len(parts) == 1:
		var out []any
		for _, ip := range f.invoicePays {
			inv, _ := ip["invoice"].(ref)
			pi, _ := ip["payment"].(obj)["payment_intent"].(ref)
			if (r.Form.Get("invoice") == "" || inv.id == r.Form.Get("invoice")) &&
				(r.Form.Get("payment[payment_intent]") == "" || pi.id == r.Form.Get("payment[payment_intent]")) {
				out = append(out, ip)
			}
		}
		f.list(w, r, out, false)
	case m == "GET" && parts[0] == "events" && len(parts) == 1:
		f.listEvents(w, r)
	default:
		f.fail(w, http.StatusNotFound, "invalid_request_error", "", "Unrecognized request URL "+m+" "+r.URL.Path)
	}
}

// paySession completes an open session as the hosted page does: a subscription that names our ID,
// with its paid invoice and charge. It returns the subscription's ID and the charge's.
func (f *fakeAPI) paySession(t *testing.T, id string) (sub, charge string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.checkouts[id]
	if c == nil || c["status"] != "open" {
		t.Fatalf("session %s is %v, want open", id, c["status"])
	}
	local, _ := c["client_reference_id"].(string)
	price := f.sessionPrices[id]
	sub, charge = f.nextID("sub"), f.nextID("ch")
	now := f.now()
	f.putSubscription(supporters.SubscriptionSnapshot{
		Ref: supporters.SubscriptionRef(sub), Customer: supporters.CustomerRef("cus_" + id), LocalID: local, Price: supporters.PriceRef(price),
		Status: supporters.StatusActive, PeriodStart: now, PeriodEnd: now.Add(30 * 24 * time.Hour),
	})
	inv := f.nextID("in")
	f.invoices[inv] = obj{"id": inv, "object": "invoice", "status": "paid", "my_subscription": sub}
	f.charges[charge] = obj{"id": charge, "object": "charge"}
	f.invoicePays = append(f.invoicePays, obj{
		"id": "inpay_" + inv, "object": "invoice_payment", "status": "paid", "invoice": ref{id: inv},
		"payment": obj{"type": "charge", "charge": ref{id: charge}},
	})
	c["status"], c["url"], c["subscription"], c["customer"] = "complete", nil, sub, "cus_"+id
	return sub, charge
}

// createRefund refunds what is left of a charge, and refuses one with nothing left or in a dispute,
// in the shape of Stripe's errors.
func (f *fakeAPI) createRefund(w http.ResponseWriter, r *http.Request) {
	id := r.Form.Get("charge")
	c, ok := f.charges[id]
	switch {
	case !ok:
		f.missing(w, "charge")
		return
	case r.Form.Get("reason") != "duplicate":
		f.bad(w, "the reason of a refund of a second subscription is duplicate")
		return
	case c["refunded"] == true:
		f.fail(w, http.StatusBadRequest, "invalid_request_error", "charge_already_refunded", "Charge "+id+" has already been refunded.")
		return
	case c["disputed"] == true:
		f.fail(w, http.StatusBadRequest, "invalid_request_error", "charge_disputed", "Charge "+id+" has been charged back.")
		return
	}
	c["refunded"] = true
	re := obj{"id": f.nextID("re"), "object": "refund", "charge": ref{id: id}, "status": "succeeded"}
	f.refunds = append(f.refunds, re)
	f.respond(w, r, re)
}

func (f *fakeAPI) serveProducts(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case r.Method == "GET" && len(parts) == 2:
		if p, ok := f.products[parts[1]]; ok {
			f.respond(w, r, p)
		} else {
			f.missing(w, "product")
		}
	case r.Method == "POST" && len(parts) == 1:
		id := cmp.Or(r.Form.Get("id"), f.nextID("prod"))
		if _, ok := f.products[id]; ok {
			f.fail(w, http.StatusBadRequest, "invalid_request_error", "resource_already_exists", "Product already exists.")
			return
		}
		if r.Form.Get("name") == "" {
			f.bad(w, "Missing required param: name.")
			return
		}
		f.products[id] = obj{"id": id, "object": "product", "active": true, "name": r.Form.Get("name"),
			"description": nullable(r.Form.Get("description")), "metadata": metadata(r.Form)}
		f.respond(w, r, f.products[id])
	case r.Method == "POST" && len(parts) == 2:
		p, ok := f.products[parts[1]]
		if !ok {
			f.missing(w, "product")
			return
		}
		for _, k := range []string{"name", "description"} {
			if _, ok := r.Form[k]; ok {
				p[k] = nullable(r.Form.Get(k))
			}
		}
		if v := r.Form.Get("active"); v != "" {
			p["active"] = v == "true"
		}
		for k, v := range metadata(r.Form) {
			p["metadata"].(obj)[k] = v
		}
		f.respond(w, r, p)
	default:
		f.missing(w, "route")
	}
}

func metadata(form url.Values) obj {
	out := obj{}
	for k, vs := range form {
		if strings.HasPrefix(k, "metadata[") && strings.HasSuffix(k, "]") {
			out[strings.TrimSuffix(strings.TrimPrefix(k, "metadata["), "]")] = vs[0]
		}
	}
	return out
}

func (f *fakeAPI) price(id string) obj {
	for _, p := range f.prices {
		if p["id"] == id {
			return p
		}
	}
	return nil
}

func (f *fakeAPI) servePrices(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case r.Method == "GET" && len(parts) == 1:
		keys := formList(r.Form, "lookup_keys")
		var out []obj
		for i := len(f.prices) - 1; i >= 0; i-- {
			p := f.prices[i]
			if len(keys) > 0 && !slices.Contains(keys, fmt.Sprint(p["lookup_key"])) {
				continue
			}
			if prod := r.Form.Get("product"); prod != "" && p["product"].(ref).id != prod {
				continue
			}
			if a := r.Form.Get("active"); a != "" && p["active"] != (a == "true") {
				continue
			}
			out = append(out, p)
		}
		items, more := page(r, out, func(o obj) string { return o["id"].(string) })
		f.list(w, r, toAny(items, func(o obj) any { return o }), more)
	case r.Method == "GET" && len(parts) == 2:
		if p := f.price(parts[1]); p != nil {
			f.respond(w, r, p)
		} else {
			f.missing(w, "price")
		}
	case r.Method == "POST" && len(parts) == 1:
		prod := r.Form.Get("product")
		if _, ok := f.products[prod]; !ok {
			f.missing(w, "product")
			return
		}
		amount, err := strconv.ParseInt(r.Form.Get("unit_amount"), 10, 64)
		if err != nil || amount < 0 || r.Form.Get("currency") == "" || r.Form.Get("recurring[interval]") == "" {
			f.bad(w, "Invalid price")
			return
		}
		key := r.Form.Get("lookup_key")
		if key != "" {
			for _, p := range f.prices {
				if p["lookup_key"] == key {
					if r.Form.Get("transfer_lookup_key") != "true" {
						f.bad(w, "A price with lookup key "+key+" already exists.")
						return
					}
					p["lookup_key"] = nil
				}
			}
		}
		count, _ := strconv.Atoi(cmp.Or(r.Form.Get("recurring[interval_count]"), "1"))
		id := f.nextID("price")
		p := obj{
			"id": id, "object": "price", "active": true, "currency": r.Form.Get("currency"), "unit_amount": amount,
			"product": ref{id: prod, get: f.getter(f.products, prod)}, "lookup_key": nullable(key), "type": "recurring",
			"recurring": obj{"interval": r.Form.Get("recurring[interval]"), "interval_count": count}, "metadata": metadata(r.Form),
		}
		f.prices = append(f.prices, p)
		f.respond(w, r, p)
	case r.Method == "POST" && len(parts) == 2:
		p := f.price(parts[1])
		if p == nil {
			f.missing(w, "price")
			return
		}
		if v := r.Form.Get("active"); v != "" {
			p["active"] = v == "true"
		}
		f.respond(w, r, p)
	default:
		f.missing(w, "route")
	}
}

func (f *fakeAPI) serveCheckouts(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case r.Method == "POST" && len(parts) == 2 && parts[1] == "sessions":
		form := r.Form
		local := form.Get("client_reference_id")
		switch {
		case form.Get("mode") != "subscription":
			f.bad(w, "mode must be subscription")
			return
		case f.price(form.Get("line_items[0][price]")) == nil:
			f.missing(w, "price")
			return
		case !strings.Contains(form.Get("success_url"), "{CHECKOUT_SESSION_ID}"):
			f.bad(w, "success_url without the checkout's ID")
			return
		case local == "" || form.Get("metadata[my_subscription_id]") != local || form.Get("subscription_data[metadata][my_subscription_id]") != local:
			f.bad(w, "our ID is not on the checkout and its subscription")
			return
		case form.Get("customer") != "" && form.Get("customer_email") != "":
			f.bad(w, "customer and customer_email")
			return
		case form.Get("ui_mode") != "" && form.Get("ui_mode") != "hosted":
			f.bad(w, "not a hosted checkout")
			return
		}
		exp, _ := strconv.ParseInt(form.Get("expires_at"), 10, 64)
		if d := time.Unix(exp, 0).Sub(f.now()); d < 30*time.Minute || d > 24*time.Hour {
			f.bad(w, "expires_at must be 30 minutes to 24 hours from now")
			return
		}
		id := f.nextID("cs_test")
		u := cmp.Or(f.checkoutURL, "https://"+CheckoutHost+"/c/pay/"+id)
		f.checkouts[id] = obj{
			"id": id, "object": "checkout.session", "mode": "subscription", "status": "open", "url": u,
			"client_reference_id": local, "metadata": metadata(form), "customer": nullable(form.Get("customer")),
			"customer_email": nullable(form.Get("customer_email")), "subscription": nil, "expires_at": exp,
			"locale": form.Get("locale"), "success_url": form.Get("success_url"), "cancel_url": form.Get("cancel_url"),
		}
		f.sessionPrices[id] = form.Get("line_items[0][price]")
		f.respond(w, r, f.checkouts[id])
	case r.Method == "GET" && len(parts) == 3 && parts[1] == "sessions":
		c, ok := f.checkouts[parts[2]]
		if !ok {
			f.missing(w, "checkout.session")
			return
		}
		if expansions(r.Form)["line_items"] {
			// Includable: Stripe returns the line items only on request.
			out := obj{}
			for k, v := range c {
				out[k] = v
			}
			out["line_items"] = obj{"object": "list", "has_more": false, "data": []any{
				obj{"id": "li_" + parts[2], "object": "item", "price": f.price(f.sessionPrices[parts[2]])},
			}}
			c = out
		}
		f.respond(w, r, c)
	case r.Method == "POST" && len(parts) == 4 && parts[1] == "sessions" && parts[3] == "expire":
		c, ok := f.checkouts[parts[2]]
		switch {
		case !ok:
			f.missing(w, "checkout.session")
		case c["status"] != "open":
			f.bad(w, "Only Checkout Sessions with a status in [\"open\"] can be expired.")
		default:
			// Stripe documents the expired session without a url.
			c["status"], c["url"] = "expired", nil
			f.respond(w, r, c)
		}
	default:
		f.missing(w, "route")
	}
}

func (f *fakeAPI) portal(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case r.Method == "GET" && len(parts) == 2 && parts[1] == "configurations":
		f.list(w, r, toAny(f.portalConfigs, func(o obj) any { return o }), false)
	case r.Method == "POST" && len(parts) == 2 && parts[1] == "configurations":
		form := r.Form
		if form.Get("features[subscription_update][enabled]") != "false" || form.Get("features[subscription_cancel][mode]") != "at_period_end" ||
			form.Get("features[payment_method_update][enabled]") != "true" || form.Get("features[invoice_history][enabled]") != "true" {
			f.bad(w, "the portal must update payment methods, show invoices, and cancel at the period's end only")
			return
		}
		c := obj{"id": f.nextID("bpc"), "object": "billing_portal.configuration", "active": true, "metadata": metadata(form)}
		f.portalConfigs = append(f.portalConfigs, c)
		f.respond(w, r, c)
	case r.Method == "POST" && len(parts) == 2 && parts[1] == "sessions":
		if f.find(f.portalConfigs, r.Form.Get("configuration")) == nil {
			f.missing(w, "configuration")
			return
		}
		if r.Form.Get("customer") == "" || r.Form.Get("return_url") == "" {
			f.bad(w, "Missing customer or return_url")
			return
		}
		id := f.nextID("bps")
		f.respond(w, r, obj{"id": id, "object": "billing_portal.session", "customer": r.Form.Get("customer"),
			"url": "https://" + PortalHost + "/p/session/" + id, "return_url": r.Form.Get("return_url")})
	default:
		f.missing(w, "route")
	}
}

func (f *fakeAPI) sub(id string) *fakeSub {
	for _, s := range f.subs {
		if s.o["id"] == id {
			return s
		}
	}
	return nil
}

func (f *fakeAPI) item(s *fakeSub) obj { return s.o["items"].(obj)["data"].([]any)[0].(obj) }

// subscriptionObject returns a subscription with its price, latest invoice, and schedule.
func (f *fakeAPI) subscriptionObject(s *fakeSub) obj {
	o := obj{}
	for k, v := range s.o {
		o[k] = v
	}
	it := obj{}
	for k, v := range f.item(s) {
		it[k] = v
	}
	priceID := it["price"].(string)
	price := f.price(priceID)
	if price == nil {
		price = obj{"id": priceID, "object": "price", "active": true, "currency": "jpy", "unit_amount": 500,
			"recurring": obj{"interval": "month", "interval_count": 1}, "product": "prod_other"}
	}
	it["price"] = price
	o["items"] = obj{"object": "list", "has_more": false, "data": []any{it}}
	inv := s.invoice
	o["latest_invoice"] = ref{id: inv["id"].(string), get: func() obj { return inv }}
	if s.schedule != "" {
		sch := f.schedules[s.schedule]
		o["schedule"] = ref{id: sch.id, get: func() obj { return f.scheduleObject(sch) }}
	} else {
		o["schedule"] = nil
	}
	return o
}

func (f *fakeAPI) scheduleObject(sch *fakeSchedule) obj {
	phases := make([]any, len(sch.phases))
	for i, ph := range sch.phases {
		phases[i] = obj{"start_date": ph.start, "end_date": ph.end, "items": []any{obj{"price": ph.price, "quantity": 1}}}
	}
	o := obj{"id": sch.id, "object": "subscription_schedule", "status": sch.status, "subscription": sch.sub,
		"end_behavior": sch.behavior, "phases": phases}
	if len(sch.phases) > 0 {
		o["current_phase"] = obj{"start_date": sch.phases[0].start, "end_date": sch.phases[0].end}
	}
	return o
}

func (f *fakeAPI) subscriptions(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 && r.Method == "GET" {
		subs := f.subs
		if st := r.Form.Get("status"); st != "all" {
			subs = slices.DeleteFunc(slices.Clone(subs), func(s *fakeSub) bool { return s.o["status"] == "canceled" })
		}
		items, more := page(r, subs, func(s *fakeSub) string { return s.o["id"].(string) })
		f.list(w, r, toAny(items, func(s *fakeSub) any { return f.subscriptionObject(s) }), more)
		return
	}
	if len(parts) != 2 {
		f.missing(w, "route")
		return
	}
	s := f.sub(parts[1])
	if s == nil {
		f.missing(w, "subscription")
		return
	}
	ended := s.o["status"] == "canceled" || s.o["status"] == "incomplete_expired"
	switch r.Method {
	case "GET":
	case "POST":
		if ended {
			f.bad(w, "A canceled subscription can only update its cancellation_details and metadata.")
			return
		}
		_, atEnd := r.Form["cancel_at_period_end"]
		_, at := r.Form["cancel_at"]
		if (atEnd || at) && s.schedule != "" {
			f.bad(w, "The subscription is managed by the subscription schedule "+s.schedule+", and updating any cancelation behavior directly is not allowed. Please update the schedule instead.")
			return
		}
		if atEnd && at {
			f.bad(w, "You cannot set both cancel_at and cancel_at_period_end.")
			return
		}
		if atEnd {
			s.o["cancel_at_period_end"] = r.Form.Get("cancel_at_period_end") == "true"
			if s.o["cancel_at_period_end"] == true {
				s.o["cancel_at"] = f.item(s)["current_period_end"]
			}
		}
		if at {
			if r.Form.Get("cancel_at") != "" {
				f.bad(w, "this fake only clears cancel_at")
				return
			}
			s.o["cancel_at"] = nil
		}
		if price := r.Form.Get("items[0][price]"); price != "" {
			if r.Form.Get("items[0][id]") != f.item(s)["id"] {
				f.bad(w, "items[0][id] is not an item of the subscription")
				return
			}
			if f.price(price) == nil {
				f.missing(w, "price")
				return
			}
			if r.Form.Get("proration_behavior") != "always_invoice" {
				f.bad(w, "the change is not prorated and invoiced at once")
				return
			}
			if r.Form.Get("payment_behavior") != "pending_if_incomplete" {
				f.bad(w, "the change would apply without its payment")
				return
			}
			if f.storeFailure {
				f.storeFailure = false
				f.fail(w, http.StatusInternalServerError, "api_error", "", "failing")
				return
			}
			if !f.payUpdate(w, s, price) {
				return
			}
		}
	case "DELETE":
		if ended {
			f.bad(w, "This subscription is already canceled.")
			return
		}
		if r.Form.Get("prorate") != "false" || r.Form.Get("invoice_now") != "false" {
			f.bad(w, "the cancellation prorates or invoices")
			return
		}
		s.o["status"] = "canceled"
		s.o["cancel_at_period_end"], s.o["cancel_at"] = false, nil
		if s.schedule != "" {
			f.schedules[s.schedule].status = "canceled"
			s.schedule = ""
		}
	default:
		f.missing(w, "route")
		return
	}
	f.respond(w, r, f.subscriptionObject(s))
}

// payUpdate invoices the update of s to price at once. Paid, the update applies; otherwise it waits
// in pending_update with its open invoice, as Stripe documents, replacing an earlier one and voiding
// that one's invoice. It reports false when it answered with the refusal of a card.
func (f *fakeAPI) payUpdate(w http.ResponseWriter, s *fakeSub, price string) bool {
	if s.payment == fakeCardRefuses {
		f.fail(w, http.StatusPaymentRequired, "card_error", "card_declined", "Your card was declined.")
		return false
	}
	if s.o["pending_update"] != nil {
		s.invoice["status"] = "void"
		s.o["pending_update"] = nil
	}
	id := f.nextID("in")
	now := f.now().Unix()
	inv := obj{"id": id, "object": "invoice", "attempted": true, "attempt_count": 1, "status_transitions": obj{"finalized_at": now}}
	f.invoices[id] = inv
	s.invoice = inv
	if s.payment == "" {
		inv["status"] = "paid"
		f.item(s)["price"] = price
		f.changeCharges++
		return true
	}
	inv["status"] = "open"
	pi := f.nextID("pi")
	f.intents[pi] = obj{"id": pi, "object": "payment_intent", "status": s.payment}
	f.invoicePays = append(f.invoicePays, obj{
		"id": f.nextID("inpay"), "object": "invoice_payment", "status": "open", "invoice": ref{id: id, get: f.getter(f.invoices, id)},
		"payment": obj{"type": "payment_intent", "payment_intent": ref{id: pi, get: f.getter(f.intents, pi)}},
	})
	s.o["pending_update"] = obj{"expires_at": now + 23*3600, "subscription_items": []any{obj{"id": f.item(s)["id"], "price": price}}}
	if f.pastDueOnFailure {
		s.o["status"] = "past_due"
	}
	return true
}

// voidInvoice voids an open invoice, which discards the pending update it pays for. It refuses one
// whose payment is processing.
func (f *fakeAPI) voidInvoice(w http.ResponseWriter, r *http.Request, id string) {
	inv, ok := f.invoices[id]
	if !ok {
		f.missing(w, "invoice")
		return
	}
	if inv["status"] != "open" {
		f.bad(w, "You can only void an open invoice.")
		return
	}
	for _, ip := range f.invoicePays {
		if i, _ := ip["invoice"].(ref); i.id == id {
			pi, _ := ip["payment"].(obj)["payment_intent"].(ref)
			if f.intents[pi.id]["status"] == fakeCardProcesses {
				f.bad(w, "The invoice's payment is processing.")
				return
			}
		}
	}
	inv["status"] = "void"
	for _, s := range f.subs {
		if s.invoice["id"] == id {
			s.o["pending_update"] = nil
			if s.o["status"] == "past_due" {
				s.o["status"] = "active"
			}
		}
	}
	f.respond(w, r, inv)
}

// attachSchedule puts s on a new schedule of one phase, its current price until the period's end.
func (f *fakeAPI) attachSchedule(s *fakeSub) *fakeSchedule {
	it := f.item(s)
	sch := &fakeSchedule{id: f.nextID("sub_sched"), sub: s.o["id"].(string), status: "active", behavior: "release",
		phases: []fakePhase{{start: it["current_period_start"].(int64), end: it["current_period_end"].(int64), price: it["price"].(string)}}}
	f.schedules[sch.id] = sch
	s.schedule = sch.id
	return sch
}

func (f *fakeAPI) serveSchedules(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case r.Method == "POST" && len(parts) == 1:
		s := f.sub(r.Form.Get("from_subscription"))
		if s == nil {
			f.missing(w, "subscription")
			return
		}
		if s.schedule != "" {
			f.bad(w, "The subscription is already attached to a schedule.")
			return
		}
		f.respond(w, r, f.scheduleObject(f.attachSchedule(s)))
	case r.Method == "POST" && len(parts) == 2:
		sch, ok := f.schedules[parts[1]]
		if !ok {
			f.missing(w, "subscription_schedule")
			return
		}
		if sch.status != "active" && sch.status != "not_started" {
			f.bad(w, "You cannot update a subscription schedule that is "+sch.status)
			return
		}
		var phases []fakePhase
		for i := 0; ; i++ {
			pre := fmt.Sprintf("phases[%d]", i)
			price := r.Form.Get(pre + "[items][0][price]")
			if price == "" {
				break
			}
			if f.price(price) == nil && price != sch.phases[0].price {
				f.missing(w, "price")
				return
			}
			ph := fakePhase{price: price}
			if i == 0 {
				start, _ := strconv.ParseInt(r.Form.Get(pre+"[start_date]"), 10, 64)
				if start != sch.phases[0].start {
					f.bad(w, "You can not modify the start date of the current phase.")
					return
				}
				ph.start = start
			} else {
				ph.start = phases[i-1].end
			}
			if end := r.Form.Get(pre + "[end_date]"); end != "" {
				ph.end, _ = strconv.ParseInt(end, 10, 64)
			} else if iv := r.Form.Get(pre + "[duration][interval]"); iv != "" {
				n, _ := strconv.Atoi(cmp.Or(r.Form.Get(pre+"[duration][interval_count]"), "1"))
				t := time.Unix(ph.start, 0).UTC()
				ph.end = map[string]time.Time{"month": t.AddDate(0, n, 0), "year": t.AddDate(n, 0, 0)}[iv].Unix()
			} else {
				f.bad(w, pre+" needs an end_date or a duration")
				return
			}
			phases = append(phases, ph)
		}
		if len(phases) == 0 {
			f.bad(w, "no phases")
			return
		}
		sch.phases = phases
		sch.behavior = cmp.Or(r.Form.Get("end_behavior"), sch.behavior)
		f.respond(w, r, f.scheduleObject(sch))
	case r.Method == "POST" && len(parts) == 3 && parts[2] == "release":
		sch, ok := f.schedules[parts[1]]
		if !ok {
			f.missing(w, "subscription_schedule")
			return
		}
		if sch.status != "active" && sch.status != "not_started" {
			f.bad(w, "You cannot release a subscription schedule that is currently in the `"+sch.status+"` status.")
			return
		}
		sch.status = "released"
		if s := f.sub(sch.sub); s != nil {
			s.schedule = ""
		}
		f.respond(w, r, f.scheduleObject(sch))
	default:
		f.missing(w, "route")
	}
}

// sortedBTs returns the balance transactions newest first.
func (f *fakeAPI) sortedBTs() []*fakeBT {
	bts := slices.Clone(f.bts)
	slices.SortFunc(bts, func(a, b *fakeBT) int {
		return cmp.Or(cmp.Compare(b.created, a.created), cmp.Compare(b.seq, a.seq))
	})
	return bts
}

func (f *fakeAPI) balance(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case r.Method == "GET" && len(parts) == 1:
		var bts []*fakeBT
		gte, _ := strconv.ParseInt(r.Form.Get("created[gte]"), 10, 64)
		for _, b := range f.sortedBTs() {
			if src := r.Form.Get("source"); src != "" && b.source != src {
				continue
			}
			if b.created < gte {
				continue
			}
			bts = append(bts, b)
		}
		items, more := page(r, bts, func(b *fakeBT) string { return b.o["id"].(string) })
		f.list(w, r, toAny(items, func(b *fakeBT) any { return b.o }), more)
	case r.Method == "GET" && len(parts) == 2:
		if o := f.btGetter(parts[1])(); o != nil {
			f.respond(w, r, o)
		} else {
			f.missing(w, "balance transaction")
		}
	default:
		f.missing(w, "route")
	}
}

func (f *fakeAPI) listEvents(w http.ResponseWriter, r *http.Request) {
	types := formList(r.Form, "types")
	gte, _ := strconv.ParseInt(r.Form.Get("created[gte]"), 10, 64)
	var evs []obj
	for i := len(f.events) - 1; i >= 0; i-- {
		e := f.events[i]
		if e["created"].(int64) < gte || (len(types) > 0 && !slices.Contains(types, e["type"].(string))) {
			continue
		}
		evs = append(evs, e)
	}
	items, more := page(r, evs, func(o obj) string { return o["id"].(string) })
	f.list(w, r, toAny(items, func(o obj) any { return o }), more)
}

// countRequests returns how many requests of method and path prefix the fake answered.
func (f *fakeAPI) countRequests(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}
