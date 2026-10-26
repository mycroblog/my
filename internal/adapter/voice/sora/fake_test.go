// SPDX-License-Identifier: AGPL-3.0-only

package sora

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeServer is an httptest stand-in for the HTTP API of a Sora server, with the requests and
// responses of Sora's documents (API, signaling, session, and forwarding filter APIs, and the
// Sora API of Sora Cloud): POST of JSON with the header x-sora-target, authenticated with the
// header of the mode. It keeps the connections of each channel and its forwarding filter.
type fakeServer struct {
	t *testing.T
	// authName and authValue are the header every call must carry.
	authName, authValue string

	mu       sync.Mutex
	channels map[string][]fakeConn
	filters  map[string]*fakeFilter
	calls    []string
	seq      int
}

type fakeConn struct {
	ClientID     string `json:"client_id"`
	ConnectionID string `json:"connection_id"`
	ChannelID    string `json:"channel_id"`
	Role         string `json:"role"`
	Created      string `json:"created_timestamp"`
}

type fakeFilter struct {
	Action   string          `json:"action"`
	Name     string          `json:"name"`
	Version  string          `json:"version,omitempty"`
	Rules    [][]rule        `json:"rules"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

func newFakeServer(t *testing.T, authName, authValue string) (*fakeServer, *httptest.Server) {
	f := &fakeServer{t: t, authName: authName, authValue: authValue, channels: map[string][]fakeConn{}, filters: map[string]*fakeFilter{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

// connect adds a sendrecv connection of clientID to a channel, as a client that the auth
// webhook let in, and returns its connection ID.
func (f *fakeServer) connect(channel, clientID string) string {
	return f.connectAs(channel, clientID, roleSendRecv)
}

// connectAs adds a connection of clientID with role to a channel.
func (f *fakeServer) connectAs(channel, clientID, role string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	id := fmt.Sprintf("CONN%04d", f.seq)
	at := time.Date(2026, 10, 26, 9, 0, f.seq, 0, time.UTC).Format(time.RFC3339Nano)
	f.channels[channel] = append(f.channels[channel], fakeConn{ClientID: clientID, ConnectionID: id, ChannelID: channel, Role: role, Created: at})
	return id
}

// blocked returns the connections whose audio the channel's filter blocks.
func (f *fakeServer) blocked(channel string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := f.filters[channel]
	if fl == nil {
		return nil
	}
	// The placeholder of a filter that blocks no connection blocks nothing.
	return slices.DeleteFunc(filter{Rules: fl.Rules}.blocked(), func(id string) bool { return id == noConnection })
}

func (f *fakeServer) conns(channel string) []fakeConn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeConn(nil), f.channels[channel]...)
}

func soraError(w http.ResponseWriter, status int, typ string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error_type": typ})
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || (r.URL.Path != "/" && r.URL.Path != "/sora-api") {
		soraError(w, http.StatusNotFound, "NOT-FOUND")
		return
	}
	if got := r.Header.Get(f.authName); f.authName != "" && got != f.authValue {
		soraError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		soraError(w, http.StatusBadRequest, "INVALID-CONTENT-TYPE")
		return
	}
	target := r.Header.Get("x-sora-target")
	b, _ := io.ReadAll(r.Body)
	var req struct {
		ChannelID       string          `json:"channel_id"`
		ConnectionID    string          `json:"connection_id"`
		Action          string          `json:"action"`
		Rules           [][]rule        `json:"rules"`
		Version         string          `json:"version"`
		ExpectedVersion string          `json:"expected_version"`
		DesiredVersion  string          `json:"desired_version"`
		Metadata        json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(b, &req); err != nil || req.ChannelID == "" {
		soraError(w, http.StatusBadRequest, "INVALID-JSON")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, target+" "+req.ChannelID)
	ch := req.ChannelID
	var res any = map[string]any{"channel_id": ch}
	switch target {
	case targetListChannelConnections:
		conns := f.channels[ch]
		if conns == nil {
			conns = []fakeConn{}
		}
		res = conns
	case targetDisconnectConnection:
		i := -1
		for j, c := range f.channels[ch] {
			if c.ConnectionID == req.ConnectionID {
				i = j
			}
		}
		if i < 0 {
			soraError(w, http.StatusBadRequest, "CONNECTION-NOT-FOUND")
			return
		}
		f.channels[ch] = append(f.channels[ch][:i], f.channels[ch][i+1:]...)
		res = map[string]any{"channel_id": ch, "connection_id": req.ConnectionID}
	case targetTerminateSession:
		if len(f.channels[ch]) == 0 {
			soraError(w, http.StatusBadRequest, "SESSION-NOT-FOUND")
			return
		}
		delete(f.channels, ch)
		delete(f.filters, ch)
		res = map[string]any{"session_id": "SESSION_" + ch}
	case targetListForwardingFilters:
		list := []fakeFilter{}
		if fl := f.filters[ch]; fl != nil {
			list = append(list, *fl)
		}
		res = map[string]any{"channel_forwarding_filters": list, "connection_forwarding_filters": []any{}}
	case targetCreateChannelFilter, targetUpdateChannelFilter:
		if !validRules(req.Rules) || (req.Action != "block" && req.Action != "allow") {
			soraError(w, http.StatusBadRequest, "INVALID-RULES")
			return
		}
		cur := f.filters[ch]
		version := req.Version
		if target == targetUpdateChannelFilter {
			if cur == nil {
				soraError(w, http.StatusBadRequest, "FORWARDING-FILTER-NOT-FOUND")
				return
			}
			if cur.Version != "" && (req.ExpectedVersion != cur.Version || req.DesiredVersion == "") {
				soraError(w, http.StatusBadRequest, "INVALID-VERSION")
				return
			}
			version = req.DesiredVersion
		} else if cur != nil {
			soraError(w, http.StatusBadRequest, "FORWARDING-FILTER-ALREADY-EXISTS")
			return
		}
		f.filters[ch] = &fakeFilter{Action: req.Action, Name: "default", Version: version, Rules: req.Rules, Metadata: req.Metadata}
		res = map[string]any{"channel_id": ch, "action": req.Action, "rules": req.Rules}
	case targetDeleteChannelFilter:
		if f.filters[ch] == nil {
			soraError(w, http.StatusBadRequest, "FORWARDING-FILTER-NOT-FOUND")
			return
		}
		delete(f.filters, ch)
	default:
		soraError(w, http.StatusBadRequest, "UNKNOWN-TARGET")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// validRules checks the rules of a forwarding filter as Sora does: an OR of ANDs of rules on
// connection_id, client_id, or kind.
func validRules(rules [][]rule) bool {
	if len(rules) == 0 {
		return false
	}
	for _, and := range rules {
		if len(and) == 0 {
			return false
		}
		for _, r := range and {
			switch {
			case r.Field != "connection_id" && r.Field != "client_id" && r.Field != "kind":
				return false
			case r.Operator != "is_in" && r.Operator != "is_not_in":
				return false
			case len(r.Values) == 0:
				return false
			}
		}
	}
	return true
}
