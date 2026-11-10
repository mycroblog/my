// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertClosureEvent implements appinstance.ClosureRepository.
func (r *Repository) InsertClosureEvent(ctx context.Context, tx uow.Tx, e instance.ClosureEvent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertServiceClosureEvent(ctx, instancegen.InsertServiceClosureEventParams{
		ID: e.ID, Kind: string(e.Kind), ChangedByUserID: postgres.ToNullString(e.ByUserID), OccurredAt: e.OccurredAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert service closure event")
	}
	if e.Kind != instance.ClosureEventAnnounced {
		return nil
	}
	err = q.InsertServiceClosureSchedule(ctx, instancegen.InsertServiceClosureScheduleParams{
		EventID: e.ID, ClosesAt: e.Schedule.ClosesAt, ExportUntil: e.Schedule.ExportUntil, Message: e.Schedule.Message,
	})
	return postgres.Wrap(err, "insert service closure schedule")
}

// ClosureHead implements appinstance.ClosureRepository.
func (r *Repository) ClosureHead(ctx context.Context, tx uow.Tx) (instance.ClosureHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ClosureHead{}, err
	}
	row, err := q.GetServiceClosureHead(ctx)
	if err != nil {
		return instance.ClosureHead{}, postgres.NotFound(err, appinstance.ErrNotFound, "service closure")
	}
	return instance.ClosureHead{State: instance.ClosureState(row.State), EventID: row.EventID, AnnouncementID: row.AnnouncementEventID}, nil
}

// UpsertClosureHead implements appinstance.ClosureRepository.
func (r *Repository) UpsertClosureHead(ctx context.Context, tx uow.Tx, h instance.ClosureHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertServiceClosureHead(ctx, instancegen.UpsertServiceClosureHeadParams{
		State: string(h.State), EventID: h.EventID, AnnouncementEventID: h.AnnouncementID,
	}), "upsert service closure head")
}

// DeleteClosureHead implements appinstance.ClosureRepository.
func (r *Repository) DeleteClosureHead(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteServiceClosureHead(ctx), "delete service closure head")
}

// ListClosureEvents implements appinstance.ClosureRepository.
func (r *Repository) ListClosureEvents(ctx context.Context, tx uow.Tx) ([]instance.ClosureEvent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListServiceClosureEvents(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list service closure events")
	}
	out := make([]instance.ClosureEvent, len(rows))
	for i, row := range rows {
		out[i] = instance.ClosureEvent{
			ID: row.ID, Kind: instance.ClosureEventKind(row.Kind), ByUserID: postgres.FromNullString(row.ChangedByUserID), OccurredAt: postgres.UTC(row.OccurredAt),
			Schedule: instance.ClosureSchedule{
				ClosesAt: postgres.FromNullTime(row.ClosesAt), ExportUntil: postgres.FromNullTime(row.ExportUntil), Message: postgres.FromNullString(row.Message),
			},
		}
	}
	return out, nil
}

// CurrentClosureTx implements appinstance.ClosureRepository.
func (r *Repository) CurrentClosureTx(ctx context.Context, tx uow.Tx) (instance.Closure, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.Closure{}, err
	}
	return currentClosure(ctx, q)
}

// CurrentClosure implements appinstance.ClosureRepository.
func (r *Repository) CurrentClosure(ctx context.Context) (instance.Closure, error) {
	return currentClosure(ctx, r.db.reader())
}

func currentClosure(ctx context.Context, q *instancegen.Queries) (instance.Closure, error) {
	row, err := q.GetCurrentServiceClosure(ctx)
	if err != nil {
		return instance.Closure{}, postgres.NotFound(err, appinstance.ErrNotFound, "service closure")
	}
	return instance.Closure{
		State: instance.ClosureState(row.State), AnnouncementID: row.AnnouncementEventID,
		Schedule:    instance.ClosureSchedule{ClosesAt: postgres.UTC(row.ClosesAt), ExportUntil: postgres.UTC(row.ExportUntil), Message: row.Message},
		AnnouncedAt: postgres.UTC(row.AnnouncedAt), ChangedAt: postgres.UTC(row.ChangedAt),
	}, nil
}

// FindClosureEvent implements appinstance.ClosureRepository.
func (r *Repository) FindClosureEvent(ctx context.Context, id string) (instance.ClosureEvent, error) {
	row, err := r.db.reader().GetServiceClosureEvent(ctx, id)
	if err != nil {
		return instance.ClosureEvent{}, postgres.NotFound(err, appinstance.ErrNotFound, "service closure event")
	}
	return closureEventFrom(row), nil
}

// closureEventFrom returns the event of row with its schedule.
func closureEventFrom(row instancegen.GetServiceClosureEventRow) instance.ClosureEvent {
	return instance.ClosureEvent{
		ID: row.ID, Kind: instance.ClosureEventKind(row.Kind), ByUserID: postgres.FromNullString(row.ChangedByUserID), OccurredAt: postgres.UTC(row.OccurredAt),
		Schedule: instance.ClosureSchedule{
			ClosesAt: postgres.FromNullTime(row.ClosesAt), ExportUntil: postgres.FromNullTime(row.ExportUntil), Message: postgres.FromNullString(row.Message),
		},
	}
}

// FederationGate implements appinstance.ClosureRepository.
func (r *Repository) FederationGate(ctx context.Context) (instance.Closure, bool, string, error) {
	row, err := r.db.reader().GetFederationGate(ctx)
	if err != nil {
		return instance.Closure{}, false, "", postgres.Wrap(err, "get the federation gate")
	}
	if row.State == nil || row.AnnouncementEventID == nil || row.ChangedAt == nil || row.AnnouncedAt == nil || row.ClosesAt == nil || row.ExportUntil == nil || row.Message == nil {
		return instance.Closure{}, false, row.FederationMode, nil
	}
	return instance.Closure{
		State: instance.ClosureState(*row.State), AnnouncementID: *row.AnnouncementEventID,
		Schedule:    instance.ClosureSchedule{ClosesAt: postgres.UTC(*row.ClosesAt), ExportUntil: postgres.UTC(*row.ExportUntil), Message: *row.Message},
		AnnouncedAt: postgres.UTC(*row.AnnouncedAt), ChangedAt: postgres.UTC(*row.ChangedAt),
	}, true, row.FederationMode, nil
}
