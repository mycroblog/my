// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InboxContract verifies the methods of the inbox repository that the synchronous part of the
// inbox records through, by those methods alone, the port contract of ADR-0153 §1: a received
// activity is recorded once per activity URI with every field; its document reads back with the
// bytes it was stored with; lookups of unknown activities and documents fail with ErrNotFound;
// and KeptReceivedActivityTx finds an activity inside a transaction only while its document is
// kept. The inbox on top of it is InboxFlows, which internal/flowtest/federation runs once on
// SQLite. Only the UnitOfWork and the Inbox of the harness are used.
func InboxContract(t *testing.T, newHarness InboxFactory) {
	t.Helper()
	inboxRepoRun(t, newHarness, []inboxRepoCase{
		{"RecordsAnActivityOnce", inboxRepoRecordsOnce},
		{"KeepsTheDocumentByteExact", inboxRepoKeepsDocumentByteExact},
		{"ReadsAKeptActivityInATransaction", inboxRepoReadsKeptActivity},
	})
}

// InboxProcessingContract verifies the methods of the inbox repository that the asynchronous
// part of the inbox uses, by those methods alone, the port contract of ADR-0153 §1: an activity
// of trust refetch is released with its document, so that it is recorded again, while one of
// trust signed stays; and a tombstone of a remote object is recorded once per actor and URI and
// holds back only the objects of its actor. The processing on top of it is InboxProcessingFlows,
// which internal/flowtest/federation runs once on SQLite; RemoteFollowContract checks the
// tombstones of follows. Only the UnitOfWork and the Inbox of the harness are used.
func InboxProcessingContract(t *testing.T, newHarness InboxFactory) {
	t.Helper()
	inboxRepoRun(t, newHarness, []inboxRepoCase{
		{"ReleasesOnlyUnconfirmedActivities", inboxRepoReleasesUnconfirmed},
		{"RecordsTombstonesOncePerActor", inboxRepoRecordsTombstonesOnce},
	})
}

type inboxRepoCase struct {
	name string
	run  func(t *testing.T, r *inboxRepo)
}

func inboxRepoRun(t *testing.T, newHarness InboxFactory, cases []inboxRepoCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			tc.run(t, &inboxRepo{h: h, clock: NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))})
		})
	}
}

type inboxRepo struct {
	h     InboxHarness
	clock *Clock
}

func (r *inboxRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// activity returns a received activity of alice named n, not recorded.
func (r *inboxRepo) activity(n string, trust federation.Trust) federation.ReceivedActivity {
	return federation.ReceivedActivity{
		ActivityURI: inboxAlice + "/activities/" + federation.IRI(n), ActorURI: inboxAlice, Type: "Create",
		Scheme: "cavage", Trust: trust, ReceivedAt: r.clock.Now(),
	}
}

// record inserts a and, when it is recorded, its document doc (none when nil), and reports
// whether it was recorded.
func (r *inboxRepo) record(t *testing.T, a federation.ReceivedActivity, doc []byte) bool {
	t.Helper()
	var recorded bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if recorded, err = r.h.Inbox.InsertReceivedActivity(ctx, tx, a); err != nil || !recorded || doc == nil {
			return err
		}
		return r.h.Inbox.InsertReceivedDocument(ctx, tx, a.ActivityURI, doc)
	})
	return recorded
}

func (r *inboxRepo) kept(t *testing.T, uri federation.IRI) (a federation.ReceivedActivity, err error) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		a, err = r.h.Inbox.KeptReceivedActivityTx(ctx, tx, uri)
		return nil
	})
	return a, err
}

func equalReceivedActivity(a, b federation.ReceivedActivity) bool {
	at, bt := a.ReceivedAt, b.ReceivedAt
	a.ReceivedAt, b.ReceivedAt = time.Time{}, time.Time{}
	return a == b && at.Equal(bt)
}

func inboxRepoRecordsOnce(t *testing.T, r *inboxRepo) {
	ctx := t.Context()
	a := r.activity("1", federation.TrustSigned)
	doc := []byte(`{"id":"` + string(a.ActivityURI) + `"}`)
	if !r.record(t, a, doc) {
		t.Fatal("InsertReceivedActivity = false, want the first recorded")
	}
	again := a
	again.Type, again.Trust, again.ReceivedAt = "Update", federation.TrustRefetch, r.clock.Now().Add(time.Minute)
	if r.record(t, again, nil) {
		t.Error("InsertReceivedActivity of a recorded URI = true, want false")
	}
	if got, err := r.h.Inbox.FindReceivedActivity(ctx, a.ActivityURI); err != nil || !equalReceivedActivity(got, a) {
		t.Errorf("FindReceivedActivity = %+v, %v; want the first %+v", got, err, a)
	}
	if got, err := r.h.Inbox.FindReceivedDocument(ctx, a.ActivityURI); err != nil || !bytes.Equal(got, doc) {
		t.Errorf("FindReceivedDocument = %s, %v; want %s", got, err, doc)
	}
	refetch := r.activity("2", federation.TrustRefetch)
	refetch.Scheme = "rfc9421"
	r.record(t, refetch, nil)
	if got, err := r.h.Inbox.FindReceivedActivity(ctx, refetch.ActivityURI); err != nil || !equalReceivedActivity(got, refetch) {
		t.Errorf("FindReceivedActivity(refetch) = %+v, %v; want %+v", got, err, refetch)
	}
	unknown := federation.IRI(inboxAlice + "/activities/unknown")
	if _, err := r.h.Inbox.FindReceivedActivity(ctx, unknown); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindReceivedActivity(unknown) = %v, want ErrNotFound", err)
	}
	for _, uri := range []federation.IRI{unknown, refetch.ActivityURI} {
		if _, err := r.h.Inbox.FindReceivedDocument(ctx, uri); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("FindReceivedDocument(%s, none stored) = %v, want ErrNotFound", uri, err)
		}
	}
}

// inboxRepoKeepsDocumentByteExact checks that a document reads back with the bytes it was stored
// with, whatever a JSON column would change: whitespace, key order, a duplicate key, escapes,
// and an escaped NUL or lone surrogate, which a PostgreSQL jsonb column refuses.
func inboxRepoKeepsDocumentByteExact(t *testing.T, r *inboxRepo) {
	a := r.activity("raw", federation.TrustSigned)
	doc := []byte(strings.NewReplacer("ID", string(a.ActivityURI), "ACTOR", inboxAlice).Replace(`{ "type":"Create",
	"id": "ID", "actor":"ACTOR", "@context": "https://www.w3.org/ns/activitystreams",
  "object": {"type": "Note", "id": "ACTOR/notes/raw", "content": "first", "content": "second",
    "summary": "nul \u0000, lone \ud800, é vs é, \/"},
  "zz": 1.50, "aa": 1e2 }
`))
	r.record(t, a, doc)
	if got, err := r.h.Inbox.FindReceivedDocument(t.Context(), a.ActivityURI); err != nil || !bytes.Equal(got, doc) {
		t.Errorf("FindReceivedDocument = %q, %v; want %q", got, err, doc)
	}
}

// inboxRepoReadsKeptActivity checks KeptReceivedActivityTx: ErrNotFound for an unknown activity
// and for one without its document, the activity while its document is kept, and ErrNotFound
// again once the document is purged.
func inboxRepoReadsKeptActivity(t *testing.T, r *inboxRepo) {
	if _, err := r.kept(t, inboxAlice+"/activities/unknown"); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("KeptReceivedActivityTx(unknown) = %v, want ErrNotFound", err)
	}
	bare := r.activity("bare", federation.TrustSigned)
	r.record(t, bare, nil)
	if _, err := r.kept(t, bare.ActivityURI); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("KeptReceivedActivityTx(no document) = %v, want ErrNotFound", err)
	}
	a := r.activity("kept", federation.TrustSigned)
	r.record(t, a, []byte(`{}`))
	if got, err := r.kept(t, a.ActivityURI); err != nil || !equalReceivedActivity(got, a) {
		t.Errorf("KeptReceivedActivityTx = %+v, %v; want %+v", got, err, a)
	}
	purge, ok := r.h.Inbox.(interface {
		DeleteReceivedDocumentsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	})
	if !ok {
		t.Fatalf("%T does not purge received documents", r.h.Inbox)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := purge.DeleteReceivedDocumentsBefore(ctx, tx, r.clock.Now().Add(time.Hour), 10)
		return err
	})
	if _, err := r.kept(t, a.ActivityURI); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("KeptReceivedActivityTx(document purged) = %v, want ErrNotFound", err)
	}
}

func inboxRepoReleasesUnconfirmed(t *testing.T, r *inboxRepo) {
	ctx := t.Context()
	signed, refetched := r.activity("signed", federation.TrustSigned), r.activity("refetched", federation.TrustRefetch)
	r.record(t, signed, []byte(`{"signed":true}`))
	r.record(t, refetched, []byte(`{"signed":false}`))
	release := func(uri federation.IRI) bool {
		t.Helper()
		var released bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			released, err = r.h.Inbox.ReleaseUnconfirmedActivity(ctx, tx, uri)
			return err
		})
		return released
	}
	if release(signed.ActivityURI) {
		t.Error("ReleaseUnconfirmedActivity(signed) = true, want false")
	}
	if release(inboxAlice + "/activities/unknown") {
		t.Error("ReleaseUnconfirmedActivity(unknown) = true, want false")
	}
	if !release(refetched.ActivityURI) {
		t.Error("ReleaseUnconfirmedActivity(refetch) = false, want true")
	}
	if release(refetched.ActivityURI) {
		t.Error("ReleaseUnconfirmedActivity(released) = true, want false")
	}
	if _, err := r.h.Inbox.FindReceivedActivity(ctx, refetched.ActivityURI); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindReceivedActivity(released) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Inbox.FindReceivedDocument(ctx, refetched.ActivityURI); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindReceivedDocument(released) = %v, want ErrNotFound", err)
	}
	if got, err := r.h.Inbox.FindReceivedDocument(ctx, signed.ActivityURI); err != nil || string(got) != `{"signed":true}` {
		t.Errorf("FindReceivedDocument(signed) = %s, %v; want it kept", got, err)
	}
	// A released activity is recorded again, with its new document, when it next arrives.
	refetched.Trust = federation.TrustSigned
	if !r.record(t, refetched, []byte(`{"again":true}`)) {
		t.Error("InsertReceivedActivity after the release = false, want it recorded again")
	}
	if got, err := r.h.Inbox.FindReceivedDocument(ctx, refetched.ActivityURI); err != nil || string(got) != `{"again":true}` {
		t.Errorf("FindReceivedDocument after the release = %s, %v; want the new document", got, err)
	}
}

func inboxRepoRecordsTombstonesOnce(t *testing.T, r *inboxRepo) {
	ctx := t.Context()
	note := federation.IRI(inboxAlice + "/notes/gone")
	first := federation.RemoteObjectTombstone{
		URI: note, ActorURI: inboxAlice, ActivityURI: inboxAlice + "/activities/delete-1", ReceivedAt: r.clock.Now(),
	}
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, note, inboxAlice); err != nil || ok {
		t.Fatalf("RemoteObjectTombstoned before = %t, %v; want false", ok, err)
	}
	insert := func(ts federation.RemoteObjectTombstone) bool {
		t.Helper()
		var inserted bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			inserted, err = r.h.Inbox.InsertRemoteObjectTombstone(ctx, tx, ts)
			return err
		})
		return inserted
	}
	// Another actor of the same origin records a tombstone of the URI first: it holds back only
	// that actor's objects, and the owner's own tombstone of the URI is still recorded.
	carol := federation.IRI("https://remote.example/users/carol")
	if !insert(federation.RemoteObjectTombstone{
		URI: note, ActorURI: carol, ActivityURI: carol + "/activities/delete-1", ReceivedAt: r.clock.Now(),
	}) {
		t.Error("InsertRemoteObjectTombstone(another actor) = false, want it recorded")
	}
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, note, inboxAlice); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned after another actor's tombstone = %t, %v; want false", ok, err)
	}
	if !insert(first) {
		t.Error("InsertRemoteObjectTombstone = false, want the first tombstone of the owner recorded")
	}
	again := first
	again.ActivityURI = inboxAlice + "/activities/delete-2"
	again.ReceivedAt = r.clock.Now().Add(time.Minute)
	if insert(again) {
		t.Error("InsertRemoteObjectTombstone again = true, want the first tombstone to stay")
	}
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, note, inboxAlice); err != nil || !ok {
		t.Errorf("RemoteObjectTombstoned = %t, %v; want true", ok, err)
	}
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, inboxAlice+"/notes/other", inboxAlice); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned(other) = %t, %v; want false", ok, err)
	}
	// The tombstone holds back only what its actor owns.
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, note, "https://remote.example/users/dave"); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned(another actor) = %t, %v; want false", ok, err)
	}
}
