// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_acme

package platform

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/adapter/acme"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/platform/config"
)

// newACMEClient returns the client of the CA of tls = acme (ADR-0163 §2).
func newACMEClient(cfg *config.Config, logger *slog.Logger) (instance.ACMEClient, error) {
	return acme.New(acme.Options{DirectoryURL: cfg.TLSACMEDirectory, HTTPClient: acmeHTTPClient(), Logger: logger})
}

// acmeHTTPClient returns the HTTP client of the ACME CA (ADR-0163 §2). The CA is the operator's
// choice (tls_acme_directory), so the client is not the SSRF-guarded kernel.HTTPClient; it
// follows no redirect, so that the requests and the account's signatures reach that CA only.
func acmeHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
