// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_billing_aws

package platform

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	billingaws "github.com/mycroblog/my/internal/adapter/billing/aws"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { billingSourceAdapters["aws"] = newAWSBillingSource }

// newAWSBillingSource returns the BillingSource of billing_sources = aws (ADR-0068 §6, §8).
func newAWSBillingSource(ctx context.Context, cfg *config.Config, _ kernel.Meter) (finance.BillingSource, error) {
	// The credentials and the HTTP client come from the SDK's default chain (the execution role,
	// the environment, the shared files), or, with billing_aws_role_arn, from the role assumed
	// with the Google ID token of the instance. The SDK reaches only the AWS endpoints, so it does
	// not go through the SSRF-guarded kernel.HTTPClient.
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(billingaws.Region))
	if err != nil {
		return nil, fmt.Errorf("load the AWS configuration: %w", err)
	}
	if cfg.BillingAWSRoleARN != "" {
		awsCfg = billingaws.WithWebIdentityRole(awsCfg, cfg.BillingAWSRoleARN)
	}
	return billingaws.NewFromConfig(awsCfg, billingaws.Options{
		Metric: cfg.BillingAWSMetric, Filter: cfg.BillingAWSFilter,
		LinkedAccounts: cfg.BillingAWSLinkedAccounts, Tag: cfg.BillingAWSTag,
	})
}
