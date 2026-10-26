// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/platform/config"
)

// openSQLite opens the SQLite database of db = sqlite and, when cfg selects them, the
// queue and the key-value store kept in it.
func (a *App) openSQLite(ctx context.Context, cfg *config.Config, queueOpts queuedb.Options) (*repositories, error) {
	d, err := sqlite.Open(ctx, cfg.DatabaseURL, uow.Options{Projectors: a.Projectors})
	if err != nil {
		return nil, err
	}
	a.UnitOfWork, a.ready, a.closer = d, d.Ready, d.Close
	d.RegisterMetrics(a.Meter)
	a.dbSize = d
	d.SetTracer(a.Tracer)
	// The attempt log is in the database whatever the queue (ADR-0064 §1).
	dbQueue := queuedb.NewSQLite(d, queueOpts)
	a.JobAttempts = dbQueue
	if cfg.Queue == "db" {
		a.setDBQueue(dbQueue)
	}
	if cfg.KV == "db" {
		a.KV = kvdb.NewSQLite(d, a.Clock)
	}
	return &repositories{
		identities: d.IdentityRepository(), accounts: d.AccountRepository(), instances: d.InstanceRepository(),
		follows: d.GraphRepository(), statuses: d.StatusRepository(), linkCards: d.StatusRepository(), quotes: d.StatusRepository(),
		interactions: d.InteractionRepository(), feeds: d.TimelineRepository(),
		notifs: d.NotificationRepository(), pushes: d.NotificationRepository(),
		medias: d.MediaRepository(), emojis: d.CustomEmojiRepository(),
		feds: d.FederationRepository(), mods: d.ModerationRepository(), hooks: d.WebhookRepository(),
		finance: d.FinanceRepository(), payments: d.PaymentRepository(), supporters: d.SupporterRepository(),
		voice: d.VoiceRepository(),
		retention: retentionRepositories{
			statuses: d.StatusRepository(), purges: d.FederationRepository(), profiles: d.AccountRepository(), emojis: d.CustomEmojiRepository(),
			autoDeletes: d.StatusRepository(),
		},
		projections: d.ProjectionStore(), dataUpgrades: d.DataUpgradeRepository(),
	}, nil
}
