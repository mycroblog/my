// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["plugin"] = newPluginSearchIndex }

// newPluginSearchIndex reports that the build tag no_plugins left the plugins out.
func newPluginSearchIndex(context.Context, *config.Config, *slog.Logger, kernel.Meter) (search.Index, error) {
	return nil, config.ExcludedError("search", "plugin", "no_plugins")
}
