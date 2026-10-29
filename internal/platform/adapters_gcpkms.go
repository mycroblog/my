// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_gcpkms

package platform

import (
	"context"
	"fmt"
	"io"

	"github.com/mycroblog/my/internal/adapter/keymanager/gcpkms"
	"github.com/mycroblog/my/internal/app/kernel"
)

// newGCPKMS returns the Cloud KMS key manager that wraps with the CryptoKey key and also unwraps
// with previous, on Application Default Credentials, and what releases its client.
func newGCPKMS(ctx context.Context, key string, previous []string) (kernel.KeyManager, io.Closer, error) {
	km, err := gcpkms.New(ctx, gcpkms.Options{Key: key, PreviousKeys: previous})
	if err != nil {
		return nil, nil, fmt.Errorf("key_manager = gcpkms: %w", err)
	}
	return km, km, nil
}
