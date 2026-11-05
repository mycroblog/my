// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/schema"
)

const (
	testOriginSecret   = "0123456789abcdef0123456789abcdef"
	testOriginPrevious = "fedcba9876543210fedcba9876543210"
	testOriginHeader   = "X-Via-My-Cdn"
	testOriginPeer     = "192.0.2.10:4000" // a trusted proxy, as a caller on the CDN's own platform is
)

// countingTracer counts the spans and the parents that a request asks for.
type countingTracer struct {
	kernel.NopTracer
	started, continued atomic.Int32
}

func (t *countingTracer) Start(ctx context.Context, spec kernel.SpanSpec, attrs kernel.Attrs) (context.Context, kernel.Span) {
	t.started.Add(1)
	return t.NopTracer.Start(ctx, spec, attrs)
}

func (t *countingTracer) Continue(ctx context.Context, _ string) context.Context {
	t.continued.Add(1)
	return ctx
}

// originApp is an App with the origin check on env, with a route that reports what its handler saw.
type originApp struct {
	*App
	h      http.Handler
	logs   *bytes.Buffer
	tracer *countingTracer
	meter  *testkit.Meter
	// reached counts the requests that reached a route, and seen is the last one's view.
	reached atomic.Int32
	seen    atomic.Pointer[originSeen]
}

type originSeen struct {
	header   string
	present  bool
	clientIP string
}

func newOriginApp(t *testing.T, env map[string]string) *originApp {
	t.Helper()
	path := filepath.Join(t.TempDir(), "my.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), schema.SQLite); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	full := map[string]string{
		"MY_HOST": "example.test", "MY_KEK": "MDEyMzQ1Njc4OWFiY2RlZmdoaWprbG1ub3BxcnN0dXY=", "MY_BLOB_FS_DIR": t.TempDir(),
		"MY_DATABASE_URL": path, "MY_TRUSTED_PROXIES": "192.0.2.10",
	}
	for k, v := range env {
		full[k] = v
	}
	cfg, _, err := config.Load(config.Source{LookupEnv: func(k string) (string, bool) { v, ok := full[k]; return v, ok }})
	if err != nil {
		t.Fatal(err)
	}
	o := &originApp{logs: &bytes.Buffer{}, tracer: &countingTracer{}, meter: testkit.NewMeter()}
	o.App, err = NewApp(t.Context(), cfg, AppOptions{Logger: NewLogger(o.logs, slog.LevelDebug), Tracer: o.tracer, Meter: o.meter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	o.handle("/test/seen", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.reached.Add(1)
		v, present := r.Header[http.CanonicalHeaderKey(testOriginHeader)]
		s := &originSeen{present: present, clientIP: kernel.RequestClientIP(r)}
		if present {
			s.header = v[0]
		}
		o.seen.Store(s)
		w.WriteHeader(http.StatusNoContent)
	}), surfaceClientAPI)
	o.h = o.Handler()
	return o
}

func (o *originApp) refusedCount() float64 { return o.meter.Sum(OriginRefused.Name, nil) }

func originSecretEnv() map[string]string {
	return map[string]string{"MY_ORIGIN_SECRET": testOriginSecret, "MY_ORIGIN_SECRET_HEADER": testOriginHeader}
}

func originRequest(t *testing.T, method, target string, secret ...string) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	r.RemoteAddr = testOriginPeer
	for _, s := range secret {
		r.Header.Add(testOriginHeader, s)
	}
	return r
}

func TestOriginCheckValues(t *testing.T) {
	env := originSecretEnv()
	env["MY_ORIGIN_SECRET_PREVIOUS"] = testOriginPrevious
	o := newOriginApp(t, env)
	for _, tt := range []struct {
		name   string
		secret []string
		want   int
	}{
		{"missing", nil, http.StatusForbidden},
		{"empty", []string{""}, http.StatusForbidden},
		{"wrong", []string{"not-the-secret"}, http.StatusForbidden},
		{"a prefix of it", []string{testOriginSecret[:31]}, http.StatusForbidden},
		{"longer than it", []string{testOriginSecret + "0"}, http.StatusForbidden},
		{"another case", []string{strings.ToUpper(testOriginSecret)}, http.StatusForbidden},
		{"two lines", []string{testOriginSecret, testOriginSecret}, http.StatusForbidden},
		{"right after wrong", []string{"not-the-secret", testOriginSecret}, http.StatusForbidden},
		{"correct", []string{testOriginSecret}, http.StatusNoContent},
		{"previous", []string{testOriginPrevious}, http.StatusNoContent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			o.h.ServeHTTP(rec, originRequest(t, http.MethodGet, "/test/seen", tt.secret...))
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// The header is gone before any handler, in logs and traces too; the trust in the peer holds only
// for a request that proved the secret.
func TestOriginCheckStripsTheHeader(t *testing.T) {
	o := newOriginApp(t, originSecretEnv())
	r := originRequest(t, http.MethodGet, "/test/seen", testOriginSecret)
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	s := o.seen.Load()
	if s.present {
		t.Errorf("the handler saw the header: %q", s.header)
	}
	if s.clientIP != "203.0.113.7" {
		t.Errorf("client IP = %q, want the proxy's report", s.clientIP)
	}
	if logs := o.logs.String(); strings.Contains(logs, testOriginSecret) || strings.Contains(logs, testOriginHeader) {
		t.Errorf("the log holds the header:\n%s", logs)
	}
}

func TestOriginCheckRefusal(t *testing.T) {
	o := newOriginApp(t, originSecretEnv())
	before, spans := o.tracer.continued.Load(), o.tracer.started.Load()
	r := originRequest(t, http.MethodGet, "/test/seen", "wrong")
	r.Header.Set("Traceparent", "00-"+incomingTraceID+"-"+incomingSpanID+"-01")
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set(HeaderRequestID, "chosen-by-the-caller")
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if o.reached.Load() != 0 {
		t.Error("a refused request reached the handler")
	}
	if got := o.refusedCount(); got != 1 {
		t.Errorf("%s = %v, want 1", OriginRefused.Name, got)
	}
	// Neither a trace nor a span: the caller connects from a trusted peer and chose a parent.
	if got := o.tracer.continued.Load() - before; got != 0 {
		t.Errorf("Continue called %d times", got)
	}
	if got := o.tracer.started.Load() - spans; got != 0 {
		t.Errorf("started %d spans", got)
	}
	// The answer is the plain 403 and names nothing.
	var p problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != "about:blank" || p.Status != http.StatusForbidden || p.Title == "" {
		t.Errorf("problem = %+v", p)
	}
	if p.RequestID == "chosen-by-the-caller" || rec.Header().Get(HeaderRequestID) == "chosen-by-the-caller" {
		t.Error("the refused request chose its request ID")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	all := strings.ToLower(rec.Body.String() + headerText(rec.Header()))
	for _, leak := range []string{strings.ToLower(testOriginHeader), testOriginSecret, "secret", "cdn", "origin_"} {
		if strings.Contains(all, leak) {
			t.Errorf("the answer reveals %q:\n%s", leak, all)
		}
	}
	// A wrong value and a missing one answer alike.
	missing := httptest.NewRecorder()
	o.h.ServeHTTP(missing, originRequest(t, http.MethodGet, "/test/seen"))
	if missing.Code != rec.Code || strings.Contains(missing.Header().Get("Vary"), testOriginHeader) {
		t.Errorf("missing = %d %v", missing.Code, missing.Header())
	}
	// The access log counts it without the values.
	logs := o.logs.String()
	if !strings.Contains(logs, `"http.response.status_code":403`) {
		t.Errorf("the access log lacks the refusal:\n%s", logs)
	}
	for _, leak := range []string{testOriginHeader, "wrong", "203.0.113.7", incomingTraceID} {
		if strings.Contains(logs, leak) {
			t.Errorf("the log holds %q:\n%s", leak, logs)
		}
	}
}

func headerText(h http.Header) string {
	var b strings.Builder
	for k, vs := range h {
		// The policy of the security headers names the origin of browsers, which says nothing.
		if k == "Content-Security-Policy" || k == "Cross-Origin-Opener-Policy" || k == "Cross-Origin-Resource-Policy" {
			continue
		}
		b.WriteString(k + ": " + strings.Join(vs, ",") + "\n")
	}
	return b.String()
}

// Each exempt request passes without the secret, loses what a trusted peer would have vouched
// for, and loses the header when it carries one. Nothing but those requests passes.
func TestOriginCheckExemptions(t *testing.T) {
	env := originSecretEnv()
	o := newOriginApp(t, env)
	for _, tt := range []struct {
		method, path string
		exempt       bool
	}{
		{http.MethodGet, PathLiveness, true},
		{http.MethodHead, PathLiveness, true},
		{http.MethodGet, PathReadiness, true},
		{http.MethodGet, PathInstanceHealth, true},
		{http.MethodPost, "/_internal/jobs", true},
		{http.MethodPost, "/_internal/tick", true},
		{http.MethodPost, "/_internal/anything", true},

		{http.MethodGet, "/", false},
		{http.MethodGet, "/assets/app.js", false},
		{http.MethodGet, "/@alice", false},
		{http.MethodGet, "/media/a/b.png", false},
		{http.MethodGet, "/media/proxy/x/original?sig=y", false},
		{http.MethodGet, "/api/v1/instance", false},
		{http.MethodPost, "/inbox", false},
		{http.MethodGet, "/.well-known/webfinger?resource=acct:a@example.test", false},
		{http.MethodPost, "/payments/webhooks/stripe", false},
		{http.MethodPost, "/oauth/token", false},
		// The web client answers the GET requests that no route serves.
		{http.MethodGet, "/_internal/jobs", false},
		{http.MethodGet, "/_internal/anything", false},
		{http.MethodPost, "/_internal", false},
		{http.MethodPost, "/_internal/../api/v1/statuses", false},
		{http.MethodPost, "/_internal//jobs", false},
		{http.MethodPost, "/healthz", false},
		{http.MethodGet, "/healthz/", false},
		{http.MethodGet, "/healthz/../media/a", false},
		{http.MethodOptions, "/api/v1/instance/health", false},
		{http.MethodPost, "/events", false},
		{http.MethodGet, "/events", false},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			before := o.refusedCount()
			rec := httptest.NewRecorder()
			o.h.ServeHTTP(rec, originRequest(t, tt.method, tt.path))
			refused := o.refusedCount() > before
			if refused == tt.exempt {
				t.Errorf("status = %d, refused = %v, want exempt = %v", rec.Code, refused, tt.exempt)
			}
		})
	}
}

// Without queue = sqs nothing poses as an SQS event; with it the event alone passes, at its path.
func TestOriginCheckSQSEvents(t *testing.T) {
	cfg := &config.Config{Queue: "db"}
	event := func(path string) *http.Request {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
		r.Header.Set("X-Amzn-Request-Context", "null")
		return r
	}
	if sqsEventsExempt(cfg)(event("/events")) {
		t.Error("an event passes without queue = sqs")
	}
	cfg = &config.Config{Queue: "sqs", QueueSQSEventsPath: "/events"}
	exempt := sqsEventsExempt(cfg)
	if !exempt(event("/events")) {
		t.Error("the event of queue = sqs does not pass")
	}
	if exempt(event("/other")) {
		t.Error("an event passes at another path")
	}
	if exempt(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/events", nil)) {
		t.Error("a request that is no event passes")
	}
	if originExempt(func() *http.Request {
		r := event("/events")
		r.Method = http.MethodGet
		return r
	}(), exempt) {
		t.Error("a GET passes as an event")
	}
}

// An exempt request without the secret proved nothing: the trusted peer's report of a client IP
// and of a trace do not count, and the header it sent does not reach the handler.
func TestOriginCheckExemptRequestTrustsNoOne(t *testing.T) {
	o := newOriginApp(t, originSecretEnv())
	o.handle("POST /_internal/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present := r.Header[http.CanonicalHeaderKey(testOriginHeader)]
		o.seen.Store(&originSeen{present: present, clientIP: kernel.RequestClientIP(r)})
		w.WriteHeader(http.StatusNoContent)
	}), surfaceQueue)
	h := o.Handler()
	r := originRequest(t, http.MethodPost, "/_internal/test", "wrong")
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set("Traceparent", "00-"+incomingTraceID+"-"+incomingSpanID+"-01")
	before := o.tracer.continued.Load()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	s := o.seen.Load()
	if s.present {
		t.Error("the handler saw the header")
	}
	if s.clientIP != "192.0.2.10" {
		t.Errorf("client IP = %q, want the peer", s.clientIP)
	}
	if o.tracer.continued.Load() != before {
		t.Error("the trace of a request that proved nothing continued")
	}
}

// With the settings unset the same requests pass as they did: the header is an ordinary header.
func TestOriginCheckUnsetChangesNothing(t *testing.T) {
	o := newOriginApp(t, nil)
	r := originRequest(t, http.MethodGet, "/test/seen", "anything")
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set("Traceparent", "00-"+incomingTraceID+"-"+incomingSpanID+"-01")
	r.Header.Set(HeaderRequestID, "kept")
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	s := o.seen.Load()
	if !s.present || s.header != "anything" || s.clientIP != "203.0.113.7" {
		t.Errorf("seen = %+v", s)
	}
	if rec.Header().Get(HeaderRequestID) != "kept" {
		t.Errorf("request ID = %q", rec.Header().Get(HeaderRequestID))
	}
	if o.tracer.continued.Load() != 1 {
		t.Errorf("Continue called %d times, want 1", o.tracer.continued.Load())
	}
	if o.refusedCount() != 0 {
		t.Error("a request was refused")
	}
	if strings.Contains(o.logs.String(), "origin check") {
		t.Error("the log mentions the origin check")
	}
}
