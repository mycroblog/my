// SPDX-License-Identifier: AGPL-3.0-only

//go:build spike_acmez

package main

// Spike #2823: what the acmez-based manager of internal/spike/acmetls adds to cmd/my. Reached
// only through MY_SPIKE_TLS.

import (
	"context"
	"net/http"
	"os"

	"github.com/mycroblog/my/internal/spike/acmetls"
)

func init() {
	if os.Getenv("MY_SPIKE_TLS") == "" {
		return
	}
	m := &acmetls.Manager{Hosts: []string{os.Getenv("MY_SPIKE_TLS")}, Directory: "https://acme-staging-v02.api.letsencrypt.org/directory", RenewRatio: 2.0 / 3, Lease: &acmetls.Lease{}}
	_ = m.Maintain(context.Background())
	go m.Listen(context.Background())
	s := &http.Server{TLSConfig: m.TLSConfig(), Handler: m.HTTPHandler()} //nolint:gosec // spike
	_ = s.ListenAndServeTLS("", "")
}
