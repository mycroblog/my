// SPDX-License-Identifier: AGPL-3.0-only

package otlp_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/errorreport/otlp"
	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
	"github.com/mycroblog/my/internal/app/kernel"
)

const (
	traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	spanID  = "00f067aa0ba902b7"
	// rawMessage is the error's text before the reporter scrubs it.
	rawMessage = "lookup of alice@example.com failed"
)

// newClient returns a Client that exports to h, through the standard environment variable.
func newClient(t *testing.T, h http.Handler) *otlp.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", srv.URL+"/v1/logs")
	c, err := otlp.New(t.Context(), otlp.Options{ServiceVersion: "1.2.3", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Shutdown(t.Context()) })
	return c
}

// event returns a scrubbed event of an error whose raw text has an email address.
func event() kernel.ErrorEvent {
	ev := kernel.NewErrorEvent(fmt.Errorf("wrap: %w", errors.New(rawMessage)), "job.deliver")
	ev.Message = "lookup of <email> failed" // what the reporter's scrub leaves (ADR-0103 §3)
	ev.Time = time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC)
	ev.Fingerprint = "0123456789abcdef0123456789abcdef"
	ev.TraceID, ev.SpanID = traceID, spanID
	ev.Environment = "staging"
	return ev
}

func TestSendErrorRecord(t *testing.T) {
	c := &otlptest.LogCollector{}
	client := newClient(t, c)
	ev := event()
	if err := client.SendError(t.Context(), ev); err != nil {
		t.Fatal(err)
	}
	logs := c.Logs()
	if len(logs) != 1 {
		t.Fatalf("received %d records, want 1", len(logs))
	}
	l := logs[0]
	if l.EventName != "exception" || l.SeverityNumber != 17 || l.SeverityText != "ERROR" {
		t.Errorf("event %q severity %d %q, want exception 17 ERROR", l.EventName, l.SeverityNumber, l.SeverityText)
	}
	if l.TimeUnixNano != uint64(ev.Time.UnixNano()) {
		t.Errorf("time = %d, want %d", l.TimeUnixNano, ev.Time.UnixNano())
	}
	if l.TraceID != traceID || l.SpanID != spanID {
		t.Errorf("trace %s span %s, want %s %s", l.TraceID, l.SpanID, traceID, spanID)
	}
	if !l.Gzip {
		t.Error("the export is not gzip-compressed")
	}
	if l.Body != "*errors.errorString: lookup of <email> failed" {
		t.Errorf("body = %q", l.Body)
	}
	want := map[string]string{
		"exception.type":              "*errors.errorString",
		"exception.message":           "lookup of <email> failed",
		"exception.stacktrace":        ev.Stack,
		"my.error.fingerprint":        ev.Fingerprint,
		"my.error.where":              "job.deliver",
		"my.error.source":             "server",
		"my.error.kind":               "error",
		"deployment.environment.name": "staging",
	}
	for k, v := range want {
		if l.Attrs[k] != v {
			t.Errorf("attribute %s = %q, want %q", k, l.Attrs[k], v)
		}
	}
	if len(l.Attrs) != len(want) {
		t.Errorf("attributes = %v, want only %v", l.Attrs, want)
	}
	if !strings.Contains(l.Attrs["exception.stacktrace"], "otlp_test.event") {
		t.Errorf("stacktrace lacks the caller:\n%s", l.Attrs["exception.stacktrace"])
	}
	for k, v := range map[string]string{"service.name": "my", "service.version": "1.2.3", "my.role": "worker"} {
		if l.Resource[k] != v {
			t.Errorf("resource %s = %q, want %q", k, l.Resource[k], v)
		}
	}
	for _, b := range c.Bodies() {
		if bytes.Contains(b, []byte("alice@example.com")) || bytes.Contains(b, []byte(rawMessage)) {
			t.Errorf("the export carries the raw message:\n%q", b)
		}
	}
}

func TestSendErrorFatalWithoutTrace(t *testing.T) {
	c := &otlptest.LogCollector{}
	client := newClient(t, c)
	ev := event()
	ev.Fatal, ev.TraceID, ev.SpanID, ev.Environment = true, "", "", ""
	if err := client.SendError(t.Context(), ev); err != nil {
		t.Fatal(err)
	}
	logs := c.Logs()
	if len(logs) != 1 {
		t.Fatalf("received %d records, want 1", len(logs))
	}
	l := logs[0]
	if l.SeverityNumber != 21 || l.SeverityText != "FATAL" {
		t.Errorf("severity %d %q, want 21 FATAL", l.SeverityNumber, l.SeverityText)
	}
	if l.TraceID != "" || l.SpanID != "" {
		t.Errorf("trace %q span %q, want none", l.TraceID, l.SpanID)
	}
	if _, ok := l.Attrs["deployment.environment.name"]; ok {
		t.Error("an event without environment has deployment.environment.name")
	}
}

func TestSendErrorFailure(t *testing.T) {
	var calls atomic.Int32
	client := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	if err := client.SendError(t.Context(), event()); err == nil {
		t.Fatal("SendError = nil for a 503")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("receiver got %d requests, want 1 (no retry)", n)
	}
}
