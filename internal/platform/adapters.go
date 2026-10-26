// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"

	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/platform/config"
)

// The adapters that need a client library, a driver, or a cloud SDK are wired in files of
// their own, adapters_<name>.go, which the build tag no_<name> replaces with
// adapters_no_<name>.go (NFR-PORT-4, ADR-0010): there, the constructor reports that the
// adapter is not in this build, and the binary links neither the adapter nor its
// dependencies. Load already rejects the settings that select such an adapter
// (config.ExcludedError), so the replacements are reached only through a Config built
// without Load. docs/reference/build-tags.md lists the tags.

// repositories are the repositories of the database that cfg.DB selects.
type repositories struct {
	identities   identity.Repository
	accounts     account.Repository
	instances    instance.Repository
	follows      graph.Repository
	statuses     content.Repository
	linkCards    content.LinkCardRepository
	quotes       content.QuoteRepository
	interactions content.InteractionRepository
	feeds        timeline.Repository
	notifs       notification.Repository
	pushes       notification.PushRepository
	medias       content.MediaRepository
	emojis       content.CustomEmojiRepository
	feds         federationRepository
	mods         moderation.Repository
	hooks        webhook.Repository
	finance      finance.Repository
	payments     supporters.PaymentRepository
	supporters   supporters.Repository
	voice        voice.Repository
	retention    retentionRepositories
	projections  projection.Store
	dataUpgrades dbupgrade.Repository
}

// openDatabase connects the database that cfg.DB selects and returns its repositories. With
// queue = db and kv = db it also sets the queue and the key-value store on the database.
func (a *App) openDatabase(ctx context.Context, cfg *config.Config) (*repositories, error) {
	queueOpts := queuedb.Options{Clock: a.Clock, IDs: a.IDs, Tracer: a.Tracer}
	switch cfg.DB {
	case "sqlite":
		return a.openSQLite(ctx, cfg, queueOpts)
	case "postgres":
		return a.openPostgres(ctx, cfg, queueOpts)
	default:
		return nil, fmt.Errorf("db %q is not implemented yet", cfg.DB)
	}
}

// valkeyServer is the connection to valkey_url that the adapters configured with it share.
type valkeyServer interface {
	// Ping reports whether the server answers.
	Ping(ctx context.Context) error
	Close()
}
