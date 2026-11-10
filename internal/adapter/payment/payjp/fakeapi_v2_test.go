// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// fakeV2 is PAY.JP API v2 for the tests: the Checkout Sessions, PaymentFlows, refunds, disputes,
// and payment transactions of the adapter only, in the JSON of the official SDK's models, lists
// newest first with starting_after and ending_before, and the Idempotency-Key of the creations.
type fakeV2 struct {
	sessions map[string]*checkoutSession
	flows    map[string]*paymentFlow
	refunds  map[string]*paymentRefund
	disputes map[string]*paymentDispute
	// txns are the payment transactions, oldest first.
	txns []paymentTransaction
	// keys are the responses of the creations by their Idempotency-Key.
	keys map[string]any
	// sessionBodies are the bodies of the sessions' creations, by session.
	sessionBodies map[string]map[string]any
	// refundsCreated counts the refunds that a POST created.
	refundsCreated int
	// requests counts the requests by method and path.
	requests map[string]int
}

func newFakeV2() fakeV2 {
	return fakeV2{
		sessions: map[string]*checkoutSession{}, flows: map[string]*paymentFlow{}, refunds: map[string]*paymentRefund{},
		disputes: map[string]*paymentDispute{}, keys: map[string]any{}, sessionBodies: map[string]map[string]any{},
		requests: map[string]int{},
	}
}

// fakeCheckoutHost is the host of the fake's hosted checkouts.
const fakeCheckoutHost = "checkout.pay.jp"

func v2Fail(w http.ResponseWriter, status int, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://pay.jp/errors/" + title, "title": title, "status": status})
}

// addSession adds a Checkout Session of one payment.
func (f *fakeAPI) addSession(s checkoutSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s.Mode = sessionModePayment
	if s.URL == "" {
		s.URL = "https://" + fakeCheckoutHost + "/c/" + s.ID
	}
	f.v2.sessions[s.ID] = &s
}

// addFlow adds a PaymentFlow.
func (f *fakeAPI) addFlow(fl paymentFlow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v2.flows[fl.ID] = &fl
}

// addTxn adds a payment transaction, and the refund or dispute that it names when it is new.
func (f *fakeAPI) addTxn(t paymentTransaction) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t.Currency == "" {
		t.Currency = currency
	}
	f.v2.txns = append(f.v2.txns, t)
}

// addRefund adds a refund of a PaymentFlow.
func (f *fakeAPI) addRefund(r paymentRefund) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v2.refunds[r.ID] = &r
}

// addDispute adds a dispute of a PaymentFlow.
func (f *fakeAPI) addDispute(d paymentDispute) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.v2.disputes[d.ID] = &d
}

func (f *fakeAPI) serveV2(w http.ResponseWriter, r *http.Request, parts []string) {
	f.v2.requests[r.Method+" /"+strings.Join(parts, "/")]++
	key := r.Header.Get("Idempotency-Key")
	if r.Method == http.MethodPost && key != "" {
		if prev, ok := f.v2.keys[r.URL.Path+" "+key]; ok {
			writeJSON(w, prev)
			return
		}
	}
	var body map[string]any
	if r.Method == http.MethodPost {
		if r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil {
			v2Fail(w, http.StatusBadRequest, "invalid_body")
			return
		}
	}
	remember := func(v any) {
		if key != "" {
			f.v2.keys[r.URL.Path+" "+key] = v
		}
		writeJSON(w, v)
	}
	n := len(parts)
	switch {
	case n == 2 && parts[0] == "checkout" && parts[1] == "sessions" && r.Method == http.MethodPost:
		s, ok := f.createSession(body)
		if !ok {
			v2Fail(w, http.StatusBadRequest, "invalid_param")
			return
		}
		remember(s)
	case n == 3 && parts[0] == "checkout" && parts[1] == "sessions" && r.Method == http.MethodGet:
		s, ok := f.v2.sessions[parts[2]]
		if !ok {
			v2Fail(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, s)
	case n == 4 && parts[0] == "checkout" && parts[1] == "sessions" && parts[3] == "expire" && r.Method == http.MethodPost:
		s, ok := f.v2.sessions[parts[2]]
		switch {
		case !ok:
			v2Fail(w, http.StatusNotFound, "not_found")
		case s.Status != sessionOpen:
			v2Fail(w, http.StatusBadRequest, "checkout_session_not_open")
		default:
			s.Status = sessionExpired
			remember(s)
		}
	case n == 2 && parts[0] == "payment_flows" && r.Method == http.MethodGet:
		fl, ok := f.v2.flows[parts[1]]
		if !ok {
			v2Fail(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, fl)
	case n == 1 && parts[0] == "payment_refunds" && r.Method == http.MethodPost:
		id, _ := body["payment_flow_id"].(string)
		fl, ok := f.v2.flows[id]
		if !ok || fl.Status != flowSucceeded {
			v2Fail(w, http.StatusBadRequest, "invalid_payment_flow")
			return
		}
		f.seq++
		rf := &paymentRefund{ID: "pr_" + strconv.Itoa(f.seq), PaymentFlowID: id, Amount: fl.received(), Status: "succeeded"}
		f.v2.refunds[rf.ID] = rf
		f.v2.refundsCreated++
		f.v2.txns = append(f.v2.txns, paymentTransaction{ID: "pt_" + rf.ID, ResourceID: rf.ID, Amount: -rf.Amount, Currency: currency, Type: txnRefund, CreatedAt: f.now()})
		remember(rf)
	case n == 2 && parts[0] == "payment_refunds" && r.Method == http.MethodGet:
		rf, ok := f.v2.refunds[parts[1]]
		if !ok {
			v2Fail(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, rf)
	case n == 1 && parts[0] == "payment_disputes" && r.Method == http.MethodGet:
		var items []paymentDispute
		for _, d := range f.v2.disputes {
			if st := r.Form["status"]; len(st) == 0 || slices.Contains(st, d.Status) {
				items = append(items, *d)
			}
		}
		slices.SortFunc(items, func(a, b paymentDispute) int {
			if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
				return c
			}
			return strings.Compare(b.ID, a.ID)
		})
		pageV2(w, r, items, func(d paymentDispute) string { return d.ID })
	case n == 2 && parts[0] == "payment_disputes" && r.Method == http.MethodGet:
		d, ok := f.v2.disputes[parts[1]]
		if !ok {
			v2Fail(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, d)
	case n == 1 && parts[0] == "payment_transactions" && r.Method == http.MethodGet:
		var items []paymentTransaction
		for _, t := range slices.Backward(f.v2.txns) {
			if typ := r.Form.Get("type"); typ != "" && t.Type != typ {
				continue
			}
			if term := r.Form.Get("term_id"); term != "" && t.TermID != term {
				continue
			}
			items = append(items, t)
		}
		pageV2(w, r, items, txnID)
	case n == 2 && parts[0] == "payment_transactions" && r.Method == http.MethodGet:
		i := slices.IndexFunc(f.v2.txns, func(t paymentTransaction) bool { return t.ID == parts[1] })
		if i < 0 {
			v2Fail(w, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(w, f.v2.txns[i])
	default:
		v2Fail(w, http.StatusNotFound, "not_found")
	}
}

// createSession creates a Checkout Session of one payment from the body of its creation.
func (f *fakeAPI) createSession(body map[string]any) (*checkoutSession, bool) {
	items, _ := body["line_items"].([]any)
	if body["mode"] != sessionModePayment || body["currency"] != currency || len(items) != 1 || body["success_url"] == "" {
		return nil, false
	}
	price, _ := items[0].(map[string]any)["price_data"].(map[string]any)
	amount, _ := price["unit_amount"].(float64)
	if amount < MinimumAmount || amount > MaximumAmount {
		return nil, false
	}
	exp, err := time.Parse(time.RFC3339, body["expires_at"].(string))
	if err != nil || !exp.After(f.now()) || exp.After(f.now().Add(24*time.Hour)) {
		return nil, false
	}
	meta := metadata{}
	if m, ok := body["metadata"].(map[string]any); ok {
		for k, v := range m {
			meta[k], _ = v.(string)
		}
	}
	ref, _ := body["client_reference_id"].(string)
	f.seq++
	id := "cs_" + strconv.Itoa(f.seq)
	s := &checkoutSession{
		ID: id, Livemode: f.live, Mode: sessionModePayment, Status: sessionOpen, URL: "https://" + fakeCheckoutHost + "/c/" + id,
		ExpiresAt: exp, ClientReferenceID: ref, Metadata: meta,
	}
	f.v2.sessions[id] = s
	f.v2.sessionBodies[id] = body
	return s, true
}

// pageV2 writes the items, newest first, after starting_after or the limit of them just before
// ending_before.
func pageV2[T any](w http.ResponseWriter, r *http.Request, items []T, idOf func(T) string) {
	limit := 10
	if v := r.Form.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	if limit < 1 || limit > 100 {
		v2Fail(w, http.StatusBadRequest, "invalid_param")
		return
	}
	index := func(id string) int { return slices.IndexFunc(items, func(t T) bool { return idOf(t) == id }) }
	var start, end int
	switch {
	case r.Form.Get("starting_after") != "":
		start = index(r.Form.Get("starting_after")) + 1
		end = min(start+limit, len(items))
	case r.Form.Get("ending_before") != "":
		end = index(r.Form.Get("ending_before"))
		if end < 0 {
			end = len(items)
		}
		start = max(end-limit, 0)
	default:
		end = min(limit, len(items))
	}
	data := []T{}
	if start < end {
		data = items[start:end]
	}
	more := end < len(items)
	if r.Form.Get("ending_before") != "" {
		more = start > 0
	}
	writeJSON(w, map[string]any{"object": "list", "data": data, "has_more": more})
}
