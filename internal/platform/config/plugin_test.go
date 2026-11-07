// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/domain/instance"
)

// loadPlugin loads the minimal configuration plus the given environment.
func loadPlugin(t *testing.T, env map[string]string) (*Config, []Issue) {
	t.Helper()
	all := minimal()
	for k, v := range env {
		all[k] = v
	}
	cfg, _, err := Load(source(all, nil))
	var ve *ValidationError
	if errors.As(err, &ve) {
		return cfg, ve.Issues
	}
	if err != nil {
		t.Fatal(err)
	}
	return cfg, nil
}

func hasIssue(issues []Issue, key string) bool {
	for _, i := range issues {
		if i.Key == key {
			return true
		}
	}
	return false
}

func TestEveryPluginIsOffByDefault(t *testing.T) {
	cfg, issues := loadPlugin(t, nil)
	if len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
	for _, k := range kind.Delegated() {
		if cfg.PluginDestination(k.Name).Enabled() {
			t.Errorf("%s is on by default", k.Name)
		}
	}
	if w := PluginWarnings(cfg); len(w) != 0 {
		t.Errorf("warnings = %v", w)
	}
}

func TestPlainTCPIsRefusedForTheKindsOfTheRule(t *testing.T) {
	refused := map[string]string{
		"translator": "MY_PLUGIN_TRANSLATOR_ADDRESS", "search": "MY_PLUGIN_SEARCH_ADDRESS", "signup.check": "MY_PLUGIN_SIGNUP_CHECK_ADDRESS",
		"key_manager": "MY_PLUGIN_KEY_MANAGER_ADDRESS", "mailer": "MY_PLUGIN_MAILER_ADDRESS",
		"inbound.status": "MY_PLUGIN_INBOUND_STATUS_ADDRESS", "local.status": "MY_PLUGIN_LOCAL_STATUS_ADDRESS",
	}
	allowed := map[string]string{
		"billing": "MY_PLUGIN_BILLING_ADDRESS", "media_image": "MY_PLUGIN_MEDIA_IMAGE_ADDRESS",
		"media_video": "MY_PLUGIN_MEDIA_VIDEO_ADDRESS", "media_audio": "MY_PLUGIN_MEDIA_AUDIO_ADDRESS",
		"inbound.follow": "MY_PLUGIN_INBOUND_FOLLOW_ADDRESS", "media.check": "MY_PLUGIN_MEDIA_CHECK_ADDRESS",
	}
	if len(refused)+len(allowed) != len(kind.Delegated()) {
		t.Fatalf("this test names %d kinds of %d", len(refused)+len(allowed), len(kind.Delegated()))
	}
	for name, env := range refused {
		k, _ := kind.ByName(name)
		for _, addr := range []string{"http://127.0.0.1:9000", "http://plugin.internal:9000"} {
			if _, issues := loadPlugin(t, map[string]string{env: addr}); !hasIssue(issues, k.Key(kind.SuffixAddress)) {
				t.Errorf("%s at %s: issues = %v", name, addr, issues)
			}
		}
		for _, addr := range []string{"unix:///run/my/p.sock", "https://plugin.internal"} {
			if _, issues := loadPlugin(t, map[string]string{env: addr, "MY_PLUGIN_TRANSLATOR_NAME": "A translator"}); len(issues) != 0 {
				t.Errorf("%s at %s: issues = %v", name, addr, issues)
			}
		}
	}
	for name, env := range allowed {
		addr := "http://127.0.0.1:9000"
		cfg, issues := loadPlugin(t, map[string]string{env: addr})
		if len(issues) != 0 {
			t.Errorf("%s: issues = %v", name, issues)
			continue
		}
		if got := cfg.PluginDestination(name).Address; got != addr {
			t.Errorf("%s: address = %q", name, got)
		}
		w := PluginWarnings(cfg)
		if len(w) != 1 || !strings.Contains(w[0].Message, name) || !strings.Contains(w[0].Message, addr) {
			t.Errorf("%s: warnings = %v", name, w)
		}
	}
}

func TestPluginSettings(t *testing.T) {
	cfg, issues := loadPlugin(t, map[string]string{
		"MY_PLUGIN_MAILER_ADDRESS":       "https://mailer.internal:8443",
		"MY_PLUGIN_MAILER_PROTOCOL":      "grpc",
		"MY_PLUGIN_MAILER_TOKEN":         "s3cret",
		"MY_PLUGIN_MAILER_TLS_CA_FILE":   "/ca",
		"MY_PLUGIN_MAILER_TLS_CERT_FILE": "/cert",
		"MY_PLUGIN_MAILER_TLS_KEY_FILE":  "/key",
		"MY_PLUGIN_MAILER_TIMEOUT":       "7s",
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}
	want := kind.Destination{
		Address: "https://mailer.internal:8443", Protocol: "grpc", Token: "s3cret",
		TLSCAFile: "/ca", TLSCertFile: "/cert", TLSKeyFile: "/key", Timeout: 7 * time.Second,
	}
	if got := cfg.PluginDestination(kind.Mailer); got != want {
		t.Errorf("destination = %+v", got)
	}

	// Nothing set leaves the kind's defaults to the client.
	cfg, _ = loadPlugin(t, map[string]string{"MY_PLUGIN_MEDIA_VIDEO_ADDRESS": "unix:///s"})
	if got := cfg.PluginDestination(kind.MediaVideo); got != (kind.Destination{Address: "unix:///s"}) {
		t.Errorf("media = %+v", got)
	}
}

func TestPluginSettingsThatConflict(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		wantKey string
	}{
		{"a token over plain TCP", map[string]string{"MY_PLUGIN_BILLING_ADDRESS": "http://127.0.0.1:1", "MY_PLUGIN_BILLING_TOKEN": "t"}, "plugin_billing_token"},
		{"a malformed address", map[string]string{"MY_PLUGIN_BILLING_ADDRESS": "billing:8080"}, "plugin_billing_address"},
		{"json for a streaming kind", map[string]string{"MY_PLUGIN_MEDIA_VIDEO_ADDRESS": "unix:///s", "MY_PLUGIN_MEDIA_VIDEO_PROTOCOL": "json"}, "plugin_media_video_protocol"},
		{"an unknown protocol", map[string]string{"MY_PLUGIN_BILLING_ADDRESS": "unix:///s", "MY_PLUGIN_BILLING_PROTOCOL": "soap"}, "plugin_billing_protocol"},
		{"a timeout as long as a job's lease", map[string]string{"MY_PLUGIN_BILLING_ADDRESS": "unix:///s", "MY_PLUGIN_BILLING_TIMEOUT": "5m"}, "plugin_billing_timeout"},
		{"not a timeout", map[string]string{"MY_PLUGIN_BILLING_TIMEOUT": "soon"}, "plugin_billing_timeout"},
		{"a certificate on a socket", map[string]string{"MY_PLUGIN_BILLING_ADDRESS": "unix:///s", "MY_PLUGIN_BILLING_TLS_CERT_FILE": "/c", "MY_PLUGIN_BILLING_TLS_KEY_FILE": "/k"}, "plugin_billing_tls_cert_file"},
		{"a certificate without a key", map[string]string{"MY_PLUGIN_BILLING_ADDRESS": "https://b", "MY_PLUGIN_BILLING_TLS_CERT_FILE": "/c"}, "plugin_billing_tls_key_file"},
		{"a token without an address", map[string]string{"MY_PLUGIN_BILLING_TOKEN": "t"}, "plugin_billing_token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, issues := loadPlugin(t, tc.env); !hasIssue(issues, tc.wantKey) {
				t.Errorf("issues = %v, want one of %s", issues, tc.wantKey)
			}
		})
	}
}

func TestPluginTokenIsSecret(t *testing.T) {
	_, settings, err := Load(source(with(minimal(), "MY_PLUGIN_MAILER_ADDRESS", "unix:///s", "MY_PLUGIN_MAILER_TOKEN", "s3cret"), nil))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range settings {
		if s.Key == "plugin_mailer_token" {
			found = true
			if !s.Secret || s.Value == "s3cret" {
				t.Errorf("token setting = %+v", s)
			}
		}
	}
	if !found {
		t.Error("no token setting")
	}
}

func TestPluginWarningsReachTheReadinessReport(t *testing.T) {
	cfg, issues := loadPlugin(t, map[string]string{"MY_PLUGIN_BILLING_ADDRESS": "http://billing.internal"})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	cfg.Profile = "production"
	for _, f := range Readiness(cfg) {
		if f.Key == "plugin_billing_address" && f.Severity == SeverityWarning {
			return
		}
	}
	t.Error("no warning about plain TCP in the report")
}

// The destination is settable only in the configuration file and the environment (ADR-0146 §5).
func TestThePluginDestinationIsNotAnInstanceSettingOrInTheAPI(t *testing.T) {
	for _, d := range instance.Definitions() {
		if strings.Contains(string(d.Key), "plugin") {
			t.Errorf("instance setting %s", d.Key)
		}
	}
	spec, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(spec), "plugin_") {
		t.Error("the client API names a plugin setting")
	}
}
