// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginMailer reports that the build tag no_plugins left the plugins out.
func newPluginMailer(*config.Config, kernel.Meter) (kernel.Mailer, error) {
	return nil, config.ExcludedError("mail", "plugin", "no_plugins")
}
