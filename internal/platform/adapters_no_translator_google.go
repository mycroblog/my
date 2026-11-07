// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_translator_google

package platform

import (
	"context"
	"io"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// googleTranslationProviders reports that the build tag no_translator_google left Google Cloud Translation out, when
// the configuration names its project.
func googleTranslationProviders(_ context.Context, cfg *config.Config) ([]translation.Provider, io.Closer, error) {
	if cfg.TranslatorGoogleProject == "" {
		return nil, nil, nil
	}
	return nil, nil, config.ExcludedError("translator_google_project", cfg.TranslatorGoogleProject, "no_translator_google")
}
