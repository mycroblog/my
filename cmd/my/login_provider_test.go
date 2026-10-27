// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func runLoginProviderCmd(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = runLoginProvider(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

var addedLoginProvider = regexp.MustCompile(`^added oidc login provider (\S+) \(Example SSO\)\ncallback URL: https://social\.example/auth/external/(\S+)/callback\n$`)

func TestLoginProviderCommands(t *testing.T) {
	newAdminEnv(t)
	const secret = "s3cret-client-secret"
	code, stdout, stderr := runLoginProviderCmd(t, secret+"\n", "add", "-kind", "oidc", "-name", "Example SSO",
		"-issuer", "https://sso.example", "-client-id", "my-client", "-scope", "groups", "-claim", "hd=example.com",
		"-claim", "hd=example.org", "-allow-signup")
	m := addedLoginProvider.FindStringSubmatch(stdout)
	if code != 0 || m == nil || m[1] != m[2] {
		t.Fatalf("login-provider add = %d %q %q", code, stdout, stderr)
	}
	id := m[1]
	if code, stdout, _ = runLoginProviderCmd(t, "", "add", "-kind", "github", "-name", "GitHub", "-client-id", "gh"); code != 1 ||
		stdout != "" {
		t.Errorf("login-provider add without a secret = %d %q", code, stdout)
	}

	list := func() string {
		t.Helper()
		code, stdout, stderr := runLoginProviderCmd(t, "", "list")
		if code != 0 {
			t.Fatalf("login-provider list = %d %q", code, stderr)
		}
		return stdout
	}
	if got := list(); !regexp.MustCompile(id+` +oidc +Example SSO +enabled +https://sso\.example +my-client +true +true +0 +\S+ +https://social\.example/auth/external/`+id+`/callback`).MatchString(got) ||
		strings.Contains(got, secret) {
		t.Errorf("login-provider list:\n%s", got)
	}

	if code, stdout, stderr = runLoginProviderCmd(t, "", "set", "-name", "Corp SSO", "-allow-signup=false", "-claim", "", id); code != 0 ||
		stdout != "updated login provider "+id+" (Corp SSO)\n" {
		t.Errorf("login-provider set = %d %q %q", code, stdout, stderr)
	}
	if got := list(); !regexp.MustCompile(id + ` +oidc +Corp SSO +enabled +\S+ +my-client +false +true`).MatchString(got) {
		t.Errorf("login-provider list after set:\n%s", got)
	}
	if code, stdout, stderr = runLoginProviderCmd(t, "another-secret\n", "rotate-secret", id); code != 0 ||
		stdout != "replaced the client secret of login provider "+id+"\n" || strings.Contains(stdout+stderr, "another-secret") {
		t.Errorf("login-provider rotate-secret = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, _ = runLoginProviderCmd(t, "", "disable", id); code != 0 ||
		stdout != "disabled login provider "+id+"; 0 linked user(s) can no longer sign in with it\n" {
		t.Errorf("login-provider disable = %d %q", code, stdout)
	}
	if got := list(); !regexp.MustCompile(id + ` +oidc +Corp SSO +disabled`).MatchString(got) {
		t.Errorf("login-provider list after disable:\n%s", got)
	}
	if code, stdout, _ = runLoginProviderCmd(t, "", "enable", id); code != 0 || stdout != "enabled login provider "+id+"\n" {
		t.Errorf("login-provider enable = %d %q", code, stdout)
	}
	if code, stdout, _ = runLoginProviderCmd(t, "", "delete", id); code != 0 ||
		stdout != "deleted login provider "+id+"; 0 linked user(s) can no longer sign in with it\n" {
		t.Errorf("login-provider delete = %d %q", code, stdout)
	}
	if got := list(); strings.Contains(got, id) {
		t.Errorf("login-provider list after delete:\n%s", got)
	}

	for _, tc := range []struct {
		stdin string
		args  []string
		code  int
		want  string
	}{
		{secret, []string{"add", "-kind", "oidc", "-name", "X", "-issuer", "http://sso.example", "-client-id", "c"}, 1, "invalid login provider URL"},
		{secret, []string{"add", "-kind", "github", "-name", "X", "-client-id", "c", "-claim", "hd=example.com"}, 1, "invalid login provider conditions"},
		{secret, []string{"add", "-kind", "oidc", "-name", "X", "-issuer", "https://sso.example", "-client-id", "c", "-claim", "hd"}, 2, "is not <claim>=<value>"},
		{secret, []string{"add", "-kind", "oidc", "-name", "X"}, 2, "-client-id is required"},
		{"", []string{"set", id}, 2, "name at least one setting"},
		{"", []string{"set", "-name", "Y", id}, 1, "no login provider has the ID"},
		{secret, []string{"rotate-secret", id}, 1, "no login provider has the ID"},
		{"", []string{"enable", id}, 1, "no login provider has the ID"},
		{"", []string{"disable"}, 2, "want 1 argument(s)"},
		{"", []string{"delete", id}, 1, "no login provider has the ID"},
		{"", []string{"burn"}, 2, `unknown subcommand "burn"`},
	} {
		if code, _, stderr := runLoginProviderCmd(t, tc.stdin, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("login-provider %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
