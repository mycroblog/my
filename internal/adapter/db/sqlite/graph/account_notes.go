// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// UpsertNote implements graph.Repository.
func (r *Repository) UpsertNote(ctx context.Context, tx uow.Tx, n graph.Note) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertAccountNote(ctx, graphgen.UpsertAccountNoteParams{
		AccountID: n.AccountID, TargetAccountID: n.TargetAccountID, Note: n.Text, UpdatedAt: sqlite.ToMillis(n.UpdatedAt),
	}), "upsert account note")
}

// DeleteNote implements graph.Repository.
func (r *Repository) DeleteNote(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.DeleteAccountNote(ctx, graphgen.DeleteAccountNoteParams{AccountID: accountID, TargetAccountID: targetAccountID})
	if err != nil {
		return false, sqlite.Wrap(err, "delete account note")
	}
	return rows == 1, nil
}

// FindNote implements graph.Repository.
func (r *Repository) FindNote(ctx context.Context, accountID, targetAccountID string) (graph.Note, error) {
	row, err := r.db.reader().FindAccountNote(ctx, graphgen.FindAccountNoteParams{AccountID: accountID, TargetAccountID: targetAccountID})
	if err != nil {
		return graph.Note{}, sqlite.NotFound(err, appgraph.ErrNotFound, "account note "+accountID+" -> "+targetAccountID)
	}
	return noteFrom(row), nil
}

// FindNotesOn implements graph.Repository.
func (r *Repository) FindNotesOn(ctx context.Context, accountID string, targetIDs []string) (map[string]graph.Note, error) {
	out := map[string]graph.Note{}
	if len(targetIDs) == 0 {
		return out, nil
	}
	targets, err := sqlite.JSONList(targetIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAccountNotesOn(ctx, graphgen.ListAccountNotesOnParams{AccountID: accountID, Targets: targets})
	if err != nil {
		return nil, sqlite.Wrap(err, "list account notes of "+accountID)
	}
	for _, row := range rows {
		out[row.TargetAccountID] = noteFrom(row)
	}
	return out, nil
}

// ListNotesBy implements graph.Repository.
func (r *Repository) ListNotesBy(ctx context.Context, accountID string) ([]graph.Note, error) {
	rows, err := r.db.reader().ListAccountNotesBy(ctx, accountID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account notes by "+accountID)
	}
	out := make([]graph.Note, len(rows))
	for i, row := range rows {
		out[i] = noteFrom(row)
	}
	return out, nil
}

func noteFrom(row graphgen.AccountNote) graph.Note {
	return graph.Note{AccountID: row.AccountID, TargetAccountID: row.TargetAccountID, Text: row.Note, UpdatedAt: sqlite.FromMillis(row.UpdatedAt)}
}
