// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAPI is PAY.JP API v1 for the tests: the objects and the requests of the adapter only, in
// the JSON of the reference (https://pay.jp/docs/api/), newest first by offsets.
type fakeAPI struct {
	t      *testing.T
	secret string
	now    func() time.Time
	live   bool

	mu sync.Mutex
	// failure, when not 0, is the status of every request.
	failure    int
	plans      map[string]*fakePlan
	subs       map[string]*fakeSub
	customers  map[string]*fakeCustomer
	charges    map[string]*fakeCharge
	events     []fakeEvent
	statements map[string]statement
	tokens     map[string]token
	seq        int

	srv *httptest.Server
}

type fakePlan struct {
	plan
	created int64
}

type fakeSub struct {
	subscription
	created int64
}

type fakeCustomer struct {
	customer
	email   string
	created int64
}

type fakeCharge struct {
	charge
}

type fakeEvent struct {
	event
	resource string
	object   string
}

func newFakeAPI(t *testing.T, secret string, live bool, now func() time.Time) *fakeAPI {
	f := &fakeAPI{
		t: t, secret: secret, live: live, now: now,
		plans: map[string]*fakePlan{}, subs: map[string]*fakeSub{}, customers: map[string]*fakeCustomer{},
		charges: map[string]*fakeCharge{}, statements: map[string]statement{}, tokens: map[string]token{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) id(prefix string) string {
	f.seq++
	return fmt.Sprintf("%s%06d", prefix, f.seq)
}

// addPlan adds a plan.
func (f *fakeAPI) addPlan(id string, amount int64, interval string, meta metadata) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.plans[id] = &fakePlan{plan: plan{ID: id, Amount: amount, Currency: currency, Interval: interval, Name: id, Metadata: meta}, created: f.now().Unix()}
}

// addSub adds a subscription.
func (f *fakeAPI) addSub(s subscription, created int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.customers[s.Customer]; !ok {
		f.customers[s.Customer] = &fakeCustomer{customer: customer{ID: s.Customer}, created: created}
	}
	f.subs[s.ID] = &fakeSub{subscription: s, created: created}
}

// addCharge adds a charge and its event charge.succeeded.
func (f *fakeAPI) addCharge(c charge, eventID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.charges[c.ID] = &fakeCharge{charge: c}
	f.addEventLocked(eventID, "charge.succeeded", c.Created, c.ID, "charge", c)
}

// refund refunds amount of a charge at at, with the event of eventID.
func (f *fakeAPI) refund(chargeID string, amount int64, at int64, eventID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.charges[chargeID]
	c.AmountRefunded += amount
	c.Refunded = true
	f.addEventLocked(eventID, "charge.refunded", at, c.ID, "charge", c.charge)
}

// dispute adds an event dispute.created of amount of a charge.
func (f *fakeAPI) dispute(chargeID string, amount int64, at int64, eventID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addEventLocked(eventID, "dispute.created", at, chargeID, "dispute", map[string]any{"object": "dispute", "charge": chargeID, "amount": amount})
}

// addEvent adds an event of an object.
func (f *fakeAPI) addEvent(id, typ string, at int64, resource, object string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addEventLocked(id, typ, at, resource, object, data)
}

func (f *fakeAPI) addEventLocked(id, typ string, at int64, resource, object string, data any) {
	if id == "" {
		id = f.id("evnt_")
	}
	b, err := json.Marshal(data)
	if err != nil {
		f.t.Fatal(err)
	}
	f.events = append(f.events, fakeEvent{event: event{ID: id, Type: typ, Livemode: f.live, Created: at, Data: b}, resource: resource, object: object})
}

func (f *fakeAPI) fail(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failure = status
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, _, ok := r.BasicAuth()
	if !ok || user != f.secret {
		apiFail(w, http.StatusUnauthorized, "invalid_api_key")
		return
	}
	if f.failure != 0 {
		apiFail(w, f.failure, "failure")
		return
	}
	if err := r.ParseForm(); err != nil {
		apiFail(w, http.StatusBadRequest, "invalid_param")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case parts[0] == "plans":
		f.servePlans(w, r, parts[1:])
	case parts[0] == "subscriptions":
		f.serveSubs(w, r, parts[1:])
	case parts[0] == "charges":
		f.serveCharges(w, r, parts[1:])
	case parts[0] == "events":
		f.serveEvents(w, r, parts[1:])
	case parts[0] == "statements" && len(parts) == 2 && r.Method == http.MethodGet:
		st, ok := f.statements[parts[1]]
		if !ok {
			apiFail(w, http.StatusNotFound, "invalid_id")
			return
		}
		writeJSON(w, st)
	case parts[0] == "customers":
		f.serveCustomers(w, r, parts[1:])
	case parts[0] == "tokens" && len(parts) == 2 && r.Method == http.MethodGet:
		tk, ok := f.tokens[parts[1]]
		if !ok {
			apiFail(w, http.StatusNotFound, "invalid_id")
			return
		}
		writeJSON(w, tk)
	default:
		apiFail(w, http.StatusNotFound, "not_found")
	}
}

func apiFail(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "status": status, "type": "client_error", "message": code}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// page writes the items newest first, limited and offset by the query.
func page[T any](w http.ResponseWriter, r *http.Request, items []T) {
	limit, offset := 10, 0
	if v := r.Form.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	if v := r.Form.Get("offset"); v != "" {
		offset, _ = strconv.Atoi(v)
	}
	if limit < 1 || limit > 100 {
		apiFail(w, http.StatusBadRequest, "invalid_param")
		return
	}
	end := min(offset+limit, len(items))
	data := []T{}
	if offset < len(items) {
		data = items[offset:end]
	}
	writeJSON(w, map[string]any{"object": "list", "data": data, "has_more": end < len(items), "count": len(data)})
}

// within reports whether a time is within the query's since and until.
func within(r *http.Request, at int64) bool {
	if v := r.Form.Get("since"); v != "" {
		if n, _ := strconv.ParseInt(v, 10, 64); at < n {
			return false
		}
	}
	if v := r.Form.Get("until"); v != "" {
		if n, _ := strconv.ParseInt(v, 10, 64); at > n {
			return false
		}
	}
	return true
}

// setMetadata applies the form's metadata[...] to m; an empty value deletes the key.
func setMetadata(r *http.Request, m metadata) metadata {
	if m == nil {
		m = metadata{}
	}
	for k, v := range r.PostForm {
		if key, ok := strings.CutPrefix(k, "metadata["); ok {
			key = strings.TrimSuffix(key, "]")
			if v[0] == "" {
				delete(m, key)
			} else {
				m[key] = v[0]
			}
		}
	}
	return m
}

func (f *fakeAPI) servePlans(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		var all []*fakePlan
		for _, p := range f.plans {
			all = append(all, p)
		}
		slices.SortFunc(all, func(a, b *fakePlan) int {
			if a.created != b.created {
				return int(b.created - a.created)
			}
			return strings.Compare(b.ID, a.ID)
		})
		out := make([]plan, len(all))
		for i, p := range all {
			out[i] = p.plan
		}
		page(w, r, out)
	case len(rest) == 0 && r.Method == http.MethodPost:
		id := r.PostForm.Get("id")
		if id == "" {
			id = f.id("pln_")
		}
		if _, dup := f.plans[id]; dup {
			apiFail(w, http.StatusBadRequest, "already_exist_id")
			return
		}
		amount, _ := strconv.ParseInt(r.PostForm.Get("amount"), 10, 64)
		if amount < MinimumAmount || amount > MaximumAmount || r.PostForm.Get("currency") != currency {
			apiFail(w, http.StatusBadRequest, "invalid_param")
			return
		}
		p := &fakePlan{plan: plan{
			ID: id, Amount: amount, Currency: currency, Interval: r.PostForm.Get("interval"), Name: r.PostForm.Get("name"),
			Metadata: setMetadata(r, nil),
		}, created: f.now().Unix()}
		f.seq++
		p.created = f.now().Unix()*1000 + int64(f.seq) // keep the order of creation within a second
		f.plans[id] = p
		writeJSON(w, p.plan)
	case len(rest) == 1:
		p, ok := f.plans[rest[0]]
		if !ok {
			apiFail(w, http.StatusNotFound, "invalid_id")
			return
		}
		if r.Method == http.MethodPost {
			if v, ok := r.PostForm["name"]; ok {
				p.Name = v[0]
			}
			p.Metadata = setMetadata(r, p.Metadata)
		}
		writeJSON(w, p.plan)
	default:
		apiFail(w, http.StatusNotFound, "not_found")
	}
}

func (f *fakeAPI) serveSubs(w http.ResponseWriter, r *http.Request, rest []string) {
	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			var all []*fakeSub
			for _, s := range f.subs {
				if c := r.Form.Get("customer"); c != "" && s.Customer != c {
					continue
				}
				if within(r, s.created) {
					all = append(all, s)
				}
			}
			slices.SortFunc(all, func(a, b *fakeSub) int {
				if a.created != b.created {
					return int(b.created - a.created)
				}
				return strings.Compare(b.ID, a.ID)
			})
			out := make([]subscription, len(all))
			for i, s := range all {
				out[i] = s.subscription
			}
			page(w, r, out)
		case http.MethodPost:
			f.createSub(w, r)
		default:
			apiFail(w, http.StatusNotFound, "not_found")
		}
		return
	}
	s, ok := f.subs[rest[0]]
	if !ok {
		apiFail(w, http.StatusNotFound, "invalid_id")
		return
	}
	switch {
	case len(rest) == 1 && r.Method == http.MethodGet:
	case len(rest) == 1 && r.Method == http.MethodDelete:
		delete(f.subs, s.ID)
		writeJSON(w, map[string]any{"deleted": true, "id": s.ID, "livemode": f.live})
		return
	case len(rest) == 1 && r.Method == http.MethodPost:
		if v := r.PostForm.Get("plan"); v != "" {
			p, ok := f.plans[v]
			if !ok {
				apiFail(w, http.StatusBadRequest, "invalid_param")
				return
			}
			s.Plan = p.plan
			if te := r.PostForm.Get("trial_end"); te != "" {
				n, _ := strconv.ParseInt(te, 10, 64)
				if n <= f.now().Unix() {
					apiFail(w, http.StatusBadRequest, "invalid_param")
					return
				}
				s.Status = subTrial
			}
		}
		if v, ok := r.PostForm["next_cycle_plan"]; ok {
			if v[0] == "" {
				s.NextCyclePlan = nil
			} else if p, ok := f.plans[v[0]]; ok {
				np := p.plan
				s.NextCyclePlan = &np
			} else {
				apiFail(w, http.StatusBadRequest, "invalid_param")
				return
			}
		}
		s.Metadata = setMetadata(r, s.Metadata)
	case len(rest) == 2 && rest[1] == "cancel" && r.Method == http.MethodPost:
		if s.Status == subCanceled {
			apiFail(w, http.StatusBadRequest, "already_canceled")
			return
		}
		s.Status = subCanceled
	case len(rest) == 2 && rest[1] == "resume" && r.Method == http.MethodPost:
		if s.Status != subCanceled && s.Status != subPaused {
			apiFail(w, http.StatusBadRequest, "already_worked")
			return
		}
		s.Status, s.PausedAt = subActive, 0
	default:
		apiFail(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, s.subscription)
}

// createSub creates a subscription of a customer with a default card, charging its plan.
func (f *fakeAPI) createSub(w http.ResponseWriter, r *http.Request) {
	c, ok := f.customers[r.PostForm.Get("customer")]
	p, okp := f.plans[r.PostForm.Get("plan")]
	if !ok || !okp || c.DefaultCard == "" {
		apiFail(w, http.StatusBadRequest, "invalid_param")
		return
	}
	now := f.now()
	s := &fakeSub{subscription: subscription{
		ID: f.id("sub_"), Customer: c.ID, Plan: p.plan, Status: subActive,
		CurrentPeriodStart: now.Unix(), CurrentPeriodEnd: addInterval(now, p.Interval).Unix(), Metadata: setMetadata(r, nil),
	}, created: now.Unix()}
	f.subs[s.ID] = s
	ch := charge{
		ID: f.id("ch_"), Amount: p.Amount, Captured: true, CapturedAt: now.Unix(), Created: now.Unix(), Currency: currency,
		Customer: c.ID, FeeRate: "3.00", Paid: true, Subscription: s.ID,
	}
	f.charges[ch.ID] = &fakeCharge{charge: ch}
	f.addEventLocked("", "charge.succeeded", now.Unix(), ch.ID, "charge", ch)
	writeJSON(w, s.subscription)
}

func (f *fakeAPI) serveCharges(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		var all []charge
		for _, c := range f.charges {
			if v := r.Form.Get("customer"); v != "" && c.Customer != v {
				continue
			}
			if v := r.Form.Get("term"); v != "" && c.TermID != v {
				continue
			}
			if within(r, c.Created) {
				all = append(all, c.charge)
			}
		}
		slices.SortFunc(all, func(a, b charge) int {
			if a.Created != b.Created {
				return int(b.Created - a.Created)
			}
			return strings.Compare(b.ID, a.ID)
		})
		page(w, r, all)
	case len(rest) == 0 && r.Method == http.MethodPost:
		amount, _ := strconv.ParseInt(r.PostForm.Get("amount"), 10, 64)
		cus, ok := f.customers[r.PostForm.Get("customer")]
		if !ok || amount < MinimumAmount || r.PostForm.Get("currency") != currency {
			apiFail(w, http.StatusBadRequest, "invalid_param")
			return
		}
		if cus.DefaultCard == "declined" {
			apiFail(w, http.StatusPaymentRequired, "card_declined")
			return
		}
		now := f.now().Unix()
		c := charge{
			ID: f.id("ch_"), Amount: amount, Captured: true, CapturedAt: now, Created: now, Currency: currency,
			Customer: cus.ID, FeeRate: "3.00", Paid: true, Metadata: setMetadata(r, nil),
		}
		f.charges[c.ID] = &fakeCharge{charge: c}
		f.addEventLocked("", "charge.succeeded", now, c.ID, "charge", c)
		writeJSON(w, c)
	case len(rest) == 1 && r.Method == http.MethodGet:
		c, ok := f.charges[rest[0]]
		if !ok {
			apiFail(w, http.StatusNotFound, "invalid_id")
			return
		}
		writeJSON(w, c.charge)
	default:
		apiFail(w, http.StatusNotFound, "not_found")
	}
}

func (f *fakeAPI) serveEvents(w http.ResponseWriter, r *http.Request, rest []string) {
	if len(rest) == 1 && r.Method == http.MethodGet {
		for _, e := range f.events {
			if e.ID == rest[0] {
				writeJSON(w, e.event)
				return
			}
		}
		apiFail(w, http.StatusNotFound, "invalid_id")
		return
	}
	if len(rest) != 0 || r.Method != http.MethodGet {
		apiFail(w, http.StatusNotFound, "not_found")
		return
	}
	var all []fakeEvent
	for _, e := range f.events {
		if v := r.Form.Get("type"); v != "" && e.Type != v {
			continue
		}
		if v := r.Form.Get("resource_id"); v != "" && e.resource != v {
			continue
		}
		if v := r.Form.Get("object"); v != "" && e.object != v {
			continue
		}
		if within(r, e.Created) {
			all = append(all, e)
		}
	}
	slices.SortStableFunc(all, func(a, b fakeEvent) int {
		if a.Created != b.Created {
			return int(b.Created - a.Created)
		}
		return strings.Compare(b.ID, a.ID)
	})
	out := make([]event, len(all))
	for i, e := range all {
		out[i] = e.event
	}
	page(w, r, out)
}

func (f *fakeAPI) serveCustomers(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodPost:
		id := r.PostForm.Get("id")
		if _, dup := f.customers[id]; dup || id == "" {
			apiFail(w, http.StatusBadRequest, "already_exist_id")
			return
		}
		c := &fakeCustomer{customer: customer{ID: id}, email: r.PostForm.Get("email"), created: f.now().Unix()}
		f.customers[id] = c
		writeJSON(w, c.customer)
	case len(rest) >= 1:
		c, ok := f.customers[rest[0]]
		if !ok {
			apiFail(w, http.StatusNotFound, "invalid_id")
			return
		}
		f.serveCustomer(w, r, c, rest[1:])
	default:
		apiFail(w, http.StatusNotFound, "not_found")
	}
}

func (f *fakeAPI) serveCustomer(w http.ResponseWriter, r *http.Request, c *fakeCustomer, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
	case len(rest) == 0 && r.Method == http.MethodPost:
		if v := r.PostForm.Get("default_card"); v != "" {
			if !slices.ContainsFunc(c.Cards.Data, func(k card) bool { return k.ID == v }) {
				apiFail(w, http.StatusBadRequest, "invalid_param")
				return
			}
			c.DefaultCard = v
		}
	case len(rest) == 1 && rest[0] == "cards" && r.Method == http.MethodPost:
		tk, ok := f.tokens[r.PostForm.Get("card")]
		if !ok || tk.Used {
			apiFail(w, http.StatusBadRequest, "invalid_param")
			return
		}
		tk.Used = true
		f.tokens[tk.ID] = tk
		k := tk.Card
		k.ID = f.id("car_")
		c.Cards.Data = append(c.Cards.Data, k)
		if r.PostForm.Get("default") == "true" || c.DefaultCard == "" {
			c.DefaultCard = k.ID
		}
		writeJSON(w, k)
		return
	case len(rest) == 2 && rest[0] == "cards" && r.Method == http.MethodDelete:
		i := slices.IndexFunc(c.Cards.Data, func(k card) bool { return k.ID == rest[1] })
		if i < 0 {
			apiFail(w, http.StatusNotFound, "invalid_id")
			return
		}
		c.Cards.Data = slices.Delete(c.Cards.Data, i, i+1)
		if c.DefaultCard == rest[1] {
			c.DefaultCard = ""
		}
		writeJSON(w, map[string]any{"deleted": true, "id": rest[1]})
		return
	default:
		apiFail(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, c.customer)
}

// query reads the query of a URL; for the tests' checks.
func query(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}
