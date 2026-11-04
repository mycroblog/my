// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/schema"
)

// The span attributes that ADR-0055 §16 allows.
var allowedSpanAttributes = map[string]bool{
	"http.request.method": true, "http.route": true, "http.response.status_code": true, "my.surface": true,
	"my.job.type": true, "my.job.attempt": true, "my.outcome": true, "activity.type": true,
	"db.system.name": true, "db.operation.name": true, "error.type": true, "server.address": true,
}

const (
	incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	incomingSpanID  = "00f067aa0ba902b7"
)

// newTracedApp returns an App on env whose route POST /test/traced enqueues a test.traced job,
// which handler runs.
func newTracedApp(t *testing.T, env map[string]string, handler queue.Handler) (*App, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "my.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), schema.SQLite); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	full := map[string]string{
		"MY_HOST": "example.test", "MY_KEK": base64.StdEncoding.EncodeToString(kek), "MY_BLOB_FS_DIR": t.TempDir(),
		"MY_DATABASE_URL": path, "MY_TRUSTED_PROXIES": "192.0.2.10",
	}
	for k, v := range env {
		full[k] = v
	}
	cfg, _, err := config.Load(config.Source{LookupEnv: func(k string) (string, bool) { v, ok := full[k]; return v, ok }})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	a, err := NewApp(t.Context(), cfg, AppOptions{Logger: NewLogger(&logs, slog.LevelInfo)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	a.Handlers.Register("test.traced", handler)
	a.handle("POST /test/traced", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := a.Jobs.Enqueue(r.Context(), queue.NewJob{Type: "test.traced"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}), surfaceClientAPI)
	return a, &logs
}

func post(t *testing.T, h http.Handler, remote string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test/traced?token=secret", nil)
	req.RemoteAddr = remote
	req.Header.Set("Traceparent", "00-"+incomingTraceID+"-"+incomingSpanID+"-01")
	req.Header.Set("Tracestate", "vendor=value")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /test/traced = %d %s", rec.Code, rec.Body)
	}
}

// TestNoneTracerCorrelatesLogs checks that traces_exporter = none records no span and sends
// nothing, yet the log lines of a request from a trusted proxy carry its traceparent's IDs
// (ADR-0055 §18), and a job enqueued then carries no TraceParent.
func TestNoneTracerCorrelatesLogs(t *testing.T) {
	var traceParents []string
	a, logs := newTracedApp(t, nil, func(_ context.Context, j queue.Job) queue.Result {
		traceParents = append(traceParents, j.TraceParent)
		return queue.Done()
	})
	h := a.Handler()
	post(t, h, "192.0.2.10:4000")
	post(t, h, "203.0.113.50:4000")
	if n, err := a.Runner.Drain(t.Context()); err != nil || n != 2 {
		t.Fatalf("Drain = %d, %v", n, err)
	}
	if !slices.Equal(traceParents, []string{"", ""}) {
		t.Errorf("TraceParents = %q, want none", traceParents)
	}
	var correlated, plain int
	for line := range strings.Lines(logs.String()) {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil || m["msg"] != "http request" {
			continue
		}
		switch m["trace_id"] {
		case incomingTraceID:
			correlated++
			if m["span_id"] != incomingSpanID {
				t.Errorf("span_id = %v, want the proxy's %s", m["span_id"], incomingSpanID)
			}
		case nil:
			plain++
		default:
			t.Errorf("trace_id = %v", m["trace_id"])
		}
	}
	if correlated != 1 || plain != 1 {
		t.Errorf("access log lines with the trace ID %d, without %d; want 1 each:\n%s", correlated, plain, logs)
	}
}
