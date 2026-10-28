// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/webpush"
	"github.com/mycroblog/my/internal/platform/config"
)

func TestE2EFakesDevPushServiceRefusesProduction(t *testing.T) {
	t.Setenv(config.EnvConfigFile, "")
	for _, profile := range []string{"", "production"} {
		t.Setenv(config.EnvPrefix+"PROFILE", profile)
		if profile == "" {
			if err := os.Unsetenv(config.EnvPrefix + "PROFILE"); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"dev", "push-service", "-listen", "127.0.0.1:0"}, &stdout, &stderr); code != 1 {
			t.Errorf("profile %q: my dev push-service = %d, want 1", profile, code)
		}
		if !strings.Contains(stderr.String(), "needs profile = development") {
			t.Errorf("profile %q: stderr = %q", profile, stderr.String())
		}
	}
}

func TestE2EFakesDevUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-h"}, &stdout, &stderr); code != 0 {
		t.Errorf("my -h = %d", code)
	}
	if !strings.Contains(stderr.String(), "my dev push-service") || !strings.Contains(stderr.String(), "e2e_fakes") {
		t.Errorf("the usage of a test build does not name my dev:\n%s", stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"dev", "nosuch"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), `unknown subcommand "nosuch"`) {
		t.Errorf("my dev nosuch = %d, %q", code, stderr.String())
	}
}

// lockedBuffer is a bytes.Buffer that the command writes while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestE2EFakesDevPushServiceServes(t *testing.T) {
	t.Setenv(config.EnvConfigFile, "")
	t.Setenv(config.EnvPrefix+"PROFILE", "development")
	certFile, keyFile, pool := writeCert(t)

	ctx, cancel := context.WithCancel(t.Context())
	var stdout, stderr lockedBuffer
	code := make(chan int, 1)
	go func() {
		code <- runDevPushService(ctx, []string{"-listen", "127.0.0.1:0", "-cert", certFile, "-key", keyFile}, &stdout, &stderr)
	}()
	listening := regexp.MustCompile(`listening on (\S+) \(origin (\S+)\)`)
	var m []string
	for deadline := time.Now().Add(10 * time.Second); m == nil; {
		if time.Now().After(deadline) {
			t.Fatalf("not listening: stdout %q, stderr %q", stdout.String(), stderr.String())
		}
		select {
		case c := <-code:
			t.Fatalf("my dev push-service = %d: %s", c, stderr.String())
		case <-time.After(10 * time.Millisecond):
		}
		m = listening.FindStringSubmatch(stdout.String())
	}
	_, port, _ := net.SplitHostPort(m[1])
	if m[2] != "https://localhost:"+port {
		t.Errorf("origin = %s, want https://localhost:%s", m[2], port)
	}
	if !strings.Contains(stderr.String(), "this is a test build") {
		t.Errorf("no warning of the test build: %q", stderr.String())
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	_, pub, err := webpush.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(m[2]+"/subscriptions", "application/json", strings.NewReader(`{"applicationServerKey":"`+pub+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || !strings.Contains(string(body), `"endpoint":"`+m[2]+`/push/`) {
		t.Errorf("POST /subscriptions = %d %s", resp.StatusCode, body)
	}

	cancel()
	if c := <-code; c != 0 {
		t.Errorf("my dev push-service = %d after the context ended: %s", c, stderr.String())
	}
}

// writeCert writes a self-signed certificate for localhost and returns its files and a pool that
// trusts it.
func writeCert(t *testing.T) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool = x509.NewCertPool()
	pool.AddCert(cert)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile, pool
}
