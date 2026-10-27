// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_otel

package platform_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform"
)

// TestOTLPErrorExporter wires errors_exporter = otlp: a report of the App reaches the OTLP/HTTP
// receiver as an exception log record by the time the App closes.
func TestOTLPErrorExporter(t *testing.T) {
	c := &otlptest.LogCollector{}
	srv := httptest.NewServer(c)
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", srv.URL+"/v1/logs")
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_ERRORS_EXPORTER": "otlp", "MY_ERRORS_ENVIRONMENT": "staging",
	})
	a, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Version: "9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	a.ErrorReporter.Report(t.Context(), kernel.NewErrorEvent(errors.New("boom"), "job.type"))
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	logs := c.Logs()
	if len(logs) != 1 {
		t.Fatalf("receiver got %d records, want 1", len(logs))
	}
	l := logs[0]
	if l.EventName != "exception" || l.SeverityText != "ERROR" {
		t.Errorf("event %q severity %q", l.EventName, l.SeverityText)
	}
	for k, v := range map[string]string{
		"exception.message": "boom", "my.error.where": "job.type", "deployment.environment.name": "staging",
	} {
		if l.Attrs[k] != v {
			t.Errorf("attribute %s = %q, want %q", k, l.Attrs[k], v)
		}
	}
	if l.Attrs["my.error.fingerprint"] == "" {
		t.Error("the record has no fingerprint")
	}
	if l.Resource["service.version"] != "9.9.9" {
		t.Errorf("resource service.version = %q", l.Resource["service.version"])
	}
}
