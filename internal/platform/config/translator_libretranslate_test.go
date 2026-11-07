// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"testing"
)

func TestTranslatorLibreTranslate(t *testing.T) {
	const url, key = "MY_TRANSLATOR_LIBRETRANSLATE_URL", "MY_TRANSLATOR_LIBRETRANSLATE_API_KEY"
	// A server that the operator runs is often on a private address over http.
	cfg, issues := loadLibreTranslate(t, map[string]string{url: " http://libretranslate:5000 ", key: "lt-key"})
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if cfg.TranslatorLibreTranslateURL != "http://libretranslate:5000" || cfg.TranslatorLibreTranslateAPIKey != "lt-key" {
		t.Errorf("config = %q, %q", cfg.TranslatorLibreTranslateURL, cfg.TranslatorLibreTranslateAPIKey)
	}
	if _, issues := loadLibreTranslate(t, map[string]string{url: "https://libretranslate.com"}); len(issues) != 0 {
		t.Errorf("without a key: issues = %+v", issues)
	}
	for _, wrong := range []string{"libretranslate:5000", "ftp://example.org", "https://user:pass@example.org", "https://example.org/?k=v"} {
		if _, issues := loadLibreTranslate(t, map[string]string{url: wrong}); !hasLibreTranslateIssue(issues, "translator_libretranslate_url") {
			t.Errorf("%q: issues = %+v", wrong, issues)
		}
	}
	// A key alone does nothing, so it is a mistake.
	if _, issues := loadLibreTranslate(t, map[string]string{key: "lt-key"}); !hasLibreTranslateIssue(issues, "translator_libretranslate_url") {
		t.Errorf("a key without a URL: issues = %+v", issues)
	}
}

func loadLibreTranslate(t *testing.T, env map[string]string) (*Config, []Issue) {
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

func hasLibreTranslateIssue(issues []Issue, key string) bool {
	for _, i := range issues {
		if i.Key == key {
			return true
		}
	}
	return false
}
