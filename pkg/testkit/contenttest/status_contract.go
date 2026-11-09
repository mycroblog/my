// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

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
	"github.com/mycroblog/my/pkg/testkit"
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
		run  func(t *testing.T, e *StatusEnv)
	}{
		{"StoresAStatus", statusStores},
		{"RejectsDuplicates", statusRejectsDuplicates},
		{"RejectsRemoteLocalStatuses", statusRejectsRemoteLocal},
		{"ReportsMissingRows", statusReportsMissing},
		{"RevisesStatuses", statusRevises},
		{"ReadsTheCurrentRevisionsOfManyStatuses", statusReadsCurrentRevisions},
		{"DeletesStatuses", statusDeletes},
		{"DeletesManyStatuses", statusDeletesMany},
		{"LinksReplies", statusLinksReplies},
		{"CountsPosts", statusCountsPosts},
		{"CountsLocalPosts", statusCountsLocalPosts},
		{"CountsDailyLocalPublicPosts", statusCountsDailyLocalPublicPosts},
		{"ListsRecentLocalPosters", statusListsRecentLocalPosters},
		{"EnsuresTags", statusEnsuresTags},
		{"ListsLiveRemoteStatusesOfAnAuthor", statusListsLiveRemote},
		{"ListsTheSearchableStatusesOfAnAuthor", statusListsSearchable},
		{"ListsTheSearchableStatusesOfTheServer", statusListsAllSearchable},
		{"IncrementalProjectionEqualsRebuild", statusProjectionEqualsRebuild},
		{"LinksAParentAndAReplyCreatedConcurrently", statusLinksConcurrentParentAndReply},
		{"KeepsADeletionOverAConcurrentRevision", statusDeletionWinsOverConcurrentRevision},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newStatusEnv(t, newHarness)) })
	}
}

// StatusEnv drives one harness with deterministic time and IDs. Its commands write the rows
// and emit the events that the content module's use cases do, without their rules.
type StatusEnv struct {
	H     StatusHarness
	Clock *testkit.Clock
	IDs   *testkit.IDGenerator
	// before and after, when set, run in the transaction of every status event, before and
	// after the content module's projectors; tests set them before starting the transactions
	// that they schedule.
	before, after func(e uow.Event)
}

func newStatusEnv(t *testing.T, newHarness StatusFactory) *StatusEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := testkit.NewClock(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	e := &StatusEnv{H: h, Clock: clock, IDs: testkit.NewIDGenerator(clock)}
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

// statusConvergesToRebuild fails the test unless the incremental projections equal a rebuild.
func statusConvergesToRebuild(t *testing.T, e *StatusEnv) {
	t.Helper()
	incremental := snapshotStatusProjections(t, e.H.Statuses)
	if err := appcontent.RebuildProjections(t.Context(), e.H.UnitOfWork, e.H.Statuses); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if rebuilt := snapshotStatusProjections(t, e.H.Statuses); !reflect.DeepEqual(incremental, rebuilt) {
		t.Errorf("incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", incremental, rebuilt)
	}
}

// statusLinksConcurrentParentAndReply creates a parent and its reply in concurrent transactions
// that each read the projections before the other commits, when the database lets them: the
// reply is linked and counted all the same.
func statusLinksConcurrentParentAndReply(t *testing.T, e *StatusEnv) {
	parent := e.NewStatus(e.IDs.NewID(), content.VisibilityPublic)
	reply := e.NewStatus(e.IDs.NewID(), content.VisibilityPublic)
	reply.InReplyToURI = parent.URI
	revs := []content.Revision{e.FirstRevision(parent), e.FirstRevision(reply)}
	meet := testkit.NewRendezvous(2)
	e.after = func(ev uow.Event) {
		if _, ok := ev.(appcontent.StatusCreated); ok {
			meet.Wait()
		}
	}
	var wg sync.WaitGroup
	for i, s := range []content.Status{parent, reply} {
		wg.Go(func() {
			if err := e.Create(t, s, nil, revs[i]); err != nil {
				t.Errorf("create %s = %v", s.URI, err)
			}
		})
	}
	wg.Wait()
	e.after = nil
	if links, err := e.H.Statuses.ListReplyLinks(t.Context()); err != nil || len(links) != 1 || links[0] != (content.ReplyLink{
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
func statusDeletionWinsOverConcurrentRevision(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	mentioned := e.IDs.NewID()
	s := e.NewStatus(e.IDs.NewID(), content.VisibilityPublic)
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
	if _, err := e.H.Statuses.FindCurrentRevision(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCurrentRevision of the deleted status = %v, want ErrNotFound", err)
	}
	if mentions, err := e.H.Statuses.ListMentions(ctx); err != nil || len(mentions) != 0 {
		t.Errorf("mentions = %+v, %v; want none", mentions, err)
	}
	statusConvergesToRebuild(t, e)
}

// NewStatus returns a status with a fresh ID, published now.
func (e *StatusEnv) NewStatus(author string, v content.Visibility) content.Status {
	e.Clock.Advance(time.Millisecond)
	id := e.IDs.NewID()
	return content.Status{
		ID:              id,
		URI:             content.StatusURI(testkit.ContractHost, id),
		AuthorAccountID: author,
		IsLocal:         true,
		Visibility:      v,
		PublishedAt:     e.Clock.Now(),
	}
}

// FirstRevision returns a minimal first revision of s.
func (e *StatusEnv) FirstRevision(s content.Status, mentions ...string) content.Revision {
	return content.Revision{
		ID:                  e.IDs.NewID(),
		StatusID:            s.ID,
		Seq:                 1,
		Language:            "en",
		RevisedAt:           s.PublishedAt,
		Bodies:              []content.Body{{Language: "en", ContentHTML: "<p>hello</p>"}},
		MentionedAccountIDs: mentions,
	}
}

// Create inserts a status, its local subtype when local is not nil, and its first revision,
// and emits StatusCreated.
func (e *StatusEnv) Create(t *testing.T, s content.Status, local *content.LocalStatus, rev content.Revision) error {
	t.Helper()
	return e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		ctx := t.Context()
		if err := e.H.Statuses.InsertStatus(ctx, tx, s); err != nil {
			return err
		}
		if local != nil {
			if err := e.H.Statuses.InsertLocalStatus(ctx, tx, *local); err != nil {
				return err
			}
		}
		if err := e.H.Statuses.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		return tx.Emit(ctx, appcontent.StatusCreated{Status: s, Revision: rev})
	})
}

func (e *StatusEnv) mustCreate(t *testing.T, s content.Status, mentions ...string) content.Revision {
	t.Helper()
	rev := e.FirstRevision(s, mentions...)
	if err := e.Create(t, s, nil, rev); err != nil {
		t.Fatalf("create status %s: %v", s.URI, err)
	}
	return rev
}

// revise inserts the next revision of a status after its current head and emits StatusRevised.
// The revision ID and time are fixed before the transaction; the seq is read inside it.
func (e *StatusEnv) revise(t *testing.T, statusID string, mentions ...string) (content.Revision, error) {
	t.Helper()
	rev := content.Revision{
		ID:                  e.IDs.NewID(),
		StatusID:            statusID,
		SpoilerText:         "edited",
		Sensitive:           true,
		Language:            "ja",
		RevisedAt:           e.Clock.Now(),
		Bodies:              []content.Body{{Language: "ja", ContentHTML: "<p>こんにちは</p>"}},
		MentionedAccountIDs: mentions,
	}
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		ctx := t.Context()
		head, err := e.H.Statuses.Head(ctx, tx, statusID)
		if err != nil {
			return err
		}
		rev.Seq = head.Seq + 1
		if err := e.H.Statuses.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		return tx.Emit(ctx, appcontent.StatusRevised{Revision: rev})
	})
	return rev, err
}

// InsertRevision inserts a given revision and emits StatusRevised.
func (e *StatusEnv) InsertRevision(t *testing.T, rev content.Revision) error {
	t.Helper()
	return e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.H.Statuses.InsertRevision(t.Context(), tx, rev); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.StatusRevised{Revision: rev})
	})
}

// remove records the deletion of s and emits StatusDeleted.
func (e *StatusEnv) remove(t *testing.T, s content.Status) (content.Deletion, error) {
	t.Helper()
	e.Clock.Advance(time.Millisecond)
	d := content.Deletion{
		StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByAuthor, DeletedAt: e.Clock.Now(),
	}
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.H.Statuses.InsertDeletion(t.Context(), tx, d); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.StatusDeleted{Deletion: d, Status: s})
	})
	return d, err
}

// mustRemoveMany records the deletions of sts together and emits one StatusesDeleted, as a
// moderation action on statuses does.
func (e *StatusEnv) mustRemoveMany(t *testing.T, sts []content.Status) {
	t.Helper()
	e.Clock.Advance(time.Millisecond)
	ev := appcontent.StatusesDeleted{}
	ds := make([]content.Deletion, len(sts))
	for i, s := range sts {
		ds[i] = content.Deletion{
			StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByModeration,
			ModerationActionID: testkit.NamedID("action"), DeletedAt: e.Clock.Now(),
		}
		ev.Deletions = append(ev.Deletions, appcontent.StatusDeleted{Deletion: ds[i], Status: s})
	}
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.H.Statuses.InsertDeletions(t.Context(), tx, ds); err != nil {
			return err
		}
		return tx.Emit(t.Context(), ev)
	})
	if err != nil {
		t.Fatalf("delete %d statuses: %v", len(sts), err)
	}
}

// MustRemove deletes s and fails t when it cannot.
func (e *StatusEnv) MustRemove(t *testing.T, s content.Status) {
	t.Helper()
	if _, err := e.remove(t, s); err != nil {
		t.Fatalf("delete status %s: %v", s.URI, err)
	}
}

func (e *StatusEnv) counters(t *testing.T, statusID string) content.Counters {
	t.Helper()
	c, err := e.H.Statuses.FindCounters(t.Context(), statusID)
	if err != nil {
		t.Fatalf("FindCounters(%s) = %v", statusID, err)
	}
	return c
}

func statusStores(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	author, mentioned := e.IDs.NewID(), e.IDs.NewID()
	s := e.NewStatus(author, content.VisibilityUnlisted)
	s.URL = "https://" + testkit.ContractHost + "/@alice/" + s.ID
	s.InReplyToURI = "https://remote.example/notes/1"
	s.ConversationURI = "https://remote.example/contexts/1"
	local := content.LocalStatus{StatusID: s.ID, AuthorAccountID: author, ClientID: e.IDs.NewID(), IdempotencyKey: "key-1"}
	tags := []string{e.IDs.NewID(), e.IDs.NewID()}
	rev := content.Revision{
		ID:          e.IDs.NewID(),
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
			{Position: 0, MediaID: e.IDs.NewID(), Description: "a cat"},
			{Position: 1, MediaID: e.IDs.NewID()},
		},
		MentionedAccountIDs: []string{mentioned},
		TagIDs:              tags,
		EmojiIDs:            []string{e.IDs.NewID()},
	}
	if err := e.Create(t, s, &local, rev); err != nil {
		t.Fatalf("create = %v", err)
	}

	for name, find := range map[string]func() (content.Status, error){
		"FindStatus":      func() (content.Status, error) { return e.H.Statuses.FindStatus(ctx, s.ID) },
		"FindStatusByURI": func() (content.Status, error) { return e.H.Statuses.FindStatusByURI(ctx, s.URI) },
	} {
		got, err := find()
		if err != nil || !reflect.DeepEqual(got, s) {
			t.Errorf("%s = %+v, %v; want %+v", name, got, err, s)
		}
	}
	if got, err := e.H.Statuses.FindStatusesByURIs(ctx, []string{s.URI, "https://remote.example/notes/missing"}); err != nil || len(got) != 1 || !reflect.DeepEqual(got[s.URI], s) {
		t.Errorf("FindStatusesByURIs = %+v, %v; want the status alone, as FindStatusByURI finds it", got, err)
	}
	missing := e.IDs.NewID()
	if got, err := e.H.Statuses.FindStatuses(ctx, []string{s.ID, missing, s.ID}); err != nil || len(got) != 1 || !reflect.DeepEqual(got[s.ID], s) {
		t.Errorf("FindStatuses = %+v, %v; want the status alone, as FindStatus finds it", got, err)
	}
	if got, err := e.H.Statuses.FindStatuses(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("FindStatuses(nil) = %+v, %v; want none", got, err)
	}
	if got, err := e.H.Statuses.FindLocalStatus(ctx, s.ID); err != nil || got != local {
		t.Errorf("FindLocalStatus = %+v, %v; want %+v", got, err, local)
	}
	if got, err := e.H.Statuses.FindLocalStatusByIdempotencyKey(ctx, author, "key-1"); err != nil || got != local {
		t.Errorf("FindLocalStatusByIdempotencyKey = %+v, %v; want %+v", got, err, local)
	}
	for name, find := range map[string]func() (content.Revision, error){
		"FindRevision":        func() (content.Revision, error) { return e.H.Statuses.FindRevision(ctx, rev.ID) },
		"FindCurrentRevision": func() (content.Revision, error) { return e.H.Statuses.FindCurrentRevision(ctx, s.ID) },
	} {
		got, err := find()
		if err != nil || !reflect.DeepEqual(got, rev) {
			t.Errorf("%s = %+v, %v;\nwant %+v", name, got, err, rev)
		}
	}
	sameCurrentRevisions(t, e, s.ID)
	heads, err := e.H.Statuses.ListHeads(ctx)
	if want := []content.Head{content.HeadOf(rev)}; err != nil || !reflect.DeepEqual(heads, want) {
		t.Errorf("ListHeads = %+v, %v; want %+v", heads, err, want)
	}
	if heads[0].Edited() {
		t.Error("a status with one revision is edited")
	}
	mentions, err := e.H.Statuses.ListMentions(ctx)
	if want := []content.Mention{{AccountID: mentioned, StatusID: s.ID}}; err != nil || !reflect.DeepEqual(mentions, want) {
		t.Errorf("ListMentions = %+v, %v; want %+v", mentions, err, want)
	}
	if c := e.counters(t, s.ID); c != (content.Counters{StatusID: s.ID}) {
		t.Errorf("counters = %+v, want zero", c)
	}
	if links, err := e.H.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Errorf("ListReplyLinks = %+v, %v; want none (the parent is unknown)", links, err)
	}
}

func statusRejectsDuplicates(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	author := e.IDs.NewID()
	first := e.NewStatus(author, content.VisibilityPublic)
	if err := e.Create(t, first, &content.LocalStatus{StatusID: first.ID, AuthorAccountID: author, IdempotencyKey: "k"}, e.FirstRevision(first)); err != nil {
		t.Fatalf("create = %v", err)
	}

	sameURI := e.NewStatus(author, content.VisibilityPublic)
	sameURI.URI = first.URI
	if err := e.Create(t, sameURI, nil, e.FirstRevision(sameURI)); !errors.Is(err, appcontent.ErrStatusExists) {
		t.Errorf("create with a taken URI = %v, want ErrStatusExists", err)
	}
	sameKey := e.NewStatus(author, content.VisibilityPublic)
	if err := e.Create(t, sameKey, &content.LocalStatus{StatusID: sameKey.ID, AuthorAccountID: author, IdempotencyKey: "k"}, e.FirstRevision(sameKey)); !errors.Is(err, appcontent.ErrIdempotencyKeyUsed) {
		t.Errorf("create with a used idempotency key = %v, want ErrIdempotencyKeyUsed", err)
	}
	if _, err := e.H.Statuses.FindStatus(ctx, sameKey.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindStatus after a rejected create = %v, want ErrNotFound (the transaction must roll back)", err)
	}

	// The key is unique per author, and statuses without a key do not collide.
	other := e.IDs.NewID()
	for _, ls := range []content.LocalStatus{
		{AuthorAccountID: other, IdempotencyKey: "k"},
		{AuthorAccountID: author},
		{AuthorAccountID: author},
	} {
		s := e.NewStatus(ls.AuthorAccountID, content.VisibilityPublic)
		ls.StatusID = s.ID
		if err := e.Create(t, s, &ls, e.FirstRevision(s)); err != nil {
			t.Errorf("create with %+v = %v", ls, err)
		}
	}

	dup := e.FirstRevision(first)
	if err := e.InsertRevision(t, dup); err == nil {
		t.Error("a second revision with seq 1 was accepted")
	}
	if c, err := e.H.Statuses.FindPostCounter(ctx, author); err != nil || c.StatusesCount != 3 {
		t.Errorf("post counter = %+v, %v; want 3 statuses", c, err)
	}
}

func statusRejectsRemoteLocal(t *testing.T, e *StatusEnv) {
	s := e.NewStatus(e.IDs.NewID(), content.VisibilityLocal)
	s.IsLocal = false
	if err := e.Create(t, s, nil, e.FirstRevision(s)); err == nil {
		t.Error("a remote status with local visibility was accepted")
	}
}

func statusReportsMissing(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	missing := "00000000-0000-7000-8000-000000000000"
	for name, err := range map[string]error{
		"FindStatus":                      Second(e.H.Statuses.FindStatus(ctx, missing)),
		"FindStatusByURI":                 Second(e.H.Statuses.FindStatusByURI(ctx, "https://nowhere.example/")),
		"FindLocalStatus":                 Second(e.H.Statuses.FindLocalStatus(ctx, missing)),
		"FindLocalStatusByIdempotencyKey": Second(e.H.Statuses.FindLocalStatusByIdempotencyKey(ctx, missing, "k")),
		"FindRevision":                    Second(e.H.Statuses.FindRevision(ctx, missing)),
		"FindCurrentRevision":             Second(e.H.Statuses.FindCurrentRevision(ctx, missing)),
		"FindDeletion":                    Second(e.H.Statuses.FindDeletion(ctx, missing)),
		"FindDeletionByURI":               Second(e.H.Statuses.FindDeletionByURI(ctx, "https://nowhere.example/")),
		"FindTagByName":                   Second(e.H.Statuses.FindTagByName(ctx, "nothing")),
		"FindTag":                         Second(e.H.Statuses.FindTag(ctx, missing)),
		"FindCounters":                    Second(e.H.Statuses.FindCounters(ctx, missing)),
		"FindPostCounter":                 Second(e.H.Statuses.FindPostCounter(ctx, missing)),
	} {
		if !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("%s(missing) = %v, want ErrNotFound", name, err)
		}
	}
	if revs, err := e.H.Statuses.ListRevisions(ctx, missing); err != nil || len(revs) != 0 {
		t.Errorf("ListRevisions(missing) = %+v, %v; want none", revs, err)
	}
}

// Second returns the error of a call that returns a value and an error.
func Second[T any](_ T, err error) error { return err }

// statusListsLiveRemote checks ListLiveRemoteStatusIDs: the remote statuses of one author that
// are not deleted, newest first, in keyset pages; local statuses and other authors are left out.
// statusListsSearchable checks ListAuthorSearchableStatusIDs: an author's public and local statuses
// in ID order, deleted ones included, and page by page.
func statusListsSearchable(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	author, other := e.IDs.NewID(), e.IDs.NewID()
	post := func(author string, v content.Visibility) content.Status {
		s := e.NewStatus(author, v)
		e.mustCreate(t, s)
		return s
	}
	public, local, deleted := post(author, content.VisibilityPublic), post(author, content.VisibilityLocal), post(author, content.VisibilityPublic)
	e.MustRemove(t, deleted)
	post(author, content.VisibilityUnlisted)
	post(author, content.VisibilityPrivate)
	post(author, content.VisibilityDirect)
	post(other, content.VisibilityPublic)

	got, err := e.H.Statuses.ListAuthorSearchableStatusIDs(ctx, author, "", 10)
	if want := []string{public.ID, local.ID, deleted.ID}; err != nil || !slices.Equal(got, want) {
		t.Errorf("ListAuthorSearchableStatusIDs = %v, %v; want %v", got, err, want)
	}
	got, err = e.H.Statuses.ListAuthorSearchableStatusIDs(ctx, author, public.ID, 1)
	if want := []string{local.ID}; err != nil || !slices.Equal(got, want) {
		t.Errorf("the page after the first = %v, %v; want %v", got, err, want)
	}
}

// statusListsAllSearchable checks ListSearchableStatusIDs: the public and local statuses of every
// author in ID order, deleted ones included, and page by page.
func statusListsAllSearchable(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	post := func(v content.Visibility) content.Status {
		s := e.NewStatus(e.IDs.NewID(), v)
		e.mustCreate(t, s)
		return s
	}
	public, local, deleted := post(content.VisibilityPublic), post(content.VisibilityLocal), post(content.VisibilityPublic)
	e.MustRemove(t, deleted)
	post(content.VisibilityUnlisted)
	post(content.VisibilityPrivate)
	post(content.VisibilityDirect)

	got, err := e.H.Statuses.ListSearchableStatusIDs(ctx, public.ID, 10)
	if want := []string{local.ID, deleted.ID}; err != nil || !slices.Equal(got, want) {
		t.Errorf("ListSearchableStatusIDs after the first = %v, %v; want %v", got, err, want)
	}
	got, err = e.H.Statuses.ListSearchableStatusIDs(ctx, public.ID, 1)
	if want := []string{local.ID}; err != nil || !slices.Equal(got, want) {
		t.Errorf("the page of 1 = %v, %v; want %v", got, err, want)
	}
	got, err = e.H.Statuses.ListSearchableStatusIDs(ctx, "", 100000)
	if err != nil || !slices.Contains(got, public.ID) || !slices.IsSorted(got) {
		t.Errorf("the first page = %v, %v; want it sorted, with %s", got, err, public.ID)
	}
}

func statusListsLiveRemote(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	author, other := e.IDs.NewID(), e.IDs.NewID()
	remote := func(author string) content.Status {
		s := e.NewStatus(author, content.VisibilityPublic)
		s.IsLocal = false
		s.URI = "https://remote.example/notes/" + s.ID
		e.mustCreate(t, s)
		return s
	}
	oldest, deleted, middle, newest := remote(author), remote(author), remote(author), remote(author)
	e.MustRemove(t, deleted)
	remote(other)
	local := e.NewStatus(author, content.VisibilityPublic)
	e.mustCreate(t, local)

	page, err := e.H.Statuses.ListLiveRemoteStatusIDs(ctx, author, "", 2)
	if err != nil || !slices.Equal(page, []string{newest.ID, middle.ID}) {
		t.Fatalf("first page = %v, %v; want [%s %s]", page, err, newest.ID, middle.ID)
	}
	page, err = e.H.Statuses.ListLiveRemoteStatusIDs(ctx, author, middle.ID, 2)
	if err != nil || !slices.Equal(page, []string{oldest.ID}) {
		t.Fatalf("second page = %v, %v; want [%s]", page, err, oldest.ID)
	}
	if page, err := e.H.Statuses.ListLiveRemoteStatusIDs(ctx, e.IDs.NewID(), "", 2); err != nil || len(page) != 0 {
		t.Errorf("unknown author = %v, %v; want none", page, err)
	}
}

func statusRevises(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	a, b, c := e.IDs.NewID(), e.IDs.NewID(), e.IDs.NewID()
	s := e.NewStatus(e.IDs.NewID(), content.VisibilityPublic)
	first := e.mustCreate(t, s, a, b)
	e.Clock.Advance(time.Minute)
	rev, err := e.revise(t, s.ID, c)
	if err != nil || rev.Seq != 2 {
		t.Fatalf("revise = %+v, %v; want seq 2", rev, err)
	}
	if got, err := e.H.Statuses.FindCurrentRevision(ctx, s.ID); err != nil || !reflect.DeepEqual(got, rev) {
		t.Errorf("FindCurrentRevision = %+v, %v; want %+v", got, err, rev)
	}
	heads, err := e.H.Statuses.ListHeads(ctx)
	if err != nil || len(heads) != 1 || heads[0] != content.HeadOf(rev) || !heads[0].Edited() {
		t.Errorf("ListHeads = %+v, %v; want the edited head %+v", heads, err, content.HeadOf(rev))
	}
	mentions, err := e.H.Statuses.ListMentions(ctx)
	if want := []content.Mention{{AccountID: c, StatusID: s.ID}}; err != nil || !reflect.DeepEqual(mentions, want) {
		t.Errorf("mentions after the edit = %+v, %v; want only the current revision's %+v", mentions, err, want)
	}

	// A revision that arrives out of order does not move the head back.
	stale := e.FirstRevision(s)
	stale.ID, stale.Seq = e.IDs.NewID(), 4
	newer := e.FirstRevision(s)
	newer.ID, newer.Seq = e.IDs.NewID(), 5
	for _, r := range []content.Revision{newer, stale} {
		if err := e.InsertRevision(t, r); err != nil {
			t.Fatalf("insert revision %d = %v", r.Seq, err)
		}
	}
	if got, err := e.H.Statuses.FindCurrentRevision(ctx, s.ID); err != nil || got.ID != newer.ID {
		t.Errorf("current revision = %s, %v; want seq 5 %s", got.ID, err, newer.ID)
	}
	sameCurrentRevisions(t, e, s.ID)
	revs, err := e.H.Statuses.ListRevisions(ctx, s.ID)
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
	if heads, err := e.H.Statuses.ListHeads(ctx); err != nil || len(heads) != 1 || heads[0].Seq != 5+writers {
		t.Errorf("after %d concurrent revisions: heads = %+v, %v; want seq %d", writers, heads, err, 5+writers)
	}
}

// statusDeletesMany records the deletions of many statuses by one action with one statement,
// each found as InsertDeletion records one, and fails as it does on a deleted status.
func statusDeletesMany(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	author := e.IDs.NewID()
	var sts []content.Status
	for range 3 {
		st := e.NewStatus(author, content.VisibilityPublic)
		e.mustCreate(t, st)
		sts = append(sts, st)
	}
	e.Clock.Advance(time.Millisecond)
	action := e.IDs.NewID()
	var ds []content.Deletion
	for _, st := range sts[:2] {
		ds = append(ds, content.Deletion{StatusID: st.ID, URI: st.URI, AuthorAccountID: author, Origin: content.DeletedByModeration, ModerationActionID: action, DeletedAt: e.Clock.Now()})
	}
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return errors.Join(e.H.Statuses.InsertDeletions(ctx, tx, ds), e.H.Statuses.InsertDeletions(ctx, tx, nil))
	}); err != nil {
		t.Fatalf("InsertDeletions = %v", err)
	}
	got, err := e.H.Statuses.FindDeletions(ctx, []string{sts[0].ID, sts[1].ID, sts[2].ID})
	if err != nil || len(got) != 2 || got[sts[0].ID] != ds[0] || got[sts[1].ID] != ds[1] {
		t.Errorf("FindDeletions after InsertDeletions = %+v, %v; want %+v", got, err, ds)
	}
	err = e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		again := ds[0]
		return e.H.Statuses.InsertDeletions(ctx, tx, []content.Deletion{again})
	})
	if !errors.Is(err, appcontent.ErrAlreadyDeleted) {
		t.Errorf("InsertDeletions of a deleted status = %v, want ErrAlreadyDeleted", err)
	}
}

func statusDeletes(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	author, mentioned := e.IDs.NewID(), e.IDs.NewID()
	s := e.NewStatus(author, content.VisibilityPublic)
	e.mustCreate(t, s, mentioned)
	kept := e.NewStatus(author, content.VisibilityPublic)
	keptRev := e.mustCreate(t, kept, mentioned)

	d, err := e.remove(t, s)
	if err != nil {
		t.Fatalf("delete = %v", err)
	}
	for name, find := range map[string]func() (content.Deletion, error){
		"FindDeletion":      func() (content.Deletion, error) { return e.H.Statuses.FindDeletion(ctx, s.ID) },
		"FindDeletionByURI": func() (content.Deletion, error) { return e.H.Statuses.FindDeletionByURI(ctx, s.URI) },
	} {
		if got, err := find(); err != nil || got != d {
			t.Errorf("%s = %+v, %v; want %+v", name, got, err, d)
		}
	}
	if got, err := e.H.Statuses.FindDeletions(ctx, []string{s.ID, kept.ID}); err != nil || len(got) != 1 || got[s.ID] != d {
		t.Errorf("FindDeletions = %+v, %v; want the deletion of the deleted status alone, as FindDeletion", got, err)
	}
	if _, err := e.remove(t, s); !errors.Is(err, appcontent.ErrAlreadyDeleted) {
		t.Errorf("second delete = %v, want ErrAlreadyDeleted", err)
	}

	// The status stays (the purge job removes it) but leaves every projection.
	if _, err := e.H.Statuses.FindStatus(ctx, s.ID); err != nil {
		t.Errorf("FindStatus after the deletion = %v", err)
	}
	if _, err := e.H.Statuses.FindCurrentRevision(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCurrentRevision of a deleted status = %v, want ErrNotFound", err)
	}
	if _, err := e.H.Statuses.FindCounters(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCounters of a deleted status = %v, want ErrNotFound", err)
	}
	mentions, err := e.H.Statuses.ListMentions(ctx)
	if want := []content.Mention{{AccountID: mentioned, StatusID: kept.ID}}; err != nil || !reflect.DeepEqual(mentions, want) {
		t.Errorf("mentions = %+v, %v; want only the kept status's %+v", mentions, err, want)
	}
	if heads, err := e.H.Statuses.ListHeads(ctx); err != nil || len(heads) != 1 || heads[0] != content.HeadOf(keptRev) {
		t.Errorf("heads = %+v, %v; want only the kept status's", heads, err)
	}

	// A late revision of a deleted status does not bring it back.
	late := e.FirstRevision(s)
	late.ID, late.Seq = e.IDs.NewID(), 2
	if err := e.InsertRevision(t, late); err != nil {
		t.Fatalf("insert late revision = %v", err)
	}
	if _, err := e.H.Statuses.FindCurrentRevision(ctx, s.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindCurrentRevision after a late revision = %v, want ErrNotFound", err)
	}
}

func statusLinksReplies(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	parentAuthor := e.IDs.NewID()
	parent := e.NewStatus(parentAuthor, content.VisibilityPublic)
	reply := func(v content.Visibility, to content.Status) content.Status {
		s := e.NewStatus(e.IDs.NewID(), v)
		s.InReplyToURI = to.URI
		return s
	}
	// Replies that arrive before their parent are linked when it arrives.
	early := reply(content.VisibilityUnlisted, parent)
	hidden := reply(content.VisibilityDirect, parent)
	e.mustCreate(t, early)
	e.mustCreate(t, hidden)
	if links, err := e.H.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Fatalf("links before the parent = %+v, %v; want none", links, err)
	}
	e.mustCreate(t, parent)
	late := reply(content.VisibilityLocal, parent)
	private := reply(content.VisibilityPrivate, parent)
	e.mustCreate(t, late)
	e.mustCreate(t, private)

	links, err := e.H.Statuses.ListReplyLinks(ctx)
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

	e.MustRemove(t, early)
	e.MustRemove(t, hidden)
	// ListReplies reads the live replies of a URI outside a transaction, ordered by ID.
	if got, err := e.H.Statuses.ListReplies(ctx, parent.URI); err != nil || len(got) != 2 || got[0].ID != late.ID || got[1].ID != private.ID {
		t.Errorf("ListReplies = %+v, %v; want the live replies %s, %s", got, err, late.ID, private.ID)
	}
	if got, err := e.H.Statuses.ListReplies(ctx, "https://nowhere.example/"); err != nil || len(got) != 0 {
		t.Errorf("ListReplies(unknown) = %+v, %v; want none", got, err)
	}
	if c := e.counters(t, parent.ID); c.RepliesCount != 1 {
		t.Errorf("replies count after deleting replies = %d, want 1", c.RepliesCount)
	}
	if links, err := e.H.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 2 {
		t.Errorf("links after deleting replies = %+v, %v; want 2", links, err)
	}
	e.MustRemove(t, parent)
	if links, err := e.H.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Errorf("links after deleting the parent = %+v, %v; want none", links, err)
	}
	if c := e.counters(t, late.ID); c != (content.Counters{StatusID: late.ID}) {
		t.Errorf("counters of a reply = %+v, want zero", c)
	}

	// A reply to a deleted status is not linked.
	orphan := reply(content.VisibilityPublic, parent)
	e.mustCreate(t, orphan)
	if _, err := e.H.Statuses.FindCounters(ctx, parent.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("counters of the deleted parent = %v, want ErrNotFound", err)
	}
	if links, err := e.H.Statuses.ListReplyLinks(ctx); err != nil || len(links) != 0 {
		t.Errorf("links after replying to a deleted status = %+v, %v; want none", links, err)
	}
}

func statusCountsPosts(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	author := e.IDs.NewID()
	counter := func() (content.PostCounter, error) { return e.H.Statuses.FindPostCounter(ctx, author) }
	older := e.NewStatus(author, content.VisibilityPublic)
	newer := e.NewStatus(author, content.VisibilityDirect)
	// A remote status can arrive after a newer one; the latest publication time wins.
	e.mustCreate(t, newer)
	e.mustCreate(t, older)
	if c, err := counter(); err != nil || c != (content.PostCounter{AccountID: author, StatusesCount: 2, LastStatusAt: newer.PublishedAt}) {
		t.Errorf("post counter = %+v, %v; want 2 statuses, last at %v", c, err, newer.PublishedAt)
	}
	e.MustRemove(t, newer)
	if c, err := counter(); err != nil || c != (content.PostCounter{AccountID: author, StatusesCount: 1, LastStatusAt: older.PublishedAt}) {
		t.Errorf("post counter after deleting the newest = %+v, %v; want 1 status, last at %v", c, err, older.PublishedAt)
	}
	e.MustRemove(t, older)
	if _, err := counter(); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("post counter after deleting every status = %v, want ErrNotFound", err)
	}
}

// statusCountsLocalPosts checks local_post_counters and its total, NodeInfo's usage.localPosts:
// the local statuses that are not deleted count, except the direct ones and remote statuses.
func statusCountsLocalPosts(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	total := func() int64 {
		t.Helper()
		n, err := e.H.Statuses.LocalPostsTotal(ctx)
		if err != nil {
			t.Fatalf("LocalPostsTotal = %v", err)
		}
		return n
	}
	if n := total(); n != 0 {
		t.Errorf("LocalPostsTotal on an empty database = %d, want 0", n)
	}
	alice, bob, remote := e.IDs.NewID(), e.IDs.NewID(), e.IDs.NewID()
	public := e.NewStatus(alice, content.VisibilityPublic)
	local := e.NewStatus(alice, content.VisibilityLocal)
	direct := e.NewStatus(alice, content.VisibilityDirect)
	private := e.NewStatus(bob, content.VisibilityPrivate)
	federated := e.NewStatus(remote, content.VisibilityPublic)
	federated.IsLocal = false
	federated.URI = "https://remote.example/notes/local-posts"
	for _, s := range []content.Status{public, local, direct, private, federated} {
		e.mustCreate(t, s)
	}
	want := []content.LocalPostCounter{{AccountID: alice, PostsCount: 2}, {AccountID: bob, PostsCount: 1}}
	slices.SortFunc(want, func(a, b content.LocalPostCounter) int { return strings.Compare(a.AccountID, b.AccountID) })
	if got, err := e.H.Statuses.ListLocalPostCounters(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("local post counters = %+v, %v; want %+v (no direct or remote statuses)", got, err, want)
	}
	if n := total(); n != 3 {
		t.Errorf("LocalPostsTotal = %d, want 3", n)
	}
	e.MustRemove(t, direct)
	e.MustRemove(t, public)
	e.MustRemove(t, private)
	e.MustRemove(t, federated)
	want = []content.LocalPostCounter{{AccountID: alice, PostsCount: 1}}
	if got, err := e.H.Statuses.ListLocalPostCounters(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("local post counters after deletions = %+v, %v; want %+v (an account without posts has no row)", got, err, want)
	}
	if n := total(); n != 1 {
		t.Errorf("LocalPostsTotal after deletions = %d, want 1", n)
	}
}

// statusCountsDailyLocalPublicPosts checks DailyLocalPublicPosts, the posts of the landing page's
// counts: only the public local statuses that are not deleted count, each in the period of 24
// hours before to that its publication time falls in.
func statusCountsDailyLocalPublicPosts(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	alice := e.IDs.NewID()
	post := func(v content.Visibility) content.Status {
		t.Helper()
		s := e.NewStatus(alice, v)
		local := content.LocalStatus{StatusID: s.ID, AuthorAccountID: alice}
		if err := e.Create(t, s, &local, e.FirstRevision(s)); err != nil {
			t.Fatalf("create status %s: %v", s.URI, err)
		}
		return s
	}
	const day = 24 * time.Hour
	post(content.VisibilityPublic) // more than 7 days before to
	e.Clock.Advance(2 * day)
	post(content.VisibilityPublic) // 5 days and 1 hour before to: the second period
	e.Clock.Advance(3 * day)
	post(content.VisibilityPublic) // 2 days and 1 hour before to: the fifth period
	for _, v := range []content.Visibility{content.VisibilityUnlisted, content.VisibilityLocal, content.VisibilityPrivate, content.VisibilityDirect} {
		post(v)
	}
	e.MustRemove(t, post(content.VisibilityPublic))
	remote := e.NewStatus(e.IDs.NewID(), content.VisibilityPublic)
	remote.IsLocal = false
	remote.URI = "https://remote.example/notes/daily"
	e.mustCreate(t, remote)
	e.Clock.Advance(2 * day)
	post(content.VisibilityPublic) // 1 hour before to: the last period
	to := e.Clock.Now().Add(time.Hour)
	got, err := e.H.Statuses.DailyLocalPublicPosts(ctx, to, 7)
	if want := []int{0, 1, 0, 0, 1, 0, 1}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("DailyLocalPublicPosts = %v, %v; want %v", got, err, want)
	}
	if got, err := e.H.Statuses.DailyLocalPublicPosts(ctx, to, 1); err != nil || !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("DailyLocalPublicPosts of 1 day = %v, %v; want [1]", got, err)
	}
}

// statusListsRecentLocalPosters checks ListRecentLocalPosters, the recently active accounts of
// the suggestions: the local accounts with a public or local status that is not deleted,
// published since the given time, the one that posted last first.
func statusListsRecentLocalPosters(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	post := func(author string, v content.Visibility) content.Status {
		t.Helper()
		s := e.NewStatus(author, v)
		local := content.LocalStatus{StatusID: s.ID, AuthorAccountID: author}
		if err := e.Create(t, s, &local, e.FirstRevision(s)); err != nil {
			t.Fatalf("create status %s: %v", s.URI, err)
		}
		e.Clock.Advance(time.Minute)
		return s
	}
	old, alice, bob, carol, quiet, gone := e.IDs.NewID(), e.IDs.NewID(), e.IDs.NewID(), e.IDs.NewID(), e.IDs.NewID(), e.IDs.NewID()
	post(old, content.VisibilityPublic)
	e.Clock.Advance(24 * time.Hour)
	since := e.Clock.Now()
	post(alice, content.VisibilityPublic)
	post(bob, content.VisibilityLocal)
	post(alice, content.VisibilityUnlisted)
	post(quiet, content.VisibilityPrivate)
	post(quiet, content.VisibilityDirect)
	e.MustRemove(t, post(gone, content.VisibilityPublic))
	remote := e.NewStatus(e.IDs.NewID(), content.VisibilityPublic)
	remote.IsLocal = false
	remote.URI = "https://remote.example/notes/recent"
	e.mustCreate(t, remote)
	post(carol, content.VisibilityPublic)

	got, err := e.H.Statuses.ListRecentLocalPosters(ctx, since, 10)
	if want := []string{carol, bob, alice}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ListRecentLocalPosters = %v, %v; want %v", got, err, want)
	}
	if got, err := e.H.Statuses.ListRecentLocalPosters(ctx, since, 1); err != nil || !reflect.DeepEqual(got, []string{carol}) {
		t.Errorf("ListRecentLocalPosters limit 1 = %v, %v; want [%s]", got, err, carol)
	}
}

func statusEnsuresTags(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	ensure := func(name string) content.Tag {
		t.Helper()
		e.Clock.Advance(time.Second)
		var got content.Tag
		err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			got, err = e.H.Statuses.EnsureTag(ctx, tx, content.Tag{
				ID: e.IDs.NewID(), Name: name, NameNormalized: "golang", CreatedAt: e.Clock.Now(),
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
	if got, err := e.H.Statuses.FindTagByName(ctx, "golang"); err != nil || got != first {
		t.Errorf("FindTagByName = %+v, %v; want %+v", got, err, first)
	}
	if got, err := e.H.Statuses.FindTag(ctx, first.ID); err != nil || got != first {
		t.Errorf("FindTag = %+v, %v; want %+v", got, err, first)
	}
	if got, err := e.H.Statuses.FindTags(ctx, []string{first.ID, e.IDs.NewID()}); err != nil || len(got) != 1 || got[first.ID] != first {
		t.Errorf("FindTags = %+v, %v; want %+v alone, as FindTag", got, err, first)
	}
	// Many tags together, as EnsureTag ensures each: the existing one is kept with its first
	// spelling, the new ones are stored.
	e.Clock.Advance(time.Second)
	tags := []content.Tag{
		{ID: e.IDs.NewID(), Name: "GOLANG", NameNormalized: "golang", CreatedAt: e.Clock.Now()},
		{ID: e.IDs.NewID(), Name: "Rust", NameNormalized: "rust", CreatedAt: e.Clock.Now()},
		{ID: e.IDs.NewID(), Name: "zig", NameNormalized: "zig", CreatedAt: e.Clock.Now()},
	}
	var got map[string]content.Tag
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) (err error) {
		got, err = e.H.Statuses.EnsureTags(ctx, tx, tags)
		return err
	}); err != nil {
		t.Fatalf("EnsureTags = %v", err)
	}
	if len(got) != 3 || got["golang"] != first || got["rust"].ID != tags[1].ID || got["rust"].Name != "Rust" || got["zig"].ID != tags[2].ID {
		t.Errorf("EnsureTags = %+v; want the first golang and the new rust and zig", got)
	}
	if one, err := e.H.Statuses.FindTagByName(ctx, "rust"); err != nil || one != got["rust"] {
		t.Errorf("FindTagByName(rust) = %+v, %v; want %+v", one, err, got["rust"])
	}
}

// statusProjectionEqualsRebuild runs random sequences of creations, revisions, and deletions
// over a fixed pool of URIs, so that replies reach parents that are known, unknown, arrive later,
// or are deleted, and checks that rebuilding the projections from their sources yields exactly
// the rows that the synchronous projectors wrote (data-model §9).
func statusProjectionEqualsRebuild(t *testing.T, e *StatusEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(94, 20261016)) //nolint:gosec // deterministic test input, not a secret
	visibilities := []content.Visibility{
		content.VisibilityPublic, content.VisibilityLocal, content.VisibilityUnlisted, content.VisibilityPrivate, content.VisibilityDirect,
	}
	accounts := make([]string, 5)
	for i := range accounts {
		accounts[i] = e.IDs.NewID()
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
			e.Clock.Advance(time.Duration(rng.IntN(5000)+1) * time.Millisecond)
			switch op := rng.IntN(10); {
			case op < 5 || len(created) == 0:
				n := rng.IntN(pool)
				if used[uri(n)] {
					continue
				}
				s := e.NewStatus(accounts[rng.IntN(len(accounts))], visibilities[rng.IntN(len(visibilities))])
				s.URI = uri(n)
				s.IsLocal = s.Visibility == content.VisibilityLocal || rng.IntN(2) == 0
				// Remote statuses arrive late, published up to a minute ago.
				s.PublishedAt = s.PublishedAt.Add(-time.Duration(rng.IntN(60000)) * time.Millisecond)
				if rng.IntN(3) > 0 {
					s.InReplyToURI = uri(rng.IntN(pool))
				}
				if err := e.Create(t, s, nil, e.FirstRevision(s, pickMentions()...)); err != nil {
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
			case op < 9 || rng.IntN(3) > 0:
				s := created[rng.IntN(len(created))]
				if !live[s.ID] {
					continue
				}
				e.MustRemove(t, s)
				live[s.ID] = false
			default:
				// Several statuses deleted together, as a moderation action deletes them.
				var batch []content.Status
				for range 2 {
					s := created[rng.IntN(len(created))]
					if live[s.ID] && !slices.ContainsFunc(batch, func(b content.Status) bool { return b.ID == s.ID }) {
						batch = append(batch, s)
					}
				}
				if len(batch) == 0 {
					continue
				}
				e.mustRemoveMany(t, batch)
				for _, s := range batch {
					live[s.ID] = false
				}
			}
		}
		incremental := snapshotStatusProjections(t, e.H.Statuses)
		if len(incremental.Heads) == 0 || len(incremental.Links) == 0 || len(incremental.Mentions) == 0 {
			t.Fatalf("round %d: projections are too sparse to compare: %+v", round, incremental)
		}
		if err := appcontent.RebuildProjections(ctx, e.H.UnitOfWork, e.H.Statuses); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		rebuilt := snapshotStatusProjections(t, e.H.Statuses)
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

// sameCurrentRevisions checks that FindCurrentRevisions, which reads the current revisions of many
// statuses together, finds each of ids's as FindCurrentRevision finds it, and leaves out a status
// that does not exist.
func sameCurrentRevisions(t *testing.T, e *StatusEnv, ids ...string) {
	t.Helper()
	ctx := t.Context()
	missing := e.IDs.NewID()
	got, err := e.H.Statuses.FindCurrentRevisions(ctx, append(slices.Clone(ids), missing))
	if err != nil {
		t.Fatalf("FindCurrentRevisions = %v", err)
	}
	for _, id := range ids {
		want, err := e.H.Statuses.FindCurrentRevision(ctx, id)
		if err != nil || !reflect.DeepEqual(got[id], want) {
			t.Errorf("FindCurrentRevisions[%s] = %+v;\nwant %+v (%v)", id, got[id], want, err)
		}
	}
	if rev, ok := got[missing]; ok {
		t.Errorf("FindCurrentRevisions[a missing status] = %+v, want none", rev)
	}
}

// statusReadsCurrentRevisions reads the current revisions of statuses of every kind together: a
// remote one with a poll and the source of its remote document, a plain one, and an edited one.
func statusReadsCurrentRevisions(t *testing.T, e *StatusEnv) {
	author := e.IDs.NewID()
	polled := e.NewStatus(author, content.VisibilityPublic)
	polled.IsLocal = false
	polled.URI = "https://remote.example/notes/" + polled.ID
	rev := e.FirstRevision(polled)
	closes := polled.PublishedAt.Add(time.Hour)
	rev.Poll = &content.Poll{ID: e.IDs.NewID(), Multiple: true, ClosesAt: &closes,
		Options: []content.PollOption{{Position: 0, Title: "yes"}, {Position: 1, Title: "no"}}}
	rev.Remote = &content.RemoteRevisionSource{ActivityURI: "https://remote.example/activities/1", HTMLPolicyVersion: 2, OmittedAttachments: 1}
	if err := e.Create(t, polled, nil, rev); err != nil {
		t.Fatalf("create the remote status = %v", err)
	}
	plain := e.NewStatus(author, content.VisibilityPublic)
	if err := e.Create(t, plain, nil, e.FirstRevision(plain)); err != nil {
		t.Fatalf("create = %v", err)
	}
	edited := e.NewStatus(author, content.VisibilityPublic)
	if err := e.Create(t, edited, nil, e.FirstRevision(edited)); err != nil {
		t.Fatalf("create = %v", err)
	}
	next := e.FirstRevision(edited)
	next.ID, next.Seq, next.SpoilerText = e.IDs.NewID(), 2, "edited"
	if err := e.InsertRevision(t, next); err != nil {
		t.Fatalf("revise = %v", err)
	}
	if got, err := e.H.Statuses.FindCurrentRevisions(t.Context(), []string{polled.ID}); err != nil ||
		got[polled.ID].Poll == nil || len(got[polled.ID].Poll.Options) != 2 || got[polled.ID].Remote == nil {
		t.Errorf("FindCurrentRevisions[the remote status] = %+v, %v; want its poll and remote source", got[polled.ID], err)
	}
	sameCurrentRevisions(t, e, polled.ID, plain.ID, edited.ID)

	// The poll's tallies and a voter's votes, as the reads of one poll find them.
	voter := e.IDs.NewID()
	tallies := []content.PollTally{{PollID: rev.Poll.ID, Position: 0, Votes: 2}, {PollID: rev.Poll.ID, Position: 1, Votes: 1}}
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.H.Statuses.ReplacePollTallies(t.Context(), tx, rev.Poll.ID, tallies); err != nil {
			return err
		}
		for _, pos := range []int{1, 0} {
			v := content.PollVote{ID: e.IDs.NewID(), PollID: rev.Poll.ID, AccountID: voter, Position: pos, VotedAt: e.Clock.Now()}
			if _, err := e.H.Statuses.InsertPollVote(t.Context(), tx, v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tallies and votes = %v", err)
	}
	other := e.IDs.NewID()
	wantTallies, err := e.H.Statuses.FindPollTallies(t.Context(), rev.Poll.ID)
	if got, gerr := e.H.Statuses.FindPollTalliesOf(t.Context(), []string{rev.Poll.ID, other}); err != nil || gerr != nil ||
		len(wantTallies) != 2 || !reflect.DeepEqual(got[rev.Poll.ID], wantTallies) || len(got[other]) != 0 {
		t.Errorf("FindPollTalliesOf = %+v, %v; want %+v (%v) as FindPollTallies", got, gerr, wantTallies, err)
	}
	wantVotes, err := e.H.Statuses.ListPollVotes(t.Context(), rev.Poll.ID, voter)
	if got, gerr := e.H.Statuses.ListPollVotesIn(t.Context(), []string{rev.Poll.ID, other}, voter); err != nil || gerr != nil ||
		len(wantVotes) != 2 || !reflect.DeepEqual(got[rev.Poll.ID], wantVotes) || len(got[other]) != 0 {
		t.Errorf("ListPollVotesIn = %+v, %v; want %+v (%v) as ListPollVotes", got, gerr, wantVotes, err)
	}
	if got, gerr := e.H.Statuses.ListPollVotesOfAccountsIn(t.Context(), []string{rev.Poll.ID, other}, []string{voter, other}); gerr != nil ||
		!reflect.DeepEqual(got[voter][rev.Poll.ID], wantVotes) || len(got[voter][other]) != 0 || len(got[other]) != 0 {
		t.Errorf("ListPollVotesOfAccountsIn = %+v, %v; want the voter's %+v alone, as ListPollVotes", got, gerr, wantVotes)
	}
	if got, err := e.H.Statuses.FindCurrentRevisions(t.Context(), nil); err != nil || len(got) != 0 {
		t.Errorf("FindCurrentRevisions(nil) = %+v, %v; want none", got, err)
	}
}
