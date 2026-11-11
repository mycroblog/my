// SPDX-License-Identifier: AGPL-3.0-only

//go:build spike_certmagic

package main

// Spike #2823: what certmagic adds to cmd/my. Reached only through MY_SPIKE_TLS.

import (
	"context"
	"net/http"
	"os"

	"github.com/caddyserver/certmagic"
)

func init() {
	if os.Getenv("MY_SPIKE_TLS") == "" {
		return
	}
	cfg := certmagic.NewDefault()
	iss := certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{CA: certmagic.LetsEncryptStagingCA, Agreed: true, Email: "ops@example.com"})
	cfg.Issuers = []certmagic.Issuer{iss}
	_ = cfg.ManageSync(context.Background(), []string{os.Getenv("MY_SPIKE_TLS")})
	s := &http.Server{TLSConfig: cfg.TLSConfig(), Handler: iss.HTTPChallengeHandler(http.NotFoundHandler())} //nolint:gosec // spike
	_ = s.ListenAndServeTLS("", "")
}
