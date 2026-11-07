// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_translator_aws

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// awsTranslationProviders reports that the build tag no_translator_aws left Amazon Translate out
// when translator_aws_region asks for it, and returns no provider otherwise.
func awsTranslationProviders(_ context.Context, cfg *config.Config) ([]translation.Provider, error) {
	if cfg.TranslatorAWSRegion == "" {
		return nil, nil
	}
	return nil, config.ExcludedError("translator_aws_region", cfg.TranslatorAWSRegion, "no_translator_aws")
}
