// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_acme

package platform

import (
	"log/slog"

	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/platform/config"
)

// newACMEClient reports that the build tag no_acme left the ACME adapter out.
func newACMEClient(*config.Config, *slog.Logger) (instance.ACMEClient, error) {
	return nil, config.ExcludedError("tls", config.TLSACME, "no_acme")
}
