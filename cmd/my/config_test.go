// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/webpush"
)

func TestRunConfigVAPIDKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"config", "vapid-key"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	var priv, pub string
	for line := range strings.Lines(stdout.String()) {
		k, v, _ := strings.Cut(line, "=")
		switch strings.TrimSpace(k) {
		case "vapid_private_key":
			priv = strings.TrimSpace(v)
		case "vapid_public_key":
			pub = strings.TrimSpace(v)
		}
	}
	key, err := webpush.ParsePrivateKey(priv)
	if err != nil {
		t.Fatalf("private key %q: %v", priv, err)
	}
	b, _ := key.PublicKey.Bytes()
	if got := base64.RawURLEncoding.EncodeToString(b); got != pub {
		t.Errorf("public key %q, want %q", pub, got)
	}
	if code := run([]string{"config", "vapid-key", "extra"}, &stdout, &stderr); code != 2 {
		t.Errorf("an extra argument exits %d, want 2", code)
	}
}

const captchaSecret = "0x5eCrE7-captcha-secret" //nolint:gosec // G101: a test secret

func TestRunConfig(t *testing.T) {
	kek := base64.StdEncoding.EncodeToString(make([]byte, 32))
	file := filepath.Join(t.TempDir(), "my.toml")
	if err := os.WriteFile(file, []byte(`role = "web"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		args       []string
		env        map[string]string
		wantCode   int
		wantStdout []string
		wantStderr []string
		// wantAbsent must appear in neither output.
		wantAbsent []string
	}{
		{
			name:       "valid",
			args:       []string{"config", "check", "-config", file},
			env:        map[string]string{"MY_HOST": "social.example.org", "MY_DATABASE_URL": "file:my.db", "MY_KEK": kek},
			wantCode:   0,
			wantStdout: []string{"web", "file", "(redacted)", "configuration OK"},
		},
		{
			name:       "an unknown MY_ variable is a warning",
			args:       []string{"config", "check", "-config", file},
			env:        map[string]string{"MY_HOST": "social.example.org", "MY_DATABASE_URL": "file:my.db", "MY_KEK": kek, "MY_HSOT": "typo.example", "MY_SERVICE_HOST": "10.0.0.1"},
			wantCode:   0,
			wantStdout: []string{"configuration OK"},
			wantStderr: []string{"warning: the environment variable MY_HSOT starts with MY_ but is not a setting, so it is ignored"},
			wantAbsent: []string{"MY_SERVICE_HOST"},
		},
		{
			name:     "invalid lists every issue",
			args:     []string{"config", "check"},
			env:      map[string]string{"MY_DB": "mysql", "LC_ALL": "C"},
			wantCode: 1,
			wantStderr: []string{
				"The required setting MY_HOST is not set.",
				"The setting MY_DB has an invalid value.",
				`db: unknown value "mysql"`,
				"The required setting MY_DATABASE_URL is not set.",
			},
		},
		{
			name: "captcha without a site key",
			args: []string{"config", "check"},
			env: map[string]string{
				"MY_HOST": "social.example.org", "MY_DATABASE_URL": "file:my.db", "MY_KEK": kek, "LC_ALL": "C",
				"MY_CAPTCHA": "turnstile", "MY_CAPTCHA_SECRET_KEY": captchaSecret,
			},
			wantCode:   1,
			wantStderr: []string{"The required setting MY_CAPTCHA_SITE_KEY is not set.", "captcha_site_key: required when captcha is not none"},
			wantAbsent: []string{captchaSecret},
		},
		{
			name:       "maintenance leaves the adapters unread",
			args:       []string{"config", "check", "-maintenance"},
			env:        map[string]string{"MY_HOST": "social.example.org", "MY_DB": "mysql", "MY_MAINTENANCE_UNTIL": "2026-10-24T03:00:00Z"},
			wantCode:   0,
			wantStdout: []string{"maintenance_until", "configuration OK"},
			wantAbsent: []string{"database_url", "mysql"},
		},
		{
			name:       "maintenance checks its settings",
			args:       []string{"config", "check", "-maintenance"},
			env:        map[string]string{"MY_HOST": "social.example.org", "MY_MAINTENANCE_UNTIL": "tomorrow", "LC_ALL": "C"},
			wantCode:   1,
			wantStderr: []string{"maintenance_until: must be an RFC 3339 time"},
		},
		{
			name:       "the normal check rejects maintenance settings too",
			args:       []string{"config", "check"},
			env:        map[string]string{"MY_HOST": "social.example.org", "MY_DATABASE_URL": "file:my.db", "MY_KEK": kek, "MY_MAINTENANCE_RETRY_AFTER": "0", "LC_ALL": "C"},
			wantCode:   1,
			wantStderr: []string{"maintenance_retry_after: must be a whole number of seconds"},
		},
		{name: "no subcommand", args: []string{"config"}, wantCode: 2, wantStderr: []string{"Usage: my config check"}},
		{name: "unknown subcommand", args: []string{"config", "nope"}, wantCode: 2, wantStderr: []string{`unknown subcommand "nope"`}},
		{name: "extra argument", args: []string{"config", "check", "x"}, wantCode: 2, wantStderr: []string{`unexpected argument "x"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LANGUAGE", "")
			for _, k := range []string{"MY_CONFIG", "MY_HOST", "MY_DATABASE_URL", "MY_KEK", "MY_DB", "LC_ALL", "MY_CAPTCHA", "MY_CAPTCHA_SITE_KEY", "MY_CAPTCHA_SECRET_KEY",
				"MY_MAINTENANCE_UNTIL", "MY_MAINTENANCE_RETRY_AFTER", "MY_HSOT", "MY_SERVICE_HOST"} {
				t.Setenv(k, tt.env[k]) // restores the original value after the test
				if _, ok := tt.env[k]; !ok {
					if err := os.Unsetenv(k); err != nil {
						t.Fatal(err)
					}
				}
			}
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d; stderr = %s", code, tt.wantCode, stderr.String())
			}
			for _, w := range tt.wantStdout {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("stdout = %q, want it to contain %q", stdout.String(), w)
				}
			}
			for _, w := range tt.wantStderr {
				if !strings.Contains(stderr.String(), w) {
					t.Errorf("stderr = %q, want it to contain %q", stderr.String(), w)
				}
			}
			for _, w := range tt.wantAbsent {
				if strings.Contains(stdout.String()+stderr.String(), w) {
					t.Errorf("the output contains %q: %s%s", w, stdout.String(), stderr.String())
				}
			}
		})
	}
}
