// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_sqlite

package platform

import (
	"context"

	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/platform/config"
)

// openSQLite reports that the build tag no_sqlite left the SQLite adapter out.
func (a *App) openSQLite(context.Context, *config.Config, queuedb.Options) (*repositories, error) {
	return nil, config.ExcludedError("database", "sqlite", "no_sqlite")
}
