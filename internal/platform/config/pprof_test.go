// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestPprofListen(t *testing.T) {
	t.Parallel()
	t.Run("off by default", func(t *testing.T) {
		t.Parallel()
		cfg, _, err := Load(source(minimal(), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.PprofListen != "" {
			t.Errorf("PprofListen = %q, want none", cfg.PprofListen)
		}
	})
	t.Run("an address of its own", func(t *testing.T) {
		t.Parallel()
		// The metrics listener and port 80 are not opened here, so their addresses are free.
		for _, addr := range []string{"127.0.0.1:6060", "127.0.0.1:9464", ":80"} {
			cfg, _, err := Load(source(with(minimal(), "MY_PPROF_LISTEN", addr), nil))
			if err != nil {
				t.Fatalf("%s: %v", addr, err)
			}
			if cfg.PprofListen != addr {
				t.Errorf("PprofListen = %q, want %q", cfg.PprofListen, addr)
			}
		}
	})

	files := func(kv ...string) map[string]string {
		return with(with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem"), kv...)
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
		msg  string
	}{
		{"an address that is not host:port", with(minimal(), "MY_PPROF_LISTEN", "6060"), "host:port"},
		{"on listen", with(minimal(), "MY_LISTEN", "127.0.0.1:8080", "MY_PPROF_LISTEN", "127.0.0.1:8080"), "must differ from listen"},
		{"on the default listen", with(minimal(), "MY_PPROF_LISTEN", ":8080"), "must differ from listen"},
		{"on the default https listen", files("MY_PPROF_LISTEN", ":443"), "must differ from listen"},
		{"on port 80", files("MY_PPROF_LISTEN", ":80"), "must differ from tls_http_listen"},
		{"on the metrics", with(minimal(), "MY_METRICS_EXPORTER", "prometheus", "MY_PPROF_LISTEN", "127.0.0.1:9464"), "must differ from metrics_listen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Load(source(tc.env, nil))
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Load = %v, want a ValidationError", err)
			}
			if !slices.ContainsFunc(verr.Issues, func(i Issue) bool {
				return i.Key == "pprof_listen" && strings.Contains(i.Message, tc.msg)
			}) {
				t.Errorf("issues = %v, want one of pprof_listen with %q", verr.Issues, tc.msg)
			}
		})
	}
}
