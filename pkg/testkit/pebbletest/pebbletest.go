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
	"math/big"
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
	// TLS-ALPN-01; zero picks free ones, which Answer serves. A test that listens there itself
	// listens first and passes the port of its listener, so that nothing takes it meanwhile.
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
	// httpLn and tlsLn listen on the ports that New picked, for Answer.
	httpLn, tlsLn net.Listener
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

// Revoked reports whether the CA revoked the certificate of serial.
func (s Server) Revoked(t testing.TB, serial *big.Int) bool {
	t.Helper()
	var status struct{ Status string }
	if err := json.Unmarshal(s.get(t, s.mgmt+"/cert-status-by-serial/"+serial.Text(16)), &status); err != nil {
		t.Fatalf("pebbletest: the status of %s: %v", serial.Text(16), err)
	}
	return status.Status == "Revoked"
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

// startAttempts is how many times New starts Pebble with new ports when one of its ports was
// taken before it bound it.
const startAttempts = 5

// New starts a Pebble for t and stops it when t ends. It skips t unless TEST_ACME is "pebble".
// The ports that Pebble binds are picked free just before; when a parallel test or another
// process binds one first, New starts Pebble again on others.
func New(t testing.TB, opts Options) Server {
	t.Helper()
	if os.Getenv(EnvACME) != "pebble" {
		t.Skipf("the Pebble tests run with %s=pebble (scripts/task.sh go:test-acme)", EnvACME)
	}
	pebble, challtestsrv := tools(t)
	var httpLn, tlsLn net.Listener
	if opts.HTTPPort == 0 {
		httpLn = listen(t)
		opts.HTTPPort = httpLn.Addr().(*net.TCPAddr).Port
	}
	if opts.TLSPort == 0 {
		tlsLn = listen(t)
		opts.TLSPort = tlsLn.Addr().(*net.TCPAddr).Port
	}
	dir := t.TempDir()
	cert := testkit.NewTLSCertificate(t, 1, time.Now().Add(-time.Hour), 24*time.Hour, "127.0.0.1", "localhost")
	certFile, keyFile := cert.WriteFiles(t, dir, time.Time{})
	for attempt := 1; ; attempt++ {
		s, procs, err := launch(t, opts, pebble, challtestsrv, dir, certFile, keyFile)
		if err == nil {
			s.roots, s.httpLn, s.tlsLn = cert.Pool(), httpLn, tlsLn
			t.Cleanup(func() {
				for _, p := range procs {
					p.stop()
					if t.Failed() {
						t.Logf("%s:\n%s", p.name, p.out.String())
					}
				}
			})
			return s
		}
		for _, p := range procs {
			p.stop()
		}
		if attempt == startAttempts {
			for _, p := range procs {
				t.Logf("%s:\n%s", p.name, p.out.String())
			}
			t.Fatalf("pebbletest: %v", err)
		}
		t.Logf("pebbletest: starting again on other ports: %v", err)
	}
}

// launch starts pebble-challtestsrv and Pebble on free ports, and waits until the CA serves its
// directory. It returns the processes it started, to stop, with its error.
func launch(t testing.TB, opts Options, pebble, challtestsrv, dir, certFile, keyFile string) (Server, []*process, error) {
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
		return Server{}, nil, err
	}
	configFile := filepath.Join(dir, "pebble-config.json")
	if err := os.WriteFile(configFile, b, 0o600); err != nil {
		return Server{}, nil, err
	}
	var procs []*process
	dns := "127.0.0.1:" + strconv.Itoa(dnsPort)
	chall, err := start("pebble-challtestsrv", exec.Command(challtestsrv, //nolint:gosec // G204: the path of a pinned Go tool
		"-defaultIPv4", "127.0.0.1", "-defaultIPv6", "", "-dnsserver", dns, "-management", "127.0.0.1:"+strconv.Itoa(challMgmtPort),
		"-http01", "", "-https01", "", "-tlsalpn01", "", "-doh", ""))
	if err != nil {
		return Server{}, procs, err
	}
	procs = append(procs, chall)
	if err := waitTCP(t, chall, "127.0.0.1:"+strconv.Itoa(challMgmtPort)); err != nil {
		return Server{}, procs, err
	}
	cmd := exec.Command(pebble, "-config", configFile, "-dnsserver", dns) //nolint:gosec // G204: the path of a pinned Go tool
	// No sleep before a validation, no nonce rejected at random, and no authorization reused,
	// so that every order validates its challenges.
	cmd.Env = append(os.Environ(), "PEBBLE_VA_NOSLEEP=1", "PEBBLE_WFE_NONCEREJECT=0", "PEBBLE_AUTHZREUSE=0")
	ca, err := start("pebble", cmd)
	if err != nil {
		return Server{}, procs, err
	}
	procs = append(procs, ca)
	s := Server{
		DirectoryURL: "https://127.0.0.1:" + strconv.Itoa(apiPort) + "/dir",
		HTTPPort:     opts.HTTPPort, TLSPort: opts.TLSPort,
		mgmt: "https://127.0.0.1:" + strconv.Itoa(mgmtPort),
	}
	return s, procs, waitReady(t, ca, s, certFile)
}

// FreePort returns a TCP port of 127.0.0.1 that nothing listens on now, and that it never
// returned before in this test binary: the parallel tests that take ports from it never share
// one, although another process may still bind it first.
func FreePort(t testing.TB) int {
	t.Helper()
	for {
		ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		portsMu.Lock()
		taken := ports[port]
		ports[port] = true
		portsMu.Unlock()
		if !taken {
			return port
		}
	}
}

// listen listens on a free port of 127.0.0.1 until t ends.
func listen(t testing.TB) net.Listener {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

var (
	portsMu sync.Mutex
	ports   = map[int]bool{}
)

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

// process is a running tool of the CA.
type process struct {
	name string
	cmd  *exec.Cmd
	out  *syncBuffer
	// done is closed when the process exits.
	done chan struct{}
}

// start runs cmd, keeping its output.
func start(name string, cmd *exec.Cmd) (*process, error) {
	p := &process{name: name, cmd: cmd, out: &syncBuffer{}, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = p.out, p.out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// stop kills the process and waits for it.
func (p *process) stop() {
	_ = p.cmd.Process.Kill()
	<-p.done
}

// exited returns an error when p has exited: it could not bind a port.
func (p *process) exited() error {
	select {
	case <-p.done:
		return fmt.Errorf("%s exited: %s", p.name, strings.TrimSpace(p.out.String()))
	default:
		return nil
	}
}

// waitTCP waits until addr accepts connections, while p runs.
func waitTCP(t testing.TB, p *process, addr string) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := p.exited(); err != nil {
			return err
		}
		c, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp", addr)
		if err == nil {
			_ = c.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s does not accept connections: %w", addr, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitReady waits until the CA serves its directory, while p runs. Pebble's own certificate is in
// certFile.
func waitReady(t testing.TB, p *process, s Server, certFile string) error {
	pem, err := os.ReadFile(certFile) //nolint:gosec // G304: a file of the test's own directory
	if err != nil {
		return err
	}
	s.roots = x509.NewCertPool()
	s.roots.AppendCertsFromPEM(pem)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := p.exited(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.DirectoryURL, nil)
		resp, err := s.HTTPClient().Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		cancel()
		if err == nil && resp.StatusCode == http.StatusOK {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the CA does not serve %s: %w", s.DirectoryURL, err)
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
// certificate of a handshake that offers acme-tls/1 on 127.0.0.1:TLSPort. Call it once per Server,
// made with neither port given.
func (s Server) Answer(t testing.TB, http01 func(ctx context.Context, token string) (string, error), tlsALPN func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	t.Helper()
	hl, tl := s.httpLn, s.tlsLn
	if hl == nil || tl == nil {
		t.Fatal("pebbletest: Answer needs the ports that New picked")
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
