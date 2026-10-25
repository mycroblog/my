// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_ses

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSESMailer reports that the build tag no_ses left the SES adapter out.
func newSESMailer(context.Context, *config.Config) (kernel.Mailer, error) {
	return nil, config.ExcludedError("mail", "ses", "no_ses")
}
