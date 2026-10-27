// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// StatusHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the content module's status repository on it.
type StatusHarness struct {
	UnitOfWork uow.UnitOfWork
	Statuses   appcontent.Repository
}

// StatusFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type StatusFactory func(t *testing.T, opts uow.Options) StatusHarness

// StatusRepositoryContract verifies the status repository of a database adapter together with
// the content module's synchronous projectors: storing statuses with their revisions and
// children, the unique keys (URI, Idempotency-Key, revision seq, deletion), revisions moving
// the head, deletions as tombstones that remove the status from every projection, reply links
// in either order of arrival with the replies count, the post counters, tags, and the property
// that incrementally updated projections equal a rebuild from the sources.
func StatusRepositoryContract(t *testing.T, newHarness StatusFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *statusEnv)
	}{
		{"StoresAStatus", statusStores},
		{"RejectsDuplicates", statusRejectsDuplicates},
		{"RejectsRemoteLocalStatuses", statusRejectsRemoteLocal},
		{"ReportsMissingRows", statusReportsMissing},
		{"RevisesStatuses", statusRevises},
		{"DeletesStatuses", statusDeletes},
		{"LinksReplies", statusLinksReplies},
		{"CountsPosts", statusCountsPosts},
		{"CountsLocalPosts", statusCountsLocalPosts},
		{"EnsuresTags", statusEnsuresTags},
		{"ListsLiveRemoteStatusesOfAnAuthor", statusListsLiveRemote},
		{"IncrementalProjectionEqualsRebuild", statusProjectionEqualsRebuild},
		{"LinksAParentAndAReplyCreatedConcurrently", statusLinksConcurrentParentAndReply},
		{"KeepsADeletionOverAConcurrentRevision", statusDeletionWinsOverConcurrentRevision},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newStatusEnv(t, newHarness)) })
	}
}

// statusEnv drives one harness with deterministic time and IDs. Its commands write the rows
// and emit the events that the content module's use cases do, without their rules.
type statusEnv struct {
	h     StatusHarness
	clock *Clock
	ids   *IDGenerator
	// before and after, when set, run in the transaction of every status event, before and
	// after the content module's projectors; tests set them before starting the transactions
	// that they schedule.
	before, after func(e uow.Event)
}

func newStatusEnv(t *testing.T, newHarness StatusFactory) *statusEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	e := &statusEnv{h: h, clock: clock, ids: NewIDGenerator(clock)}
	events := []string{appcontent.EventStatusCreated, appcontent.EventStatusRevised, appcontent.EventStatusDeleted}
	hook := func(fn *func(uow.Event)) uow.Projector {
		return func(_ context.Context, _ uow.Tx, ev uow.Event) error {
			if *fn != nil {
				(*fn)(ev)
			}
			return nil
		}
	}
	for _, name := range events {
		projectors.Register(name, hook(&e.before))
	}
	appcontent.RegisterProjectors(projectors, h.Statuses)
	for _, name := range events {
		projectors.Register(name, hook(&e.after))
	}
	return e
}

// rendezvous lets each of n transactions wait, inside the transaction, until all n reach it or
// patience runs out. A transaction that waits for a lock that another one holds never reaches
// it, so patience bounds the wait; the transactions then go on in the order the database allows.
type rendezvous struct {
	arrived  chan struct{}
	n        int
	patience time.Duration
}

func newRendezvous(n int) *rendezvous {
	return &rendezvous{arrived: make(chan struct{}, n), n: n, patience: 300 * time.Millisecond}
}

func (r *rendezvous) wait() {
	r.arrived <- struct{}{}
	deadline := time.After(r.patience)
	for {
		if len(r.arrived) == r.n {
			return
		}
		select {
		case <-deadline:
			return
		case <-time.After(time.Millisecond):
		}
	}
}

// statusConvergesToRebuild fails the test unless the incremental projections equal a rebuild.
func statusConvergesToRebuild(t *testing.T, e *statusEnv) {
	t.Helper()
	incremental := snapshotStatusProjections(t, e.h.Statuses)
	if err := appcontent.RebuildProjections(t.Context(), e.h.UnitOfWork, e.h.Statuses); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if rebuilt := snapshotStatusProjections(t, e.h.Statuses); !reflect.DeepEqual(incremental, rebuilt) {
		t.Errorf("incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", incremental, rebuilt)
	}
}

// statusLinksConcurrentParentAndReply creates a parent and its reply in concurrent transactions
// that each read the projections before the other commits, when the database lets them: the
// reply is linked and counted all the same.
func statusLinksConcurrentParentAndReply(t *testing.T, e *statusEnv) {
	parent := e.newStatus(e.ids.NewID(), content.VisibilityPublic)
	reply := e.newStatus(e.ids.NewID(), content.VisibilityPublic)
	reply.InReplyToURI = parent.URI
	revs := []content.Revision{e.firstRevision(parent), e.firstRevision(reply)}
	meet := newRendezvous(2)
	e.after = func(ev uow.Event) {
		if _, ok := ev.(appcontent.StatusCreated); ok {
			meet.wait()
		}
	}
	var wg sync.WaitGroup
	for i, s := range []content.Status{parent, reply} {
		wg.Go(func() {
			if err := e.create(t, s, nil, revs[i]); err != nil {
				t.Errorf("create %s = %v", s.URI, err)
			}
		})
	}
	wg.Wait()
	e.after = nil
	if links, err := e.h.Statuses.ListReplyLinks(t.Context()); err != nil || len(links) != 1 || links[0] != (content.ReplyLink{
		StatusID: reply.ID, ParentStatusID: parent.ID, ParentAuthorAccountID: parent.AuthorAccountID,
	}) {
		t.Errorf("reply links = %+v, %v; want the reply linked to the parent", links, err)
	}
	if c := e.counters(t, parent.ID); c.RepliesCount != 1 {
		t.Errorf("replies count = %d, want 1", c.RepliesCount)
	}
	statusConvergesToRebuild(t, e)
}

// statusDeletionWinsOverConcurrentRevision revises a status while its deletion is being
// projected, when the database lets it: the revision does not bring the deleted status back into
// the projections.
func statusDeletionWinsOverConcurrentRevision(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	mentioned := e.ids.NewID()
	s := e.newStatus(e.ids.NewID(), content.VisibilityPublic)
	e.mustCreate(t, s, mentioned)
	reviseStarted := make(chan struct{})
	var once sync.Once
	e.before = func(ev uow.Event) {
		if _, ok := ev.(appcontent.StatusRevised); ok {
			once.Do(func() { close(reviseStarted) })
		}
	}
	e.after = func(ev uow.Event) {
		if _, ok := ev.(appcontent.StatusDeleted); ok {
			// Let the revision read the projections before the deletion commits.
			select {
			case <-reviseStarted:
				time.Sleep(100 * time.Millisecond)
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		if _, err := e.remove(t, s); err != nil {
			t.Errorf("delete = %v", err)
		}
	})
	wg.Go(func() {
		time.Sleep(20 * time.Millisecond)
		if _, err := e.revise(t, s.ID, mentioned); err != nil && !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("revise = %v", err)
		}
	})
	wg.Wait()
	e.before, e.after = nil, nil
	if _, err := e.h.Statuses.FindCurrentRevision(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCurrentRevision of the deleted status = %v, want ErrNotFound", err)
	}
	if mentions, err := e.h.Statuses.ListMentions(ctx); err != nil || len(mentions) != 0 {
		t.Errorf("mentions = %+v, %v; want none", mentions, err)
	}
	statusConvergesToRebuild(t, e)
}

// newStatus returns a status with a fresh ID, published now.
func (e *statusEnv) newStatus(author string, v content.Visibility) content.Status {
	e.clock.Advance(time.Millisecond)
	id := e.ids.NewID()
	return content.Status{
		ID:              id,
		URI:             content.StatusURI(contractHost, id),
		AuthorAccountID: author,
		IsLocal:         true,
		Visibility:      v,
		PublishedAt:     e.clock.Now(),
	}
}

// firstRevision returns a minimal first revision of s.
func (e *statusEnv) firstRevision(s content.Status, mentions ...string) content.Revision {
	return content.Revision{
		ID:                  e.ids.NewID(),
		StatusID:            s.ID,
		Seq:                 1,
		Language:            "en",
		RevisedAt:           s.PublishedAt,
		Bodies:              []content.Body{{Language: "en", ContentHTML: "<p>hello</p>"}},
		MentionedAccountIDs: mentions,
	}
}

// create inserts a status, its local subtype when local is not nil, and its first revision,
// and emits StatusCreated.
func (e *statusEnv) create(t *testing.T, s content.Status, local *content.LocalStatus, rev content.Revision) error {
	t.Helper()
	return e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		ctx := t.Context()
		if err := e.h.Statuses.InsertStatus(ctx, tx, s); err != nil {
			return err
		}
		if local != nil {
			if err := e.h.Statuses.InsertLocalStatus(ctx, tx, *local); err != nil {
				return err
			}
		}
		if err := e.h.Statuses.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		return tx.Emit(ctx, appcontent.StatusCreated{Status: s, Revision: rev})
	})
}

func (e *statusEnv) mustCreate(t *testing.T, s content.Status, mentions ...string) content.Revision {
	t.Helper()
	rev := e.firstRevision(s, mentions...)
	if err := e.create(t, s, nil, rev); err != nil {
		t.Fatalf("create status %s: %v", s.URI, err)
	}
	return rev
}

// revise inserts the next revision of a status after its current head and emits StatusRevised.
// The revision ID and time are fixed before the transaction; the seq is read inside it.
func (e *statusEnv) revise(t *testing.T, statusID string, mentions ...string) (content.Revision, error) {
	t.Helper()
	rev := content.Revision{
		ID:                  e.ids.NewID(),
		StatusID:            statusID,
		SpoilerText:         "edited",
		Sensitive:           true,
		Language:            "ja",
		RevisedAt:           e.clock.Now(),
		Bodies:              []content.Body{{Language: "ja", ContentHTML: "<p>こんにちは</p>"}},
		MentionedAccountIDs: mentions,
	}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		ctx := t.Context()
		head, err := e.h.Statuses.Head(ctx, tx, statusID)
		if err != nil {
			return err
		}
		rev.Seq = head.Seq + 1
		if err := e.h.Statuses.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		return tx.Emit(ctx, appcontent.StatusRevised{Revision: rev})
	})
	return rev, err
}

// insertRevision inserts a given revision and emits StatusRevised.
func (e *statusEnv) insertRevision(t *testing.T, rev content.Revision) error {
	t.Helper()
	return e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.h.Statuses.InsertRevision(t.Context(), tx, rev); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.StatusRevised{Revision: rev})
	})
}

// remove records the deletion of s and emits StatusDeleted.
func (e *statusEnv) remove(t *testing.T, s content.Status) (content.Deletion, error) {
	t.Helper()
	e.clock.Advance(time.Millisecond)
	d := content.Deletion{
		StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByAuthor, DeletedAt: e.clock.Now(),
	}
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.h.Statuses.InsertDeletion(t.Context(), tx, d); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.StatusDeleted{Deletion: d, Status: s})
	})
	return d, err
}

func (e *statusEnv) mustRemove(t *testing.T, s content.Status) {
	t.Helper()
	if _, err := e.remove(t, s); err != nil {
		t.Fatalf("delete status %s: %v", s.URI, err)
	}
}

func (e *statusEnv) counters(t *testing.T, statusID string) content.Counters {
	t.Helper()
	c, err := e.h.Statuses.FindCounters(t.Context(), statusID)
	if err != nil {
		t.Fatalf("FindCounters(%s) = %v", statusID, err)
	}
	return c
}

func statusStores(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	author, mentioned := e.ids.NewID(), e.ids.NewID()
	s := e.newStatus(author, content.VisibilityUnlisted)
	s.URL = "https://" + contractHost + "/@alice/" + s.ID
	s.InReplyToURI = "https://remote.example/notes/1"
	s.ConversationURI = "https://remote.example/contexts/1"
	local := content.LocalStatus{StatusID: s.ID, AuthorAccountID: author, ClientID: e.ids.NewID(), IdempotencyKey: "key-1"}
	tags := []string{e.ids.NewID(), e.ids.NewID()}
	rev := content.Revision{
		ID:          e.ids.NewID(),
		StatusID:    s.ID,
		Seq:         1,
		SpoilerText: "cw",
		Sensitive:   true,
		Language:    "en",
		RevisedAt:   s.PublishedAt,
		Bodies: []content.Body{
			{Language: "en", ContentHTML: "<p>hello</p>"},
			{Language: "ja", ContentHTML: "<p>こんにちは</p>"},
		},
		Source: &content.Source{Text: "hello #tag", Format: content.SourceTextPlain},
		Attachments: []content.Attachment{
			{Position: 0, MediaID: e.ids.NewID(), Description: "a cat"},
			{Position: 1, MediaID: e.ids.NewID()},
		},
		MentionedAccountIDs: []string{mentioned},
		TagIDs:              tags,
		EmojiIDs:            []string{e.ids.NewID()},
	}
	if err := e.create(t, s, &local, rev); err != nil {
		t.Fatalf("create = %v", err)
	}

	for name, find := range map[string]func() (content.Status, error){
		"FindStatus":      func() (content.Status, error) { return e.h.Statuses.FindStatus(ctx, s.ID) },
		"FindStatusByURI": func() (content.Status, error) { return e.h.Statuses.FindStatusByURI(ctx, s.URI) },
	} {
		got, err := find()
		if err != nil || !reflect.DeepEqual(got, s) {
			t.Errorf("%s = %+v, %v; want %+v", name, got, err, s)
		}
	}
	if got, err := e.h.Statuses.FindLocalStatus(ctx, s.ID); err != nil || got != local {
		t.Errorf("FindLocalStatus = %+v, %v; want %+v", got, err, local)
	}
	if got, err := e.h.Statuses.FindLocalStatusByIdempotencyKey(ctx, author, "key-1"); err != nil || got != local {
		t.Errorf("FindLocalStatusByIdempotencyKey = %+v, %v; want %+v", got, err, local)
	}
	for name, find := range map[string]func() (content.Revision, error){
		"FindRevision":        func() (content.Revision, error) { return e.h.Statuses.FindRevision(ctx, rev.ID) },
		"FindCurrentRevision": func() (content.Revision, error) { return e.h.Statuses.FindCurrentRevision(ctx, s.ID) },
	} {
		got, err := find()
		if err != nil || !reflect.DeepEqual(got, rev) {
			t.Errorf("%s = %+v, %v;\nwant %+v", name, got, err, rev)
		}
	}
	heads, err := e.h.Statuses.ListHeads(ctx)
	if want := []content.Head{content.HeadOf(rev)}; err != nil || !reflect.DeepEqual(heads, want) {
		t.Errorf("ListHeads = %+v, %v; want %+v", heads, err, want)
	}
	if heads[0].Edited() {
		t.Error("a status with one revision is edited")
	}
	mentions, err := e.h.Statuses.ListMentions(ctx)
	if want := []content.Mention{{AccountID: mentioned, StatusID: s.ID}}; err != nil || !reflect.DeepEqual(mentions, want) {
		t.Errorf("ListMentions = %+v, %v; want %+v", mentions, err, want)
	}
	if c := e.counters(t, s.ID); c != (content.Counters{StatusID: s.ID}) {
		t.Errorf("counters = %+v, want zero", c)
	}
	if links, err := e.h.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Errorf("ListReplyLinks = %+v, %v; want none (the parent is unknown)", links, err)
	}
}

func statusRejectsDuplicates(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	author := e.ids.NewID()
	first := e.newStatus(author, content.VisibilityPublic)
	if err := e.create(t, first, &content.LocalStatus{StatusID: first.ID, AuthorAccountID: author, IdempotencyKey: "k"}, e.firstRevision(first)); err != nil {
		t.Fatalf("create = %v", err)
	}

	sameURI := e.newStatus(author, content.VisibilityPublic)
	sameURI.URI = first.URI
	if err := e.create(t, sameURI, nil, e.firstRevision(sameURI)); !errors.Is(err, appcontent.ErrStatusExists) {
		t.Errorf("create with a taken URI = %v, want ErrStatusExists", err)
	}
	sameKey := e.newStatus(author, content.VisibilityPublic)
	if err := e.create(t, sameKey, &content.LocalStatus{StatusID: sameKey.ID, AuthorAccountID: author, IdempotencyKey: "k"}, e.firstRevision(sameKey)); !errors.Is(err, appcontent.ErrIdempotencyKeyUsed) {
		t.Errorf("create with a used idempotency key = %v, want ErrIdempotencyKeyUsed", err)
	}
	if _, err := e.h.Statuses.FindStatus(ctx, sameKey.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindStatus after a rejected create = %v, want ErrNotFound (the transaction must roll back)", err)
	}

	// The key is unique per author, and statuses without a key do not collide.
	other := e.ids.NewID()
	for _, ls := range []content.LocalStatus{
		{AuthorAccountID: other, IdempotencyKey: "k"},
		{AuthorAccountID: author},
		{AuthorAccountID: author},
	} {
		s := e.newStatus(ls.AuthorAccountID, content.VisibilityPublic)
		ls.StatusID = s.ID
		if err := e.create(t, s, &ls, e.firstRevision(s)); err != nil {
			t.Errorf("create with %+v = %v", ls, err)
		}
	}

	dup := e.firstRevision(first)
	if err := e.insertRevision(t, dup); err == nil {
		t.Error("a second revision with seq 1 was accepted")
	}
	if c, err := e.h.Statuses.FindPostCounter(ctx, author); err != nil || c.StatusesCount != 3 {
		t.Errorf("post counter = %+v, %v; want 3 statuses", c, err)
	}
}

func statusRejectsRemoteLocal(t *testing.T, e *statusEnv) {
	s := e.newStatus(e.ids.NewID(), content.VisibilityLocal)
	s.IsLocal = false
	if err := e.create(t, s, nil, e.firstRevision(s)); err == nil {
		t.Error("a remote status with local visibility was accepted")
	}
}

func statusReportsMissing(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	missing := "00000000-0000-7000-8000-000000000000"
	for name, err := range map[string]error{
		"FindStatus":                      second(e.h.Statuses.FindStatus(ctx, missing)),
		"FindStatusByURI":                 second(e.h.Statuses.FindStatusByURI(ctx, "https://nowhere.example/")),
		"FindLocalStatus":                 second(e.h.Statuses.FindLocalStatus(ctx, missing)),
		"FindLocalStatusByIdempotencyKey": second(e.h.Statuses.FindLocalStatusByIdempotencyKey(ctx, missing, "k")),
		"FindRevision":                    second(e.h.Statuses.FindRevision(ctx, missing)),
		"FindCurrentRevision":             second(e.h.Statuses.FindCurrentRevision(ctx, missing)),
		"FindDeletion":                    second(e.h.Statuses.FindDeletion(ctx, missing)),
		"FindDeletionByURI":               second(e.h.Statuses.FindDeletionByURI(ctx, "https://nowhere.example/")),
		"FindTagByName":                   second(e.h.Statuses.FindTagByName(ctx, "nothing")),
		"FindTag":                         second(e.h.Statuses.FindTag(ctx, missing)),
		"FindCounters":                    second(e.h.Statuses.FindCounters(ctx, missing)),
		"FindPostCounter":                 second(e.h.Statuses.FindPostCounter(ctx, missing)),
	} {
		if !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("%s(missing) = %v, want ErrNotFound", name, err)
		}
	}
	if revs, err := e.h.Statuses.ListRevisions(ctx, missing); err != nil || len(revs) != 0 {
		t.Errorf("ListRevisions(missing) = %+v, %v; want none", revs, err)
	}
}

func second[T any](_ T, err error) error { return err }

// statusListsLiveRemote checks ListLiveRemoteStatusIDs: the remote statuses of one author that
// are not deleted, newest first, in keyset pages; local statuses and other authors are left out.
func statusListsLiveRemote(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	author, other := e.ids.NewID(), e.ids.NewID()
	remote := func(author string) content.Status {
		s := e.newStatus(author, content.VisibilityPublic)
		s.IsLocal = false
		s.URI = "https://remote.example/notes/" + s.ID
		e.mustCreate(t, s)
		return s
	}
	oldest, deleted, middle, newest := remote(author), remote(author), remote(author), remote(author)
	e.mustRemove(t, deleted)
	remote(other)
	local := e.newStatus(author, content.VisibilityPublic)
	e.mustCreate(t, local)

	page, err := e.h.Statuses.ListLiveRemoteStatusIDs(ctx, author, "", 2)
	if err != nil || !slices.Equal(page, []string{newest.ID, middle.ID}) {
		t.Fatalf("first page = %v, %v; want [%s %s]", page, err, newest.ID, middle.ID)
	}
	page, err = e.h.Statuses.ListLiveRemoteStatusIDs(ctx, author, middle.ID, 2)
	if err != nil || !slices.Equal(page, []string{oldest.ID}) {
		t.Fatalf("second page = %v, %v; want [%s]", page, err, oldest.ID)
	}
	if page, err := e.h.Statuses.ListLiveRemoteStatusIDs(ctx, e.ids.NewID(), "", 2); err != nil || len(page) != 0 {
		t.Errorf("unknown author = %v, %v; want none", page, err)
	}
}

func statusRevises(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	a, b, c := e.ids.NewID(), e.ids.NewID(), e.ids.NewID()
	s := e.newStatus(e.ids.NewID(), content.VisibilityPublic)
	first := e.mustCreate(t, s, a, b)
	e.clock.Advance(time.Minute)
	rev, err := e.revise(t, s.ID, c)
	if err != nil || rev.Seq != 2 {
		t.Fatalf("revise = %+v, %v; want seq 2", rev, err)
	}
	if got, err := e.h.Statuses.FindCurrentRevision(ctx, s.ID); err != nil || !reflect.DeepEqual(got, rev) {
		t.Errorf("FindCurrentRevision = %+v, %v; want %+v", got, err, rev)
	}
	heads, err := e.h.Statuses.ListHeads(ctx)
	if err != nil || len(heads) != 1 || heads[0] != content.HeadOf(rev) || !heads[0].Edited() {
		t.Errorf("ListHeads = %+v, %v; want the edited head %+v", heads, err, content.HeadOf(rev))
	}
	mentions, err := e.h.Statuses.ListMentions(ctx)
	if want := []content.Mention{{AccountID: c, StatusID: s.ID}}; err != nil || !reflect.DeepEqual(mentions, want) {
		t.Errorf("mentions after the edit = %+v, %v; want only the current revision's %+v", mentions, err, want)
	}

	// A revision that arrives out of order does not move the head back.
	stale := e.firstRevision(s)
	stale.ID, stale.Seq = e.ids.NewID(), 4
	newer := e.firstRevision(s)
	newer.ID, newer.Seq = e.ids.NewID(), 5
	for _, r := range []content.Revision{newer, stale} {
		if err := e.insertRevision(t, r); err != nil {
			t.Fatalf("insert revision %d = %v", r.Seq, err)
		}
	}
	if got, err := e.h.Statuses.FindCurrentRevision(ctx, s.ID); err != nil || got.ID != newer.ID {
		t.Errorf("current revision = %s, %v; want seq 5 %s", got.ID, err, newer.ID)
	}
	revs, err := e.h.Statuses.ListRevisions(ctx, s.ID)
	if err != nil || len(revs) != 4 || revs[0].ID != first.ID || revs[1].ID != rev.ID || revs[3].ID != newer.ID {
		t.Errorf("ListRevisions = %+v, %v; want seq 1, 2, 4, 5", revs, err)
	}
	if revs[0].Bodies != nil {
		t.Errorf("ListRevisions returned children %+v", revs[0].Bodies)
	}

	// Concurrent revisions each take the next seq; none is lost.
	const writers = 8
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			if _, err := e.revise(t, s.ID); err != nil {
				t.Errorf("concurrent revise = %v", err)
			}
		})
	}
	wg.Wait()
	if heads, err := e.h.Statuses.ListHeads(ctx); err != nil || len(heads) != 1 || heads[0].Seq != 5+writers {
		t.Errorf("after %d concurrent revisions: heads = %+v, %v; want seq %d", writers, heads, err, 5+writers)
	}
}

func statusDeletes(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	author, mentioned := e.ids.NewID(), e.ids.NewID()
	s := e.newStatus(author, content.VisibilityPublic)
	e.mustCreate(t, s, mentioned)
	kept := e.newStatus(author, content.VisibilityPublic)
	keptRev := e.mustCreate(t, kept, mentioned)

	d, err := e.remove(t, s)
	if err != nil {
		t.Fatalf("delete = %v", err)
	}
	for name, find := range map[string]func() (content.Deletion, error){
		"FindDeletion":      func() (content.Deletion, error) { return e.h.Statuses.FindDeletion(ctx, s.ID) },
		"FindDeletionByURI": func() (content.Deletion, error) { return e.h.Statuses.FindDeletionByURI(ctx, s.URI) },
	} {
		if got, err := find(); err != nil || got != d {
			t.Errorf("%s = %+v, %v; want %+v", name, got, err, d)
		}
	}
	if _, err := e.remove(t, s); !errors.Is(err, appcontent.ErrAlreadyDeleted) {
		t.Errorf("second delete = %v, want ErrAlreadyDeleted", err)
	}

	// The status stays (the purge job removes it) but leaves every projection.
	if _, err := e.h.Statuses.FindStatus(ctx, s.ID); err != nil {
		t.Errorf("FindStatus after the deletion = %v", err)
	}
	if _, err := e.h.Statuses.FindCurrentRevision(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCurrentRevision of a deleted status = %v, want ErrNotFound", err)
	}
	if _, err := e.h.Statuses.FindCounters(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCounters of a deleted status = %v, want ErrNotFound", err)
	}
	mentions, err := e.h.Statuses.ListMentions(ctx)
	if want := []content.Mention{{AccountID: mentioned, StatusID: kept.ID}}; err != nil || !reflect.DeepEqual(mentions, want) {
		t.Errorf("mentions = %+v, %v; want only the kept status's %+v", mentions, err, want)
	}
	if heads, err := e.h.Statuses.ListHeads(ctx); err != nil || len(heads) != 1 || heads[0] != content.HeadOf(keptRev) {
		t.Errorf("heads = %+v, %v; want only the kept status's", heads, err)
	}

	// A late revision of a deleted status does not bring it back.
	late := e.firstRevision(s)
	late.ID, late.Seq = e.ids.NewID(), 2
	if err := e.insertRevision(t, late); err != nil {
		t.Fatalf("insert late revision = %v", err)
	}
	if _, err := e.h.Statuses.FindCurrentRevision(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCurrentRevision after a late revision = %v, want ErrNotFound", err)
	}
}

func statusLinksReplies(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	parentAuthor := e.ids.NewID()
	parent := e.newStatus(parentAuthor, content.VisibilityPublic)
	reply := func(v content.Visibility, to content.Status) content.Status {
		s := e.newStatus(e.ids.NewID(), v)
		s.InReplyToURI = to.URI
		return s
	}
	// Replies that arrive before their parent are linked when it arrives.
	early := reply(content.VisibilityUnlisted, parent)
	hidden := reply(content.VisibilityDirect, parent)
	e.mustCreate(t, early)
	e.mustCreate(t, hidden)
	if links, err := e.h.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Fatalf("links before the parent = %+v, %v; want none", links, err)
	}
	e.mustCreate(t, parent)
	late := reply(content.VisibilityLocal, parent)
	private := reply(content.VisibilityPrivate, parent)
	e.mustCreate(t, late)
	e.mustCreate(t, private)

	links, err := e.h.Statuses.ListReplyLinks(ctx)
	if err != nil || len(links) != 4 {
		t.Fatalf("ListReplyLinks = %+v, %v; want 4 links", links, err)
	}
	for _, l := range links {
		if l.ParentStatusID != parent.ID || l.ParentAuthorAccountID != parentAuthor {
			t.Errorf("link %+v does not point at the parent %s", l, parent.ID)
		}
	}
	if c := e.counters(t, parent.ID); c.RepliesCount != 2 {
		t.Errorf("replies count = %d, want 2 (private and direct replies are not counted)", c.RepliesCount)
	}

	e.mustRemove(t, early)
	e.mustRemove(t, hidden)
	// ListReplies reads the live replies of a URI outside a transaction, ordered by ID.
	if got, err := e.h.Statuses.ListReplies(ctx, parent.URI); err != nil || len(got) != 2 || got[0].ID != late.ID || got[1].ID != private.ID {
		t.Errorf("ListReplies = %+v, %v; want the live replies %s, %s", got, err, late.ID, private.ID)
	}
	if got, err := e.h.Statuses.ListReplies(ctx, "https://nowhere.example/"); err != nil || len(got) != 0 {
		t.Errorf("ListReplies(unknown) = %+v, %v; want none", got, err)
	}
	if c := e.counters(t, parent.ID); c.RepliesCount != 1 {
		t.Errorf("replies count after deleting replies = %d, want 1", c.RepliesCount)
	}
	if links, err := e.h.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 2 {
		t.Errorf("links after deleting replies = %+v, %v; want 2", links, err)
	}
	e.mustRemove(t, parent)
	if links, err := e.h.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Errorf("links after deleting the parent = %+v, %v; want none", links, err)
	}
	if c := e.counters(t, late.ID); c != (content.Counters{StatusID: late.ID}) {
		t.Errorf("counters of a reply = %+v, want zero", c)
	}

	// A reply to a deleted status is not linked.
	orphan := reply(content.VisibilityPublic, parent)
	e.mustCreate(t, orphan)
	if _, err := e.h.Statuses.FindCounters(ctx, parent.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("counters of the deleted parent = %v, want ErrNotFound", err)
	}
	if links, err := e.h.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Errorf("links after replying to a deleted status = %+v, %v; want none", links, err)
	}
}

func statusCountsPosts(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	author := e.ids.NewID()
	counter := func() (content.PostCounter, error) { return e.h.Statuses.FindPostCounter(ctx, author) }
	older := e.newStatus(author, content.VisibilityPublic)
	newer := e.newStatus(author, content.VisibilityDirect)
	// A remote status can arrive after a newer one; the latest publication time wins.
	e.mustCreate(t, newer)
	e.mustCreate(t, older)
	if c, err := counter(); err != nil || c != (content.PostCounter{AccountID: author, StatusesCount: 2, LastStatusAt: newer.PublishedAt}) {
		t.Errorf("post counter = %+v, %v; want 2 statuses, last at %v", c, err, newer.PublishedAt)
	}
	e.mustRemove(t, newer)
	if c, err := counter(); err != nil || c != (content.PostCounter{AccountID: author, StatusesCount: 1, LastStatusAt: older.PublishedAt}) {
		t.Errorf("post counter after deleting the newest = %+v, %v; want 1 status, last at %v", c, err, older.PublishedAt)
	}
	e.mustRemove(t, older)
	if _, err := counter(); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("post counter after deleting every status = %v, want ErrNotFound", err)
	}
}

// statusCountsLocalPosts checks local_post_counters and its total, NodeInfo's usage.localPosts:
// the local statuses that are not deleted count, except the direct ones and remote statuses.
func statusCountsLocalPosts(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	total := func() int64 {
		t.Helper()
		n, err := e.h.Statuses.LocalPostsTotal(ctx)
		if err != nil {
			t.Fatalf("LocalPostsTotal = %v", err)
		}
		return n
	}
	if n := total(); n != 0 {
		t.Errorf("LocalPostsTotal on an empty database = %d, want 0", n)
	}
	alice, bob, remote := e.ids.NewID(), e.ids.NewID(), e.ids.NewID()
	public := e.newStatus(alice, content.VisibilityPublic)
	local := e.newStatus(alice, content.VisibilityLocal)
	direct := e.newStatus(alice, content.VisibilityDirect)
	private := e.newStatus(bob, content.VisibilityPrivate)
	federated := e.newStatus(remote, content.VisibilityPublic)
	federated.IsLocal = false
	federated.URI = "https://remote.example/notes/local-posts"
	for _, s := range []content.Status{public, local, direct, private, federated} {
		e.mustCreate(t, s)
	}
	want := []content.LocalPostCounter{{AccountID: alice, PostsCount: 2}, {AccountID: bob, PostsCount: 1}}
	slices.SortFunc(want, func(a, b content.LocalPostCounter) int { return strings.Compare(a.AccountID, b.AccountID) })
	if got, err := e.h.Statuses.ListLocalPostCounters(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("local post counters = %+v, %v; want %+v (no direct or remote statuses)", got, err, want)
	}
	if n := total(); n != 3 {
		t.Errorf("LocalPostsTotal = %d, want 3", n)
	}
	e.mustRemove(t, direct)
	e.mustRemove(t, public)
	e.mustRemove(t, private)
	e.mustRemove(t, federated)
	want = []content.LocalPostCounter{{AccountID: alice, PostsCount: 1}}
	if got, err := e.h.Statuses.ListLocalPostCounters(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("local post counters after deletions = %+v, %v; want %+v (an account without posts has no row)", got, err, want)
	}
	if n := total(); n != 1 {
		t.Errorf("LocalPostsTotal after deletions = %d, want 1", n)
	}
}

func statusEnsuresTags(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	ensure := func(name string) content.Tag {
		t.Helper()
		e.clock.Advance(time.Second)
		var got content.Tag
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			got, err = e.h.Statuses.EnsureTag(ctx, tx, content.Tag{
				ID: e.ids.NewID(), Name: name, NameNormalized: "golang", CreatedAt: e.clock.Now(),
			})
			return err
		})
		if err != nil {
			t.Fatalf("EnsureTag(%s) = %v", name, err)
		}
		return got
	}
	first := ensure("GoLang")
	if again := ensure("golang"); again != first {
		t.Errorf("EnsureTag again = %+v, want the first tag %+v", again, first)
	}
	if first.Name != "GoLang" {
		t.Errorf("tag name = %q, want the first spelling", first.Name)
	}
	if got, err := e.h.Statuses.FindTagByName(ctx, "golang"); err != nil || got != first {
		t.Errorf("FindTagByName = %+v, %v; want %+v", got, err, first)
	}
	if got, err := e.h.Statuses.FindTag(ctx, first.ID); err != nil || got != first {
		t.Errorf("FindTag = %+v, %v; want %+v", got, err, first)
	}
}

// statusProjectionEqualsRebuild runs random sequences of creations, revisions, and deletions
// over a fixed pool of URIs, so that replies reach parents that are known, unknown, arrive later,
// or are deleted, and checks that rebuilding the projections from their sources yields exactly
// the rows that the synchronous projectors wrote (data-model §9).
func statusProjectionEqualsRebuild(t *testing.T, e *statusEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(94, 20261016)) //nolint:gosec // deterministic test input, not a secret
	visibilities := []content.Visibility{
		content.VisibilityPublic, content.VisibilityLocal, content.VisibilityUnlisted, content.VisibilityPrivate, content.VisibilityDirect,
	}
	accounts := make([]string, 5)
	for i := range accounts {
		accounts[i] = e.ids.NewID()
	}
	const pool = 40
	uri := func(i int) string { return fmt.Sprintf("https://remote.example/notes/%d", i) }
	var (
		created []content.Status
		live    = map[string]bool{}
		used    = map[string]bool{}
	)
	pickMentions := func() []string {
		var ids []string
		for _, a := range accounts {
			if rng.IntN(3) == 0 {
				ids = append(ids, a)
			}
		}
		return ids
	}
	for round := range 3 {
		for range 60 {
			e.clock.Advance(time.Duration(rng.IntN(5000)+1) * time.Millisecond)
			switch op := rng.IntN(10); {
			case op < 5 || len(created) == 0:
				n := rng.IntN(pool)
				if used[uri(n)] {
					continue
				}
				s := e.newStatus(accounts[rng.IntN(len(accounts))], visibilities[rng.IntN(len(visibilities))])
				s.URI = uri(n)
				s.IsLocal = s.Visibility == content.VisibilityLocal || rng.IntN(2) == 0
				// Remote statuses arrive late, published up to a minute ago.
				s.PublishedAt = s.PublishedAt.Add(-time.Duration(rng.IntN(60000)) * time.Millisecond)
				if rng.IntN(3) > 0 {
					s.InReplyToURI = uri(rng.IntN(pool))
				}
				if err := e.create(t, s, nil, e.firstRevision(s, pickMentions()...)); err != nil {
					t.Fatalf("create = %v", err)
				}
				used[s.URI] = true
				live[s.ID] = true
				created = append(created, s)
			case op < 8:
				s := created[rng.IntN(len(created))]
				if !live[s.ID] {
					continue
				}
				if _, err := e.revise(t, s.ID, pickMentions()...); err != nil {
					t.Fatalf("revise = %v", err)
				}
			default:
				s := created[rng.IntN(len(created))]
				if !live[s.ID] {
					continue
				}
				e.mustRemove(t, s)
				live[s.ID] = false
			}
		}
		incremental := snapshotStatusProjections(t, e.h.Statuses)
		if len(incremental.Heads) == 0 || len(incremental.Links) == 0 || len(incremental.Mentions) == 0 {
			t.Fatalf("round %d: projections are too sparse to compare: %+v", round, incremental)
		}
		if err := appcontent.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Statuses); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		rebuilt := snapshotStatusProjections(t, e.h.Statuses)
		if !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}

type statusProjections struct {
	Heads        []content.Head
	Mentions     []content.Mention
	Links        []content.ReplyLink
	Counters     []content.Counters
	PostCounters []content.PostCounter
	LocalPosts   []content.LocalPostCounter
}

func snapshotStatusProjections(t *testing.T, repo appcontent.Repository) statusProjections {
	t.Helper()
	var (
		p   statusProjections
		err error
	)
	if p.Heads, err = repo.ListHeads(t.Context()); err != nil {
		t.Fatalf("ListHeads = %v", err)
	}
	if p.Mentions, err = repo.ListMentions(t.Context()); err != nil {
		t.Fatalf("ListMentions = %v", err)
	}
	if p.Links, err = repo.ListReplyLinks(t.Context()); err != nil {
		t.Fatalf("ListReplyLinks = %v", err)
	}
	if p.Counters, err = repo.ListCounters(t.Context()); err != nil {
		t.Fatalf("ListCounters = %v", err)
	}
	if p.PostCounters, err = repo.ListPostCounters(t.Context()); err != nil {
		t.Fatalf("ListPostCounters = %v", err)
	}
	if p.LocalPosts, err = repo.ListLocalPostCounters(t.Context()); err != nil {
		t.Fatalf("ListLocalPostCounters = %v", err)
	}
	return p
}
