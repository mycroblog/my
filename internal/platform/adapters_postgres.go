// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	postgresaccount "github.com/mycroblog/my/internal/adapter/db/postgres/account"
	postgrescontent "github.com/mycroblog/my/internal/adapter/db/postgres/content"
	postgresdbupgrade "github.com/mycroblog/my/internal/adapter/db/postgres/dbupgrade"
	postgresdiagnostics "github.com/mycroblog/my/internal/adapter/db/postgres/diagnostics"
	postgresfederation "github.com/mycroblog/my/internal/adapter/db/postgres/federation"
	postgresfinance "github.com/mycroblog/my/internal/adapter/db/postgres/finance"
	postgresgraph "github.com/mycroblog/my/internal/adapter/db/postgres/graph"
	postgreshook "github.com/mycroblog/my/internal/adapter/db/postgres/hook"
	postgresidentity "github.com/mycroblog/my/internal/adapter/db/postgres/identity"
	postgresinstance "github.com/mycroblog/my/internal/adapter/db/postgres/instance"
	postgresmoderation "github.com/mycroblog/my/internal/adapter/db/postgres/moderation"
	postgresnotification "github.com/mycroblog/my/internal/adapter/db/postgres/notification"
	postgresrewrap "github.com/mycroblog/my/internal/adapter/db/postgres/rewrap"
	postgressearch "github.com/mycroblog/my/internal/adapter/db/postgres/search"
	postgressupporters "github.com/mycroblog/my/internal/adapter/db/postgres/supporters"
	postgrestimeline "github.com/mycroblog/my/internal/adapter/db/postgres/timeline"
	postgrestranslation "github.com/mycroblog/my/internal/adapter/db/postgres/translation"
	postgresvoice "github.com/mycroblog/my/internal/adapter/db/postgres/voice"
	postgreswebhook "github.com/mycroblog/my/internal/adapter/db/postgres/webhook"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	pgstream "github.com/mycroblog/my/internal/adapter/stream/postgres"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/schema"
)

// openPostgres opens the PostgreSQL database of database = postgres and, when cfg selects them, the
// queue and the key-value store kept in it.
func (a *App) openPostgres(ctx context.Context, cfg *config.Config, queueOpts queuedb.Options) (*repositories, error) {
	d, err := postgres.Open(ctx, cfg.DatabaseURL, uow.Options{Projectors: a.Projectors})
	if err != nil {
		return nil, err
	}
	d.SetAcquireTimeout(cfg.DatabaseAcquireTimeout)
	tables := schema.Tables(schema.Postgres)
	a.UnitOfWork, a.closer = d, func() error { d.Close(); return nil }
	a.ready = func(ctx context.Context) error { return d.Ready(ctx, tables) }
	pool := d.Pool()
	a.dbPoolMax = int(pool.Config().MaxConns)
	a.newPostgresBus = func(listenURL string) (stream.Bus, error) {
		return pgstream.New(pgstream.Options{Pool: pool, Exec: d.Exec, ListenURL: listenURL, Reporter: a.ErrorReporter})
	}
	d.RegisterMetrics(a.Meter)
	a.dbSize = d
	d.SetTracer(a.Tracer)
	// The attempt log is in the database whatever the queue (ADR-0004 §5).
	dbQueue := queuedb.NewPostgres(d, queueOpts)
	a.JobAttempts = dbQueue
	if queueInDatabase(cfg.Queue) {
		a.setDBQueue(dbQueue)
	}
	if cfg.KV == "db" {
		a.KV = kvdb.NewPostgres(d, a.Clock)
	}
	return &repositories{
		identities: postgresidentity.NewRepository(d), accounts: postgresaccount.NewRepository(d), instances: postgresinstance.NewRepository(d),
		follows: postgresgraph.NewRepository(d), statuses: postgrescontent.NewStatusRepository(d), linkCards: postgrescontent.NewStatusRepository(d), quotes: postgrescontent.NewStatusRepository(d),
		interactions: postgrescontent.NewInteractionRepository(d), feeds: postgrestimeline.NewRepository(d),
		notifs: postgresnotification.NewRepository(d), pushes: postgresnotification.NewRepository(d),
		medias: postgrescontent.NewMediaRepository(d), emojis: postgrescontent.NewCustomEmojiRepository(d),
		feds: postgresfederation.NewRepository(d), mods: postgresmoderation.NewRepository(d), hooks: postgreswebhook.NewRepository(d),
		finance: postgresfinance.NewRepository(d), payments: postgressupporters.NewPaymentRepository(d), supporters: postgressupporters.NewSupporterRepository(d),
		voice: postgresvoice.NewRepository(d), diagnostics: postgresdiagnostics.NewRepository(d), translations: postgrestranslation.NewRepository(d), search: postgressearch.NewRepository(d),
		decisions: postgreshook.NewRepository(d), existingStatuses: postgrescontent.NewStatusRepository(d), drafts: postgrescontent.NewStatusRepository(d),
		retention: retentionRepositories{
			statuses: postgrescontent.NewStatusRepository(d), purges: postgresfederation.NewRepository(d), profiles: postgresaccount.NewRepository(d), emojis: postgrescontent.NewCustomEmojiRepository(d),
			autoDeletes: postgrescontent.NewStatusRepository(d),
		},
		projections: d.ProjectionStore(tables), dataUpgrades: postgresdbupgrade.NewDataUpgradeRepository(d),
		sealedValues: postgresrewrap.NewSealedValueStore(d),
	}, nil
}
