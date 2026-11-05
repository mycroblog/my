// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package config

import (
	"errors"
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
