// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
)

// setUpClosureNotifications registers the service closure notifications (ADR-0088 §1, §2): the
// projector that enqueues them from the service closure events and the jobs that create
// service_closure for every local user and admin_service_closure_due once the closing date has
// passed. moderators are the ports of the moderator notifications, whose emails
// admin_service_closure_due shares; push enqueues the Web Push messages, nil none.
func (a *App) setUpClosureNotifications(repo notification.Repository, moderators notification.ModeratorDeps, push *notification.Pusher) {
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	d := notification.ClosureDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Recipients: closureRecipients{a}, Closures: a.Instance,
		IDs: a.IDs, Clock: a.Clock, Outbox: outbox, Push: push, Moderators: moderators,
	}
	notification.RegisterClosureProjectors(a.Projectors, d)
	notification.RegisterClosureJobs(a.Handlers, d)
}

// closureRecipients pages the local accounts of users for the service closure notifications.
type closureRecipients struct{ a *App }

// LocalUserAccountIDs implements notification.ClosureRecipients: the local accounts but the
// instance actor.
func (r closureRecipients) LocalUserAccountIDs(ctx context.Context, after string, limit int) ([]string, string, error) {
	ids, err := r.a.Accounts.LocalAccountIDs(ctx, after, limit)
	if err != nil || len(ids) == 0 {
		return nil, "", err
	}
	next := ""
	if len(ids) == limit {
		next = ids[len(ids)-1]
	}
	actor, err := r.a.Accounts.InstanceActor(ctx)
	switch {
	case errors.Is(err, account.ErrNotFound):
		return ids, next, nil
	case err != nil:
		return nil, "", err
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != actor.Account.ID {
			out = append(out, id)
		}
	}
	return out, next, nil
}

// closureView returns the state of the service closure as the identity module's closure emails
// read it.
func (a *App) closureView(ctx context.Context) (domainidentity.ServiceClosureView, error) {
	c, err := a.Instance.Closure(ctx)
	if err != nil {
		return domainidentity.ServiceClosureView{}, err
	}
	v := domainidentity.ServiceClosureView{
		AnnouncementID: c.AnnouncementID, ClosesAt: c.Schedule.ClosesAt, ExportUntil: c.Schedule.ExportUntil,
		Message: c.Schedule.Message, AnnouncedAt: c.AnnouncedAt,
	}
	switch c.CurrentState() {
	case instance.ClosureAnnounced:
		v.Stage = domainidentity.ClosureStageAnnounced
	case instance.ClosureClosed, instance.ClosureDeliveryCompleted:
		v.Stage = domainidentity.ClosureStageClosed
	default:
		v = domainidentity.ServiceClosureView{}
	}
	return v, nil
}
