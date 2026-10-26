// SPDX-License-Identifier: AGPL-3.0-only

package apitest_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
)

// recorder is a testing.TB that records failures instead of failing the test.
type recorder struct {
	testing.TB
	failures []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.Errorf(format, args...)
	runtime.Goexit()
}

// failures runs f with a recorder and returns what it recorded.
func failures(t *testing.T, f func(tb testing.TB)) []string {
	t.Helper()
	r := &recorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f(r)
	}()
	<-done
	return r.failures
}

func TestServe(t *testing.T) {
	spec := apitest.Load(t)
	respond := func(status int, contentType, body string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		})
	}
	const instance = `{"domain":"a.example","name":"A","version":"1","source_url":"https://a.example/src","registrations":{"mode":"open"},"limits":{"max_post_characters":500,"require_media_description":false},"timelines":{"local":"everyone","federated":"everyone"},"federation":{"mode":"both"},"sends_email":false,"rules":[],"search_indexing":"deny","ai_usage":"deny","finance_visibility":"private","income_visibility":"private","supporters_enabled":false,"supporter_list_visibility":"off","voice_enabled":false,"voice_room_starters":"permitted"}`
	tests := []struct {
		name     string
		path     string
		handler  http.Handler
		wantFail bool
	}{
		{name: "conforming response", path: "/api/v1/instance", handler: respond(http.StatusOK, "application/json", instance)},
		{name: "conforming problem", path: "/api/v1/instance", handler: respond(http.StatusNotFound, "application/problem+json", `{"type":"about:blank","title":"Not found","status":404}`)},
		{name: "missing required property", path: "/api/v1/instance", handler: respond(http.StatusOK, "application/json", `{"domain":"a.example"}`), wantFail: true},
		{name: "wrong property type", path: "/api/v1/instance", handler: respond(http.StatusOK, "application/json", `{"domain":"a.example","name":"A","version":"1","source_url":"https://a.example/src","registrations":{"mode":"open"},"limits":{"max_post_characters":"500"},"timelines":{"local":"everyone","federated":"everyone"},"sends_email":false,"rules":[]}`), wantFail: true},
		{name: "undocumented content type", path: "/api/v1/instance", handler: respond(http.StatusOK, "text/plain", "hello"), wantFail: true},
		{name: "error that is not a problem", path: "/api/v1/instance", handler: respond(http.StatusInternalServerError, "text/plain", "boom"), wantFail: true},
		{name: "path outside the spec", path: "/api/v1/nope", handler: respond(http.StatusOK, "application/json", "{}"), wantFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := failures(t, func(tb testing.TB) {
				spec.Serve(tb, tt.handler, httptest.NewRequest(http.MethodGet, tt.path, nil))
			})
			if failed := len(got) > 0; failed != tt.wantFail {
				t.Errorf("Serve failed = %v (%q), want %v", failed, got, tt.wantFail)
			}
		})
	}
}

func TestValidateSchema(t *testing.T) {
	spec := apitest.Load(t)
	tests := []struct {
		name     string
		data     string
		wantFail bool
	}{
		{name: "conforming", data: `{"type":"about:blank","title":"Not found","status":404}`},
		{name: "missing required property", data: `{"type":"about:blank","status":404}`, wantFail: true},
		{name: "not JSON", data: `event: update`, wantFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := failures(t, func(tb testing.TB) { spec.ValidateSchema(tb, "Problem", []byte(tt.data)) })
			if failed := len(got) > 0; failed != tt.wantFail {
				t.Errorf("ValidateSchema failed = %v (%q), want %v", failed, got, tt.wantFail)
			}
		})
	}
}
