// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

// TestWarnPlainWithoutProxy checks that with tls = off and no trusted_proxies, the first plain
// request from an address other than loopback logs the warning, once per start.
func TestWarnPlainWithoutProxy(t *testing.T) {
	t.Parallel()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	cases := []struct {
		name  string
		cfg   config.Config
		peers []string
		want  int
	}{
		{"loopback only", config.Config{TLS: config.TLSOff}, []string{"127.0.0.1:1000", "[::1]:1000"}, 0},
		{"once for many peers", config.Config{TLS: config.TLSOff}, []string{"127.0.0.1:1000", "192.0.2.1:1000", "198.51.100.2:1000", "[2001:db8::1]:1000"}, 1},
		{"behind trusted proxies", config.Config{TLS: config.TLSOff, TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}}, []string{"192.0.2.1:1000"}, 0},
		{"with tls", config.Config{TLS: config.TLSFiles}, []string{"192.0.2.1:1000"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			a := &App{Config: &tc.cfg, Logger: slog.New(slog.NewTextHandler(&logs, nil))}
			h := a.warnPlainWithoutProxy(ok)
			for _, peer := range tc.peers {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
				req.RemoteAddr = peer
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusNoContent {
					t.Fatalf("status = %d, want the next handler's", rec.Code)
				}
			}
			if got := strings.Count(logs.String(), "nothing seems to terminate TLS"); got != tc.want {
				t.Errorf("warnings = %d, want %d; logs:\n%s", got, tc.want, logs.String())
			}
		})
	}
}
