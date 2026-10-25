// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"errors"
	"fmt"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The service closure (FR-808, ADR-0088 §1): the announcement of the closure, the change of its
// dates, its withdrawal, and the state that every entry point reads per request and per job.

// ClosureRepository is the part of Repository about the service closure (data-model §4.9).
type ClosureRepository interface {
	// InsertClosureEvent inserts an event, with its schedule when it is an announcement.
	InsertClosureEvent(ctx context.Context, tx uow.Tx, e instance.ClosureEvent) error
	// ClosureHead returns the service_closure_heads row.
	ClosureHead(ctx context.Context, tx uow.Tx) (instance.ClosureHead, error)
	UpsertClosureHead(ctx context.Context, tx uow.Tx, h instance.ClosureHead) error
	DeleteClosureHead(ctx context.Context, tx uow.Tx) error
	// ListClosureEvents returns every event with its schedule, oldest first, for rebuilding.
	ListClosureEvents(ctx context.Context, tx uow.Tx) ([]instance.ClosureEvent, error)
	// CurrentClosureTx returns the closure that the head points at, in tx.
	CurrentClosureTx(ctx context.Context, tx uow.Tx) (instance.Closure, error)

	// CurrentClosure returns the closure that the head points at.
	CurrentClosure(ctx context.Context) (instance.Closure, error)
}

// EventClosureRecorded is the event type of ClosureRecorded.
const EventClosureRecorded = "instance.closure_recorded"

// ClosureRecorded is emitted when a service closure event is inserted.
type ClosureRecorded struct {
	Event instance.ClosureEvent
}

// EventType implements uow.Event.
func (ClosureRecorded) EventType() string { return EventClosureRecorded }

// AuditTargetServiceClosure is the target ID (and label) of the audit log entries of the
// service closure, whose target kind is the instance.
const AuditTargetServiceClosure = "service_closure"

func registerClosureProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventClosureRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		head, err := repo.ClosureHead(ctx, tx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		return saveClosureHead(ctx, tx, repo, head.Apply(e.(ClosureRecorded).Event))
	})
}

func saveClosureHead(ctx context.Context, tx uow.Tx, repo Repository, h instance.ClosureHead) error {
	if h == (instance.ClosureHead{}) {
		return repo.DeleteClosureHead(ctx, tx)
	}
	return repo.UpsertClosureHead(ctx, tx, h)
}

func rebuildClosureHead(ctx context.Context, tx uow.Tx, repo Repository) error {
	events, err := repo.ListClosureEvents(ctx, tx)
	if err != nil {
		return err
	}
	var head instance.ClosureHead
	for _, e := range events {
		head = head.Apply(e)
	}
	return saveClosureHead(ctx, tx, repo, head)
}

// Closure returns the current state of the service closure, the zero instance.Closure (none)
// when no closure is announced. It is read per request and per job, so that a change applies at
// once.
func (s *Service) Closure(ctx context.Context) (instance.Closure, error) {
	c, err := s.d.Repository.CurrentClosure(ctx)
	if errors.Is(err, ErrNotFound) {
		return instance.Closure{}, nil
	}
	return c, err
}

// AnnounceClosureInput announces the closure of the service or changes its dates.
type AnnounceClosureInput struct {
	ClosesAt time.Time
	// ExportUntil is until when the users may export their data; the zero time is ClosesAt
	// plus instance.DefaultClosureExportPeriod.
	ExportUntil time.Time
	// Message is the operator's message to the users, "" for none.
	Message string
	// ByUserID is the user making the change, or "" for the CLI.
	ByUserID string
}

// AnnounceClosure records an announcement of the closure with its schedule
// (instance.NewClosureSchedule), which also changes the dates of an announcement in effect, and
// audits it in the same transaction. It fails with an error wrapping instance.ErrInvalidValue
// for a bad schedule and instance.ErrClosureTransition after the closure. The result's
// ShortNotice tells a notice under instance.ClosureNoticeWarning, which the caller warns about.
func (s *Service) AnnounceClosure(ctx context.Context, in AnnounceClosureInput) (instance.Closure, error) {
	schedule, err := instance.NewClosureSchedule(in.ClosesAt, in.ExportUntil, in.Message)
	if err != nil {
		return instance.Closure{}, err
	}
	return s.recordClosure(ctx, instance.ClosureEvent{Kind: instance.ClosureEventAnnounced, Schedule: schedule, ByUserID: in.ByUserID})
}

// WithdrawClosure withdraws the announcement of the closure, by byUserID ("" for the CLI), and
// audits it in the same transaction. It fails with an error wrapping
// instance.ErrClosureTransition when no closure is announced or the server is closed.
func (s *Service) WithdrawClosure(ctx context.Context, byUserID string) (instance.Closure, error) {
	return s.recordClosure(ctx, instance.ClosureEvent{Kind: instance.ClosureEventWithdrawn, ByUserID: byUserID})
}

// recordClosure checks e against the current state in a transaction, records it with its audit
// entry, and returns the state after it.
func (s *Service) recordClosure(ctx context.Context, e instance.ClosureEvent) (instance.Closure, error) {
	var action appmoderation.AuditAction
	switch e.Kind {
	case instance.ClosureEventAnnounced:
		action = appmoderation.AuditServiceClosureAnnounced
	case instance.ClosureEventWithdrawn:
		action = appmoderation.AuditServiceClosureWithdrawn
	case instance.ClosureEventClosed:
		action = appmoderation.AuditServiceClosureClosed
	case instance.ClosureEventDeliveryCompleted:
		action = appmoderation.AuditServiceClosureDeliveryCompleted
	default:
		return instance.Closure{}, fmt.Errorf("record service closure: unsupported event %s", e.Kind)
	}
	var out instance.Closure
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.d.Repository.CurrentClosureTx(ctx, tx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		ev := e
		ev.ID, ev.OccurredAt = s.d.IDs.NewID(), s.d.Clock.Now()
		if err := cur.Check(ev.Kind, ev.OccurredAt); err != nil {
			return err
		}
		if err := s.d.Repository.InsertClosureEvent(ctx, tx, ev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, ClosureRecorded{Event: ev}); err != nil {
			return err
		}
		if err := tx.Emit(ctx, s.audit(action, appmoderation.AuditTargetInstance, AuditTargetServiceClosure,
			AuditTargetServiceClosure, ev.ID, ev.ByUserID, ev.OccurredAt)); err != nil {
			return err
		}
		out, err = s.d.Repository.CurrentClosureTx(ctx, tx)
		if errors.Is(err, ErrNotFound) {
			out, err = instance.Closure{}, nil
		}
		return err
	})
	if err != nil {
		return instance.Closure{}, fmt.Errorf("record service closure %s: %w", e.Kind, err)
	}
	return out, nil
}

// Closes reports whether the event closes the server.
func (e ClosureRecorded) Closes() bool { return e.Event.Kind == instance.ClosureEventClosed }

// CloseService closes the server for good (FR-809, ADR-0088 §1), by byUserID ("" for the CLI),
// and audits it in the same transaction. The modules that the closure ends subscribe to
// ClosureRecorded in the same transaction: the supporters module cancels every live
// subscription. It fails with an error wrapping instance.ErrClosureNotDue before closes_at and
// instance.ErrClosureTransition without an announcement or once closed. The callers ask for
// the confirmation (the domain name) before calling it.
func (s *Service) CloseService(ctx context.Context, byUserID string) (instance.Closure, error) {
	return s.recordClosure(ctx, instance.ClosureEvent{Kind: instance.ClosureEventClosed, ByUserID: byUserID})
}

// Jobs of the closure of the service (ADR-0088 §3). The federation module runs them: it records
// and fans out the closure's Delete of every local actor, and tells when their deliveries end.
const (
	// JobClosureFanOut records the Delete of one page of local accounts and passes them to the
	// fan-out; the federation module enqueues the first page in the transaction of the closure.
	JobClosureFanOut = "instance.closure_fanout"
	// JobClosureCompletion is the periodic check that records delivery_completed once the
	// closure's Deletes have all been delivered.
	JobClosureCompletion = "instance.closure_completion"
)

// CompleteClosureDelivery records that the Delete of every local account has been delivered
// (ADR-0088 §3), as the server's own change, and audits it in the same transaction. The periodic
// check of the federation module calls it. It fails with an error wrapping
// instance.ErrClosureTransition unless the server is closed.
func (s *Service) CompleteClosureDelivery(ctx context.Context) (instance.Closure, error) {
	return s.recordClosure(ctx, instance.ClosureEvent{Kind: instance.ClosureEventDeliveryCompleted})
}

// ClosureProgress is how far the closure's Deletes have gone (ADR-0088 §3): the local accounts
// whose Delete is recorded, those Deletes whose fan-out has not ended, and their deliveries by
// state (pending, succeeded, permanent_failure, skipped, dead).
type ClosureProgress struct {
	Accounts   int
	FanOuts    int
	Deliveries map[string]int
}

// ClosureDeliveries reads the progress of the closure's Deletes; the composition root implements
// it over the federation module.
type ClosureDeliveries interface {
	ClosureProgress(ctx context.Context) (ClosureProgress, error)
}

// ClosureProgress returns how far the closure's Deletes have gone, and false while the server is
// not closed or nothing reads the progress.
func (s *Service) ClosureProgress(ctx context.Context) (ClosureProgress, bool, error) {
	c, err := s.Closure(ctx)
	if err != nil {
		return ClosureProgress{}, false, err
	}
	if st := c.CurrentState(); s.d.ClosureDeliveries == nil || (st != instance.ClosureClosed && st != instance.ClosureDeliveryCompleted) {
		return ClosureProgress{}, false, nil
	}
	p, err := s.d.ClosureDeliveries.ClosureProgress(ctx)
	if err != nil {
		return ClosureProgress{}, false, fmt.Errorf("service closure progress: %w", err)
	}
	return p, true, nil
}

// ClosureCounts counts what the closure of the service ends, for its checks (ADR-0088 §1). The
// composition root implements it over the account, supporters, and moderation modules.
type ClosureCounts interface {
	// CountLocalUsers counts the local accounts of users that are not deleted.
	CountLocalUsers(ctx context.Context) (int, error)
	// CountKnownSharedInboxes counts the shared inboxes that the Delete of a local account
	// reaches.
	CountKnownSharedInboxes(ctx context.Context) (int, error)
	// LiveSubscriptionCount counts the supporter subscriptions that go on.
	LiveSubscriptionCount(ctx context.Context) (int, error)
	// PendingMigrations counts the account migrations (ADR-0089: the sending of a Move and the
	// jobs that move the followers) that have not ended.
	PendingMigrations(ctx context.Context) (int, error)
	// HeldAccounts counts the accounts that a retention hold (ADR-0038) preserves.
	HeldAccounts(ctx context.Context) (int, error)
}

// ClosureCheck is what the operator reviews before closing the server (ADR-0088 §1, §4, §5).
type ClosureCheck struct {
	// LocalAccounts is the number of local accounts that the closure sends a Delete of, and
	// DeliveryInboxes the number of inboxes that each Delete goes to; EstimatedDeletes is their
	// product, the estimate of the deliveries.
	LocalAccounts, DeliveryInboxes, EstimatedDeletes int
	// LiveSupporterSubscriptions is the number of supporter subscriptions that the closure
	// cancels at once.
	LiveSupporterSubscriptions int
	// PendingMigrations is the number of account migrations that have not ended; they must end
	// before the closure's deliveries (ADR-0089).
	PendingMigrations int
	// RetentionHolds is the number of accounts under a retention hold, whose records the
	// operator exports before discarding the data (ADR-0088 §5).
	RetentionHolds int
}

// CheckClosure returns the checks before the closure. Without ClosureCounts every count is 0.
func (s *Service) CheckClosure(ctx context.Context) (ClosureCheck, error) {
	var out ClosureCheck
	c := s.d.ClosureCounts
	if c == nil {
		return out, nil
	}
	for _, f := range []struct {
		name  string
		count func(context.Context) (int, error)
		into  *int
	}{
		{"local accounts", c.CountLocalUsers, &out.LocalAccounts},
		{"delivery inboxes", c.CountKnownSharedInboxes, &out.DeliveryInboxes},
		{"live supporter subscriptions", c.LiveSubscriptionCount, &out.LiveSupporterSubscriptions},
		{"pending migrations", c.PendingMigrations, &out.PendingMigrations},
		{"retention holds", c.HeldAccounts, &out.RetentionHolds},
	} {
		n, err := f.count(ctx)
		if err != nil {
			return ClosureCheck{}, fmt.Errorf("check the service closure: %s: %w", f.name, err)
		}
		*f.into = n
	}
	out.EstimatedDeletes = out.LocalAccounts * out.DeliveryInboxes
	return out, nil
}

// ClosureAllows reports whether the current state of the service closure runs op
// (instance.ClosureAllows), read per call so that a change applies at once.
func (s *Service) ClosureAllows(ctx context.Context, op instance.ClosureOperation) (bool, error) {
	c, err := s.Closure(ctx)
	if err != nil {
		return false, err
	}
	return instance.ClosureAllows(c.CurrentState(), op), nil
}

// PushStopped reports whether the service closure stops Web Push (instance.ClosureStream).
func (s *Service) PushStopped(ctx context.Context) (bool, error) {
	ok, err := s.ClosureAllows(ctx, instance.ClosureStream)
	return !ok, err
}
