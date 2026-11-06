// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	mediaplugin "github.com/mycroblog/my/internal/adapter/media/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginMediaProcessor returns the processor of the kinds of media whose setting is plugin (ADR-0146 §13): the plugin
// of each such kind re-encodes or remuxes, and builtin processes the other kinds, inspects, previews, and checks every
// output. It opens no connection: a plugin that is down at start delays the processing of media, and the server still
// starts (ADR-0146 §15).
func newPluginMediaProcessor(cfg *config.Config, builtin media.Processor, meter kernel.Meter) (media.Processor, error) {
	o := mediaplugin.Options{Builtin: builtin, CoverMaxBytes: cfg.MediaMaxImageBytes}
	for _, c := range []struct {
		setting string
		name    string
		conn    **client.Conn
	}{
		{cfg.MediaImage, kind.MediaImage, &o.Image},
		{cfg.MediaVideo, kind.MediaVideo, &o.Video},
		{cfg.MediaAudio, kind.MediaAudio, &o.Audio},
	} {
		if c.setting != "plugin" {
			continue
		}
		conn, err := NewPluginConn(cfg, c.name, meter)
		if err != nil {
			return nil, err
		}
		*c.conn = conn
	}
	return mediaplugin.New(o)
}
