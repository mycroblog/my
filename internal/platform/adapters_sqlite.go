// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitedbupgrade "github.com/mycroblog/my/internal/adapter/db/sqlite/dbupgrade"
	sqlitediagnostics "github.com/mycroblog/my/internal/adapter/db/sqlite/diagnostics"
	sqlitefederation "github.com/mycroblog/my/internal/adapter/db/sqlite/federation"
	sqlitefinance "github.com/mycroblog/my/internal/adapter/db/sqlite/finance"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqlitehook "github.com/mycroblog/my/internal/adapter/db/sqlite/hook"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqliteinstance "github.com/mycroblog/my/internal/adapter/db/sqlite/instance"
	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	sqlitenotification "github.com/mycroblog/my/internal/adapter/db/sqlite/notification"
	sqliterewrap "github.com/mycroblog/my/internal/adapter/db/sqlite/rewrap"
	sqlitesearch "github.com/mycroblog/my/internal/adapter/db/sqlite/search"
	sqlitesupporters "github.com/mycroblog/my/internal/adapter/db/sqlite/supporters"
	sqlitetimeline "github.com/mycroblog/my/internal/adapter/db/sqlite/timeline"
	sqlitetranslation "github.com/mycroblog/my/internal/adapter/db/sqlite/translation"
	sqlitevoice "github.com/mycroblog/my/internal/adapter/db/sqlite/voice"
	sqlitewebhook "github.com/mycroblog/my/internal/adapter/db/sqlite/webhook"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/schema"
)

// openSQLite opens the SQLite database of database = sqlite and, when cfg selects them, the
// queue and the key-value store kept in it.
func (a *App) openSQLite(ctx context.Context, cfg *config.Config, queueOpts queuedb.Options) (*repositories, error) {
	d, err := sqlite.Open(ctx, cfg.DatabaseURL, uow.Options{Projectors: a.Projectors})
	if err != nil {
		return nil, err
	}
	tables := schema.Tables(schema.SQLite)
	a.UnitOfWork, a.closer = d, d.Close
	a.ready = func(ctx context.Context) error { return d.Ready(ctx, tables) }
	d.RegisterMetrics(a.Meter)
	a.dbSize = d
	d.SetTracer(a.Tracer)
	// The attempt log is in the database whatever the queue (ADR-0004 §5).
	dbQueue := queuedb.NewSQLite(d, queueOpts)
	a.JobAttempts = dbQueue
	if queueInDatabase(cfg.Queue) {
		a.setDBQueue(dbQueue)
	}
	if cfg.KV == "db" {
		a.KV = kvdb.NewSQLite(d, a.Clock)
	}
	return &repositories{
		identities: sqliteidentity.NewRepository(d), accounts: sqliteaccount.NewRepository(d), instances: sqliteinstance.NewRepository(d),
		follows: sqlitegraph.NewRepository(d), statuses: sqlitecontent.NewStatusRepository(d), linkCards: sqlitecontent.NewStatusRepository(d), quotes: sqlitecontent.NewStatusRepository(d),
		interactions: sqlitecontent.NewInteractionRepository(d), feeds: sqlitetimeline.NewRepository(d),
		notifs: sqlitenotification.NewRepository(d), pushes: sqlitenotification.NewRepository(d),
		medias: sqlitecontent.NewMediaRepository(d), emojis: sqlitecontent.NewCustomEmojiRepository(d),
		feds: sqlitefederation.NewRepository(d), mods: sqlitemoderation.NewRepository(d), hooks: sqlitewebhook.NewRepository(d),
		finance: sqlitefinance.NewRepository(d), payments: sqlitesupporters.NewPaymentRepository(d), supporters: sqlitesupporters.NewSupporterRepository(d),
		voice: sqlitevoice.NewRepository(d), diagnostics: sqlitediagnostics.NewRepository(d), translations: sqlitetranslation.NewRepository(d), search: sqlitesearch.NewRepository(d),
		decisions: sqlitehook.NewRepository(d), existingStatuses: sqlitecontent.NewStatusRepository(d),
		retention: retentionRepositories{
			statuses: sqlitecontent.NewStatusRepository(d), purges: sqlitefederation.NewRepository(d), profiles: sqliteaccount.NewRepository(d), emojis: sqlitecontent.NewCustomEmojiRepository(d),
			autoDeletes: sqlitecontent.NewStatusRepository(d),
		},
		projections: d.ProjectionStore(tables), dataUpgrades: sqlitedbupgrade.NewDataUpgradeRepository(d),
		sealedValues: sqliterewrap.NewSealedValueStore(d),
	}, nil
}
