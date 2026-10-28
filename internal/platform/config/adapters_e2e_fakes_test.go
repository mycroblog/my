// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package config

import (
	"errors"
	"testing"
)

// TestE2EFakesPaymentsProfile: a test build takes payments_provider = fake with profile =
// development only.
func TestE2EFakesPaymentsProfile(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_PAYMENTS_PROVIDER", "fake", "MY_PROFILE", "development"), nil))
	if err != nil || cfg.PaymentsProvider != "fake" {
		t.Fatalf("Load() with payments_provider = fake, profile = development: %+v, %v", cfg, err)
	}
	for _, profile := range []string{"production", unset} {
		_, _, err := Load(source(with(minimal(), "MY_PAYMENTS_PROVIDER", "fake", "MY_PROFILE", profile), nil))
		var ve *ValidationError
		want := "payments_provider (MY_PAYMENTS_PROVIDER): fake is refused in the production profile: it records payments as paid without any money moving; set profile = development"
		if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].String() != want {
			t.Errorf("Load() with payments_provider = fake, profile = %q: %v, want %q", profile, err, want)
		}
	}
}
