// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_gcs

package platform

import (
	"context"
	"fmt"
	"io"

	blobgcs "github.com/mycroblog/my/internal/adapter/blob/gcs"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/platform/config"
)

// newGCSBlobs returns the blob store of blob = gcs (ADR-0005), on Application Default
// Credentials, and what releases its client. The client reaches only the Google APIs, so it does
// not go through the SSRF-guarded kernel.HTTPClient, whose guard is for the destinations that
// others choose.
func newGCSBlobs(ctx context.Context, cfg *config.Config) (blob.Store, io.Closer, error) {
	store, err := blobgcs.New(ctx, blobgcs.Options{Bucket: cfg.BlobGCSBucket, BaseURL: cfg.MediaBaseURL()})
	if err != nil {
		return nil, nil, fmt.Errorf("blob = gcs: %w", err)
	}
	return store, store, nil
}
