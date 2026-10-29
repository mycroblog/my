// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_gcpkms

package platform

import (
	"context"
	"io"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newGCPKMS reports that the build tag no_gcpkms left the Cloud KMS key manager out.
func newGCPKMS(context.Context, string, []string) (kernel.KeyManager, io.Closer, error) {
	return nil, nil, config.ExcludedError("key_manager", "gcpkms", "no_gcpkms")
}
