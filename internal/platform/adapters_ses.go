// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_ses

package platform

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	mailses "github.com/mycroblog/my/internal/adapter/mail/ses"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSESMailer returns the Mailer of mail = ses (ADR-0086 §6).
func newSESMailer(ctx context.Context, cfg *config.Config) (kernel.Mailer, error) {
	// The credentials and the HTTP client come from the SDK's default chain: the execution
	// role, the environment, the shared files. The SDK reaches only the SES endpoint, so it does
	// not go through the SSRF-guarded kernel.HTTPClient, whose guard is for the destinations
	// that others choose.
	var opts []func(*awsconfig.LoadOptions) error
	if cfg.SESRegion != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.SESRegion))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("mail = ses: load the AWS configuration: %w", err)
	}
	return mailses.NewFromConfig(awsCfg, mailses.Options{From: cfg.MailFrom, ConfigurationSet: cfg.SESConfigurationSet})
}
