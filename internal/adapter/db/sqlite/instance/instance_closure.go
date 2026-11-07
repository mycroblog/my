// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
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
		ID: e.ID, Kind: string(e.Kind), ChangedByUserID: sqlite.ToNullString(e.ByUserID), OccurredAt: sqlite.ToMillis(e.OccurredAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert service closure event")
	}
	if e.Kind != instance.ClosureEventAnnounced {
		return nil
	}
	err = q.InsertServiceClosureSchedule(ctx, instancegen.InsertServiceClosureScheduleParams{
		EventID: e.ID, ClosesAt: sqlite.ToMillis(e.Schedule.ClosesAt), ExportUntil: sqlite.ToMillis(e.Schedule.ExportUntil), Message: e.Schedule.Message,
	})
	return sqlite.Wrap(err, "insert service closure schedule")
}

// ClosureHead implements appinstance.ClosureRepository.
func (r *Repository) ClosureHead(ctx context.Context, tx uow.Tx) (instance.ClosureHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return instance.ClosureHead{}, err
	}
	row, err := q.GetServiceClosureHead(ctx)
	if err != nil {
		return instance.ClosureHead{}, sqlite.NotFound(err, appinstance.ErrNotFound, "service closure")
	}
	return instance.ClosureHead{State: instance.ClosureState(row.State), EventID: row.EventID, AnnouncementID: row.AnnouncementEventID}, nil
}

// UpsertClosureHead implements appinstance.ClosureRepository.
func (r *Repository) UpsertClosureHead(ctx context.Context, tx uow.Tx, h instance.ClosureHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertServiceClosureHead(ctx, instancegen.UpsertServiceClosureHeadParams{
		State: string(h.State), EventID: h.EventID, AnnouncementEventID: h.AnnouncementID,
	}), "upsert service closure head")
}

// DeleteClosureHead implements appinstance.ClosureRepository.
func (r *Repository) DeleteClosureHead(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteServiceClosureHead(ctx), "delete service closure head")
}

// ListClosureEvents implements appinstance.ClosureRepository.
func (r *Repository) ListClosureEvents(ctx context.Context, tx uow.Tx) ([]instance.ClosureEvent, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListServiceClosureEvents(ctx)
	if err != nil {
		return nil, sqlite.Wrap(err, "list service closure events")
	}
	out := make([]instance.ClosureEvent, len(rows))
	for i, row := range rows {
		out[i] = instance.ClosureEvent{
			ID: row.ID, Kind: instance.ClosureEventKind(row.Kind), ByUserID: row.ChangedByUserID.String, OccurredAt: sqlite.FromMillis(row.OccurredAt),
			Schedule: instance.ClosureSchedule{
				ClosesAt: sqlite.FromNullTime(row.ClosesAt), ExportUntil: sqlite.FromNullTime(row.ExportUntil), Message: row.Message.String,
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
		return instance.Closure{}, sqlite.NotFound(err, appinstance.ErrNotFound, "service closure")
	}
	return instance.Closure{
		State: instance.ClosureState(row.State), AnnouncementID: row.AnnouncementEventID,
		Schedule:    instance.ClosureSchedule{ClosesAt: sqlite.FromMillis(row.ClosesAt), ExportUntil: sqlite.FromMillis(row.ExportUntil), Message: row.Message},
		AnnouncedAt: sqlite.FromMillis(row.AnnouncedAt), ChangedAt: sqlite.FromMillis(row.ChangedAt),
	}, nil
}

// FindClosureEvent implements appinstance.ClosureRepository.
func (r *Repository) FindClosureEvent(ctx context.Context, id string) (instance.ClosureEvent, error) {
	row, err := r.db.reader().GetServiceClosureEvent(ctx, id)
	if err != nil {
		return instance.ClosureEvent{}, sqlite.NotFound(err, appinstance.ErrNotFound, "service closure event")
	}
	return instance.ClosureEvent{
		ID: row.ID, Kind: instance.ClosureEventKind(row.Kind), ByUserID: row.ChangedByUserID.String, OccurredAt: sqlite.FromMillis(row.OccurredAt),
		Schedule: instance.ClosureSchedule{
			ClosesAt: sqlite.FromNullTime(row.ClosesAt), ExportUntil: sqlite.FromNullTime(row.ExportUntil), Message: row.Message.String,
		},
	}, nil
}
