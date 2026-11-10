// SPDX-License-Identifier: AGPL-3.0-only

package graphtest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// blockRepoNotes checks the private notes (FR-310): an upsert replaces the note of a pair, a note
// is found only for its owner and target, the notes of one owner on many targets come by target,
// the notes of an owner come oldest first, and a delete reports whether there was a note.
func blockRepoNotes(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(4)
	alice, bob, carol, dave := ids[0], ids[1], ids[2], ids[3]
	upsert := func(n graph.Note) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.UpsertNote(ctx, tx, n) })
	}
	first := graph.Note{AccountID: alice, TargetAccountID: bob, Text: "Met at the meetup", UpdatedAt: r.clock.Now()}
	upsert(first)
	r.clock.Advance(time.Second)
	onCarol := graph.Note{AccountID: alice, TargetAccountID: carol, Text: "カメラの話", UpdatedAt: r.clock.Now()}
	upsert(onCarol)
	r.clock.Advance(time.Second)
	onBob := graph.Note{AccountID: alice, TargetAccountID: bob, Text: "Met at the meetup; likes tea", UpdatedAt: r.clock.Now()}
	upsert(onBob)
	upsert(graph.Note{AccountID: bob, TargetAccountID: alice, Text: "Bob's own note", UpdatedAt: r.clock.Now()})

	if got, err := r.repo.FindNote(ctx, alice, bob); err != nil || !reflect.DeepEqual(got, onBob) {
		t.Errorf("FindNote(alice, bob) = %+v, %v; want the replaced note %+v", got, err, onBob)
	}
	if _, err := r.repo.FindNote(ctx, carol, bob); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindNote(carol, bob) = %v, want ErrNotFound", err)
	}
	got, err := r.repo.FindNotesOn(ctx, alice, []string{bob, carol, dave})
	if want := map[string]graph.Note{bob: onBob, carol: onCarol}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("FindNotesOn(alice) = %+v, %v; want %+v", got, err, want)
	}
	if got, err := r.repo.FindNotesOn(ctx, alice, nil); err != nil || len(got) != 0 {
		t.Errorf("FindNotesOn(alice, none) = %+v, %v; want none", got, err)
	}
	if list, err := r.repo.ListNotesBy(ctx, alice); err != nil || !reflect.DeepEqual(list, []graph.Note{onCarol, onBob}) {
		t.Errorf("ListNotesBy(alice) = %+v, %v; want the note on carol, then the one on bob", list, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if deleted, err := r.repo.DeleteNote(ctx, tx, alice, bob); err != nil || !deleted {
			t.Errorf("DeleteNote = %v, %v; want true", deleted, err)
		}
		if deleted, err := r.repo.DeleteNote(ctx, tx, alice, bob); err != nil || deleted {
			t.Errorf("DeleteNote(again) = %v, %v; want false", deleted, err)
		}
		return nil
	})
	if _, err := r.repo.FindNote(ctx, alice, bob); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindNote after DeleteNote = %v, want ErrNotFound", err)
	}
	if _, err := r.repo.FindNote(ctx, bob, alice); err != nil {
		t.Errorf("FindNote(bob, alice) after alice's delete = %v, want bob's note", err)
	}
}
