// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginMediaProcessor reports that the build tag no_plugins left the plugins out.
func newPluginMediaProcessor(*config.Config, media.Processor, kernel.Meter) (media.Processor, error) {
	return nil, config.ExcludedError("media", "plugin", "no_plugins")
}
