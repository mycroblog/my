// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_s3

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/platform/config"
)

// newS3Blobs reports that the build tag no_s3 left the S3 blob store out.
func newS3Blobs(context.Context, *config.Config) (blob.Store, error) {
	return nil, config.ExcludedError("blob", "s3", "no_s3")
}
