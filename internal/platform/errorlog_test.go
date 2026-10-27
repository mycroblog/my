// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

func TestLogSeverity(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelDebug)
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError, slog.LevelError + 4} {
		logger.Log(context.Background(), level, "x", slog.String("other", "y"))
	}
	logger.WithGroup("g").Info("grouped", slog.Any("level", slog.LevelWarn))
	want := []struct{ level, severity string }{
		{"DEBUG", "DEBUG"}, {"INFO", "INFO"}, {"WARN", "WARNING"}, {"ERROR", "ERROR"}, {"ERROR+4", "CRITICAL"}, {"INFO", "INFO"},
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != len(want) {
		t.Fatalf("%d lines, want %d:\n%s", len(lines), len(want), buf.String())
	}
	for i, line := range lines {
		var got map[string]any
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if got["level"] != want[i].level || got["severity"] != want[i].severity {
			t.Errorf("line %d: level %v severity %v, want %s %s", i, got["level"], got["severity"], want[i].level, want[i].severity)
		}
		if !strings.HasPrefix(line, `{"time":`) || !strings.Contains(line, `"level":"`+want[i].level+`","severity":`) {
			t.Errorf("line %d: severity does not follow level: %s", i, line)
		}
	}
	if !strings.Contains(lines[5], `"g":{"level":"WARN"}`) {
		t.Errorf("an attribute named level in a group got a severity: %s", lines[5])
	}
}

func TestErrorLogLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := NewErrorReporter("1.2.3", "worker", "staging", nil, nil)
	r.AddSink(NewErrorLogSink(NewLogger(&buf, slog.LevelInfo)))
	ev := kernel.NewErrorEvent(errors.New("deliver to https://remote.example/inbox: refused"), "deliver")
	ev.JobID, ev.Attempt, ev.TraceID, ev.SpanID = "job-1", 3, "trace-1", "span-1"
	ev.Time = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	r.Report(context.Background(), ev)

	line := strings.TrimSpace(buf.String())
	if strings.Count(line, "\n") != 0 || line == "" {
		t.Fatalf("want one line, got:\n%s", buf.String())
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"msg": "error report", "level": "ERROR", "severity": "ERROR",
		"@type":           "type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent",
		"my.error.source": "server", "my.error.kind": "error", "my.error.where": "deliver", "my.role": "worker",
		"deployment.environment.name": "staging", "job_id": "job-1", "attempt": float64(3), "trace_id": "trace-1", "span_id": "span-1",
	} {
		if got[key] != want {
			t.Errorf("%s = %v, want %v", key, got[key], want)
		}
	}
	if sc, _ := got["serviceContext"].(map[string]any); sc["service"] != "my" || sc["version"] != "1.2.3" {
		t.Errorf("serviceContext = %v", got["serviceContext"])
	}
	if fp, _ := got["my.error.fingerprint"].(string); len(fp) != 32 {
		t.Errorf("fingerprint = %q", fp)
	}
	msg, _ := got["message"].(string)
	header, stack, _ := strings.Cut(msg, "\n")
	if header != "*errors.errorString: deliver to <url>: refused" {
		t.Errorf("header = %q", header)
	}
	if !strings.HasPrefix(stack, "goroutine ") || strings.Contains(stack, "kernel.NewErrorEvent") {
		t.Errorf("stack =\n%s", stack)
	}
	if !strings.HasSuffix(line, `"}`) || strings.LastIndex(line, `"message":`) < strings.LastIndex(line, `"span_id":`) {
		t.Errorf("message is not the last field: %s", line)
	}
	if _, ok := got["request_id"]; ok {
		t.Error("request_id written without a request")
	}
}

func TestErrorLogLineFatal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ReportStartupFailure(context.Background(), NewLogger(&buf, slog.LevelInfo), "1.2.3", "web", "production", errors.New("database: connection refused"))
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("%v:\n%s", err, buf.String())
	}
	if got["severity"] != "CRITICAL" || got["my.error.where"] != "startup" || got["my.role"] != "web" {
		t.Errorf("line = %v", got)
	}
}

func TestClientGoneIsNotReported(t *testing.T) {
	t.Parallel()
	r := NewErrorReporter("dev", "all", "production", nil, nil)
	var n int
	r.AddSink(sinkFunc(func(context.Context, kernel.ErrorEvent) { n++ }))
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), requestRouteKey{}, &requestRoute{route: "/x", surface: "web", method: "GET"}))
	cancel()
	r.Report(ctx, kernel.NewErrorEvent(context.Canceled, ""))
	if n != 0 {
		t.Errorf("the error of a request whose client left was reported")
	}
	r.Report(ctx, kernel.PanicEvent("boom", nil, ""))
	if n != 1 {
		t.Errorf("a panic of a request whose client left was not reported")
	}
}
