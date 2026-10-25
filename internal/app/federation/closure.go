// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The deliveries of the service closure (FR-809, ADR-0088 §3): once the server is closed, the
// Delete of every local actor goes to its followers and to every known shared inbox, a page of
// accounts at a time, the accounts that moved last (ADR-0089); a periodic check records
// delivery_completed when they have all ended.

// ClosureSettings reads the service closure per request and per job (ADR-0088 §2); the
// composition root implements it over the instance module.
type ClosureSettings interface {
	// ClosureRetracts reports whether the server is closed and still delivers the Delete of its
	// local actors: nothing else goes out, whatever the federation mode.
	ClosureRetracts(ctx context.Context) (bool, error)
}

// closureRetracts reports whether s says that the server delivers the closure's Deletes, false
// when s is nil.
func closureRetracts(ctx context.Context, s ClosureSettings) (bool, error) {
	if s == nil {
		return false, nil
	}
	closing, err := s.ClosureRetracts(ctx)
	if err != nil {
		return false, fmt.Errorf("read the service closure: %w", err)
	}
	return closing, nil
}

// Defaults of the closure's deliveries (ADR-0088 §3).
const (
	// ClosurePage is the number of local accounts that one JobClosureFanOut reads.
	ClosurePage = 100
	// ClosureBacklog is the number of deliveries of the closure's Deletes that have not ended
	// above which the next page waits (Mastodon's value).
	ClosureBacklog = 10_000
	// ClosureWait is how long a page waits before it looks again at the fan-outs and the
	// deliveries of the earlier pages.
	ClosureWait = time.Minute
	// ClosureCompletionInterval is how often JobClosureCompletion runs.
	ClosureCompletionInterval = 10 * time.Minute
)

// ClosureAccounts is what the closure's deliveries read of the account module; *appaccount.Service
// implements it.
type ClosureAccounts interface {
	// LocalAccountIDs pages the local accounts in ID order, the instance actor included.
	LocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error)
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
}

// ClosureDeletesDeps are the ports and settings of the closure's deliveries.
type ClosureDeletesDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	// Outbox enqueues the fan-out of each Delete and the next page in the transaction that
	// records them.
	Outbox   queue.Outbox
	Accounts ClosureAccounts
	// Moves tells the accounts that moved (ADR-0089), whose Deletes go in the last pages; nil
	// means that no account moved.
	Moves MoveReader
	// Closure tells whether the server delivers the closure's Deletes; nil never closes.
	Closure ClosureSettings
	// Complete records delivery_completed (appinstance.Service.CompleteClosureDelivery); nil
	// records nothing.
	Complete func(ctx context.Context) error
	Clock    kernel.Clock
	IDs      kernel.IDGenerator
	// Host is the serving domain of the activity URIs (ADR-0018).
	Host string
	// Page, Backlog, and Wait default to ClosurePage, ClosureBacklog, and ClosureWait.
	Page    int
	Backlog int
	Wait    time.Duration
	// Logger logs the pages that wait and the end of the deliveries; nil means slog.Default().
	Logger *slog.Logger
}

// ClosureDeletes records and fans out the closure's Delete of every local actor, and tells how
// far they have gone (ADR-0088 §3).
//
// JobClosureFanOut, which RegisterClosureProjectors enqueues in the transaction of the closure,
// reads Page local accounts in ID order (the instance actor and the deleted accounts left out,
// the suspended ones in) and, in one transaction, records for each one closure_deletes row and
// the Delete of its actor, enqueues the Delete's fan-out (federation.md §7.2: its followers, then
// every known shared inbox), and enqueues the next page. A first pass takes the accounts that did
// not move, and a second one those that moved. The lifecycle of the accounts does not change. A
// page waits, by enqueueing itself again after Wait, while a Delete of the earlier pages has not
// been fanned out or more than Backlog deliveries have not ended. Running a page again records
// no Delete twice: closure_deletes holds one per account.
//
// JobClosureCompletion, every ClosureCompletionInterval, records delivery_completed once every
// local account has its Delete, every Delete has been fanned out, and every delivery has ended
// (succeeded, permanent_failure, skipped, or dead).
type ClosureDeletes struct {
	d   ClosureDeletesDeps
	rec recorder
}

// NewClosureDeletes returns the closure's deliveries on d.
func NewClosureDeletes(d ClosureDeletesDeps) *ClosureDeletes {
	if d.Page <= 0 {
		d.Page = ClosurePage
	}
	if d.Backlog <= 0 {
		d.Backlog = ClosureBacklog
	}
	if d.Wait <= 0 {
		d.Wait = ClosureWait
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &ClosureDeletes{d: d, rec: recorder{d: RecorderDeps{Repository: d.Repository, Outbox: d.Outbox, IDs: d.IDs, Host: d.Host}}}
}

// RegisterJobs registers the handlers of JobClosureFanOut and JobClosureCompletion, and the
// schedule of the latter.
func (c *ClosureDeletes) RegisterJobs(h *queue.JobHandlerRegistry, s *queue.Scheduler) {
	h.Register(appinstance.JobClosureFanOut, c.handleFanOut)
	h.Register(appinstance.JobClosureCompletion, c.handleCompletion)
	s.Add(queue.Schedule{Name: appinstance.JobClosureCompletion, Every: ClosureCompletionInterval, Job: queue.NewJob{Type: appinstance.JobClosureCompletion}})
}

// RegisterClosureProjectors enqueues the first page of JobClosureFanOut in the transaction that
// closes the server.
func RegisterClosureProjectors(r *uow.Projectors, outbox queue.Outbox) {
	r.Register(appinstance.EventClosureRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appinstance.ClosureRecorded)
		if !ok || !ev.Closes() {
			return nil
		}
		j, err := closurePage{}.job(time.Time{})
		if err != nil {
			return err
		}
		return outbox.EnqueueTx(ctx, tx, j)
	})
}

// closurePage is the payload of JobClosureFanOut: the pass (the accounts that moved, or the
// others), the cursor in it, and how many times the page has waited.
type closurePage struct {
	Moved bool   `json:"moved,omitempty"`
	After string `json:"after,omitempty"`
	Waits int    `json:"waits,omitempty"`
}

// job returns the JobClosureFanOut of p, to run at runAt (now for the zero time). The pages share
// an ordering key, so that two runs of the chain never overlap.
func (p closurePage) job(runAt time.Time) (queue.NewJob, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{
		Type: appinstance.JobClosureFanOut, Payload: b, RunAt: runAt, OrderingKey: appinstance.JobClosureFanOut,
		IdempotencyKey: "closure_fanout:" + strconv.FormatBool(p.Moved) + ":" + p.After + ":" + strconv.Itoa(p.Waits),
	}, nil
}

func (c *ClosureDeletes) handleFanOut(ctx context.Context, j queue.Job) queue.Result {
	var p closurePage
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return queue.Fail(fmt.Errorf("federation: closure fan-out payload %s: %w", j.Payload, err))
		}
	}
	if err := c.runPage(ctx, p); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// runPage runs one page of the closure's fan-out. It is safe to repeat.
func (c *ClosureDeletes) runPage(ctx context.Context, p closurePage) error {
	switch closing, err := closureRetracts(ctx, c.d.Closure); {
	case err != nil:
		return err
	case !closing:
		c.d.Logger.InfoContext(ctx, "closure fan-out stopped: the server does not deliver the closure's Deletes")
		return nil
	}
	progress, err := c.Progress(ctx)
	if err != nil {
		return err
	}
	if progress.FanOuts > 0 || progress.Pending() > c.d.Backlog {
		wait := p
		wait.Waits++
		j, err := wait.job(c.d.Clock.Now().Add(c.d.Wait))
		if err != nil {
			return err
		}
		c.d.Logger.DebugContext(ctx, "closure fan-out waits", slog.Int("fan_outs", progress.FanOuts), slog.Int("pending_deliveries", progress.Pending()))
		return c.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return c.d.Outbox.EnqueueTx(ctx, tx, j) })
	}
	ids, err := c.d.Accounts.LocalAccountIDs(ctx, p.After, c.d.Page)
	if err != nil {
		return err
	}
	var accounts []string
	for _, id := range ids {
		ok, moved, err := c.closes(ctx, id)
		if err != nil {
			return err
		}
		if ok && moved == p.Moved {
			accounts = append(accounts, id)
		}
	}
	var next *queue.NewJob
	switch {
	case len(ids) == c.d.Page:
		j, err := closurePage{Moved: p.Moved, After: ids[len(ids)-1]}.job(time.Time{})
		if err != nil {
			return err
		}
		next = &j
	case !p.Moved:
		j, err := closurePage{Moved: true}.job(time.Time{})
		if err != nil {
			return err
		}
		next = &j
	}
	now := kernel.Normalize(c.d.Clock.Now())
	return c.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, id := range accounts {
			d := federation.ClosureDelete{AccountID: id, OutboundActivityID: c.d.IDs.NewID(), RecordedAt: now}
			switch inserted, err := c.d.Repository.InsertClosureDelete(ctx, tx, d); {
			case err != nil:
				return err
			case !inserted:
				continue
			}
			if err := c.rec.record(ctx, tx, federation.OutboundActivity{
				ID: d.OutboundActivityID, Type: federation.ActivityDelete, ActorAccountID: id,
				ObjectKind: federation.OutboundAccount, ObjectID: id, CreatedAt: now,
			}, ""); err != nil {
				return err
			}
		}
		if next == nil {
			return nil
		}
		return c.d.Outbox.EnqueueTx(ctx, tx, *next)
	})
}

// closes reports whether the closure sends the Delete of the local account id: a local user's
// account that is not deleted, suspended or not; and whether it moved (ADR-0089).
func (c *ClosureDeletes) closes(ctx context.Context, id string) (ok, moved bool, err error) {
	a, err := c.d.Accounts.FindByID(ctx, id)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return false, false, nil
	case err != nil:
		return false, false, err
	case !a.Account.IsLocal() || a.IsInstanceActor() || a.Deleted():
		return false, false, nil
	}
	movedTo, err := LocalActorMovedTo(ctx, c.d.Moves, a)
	return err == nil, movedTo != "", err
}

// Progress returns how far the closure's Deletes have gone.
func (c *ClosureDeletes) Progress(ctx context.Context) (federation.ClosureProgress, error) {
	return c.d.Repository.ClosureProgress(ctx, DeliveryRetryPolicy.MaxAttempts)
}

// PendingMoves counts the Moves of local accounts that are still being sent: recorded within the
// retention of the delivery attempts, with a fan-out that has not ended or a delivery that will
// be attempted again (ADR-0089). The closure's checks warn about them.
func (c *ClosureDeletes) PendingMoves(ctx context.Context) (int, error) {
	return c.d.Repository.CountPendingMoves(ctx, c.d.Clock.Now().Add(-DeliveryAttemptRetention), DeliveryRetryPolicy.MaxAttempts)
}

func (c *ClosureDeletes) handleCompletion(ctx context.Context, _ queue.Job) queue.Result {
	if err := c.Complete(ctx); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Complete records delivery_completed when the server delivers the closure's Deletes and they
// have all ended (Delivered). It does nothing otherwise, and is safe to repeat.
func (c *ClosureDeletes) Complete(ctx context.Context) error {
	switch closing, err := closureRetracts(ctx, c.d.Closure); {
	case err != nil:
		return err
	case !closing || c.d.Complete == nil:
		return nil
	}
	switch done, err := c.Delivered(ctx); {
	case err != nil:
		return err
	case !done:
		return nil
	}
	if err := c.d.Complete(ctx); err != nil {
		return err
	}
	c.d.Logger.InfoContext(ctx, "the closure's Deletes have all been delivered")
	return nil
}

// Delivered reports whether every local account that the closure sends a Delete of has one, and
// every one of them has been fanned out and delivered (federation.ClosureProgress.Ended).
func (c *ClosureDeletes) Delivered(ctx context.Context) (bool, error) {
	progress, err := c.Progress(ctx)
	if err != nil || !progress.Ended() {
		return false, err
	}
	after := ""
	for {
		ids, err := c.d.Accounts.LocalAccountIDs(ctx, after, c.d.Page)
		if err != nil {
			return false, err
		}
		recorded, err := c.d.Repository.ClosureDeleteAccounts(ctx, ids)
		if err != nil {
			return false, err
		}
		for _, id := range ids {
			if slices.Contains(recorded, id) {
				continue
			}
			ok, _, err := c.closes(ctx, id)
			if err != nil || ok {
				return false, err
			}
		}
		if len(ids) < c.d.Page {
			return true, nil
		}
		after = ids[len(ids)-1]
	}
}
