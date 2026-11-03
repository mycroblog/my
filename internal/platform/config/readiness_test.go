// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"encoding/base64"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

// randomKEK is a key-encryption key with as many distinct bytes as a random one.
var randomKEK = func() string {
	b := make([]byte, kekSize)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return base64.StdEncoding.EncodeToString(b)
}()

// ready is a configuration with no finding but the absent VAPID key and mail, which the
// production checks warn about.
func ready() map[string]string {
	return with(minimal(), "MY_KEK", randomKEK, "MY_DATABASE_URL", "file:/var/lib/my/my.db")
}

type readinessCase struct {
	name string
	env  map[string]string
	// want lists "<severity> <key>" of each finding, in order.
	want []string
	// wantMessage must appear in one of the messages.
	wantMessage string
}

func TestReadiness(t *testing.T) {
	base := []string{"warning mail", "warning vapid_private_key"}
	tests := []readinessCase{
		{name: "ready", env: ready(), want: base},
		{
			name: "development profile",
			env:  with(minimal(), "MY_PROFILE", "development", "MY_HOST", "localhost:8080", "MY_DATABASE_URL", ":memory:"),
			want: nil,
		},
		{
			name:        "host with a port",
			env:         with(ready(), "MY_HOST", "social.example.org:8443"),
			want:        append([]string{"error host"}, base...),
			wantMessage: "has a port",
		},
		{
			name:        "localhost",
			env:         with(ready(), "MY_HOST", "localhost"),
			want:        append([]string{"error host"}, base...),
			wantMessage: "not a public domain name",
		},
		{
			name:        "reserved top-level domain of the account domain",
			env:         with(ready(), "MY_ACCOUNT_DOMAIN", "my.test"),
			want:        append([]string{"error account_domain"}, base...),
			wantMessage: "under .test",
		},
		{
			name:        "http web origin",
			env:         with(ready(), "MY_WEB_ORIGIN", "http://app.example.org"),
			want:        append([]string{"error web_origin"}, base...),
			wantMessage: "not https",
		},
		{
			name:        "in-memory SQLite",
			env:         with(ready(), "MY_DATABASE_URL", "file:my?mode=memory&cache=shared"),
			want:        append([]string{"error database_url"}, base...),
			wantMessage: "in memory",
		},
		{
			name:        "SQLite in /tmp",
			env:         with(ready(), "MY_DATABASE_URL", "file:///tmp/my.db?_pragma=busy_timeout(5000)"),
			want:        append([]string{"warning database_url"}, base...),
			wantMessage: "temporary directory, which",
		},
		{
			name: "SQLite path is not checked for PostgreSQL",
			env:  with(ready(), "MY_DATABASE", "postgres", "MY_DATABASE_URL", "postgres:///tmp"),
			want: base,
		},
		{
			name:        "zero KEK",
			env:         with(ready(), "MY_KEK", testKEK),
			want:        append([]string{"error kek"}, base...),
			wantMessage: "1 distinct bytes in 32",
		},
		{
			name: "CAPTCHA test key",
			env: with(ready(), "MY_CAPTCHA", "turnstile", "MY_CAPTCHA_SITE_KEY", "1x00000000000000000000AA",
				"MY_CAPTCHA_SECRET_KEY", "1x0000000000000000000000000000000AA"),
			want:        append([]string{"error captcha_secret_key"}, base...),
			wantMessage: "test key of turnstile",
		},
		{
			name:        "inline queue and blobs in /tmp",
			env:         with(ready(), "MY_QUEUE", "inline", "MY_BLOB_FS_DIR", "/tmp/blobs"),
			want:        []string{"warning queue", "warning blob_fs_dir", "warning mail", "warning vapid_private_key"},
			wantMessage: "inline",
		},
		{
			name: "mail and Web Push configured",
			env: with(ready(), "MY_MAIL", "smtp", "MY_MAIL_FROM", "noreply@social.example.org", "MY_MAIL_SMTP_HOST", "smtp.example.org",
				"MY_VAPID_PRIVATE_KEY", "kJpUOWsBf0s0ReN3iZK9WLmCnxwcZr2dsHWWUJGGqVM"),
			want: nil,
		},
	}
	for _, key := range []string{"sk_test_abc", "rk_test_abc"} {
		tests = append(tests, readinessCase{
			name: "Stripe test key " + key,
			env: with(ready(), "MY_PAYMENTS", "stripe", "MY_PAYMENTS_STRIPE_SECRET_KEY", key,
				"MY_PAYMENTS_STRIPE_WEBHOOK_SECRET", "whsec_abc"),
			want:        append(slices.Clone(base), "warning payments_stripe_secret_key"),
			wantMessage: "test-mode key",
		}, readinessCase{
			name: "Stripe test key " + key + " in the development profile",
			env: with(minimal(), "MY_PROFILE", "development", "MY_HOST", "localhost:8080", "MY_DATABASE_URL", ":memory:",
				"MY_PAYMENTS", "stripe", "MY_PAYMENTS_STRIPE_SECRET_KEY", key, "MY_PAYMENTS_STRIPE_WEBHOOK_SECRET", "whsec_abc"),
			want: nil,
		})
	}
	for _, key := range []string{"sk_live_abc", "rk_live_abc"} {
		tests = append(tests, readinessCase{
			name: "Stripe live key " + key,
			env: with(ready(), "MY_PAYMENTS", "stripe", "MY_PAYMENTS_STRIPE_SECRET_KEY", key,
				"MY_PAYMENTS_STRIPE_WEBHOOK_SECRET", "whsec_abc"),
			want: base,
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _, err := Load(source(tt.env, nil))
			if err != nil {
				t.Fatal(err)
			}
			findings := Readiness(cfg)
			var got []string
			found := tt.wantMessage == ""
			for _, f := range findings {
				got = append(got, string(f.Severity)+" "+f.Key)
				found = found || strings.Contains(f.Message, tt.wantMessage)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("findings = %q, want %q", got, tt.want)
			}
			if !found {
				t.Errorf("no message contains %q: %v", tt.wantMessage, findings)
			}
		})
	}
}

func TestFindingEnv(t *testing.T) {
	if got := (Finding{Key: "kek"}).Env(); got != "MY_KEK" {
		t.Errorf("Env() = %q, want MY_KEK", got)
	}
}

// TestReadinessWarnsAboutSettingsOfOneProcess covers the settings that work in one process and not
// across several, and the voice settings that were likely not meant.
func TestReadinessWarnsAboutSettingsOfOneProcess(t *testing.T) {
	has := func(findings []Finding, key string) bool {
		return slices.ContainsFunc(findings, func(f Finding) bool { return f.Key == key && f.Severity == SeverityWarning })
	}
	load := func(env map[string]string) []Finding {
		t.Helper()
		cfg, _, err := Load(source(env, nil))
		if err != nil {
			t.Fatal(err)
		}
		return Readiness(cfg)
	}
	if f := load(ready()); has(f, "stream") || has(f, "kv") || has(f, "voice") {
		t.Errorf("Readiness of one process = %v, want no warning about stream, kv, or voice", f)
	}
	worker := load(with(ready(), "MY_ROLE", "web", "MY_KV", "memory"))
	if !has(worker, "stream") || !has(worker, "kv") {
		t.Errorf("Readiness with role web = %v, want warnings about stream and kv", worker)
	}
	if f := load(with(ready(), "MY_VOICE", "p2p")); !has(f, "voice") {
		t.Errorf("Readiness with p2p alone = %v, want a warning about voice", f)
	}
}

// TestLoadedValuesOfConvenience covers the TOML forms and the defaults that follow other settings.
func TestLoadedValuesOfConvenience(t *testing.T) {
	src := source(minimal(), map[string]string{"my.toml": "mail_smtp_port = 2525\nmaintenance = false\n"})
	src.File = "my.toml"
	cfg, _, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTPPort != 2525 || cfg.Maintenance {
		t.Errorf("mail_smtp_port = %d, maintenance = %v; want 2525 and false", cfg.SMTPPort, cfg.Maintenance)
	}
	dev, _, err := Load(source(with(minimal(), "MY_PROFILE", "development"), nil))
	if err != nil {
		t.Fatal(err)
	}
	if dev.ErrorsEnvironment != "development" {
		t.Errorf("errors_environment = %q, want the profile", dev.ErrorsEnvironment)
	}
	prod, _, err := Load(source(minimal(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if prod.ErrorsEnvironment != "production" {
		t.Errorf("errors_environment = %q, want production", prod.ErrorsEnvironment)
	}
	var logged slog.Value
	for _, a := range prod.LogValue().Group() {
		if a.Key == "worker_job_concurrency" {
			logged = a.Value
		}
	}
	if logged.Int64() != 1 {
		t.Errorf("logged worker_job_concurrency = %v, want the effective 1", logged)
	}
}
