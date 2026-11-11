// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
)

// tlsServer is a server started with tls = files.
type tlsServer struct {
	*server
	addr     string
	httpURL  string
	certFile string
	keyFile  string
}

func startTLS(t *testing.T, env map[string]string, cert testkit.TLSCertificate) *tlsServer {
	t.Helper()
	dir := t.TempDir()
	certFile, keyFile := cert.WriteFiles(t, dir, time.Now().Add(-time.Hour))
	env["MY_DATABASE_URL"] = newDatabase(t, true)
	env["MY_TLS"] = "files"
	env["MY_TLS_CERT_FILE"] = certFile
	env["MY_TLS_KEY_FILE"] = keyFile
	cfg := loadConfig(t, env)
	httpLn, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := start(t, cfg, platform.ServeOptions{HTTPListener: httpLn, TLSFileCheckInterval: 20 * time.Millisecond}, nil)
	return &tlsServer{
		server: s, addr: strings.TrimPrefix(s.url, "http://"), httpURL: "http://" + httpLn.Addr().String(),
		certFile: certFile, keyFile: keyFile,
	}
}

// client returns a client that trusts roots and connects every name to the server.
func (s *tlsServer) client(roots testkit.TLSCertificate, serverName string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots.Pool(), ServerName: serverName, MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, s.addr)
			},
			ForceAttemptHTTP2: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// peerCertificate returns the certificate that the server presents for serverName.
func (s *tlsServer) peerCertificate(t *testing.T, serverName string) ([]byte, error) {
	t.Helper()
	conn, err := tls.Dial("tcp", s.addr, &tls.Config{ServerName: serverName, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec // G402: the test reads the certificate presented
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	return conn.ConnectionState().PeerCertificates[0].Raw, nil
}

func TestServeTLSFiles(t *testing.T) {
	t.Parallel()
	first := testkit.NewTLSCertificate(t, 1, time.Now().Add(-time.Hour), 90*24*time.Hour, "example.test")
	s := startTLS(t, map[string]string{}, first)

	resp, err := s.client(first, "example.test").Get("https://example.test" + platform.PathLiveness)
	if err != nil {
		t.Fatalf("GET over https: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ProtoMajor != 2 || !strings.Contains(string(body), "ok") {
		t.Errorf("GET over https = %d %s %q, want 200 over HTTP/2", resp.StatusCode, resp.Proto, body)
	}
	if got := resp.Header.Get("Strict-Transport-Security"); got == "" {
		t.Error("no Strict-Transport-Security over https")
	}

	t.Run("an unknown server name fails the handshake", func(t *testing.T) {
		if _, err := s.peerCertificate(t, "other.test"); err == nil {
			t.Error("handshake of other.test succeeded")
		}
		if _, err := s.peerCertificate(t, ""); err == nil {
			t.Error("handshake without a server name succeeded")
		}
	})

	t.Run("plain HTTP on listen gets no answer", func(t *testing.T) {
		resp, err := http.Get(s.url + platform.PathLiveness) //nolint:noctx // a test of the listener
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Error("plain HTTP on the TLS listener answered 200")
			}
		}
	})

	t.Run("port 80 redirects", func(t *testing.T) {
		noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			req, _ := http.NewRequestWithContext(t.Context(), method, s.httpURL+"/@alice?page=2", nil)
			req.Host = "example.test"
			resp, err := noFollow.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "https://example.test/@alice?page=2" {
				t.Errorf("%s on port 80 = %d %q, want 301 to https://example.test/@alice?page=2", method, resp.StatusCode, resp.Header.Get("Location"))
			}
		}
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.httpURL+"/inbox", strings.NewReader("{}"))
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("POST on port 80 = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("a replaced certificate is served without a restart", func(t *testing.T) {
		second := testkit.NewTLSCertificate(t, 2, time.Now().Add(-time.Hour), 90*24*time.Hour, "example.test")
		second.WriteFiles(t, strings.TrimSuffix(s.certFile, "/cert.pem"), time.Now())
		deadline := time.Now().Add(5 * time.Second)
		for {
			raw, err := s.peerCertificate(t, "example.test")
			if err == nil && bytes.Equal(raw, second.Leaf.Raw) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the server still presents the replaced certificate (%v)", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

// TestServeTLSWorkerListensInPlainHTTP checks that the worker role never opens a TLS listener,
// nor port 80: it serves its health checks in plain HTTP whatever tls says.
func TestServeTLSWorkerListensInPlainHTTP(t *testing.T) {
	t.Parallel()
	cert := testkit.NewTLSCertificate(t, 1, time.Now().Add(-time.Hour), 90*24*time.Hour, "example.test")
	certFile, keyFile := cert.WriteFiles(t, t.TempDir(), time.Time{})
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_ROLE": "worker",
		"MY_TLS": "files", "MY_TLS_CERT_FILE": certFile, "MY_TLS_KEY_FILE": keyFile,
	})
	if cfg.Listen != ":8080" || cfg.ServesTLS() {
		t.Errorf("worker with tls = files: listen %s, ServesTLS %t; want :8080 in plain HTTP", cfg.Listen, cfg.ServesTLS())
	}
	httpLn, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := start(t, cfg, platform.ServeOptions{HTTPListener: httpLn}, nil)
	if s.app.TLSCertificates != nil {
		t.Error("the worker has certificates in memory")
	}
	if resp, _ := s.do(t, http.MethodGet, platform.PathLiveness, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s in plain HTTP = %d, want 200", platform.PathLiveness, resp.StatusCode)
	}
	if conn, err := net.DialTimeout("tcp", httpLn.Addr().String(), time.Second); err == nil {
		_ = conn.Close()
		t.Error("the worker serves port 80")
	}
}

// TestServeTLSOffKeepsPlainHTTP checks that tls = off listens in plain HTTP on :8080 by default,
// as before tls existed, and opens no port 80.
func TestServeTLSOffKeepsPlainHTTP(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	if cfg.TLS != config.TLSOff || cfg.Listen != ":8080" || cfg.ServesTLS() {
		t.Errorf("default: tls %q, listen %s, ServesTLS %t; want off on :8080", cfg.TLS, cfg.Listen, cfg.ServesTLS())
	}
	s := start(t, cfg, platform.ServeOptions{}, nil)
	if s.app.TLSCertificates != nil {
		t.Error("tls = off has certificates in memory")
	}
	if resp, _ := s.do(t, http.MethodGet, platform.PathLiveness, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s = %d, want 200", platform.PathLiveness, resp.StatusCode)
	}
}

// TestServeTLSRefusesUnusableFiles checks that the server does not start on files it cannot use.
func TestServeTLSRefusesUnusableFiles(t *testing.T) {
	t.Parallel()
	cert := testkit.NewTLSCertificate(t, 1, time.Now().Add(-time.Hour), 90*24*time.Hour, "elsewhere.test")
	certFile, keyFile := cert.WriteFiles(t, t.TempDir(), time.Time{})
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true),
		"MY_TLS": "files", "MY_TLS_CERT_FILE": certFile, "MY_TLS_KEY_FILE": keyFile,
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: platform.NewLogger(io.Discard, slog.LevelError)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := platform.Serve(t.Context(), app, platform.ServeOptions{Listener: ln, HTTPListener: newLocalListener(t)}); err == nil || !strings.Contains(err.Error(), "covers none") {
		t.Errorf("Serve with a certificate for another host = %v, want an error", err)
	}
}

func newLocalListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}
