// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
)

func runPluginTest(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(append([]string{"plugin", "test"}, args...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestPluginTestPassesAPlugin(t *testing.T) {
	h, _ := fakeplugin.JSON(fakeplugin.Options{})
	code, stdout, stderr := runPluginTest("--kind", "probe", fakeplugin.ServeUnix(t, h))
	if code != 0 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{"PASS reachable\n", "PASS echo\n", "PASS echo_empty\n", "3 passed, 0 failed, 0 skipped\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestPluginTestReportsEachFailure(t *testing.T) {
	h, _ := fakeplugin.JSON(fakeplugin.Options{Broken: true})
	code, stdout, stderr := runPluginTest("--kind", "probe", fakeplugin.ServeTCP(t, h))
	if code != 1 {
		t.Fatalf("code = %d\n%s%s", code, stdout, stderr)
	}
	for _, want := range []string{"PASS reachable\n", "FAIL echo: ", "FAIL echo_empty: ", "1 passed, 2 failed, 0 skipped\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "warning: address is plain TCP") {
		t.Errorf("no warning about plain TCP: %s", stderr)
	}
}

func TestPluginTestAuthentication(t *testing.T) {
	h, _ := fakeplugin.Connect(fakeplugin.Options{Token: "s3cret"})
	addr := fakeplugin.ServeUnix(t, h)
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runPluginTest("--kind", "probe", "--token-file", file, "--protocol", "proto", addr); code != 0 {
		t.Errorf("code = %d\n%s%s", code, stdout, stderr)
	}
	if code, stdout, _ := runPluginTest("--kind", "probe", addr); code != 1 || !strings.Contains(stdout, "FAIL reachable") {
		t.Errorf("no token: code = %d\n%s", code, stdout)
	}
}

func TestPluginTestMutualTLS(t *testing.T) {
	h, _ := fakeplugin.Connect(fakeplugin.Options{})
	files := fakeplugin.NewTLSFiles(t, true)
	addr := fakeplugin.ServeTLS(t, h, files.Server)
	if code, stdout, stderr := runPluginTest("--kind", "probe", "--tls-ca-file", files.CAFile, "--tls-cert-file", files.CertFile, "--tls-key-file", files.KeyFile, addr); code != 0 {
		t.Errorf("code = %d\n%s%s", code, stdout, stderr)
	}
}

func TestPluginTestRefusesABadDestination(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte("t"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"plain TCP for a kind that refuses it", []string{"--kind", "key_manager", "http://127.0.0.1:1"}, 1, "address must not be http://"},
		{"a token over plain TCP", []string{"--kind", "probe", "--token-file", file, "http://127.0.0.1:1"}, 1, "token must not"},
		{"an unknown kind", []string{"--kind", "nope", "unix:///s"}, 2, "--kind must be one of"},
		{"no address", []string{"--kind", "probe"}, 2, "want the address"},
		{"no such flag", []string{"--nope"}, 2, "not defined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runPluginTest(tc.args...)
			if code != tc.code || !strings.Contains(stderr, tc.want) {
				t.Errorf("code = %d, stderr = %s", code, stderr)
			}
		})
	}
}

func TestPluginUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"plugin", "-help"}, &out, &errOut); code != 0 || !strings.Contains(errOut.String(), "my plugin test --kind") {
		t.Errorf("code = %d, stderr = %s", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"plugin", "nope"}, &out, &errOut); code != 2 {
		t.Errorf("code = %d", code)
	}
}
