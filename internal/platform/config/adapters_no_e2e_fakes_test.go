// SPDX-License-Identifier: AGPL-3.0-only

//go:build !e2e_fakes

package config

import (
	"errors"
	"strings"
	"testing"
)

// A build without the tag e2e_fakes rejects payments_provider = fake before start, in any
// profile, and does not offer it among the values.
func TestLoadRejectsFakePaymentsWithoutTheTag(t *testing.T) {
	for _, profile := range []string{"development", "production"} {
		_, _, err := Load(source(with(minimal(), "MY_PAYMENTS_PROVIDER", "fake", "MY_PROFILE", profile), nil))
		var ve *ValidationError
		if !errors.As(err, &ve) || len(ve.Issues) != 1 {
			t.Fatalf("Load() with payments_provider = fake, profile = %s: %v, want one issue", profile, err)
		}
		want := `payments_provider (MY_PAYMENTS_PROVIDER): "fake" is only in test builds: the build tag e2e_fakes adds its adapter, and released binaries never have it (docs/reference/build-tags.md)`
		if got := ve.Issues[0].String(); got != want {
			t.Errorf("issue = %q, want %q", got, want)
		}
	}
	_, _, err := Load(source(with(minimal(), "MY_PAYMENTS_PROVIDER", "other"), nil))
	if err == nil || strings.Contains(err.Error(), "fake") {
		t.Errorf("Load() with an unknown provider: %v, want an error that does not offer fake", err)
	}
}
