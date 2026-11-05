// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	"context"
	"io"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPluginKeyManager reports that the build tag no_plugins left the plugins out.
func newPluginKeyManager(context.Context, *config.Config, kernel.Meter) (kernel.KeyManager, io.Closer, error) {
	return nil, nil, config.ExcludedError("key_manager", "plugin", "no_plugins")
}
