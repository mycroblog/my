// SPDX-License-Identifier: AGPL-3.0-only

//go:build spike_autocert

package main

// Spike #2823: what autocert adds to cmd/my. Reached only through MY_SPIKE_TLS.

import (
	"net/http"
	"os"

	"golang.org/x/crypto/acme/autocert"
)

func init() {
	if os.Getenv("MY_SPIKE_TLS") == "" {
		return
	}
	m := &autocert.Manager{Prompt: autocert.AcceptTOS, HostPolicy: autocert.HostWhitelist(os.Getenv("MY_SPIKE_TLS")), Cache: autocert.DirCache(os.TempDir())}
	s := &http.Server{TLSConfig: m.TLSConfig(), Handler: m.HTTPHandler(nil)} //nolint:gosec // spike
	_ = s.ListenAndServeTLS("", "")
}
