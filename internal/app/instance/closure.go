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
