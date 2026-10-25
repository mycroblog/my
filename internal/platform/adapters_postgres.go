// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	pgstream "github.com/mycroblog/my/internal/adapter/stream/postgres"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/platform/config"
)

// openPostgres opens the PostgreSQL database of db = postgres and, when cfg selects them, the
// queue and the key-value store kept in it.
func (a *App) openPostgres(ctx context.Context, cfg *config.Config, queueOpts queuedb.Options) (*repositories, error) {
	d, err := postgres.Open(ctx, cfg.DatabaseURL, uow.Options{Projectors: a.Projectors})
	if err != nil {
		return nil, err
	}
	a.UnitOfWork, a.ready, a.closer = d, d.Ready, func() error { d.Close(); return nil }
	pool := d.Pool()
	a.newPostgresBus = func(listenURL string) (stream.Bus, error) {
		return pgstream.New(pgstream.Options{Pool: pool, ListenURL: listenURL})
	}
	d.RegisterMetrics(a.Meter)
	d.SetTracer(a.Tracer)
	// The attempt log is in the database whatever the queue (ADR-0064 §1).
	dbQueue := queuedb.NewPostgres(d, queueOpts)
	a.JobAttempts = dbQueue
	if cfg.Queue == "db" {
		a.setDBQueue(dbQueue)
	}
	if cfg.KV == "db" {
		a.KV = kvdb.NewPostgres(d, a.Clock)
	}
	return &repositories{
		identities: d.IdentityRepository(), accounts: d.AccountRepository(), instances: d.InstanceRepository(),
		follows: d.GraphRepository(), statuses: d.StatusRepository(), linkCards: d.StatusRepository(), quotes: d.StatusRepository(),
		interactions: d.InteractionRepository(), feeds: d.TimelineRepository(),
		notifs: d.NotificationRepository(), pushes: d.NotificationRepository(),
		medias: d.MediaRepository(), emojis: d.CustomEmojiRepository(),
		feds: d.FederationRepository(), mods: d.ModerationRepository(), hooks: d.WebhookRepository(),
		finance: d.FinanceRepository(), payments: d.PaymentRepository(), supporters: d.SupporterRepository(),
		retention: retentionRepositories{
			statuses: d.StatusRepository(), purges: d.FederationRepository(), profiles: d.AccountRepository(), emojis: d.CustomEmojiRepository(),
			autoDeletes: d.StatusRepository(),
		},
		projections: d.ProjectionStore(), dataUpgrades: d.DataUpgradeRepository(),
	}, nil
}
