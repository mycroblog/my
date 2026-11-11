// SPDX-License-Identifier: AGPL-3.0-only

package tlsserve_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform/tlsserve"
	"github.com/mycroblog/my/pkg/testkit"
)

var day = 24 * time.Hour

func newCert(t *testing.T, serial int64, hosts ...string) testkit.TLSCertificate {
	t.Helper()
	return testkit.NewTLSCertificate(t, serial, time.Now().Add(-time.Hour), 90*day, hosts...)
}

func keyPair(t *testing.T, c testkit.TLSCertificate) *tls.Certificate {
	t.Helper()
	pair, err := tls.X509KeyPair(c.CertPEM, c.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return &pair
}

func TestHolderGetCertificate(t *testing.T) {
	t.Parallel()
	h := tlsserve.NewHolder("Social.Example:8443", "web.example", "social.example")
	if got := h.Hosts(); len(got) != 2 || got[0] != "social.example" || got[1] != "web.example" {
		t.Fatalf("Hosts = %v, want social.example and web.example", got)
	}
	cert := keyPair(t, newCert(t, 1, "social.example"))
	if err := h.Set("other.example", cert); err == nil {
		t.Error("Set of a host the holder does not serve = nil, want an error")
	}
	if _, err := h.GetCertificate(&tls.ClientHelloInfo{ServerName: "social.example"}); err == nil {
		t.Error("GetCertificate before Set = nil, want an error")
	}
	if err := h.Set("SOCIAL.example.", cert); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"social.example", "Social.Example", "social.example."} {
		if got, err := h.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err != nil || got != cert {
			t.Errorf("GetCertificate(%q) = %v, %v; want the certificate set", name, got, err)
		}
	}
	for _, name := range []string{"", "other.example", "192.0.2.1"} {
		if _, err := h.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); !errors.Is(err, tlsserve.ErrUnknownHost) {
			t.Errorf("GetCertificate(%q) = %v, want ErrUnknownHost", name, err)
		}
	}
	if _, err := h.GetCertificate(&tls.ClientHelloInfo{ServerName: "web.example"}); err == nil {
		t.Error("GetCertificate of a host without a certificate = nil, want an error")
	}

	// The handshakes that offer only acme-tls/1 go to the challenge, and fail without one.
	alpn := &tls.ClientHelloInfo{ServerName: "social.example", SupportedProtos: []string{tlsserve.ACMETLS1Protocol}}
	if _, err := h.GetCertificate(alpn); err == nil {
		t.Error("GetCertificate of acme-tls/1 without a challenge = nil, want an error")
	}
	if protos := h.Config().NextProtos; strings.Contains(strings.Join(protos, ","), tlsserve.ACMETLS1Protocol) {
		t.Errorf("NextProtos without a challenge = %v, want no acme-tls/1", protos)
	}
	challenge := keyPair(t, newCert(t, 2, "social.example"))
	h.SetALPNChallenge(func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return challenge, nil })
	if got, err := h.GetCertificate(alpn); err != nil || got != challenge {
		t.Errorf("GetCertificate of acme-tls/1 = %v, %v; want the challenge's certificate", got, err)
	}
	if cfg := h.Config(); cfg.MinVersion != tls.VersionTLS12 || cfg.CipherSuites != nil || cfg.NextProtos[0] != "h2" {
		t.Errorf("Config = MinVersion %x, CipherSuites %v, NextProtos %v; want TLS 1.2, Go's defaults, h2 first", cfg.MinVersion, cfg.CipherSuites, cfg.NextProtos)
	}
}

// TestHandshakeOfUnknownServerNameFails checks that a server name without a certificate fails the
// handshake itself, before any request.
func TestHandshakeOfUnknownServerNameFails(t *testing.T) {
	t.Parallel()
	c := newCert(t, 1, "social.example", "other.example")
	h := tlsserve.NewHolder("social.example")
	if err := h.Set("social.example", keyPair(t, c)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	srv.TLS = h.Config()
	srv.StartTLS()
	t.Cleanup(srv.Close)
	dial := func(name string) error {
		conn, err := tls.Dial("tcp", srv.Listener.Addr().String(), &tls.Config{ServerName: name, RootCAs: c.Pool(), MinVersion: tls.VersionTLS12})
		if err == nil {
			_ = conn.Close()
		}
		return err
	}
	if err := dial("social.example"); err != nil {
		t.Errorf("handshake of social.example: %v", err)
	}
	if err := dial("other.example"); err == nil {
		t.Error("handshake of a server name the holder does not serve succeeded")
	}
}

func TestFileSourceReloadsChangedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := newCert(t, 1, "social.example")
	start := time.Now().Add(-time.Hour)
	certFile, keyFile := first.WriteFiles(t, dir, start)
	h := tlsserve.NewHolder("social.example", "media.example")
	s := tlsserve.NewFileSource(h, certFile, keyFile, slog.New(slog.DiscardHandler))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	served := func() []byte {
		c := h.Certificate("social.example")
		if c == nil {
			t.Fatal("no certificate for social.example")
		}
		return c.Certificate[0]
	}
	if !bytes.Equal(served(), first.Leaf.Raw) {
		t.Fatal("Load did not put the certificate of the files")
	}
	if h.Certificate("media.example") != nil {
		t.Error("the certificate serves media.example, which it does not cover")
	}
	if ok, err := s.Reload(); ok || err != nil {
		t.Errorf("Reload of unchanged files = %t, %v; want nothing done", ok, err)
	}

	// A new certificate whose key is not written yet keeps the old one.
	second := newCert(t, 2, "social.example")
	second.WriteFiles(t, dir, start.Add(time.Minute))
	if err := writeFile(keyFile, first.KeyPEM, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reload(); err == nil {
		t.Error("Reload of a certificate with another's key = nil, want an error")
	}
	if !bytes.Equal(served(), first.Leaf.Raw) {
		t.Error("a failed reload replaced the certificate")
	}
	// Once the key follows, the next check picks the pair up.
	if err := writeFile(keyFile, second.KeyPEM, start.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Reload(); !ok || err != nil {
		t.Fatalf("Reload of replaced files = %t, %v; want reloaded", ok, err)
	}
	if !bytes.Equal(served(), second.Leaf.Raw) {
		t.Error("Reload did not put the new certificate")
	}

	// A certificate that covers none of the hosts is refused.
	elsewhere := newCert(t, 3, "elsewhere.example")
	elsewhere.WriteFiles(t, dir, start.Add(3*time.Minute))
	if _, err := s.Reload(); err == nil {
		t.Error("Reload of a certificate for another host = nil, want an error")
	}
	if !bytes.Equal(served(), second.Leaf.Raw) {
		t.Error("a refused reload replaced the certificate")
	}
}

func writeFile(name string, data []byte, modTime time.Time) error {
	if err := os.WriteFile(name, data, 0o600); err != nil {
		return err
	}
	return os.Chtimes(name, modTime, modTime)
}

func TestFileSourceWatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := newCert(t, 1, "social.example")
	certFile, keyFile := first.WriteFiles(t, dir, time.Now().Add(-time.Hour))
	h := tlsserve.NewHolder("social.example")
	s := tlsserve.NewFileSource(h, certFile, keyFile, slog.New(slog.DiscardHandler))
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Watch(ctx, 10*time.Millisecond); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	second := newCert(t, 2, "social.example")
	second.WriteFiles(t, dir, time.Now())
	deadline := time.Now().Add(5 * time.Second)
	for !bytes.Equal(h.Certificate("social.example").Certificate[0], second.Leaf.Raw) {
		if time.Now().After(deadline) {
			t.Fatal("Watch did not load the replaced certificate")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRedirectHandler(t *testing.T) {
	t.Parallel()
	h := tlsserve.NewHolder("social.example", "web.example")
	challenge := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "token:"+strings.TrimPrefix(r.URL.Path, tlsserve.ACMEChallengePrefix))
	})
	cases := []struct {
		name       string
		handler    http.Handler
		method     string
		target     string
		host       string
		status     int
		location   string
		body       string
		bodyAbsent bool
	}{
		{"GET to the same path", tlsserve.RedirectHandler(h, "social.example", "443", nil), http.MethodGet, "/@alice?x=1", "social.example", http.StatusMovedPermanently, "https://social.example/@alice?x=1", "", true},
		{"HEAD", tlsserve.RedirectHandler(h, "social.example", "443", nil), http.MethodHead, "/about", "web.example:80", http.StatusMovedPermanently, "https://web.example/about", "", true},
		{"an unknown host goes to host", tlsserve.RedirectHandler(h, "social.example", "443", nil), http.MethodGet, "/", "evil.example", http.StatusMovedPermanently, "https://social.example/", "", true},
		{"another https port", tlsserve.RedirectHandler(h, "social.example", "8443", nil), http.MethodGet, "/x", "social.example", http.StatusMovedPermanently, "https://social.example:8443/x", "", true},
		{"POST", tlsserve.RedirectHandler(h, "social.example", "443", nil), http.MethodPost, "/inbox", "social.example", http.StatusBadRequest, "", "", true},
		{"PUT", tlsserve.RedirectHandler(h, "social.example", "443", nil), http.MethodPut, "/x", "social.example", http.StatusBadRequest, "", "", true},
		{"a challenge without a handler redirects", tlsserve.RedirectHandler(h, "social.example", "443", nil), http.MethodGet, "/.well-known/acme-challenge/abc", "social.example", http.StatusMovedPermanently, "https://social.example/.well-known/acme-challenge/abc", "", true},
		{"a challenge with a handler", tlsserve.RedirectHandler(h, "social.example", "443", challenge), http.MethodGet, "/.well-known/acme-challenge/abc", "social.example", http.StatusOK, "", "token:abc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.target, strings.NewReader("secret=1"))
			req.Host = tc.host
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Header().Get("Location") != tc.location {
				t.Errorf("%s %s = %d Location %q, want %d %q", tc.method, tc.target, rec.Code, rec.Header().Get("Location"), tc.status, tc.location)
			}
			if tc.bodyAbsent && rec.Body.Len() != 0 {
				t.Errorf("body = %q, want none", rec.Body.String())
			}
			if !tc.bodyAbsent && rec.Body.String() != tc.body {
				t.Errorf("body = %q, want %q", rec.Body.String(), tc.body)
			}
		})
	}
}
