// SPDX-License-Identifier: AGPL-3.0-only

//go:build unix && !no_otel

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/telemetry/otel/otlptest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

// setOTLPLogsEnv sets the environment of `my serve` with logs_exporter = otlp to a collector,
// and returns the collector.
func setOTLPLogsEnv(t *testing.T) *otlptest.LogCollector {
	t.Helper()
	c := &otlptest.LogCollector{}
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", srv.URL+"/v1/logs")
	t.Setenv("MY_LOGS_EXPORTER", config.LogsOTLP)
	t.Setenv("MY_HOST", "example.test")
	t.Setenv("MY_DATABASE_URL", sqlitetest.NewFile(t))
	t.Setenv("MY_BLOB_FS_DIR", t.TempDir())
	t.Setenv("MY_KEK", base64.StdEncoding.EncodeToString(kek))
	t.Setenv("MY_LISTEN", freeAddr(t))
	return c
}

// TestServeLogsBeforeTheAppOnStderr checks that with logs_exporter = otlp the logger that
// `my serve` hands to NewApp writes on standard error, so that the lines of the setup and the
// failure that ends it reach standard error before any exporter is ready.
func TestServeLogsBeforeTheAppOnStderr(t *testing.T) {
	c := setOTLPLogsEnv(t)
	newApp = func(_ context.Context, _ *config.Config, opts platform.AppOptions) (*platform.App, error) {
		if opts.Logger == nil {
			t.Error("NewApp got no Logger")
		} else {
			opts.Logger.Info("setting up the app")
		}
		return nil, errors.New("the setup broke")
	}
	t.Cleanup(func() { newApp = platform.NewApp })

	var stderr bytes.Buffer
	if code := run([]string{"serve"}, &bytes.Buffer{}, &stderr); code != 1 {
		t.Errorf("serve = %d, want 1; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"msg":"setting up the app"`) || !strings.Contains(stderr.String(), "the setup broke") {
		t.Errorf("stderr lacks the setup's line or its failure:\n%s", stderr.String())
	}
	if logs := c.Logs(); len(logs) != 0 {
		t.Errorf("the collector got %d records before the App was built: %+v", len(logs), logs)
	}
}

// TestServeStartupFailureWithOTLPLogsOnStderr checks that a failure of NewApp itself, with
// logs_exporter = otlp, appears on standard error.
func TestServeStartupFailureWithOTLPLogsOnStderr(t *testing.T) {
	setOTLPLogsEnv(t)
	t.Setenv("MY_ROLE", "worker")
	t.Setenv("MY_WORKER_JOB_TYPES", "federation.delivr")
	var stderr bytes.Buffer
	if code := run([]string{"serve"}, &bytes.Buffer{}, &stderr); code != 1 {
		t.Errorf("serve = %d, want 1; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no job type matches") {
		t.Errorf("stderr lacks the startup failure:\n%s", stderr.String())
	}
}
