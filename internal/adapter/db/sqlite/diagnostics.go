// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

// DiagnosticsRepository implements appdiagnostics.Repository (data-model §4.15, ADR-0103 §7).
type DiagnosticsRepository struct {
	db *DB
}

var _ appdiagnostics.Repository = (*DiagnosticsRepository)(nil)

// DiagnosticsRepository returns the diagnostics module's repository on this database.
func (d *DB) DiagnosticsRepository() *DiagnosticsRepository { return &DiagnosticsRepository{db: d} }

// InsertGroup implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) InsertGroup(ctx context.Context, tx uow.Tx, g appdiagnostics.Group) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertErrorGroup(ctx, sqlitegen.InsertErrorGroupParams{
		ID: g.ID, Fingerprint: g.Fingerprint, Source: g.Source, Kind: g.Kind, ErrorType: g.ErrorType,
		Message: g.Message, WhereName: g.Where, FirstVersion: g.FirstVersion, Unauthenticated: boolToInt(g.Unauthenticated), FirstSeenAt: toMillis(g.FirstSeenAt),
	})
	return n > 0, wrap(err, "insert error group")
}

func currentFromRow(id, fingerprint, source, kind, errorType, message, where, firstVersion string, unauthenticated bool, firstSeenAt int64,
	state, resolvedVersion string, since int64, lastVersion string,
) appdiagnostics.Current {
	return appdiagnostics.Current{
		Group: appdiagnostics.Group{
			ID: id, Fingerprint: fingerprint, Source: source, Kind: kind, ErrorType: errorType, Message: message,
			Where: where, FirstVersion: firstVersion, Unauthenticated: unauthenticated, FirstSeenAt: fromMillis(firstSeenAt),
		},
		Head: appdiagnostics.Head{
			GroupID: id, State: diagnostics.State(state), ResolvedVersion: resolvedVersion, Since: fromMillis(since),
		},
		LastVersion: lastVersion,
	}
}

// CurrentByFingerprint implements appdiagnostics.Repository. SQLite locks the whole database
// for the transaction, which serializes the changes.
func (r *DiagnosticsRepository) CurrentByFingerprint(ctx context.Context, tx uow.Tx, fingerprint string) (appdiagnostics.Current, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appdiagnostics.Current{}, err
	}
	row, err := q.GetErrorGroupCurrentByFingerprint(ctx, fingerprint)
	if err != nil {
		return appdiagnostics.Current{}, notFound(err, appdiagnostics.ErrNotFound, "error group "+fingerprint)
	}
	return currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, row.LastVersion), nil
}

// CurrentForUpdate implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) CurrentForUpdate(ctx context.Context, tx uow.Tx, groupID string) (appdiagnostics.Current, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appdiagnostics.Current{}, err
	}
	row, err := q.GetErrorGroupCurrent(ctx, groupID)
	if err != nil {
		return appdiagnostics.Current{}, notFound(err, appdiagnostics.ErrNotFound, "error group "+groupID)
	}
	return currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, row.LastVersion), nil
}

// InsertEvent implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) InsertEvent(ctx context.Context, tx uow.Tx, e appdiagnostics.Event) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertErrorGroupEvent(ctx, sqlitegen.InsertErrorGroupEventParams{
		ID: e.ID, GroupID: e.GroupID, Kind: string(e.Kind), ActorKind: string(e.Actor.Kind),
		ActorUserID: toNullString(e.Actor.UserID), Version: e.Version, OccurredAt: toMillis(e.OccurredAt),
	}), "insert error group event")
}

// InsertConfirmation implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) InsertConfirmation(ctx context.Context, tx uow.Tx, groupID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertErrorGroupConfirmation(ctx, sqlitegen.InsertErrorGroupConfirmationParams{GroupID: groupID, ConfirmedAt: toMillis(at)})
	return n > 0, wrap(err, "insert error group confirmation")
}

// UpsertHead implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) UpsertHead(ctx context.Context, tx uow.Tx, h appdiagnostics.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertErrorGroupState(ctx, sqlitegen.UpsertErrorGroupStateParams{
		GroupID: h.GroupID, State: string(h.State), ResolvedVersion: h.ResolvedVersion, Since: toMillis(h.Since),
	}), "upsert error group state")
}

// AddCount implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) AddCount(ctx context.Context, c appdiagnostics.Count) error {
	return wrap(r.db.reader().AddErrorGroupCount(ctx, sqlitegen.AddErrorGroupCountParams{
		HourStart: toMillis(c.Hour), Occurrences: c.Occurrences, LastSeenAt: toMillis(c.LastSeenAt),
		LastVersion: c.LastVersion, Fingerprint: c.Fingerprint,
	}), "add error group count")
}

// PutSample implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) PutSample(ctx context.Context, s appdiagnostics.Sample) error {
	return wrap(r.db.reader().PutErrorGroupSample(ctx, sqlitegen.PutErrorGroupSampleParams{
		ErrorType: s.ErrorType, Message: s.Message, Stack: s.Stack, RequestID: s.RequestID, TraceID: s.TraceID,
		Version: s.Version, Role: s.Role, OccurredAt: toMillis(s.OccurredAt), Fingerprint: s.Fingerprint,
	}), "put error group sample")
}

// DeleteCountsBefore implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) DeleteCountsBefore(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.db.reader().DeleteErrorGroupCountsBefore(ctx, toMillis(before))
	return n, wrap(err, "delete error group counts")
}

// FindGroup implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) FindGroup(ctx context.Context, id string) (appdiagnostics.Group, appdiagnostics.Head, error) {
	row, err := r.db.reader().GetErrorGroup(ctx, id)
	if err != nil {
		return appdiagnostics.Group{}, appdiagnostics.Head{}, notFound(err, appdiagnostics.ErrNotFound, "error group "+id)
	}
	c := currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, "")
	return c.Group, c.Head, nil
}

// ListSummaries implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) ListSummaries(ctx context.Context, q appdiagnostics.SummaryQuery) ([]appdiagnostics.Summary, error) {
	rows, err := r.db.reader().ListErrorGroupSummaries(ctx, sqlitegen.ListErrorGroupSummariesParams{
		SinceDay: toMillis(q.Window.SinceDay), SinceWeek: toMillis(q.Window.SinceWeek), State: string(q.State), Source: q.Source,
		BeforeID: q.BeforeID, BeforeSeen: toMillis(q.BeforeSeen), RowLimit: int64(q.Limit),
	})
	if err != nil {
		return nil, wrap(err, "list error group summaries")
	}
	out := make([]appdiagnostics.Summary, len(rows))
	for i, row := range rows {
		out[i] = summaryFromRow(sqlitegen.GetErrorGroupSummaryRow(row))
	}
	return out, nil
}

// FindSummary implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) FindSummary(ctx context.Context, tx uow.Tx, id string, w appdiagnostics.Window) (appdiagnostics.Summary, error) {
	q := r.db.reader()
	if tx != nil {
		var err error
		if q, err = r.db.queries(tx); err != nil {
			return appdiagnostics.Summary{}, err
		}
	}
	row, err := q.GetErrorGroupSummary(ctx, sqlitegen.GetErrorGroupSummaryParams{
		SinceDay: toMillis(w.SinceDay), SinceWeek: toMillis(w.SinceWeek), ID: id,
	})
	if err != nil {
		return appdiagnostics.Summary{}, notFound(err, appdiagnostics.ErrNotFound, "error group "+id)
	}
	return summaryFromRow(row), nil
}

func summaryFromRow(row sqlitegen.GetErrorGroupSummaryRow) appdiagnostics.Summary {
	c := currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, row.LastVersion)
	return appdiagnostics.Summary{
		Group: c.Group, Head: c.Head, LastSeenAt: fromMillis(row.LastSeenAt), LastVersion: row.LastVersion,
		CountDay: row.CountDay, CountWeek: row.CountWeek,
	}
}

// FindSample implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) FindSample(ctx context.Context, groupID string) (appdiagnostics.Sample, error) {
	row, err := r.db.reader().GetErrorGroupSample(ctx, groupID)
	if err != nil {
		return appdiagnostics.Sample{}, notFound(err, appdiagnostics.ErrNotFound, "error group sample "+groupID)
	}
	return appdiagnostics.Sample{
		Fingerprint: row.Fingerprint, ErrorType: row.ErrorType, Message: row.Message, Stack: row.Stack,
		RequestID: row.RequestID, TraceID: row.TraceID, Version: row.Version, Role: row.Role, OccurredAt: fromMillis(row.OccurredAt),
	}, nil
}

// HourCounts implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) HourCounts(ctx context.Context, groupID string) ([]appdiagnostics.HourCount, error) {
	rows, err := r.db.reader().ListErrorGroupCounts(ctx, groupID)
	if err != nil {
		return nil, wrap(err, "list error group counts")
	}
	out := make([]appdiagnostics.HourCount, len(rows))
	for i, row := range rows {
		out[i] = appdiagnostics.HourCount{
			Hour: fromMillis(row.HourStart), Occurrences: row.Occurrences, LastSeenAt: fromMillis(row.LastSeenAt), LastVersion: row.LastVersion,
		}
	}
	return out, nil
}

// Events implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) Events(ctx context.Context, groupID string) ([]appdiagnostics.Event, error) {
	rows, err := r.db.reader().ListErrorGroupEvents(ctx, groupID)
	if err != nil {
		return nil, wrap(err, "list error group events")
	}
	out := make([]appdiagnostics.Event, len(rows))
	for i, row := range rows {
		out[i] = appdiagnostics.Event{
			ID: row.ID, GroupID: row.GroupID, Kind: diagnostics.EventKind(row.Kind),
			Actor:   appdiagnostics.Actor{Kind: diagnostics.ActorKind(row.ActorKind), UserID: row.ActorUserID.String},
			Version: row.Version, OccurredAt: fromMillis(row.OccurredAt),
		}
	}
	return out, nil
}

// RebuildHeads implements appdiagnostics.Repository.
func (r *DiagnosticsRepository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteErrorGroupStates(ctx); err != nil {
		return wrap(err, "delete error group states")
	}
	return wrap(q.RebuildErrorGroupStates(ctx), "rebuild error group states")
}
