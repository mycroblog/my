// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_awskms

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newAWSKMS reports that the build tag no_awskms left the AWS KMS key manager out.
func newAWSKMS(context.Context, string, []string) (kernel.KeyManager, error) {
	return nil, config.ExcludedError("key_manager", "awskms", "no_awskms")
}
