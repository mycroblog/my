// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/testkit"
)

func TestTLSSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		env        map[string]string
		listen     string
		serves     bool
		httpListen string
	}{
		{"off by default on :8080", minimal(), ":8080", false, ":80"},
		{"files on :443", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem"), ":443", true, ":80"},
		{"files on a listen of its own", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem", "MY_LISTEN", ":8443", "MY_TLS_HTTP_LISTEN", ""), ":8443", true, ""},
		{"a worker in plain HTTP", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem", "MY_ROLE", "worker"), ":8080", false, ":80"},
		{"web", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem", "MY_ROLE", "web"), ":443", true, ":80"},
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

func TestTLSSettingsInvalid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		env  map[string]string
		key  string
		msg  string
	}{
		{"an unknown mode", with(minimal(), "MY_TLS", "acme"), "tls", "unknown value"},
		{"files without a certificate", with(minimal(), "MY_TLS", "files", "MY_TLS_KEY_FILE", "k.pem"), "tls_cert_file", "required when tls is files"},
		{"files without a key", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem"), "tls_key_file", "required when tls is files"},
		{"port 80 on listen", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem", "MY_TLS_HTTP_LISTEN", ":443"), "tls_http_listen", "must differ from listen"},
		{"port 80 on the metrics", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem", "MY_METRICS_EXPORTER", "prometheus", "MY_METRICS_LISTEN", ":80"), "tls_http_listen", "must differ from metrics_listen"},
		{"an address that is not host:port", with(minimal(), "MY_TLS_HTTP_LISTEN", "80"), "tls_http_listen", "host:port"},
		{"metrics on the default https listen", with(minimal(), "MY_TLS", "files", "MY_TLS_CERT_FILE", "c.pem", "MY_TLS_KEY_FILE", "k.pem", "MY_METRICS_EXPORTER", "prometheus", "MY_METRICS_LISTEN", ":443"), "metrics_listen", "must differ from listen"},
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

func TestTLSFindings(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	files := func(name string, c testkit.TLSCertificate) (string, string) {
		d := filepath.Join(dir, name)
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		return c.WriteFiles(t, d, time.Time{})
	}
	good := testkit.NewTLSCertificate(t, 1, now.Add(-time.Hour), 90*24*time.Hour, "social.example")
	goodCert, goodKey := files("good", good)
	other := testkit.NewTLSCertificate(t, 2, now.Add(-time.Hour), 90*24*time.Hour, "social.example")
	_, otherKey := files("other", other)
	wrongHostCert, wrongHostKey := files("host", testkit.NewTLSCertificate(t, 3, now.Add(-time.Hour), 90*24*time.Hour, "elsewhere.example"))
	expiredCert, expiredKey := files("expired", testkit.NewTLSCertificate(t, 4, now.Add(-91*24*time.Hour), 90*24*time.Hour, "social.example"))
	soonCert, soonKey := files("soon", testkit.NewTLSCertificate(t, 5, now.Add(-80*24*time.Hour), 90*24*time.Hour, "social.example"))
	futureCert, futureKey := files("future", testkit.NewTLSCertificate(t, 6, now.Add(time.Hour), 90*24*time.Hour, "social.example"))

	tlsFiles := func(cert, key string) Config {
		return Config{Host: "social.example", TLS: TLSFiles, TLSCertFile: cert, TLSKeyFile: key, Listen: ":443"}
	}
	proxies := []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
	cases := []struct {
		name string
		cfg  Config
		want []string // severity:key:fragment
	}{
		{"usable files", tlsFiles(goodCert, goodKey), nil},
		{"a port on host", Config{Host: "social.example:8443", TLS: TLSFiles, TLSCertFile: goodCert, TLSKeyFile: goodKey, Listen: ":8443"}, nil},
		{"an unreadable certificate", tlsFiles(filepath.Join(dir, "missing.pem"), goodKey), []string{"error:tls_cert_file:cannot be used"}},
		{"an unreadable key", tlsFiles(goodCert, filepath.Join(dir, "missing.pem")), []string{"error:tls_cert_file:cannot be used"}},
		{"a key of another certificate", tlsFiles(goodCert, otherKey), []string{"error:tls_cert_file:cannot be used"}},
		{"a certificate of another host", tlsFiles(wrongHostCert, wrongHostKey), []string{"error:tls_cert_file:does not cover host social.example"}},
		{"an expired certificate", tlsFiles(expiredCert, expiredKey), []string{"error:tls_cert_file:expired"}},
		{"a certificate not valid yet", tlsFiles(futureCert, futureKey), []string{"error:tls_cert_file:not valid before"}},
		{"fewer than 14 days left", tlsFiles(soonCert, soonKey), []string{"warning:tls_cert_file:in less than 14 days"}},
		{"trusted proxies with tls", func() Config { c := tlsFiles(goodCert, goodKey); c.TrustedProxies = proxies; return c }(), []string{"warning:trusted_proxies:leave it empty"}},
		{"off on :443 of every address", Config{TLS: TLSOff, Listen: ":443"}, []string{"warning:tls:nothing seems to terminate TLS"}},
		{"off on :80 of a public address", Config{TLS: TLSOff, Listen: "203.0.113.1:80"}, []string{"warning:tls:nothing seems to terminate TLS"}},
		{"off on :443 of loopback", Config{TLS: TLSOff, Listen: "127.0.0.1:443"}, nil},
		{"off on :443 of localhost", Config{TLS: TLSOff, Listen: "localhost:443"}, nil},
		{"off on ::1", Config{TLS: TLSOff, Listen: "[::1]:80"}, nil},
		{"off on :8080", Config{TLS: TLSOff, Listen: ":8080"}, nil},
		{"off behind proxies", Config{TLS: TLSOff, Listen: ":8080", TrustedProxies: proxies}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, f := range TLSFindings(&tc.cfg, now) {
				got = append(got, string(f.Severity)+":"+f.Key+":"+f.Message)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("TLSFindings = %q, want %q", got, tc.want)
			}
			for i, w := range tc.want {
				parts := strings.SplitN(w, ":", 3)
				if !strings.HasPrefix(got[i], parts[0]+":"+parts[1]+":") || !strings.Contains(got[i], parts[2]) {
					t.Errorf("finding %d = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}
