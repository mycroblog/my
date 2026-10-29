// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_gcs

package platform

import (
	"context"
	"io"

	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/platform/config"
)

// newGCSBlobs reports that the build tag no_gcs left the Cloud Storage blob store out.
func newGCSBlobs(context.Context, *config.Config) (blob.Store, io.Closer, error) {
	return nil, nil, config.ExcludedError("blob", "gcs", "no_gcs")
}
