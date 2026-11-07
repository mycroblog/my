// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_translator_aws

package platform

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	translatoraws "github.com/mycroblog/my/internal/adapter/translator/aws"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/platform/config"
)

// awsTranslationProviders returns Amazon Translate as a provider that the instance settings can
// enable, when translator_aws_region is set (ADR-0148 §15). It makes no call.
func awsTranslationProviders(ctx context.Context, cfg *config.Config) ([]translation.Provider, error) {
	if cfg.TranslatorAWSRegion == "" {
		return nil, nil
	}
	// The credentials and the HTTP client come from the SDK's default chain: the execution role,
	// the environment, the shared files. The SDK reaches only the endpoint of Amazon Translate,
	// so it does not go through the SSRF-guarded kernel.HTTPClient, whose guard is for the
	// destinations that others choose.
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.TranslatorAWSRegion))
	if err != nil {
		return nil, fmt.Errorf("translator_aws_region: load the AWS configuration: %w", err)
	}
	t, err := translatoraws.NewFromConfig(awsCfg)
	if err != nil {
		return nil, err
	}
	return []translation.Provider{{
		ID: translatoraws.ProviderID, Name: translatoraws.ProviderName, URL: translatoraws.ProviderURL, Translator: t,
	}}, nil
}
