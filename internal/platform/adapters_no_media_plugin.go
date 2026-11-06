// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginMediaProcessor reports that the build tag no_plugins left the plugins out.
func newPluginMediaProcessor(cfg *config.Config, _ media.Processor, _ kernel.Meter) (media.Processor, error) {
	key := "media_image"
	switch {
	case cfg.MediaImage == "plugin":
	case cfg.MediaVideo == "plugin":
		key = "media_video"
	case cfg.MediaAudio == "plugin":
		key = "media_audio"
	}
	return nil, config.ExcludedError(key, "plugin", "no_plugins")
}
