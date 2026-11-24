// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestRevisionReadsAreOneQuery checks that a revision is read with every part of it in one query,
// and the current revisions of many statuses with theirs in one query whatever their number.
func TestRevisionReadsAreOneQuery(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	repo := NewStatusRepository(db)
	ctx := t.Context()
	published := time.UnixMilli(1_700_000_000_000).UTC()
	var n uint32
	newID := func() string { n++; return testkit.OrderedID(n) }
	revs := make([]content.Revision, 3)
	statusIDs := make([]string, len(revs))
	err := db.Tx(ctx, func(tx uow.Tx) error {
		for i := range revs {
			s := content.Status{
				ID: newID(), AuthorAccountID: testkit.NamedID("alice"), IsLocal: false,
				Visibility: content.VisibilityPublic, PublishedAt: published,
			}
			s.URI = "https://remote.example/notes/" + s.ID
			statusIDs[i] = s.ID
			closes := published.Add(time.Hour)
			revs[i] = content.Revision{
				ID: newID(), StatusID: s.ID, Seq: 1, Language: "en", RevisedAt: published,
				Bodies:              []content.Body{{Language: "en", ContentHTML: "<p>hello</p>"}, {Language: "ja", ContentHTML: "<p>hi</p>"}},
				Source:              &content.Source{Text: "hello", Format: content.SourceTextPlain},
				Attachments:         []content.Attachment{{Position: 0, MediaID: newID(), Description: "a cat"}, {Position: 1, MediaID: newID()}},
				MentionedAccountIDs: []string{newID(), newID()},
				TagIDs:              []string{newID()},
				EmojiIDs:            []string{newID()},
				Poll: &content.Poll{ID: newID(), Multiple: true, ClosesAt: &closes,
					Options: []content.PollOption{{Position: 0, Title: "yes"}, {Position: 1, Title: "no"}}},
				Remote: &content.RemoteRevisionSource{ActivityURI: "https://remote.example/activities/" + s.ID, HTMLPolicyVersion: 1},
			}
			if err := repo.InsertStatus(ctx, tx, s); err != nil {
				return err
			}
			if err := repo.InsertRevision(ctx, tx, revs[i]); err != nil {
				return err
			}
			if err := repo.UpsertHead(ctx, tx, content.HeadOf(revs[i])); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create the statuses = %v", err)
	}

	if n := queriesOf(t, db.SetTracer, func(ctx context.Context) {
		if got, err := repo.FindRevision(ctx, revs[0].ID); err != nil || !reflect.DeepEqual(got, revs[0]) {
			t.Errorf("FindRevision = %+v, %v;\nwant %+v", got, err, revs[0])
		}
	}); n != 1 {
		t.Errorf("FindRevision made %d queries, want 1", n)
	}
	if n := queriesOf(t, db.SetTracer, func(ctx context.Context) {
		got, err := repo.FindCurrentRevisions(ctx, statusIDs)
		if err != nil || len(got) != len(revs) {
			t.Errorf("FindCurrentRevisions = %d revisions, %v; want %d", len(got), err, len(revs))
		}
	}); n != 1 {
		t.Errorf("FindCurrentRevisions of %d statuses made %d queries, want 1", len(revs), n)
	}
}

// queriesOf returns how many queries read runs, as the spans of a recording tracer count them.
func queriesOf(t *testing.T, setTracer func(kernel.Tracer), read func(ctx context.Context)) int {
	t.Helper()
	tr := testkit.NewTracer()
	setTracer(tr)
	defer setTracer(nil)
	ctx, parent := tr.Start(t.Context(), kernel.SpanSpec{Name: "read"}, kernel.Attrs{})
	read(ctx)
	parent.End("")
	n := 0
	for _, s := range tr.Spans() {
		if s.Attrs["db.operation.name"] != "" {
			n++
		}
	}
	return n
}
