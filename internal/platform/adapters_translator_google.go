// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_translator_google

package platform

import (
	"context"
	"fmt"
	"io"

	translatorgoogle "github.com/mycroblog/my/internal/adapter/translator/google"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// googleTranslationProviders returns Google Cloud Translation as a provider that the instance settings can enable, when
// the configuration names its project, on Application Default Credentials, with what releases its client. It makes no
// call: a project without the API or the role fails translations, and the server still starts.
//
// Google's attribution requirements ask for its "powered by Google Translate" badge next to every translation, linking
// to translate.google.com. Google offers the badge only as a download, with no address that it keeps for embedding, so
// the provider has no LogoURL, and readers see the name and the link (ADR-0148 §15).
func googleTranslationProviders(ctx context.Context, cfg *config.Config) ([]translation.Provider, io.Closer, error) {
	if cfg.TranslatorGoogleProject == "" {
		return nil, nil, nil
	}
	t, err := translatorgoogle.New(ctx, translatorgoogle.Options{Project: cfg.TranslatorGoogleProject, Location: cfg.TranslatorGoogleLocation})
	if err != nil {
		return nil, nil, fmt.Errorf("translator_google_project: %w", err)
	}
	return []translation.Provider{{
		ID: translatorgoogle.ProviderID, Name: translatorgoogle.Name, URL: translatorgoogle.AttributionURL, Translator: t,
	}}, t, nil
}
