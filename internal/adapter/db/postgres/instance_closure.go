// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertClosureEvent implements appinstance.ClosureRepository.
func (r *InstanceRepository) InsertClosureEvent(ctx context.Context, tx uow.Tx, e instance.ClosureEvent) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertServiceClosureEvent(ctx, postgresgen.InsertServiceClosureEventParams{
		ID: e.ID, Kind: string(e.Kind), ByUserID: toNullString(e.ByUserID), OccurredAt: e.OccurredAt,
	})
	if err != nil {
		return wrap(err, "insert service closure event")
	}
	if e.Kind != instance.ClosureEventAnnounced {
		return nil
	}
	err = q.InsertServiceClosureSchedule(ctx, postgresgen.InsertServiceClosureScheduleParams{
		EventID: e.ID, ClosesAt: e.Schedule.ClosesAt, ExportUntil: e.Schedule.ExportUntil, Message: e.Schedule.Message,
	})
	return wrap(err, "insert service closure schedule")
}

// ClosureHead implements appinstance.ClosureRepository.
func (r *InstanceRepository) ClosureHead(ctx context.Context, tx uow.Tx) (instance.ClosureHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ClosureHead{}, err
	}
	row, err := q.GetServiceClosureHead(ctx)
	if err != nil {
		return instance.ClosureHead{}, notFound(err, appinstance.ErrNotFound, "service closure")
	}
	return instance.ClosureHead{State: instance.ClosureState(row.State), EventID: row.EventID, AnnouncementID: row.AnnouncementEventID}, nil
}

// UpsertClosureHead implements appinstance.ClosureRepository.
func (r *InstanceRepository) UpsertClosureHead(ctx context.Context, tx uow.Tx, h instance.ClosureHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertServiceClosureHead(ctx, postgresgen.UpsertServiceClosureHeadParams{
		State: string(h.State), EventID: h.EventID, AnnouncementEventID: h.AnnouncementID,
	}), "upsert service closure head")
}

// DeleteClosureHead implements appinstance.ClosureRepository.
func (r *InstanceRepository) DeleteClosureHead(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteServiceClosureHead(ctx), "delete service closure head")
}

// ListClosureEvents implements appinstance.ClosureRepository.
func (r *InstanceRepository) ListClosureEvents(ctx context.Context, tx uow.Tx) ([]instance.ClosureEvent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListServiceClosureEvents(ctx)
	if err != nil {
		return nil, wrap(err, "list service closure events")
	}
	out := make([]instance.ClosureEvent, len(rows))
	for i, row := range rows {
		out[i] = instance.ClosureEvent{
			ID: row.ID, Kind: instance.ClosureEventKind(row.Kind), ByUserID: fromNullString(row.ByUserID), OccurredAt: utc(row.OccurredAt),
			Schedule: instance.ClosureSchedule{
				ClosesAt: fromNullTime(row.ClosesAt), ExportUntil: fromNullTime(row.ExportUntil), Message: fromNullString(row.Message),
			},
		}
	}
	return out, nil
}

// CurrentClosureTx implements appinstance.ClosureRepository.
func (r *InstanceRepository) CurrentClosureTx(ctx context.Context, tx uow.Tx) (instance.Closure, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.Closure{}, err
	}
	return currentClosure(ctx, q)
}

// CurrentClosure implements appinstance.ClosureRepository.
func (r *InstanceRepository) CurrentClosure(ctx context.Context) (instance.Closure, error) {
	return currentClosure(ctx, r.db.reader())
}

func currentClosure(ctx context.Context, q *postgresgen.Queries) (instance.Closure, error) {
	row, err := q.GetCurrentServiceClosure(ctx)
	if err != nil {
		return instance.Closure{}, notFound(err, appinstance.ErrNotFound, "service closure")
	}
	return instance.Closure{
		State: instance.ClosureState(row.State), AnnouncementID: row.AnnouncementEventID,
		Schedule:    instance.ClosureSchedule{ClosesAt: utc(row.ClosesAt), ExportUntil: utc(row.ExportUntil), Message: row.Message},
		AnnouncedAt: utc(row.AnnouncedAt), ChangedAt: utc(row.ChangedAt),
	}, nil
}

// FindClosureEvent implements appinstance.ClosureRepository.
func (r *InstanceRepository) FindClosureEvent(ctx context.Context, id string) (instance.ClosureEvent, error) {
	row, err := r.db.reader().GetServiceClosureEvent(ctx, id)
	if err != nil {
		return instance.ClosureEvent{}, notFound(err, appinstance.ErrNotFound, "service closure event")
	}
	return instance.ClosureEvent{
		ID: row.ID, Kind: instance.ClosureEventKind(row.Kind), ByUserID: fromNullString(row.ByUserID), OccurredAt: utc(row.OccurredAt),
		Schedule: instance.ClosureSchedule{
			ClosesAt: fromNullTime(row.ClosesAt), ExportUntil: fromNullTime(row.ExportUntil), Message: fromNullString(row.Message),
		},
	}, nil
}
