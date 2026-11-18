// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform

import (
	"context"
	"errors"
	"log/slog"

	translatorfake "github.com/mycroblog/my/internal/adapter/translator/fake"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// This file is the only one outside the tests that imports the fake translation provider, and only
// the test builds with the build tag e2e_fakes have it (ADR-0015 §6). scripts/go-fakes.sh
// fails when cmd/my without the tag depends on the fake, the Dockerfile refuses the tag, and
// scripts/check-release-build.sh refuses a binary built with it.

func init() { fakeTranslationProviders = newFakeTranslationProviders }

// newFakeTranslationProviders returns the provider fake when translator_fake = true: it puts the
// target language before each text and fails as a provider that is down on a text with
// translatorfake.FailMarker. Like the configuration's check, it refuses any profile but
// development.
func newFakeTranslationProviders(ctx context.Context, cfg *config.Config, logger *slog.Logger) ([]translation.Provider, error) {
	if !cfg.TranslatorFake {
		return nil, nil
	}
	if cfg.Profile != "development" {
		return nil, errors.New("translator_fake needs profile = development: the provider fake translates nothing")
	}
	logger.WarnContext(ctx, "this is a test build (build tag e2e_fakes): translator_fake registers a provider that translates nothing")
	return []translation.Provider{{
		ID:         translatorfake.ProviderID,
		Name:       "Fake",
		Translator: translatorfake.New(translatorfake.Options{FailOn: translatorfake.FailMarker}),
	}}, nil
}
