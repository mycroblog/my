// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

// RemoteStatusContract verifies the repository methods that the ingestion of remote posts uses on a
// database adapter, by those methods alone, the port contract of ADR-0153 §1: a remote status is
// stored once per URI and found by its URI inside a transaction, alive until its deletion, which is
// recorded once per status and URI; its revisions keep their remote source and the head points at
// the newest; the live replies to a URI are listed in ID order; the link of a reply is upserted;
// a received activity is recorded once with its document, and one of trust refetch, never one of
// trust signed, is released; the tombstones of objects are recorded once per actor and URI and
// hold back only the activities of their actor; the quotes of posts, their events, policies,
// approvals, and states are stored and listed (ADR-0069); and the pins that featured collections
// become take one of content.MaxPins slots (ADR-0006). The ingestion on top of it is RemoteStatusFlows,
// which internal/flowtest/federation runs once on SQLite; StatusRepositoryContract checks the rest
// of the status repository.
func RemoteStatusContract(t *testing.T, newHarness RemoteStatusFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *remoteStatusRepo)
	}{
		{"StoresRemoteStatusesByURI", remoteStatusRepoStatuses},
		{"DeletesRemoteStatusesOnce", remoteStatusRepoDeletions},
		{"KeepsTheRemoteSourceOfRevisions", remoteStatusRepoRevisions},
		{"ListsTheLiveRepliesAndLinksThem", remoteStatusRepoReplies},
		{"RecordsAndReleasesReceivedActivities", remoteStatusRepoReceived},
		{"RecordsTheTombstonesOfObjects", remoteStatusRepoTombstones},
		{"StoresTheQuotesOfPosts", remoteStatusRepoQuotes},
		{"StoresThePinsOfFeaturedCollections", remoteStatusRepoPins},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &remoteStatusRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type remoteStatusRepo struct {
	h     RemoteStatusHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *remoteStatusRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// remoteStatus returns a remote status, not stored, that replies to inReplyTo ("" for none).
func (r *remoteStatusRepo) remoteStatus(inReplyTo string) content.Status {
	id := r.ids.NewID()
	return content.Status{
		ID: id, URI: "https://remote.example/notes/" + id, URL: "https://remote.example/@alice/" + id,
		AuthorAccountID: testkit.NamedID("remote-author"), Visibility: content.VisibilityPublic, InReplyToURI: inReplyTo,
		ConversationURI: "https://remote.example/contexts/1", PublishedAt: r.clock.Now(),
	}
}

func (r *remoteStatusRepo) insert(t *testing.T, s content.Status) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Statuses.InsertStatus(ctx, tx, s) })
}

func remoteStatusRepoStatuses(t *testing.T, r *remoteStatusRepo) {
	ctx := t.Context()
	s := r.remoteStatus("")
	r.insert(t, s)
	again := s
	again.ID = r.ids.NewID()
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Statuses.InsertStatus(ctx, tx, again) })
	if !errors.Is(err, appcontent.ErrStatusExists) {
		t.Errorf("InsertStatus of a stored URI = %v, want ErrStatusExists", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Statuses.FindStatusByURITx(ctx, tx, s.URI); err != nil || got != s {
			t.Errorf("FindStatusByURITx = %+v, %v; want %+v", got, err, s)
		}
		if got, err := r.h.Statuses.FindLiveStatusByURI(ctx, tx, s.URI); err != nil || got != s {
			t.Errorf("FindLiveStatusByURI = %+v, %v; want %+v", got, err, s)
		}
		if _, err := r.h.Statuses.FindStatusByURITx(ctx, tx, "https://remote.example/notes/unknown"); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindStatusByURITx(unknown) = %v, want ErrNotFound", err)
		}
		if deleted, err := r.h.Statuses.IsDeleted(ctx, tx, s.ID); err != nil || deleted {
			t.Errorf("IsDeleted(stored) = %v, %v; want false", deleted, err)
		}
		return nil
	})
}

func remoteStatusRepoDeletions(t *testing.T, r *remoteStatusRepo) {
	ctx := t.Context()
	s := r.remoteStatus("")
	r.insert(t, s)
	d := content.Deletion{StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByRemote, DeletedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Statuses.InsertDeletion(ctx, tx, d) })
	for name, again := range map[string]content.Deletion{
		"the status": d,
		"its URI":    {StatusID: r.ids.NewID(), URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByRemote, DeletedAt: r.clock.Now()},
	} {
		err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Statuses.InsertDeletion(ctx, tx, again) })
		if !errors.Is(err, appcontent.ErrAlreadyDeleted) {
			t.Errorf("a second deletion of %s = %v, want ErrAlreadyDeleted", name, err)
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.h.Statuses.FindLiveStatusByURI(ctx, tx, s.URI); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindLiveStatusByURI(deleted) = %v, want ErrNotFound", err)
		}
		if deleted, err := r.h.Statuses.IsDeleted(ctx, tx, s.ID); err != nil || !deleted {
			t.Errorf("IsDeleted(deleted) = %v, %v; want true", deleted, err)
		}
		if got, err := r.h.Statuses.FindDeletionByURITx(ctx, tx, s.URI); err != nil || got != d {
			t.Errorf("FindDeletionByURITx = %+v, %v; want %+v", got, err, d)
		}
		return nil
	})
	// A Delete of a URI never stored leaves a tombstone without a status.
	tomb := content.Deletion{StatusID: r.ids.NewID(), URI: "https://remote.example/notes/never", AuthorAccountID: s.AuthorAccountID, Origin: content.DeletedByRemote, DeletedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Statuses.InsertDeletion(ctx, tx, tomb) })
	if got, err := r.h.Statuses.FindDeletionByURI(ctx, tomb.URI); err != nil || got.StatusID != tomb.StatusID {
		t.Errorf("FindDeletionByURI(the tombstone of an unknown URI) = %+v, %v; want %+v", got, err, tomb)
	}
}

func remoteStatusRepoRevisions(t *testing.T, r *remoteStatusRepo) {
	ctx := t.Context()
	s := r.remoteStatus("")
	r.insert(t, s)
	var revs []content.Revision
	for seq := 1; seq <= 2; seq++ {
		rev := content.Revision{
			ID: r.ids.NewID(), StatusID: s.ID, Seq: seq, SpoilerText: "cw", Sensitive: seq == 2, RevisedAt: r.clock.Now(),
			Bodies: []content.Body{{Language: "en", ContentHTML: "<p>hi</p>"}},
			Remote: &content.RemoteRevisionSource{ActivityURI: s.URI + "/activity/" + strconv.Itoa(seq), HTMLPolicyVersion: 3, OmittedAttachments: seq},
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return errors.Join(
				r.h.Statuses.InsertRevision(ctx, tx, rev),
				r.h.Statuses.UpsertHead(ctx, tx, content.Head{StatusID: s.ID, RevisionID: rev.ID, Seq: seq, LastRevisedAt: rev.RevisedAt}),
			)
		})
		revs = append(revs, rev)
		r.clock.Advance(time.Second)
	}
	got, err := r.h.Statuses.FindCurrentRevision(ctx, s.ID)
	if err != nil || got.ID != revs[1].ID || got.Remote == nil || *got.Remote != *revs[1].Remote || !got.Sensitive {
		t.Errorf("FindCurrentRevision = %+v, %v; want %+v with its remote source", got, err, revs[1])
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		first, err := r.h.Statuses.FindRevisionTx(ctx, tx, revs[0].ID)
		if err != nil || first.Remote == nil || *first.Remote != *revs[0].Remote {
			t.Errorf("FindRevisionTx(first) = %+v, %v; want the remote source %+v", first, err, revs[0].Remote)
		}
		if head, err := r.h.Statuses.Head(ctx, tx, s.ID); err != nil || head.RevisionID != revs[1].ID || head.Seq != 2 {
			t.Errorf("Head = %+v, %v; want the second revision", head, err)
		}
		return nil
	})
}

func remoteStatusRepoReplies(t *testing.T, r *remoteStatusRepo) {
	parent := r.remoteStatus("")
	r.insert(t, parent)
	first, deleted, second := r.remoteStatus(parent.URI), r.remoteStatus(parent.URI), r.remoteStatus(parent.URI)
	for _, s := range []content.Status{first, deleted, second, r.remoteStatus("")} {
		r.insert(t, s)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.InsertDeletion(ctx, tx, content.Deletion{
			StatusID: deleted.ID, URI: deleted.URI, AuthorAccountID: deleted.AuthorAccountID, Origin: content.DeletedByRemote, DeletedAt: r.clock.Now(),
		})
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		replies, err := r.h.Statuses.ListLiveRepliesTo(ctx, tx, parent.URI)
		if want := []string{first.ID, second.ID}; err != nil || !slices.Equal(statusIDs(replies), want) {
			t.Errorf("ListLiveRepliesTo = %v, %v; want %v", statusIDs(replies), err, want)
		}
		if _, err := r.h.Statuses.ReplyLink(ctx, tx, first.ID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("ReplyLink before one = %v, want ErrNotFound", err)
		}
		link := content.ReplyLink{StatusID: first.ID, ParentStatusID: parent.ID, ParentAuthorAccountID: parent.AuthorAccountID}
		if err := r.h.Statuses.UpsertReplyLink(ctx, tx, link); err != nil {
			return err
		}
		if err := r.h.Statuses.UpsertReplyLink(ctx, tx, link); err != nil {
			return err
		}
		if got, err := r.h.Statuses.ReplyLink(ctx, tx, first.ID); err != nil || got != link {
			t.Errorf("ReplyLink = %+v, %v; want %+v", got, err, link)
		}
		return nil
	})
}

func remoteStatusRepoReceived(t *testing.T, r *remoteStatusRepo) {
	ctx := t.Context()
	const actor = federation.IRI("https://remote.example/users/alice")
	record := func(n string, trust federation.Trust) federation.ReceivedActivity {
		a := federation.ReceivedActivity{
			ActivityURI: actor + "/activities/" + federation.IRI(n), ActorURI: actor, Type: "Create", Scheme: "rfc9421", Trust: trust, ReceivedAt: r.clock.Now(),
		}
		for i, want := range []bool{true, false} {
			r.tx(t, func(ctx context.Context, tx uow.Tx) error {
				ok, err := r.h.Inbox.InsertReceivedActivity(ctx, tx, a)
				if err == nil && ok != want {
					t.Errorf("InsertReceivedActivity(%s) #%d = %v, want %v", a.ActivityURI, i+1, ok, want)
				}
				if err != nil || !ok {
					return err
				}
				return r.h.Inbox.InsertReceivedDocument(ctx, tx, a.ActivityURI, []byte(`{"id":"`+string(a.ActivityURI)+`"}`))
			})
		}
		return a
	}
	signed, refetched := record("1", federation.TrustSigned), record("2", federation.TrustRefetch)

	if got, err := r.h.Inbox.FindReceivedActivity(ctx, signed.ActivityURI); err != nil || got.ActivityURI != signed.ActivityURI || got.Trust != signed.Trust || !got.ReceivedAt.Equal(signed.ReceivedAt) {
		t.Errorf("FindReceivedActivity = %+v, %v; want %+v", got, err, signed)
	}
	if doc, err := r.h.Inbox.FindReceivedDocument(ctx, refetched.ActivityURI); err != nil || len(doc) == 0 {
		t.Errorf("FindReceivedDocument = %s, %v; want the document", doc, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Inbox.KeptReceivedActivityTx(ctx, tx, signed.ActivityURI); err != nil || got.ActivityURI != signed.ActivityURI {
			t.Errorf("KeptReceivedActivityTx = %+v, %v; want %s", got, err, signed.ActivityURI)
		}
		if ok, err := r.h.Inbox.ReleaseUnconfirmedActivity(ctx, tx, signed.ActivityURI); err != nil || ok {
			t.Errorf("ReleaseUnconfirmedActivity(signed) = %v, %v; want false", ok, err)
		}
		if ok, err := r.h.Inbox.ReleaseUnconfirmedActivity(ctx, tx, refetched.ActivityURI); err != nil || !ok {
			t.Errorf("ReleaseUnconfirmedActivity(refetched) = %v, %v; want true", ok, err)
		}
		return nil
	})
	if _, err := r.h.Inbox.FindReceivedActivity(ctx, refetched.ActivityURI); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindReceivedActivity(released) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Inbox.FindReceivedActivity(ctx, signed.ActivityURI); err != nil {
		t.Errorf("FindReceivedActivity(signed) = %v, want it kept", err)
	}
	// A released activity is recorded again when it next arrives.
	record("2", federation.TrustSigned)
}

func remoteStatusRepoTombstones(t *testing.T, r *remoteStatusRepo) {
	ctx := t.Context()
	const (
		alice  = federation.IRI("https://remote.example/users/alice")
		mallet = federation.IRI("https://remote.example/users/mallet")
		note   = federation.IRI("https://remote.example/notes/1")
	)
	insert := func(actor federation.IRI, want bool) {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Inbox.InsertRemoteObjectTombstone(ctx, tx, federation.RemoteObjectTombstone{
				URI: note, ActorURI: actor, ActivityURI: actor + "/deletes/" + federation.IRI(r.ids.NewID()), ReceivedAt: r.clock.Now(),
			})
			if err == nil && ok != want {
				t.Errorf("InsertRemoteObjectTombstone by %s = %v, want %v", actor, ok, want)
			}
			return err
		})
	}
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, note, alice); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned before a tombstone = %v, %v; want false", ok, err)
	}
	insert(alice, true)
	insert(alice, false)
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, note, alice); err != nil || !ok {
		t.Errorf("RemoteObjectTombstoned(its actor) = %v, %v; want true", ok, err)
	}
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, note, mallet); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned(another actor) = %v, %v; want false", ok, err)
	}
	insert(mallet, true)
}
