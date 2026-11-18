// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package config

import (
	"errors"
	"strings"
	"testing"
)

// TestE2EFakesPaymentsProfile: a test build takes payments = fake with profile =
// development only.
func TestE2EFakesPaymentsProfile(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_PAYMENTS", "fake", "MY_PROFILE", "development"), nil))
	if err != nil || cfg.PaymentsProvider != "fake" {
		t.Fatalf("Load() with payments = fake, profile = development: %+v, %v", cfg, err)
	}
	for _, profile := range []string{"production", unset} {
		_, _, err := Load(source(with(minimal(), "MY_PAYMENTS", "fake", "MY_PROFILE", profile), nil))
		var ve *ValidationError
		want := "payments (MY_PAYMENTS): fake is refused in the production profile: it records payments as paid without any money moving; set profile = development"
		if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].String() != want {
			t.Errorf("Load() with payments = fake, profile = %q: %v, want %q", profile, err, want)
		}
	}
}

// TestE2EFakesMailProfile: a test build takes mail = fake with profile = development only, and
// the sender address is still required.
func TestE2EFakesMailProfile(t *testing.T) {
	env := with(minimal(), "MY_MAIL", "fake", "MY_MAIL_FROM", "noreply@example.test")
	cfg, _, err := Load(source(with(env, "MY_PROFILE", "development"), nil))
	if err != nil || cfg.Mail != "fake" {
		t.Fatalf("Load() with mail = fake, profile = development: %+v, %v", cfg, err)
	}
	for _, profile := range []string{"production", unset} {
		_, _, err := Load(source(with(env, "MY_PROFILE", profile), nil))
		var ve *ValidationError
		want := "mail (MY_MAIL): fake is refused in the production profile: it sends nothing and writes each message to the log; set profile = development"
		if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].String() != want {
			t.Errorf("Load() with mail = fake, profile = %q: %v, want %q", profile, err, want)
		}
	}
	_, _, err = Load(source(with(minimal(), "MY_MAIL", "fake", "MY_PROFILE", "development"), nil))
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 1 || !strings.Contains(ve.Issues[0].String(), "MY_MAIL_FROM") {
		t.Errorf("Load() with mail = fake and no mail_from: %v, want one issue about MY_MAIL_FROM", err)
	}
}

// TestE2EFakesSearchProfile: a test build takes search = fake with profile = development only
// (ADR-0149 §3).
func TestE2EFakesSearchProfile(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_SEARCH", "fake", "MY_PROFILE", "development"), nil))
	if err != nil || cfg.Search != "fake" {
		t.Fatalf("Load() with search = fake, profile = development: %+v, %v", cfg, err)
	}
	for _, profile := range []string{"production", unset} {
		_, _, err := Load(source(with(minimal(), "MY_SEARCH", "fake", "MY_PROFILE", profile), nil))
		var ve *ValidationError
		want := "search (MY_SEARCH): fake is refused in the production profile: it is an index in memory for tests; set profile = development"
		if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].String() != want {
			t.Errorf("Load() with search = fake, profile = %q: %v, want %q", profile, err, want)
		}
	}
}

// TestE2EFakesTranslatorProfile: a test build takes translator_fake = true with profile =
// development only.
func TestE2EFakesTranslatorProfile(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_TRANSLATOR_FAKE", "true", "MY_PROFILE", "development"), nil))
	if err != nil || !cfg.TranslatorFake {
		t.Fatalf("Load() with translator_fake = true, profile = development: %+v, %v", cfg, err)
	}
	for _, profile := range []string{"production", unset} {
		_, _, err := Load(source(with(minimal(), "MY_TRANSLATOR_FAKE", "true", "MY_PROFILE", profile), nil))
		var ve *ValidationError
		want := "translator_fake (MY_TRANSLATOR_FAKE): true is refused in the production profile: the provider fake translates nothing; set profile = development"
		if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].String() != want {
			t.Errorf("Load() with translator_fake = true, profile = %q: %v, want %q", profile, err, want)
		}
	}
}
