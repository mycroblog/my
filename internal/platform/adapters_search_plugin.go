// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	searchplugin "github.com/mycroblog/my/internal/adapter/search/plugin"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["plugin"] = newPluginSearchIndex }

// newPluginSearchIndex returns the Index of search = plugin (ADR-0146 §11, ADR-0149 §3). It opens no connection: a
// plugin that is down at start fails the searches and delays the writes, which their jobs retry, and the server still
// starts (ADR-0146 §15).
func newPluginSearchIndex(_ context.Context, cfg *config.Config, _ *slog.Logger, meter kernel.Meter) (search.Index, error) {
	conn, err := NewPluginConn(cfg, kind.Search, meter)
	if err != nil {
		return nil, err
	}
	return searchplugin.New(conn), nil
}
