// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_acme

package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestTLSACMESettings(t *testing.T) {
	t.Parallel()
	cfg, _, err := Load(source(with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true"), nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLS != TLSACME || cfg.TLSACMEDirectory != DefaultTLSACMEDirectory || !cfg.TLSACMEAgreeTerms || cfg.TLSACMEEmail != "" || cfg.TLSACMEProfile != "" {
		t.Errorf("defaults = %q %q %t %q %q", cfg.TLS, cfg.TLSACMEDirectory, cfg.TLSACMEAgreeTerms, cfg.TLSACMEEmail, cfg.TLSACMEProfile)
	}
	cfg, _, err = Load(source(with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true",
		"MY_TLS_ACME_DIRECTORY", "https://acme-staging-v02.api.letsencrypt.org/directory", "MY_TLS_ACME_EMAIL", "ops@example.org",
		"MY_TLS_ACME_PROFILE", "shortlived"), nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLSACMEDirectory != "https://acme-staging-v02.api.letsencrypt.org/directory" || cfg.TLSACMEEmail != "ops@example.org" || cfg.TLSACMEProfile != "shortlived" {
		t.Errorf("settings = %q %q %q", cfg.TLSACMEDirectory, cfg.TLSACMEEmail, cfg.TLSACMEProfile)
	}
}

func TestTLSACMESettingsValid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		env        map[string]string
		listen     string
		serves     bool
		httpListen string
	}{
		{"acme on :443", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true"), ":443", true, ":80"},
		{"acme with kv = memory in one process", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_KV", "memory"), ":443", true, ":80"},
		{"acme with TLS-ALPN-01 alone", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_TLS_HTTP_LISTEN", ""), ":443", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, _, err := Load(source(tc.env, nil))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Listen != tc.listen || cfg.ServesTLS() != tc.serves || cfg.TLSHTTPListen != tc.httpListen {
				t.Errorf("listen %q, ServesTLS %t, tls_http_listen %q; want %q, %t, %q", cfg.Listen, cfg.ServesTLS(), cfg.TLSHTTPListen, tc.listen, tc.serves, tc.httpListen)
			}
		})
	}
}

func TestTLSACMESettingsInvalid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		env  map[string]string
		key  string
		msg  string
	}{
		{"acme without agreeing to the terms", with(minimal(), "MY_TLS", "acme"), "tls_acme_agree_terms", "must be true when tls is acme"},
		{"acme with the terms refused", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "false"), "tls_acme_agree_terms", "must be true when tls is acme"},
		{"acme on an IP address", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_HOST", "192.0.2.1"), "host", ""},
		{"acme on localhost", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_HOST", "localhost:3000"), "host", "must be a DNS name with a dot when tls is acme"},
		{"acme on a subdomain of localhost", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_HOST", "my.localhost"), "host", "must be a DNS name with a dot when tls is acme"},
		{"acme on a name without a dot", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_HOST", "intranet"), "host", "must be a DNS name with a dot when tls is acme"},
		{"acme with a directory over http", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_TLS_ACME_DIRECTORY", "http://ca.example/directory"), "tls_acme_directory", "https URL"},
		{"acme with a directory that is not a URL", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_TLS_ACME_DIRECTORY", "ca.example"), "tls_acme_directory", "https URL"},
		{"acme with a contact that is not an address", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_TLS_ACME_EMAIL", "Ops <ops@example.org>"), "tls_acme_email", "bare email address"},
		{"acme in a web process with kv = memory", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_KV", "memory", "MY_ROLE", "web"), "kv", "every process shares"},
		{"acme in a worker with kv = memory", with(minimal(), "MY_TLS", "acme", "MY_TLS_ACME_AGREE_TERMS", "true", "MY_KV", "memory", "MY_ROLE", "worker"), "kv", "every process shares"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Load(source(tc.env, nil))
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Load = %v, want a ValidationError", err)
			}
			if !slices.ContainsFunc(verr.Issues, func(i Issue) bool {
				return i.Key == tc.key && strings.Contains(i.Message, tc.msg)
			}) {
				t.Errorf("issues = %v, want one of %s with %q", verr.Issues, tc.key, tc.msg)
			}
		})
	}
}
