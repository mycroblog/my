// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"net/http"

	"github.com/mycroblog/my/internal/adapter/errorreport/sentry"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { errorExporters[sentry.Name] = newSentryErrorExporter }

// newSentryErrorExporter returns the exporter of errors_exporter = sentry (ADR-0103 §4 (3)): the
// shared send queue in front of the envelope client of errors_sentry_dsn. The DSN names the
// service that the operator configured, so the HTTP client is not the SSRF-guarded
// kernel.HTTPClient; one envelope has the queue's 10 seconds.
func newSentryErrorExporter(_ context.Context, a *App, cfg *config.Config, opts AppOptions) (ErrorSink, error) {
	c, err := sentry.New(sentry.Options{DSN: cfg.ErrorsSentryDSN, HTTPClient: &http.Client{Timeout: exportSendTimeout}, Version: opts.Version})
	if err != nil {
		return nil, err
	}
	return NewExportQueue(c, a.Meter), nil
}
