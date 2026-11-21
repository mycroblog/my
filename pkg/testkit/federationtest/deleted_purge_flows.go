// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"errors"
	"maps"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The flows of RetentionFlows on the purge of deleted statuses.

// deletedContentRows counts the rows of the content of a post, by table: the status, its local
// subtype, its head, and its revisions with their bodies, sources, attachments, mentions, tags,
// emoji, and polls.
func (e *retentionEnv) deletedContentRows(t *testing.T, p appcontent.Post) map[string]int {
	t.Helper()
	id, rev := p.Status.ID, p.Revision.ID
	rows := map[string]int{
		"statuses":         e.rh.Rows(t, "statuses", "id", id),
		"local_statuses":   e.rh.Rows(t, "local_statuses", "status_id", id),
		"status_heads":     e.rh.Rows(t, "status_heads", "status_id", id),
		"status_revisions": e.rh.Rows(t, "status_revisions", "status_id", id),
	}
	for _, table := range []string{
		"status_revision_bodies", "status_revision_sources", "status_revision_attachments",
		"status_revision_mentions", "status_revision_tags", "status_revision_emojis", "polls",
	} {
		rows[table] = e.rh.Rows(t, table, "revision_id", rev)
	}
	if p.Revision.Poll != nil {
		rows["poll_options"] = e.rh.Rows(t, "poll_options", "poll_id", p.Revision.Poll.ID)
	}
	return rows
}

// retentionPurgesDeletedStatuses checks the purge of deleted statuses through the use cases: local
// statuses that their author or a moderator deleted, and a remote one that its server deleted,
// keep their rows for DeletedContentRetention; then the job content.purge_deleted_statuses
// deletes every row of their content and keeps their deletions as tombstones, and a live status
// keeps its rows.
func retentionPurgesDeletedStatuses(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	author, reader := e.user(t, "author"), e.user(t, "reader")
	create := func(text string, poll *content.PollInput) appcontent.Post {
		t.Helper()
		p, err := e.content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: author, TextInput: appcontent.TextInput{Text: text}, Visibility: content.VisibilityPublic, Poll: poll,
		})
		if err != nil {
			t.Fatalf("Create(%q) = %v", text, err)
		}
		return p
	}
	byAuthor := create("deleted by its author @reader #purge", &content.PollInput{Options: []string{"yes", "no"}, Duration: time.Hour})
	byModeration := create("deleted by a moderator #purge", nil)
	live := create("live @reader #purge", nil)
	for name, p := range map[string]appcontent.Post{"by its author": byAuthor, "by a moderator": byModeration, "live": live} {
		rows := e.deletedContentRows(t, p)
		for _, table := range []string{"statuses", "local_statuses", "status_heads", "status_revisions", "status_revision_bodies", "status_revision_sources", "status_revision_tags"} {
			if rows[table] == 0 {
				t.Fatalf("the status %s has no row in %s after its creation", name, table)
			}
		}
	}
	if rows := e.deletedContentRows(t, byAuthor); rows["status_revision_mentions"] == 0 || rows["polls"] == 0 || rows["poll_options"] == 0 {
		t.Fatalf("rows of the status with a mention and a poll = %v, want its mention and poll", rows)
	}

	liveRows := e.deletedContentRows(t, live)

	const alice = "https://remote.example/users/alice"
	e.follow(t, reader, e.actor(t, alice))
	remote := e.note(t, alice, alice+"/notes/deleted", time.Hour, nil)
	remoteRev := e.post(t, remote.URI).Revision

	if _, err := e.content.Delete(ctx, author, byAuthor.Status.ID); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.content.DeleteByModeration(ctx, tx, byModeration.Status, e.IDs.NewID(), e.Clock.Now())
	}); err != nil {
		t.Fatalf("DeleteByModeration = %v", err)
	}
	e.expect(t, remoteActivity(t, "Delete", remote.URI+"#delete", alice, map[string]any{"id": remote.URI, "type": "Tombstone"}), appfederation.InboxHandled)

	reg := &queue.JobHandlerRegistry{}
	appcontent.NewDeletedPurge(appcontent.DeletedPurgeDeps{
		UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Deleted, Clock: e.Clock,
	}).RegisterJobs(reg, queue.NewScheduler(nil, e.Clock, nil))
	h, _, ok := reg.Lookup(appcontent.JobPurgeDeletedStatuses)
	if !ok {
		t.Fatalf("no handler of %s", appcontent.JobPurgeDeletedStatuses)
	}
	runJob := func() {
		t.Helper()
		if r := h(ctx, queue.Job{ID: e.IDs.NewID(), Type: appcontent.JobPurgeDeletedStatuses, Attempt: 1, CreatedAt: e.Clock.Now()}); !r.IsDone() {
			t.Fatalf("%s = %+v, want done", appcontent.JobPurgeDeletedStatuses, r)
		}
	}

	e.Clock.Advance(appcontent.DeletedContentRetention - time.Minute)
	runJob()
	for name, p := range map[string]appcontent.Post{"by its author": byAuthor, "by a moderator": byModeration} {
		if rows := e.deletedContentRows(t, p); rows["statuses"] != 1 || rows["status_revisions"] == 0 || rows["status_revision_bodies"] == 0 {
			t.Errorf("rows of the status deleted %s before the purge is due = %v, want them kept", name, rows)
		}
	}

	e.Clock.Advance(time.Minute)
	runJob()
	for name, p := range map[string]appcontent.Post{"by its author": byAuthor, "by a moderator": byModeration, "by its server": {Status: remote, Revision: remoteRev}} {
		for table, n := range e.deletedContentRows(t, p) {
			if n != 0 {
				t.Errorf("the status deleted %s has %d rows in %s after the purge, want none", name, n, table)
			}
		}
		if _, err := e.rh.Statuses.FindRevision(ctx, p.Revision.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindRevision(of the status deleted %s) = %v, want ErrNotFound", name, err)
		}
		if d, err := e.rh.Statuses.FindDeletion(ctx, p.Status.ID); err != nil || d.URI != p.Status.URI {
			t.Errorf("FindDeletion(the status deleted %s) = %+v, %v; want the tombstone of %s", name, d, err, p.Status.URI)
		}
		if n := e.rh.Rows(t, "status_content_purges", "status_id", p.Status.ID); n != 0 {
			t.Errorf("purges of the status deleted %s after the purge = %d, want none", name, n)
		}
	}
	if tomb, err := e.content.Tombstoned(ctx, byAuthor.Status.URI); err != nil || !tomb {
		t.Errorf("Tombstoned(the purged status) = %v, %v; want true", tomb, err)
	}
	if rows := e.deletedContentRows(t, live); !maps.Equal(rows, liveRows) {
		t.Errorf("rows of the live status after the purge = %v, want them kept: %v", rows, liveRows)
	}
	if _, err := e.content.Find(ctx, live.Status.ID); err != nil {
		t.Errorf("Find(the live status) = %v, want it kept", err)
	}
}
