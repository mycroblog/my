// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
)

func tlsCmd(args []string, out, errOut *bytes.Buffer) int { return runTLS(args, out, errOut) }

// useACMEClient makes `my tls` order from client.
func useACMEClient(t *testing.T, client appinstance.ACMEClient) {
	t.Helper()
	prev := tlsAppOptions
	tlsAppOptions = platform.AppOptions{ACMEClient: client}
	t.Cleanup(func() { tlsAppOptions = prev })
}

func TestTLSCommandsWithACME(t *testing.T) {
	newAdminEnv(t)
	t.Setenv(config.EnvPrefix+"TLS", "acme")
	t.Setenv(config.EnvPrefix+"TLS_ACME_AGREE_TERMS", "true")
	t.Setenv(config.EnvPrefix+"KV", "db")
	ca := instancetest.NewFakeACMEClient(t)
	// The processes of the server would answer the challenges; here every answer is right.
	ca.Validate = func(context.Context, appinstance.ACMEChallenge) error { return nil }
	useACMEClient(t, ca)

	code, stdout, stderr := runCmd(t, tlsCmd, "status")
	if code != 0 || !strings.Contains(stdout, "social.example\n  certificate  none: the server orders one") || !strings.Contains(stdout, "orders       none") {
		t.Fatalf("status before an order = %d %q %q", code, stdout, stderr)
	}

	code, stdout, stderr = runCmd(t, tlsCmd, "renew", "-host", "Social.Example")
	if code != 0 || !strings.HasPrefix(stdout, "obtained the certificate ") || !strings.Contains(stdout, " of social.example, which expires ") {
		t.Fatalf("renew = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd(t, tlsCmd, "status")
	for _, want := range []string{"issued by CN=Fake ACME root", "days left", "(the certificate authority's window)", "issued"} {
		if code != 0 || !strings.Contains(stdout, want) {
			t.Fatalf("status after the renewal = %d %q %q; want %q", code, stdout, stderr, want)
		}
	}

	// A failed order shows in the status and in the exit code.
	ca.FailOrders(errors.New("the CA is down"))
	code, _, stderr = runCmd(t, tlsCmd, "renew", "-host", "social.example")
	if code != 1 || !strings.Contains(stderr, "the CA is down") {
		t.Fatalf("renew with the CA down = %d %q", code, stderr)
	}
	code, stdout, _ = runCmd(t, tlsCmd, "status")
	if code != 0 || !strings.Contains(stdout, "1 order(s) in a row obtained no certificate") || !strings.Contains(stdout, "failed: ") {
		t.Fatalf("status after a failure = %d %q", code, stdout)
	}

	// A revocation whose replacement fails still revokes, and says so.
	code, stdout, stderr = runCmd(t, tlsCmd, "revoke", "-host", "social.example", "-reason", "superseded")
	if code != 0 || !strings.Contains(stdout, "revoked the certificate ") || !strings.Contains(stderr, "the replacement could not be ordered") {
		t.Fatalf("revoke without a replacement = %d %q %q", code, stdout, stderr)
	}
	code, _, stderr = runCmd(t, tlsCmd, "revoke", "-host", "social.example", "-reason", "operator")
	if code != 1 || !strings.Contains(stderr, "the host has no certificate to revoke") {
		t.Fatalf("revoke without a certificate = %d %q", code, stderr)
	}
	ca.FailOrders(nil)
	if code, stdout, stderr = runCmd(t, tlsCmd, "renew", "-host", "social.example"); code != 0 {
		t.Fatalf("renew = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd(t, tlsCmd, "revoke", "-host", "social.example", "-reason", "key_compromise")
	if code != 0 || !strings.Contains(stdout, "(key_compromise)\nreplaced it with the certificate ") || stderr != "" {
		t.Fatalf("revoke = %d %q %q", code, stdout, stderr)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rotate"}, `unknown subcommand "rotate"`},
		{[]string{"renew"}, "-host is required"},
		{[]string{"revoke", "-host", "social.example"}, "-reason must be"},
		{[]string{"revoke", "-host", "social.example", "-reason", "lost"}, "-reason must be"},
		{[]string{"status", "extra"}, "want 0 argument(s)"},
	} {
		if code, _, stderr := runCmd(t, tlsCmd, tc.args...); code != 2 || !strings.Contains(stderr, tc.want) {
			t.Errorf("tls %q = %d %q; want 2 with %q", tc.args, code, stderr, tc.want)
		}
	}

	// With kv = memory, the challenges' answers would stay in this process.
	t.Setenv(config.EnvPrefix+"KV", "memory")
	if code, _, stderr := runCmd(t, tlsCmd, "renew", "-host", "social.example"); code != 1 || !strings.Contains(stderr, "kv = memory") {
		t.Errorf("renew with kv = memory = %d %q", code, stderr)
	}
}

func TestTLSCommandsWithFiles(t *testing.T) {
	newAdminEnv(t)
	cert := testkit.NewTLSCertificate(t, 0x2a, time.Now().Add(-time.Hour), 30*24*time.Hour, "social.example")
	certFile, keyFile := cert.WriteFiles(t, t.TempDir(), time.Time{})
	t.Setenv(config.EnvPrefix+"TLS", "files")
	t.Setenv(config.EnvPrefix+"TLS_CERT_FILE", certFile)
	t.Setenv(config.EnvPrefix+"TLS_KEY_FILE", keyFile)

	code, stdout, stderr := runCmd(t, tlsCmd, "status")
	if code != 0 || !strings.Contains(stdout, "tls = files: the certificate comes from "+certFile) ||
		!strings.Contains(stdout, "serial 2a") || !strings.Contains(stdout, "names        social.example") {
		t.Fatalf("status = %d %q %q", code, stdout, stderr)
	}
	for _, args := range [][]string{{"renew", "-host", "social.example"}, {"revoke", "-host", "social.example", "-reason", "operator"}} {
		if code, _, stderr := runCmd(t, tlsCmd, args...); code != 1 || !strings.Contains(stderr, "with tls = files") {
			t.Errorf("tls %q = %d %q; want 1 refused", args, code, stderr)
		}
	}

	t.Setenv(config.EnvPrefix+"TLS", "off")
	if code, stdout, _ := runCmd(t, tlsCmd, "status"); code != 0 || !strings.Contains(stdout, "tls is off") {
		t.Errorf("status with tls = off = %d %q", code, stdout)
	}
	if code, _, stderr := runCmd(t, tlsCmd, "renew", "-host", "social.example"); code != 1 || !strings.Contains(stderr, "tls is off") {
		t.Errorf("renew with tls = off = %d %q", code, stderr)
	}
}
