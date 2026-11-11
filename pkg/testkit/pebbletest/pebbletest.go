// SPDX-License-Identifier: AGPL-3.0-only

// Package pebbletest gives tests an ACME CA: Pebble (github.com/letsencrypt/pebble/v2, MPL-2.0),
// Let's Encrypt's test server, with pebble-challtestsrv as its DNS, which resolves every name to
// 127.0.0.1. The tests of the ACME adapter (internal/adapter/acme) and of the server with tls =
// acme (internal/platform) order real certificates from it (ADR-0163).
//
// Both run as Go tools pinned in tools/go.mod (`go tool -modfile=tools/go.mod pebble`), built
// once per test binary. Each New starts a Pebble of its own, which validates the HTTP-01
// challenges on 127.0.0.1:Options.HTTPPort and the TLS-ALPN-01 challenges on
// 127.0.0.1:Options.TLSPort: the test listens there. The tests run only with TEST_ACME=pebble
// (scripts/task.sh go:test-acme; CI with the contract suites): without it New skips the test.
package pebbletest

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/testkit"
)

// EnvACME is the environment variable that runs the tests: "pebble".
const EnvACME = "TEST_ACME"

// BlockedHost is a name the CA refuses to issue for (urn:ietf:params:acme:error:rejectedIdentifier).
const BlockedHost = "blocked-domain.example"

// ShortProfile is a profile the CA offers besides its default: certificates of ShortValidity.
// Pebble picks one of its profiles at random for an order that names none.
const (
	ShortProfile  = "shortlived"
	ShortValidity = 6 * 24 * time.Hour
)

// Options configure New.
type Options struct {
	// HTTPPort and TLSPort are the ports of 127.0.0.1 where the CA validates HTTP-01 and
	// TLS-ALPN-01; zero picks free ones.
	HTTPPort int
	TLSPort  int
}

// Server is the Pebble of one test.
type Server struct {
	// DirectoryURL is the CA's directory.
	DirectoryURL string
	// HTTPPort and TLSPort are where the CA validates the challenges.
	HTTPPort int
	TLSPort  int
	// roots trusts Pebble's own TLS certificate; mgmt is its management interface.
	roots *x509.CertPool
	mgmt  string
}

// HTTPClient returns a client that reaches the CA: it trusts Pebble's TLS certificate and
// follows no redirects, like the client of the server.
func (s Server) HTTPClient() *http.Client {
	return &http.Client{
		Timeout:       30 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: s.roots, MinVersion: tls.VersionTLS12}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Roots returns the roots that the certificates the CA issues chain to.
func (s Server) Roots(t testing.TB) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	b := s.get(t, s.mgmt+"/roots/0")
	if !pool.AppendCertsFromPEM(b) {
		t.Fatalf("pebbletest: no root in %q", b)
	}
	return pool
}

func (s Server) get(t testing.TB, url string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("pebbletest: GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("pebbletest: GET %s: %d %v", url, resp.StatusCode, err)
	}
	return b
}

// New starts a Pebble for t and stops it when t ends. It skips t unless TEST_ACME is "pebble".
func New(t testing.TB, opts Options) Server {
	t.Helper()
	if os.Getenv(EnvACME) != "pebble" {
		t.Skipf("the Pebble tests run with %s=pebble (scripts/task.sh go:test-acme)", EnvACME)
	}
	pebble, challtestsrv := tools(t)
	if opts.HTTPPort == 0 {
		opts.HTTPPort = FreePort(t)
	}
	if opts.TLSPort == 0 {
		opts.TLSPort = FreePort(t)
	}
	dir := t.TempDir()
	cert := testkit.NewTLSCertificate(t, 1, time.Now().Add(-time.Hour), 24*time.Hour, "127.0.0.1", "localhost")
	certFile, keyFile := cert.WriteFiles(t, dir, time.Time{})
	apiPort, mgmtPort, dnsPort, challMgmtPort := FreePort(t), FreePort(t), FreePort(t), FreePort(t)
	config := map[string]any{"pebble": map[string]any{
		"listenAddress":                  "127.0.0.1:" + strconv.Itoa(apiPort),
		"managementListenAddress":        "127.0.0.1:" + strconv.Itoa(mgmtPort),
		"certificate":                    certFile,
		"privateKey":                     keyFile,
		"httpPort":                       opts.HTTPPort,
		"tlsPort":                        opts.TLSPort,
		"ocspResponderURL":               "",
		"externalAccountBindingRequired": false,
		"domainBlocklist":                []string{BlockedHost},
		"retryAfter":                     map[string]int{"authz": 1, "order": 1},
		"keyAlgorithm":                   "ecdsa",
		"profiles": map[string]any{
			"default":    map[string]any{"description": "The default profile", "validityPeriod": 7776000},
			ShortProfile: map[string]any{"description": "Certificates of 6 days", "validityPeriod": 518400},
		},
	}}
	b, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(dir, "pebble-config.json")
	if err := os.WriteFile(configFile, b, 0o600); err != nil {
		t.Fatal(err)
	}
	dns := "127.0.0.1:" + strconv.Itoa(dnsPort)
	start(t, "pebble-challtestsrv", exec.Command(challtestsrv, //nolint:gosec // G204: the path of a pinned Go tool
		"-defaultIPv4", "127.0.0.1", "-defaultIPv6", "", "-dnsserver", dns, "-management", "127.0.0.1:"+strconv.Itoa(challMgmtPort),
		"-http01", "", "-https01", "", "-tlsalpn01", "", "-doh", ""))
	waitTCP(t, "127.0.0.1:"+strconv.Itoa(challMgmtPort))
	cmd := exec.Command(pebble, "-config", configFile, "-dnsserver", dns) //nolint:gosec // G204: the path of a pinned Go tool
	// No sleep before a validation, no nonce rejected at random, and no authorization reused,
	// so that every order validates its challenges.
	cmd.Env = append(os.Environ(), "PEBBLE_VA_NOSLEEP=1", "PEBBLE_WFE_NONCEREJECT=0", "PEBBLE_AUTHZREUSE=0")
	start(t, "pebble", cmd)
	s := Server{
		DirectoryURL: "https://127.0.0.1:" + strconv.Itoa(apiPort) + "/dir",
		HTTPPort:     opts.HTTPPort, TLSPort: opts.TLSPort,
		roots: cert.Pool(), mgmt: "https://127.0.0.1:" + strconv.Itoa(mgmtPort),
	}
	waitReady(t, s)
	return s
}

// FreePort returns a TCP port of 127.0.0.1 that nothing listens on now.
func FreePort(t testing.TB) int {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

var (
	toolsOnce sync.Once
	toolPaths [2]string
	toolsErr  error
)

// tools returns the paths of pebble and pebble-challtestsrv, built from tools/go.mod.
func tools(t testing.TB) (pebble, challtestsrv string) {
	t.Helper()
	toolsOnce.Do(func() {
		root, err := repositoryRoot()
		if err != nil {
			toolsErr = err
			return
		}
		for i, name := range []string{"pebble", "pebble-challtestsrv"} {
			cmd := exec.Command("go", "tool", "-modfile="+filepath.Join(root, "tools", "go.mod"), "-n", name) //nolint:gosec // G204: the names above, in the repository's tools/go.mod
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GOWORK=off")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				toolsErr = fmt.Errorf("go tool -n %s: %w: %s", name, err, stderr.String())
				return
			}
			toolPaths[i] = strings.TrimSpace(string(out))
		}
	})
	if toolsErr != nil {
		t.Fatalf("pebbletest: %v", toolsErr)
	}
	return toolPaths[0], toolPaths[1]
}

// repositoryRoot returns the directory above the working directory that holds tools/go.mod.
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "tools", "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no tools/go.mod above the working directory")
		}
		dir = parent
	}
}

// start runs cmd until t ends, and logs its output when t failed.
func start(t testing.TB, name string, cmd *exec.Cmd) {
	t.Helper()
	var out syncBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("pebbletest: start %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("%s:\n%s", name, out.String())
		}
	})
}

// waitTCP waits until addr accepts connections.
func waitTCP(t testing.TB, addr string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		c, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp", addr)
		if err == nil {
			_ = c.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pebbletest: %s does not accept connections: %v", addr, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitReady waits until the CA serves its directory.
func waitReady(t testing.TB, s Server) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.DirectoryURL, nil)
		resp, err := s.HTTPClient().Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		cancel()
		if err == nil && resp.StatusCode == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pebbletest: the CA does not serve %s: %v", s.DirectoryURL, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// syncBuffer is a bytes.Buffer that two processes' output may write at once.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// Answer serves the answers of the challenges where the CA validates them, until t ends: http01
// returns the body of the HTTP-01 resource of a token on 127.0.0.1:HTTPPort, and tlsALPN the
// certificate of a handshake that offers acme-tls/1 on 127.0.0.1:TLSPort. Call it once per Server.
func (s Server) Answer(t testing.TB, http01 func(ctx context.Context, token string) (string, error), tlsALPN func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	t.Helper()
	lc := net.ListenConfig{}
	hl, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:"+strconv.Itoa(s.HTTPPort))
	if err != nil {
		t.Fatal(err)
	}
	tl, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:"+strconv.Itoa(s.TLSPort))
	if err != nil {
		_ = hl.Close()
		t.Fatal(err)
	}
	hs := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.URL.Path, "/.well-known/acme-challenge/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		body, err := http01(r.Context(), token)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	})}
	ts := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.NotFoundHandler()}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"acme-tls/1"}, GetCertificate: tlsALPN}
	go func() { _ = hs.Serve(hl) }()
	go func() { _ = ts.Serve(tls.NewListener(tl, cfg)) }()
	t.Cleanup(func() {
		_ = hs.Close()
		_ = ts.Close()
	})
}
