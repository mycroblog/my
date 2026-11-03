// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strings"
	"testing"
)

// TestLoginProvidersOnlyCommands is the operator's side: turning the setting on prints who it
// affects and does not refuse, the commands on providers say what they do to the limit, the reset
// link warns, and turning the setting off is the recovery.
func TestLoginProvidersOnlyCommands(t *testing.T) {
	newAdminEnv(t)
	if code, _, stderr := runAdminCreate(t, adminPassword+"\n", "-username", "root", "-email", "root@example.com"); code != 0 || stderr != "" {
		t.Fatalf("admin create: %d %q", code, stderr)
	}
	code, stdout, stderr := runInstanceCmd(t, "", "set", "login_providers_only", "true")
	if code != 0 || !strings.Contains(stdout, "set login_providers_only = true") || !strings.Contains(stdout, "no login provider is enabled, so it does not apply") {
		t.Errorf("set without a provider = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runLoginProviderCmd(t, "s3cret-client-secret\n", "create", "-kind", "oidc", "-name", "Example SSO", "-issuer", "https://sso.example", "-client-id", "my-client")
	m := addedLoginProvider.FindStringSubmatch(strings.SplitAfterN(stdout, "\n", 3)[0] + strings.SplitAfterN(stdout, "\n", 3)[1])
	if code != 0 || m == nil || !strings.Contains(stdout, "sign-in is limited to 1 login provider(s); 1 active user(s) have no link") {
		t.Fatalf("add under the setting = %d %q %q", code, stdout, stderr)
	}
	id := m[1]
	code, stdout, stderr = runCmd(t, adminCmd, "account", "password-reset-link", "@root")
	if code != 0 || !strings.Contains(stdout, "/auth/reset_password?token=") || !strings.Contains(stderr, "does not work until the limit ends") {
		t.Errorf("password-reset-link under the limit = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runAdminCreate(t, adminPassword+"\n", "-username", "second", "-email", "second@example.com")
	if code != 0 || !strings.Contains(stdout, "created admin") || !strings.Contains(stderr, "this password cannot sign in until the limit ends") {
		t.Errorf("admin create under the limit = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runLoginProviderCmd(t, "", "disable", id)
	if code != 0 || !strings.Contains(stdout, "no login provider is enabled, so it does not apply") {
		t.Errorf("disable the last provider = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runLoginProviderCmd(t, "", "enable", id)
	if code != 0 || !strings.Contains(stdout, "2 active user(s) have no link") {
		t.Errorf("enable it again = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runInstanceCmd(t, "", "set", "login_providers_only", "false")
	if code != 0 || !strings.Contains(stdout, "login_providers_only is off: passwords and passkeys sign in") {
		t.Errorf("set false = %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd(t, adminCmd, "account", "password-reset-link", "@root")
	if code != 0 || strings.Contains(stderr, "does not work") {
		t.Errorf("password-reset-link without the limit = %d %q %q", code, stdout, stderr)
	}
	// With the setting off, the commands on providers say nothing about it.
	code, stdout, stderr = runLoginProviderCmd(t, "", "disable", id)
	if code != 0 || strings.Contains(stdout, "login_providers_only") || strings.Contains(stdout, "limited") {
		t.Errorf("disable with the setting off = %d %q %q", code, stdout, stderr)
	}
}
