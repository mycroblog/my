// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// The cases of RetentionContract on the purge of deleted statuses (status_content_purges): a
// deletion queues the purge of its status's rows DeletedContentRetention later, except a deletion
// by retention or by the erasure of an account; the due purges are listed earliest first, moved,
// and deleted; content.purge_deleted_statuses on the adapter deletes the content rows of the due
// statuses and keeps their deletions, the other statuses, and the statuses that the moderation
// records keep, whose purges come due again DeletedContentRecheck later; a run is bounded and
// ends, and a job that used up its batches enqueues its continuation; and the purges of
// retention and of the erasure of an account delete the purges of the statuses they remove.

// contentPost inserts a local status of author, published now, with a revision that has every
// part of a status's content (bodies, source, attachments, mentions, tags, emoji, and a poll), its
// local subtype, and its head.
func (r *retentionRepo) contentPost(t *testing.T, author string) (content.Status, content.Revision) {
	t.Helper()
	id := r.ids.NewID()
	s := content.Status{
		ID: id, URI: "https://" + testkit.ContractHost + "/statuses/" + id, AuthorAccountID: author, IsLocal: true,
		Visibility: content.VisibilityPublic, PublishedAt: r.clock.Now(),
	}
	rev := content.Revision{
		ID: r.ids.NewID(), StatusID: id, Seq: 1, Language: "en", RevisedAt: s.PublishedAt,
		Bodies:              []content.Body{{Language: "en", ContentHTML: "<p>hello</p>"}, {Language: "ja", ContentHTML: "<p>こんにちは</p>"}},
		Source:              &content.Source{Text: "hello #purge", Format: content.SourceTextPlain},
		Attachments:         []content.Attachment{{Position: 0, MediaID: r.ids.NewID(), Description: "a cat"}},
		MentionedAccountIDs: []string{r.ids.NewID()},
		TagIDs:              []string{r.ids.NewID()},
		EmojiIDs:            []string{r.ids.NewID()},
		Poll:                &content.Poll{ID: r.ids.NewID(), Options: []content.PollOption{{Position: 0, Title: "yes"}, {Position: 1, Title: "no"}}},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Statuses.InsertStatus(ctx, tx, s),
			r.h.Statuses.InsertLocalStatus(ctx, tx, content.LocalStatus{StatusID: id, AuthorAccountID: author}),
			r.h.Statuses.InsertRevision(ctx, tx, rev),
			r.h.Statuses.UpsertHead(ctx, tx, content.Head{StatusID: id, RevisionID: rev.ID, Seq: 1, LastRevisedAt: rev.RevisedAt}),
		)
	})
	return s, rev
}

// contentRows counts the rows of the content of a status and its revision, by table. The head is
// left out: it is a projection, which the deletion's projectors remove (RetentionFlows checks it
// through the use cases), and contentPost writes it without them.
func (r *retentionRepo) contentRows(t *testing.T, s content.Status, rev content.Revision) map[string]int {
	t.Helper()
	rows := map[string]int{
		"statuses":         r.h.Rows(t, "statuses", "id", s.ID),
		"local_statuses":   r.h.Rows(t, "local_statuses", "status_id", s.ID),
		"status_revisions": r.h.Rows(t, "status_revisions", "status_id", s.ID),
	}
	for _, table := range []string{
		"status_revision_bodies", "status_revision_sources", "status_revision_attachments",
		"status_revision_mentions", "status_revision_tags", "status_revision_emojis", "polls",
	} {
		rows[table] = r.h.Rows(t, table, "revision_id", rev.ID)
	}
	rows["poll_options"] = r.h.Rows(t, "poll_options", "poll_id", rev.Poll.ID)
	return rows
}

// requireContent fails unless every table of the content of s has a row (the fixture is whole).
func (r *retentionRepo) requireContent(t *testing.T, s content.Status, rev content.Revision) {
	t.Helper()
	for table, n := range r.contentRows(t, s, rev) {
		if n == 0 {
			t.Fatalf("%s has no row of the status %s before the purge", table, s.ID)
		}
	}
}

// wantContent fails unless every table of the content of s has a row (kept) or none (purged).
func (r *retentionRepo) wantContent(t *testing.T, what string, s content.Status, rev content.Revision, kept bool) {
	t.Helper()
	for table, n := range r.contentRows(t, s, rev) {
		if kept && n == 0 {
			t.Errorf("%s: %s has no row of the status, want its rows kept", what, table)
		}
		if !kept && n != 0 {
			t.Errorf("%s: %s has %d rows of the status, want none", what, table, n)
		}
	}
}

// purgeQueued is the number of purges of the status statusID (0 or 1).
func (r *retentionRepo) purgeQueued(t *testing.T, statusID string) int {
	t.Helper()
	return r.h.Rows(t, "status_content_purges", "status_id", statusID)
}

// due returns the IDs of the purges ListDueContentPurges lists due at before, up to limit.
func (r *retentionRepo) due(t *testing.T, before time.Time, limit int) []appcontent.DueContentPurge {
	t.Helper()
	var got []appcontent.DueContentPurge
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		got, err = r.h.Deleted.ListDueContentPurges(ctx, tx, before, limit)
		return err
	})
	return got
}

func dueIDs(purges []appcontent.DueContentPurge) []string {
	ids := make([]string, len(purges))
	for i, p := range purges {
		ids[i] = p.StatusID
	}
	return ids
}

// deletedPurge returns the purge of deleted statuses on the harness, with evidence (nil: none),
// jobs (nil: none), and batch (0: the default).
func (r *retentionRepo) deletedPurge(evidence appcontent.RetainedStatuses, jobs queue.JobQueue, batch int) *appcontent.DeletedPurge {
	return appcontent.NewDeletedPurge(appcontent.DeletedPurgeDeps{
		UnitOfWork: r.h.UnitOfWork, Repository: r.h.Deleted, Evidence: evidence, Jobs: jobs, Clock: r.clock, Batch: batch,
	})
}

// run runs p once and fails on an error.
func run(t *testing.T, p *appcontent.DeletedPurge) (more bool, purged int) {
	t.Helper()
	more, purged, err := p.Run(t.Context())
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	return more, purged
}

// runToEnd runs p until it says no more may be due and returns how many statuses it purged.
func runToEnd(t *testing.T, p *appcontent.DeletedPurge) int {
	t.Helper()
	total := 0
	for range 100 {
		more, n := run(t, p)
		total += n
		if !more {
			return total
		}
	}
	t.Fatal("the purge of deleted statuses did not end")
	return total
}

// retentionRepoQueuesContentPurges checks that a deletion queues the purge of its status's rows
// DeletedContentRetention later, as ContentPurgeDue says, for every origin but retention and the
// erasure of an account, singly and in bulk.
func retentionRepoQueuesContentPurges(t *testing.T, r *retentionRepo) {
	deletedAt := r.clock.Now()
	wantDue := deletedAt.Add(appcontent.DeletedContentRetention)
	queued := map[string]content.Status{}
	for _, tc := range []struct {
		origin content.DeletionOrigin
		queued bool
	}{
		{content.DeletedByAuthor, true},
		{content.DeletedByModeration, true},
		{content.DeletedByRemote, true},
		{content.DeletedByAutoDelete, true},
		{content.DeletedByRetention, false},
		{content.DeletedByAccountDeletion, false},
	} {
		s := r.status(t, tc.origin != content.DeletedByRemote, time.Hour, "")
		d := r.deleteStatus(t, s, tc.origin)
		due, ok := appcontent.ContentPurgeDue(d)
		if ok != tc.queued || (ok && !due.Equal(wantDue)) {
			t.Errorf("ContentPurgeDue(%s) = %v, %v; want %v, %v", tc.origin, due, ok, wantDue, tc.queued)
		}
		want := 0
		if tc.queued {
			want = 1
			queued[s.ID] = s
		}
		if got := r.purgeQueued(t, s.ID); got != want {
			t.Errorf("purges of a status deleted by %s = %d, want %d", tc.origin, got, want)
		}
	}
	// The bulk deletion of the moderation (delete_statuses) queues one purge per status.
	bulk := []content.Status{r.status(t, true, time.Hour, ""), r.status(t, false, time.Hour, "")}
	actionID := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		ds := make([]content.Deletion, len(bulk))
		for i, s := range bulk {
			ds[i] = content.Deletion{
				StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByModeration,
				ModerationActionID: actionID, DeletedAt: deletedAt,
			}
		}
		return r.h.Statuses.InsertDeletions(ctx, tx, ds)
	})
	for _, s := range bulk {
		if got := r.purgeQueued(t, s.ID); got != 1 {
			t.Errorf("purges of a status of a bulk deletion = %d, want 1", got)
		}
		queued[s.ID] = s
	}

	if got := r.due(t, wantDue.Add(-time.Second), 100); len(got) != 0 {
		t.Errorf("ListDueContentPurges before the retention ends = %+v, want none", got)
	}
	got := r.due(t, wantDue, 100)
	if len(got) != len(queued) {
		t.Errorf("ListDueContentPurges once the retention ends = %+v, want the %d queued", got, len(queued))
	}
	for _, p := range got {
		s, ok := queued[p.StatusID]
		if !ok || p.AuthorAccountID != s.AuthorAccountID || !p.Present {
			t.Errorf("due purge %+v, want one of a queued status, with its author, present", p)
		}
	}
}

// retentionRepoDueContentPurges checks ListDueContentPurges, PostponeContentPurges, and
// DeleteContentPurges: the due purges come earliest first and up to the limit, a postponed one
// comes due at its new time, a deleted one never again, and the purge of a status without its row
// says so.
func retentionRepoDueContentPurges(t *testing.T, r *retentionRepo) {
	start := r.clock.Now()
	var statuses []content.Status
	for range 3 {
		s := r.status(t, true, time.Hour, "")
		r.deleteStatus(t, s, content.DeletedByAuthor)
		statuses = append(statuses, s)
		r.clock.Advance(time.Hour)
	}
	a, b, c := statuses[0].ID, statuses[1].ID, statuses[2].ID
	allDue := start.Add(appcontent.DeletedContentRetention + 2*time.Hour)
	if got, want := dueIDs(r.due(t, allDue, 10)), []string{a, b, c}; !slices.Equal(got, want) {
		t.Errorf("ListDueContentPurges = %v, want the earliest first %v", got, want)
	}
	if got, want := dueIDs(r.due(t, allDue, 2)), []string{a, b}; !slices.Equal(got, want) {
		t.Errorf("ListDueContentPurges(limit 2) = %v, want %v", got, want)
	}
	if got, want := dueIDs(r.due(t, allDue.Add(-time.Hour), 10)), []string{a, b}; !slices.Equal(got, want) {
		t.Errorf("ListDueContentPurges an hour before the last is due = %v, want %v", got, want)
	}

	later := allDue.Add(30 * 24 * time.Hour)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Deleted.PostponeContentPurges(ctx, tx, []string{a}, later)
	})
	if got, want := dueIDs(r.due(t, allDue, 10)), []string{b, c}; !slices.Equal(got, want) {
		t.Errorf("ListDueContentPurges after postponing the first = %v, want %v", got, want)
	}
	if got, want := dueIDs(r.due(t, later, 10)), []string{b, c, a}; !slices.Equal(got, want) {
		t.Errorf("ListDueContentPurges at the postponed time = %v, want %v", got, want)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Deleted.DeleteContentPurges(ctx, tx, []string{b})
	})
	if got, want := dueIDs(r.due(t, later, 10)), []string{c, a}; !slices.Equal(got, want) {
		t.Errorf("ListDueContentPurges after deleting the second = %v, want %v", got, want)
	}
	if _, err := r.h.Statuses.FindDeletion(t.Context(), b); err != nil {
		t.Errorf("FindDeletion(a status whose purge is deleted) = %v, want the deletion kept", err)
	}
	if _, err := r.h.Statuses.FindStatus(t.Context(), b); err != nil {
		t.Errorf("FindStatus(a status whose purge is deleted) = %v, want its rows kept", err)
	}

	// A deletion of a status whose row is gone is listed as not present.
	absent := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.InsertDeletion(ctx, tx, content.Deletion{
			StatusID: absent, URI: "https://" + testkit.ContractHost + "/statuses/" + absent, AuthorAccountID: r.ids.NewID(),
			Origin: content.DeletedByAuthor, DeletedAt: r.clock.Now(),
		})
	})
	got := r.due(t, later, 10)
	i := slices.IndexFunc(got, func(p appcontent.DueContentPurge) bool { return p.StatusID == absent })
	if i < 0 || got[i].Present {
		t.Errorf("ListDueContentPurges = %+v, want the purge of %s listed as not present", got, absent)
	}
	for _, p := range got {
		if p.StatusID != absent && !p.Present {
			t.Errorf("due purge %+v, want present", p)
		}
	}
}

// retentionRepoPurgesDeletedContent checks the purge of deleted statuses on the adapter: a local
// status that its author deleted keeps its rows until DeletedContentRetention has passed, and then
// loses every row of its content while its deletion stays; another status keeps its rows; and the
// purge is done once.
func retentionRepoPurgesDeletedContent(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	author := r.ids.NewID()
	deleted, deletedRev := r.contentPost(t, author)
	live, liveRev := r.contentPost(t, author)
	r.requireContent(t, deleted, deletedRev)
	r.requireContent(t, live, liveRev)
	deletion := r.deleteStatus(t, deleted, content.DeletedByAuthor)
	p := r.deletedPurge(nil, nil, 0)

	r.clock.Advance(appcontent.DeletedContentRetention - time.Minute)
	if more, n := run(t, p); more || n != 0 {
		t.Errorf("Run before the purge is due = %v, %d; want false, 0", more, n)
	}
	r.wantContent(t, "before the purge is due", deleted, deletedRev, true)

	r.clock.Advance(time.Minute)
	if more, n := run(t, p); more || n != 1 {
		t.Errorf("Run once the purge is due = %v, %d; want false, 1", more, n)
	}
	r.wantContent(t, "the deleted status", deleted, deletedRev, false)
	r.wantContent(t, "another status", live, liveRev, true)
	if got, err := r.h.Statuses.FindDeletion(ctx, deleted.ID); err != nil || got.Origin != deletion.Origin || got.URI != deletion.URI {
		t.Errorf("FindDeletion(purged) = %+v, %v; want the deletion %+v kept as the tombstone", got, err, deletion)
	}
	if n := r.purgeQueued(t, deleted.ID); n != 0 {
		t.Errorf("purges of the purged status = %d, want none", n)
	}
	if _, err := r.h.Statuses.FindRevision(ctx, deletedRev.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindRevision(of the purged status) = %v, want ErrNotFound", err)
	}
	if more, n := run(t, p); more || n != 0 {
		t.Errorf("Run after a complete run = %v, %d; want false, 0", more, n)
	}

	// The purge of a status whose rows are already gone is deleted.
	absent := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.InsertDeletion(ctx, tx, content.Deletion{
			StatusID: absent, URI: "https://" + testkit.ContractHost + "/statuses/" + absent, AuthorAccountID: author,
			Origin: content.DeletedByAuthor, DeletedAt: r.clock.Now(),
		})
	})
	r.clock.Advance(appcontent.DeletedContentRetention)
	runToEnd(t, p)
	if n := r.purgeQueued(t, absent); n != 0 {
		t.Errorf("purges of a status without its rows after a run = %d, want none", n)
	}
	if _, err := r.h.Statuses.FindDeletion(ctx, absent); err != nil {
		t.Errorf("FindDeletion(a status without its rows) = %v, want the deletion kept", err)
	}
}

// retentionRepoKeepsRetainedDeletedContent checks that a deleted status that the moderation
// records keep (a report or an action points at it, or a hold preserves its author) keeps its rows
// and comes due again DeletedContentRecheck later, and goes once the records no longer keep it.
func retentionRepoKeepsRetainedDeletedContent(t *testing.T, r *retentionRepo) {
	heldAuthor := r.ids.NewID()
	reported, reportedRev := r.contentPost(t, r.ids.NewID())
	held, heldRev := r.contentPost(t, heldAuthor)
	plain, plainRev := r.contentPost(t, r.ids.NewID())
	for _, s := range []content.Status{reported, held, plain} {
		r.deleteStatus(t, s, content.DeletedByModeration)
	}
	evidence := stubEvidence{statuses: map[string]bool{reported.ID: true}, authors: map[string]bool{heldAuthor: true}}

	r.clock.Advance(appcontent.DeletedContentRetention)
	if n := runToEnd(t, r.deletedPurge(evidence, nil, 0)); n != 1 {
		t.Errorf("purged %d statuses, want 1 (the one no record keeps)", n)
	}
	r.wantContent(t, "the status no record keeps", plain, plainRev, false)
	r.wantContent(t, "the reported status", reported, reportedRev, true)
	r.wantContent(t, "the status of a held author", held, heldRev, true)
	recheck := r.clock.Now().Add(appcontent.DeletedContentRecheck)
	if got := r.due(t, recheck.Add(-time.Second), 10); len(got) != 0 {
		t.Errorf("ListDueContentPurges before the recheck = %+v, want none", got)
	}
	if got, want := dueIDs(r.due(t, recheck, 10)), []string{reported.ID, held.ID}; !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Errorf("ListDueContentPurges at the recheck = %v, want the kept %v", got, want)
	}

	// Without the records, nothing happens before the recheck is due.
	if n := runToEnd(t, r.deletedPurge(nil, nil, 0)); n != 0 {
		t.Errorf("purged %d statuses before the recheck, want 0", n)
	}
	// At the recheck, the report is still open and the hold has ended.
	r.clock.Advance(appcontent.DeletedContentRecheck)
	stillReported := stubEvidence{statuses: map[string]bool{reported.ID: true}}
	if n := runToEnd(t, r.deletedPurge(stillReported, nil, 0)); n != 1 {
		t.Errorf("purged %d statuses at the recheck, want 1 (the hold ended)", n)
	}
	r.wantContent(t, "the status of a formerly held author", held, heldRev, false)
	r.wantContent(t, "the status still reported", reported, reportedRev, true)
	// Once the report is closed, the next recheck purges it.
	r.clock.Advance(appcontent.DeletedContentRecheck)
	if n := runToEnd(t, r.deletedPurge(stubEvidence{}, nil, 0)); n != 1 {
		t.Errorf("purged %d statuses once the report closed, want 1", n)
	}
	r.wantContent(t, "the formerly reported status", reported, reportedRev, false)
	for _, s := range []content.Status{reported, held, plain} {
		if _, err := r.h.Statuses.FindDeletion(t.Context(), s.ID); err != nil {
			t.Errorf("FindDeletion(%s) = %v, want the deletion kept", s.ID, err)
		}
	}
}

// deleteMany inserts n statuses of author with their content and deletes them by their author,
// and returns them.
func (r *retentionRepo) deleteMany(t *testing.T, author string, n int) []content.Status {
	t.Helper()
	out := make([]content.Status, n)
	for i := range out {
		out[i], _ = r.contentPost(t, author)
		r.deleteStatus(t, out[i], content.DeletedByAuthor)
	}
	return out
}

// retentionRepoBoundsDeletedPurges checks that a run of the purge of deleted statuses is bounded
// to 10 batches, says when more may be due, and ends even when every due status is kept.
func retentionRepoBoundsDeletedPurges(t *testing.T, r *retentionRepo) {
	const batch = 2
	author := r.ids.NewID()
	statuses := r.deleteMany(t, author, 25)
	r.clock.Advance(appcontent.DeletedContentRetention)

	// Every due status is kept: the runs end, and every purge comes due again later.
	if n := runToEnd(t, r.deletedPurge(stubEvidence{authors: map[string]bool{author: true}}, nil, batch)); n != 0 {
		t.Errorf("purged %d kept statuses, want 0", n)
	}
	if got := r.due(t, r.clock.Now(), 100); len(got) != 0 {
		t.Errorf("ListDueContentPurges after the runs over kept statuses = %d purges, want none due", len(got))
	}

	r.clock.Advance(appcontent.DeletedContentRecheck)
	p := r.deletedPurge(nil, nil, batch)
	if more, n := run(t, p); !more || n != 10*batch {
		t.Errorf("the first run over 25 due statuses = %v, %d; want true, %d (10 full batches)", more, n, 10*batch)
	}
	if more, n := run(t, p); more || n != 25-10*batch {
		t.Errorf("the second run = %v, %d; want false, %d", more, n, 25-10*batch)
	}
	if more, n := run(t, p); more || n != 0 {
		t.Errorf("a run after a complete one = %v, %d; want false, 0", more, n)
	}
	for _, s := range statuses {
		if _, err := r.h.Statuses.FindStatus(t.Context(), s.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindStatus(%s) = %v, want ErrNotFound", s.ID, err)
		}
	}
}

// recordingJobs is a queue.JobQueue that keeps the jobs it is given.
type recordingJobs struct{ jobs []queue.NewJob }

// Enqueue implements queue.JobQueue.
func (q *recordingJobs) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	q.jobs = append(q.jobs, jobs...)
	return nil
}

// retentionRepoContinuesDeletedPurges checks the job content.purge_deleted_statuses: it is
// scheduled hourly; a job that used up its batches enqueues one continuation, whose idempotency
// key names the job; a job that finishes enqueues none.
func retentionRepoContinuesDeletedPurges(t *testing.T, r *retentionRepo) {
	r.deleteMany(t, r.ids.NewID(), 25)
	r.clock.Advance(appcontent.DeletedContentRetention)
	jobs := &recordingJobs{}
	p := r.deletedPurge(nil, jobs, 2)
	reg := &queue.JobHandlerRegistry{}
	sched := queue.NewScheduler(nil, r.clock, nil)
	p.RegisterJobs(reg, sched)
	if !slices.ContainsFunc(sched.Schedules(), func(s queue.Schedule) bool {
		return s.Job.Type == appcontent.JobPurgeDeletedStatuses && s.Every == time.Hour
	}) {
		t.Errorf("schedules = %+v, want %s hourly", sched.Schedules(), appcontent.JobPurgeDeletedStatuses)
	}
	h, _, ok := reg.Lookup(appcontent.JobPurgeDeletedStatuses)
	if !ok {
		t.Fatalf("no handler of %s", appcontent.JobPurgeDeletedStatuses)
	}

	first := queue.Job{ID: r.ids.NewID(), Type: appcontent.JobPurgeDeletedStatuses, Attempt: 1, CreatedAt: r.clock.Now()}
	if res := h(t.Context(), first); !res.IsDone() {
		t.Fatalf("the first job = %+v, want done", res)
	}
	if len(jobs.jobs) != 1 {
		t.Fatalf("the first job enqueued %+v, want one continuation", jobs.jobs)
	}
	next := jobs.jobs[0]
	if next.Type != appcontent.JobPurgeDeletedStatuses || next.IdempotencyKey == "" || !strings.Contains(next.IdempotencyKey, first.ID) {
		t.Errorf("continuation = %+v, want a %s whose idempotency key names the job %s", next, appcontent.JobPurgeDeletedStatuses, first.ID)
	}
	jobs.jobs = nil
	second := queue.Job{ID: r.ids.NewID(), Type: next.Type, Payload: next.Payload, IdempotencyKey: next.IdempotencyKey, Attempt: 1, CreatedAt: r.clock.Now()}
	if res := h(t.Context(), second); !res.IsDone() {
		t.Fatalf("the continuation = %+v, want done", res)
	}
	if len(jobs.jobs) != 0 {
		t.Errorf("the continuation that finished enqueued %+v, want none", jobs.jobs)
	}
	if got := r.due(t, r.clock.Now(), 100); len(got) != 0 {
		t.Errorf("ListDueContentPurges after the jobs = %d purges, want none", len(got))
	}
}

// retentionRepoRetentionDeletesContentPurges checks that PurgeStatuses, which the purges of
// remote and relayed statuses use, deletes the purges of the statuses it removes.
func retentionRepoRetentionDeletesContentPurges(t *testing.T, r *retentionRepo) {
	purged := r.status(t, false, time.Hour, "")
	other := r.status(t, false, time.Hour, "")
	r.deleteStatus(t, purged, content.DeletedByRemote)
	r.deleteStatus(t, other, content.DeletedByRemote)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Retention.PurgeStatuses(ctx, tx, []string{purged.ID})
	})
	if n := r.purgeQueued(t, purged.ID); n != 0 {
		t.Errorf("purges of a status that PurgeStatuses removed = %d, want none", n)
	}
	if n := r.purgeQueued(t, other.ID); n != 1 {
		t.Errorf("purges of another deleted status = %d, want 1", n)
	}
}

// erasureRepoContentPurges checks that EraseStatuses deletes the purges of the statuses it
// erases.
func erasureRepoContentPurges(t *testing.T, r *erasureRepo) {
	author := r.account(t)
	post := func() content.Status {
		id := r.ids.NewID()
		s := content.Status{ID: id, URI: "https://" + testkit.ContractHost + "/statuses/" + id, AuthorAccountID: author, IsLocal: true, Visibility: content.VisibilityPublic, PublishedAt: r.clock.Now()}
		rev := content.Revision{ID: r.ids.NewID(), StatusID: id, Seq: 1, RevisedAt: s.PublishedAt, Bodies: []content.Body{{ContentHTML: "<p>hi</p>"}}}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(
				r.h.Statuses.InsertStatus(ctx, tx, s),
				r.h.Statuses.InsertRevision(ctx, tx, rev),
				r.h.Statuses.UpsertHead(ctx, tx, content.Head{StatusID: id, RevisionID: rev.ID, Seq: 1, LastRevisedAt: rev.RevisedAt}),
				r.h.Statuses.InsertDeletion(ctx, tx, content.Deletion{StatusID: id, URI: s.URI, AuthorAccountID: author, Origin: content.DeletedByAuthor, DeletedAt: r.clock.Now()}),
			)
		})
		return s
	}
	erased, other := post(), post()
	r.rows(t, "status_content_purges", "status_id", erased.ID, 1)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.ContentErasure.EraseStatuses(ctx, tx, []string{erased.ID})
	})
	r.rows(t, "status_content_purges", "status_id", erased.ID, 0)
	r.rows(t, "status_content_purges", "status_id", other.ID, 1)
}
