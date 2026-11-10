// SPDX-License-Identifier: AGPL-3.0-only

package graphtest

import (
	"errors"
	"strings"
	"testing"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/graph"
)

// noteKeepsPrivateNotes checks the private notes (FR-310): a note on a local or remote account
// shows in the writer's relationship and in no one else's, writing again replaces it, an empty
// note and DeleteNote remove it, and the data export holds the writer's notes only.
func noteKeepsPrivateNotes(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	carol := e.remoteUser(t, "carol")
	rel, err := e.graph.SetNote(ctx, alice, bob, "Met at the meetup")
	if err != nil || rel.Note != "Met at the meetup" {
		t.Fatalf("SetNote = %+v, %v; want the note", rel, err)
	}
	if _, err := e.graph.SetNote(ctx, alice, carol, "Remote friend"); err != nil {
		t.Fatalf("SetNote(remote) = %v", err)
	}
	if rel, err := e.graph.Relationship(ctx, bob, alice); err != nil || rel.Note != "" {
		t.Errorf("Relationship(bob, alice) = %+v, %v; want no note: bob did not write one", rel, err)
	}
	rels, err := e.graph.Relationships(ctx, alice, []string{bob, carol})
	if err != nil || rels[bob].Note != "Met at the meetup" || rels[carol].Note != "Remote friend" {
		t.Errorf("Relationships(alice) = %+v, %v; want both notes", rels, err)
	}
	with, err := e.graph.RelationshipsWith(ctx, bob, []string{alice})
	if err != nil || with[alice].Note != "" {
		t.Errorf("RelationshipsWith(bob) = %+v, %v; want no note", with, err)
	}
	e.Clock.Advance(time.Second)
	if rel, err := e.graph.SetNote(ctx, alice, bob, "Likes tea"); err != nil || rel.Note != "Likes tea" {
		t.Errorf("SetNote(again) = %+v, %v; want the new note", rel, err)
	}

	x, err := e.graph.ExportNotes(ctx, alice)
	if err != nil || len(x.Notes) != 2 || x.Notes[0].AccountID != carol || x.Notes[0].Note != "Remote friend" ||
		x.Notes[1].AccountID != bob || x.Notes[1].Note != "Likes tea" || x.Notes[0].AccountURI == "" {
		t.Errorf("ExportNotes(alice) = %+v, %v; want the note on carol, then the one on bob", x, err)
	}
	if x, err := e.graph.ExportNotes(ctx, bob); err != nil || len(x.Notes) != 0 {
		t.Errorf("ExportNotes(bob) = %+v, %v; want none", x, err)
	}

	if rel, err := e.graph.SetNote(ctx, alice, bob, ""); err != nil || rel.Note != "" {
		t.Errorf("SetNote(empty) = %+v, %v; want the note removed", rel, err)
	}
	if rel, err := e.graph.DeleteNote(ctx, alice, carol); err != nil || rel.Note != "" {
		t.Errorf("DeleteNote = %+v, %v; want the note removed", rel, err)
	}
	if _, err := e.graph.DeleteNote(ctx, alice, carol); err != nil {
		t.Errorf("DeleteNote(again) = %v, want nil", err)
	}
}

func noteRejectsInvalid(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	carol := e.remoteUser(t, "carol")
	if _, err := e.graph.SetNote(ctx, alice, alice, "me"); !errors.Is(err, graph.ErrSelfNote) {
		t.Errorf("SetNote(self) = %v, want ErrSelfNote", err)
	}
	if _, err := e.graph.SetNote(ctx, alice, bob, strings.Repeat("a", graph.MaxNoteLength+1)); !errors.Is(err, graph.ErrInvalidNote) {
		t.Errorf("SetNote(too long) = %v, want ErrInvalidNote", err)
	}
	if _, err := e.graph.SetNote(ctx, alice, bob, strings.Repeat("あ", graph.MaxNoteLength)); err != nil {
		t.Errorf("SetNote(longest) = %v", err)
	}
	missing := "00000000-0000-7000-8000-000000000000"
	if _, err := e.graph.SetNote(ctx, alice, missing, "x"); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("SetNote(missing) = %v, want ErrAccountUnavailable", err)
	}
	if _, err := e.graph.SetNote(ctx, carol, alice, "x"); !errors.Is(err, appgraph.ErrRemoteAccount) {
		t.Errorf("SetNote(from remote) = %v, want ErrRemoteAccount", err)
	}
}
