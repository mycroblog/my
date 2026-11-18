// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform/config"
)

func TestE2EFakesDevOIDCProviderRefusesProduction(t *testing.T) {
	t.Setenv(config.EnvConfigFile, "")
	for _, profile := range []string{"", "production"} {
		t.Setenv(config.EnvPrefix+"PROFILE", profile)
		if profile == "" {
			if err := os.Unsetenv(config.EnvPrefix + "PROFILE"); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		args := []string{"dev", "oidc-provider", "-listen", "127.0.0.1:0", "-client-id", "example-client", "-client-secret", "example-secret"}
		if code := run(args, &stdout, &stderr); code != 1 {
			t.Errorf("profile %q: my dev oidc-provider = %d, want 1", profile, code)
		}
		if !strings.Contains(stderr.String(), "needs profile = development") {
			t.Errorf("profile %q: stderr = %q", profile, stderr.String())
		}
	}
}

func TestE2EFakesDevOIDCProviderServes(t *testing.T) {
	t.Setenv(config.EnvConfigFile, "")
	t.Setenv(config.EnvPrefix+"PROFILE", "development")
	certFile, keyFile, pool := writeCert(t)

	ctx, cancel := context.WithCancel(t.Context())
	var stdout, stderr lockedBuffer
	code := make(chan int, 1)
	go func() {
		code <- runDevOIDCProvider(ctx, []string{
			"-listen", "127.0.0.1:0", "-cert", certFile, "-key", keyFile, "-client-id", "example-client", "-client-secret", "example-secret",
		}, &stdout, &stderr)
	}()
	listening := regexp.MustCompile(`my dev oidc-provider: listening on (\S+) \(issuer (\S+)\)`)
	var m []string
	for deadline := time.Now().Add(10 * time.Second); m == nil; {
		if time.Now().After(deadline) {
			t.Fatalf("not listening: stdout %q, stderr %q", stdout.String(), stderr.String())
		}
		select {
		case c := <-code:
			t.Fatalf("my dev oidc-provider = %d: %s", c, stderr.String())
		case <-time.After(10 * time.Millisecond):
		}
		m = listening.FindStringSubmatch(stdout.String())
	}
	_, port, _ := net.SplitHostPort(m[1])
	if m[2] != "https://localhost:"+port {
		t.Errorf("issuer = %s, want https://localhost:%s", m[2], port)
	}
	if !strings.Contains(stderr.String(), "this is a test build") {
		t.Errorf("no warning of the test build: %q", stderr.String())
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	resp, err := client.Get(m[2] + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var doc struct {
		Issuer        string `json:"issuer"`
		Authorization string `json:"authorization_endpoint"`
		Token         string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET discovery = %d %s: %v", resp.StatusCode, body, err)
	}
	if doc.Issuer != m[2] || !strings.HasPrefix(doc.Authorization, m[2]+"/") || !strings.HasPrefix(doc.Token, m[2]+"/") {
		t.Errorf("discovery = %+v, want the endpoints of %s", doc, m[2])
	}

	cancel()
	if c := <-code; c != 0 {
		t.Errorf("my dev oidc-provider = %d after the context ended: %s", c, stderr.String())
	}
}
