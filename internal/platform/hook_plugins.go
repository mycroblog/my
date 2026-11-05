// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	hookconnect "github.com/mycroblog/my/internal/adapter/hook/connect"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/platform/config"
)

// newHookPlugins returns the plugin of each hook whose address is set.
func newHookPlugins(cfg *config.Config, a *App) (map[hook.Name]apphook.Plugin, error) {
	plugins := map[hook.Name]apphook.Plugin{}
	for _, name := range hook.Names() {
		if !cfg.PluginDestination(string(name)).Enabled() {
			continue
		}
		conn, err := NewPluginConn(cfg, string(name), a.Meter)
		if err != nil {
			return nil, err
		}
		plugins[name] = hookconnect.New(conn)
	}
	return plugins, nil
}
