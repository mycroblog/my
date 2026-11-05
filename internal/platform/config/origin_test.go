// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"strings"
	"testing"
)

const (
	testOriginSecret   = "0123456789abcdef0123456789abcdef"
	testOriginPrevious = "fedcba9876543210fedcba9876543210"
)

func TestOriginSecretHeaderNames(t *testing.T) {
	for _, name := range []string{"X-Origin-Secret", "x-via-cdn", "MyCdn1"} {
		if err := checkOriginSecretHeader(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{
		"cf-origin", "CF-Connecting-IP", "x-cf-secret", "X-Forwarded-For", "x-forwarded-proto", "x-real-ip", "true-client-ip",
		"cookie", "Accept-Encoding", "Host", "Authorization", "Traceparent", "x-amz-origin", "x-amzn-request-context", "x-edge-origin", "x-request-id", "sec-origin", "if-match",
		"has space", "under_score", "colon:", "-leading", "trailing-", "日本語",
	} {
		if err := checkOriginSecretHeader(name); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestOriginSecretValues(t *testing.T) {
	if err := checkOriginSecret(testOriginSecret); err != nil {
		t.Error(err)
	}
	for _, v := range []string{"short", strings.Repeat("a", 31), strings.Repeat("a", 31) + " ", strings.Repeat("あ", 16), strings.Repeat("a", 31) + "\n"} {
		err := checkOriginSecret(v)
		if err == nil {
			t.Errorf("%q was accepted", v)
		} else if strings.Contains(err.Error(), v) {
			t.Errorf("the error holds the value: %v", err)
		}
	}
}

// The settings hold together in the maintenance mode too, which runs no crossChecks, and neither
// the settings nor the errors show the values.
func TestOriginSecretCombinations(t *testing.T) {
	for _, tt := range []struct {
		name    string
		env     map[string]string
		wantKey string
	}{
		{"no header", map[string]string{"MY_ORIGIN_SECRET": testOriginSecret}, "origin_secret_header"},
		{"no secret", map[string]string{"MY_ORIGIN_SECRET_HEADER": "X-Via-Cdn"}, "origin_secret_header"}, //nolint:gosec // G101: made-up values of the test
		{"only previous", map[string]string{"MY_ORIGIN_SECRET_PREVIOUS": testOriginPrevious}, "origin_secret_previous"},
		{"same twice", map[string]string{"MY_ORIGIN_SECRET": testOriginSecret, "MY_ORIGIN_SECRET_PREVIOUS": testOriginSecret, "MY_ORIGIN_SECRET_HEADER": "X-Via-Cdn"}, "origin_secret_previous"}, //nolint:gosec // G101: made-up values of the test
		{"a header that the CDN refuses", map[string]string{"MY_ORIGIN_SECRET": testOriginSecret, "MY_ORIGIN_SECRET_HEADER": "cf-secret"}, "origin_secret_header"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for name, load := range map[string]func(Source) (*Config, []Setting, error){"Load": Load, "LoadMaintenance": LoadMaintenance} {
				env := minimal()
				for k, v := range tt.env {
					env[k] = v
				}
				_, _, err := load(source(env, nil))
				var verr *ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("%s: err = %v", name, err)
				}
				found := false
				for _, i := range verr.Issues {
					found = found || i.Key == tt.wantKey
				}
				if !found {
					t.Errorf("%s: issues = %+v, want one of %s", name, verr.Issues, tt.wantKey)
				}
				if strings.Contains(err.Error(), testOriginSecret) {
					t.Errorf("%s: the error holds the secret: %v", name, err)
				}
			}
		})
	}
}

func TestOriginSecretLoads(t *testing.T) {
	env := minimal()
	env["MY_ORIGIN_SECRET"], env["MY_ORIGIN_SECRET_PREVIOUS"], env["MY_ORIGIN_SECRET_HEADER"] = testOriginSecret, testOriginPrevious, "X-Via-Cdn"
	for name, load := range map[string]func(Source) (*Config, []Setting, error){"Load": Load, "LoadMaintenance": LoadMaintenance} {
		cfg, settings, err := load(source(env, nil))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cfg.OriginSecret != testOriginSecret || cfg.OriginSecretPrevious != testOriginPrevious || cfg.OriginSecretHeader != "X-Via-Cdn" {
			t.Errorf("%s: cfg = %q %q %q", name, cfg.OriginSecret, cfg.OriginSecretPrevious, cfg.OriginSecretHeader)
		}
		for _, s := range settings {
			if s.Value == testOriginSecret || s.Value == testOriginPrevious {
				t.Errorf("%s: %s is shown as %q", name, s.Key, s.Value)
			}
		}
	}
	// The file holds no secret, and a file of the secret is read.
	_, _, err := Load(source(map[string]string{"MY_CONFIG": "f"}, map[string]string{"f": `origin_secret = "` + testOriginSecret + `"`}))
	if err == nil || !strings.Contains(err.Error(), "is a secret") {
		t.Errorf("a secret in the file: %v", err)
	}
	fileEnv := minimal()
	fileEnv["MY_ORIGIN_SECRET_FILE"], fileEnv["MY_ORIGIN_SECRET_HEADER"] = "secret", "X-Via-Cdn"
	cfg, _, err := Load(source(fileEnv, map[string]string{"secret": testOriginSecret + "\n"}))
	if err != nil || cfg.OriginSecret != testOriginSecret {
		t.Errorf("MY_ORIGIN_SECRET_FILE: %v, %q", err, cfg.OriginSecret)
	}
}

func TestOriginSecretReadiness(t *testing.T) {
	find := func(env map[string]string) map[string]Severity {
		cfg, _, err := Load(source(env, nil))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]Severity{}
		for _, f := range Readiness(cfg) {
			got[f.Key] = f.Severity
		}
		return got
	}
	got := find(minimal())
	if _, ok := got["origin_secret_previous"]; ok {
		t.Errorf("unset: %v", got)
	}
	if _, ok := got["trusted_proxies"]; ok {
		t.Errorf("unset: %v", got)
	}
	env := minimal()
	env["MY_ORIGIN_SECRET"], env["MY_ORIGIN_SECRET_PREVIOUS"], env["MY_ORIGIN_SECRET_HEADER"] = testOriginSecret, testOriginPrevious, "X-Via-Cdn"
	got = find(env)
	if got["origin_secret_previous"] != SeverityWarning || got["trusted_proxies"] != SeverityWarning {
		t.Errorf("set: %v", got)
	}
}
