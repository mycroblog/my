// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"io"

	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/dbupgrade"
	"github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/translation"
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
	diagnostics  diagnostics.Repository
	translations translation.Repository
	search       search.Repository
	retention    retentionRepositories
	projections  projection.Store
	dataUpgrades dbupgrade.Repository
	sealedValues rewrap.Store
}

// openDatabase connects the database that cfg.DB selects and returns its repositories. With
// queue = db and kv = db it also sets the queue and the key-value store on the database.
func (a *App) openDatabase(ctx context.Context, cfg *config.Config) (*repositories, error) {
	queueOpts := queuedb.Options{Clock: a.Clock, IDs: a.IDs, Tracer: a.Tracer}
	if hasWaker(cfg.Queue) {
		if a.waker == nil {
			w, closer, err := newWaker(ctx, cfg, a.Clock, a.ErrorReporter)
			if err != nil {
				return nil, err
			}
			a.waker, a.closeWaker = w, closer
		}
		if err := a.routeWaker(ctx, cfg, a.routeWakerForTests); err != nil {
			return nil, err
		}
		queueOpts.Waker, queueOpts.Projectors, queueOpts.Logger = a.waker, a.Projectors, a.Logger
	} else {
		a.waker = nil
	}
	switch cfg.DB {
	case "sqlite":
		return a.openSQLite(ctx, cfg, queueOpts)
	case "postgres":
		return a.openPostgres(ctx, cfg, queueOpts)
	default:
		return nil, fmt.Errorf("db %q is not implemented yet", cfg.DB)
	}
}

// queueInDatabase reports whether the jobs of the queue are in the database: queue = db, and the
// queues with a waker, which only carry wake-up signals for them (ADR-0004).
func queueInDatabase(q string) bool { return q == "db" || hasWaker(q) }

// hasWaker reports whether the queue is the database queue with an external queue carrying its
// wake-up signals: cloudtasks and sqs (ADR-0004).
func hasWaker(q string) bool { return q == "cloudtasks" || q == "sqs" }

// newWaker returns the waker of the default queue of cfg, which hasWaker accepts, and what
// releases its client.
func newWaker(ctx context.Context, cfg *config.Config, clock kernel.Clock, reporter kernel.ErrorReporter) (queue.Waker, io.Closer, error) {
	if cfg.Queue == "sqs" {
		return newSQSWaker(ctx, cfg.QueueSQSQueueURL, clock)
	}
	return newCloudTasksWaker(ctx, cfg, config.QueueRoute{Queue: cfg.QueueCloudTasksQueue, URL: cfg.QueueCloudTasksURL}, clock, reporter)
}

// routeWaker puts the routes of cfg in front of a.waker (ADR-0004 §24). Without routes it changes
// nothing, so that a deployment with one queue behaves as if they did not exist. stand, when set,
// makes the waker of a route; a route to the same target shares its waker.
func (a *App) routeWaker(ctx context.Context, cfg *config.Config, stand func(config.QueueRoute) queue.Waker) error {
	routes := cfg.QueueRoutes()
	if len(routes) == 0 {
		return nil
	}
	wakers := map[config.QueueRoute]queue.Waker{}
	closers := []io.Closer{}
	if a.closeWaker != nil {
		closers = append(closers, a.closeWaker)
	}
	wakeRoutes := make([]queue.WakeRoute, len(routes))
	for i, r := range routes {
		target := config.QueueRoute{Queue: r.Queue, URL: r.URL}
		w, ok := wakers[target]
		if !ok {
			var closer io.Closer
			var err error
			switch {
			case stand != nil:
				w = stand(target)
			case cfg.Queue == "sqs":
				w, closer, err = newSQSWaker(ctx, r.Queue, a.Clock)
			default:
				w, closer, err = newCloudTasksWaker(ctx, cfg, target, a.Clock, a.ErrorReporter)
			}
			if err != nil {
				return err
			}
			if closer != nil {
				closers = append(closers, closer)
			}
			wakers[target] = w
		}
		wakeRoutes[i] = queue.WakeRoute{Pattern: r.Pattern, Waker: w}
	}
	a.wakeRouter = queue.NewWakeRouter(a.waker, wakeRoutes)
	a.waker, a.closeWaker = a.wakeRouter, closeAll(closers)
	return nil
}

// closeAll is an io.Closer of several, which closes all of them.
type closeAll []io.Closer

// Close implements io.Closer.
func (c closeAll) Close() error {
	var errs []error
	for _, closer := range c {
		errs = append(errs, closer.Close())
	}
	return errors.Join(errs...)
}

// valkeyServer is the connection to valkey_url that the adapters configured with it share.
type valkeyServer interface {
	// Ping reports whether the server answers.
	Ping(ctx context.Context) error
	Close()
}
