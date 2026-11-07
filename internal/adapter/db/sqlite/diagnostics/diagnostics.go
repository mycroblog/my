// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/diagnosticsgen"
	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

// Repository implements appdiagnostics.Repository (data-model §4.15, ADR-0103 §7).
type Repository struct {
	db *moduleDB
}

var _ appdiagnostics.Repository = (*Repository)(nil)

// NewRepository returns the diagnostics module's repository on this database.
func NewRepository(d *sqlite.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertGroup implements appdiagnostics.Repository.
func (r *Repository) InsertGroup(ctx context.Context, tx uow.Tx, g appdiagnostics.Group) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertErrorGroup(ctx, diagnosticsgen.InsertErrorGroupParams{
		ID: g.ID, Fingerprint: g.Fingerprint, Source: g.Source, Kind: g.Kind, ErrorType: g.ErrorType,
		Message: g.Message, WhereName: g.Where, FirstVersion: g.FirstVersion, Unauthenticated: sqlite.BoolToInt(g.Unauthenticated), FirstSeenAt: sqlite.ToMillis(g.FirstSeenAt),
	})
	return n > 0, sqlite.Wrap(err, "insert error group")
}

func currentFromRow(id, fingerprint, source, kind, errorType, message, where, firstVersion string, unauthenticated bool, firstSeenAt int64,
	state, resolvedVersion string, since int64, lastVersion string,
) appdiagnostics.Current {
	return appdiagnostics.Current{
		Group: appdiagnostics.Group{
			ID: id, Fingerprint: fingerprint, Source: source, Kind: kind, ErrorType: errorType, Message: message,
			Where: where, FirstVersion: firstVersion, Unauthenticated: unauthenticated, FirstSeenAt: sqlite.FromMillis(firstSeenAt),
		},
		Head: appdiagnostics.Head{
			GroupID: id, State: diagnostics.State(state), ResolvedVersion: resolvedVersion, Since: sqlite.FromMillis(since),
		},
		LastVersion: lastVersion,
	}
}

// CurrentByFingerprint implements appdiagnostics.Repository. SQLite locks the whole database
// for the transaction, which serializes the changes.
func (r *Repository) CurrentByFingerprint(ctx context.Context, tx uow.Tx, fingerprint string) (appdiagnostics.Current, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appdiagnostics.Current{}, err
	}
	row, err := q.GetErrorGroupCurrentByFingerprint(ctx, fingerprint)
	if err != nil {
		return appdiagnostics.Current{}, sqlite.NotFound(err, appdiagnostics.ErrNotFound, "error group "+fingerprint)
	}
	return currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, row.LastVersion), nil
}

// CurrentForUpdate implements appdiagnostics.Repository.
func (r *Repository) CurrentForUpdate(ctx context.Context, tx uow.Tx, groupID string) (appdiagnostics.Current, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appdiagnostics.Current{}, err
	}
	row, err := q.GetErrorGroupCurrent(ctx, groupID)
	if err != nil {
		return appdiagnostics.Current{}, sqlite.NotFound(err, appdiagnostics.ErrNotFound, "error group "+groupID)
	}
	return currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, row.LastVersion), nil
}

// InsertEvent implements appdiagnostics.Repository.
func (r *Repository) InsertEvent(ctx context.Context, tx uow.Tx, e appdiagnostics.Event) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertErrorGroupEvent(ctx, diagnosticsgen.InsertErrorGroupEventParams{
		ID: e.ID, GroupID: e.GroupID, Kind: string(e.Kind), ActorKind: string(e.Actor.Kind),
		ActorUserID: sqlite.ToNullString(e.Actor.UserID), Version: e.Version, OccurredAt: sqlite.ToMillis(e.OccurredAt),
	}), "insert error group event")
}

// InsertConfirmation implements appdiagnostics.Repository.
func (r *Repository) InsertConfirmation(ctx context.Context, tx uow.Tx, groupID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertErrorGroupConfirmation(ctx, diagnosticsgen.InsertErrorGroupConfirmationParams{GroupID: groupID, ConfirmedAt: sqlite.ToMillis(at)})
	return n > 0, sqlite.Wrap(err, "insert error group confirmation")
}

// UpsertHead implements appdiagnostics.Repository.
func (r *Repository) UpsertHead(ctx context.Context, tx uow.Tx, h appdiagnostics.Head) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertErrorGroupState(ctx, diagnosticsgen.UpsertErrorGroupStateParams{
		GroupID: h.GroupID, State: string(h.State), ResolvedVersion: h.ResolvedVersion, Since: sqlite.ToMillis(h.Since),
	}), "upsert error group state")
}

// AddCount implements appdiagnostics.Repository.
func (r *Repository) AddCount(ctx context.Context, c appdiagnostics.Count) error {
	return sqlite.Wrap(r.db.reader().AddErrorGroupCount(ctx, diagnosticsgen.AddErrorGroupCountParams{
		HourStart: sqlite.ToMillis(c.Hour), Occurrences: c.Occurrences, LastSeenAt: sqlite.ToMillis(c.LastSeenAt),
		LastVersion: c.LastVersion, Fingerprint: c.Fingerprint,
	}), "add error group count")
}

// PutSample implements appdiagnostics.Repository.
func (r *Repository) PutSample(ctx context.Context, s appdiagnostics.Sample) error {
	return sqlite.Wrap(r.db.reader().PutErrorGroupSample(ctx, diagnosticsgen.PutErrorGroupSampleParams{
		ErrorType: s.ErrorType, Message: s.Message, Stack: s.Stack, RequestID: s.RequestID, TraceID: s.TraceID,
		Version: s.Version, Role: s.Role, OccurredAt: sqlite.ToMillis(s.OccurredAt), Fingerprint: s.Fingerprint,
	}), "put error group sample")
}

// DeleteCountsBefore implements appdiagnostics.Repository.
func (r *Repository) DeleteCountsBefore(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.db.reader().DeleteErrorGroupCountsBefore(ctx, sqlite.ToMillis(before))
	return n, sqlite.Wrap(err, "delete error group counts")
}

// FindGroup implements appdiagnostics.Repository.
func (r *Repository) FindGroup(ctx context.Context, id string) (appdiagnostics.Group, appdiagnostics.Head, error) {
	row, err := r.db.reader().GetErrorGroup(ctx, id)
	if err != nil {
		return appdiagnostics.Group{}, appdiagnostics.Head{}, sqlite.NotFound(err, appdiagnostics.ErrNotFound, "error group "+id)
	}
	c := currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, "")
	return c.Group, c.Head, nil
}

// ListSummaries implements appdiagnostics.Repository.
func (r *Repository) ListSummaries(ctx context.Context, q appdiagnostics.SummaryQuery) ([]appdiagnostics.Summary, error) {
	rows, err := r.db.reader().ListErrorGroupSummaries(ctx, diagnosticsgen.ListErrorGroupSummariesParams{
		SinceDay: sqlite.ToMillis(q.Window.SinceDay), SinceWeek: sqlite.ToMillis(q.Window.SinceWeek), State: string(q.State), Source: q.Source,
		BeforeID: q.BeforeID, BeforeSeen: sqlite.ToMillis(q.BeforeSeen), RowLimit: int64(q.Limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list error group summaries")
	}
	out := make([]appdiagnostics.Summary, len(rows))
	for i, row := range rows {
		out[i] = summaryFromRow(diagnosticsgen.GetErrorGroupSummaryRow(row))
	}
	return out, nil
}

// FindSummary implements appdiagnostics.Repository.
func (r *Repository) FindSummary(ctx context.Context, tx uow.Tx, id string, w appdiagnostics.Window) (appdiagnostics.Summary, error) {
	db := r.db
	if tx != nil {
		var err error
		if db, err = r.db.in(tx); err != nil {
			return appdiagnostics.Summary{}, err
		}
	}
	q := db.reader()
	row, err := q.GetErrorGroupSummary(ctx, diagnosticsgen.GetErrorGroupSummaryParams{
		SinceDay: sqlite.ToMillis(w.SinceDay), SinceWeek: sqlite.ToMillis(w.SinceWeek), ID: id,
	})
	if err != nil {
		return appdiagnostics.Summary{}, sqlite.NotFound(err, appdiagnostics.ErrNotFound, "error group "+id)
	}
	return summaryFromRow(row), nil
}

func summaryFromRow(row diagnosticsgen.GetErrorGroupSummaryRow) appdiagnostics.Summary {
	c := currentFromRow(row.ID, row.Fingerprint, row.Source, row.Kind, row.ErrorType, row.Message, row.WhereName,
		row.FirstVersion, row.Unauthenticated != 0, row.FirstSeenAt, row.State, row.ResolvedVersion, row.Since, row.LastVersion)
	return appdiagnostics.Summary{
		Group: c.Group, Head: c.Head, LastSeenAt: sqlite.FromMillis(row.LastSeenAt), LastVersion: row.LastVersion,
		CountDay: row.CountDay, CountWeek: row.CountWeek,
	}
}

// FindSample implements appdiagnostics.Repository.
func (r *Repository) FindSample(ctx context.Context, groupID string) (appdiagnostics.Sample, error) {
	row, err := r.db.reader().GetErrorGroupSample(ctx, groupID)
	if err != nil {
		return appdiagnostics.Sample{}, sqlite.NotFound(err, appdiagnostics.ErrNotFound, "error group sample "+groupID)
	}
	return appdiagnostics.Sample{
		Fingerprint: row.Fingerprint, ErrorType: row.ErrorType, Message: row.Message, Stack: row.Stack,
		RequestID: row.RequestID, TraceID: row.TraceID, Version: row.Version, Role: row.Role, OccurredAt: sqlite.FromMillis(row.OccurredAt),
	}, nil
}

// HourCounts implements appdiagnostics.Repository.
func (r *Repository) HourCounts(ctx context.Context, groupID string) ([]appdiagnostics.HourCount, error) {
	rows, err := r.db.reader().ListErrorGroupCounts(ctx, groupID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list error group counts")
	}
	out := make([]appdiagnostics.HourCount, len(rows))
	for i, row := range rows {
		out[i] = appdiagnostics.HourCount{
			Hour: sqlite.FromMillis(row.HourStart), Occurrences: row.Occurrences, LastSeenAt: sqlite.FromMillis(row.LastSeenAt), LastVersion: row.LastVersion,
		}
	}
	return out, nil
}

// CountsSince implements appdiagnostics.Repository.
func (r *Repository) CountsSince(ctx context.Context, groupIDs []string, since time.Time) ([]appdiagnostics.GroupHourCount, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.reader().ListErrorGroupCountsSince(ctx, diagnosticsgen.ListErrorGroupCountsSinceParams{GroupIds: groupIDs, Since: sqlite.ToMillis(since)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list error group counts since")
	}
	out := make([]appdiagnostics.GroupHourCount, len(rows))
	for i, row := range rows {
		out[i] = appdiagnostics.GroupHourCount{GroupID: row.GroupID, Hour: sqlite.FromMillis(row.HourStart), Occurrences: row.Occurrences}
	}
	return out, nil
}

// Events implements appdiagnostics.Repository.
func (r *Repository) Events(ctx context.Context, groupID string) ([]appdiagnostics.Event, error) {
	rows, err := r.db.reader().ListErrorGroupEvents(ctx, groupID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list error group events")
	}
	out := make([]appdiagnostics.Event, len(rows))
	for i, row := range rows {
		out[i] = appdiagnostics.Event{
			ID: row.ID, GroupID: row.GroupID, Kind: diagnostics.EventKind(row.Kind),
			Actor:   appdiagnostics.Actor{Kind: diagnostics.ActorKind(row.ActorKind), UserID: row.ActorUserID.String},
			Version: row.Version, OccurredAt: sqlite.FromMillis(row.OccurredAt),
		}
	}
	return out, nil
}

// RebuildHeads implements appdiagnostics.Repository.
func (r *Repository) RebuildHeads(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteErrorGroupStates(ctx); err != nil {
		return sqlite.Wrap(err, "delete error group states")
	}
	return sqlite.Wrap(q.RebuildErrorGroupStates(ctx), "rebuild error group states")
}
