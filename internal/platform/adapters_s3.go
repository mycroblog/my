// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_s3

package platform

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	blobs3 "github.com/mycroblog/my/internal/adapter/blob/s3"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/platform/config"
)

// newS3Blobs returns the blob store of blob = s3 (ADR-0005).
func newS3Blobs(ctx context.Context, cfg *config.Config) (blob.Store, error) {
	// Without a key in the configuration, the credentials come from the SDK's default chain:
	// the environment, the shared files, the role of the instance or container. The SDK reaches
	// only the endpoint that the operator configured, so it does not go through the
	// SSRF-guarded kernel.HTTPClient, whose guard is for the destinations that others choose.
	var opts []func(*awsconfig.LoadOptions) error
	if cfg.BlobS3Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.BlobS3Region))
	}
	if cfg.BlobS3AccessKeyID != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.BlobS3AccessKeyID, cfg.BlobS3SecretAccessKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("blob = s3: load the AWS configuration: %w", err)
	}
	return blobs3.New(awsCfg, blobs3.Options{
		Bucket: cfg.BlobS3Bucket, Endpoint: cfg.BlobS3Endpoint, PathStyle: cfg.BlobS3PathStyle,
		BaseURL: cfg.MediaBaseURL(),
	})
}
