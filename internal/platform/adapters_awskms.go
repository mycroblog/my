// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_awskms

package platform

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/mycroblog/my/internal/adapter/keymanager/awskms"
	"github.com/mycroblog/my/internal/app/kernel"
)

// newAWSKMS returns the AWS KMS key manager that wraps with the key key and also unwraps with
// previous, resolving each to its ARN. The credentials, the region, and the HTTP client come from
// the SDK's default chain: the execution role, the environment, the shared files. The SDK reaches
// only the AWS KMS endpoint, so it does not go through the SSRF-guarded kernel.HTTPClient.
func newAWSKMS(ctx context.Context, key string, previous []string) (kernel.KeyManager, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("key_manager = awskms: load the AWS configuration: %w", err)
	}
	km, err := awskms.New(ctx, cfg, awskms.Options{Key: key, PreviousKeys: previous})
	if err != nil {
		return nil, fmt.Errorf("key_manager = awskms: %w", err)
	}
	return km, nil
}
