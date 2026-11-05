// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	mediaplugin "github.com/mycroblog/my/internal/adapter/media/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginMediaProcessor returns the processor of media = plugin (ADR-0146 §13): the plugin re-encodes and remuxes, and
// builtin inspects, previews, and checks every output. It opens no connection: a plugin that is down at start delays the
// processing of media, and the server still starts (ADR-0146 §15).
func newPluginMediaProcessor(cfg *config.Config, builtin media.Processor, meter kernel.Meter) (media.Processor, error) {
	conn, err := NewPluginConn(cfg, kind.Media, meter)
	if err != nil {
		return nil, err
	}
	return mediaplugin.New(conn, mediaplugin.Options{Builtin: builtin, CoverMaxBytes: cfg.MediaMaxImageBytes})
}
