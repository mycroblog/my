// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/platform/config"
)

// newHookPlugins returns no plugin: the configuration refuses an address in this build.
func newHookPlugins(*config.Config, *App) (map[hook.Name]apphook.Plugin, error) {
	return nil, nil
}
