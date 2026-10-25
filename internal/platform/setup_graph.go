// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpGraph registers the graph module's projectors, builds its service, and registers its jobs.
// The moves of accounts enqueue the migration of their local followers, blocks, and mutes
// (ADR-0089).
func (a *App) setUpGraph(cfg *config.Config, repo graph.Repository) {
	graph.RegisterProjectors(a.Projectors, repo)
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Graph = graph.NewService(graph.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Accounts: a.Accounts, Clock: a.Clock, IDs: a.IDs, Host: cfg.Host,
		Interactions: a.AccountPolicy, Outbox: outbox, Jobs: a.Jobs,
	})
	a.Graph.RegisterMoveProjector(a.Projectors)
	a.Graph.RegisterJobs(a.Handlers, a.Scheduler)
}
