// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/federation"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/queue"
	domainfederation "github.com/mycroblog/my/internal/domain/federation"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpClosureDeletes builds the deliveries of the service closure (ADR-0088 §3): the pages of
// the closure's Deletes, their first page in the transaction of the closure, and the periodic
// check that records delivery_completed.
func (a *App) setUpClosureDeletes(cfg *config.Config, repo federation.Repository, outbox queue.Outbox) {
	a.ClosureDeletes = federation.NewClosureDeletes(federation.ClosureDeletesDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Outbox: outbox, Accounts: a.Accounts, Moves: a.Accounts,
		Closure: closureSettings{a},
		Complete: func(ctx context.Context) error {
			_, err := a.Instance.CompleteClosureDelivery(ctx)
			if errors.Is(err, domaininstance.ErrClosureTransition) {
				// Another worker recorded it first.
				return nil
			}
			return err
		},
		Clock: a.Clock, IDs: a.IDs, Host: cfg.Host, Logger: a.Logger,
	})
	a.ClosureDeletes.RegisterJobs(a.Handlers, a.Scheduler)
	federation.RegisterClosureProjectors(a.Projectors, outbox)
}

// closureState reads the state of the service closure, per call.
func (a *App) closureState(ctx context.Context) (domaininstance.ClosureState, error) {
	c, err := a.Instance.Closure(ctx)
	return c.CurrentState(), err
}

// closureSettings tells the federation module whether the server is closed and delivers the
// closure's Deletes (instance.ClosureAllows: retract without federate). It reads a.Instance per
// call.
type closureSettings struct{ a *App }

// ClosureRetracts implements federation.ClosureSettings.
func (c closureSettings) ClosureRetracts(ctx context.Context) (bool, error) {
	st, err := c.a.closureState(ctx)
	if err != nil {
		return false, err
	}
	return domaininstance.ClosureAllows(st, domaininstance.ClosureRetract) && !domaininstance.ClosureAllows(st, domaininstance.ClosureFederate), nil
}

// closureDeliveries reads the progress of the closure's Deletes from the federation module for
// the instance module, which is set up before it.
type closureDeliveries struct{ a *App }

// ClosureProgress implements appinstance.ClosureDeliveries.
func (c closureDeliveries) ClosureProgress(ctx context.Context) (appinstance.ClosureProgress, error) {
	if c.a.ClosureDeletes == nil {
		return appinstance.ClosureProgress{}, nil
	}
	p, err := c.a.ClosureDeletes.Progress(ctx)
	if err != nil {
		return appinstance.ClosureProgress{}, err
	}
	out := appinstance.ClosureProgress{Accounts: p.Accounts, FanOuts: p.FanOuts, Deliveries: map[string]int{}}
	for _, st := range domainfederation.ClosureDeliveryStates() {
		out.Deliveries[string(st)] = p.Deliveries[st]
	}
	return out, nil
}
