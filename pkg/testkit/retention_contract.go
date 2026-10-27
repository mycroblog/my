// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// RetentionHarness is one database under test for the purges of federated data: the harness of
// the remote status contract, plus the interaction and federation repositories and the
// repositories of the purges on the same database.
type RetentionHarness struct {
	RemoteStatusHarness
	Interactions appcontent.InteractionRepository
	Federation   appfederation.Repository
	Retention    appcontent.RetentionRepository
	Purges       appfederation.PurgeRepository
	Profiles     appaccount.ProfilePruneRepository
	Emojis       EmojiPurgeStore
}

// EmojiPurgeStore is the content module's custom emoji repository, which the purge of remote
// custom emoji uses.
type EmojiPurgeStore interface {
	appcontent.CustomEmojiRepository
	appcontent.EmojiPurgeRepository
}

// RetentionFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type RetentionFactory func(t *testing.T, opts uow.Options) RetentionHarness

// RetentionContract verifies the purges of federated data (ADR-0034, FR-803) on a database
// adapter, with the real content, account, graph, timeline, notification, and federation
// modules: remote statuses past remote_status_retention that nothing local refers to are
// deleted with origin retention and their rows purged, while those a local reply, thread, boost,
// favourite, bookmark, or notification refers to stay; past remote_status_max_retention every
// remote status goes, leaving the local accounts' interactions, except those that the moderation
// records retain (a reported status, a held author); each transaction deletes a
// bounded batch; a purged status can be fetched and deleted again; the inbox and delivery records
// and the tombstones of unknown objects and Follows go after their retention; unrelated remote accounts keep only their current profile;
// and the remote custom emoji that no status, profile, or reaction refers to any more go with the
// remote media of their images, while the referenced and the disabled ones stay, and a disabled
// one stays disabled when it arrives again. The statuses that only a relay brought in go after
// relay_status_retention unless something local refers to them or a local account follows their
// author (ADR-0061).
func RetentionContract(t *testing.T, newHarness RetentionFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *retentionEnv)
	}{
		{"PurgesUnreferencedRemoteStatuses", retentionPurgesUnreferenced},
		{"KeepsReferencedRemoteStatuses", retentionKeepsReferenced},
		{"PurgesEveryRemoteStatusPastTheMaximum", retentionMaximum},
		{"KeepsTheEvidenceOfModeration", retentionKeepsModerationEvidence},
		{"PurgesInBoundedBatches", retentionBatches},
		{"IngestsAPurgedStatusAgain", retentionRefetch},
		{"PurgesInboxAndDeliveryRecords", retentionFederationRecords},
		{"PrunesProfilesOfUnrelatedRemoteAccounts", retentionProfiles},
		{"PurgesUnreferencedRemoteEmoji", retentionEmojis},
		{"PurgesRelayedStatuses", retentionRelayed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRetentionEnv(t, newHarness)) })
	}
}

type retentionEnv struct {
	*remoteStatusEnv
	rh           RetentionHarness
	interactions *appcontent.Interactions
}

func newRetentionEnv(t *testing.T, newHarness RetentionFactory) *retentionEnv {
	t.Helper()
	var (
		rh       RetentionHarness
		captured uow.Options
	)
	e := newRemoteStatusEnv(t, func(t *testing.T, opts uow.Options) RemoteStatusHarness {
		captured = opts
		rh = newHarness(t, opts)
		return rh.RemoteStatusHarness
	})
	appcontent.RegisterInteractionProjectors(captured.Projectors, rh.Interactions)
	interactions := appcontent.NewInteractions(appcontent.InteractionDeps{
		UnitOfWork: rh.UnitOfWork, Statuses: rh.Statuses, Interactions: rh.Interactions,
		Accounts: e.accounts, Relationships: e.graph, Clock: e.clock, IDs: e.ids, Host: contractHost,
	})
	return &retentionEnv{remoteStatusEnv: e, rh: rh, interactions: interactions}
}

// retention returns the purge of remote statuses with the retention period and maximum.
func (e *retentionEnv) retention(period, maxPeriod time.Duration, batch int) *appcontent.Retention {
	return e.retentionWith(period, maxPeriod, batch, nil)
}

// retentionWith is retention with the moderation records' evidence (nil: none).
func (e *retentionEnv) retentionWith(period, maxPeriod time.Duration, batch int, evidence appcontent.RetainedStatuses) *appcontent.Retention {
	return appcontent.NewRetention(appcontent.RetentionDeps{
		UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Statuses, Retention: e.rh.Retention, Notifications: e.notifications,
		Evidence: evidence, Clock: e.clock, Period: period, MaxPeriod: maxPeriod, Batch: batch,
	})
}

// stubEvidence stands in for the moderation module: the statuses that a report or an action
// points at, and the held authors.
type stubEvidence struct{ statuses, authors map[string]bool }

// StatusRetained implements appcontent.RetainedStatuses.
func (s stubEvidence) StatusRetained(_ context.Context, _ uow.Tx, statusID, authorAccountID string) (bool, error) {
	return s.statuses[statusID] || s.authors[authorAccountID], nil
}

func retentionKeepsModerationEvidence(t *testing.T, e *retentionEnv) {
	const alice, bob = "https://remote.example/users/alice", "https://remote.example/users/bob"
	aliceID, bobID := e.actor(t, alice), e.actor(t, bob)
	reader := e.user(t, "reader")
	e.follow(t, reader, aliceID)
	e.follow(t, reader, bobID)
	reported := e.note(t, alice, alice+"/notes/reported", 400*24*time.Hour, nil)
	held := e.note(t, bob, bob+"/notes/held", 400*24*time.Hour, nil)
	plain := e.note(t, alice, alice+"/notes/plain", 400*24*time.Hour, nil)
	plainRev := e.post(t, plain.URI).Revision.ID
	evidence := stubEvidence{statuses: map[string]bool{reported.ID: true}, authors: map[string]bool{bobID: true}}
	// Even past the maximum period, the evidence stays.
	if n := e.purgeAll(t, e.retentionWith(retentionPeriod, 180*24*time.Hour, 0, evidence)); n != 1 {
		t.Errorf("purged %d statuses, want 1", n)
	}
	e.purged(t, plain, plainRev)
	e.kept(t, reported)
	e.kept(t, held)
	// Once the records are gone, so are the statuses.
	if n := e.purgeAll(t, e.retentionWith(retentionPeriod, 180*24*time.Hour, 0, stubEvidence{})); n != 2 {
		t.Errorf("purged %d statuses after the records went, want 2", n)
	}
}

// purgeAll runs the purge until it reaches the end and returns how many statuses it purged.
func (e *retentionEnv) purgeAll(t *testing.T, r *appcontent.Retention) int {
	t.Helper()
	total, after := 0, ""
	for range 100 {
		next, n, err := r.Run(t.Context(), after)
		if err != nil {
			t.Fatalf("Run(%q) = %v", after, err)
		}
		total += n
		if next == "" {
			return total
		}
		after = next
	}
	t.Fatal("the purge did not reach the end")
	return total
}

// note delivers the Create of a public note of author published age ago, with fields added,
// and returns the stored status.
func (e *retentionEnv) note(t *testing.T, author, id string, age time.Duration, fields map[string]any) content.Status {
	t.Helper()
	e.expect(t, remoteActivity(t, "Create", id+"/activity", author, remoteNote(id, author, remoteNow.Add(-age), fields)), appfederation.InboxHandled)
	return e.post(t, id).Status
}

// purged fails unless the status is gone with a deletion by retention and its revisions purged.
func (e *retentionEnv) purged(t *testing.T, st content.Status, revisionID string) {
	t.Helper()
	ctx := t.Context()
	if e.stored(t, st.URI) {
		t.Errorf("status %s is still stored", st.URI)
	}
	d, err := e.rh.Statuses.FindDeletion(ctx, st.ID)
	if err != nil || d.Origin != content.DeletedByRetention || d.URI != st.URI {
		t.Errorf("FindDeletion(%s) = %+v, %v; want a deletion by retention", st.URI, d, err)
	}
	if tomb, err := e.content.Tombstoned(ctx, st.URI); err != nil || tomb {
		t.Errorf("Tombstoned(%s) = %v, %v; want false (a purged status may come back)", st.URI, tomb, err)
	}
	if _, err := e.rh.Statuses.FindRevision(ctx, revisionID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindRevision(%s) = %v; want ErrNotFound", revisionID, err)
	}
}

// kept fails unless the status is stored and not deleted.
func (e *retentionEnv) kept(t *testing.T, st content.Status) {
	t.Helper()
	if !e.stored(t, st.URI) {
		t.Errorf("status %s is gone, want it kept", st.URI)
		return
	}
	if _, err := e.rh.Statuses.FindDeletion(t.Context(), st.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindDeletion(%s) = %v; want ErrNotFound", st.URI, err)
	}
}

const retentionPeriod = 90 * 24 * time.Hour

func retentionPurgesUnreferenced(t *testing.T, e *retentionEnv) {
	const alice = "https://remote.example/users/alice"
	aliceID := e.actor(t, alice)
	reader := e.user(t, "reader")
	e.follow(t, reader, aliceID)
	old := e.note(t, alice, alice+"/notes/old", 100*24*time.Hour, map[string]any{
		"tag":        []any{map[string]any{"type": "Hashtag", "name": "#retention"}},
		"attachment": []any{map[string]any{"type": "Document", "mediaType": "image/png", "url": "https://remote.example/media/1.png"}},
	})
	oldRev := e.post(t, old.URI).Revision
	poll := alice + "/questions/old"
	e.expect(t, remoteActivity(t, "Create", poll+"/activity", alice, remoteNote(poll, alice, remoteNow.Add(-95*24*time.Hour), map[string]any{
		"type": "Question", "oneOf": []any{
			map[string]any{"type": "Note", "name": "yes", "replies": map[string]any{"type": "Collection", "totalItems": 3}},
			map[string]any{"type": "Note", "name": "no", "replies": map[string]any{"type": "Collection", "totalItems": 1}},
		},
	})), appfederation.InboxHandled)
	question := e.post(t, poll)
	if question.Revision.Poll == nil {
		t.Fatalf("the question has no poll: %+v", question.Revision)
	}
	recent := e.note(t, alice, alice+"/notes/recent", 10*24*time.Hour, nil)

	if n := e.purgeAll(t, e.retention(retentionPeriod, 0, 0)); n != 2 {
		t.Errorf("purged %d statuses, want 2", n)
	}
	e.purged(t, old, oldRev.ID)
	e.purged(t, question.Status, question.Revision.ID)
	e.kept(t, recent)
	if tallies, err := e.content.PollTallies(t.Context(), question.Revision.Poll.ID); err != nil || len(tallies) != 0 {
		t.Errorf("PollTallies of the purged poll = %+v, %v; want none", tallies, err)
	}
	if posts, err := e.h.Statuses.FindPostCounter(t.Context(), aliceID); err != nil || posts.StatusesCount != 1 {
		t.Errorf("post counter of alice = %+v, %v; want 1", posts, err)
	}
	home, err := e.timeline.Home(t.Context(), appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: reader}, apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil || slices.ContainsFunc(home.Entries, func(en apptimeline.Entry) bool { return en.StatusID == old.ID || en.StatusID == question.Status.ID }) {
		t.Errorf("home of the follower = %+v, %v; want the purged statuses gone", home.Entries, err)
	}
	// Running again finds nothing more.
	if n := e.purgeAll(t, e.retention(retentionPeriod, 0, 0)); n != 0 {
		t.Errorf("the second run purged %d statuses, want 0", n)
	}
}

// referencedStatuses posts the remote statuses that something local refers to, all published
// 200 days ago, and returns them by what refers to them, with the local account that replied,
// boosted, favourited, and bookmarked, and the one mentioned.
func (e *retentionEnv) referencedStatuses(t *testing.T) (statuses map[string]content.Status, reader, mentioned string) {
	t.Helper()
	ctx := t.Context()
	const alice = "https://remote.example/users/alice"
	aliceID := e.actor(t, alice)
	reader, mentioned = e.user(t, "reader"), e.user(t, "mentioned")
	e.follow(t, reader, aliceID)
	age := 200 * 24 * time.Hour
	out := map[string]content.Status{}
	for _, name := range []string{"reply", "thread", "boost", "favourite", "bookmark", "unrelated"} {
		out[name] = e.note(t, alice, alice+"/notes/"+name, age, nil)
	}
	out["mention"] = e.note(t, alice, alice+"/notes/mention", age, map[string]any{
		"cc":  []string{alice + "/followers", e.localIRI(mentioned)},
		"tag": []any{map[string]any{"type": "Mention", "href": e.localIRI(mentioned)}},
	})
	// A remote reply in the thread of a remote post that a local account replied to.
	out["in-thread"] = e.note(t, alice, alice+"/notes/in-thread", age, map[string]any{"inReplyTo": out["thread"].URI})
	for _, name := range []string{"reply", "thread"} {
		if _, err := e.content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: reader, TextInput: appcontent.TextInput{Text: "re"}, Visibility: content.VisibilityPublic, InReplyToID: out[name].ID,
		}); err != nil {
			t.Fatalf("reply to %s: %v", name, err)
		}
	}
	if _, err := e.interactions.Boost(ctx, reader, out["boost"].ID); err != nil {
		t.Fatalf("Boost: %v", err)
	}
	if _, err := e.interactions.Favourite(ctx, reader, out["favourite"].ID); err != nil {
		t.Fatalf("Favourite: %v", err)
	}
	if err := e.interactions.Bookmark(ctx, reader, out["bookmark"].ID); err != nil {
		t.Fatalf("Bookmark: %v", err)
	}
	return out, reader, mentioned
}

func retentionKeepsReferenced(t *testing.T, e *retentionEnv) {
	statuses, reader, _ := e.referencedStatuses(t)
	// A boost that the local account retracted no longer refers to its status.
	retracted := e.note(t, "https://remote.example/users/alice", "https://remote.example/users/alice/notes/retracted", 200*24*time.Hour, nil)
	retractedRev := e.post(t, retracted.URI).Revision
	if _, err := e.interactions.Boost(t.Context(), reader, retracted.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.interactions.Unboost(t.Context(), reader, retracted.ID); err != nil {
		t.Fatal(err)
	}
	unrelatedRev := e.post(t, statuses["unrelated"].URI).Revision

	if n := e.purgeAll(t, e.retention(retentionPeriod, 0, 0)); n != 2 {
		t.Errorf("purged %d statuses, want 2 (the unrelated one and the one of the retracted boost)", n)
	}
	e.purged(t, statuses["unrelated"], unrelatedRev.ID)
	e.purged(t, retracted, retractedRev.ID)
	for name, st := range statuses {
		if name == "unrelated" {
			continue
		}
		t.Run(name, func(t *testing.T) { e.kept(t, st) })
	}
}

func retentionMaximum(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	statuses, reader, mentioned := e.referencedStatuses(t)
	recent := e.note(t, "https://remote.example/users/alice", "https://remote.example/users/alice/notes/recent", 100*24*time.Hour, nil)
	if err := e.interactions.Bookmark(ctx, reader, recent.ID); err != nil {
		t.Fatal(err)
	}
	revisions := map[string]string{}
	for name, st := range statuses {
		revisions[name] = e.post(t, st.URI).Revision.ID
	}

	if n := e.purgeAll(t, e.retention(retentionPeriod, 180*24*time.Hour, 0)); n != len(statuses) {
		t.Errorf("purged %d statuses, want %d", n, len(statuses))
	}
	for name, st := range statuses {
		t.Run(name, func(t *testing.T) { e.purged(t, st, revisions[name]) })
	}
	e.kept(t, recent)
	// The local accounts' interactions stay, and their statuses read as deleted.
	if _, err := e.rh.Interactions.FindFavourite(ctx, reader, statuses["favourite"].ID); err != nil {
		t.Errorf("FindFavourite of the purged status = %v; want the favourite kept", err)
	}
	if _, err := e.rh.Interactions.FindBookmark(ctx, reader, statuses["bookmark"].ID); err != nil {
		t.Errorf("FindBookmark of the purged status = %v; want the bookmark kept", err)
	}
	marks, err := e.rh.Interactions.ListBookmarks(ctx, reader, "", 10)
	if err != nil || len(marks) != 2 {
		t.Errorf("bookmarks = %+v, %v; want both kept", marks, err)
	}
	page, err := e.statusReader().Bookmarks(ctx, reader, "", 10)
	if err != nil || len(page.Statuses) != 1 || page.Statuses[0].Status.ID != recent.ID {
		t.Errorf("bookmarked statuses = %+v, %v; want only the kept one (the purged one reads as deleted)", page.Statuses, err)
	}
	ns, err := e.notifications.List(ctx, mentioned, appnotification.Query{Limit: appnotification.MaxPageLimit})
	if err != nil || len(ns) != 0 {
		t.Errorf("notifications of the mentioned account = %+v, %v; want the mention hidden", ns, err)
	}
}

func retentionBatches(t *testing.T, e *retentionEnv) {
	const alice = "https://remote.example/users/alice"
	aliceID := e.actor(t, alice)
	e.follow(t, e.user(t, "reader"), aliceID)
	const total, batch = 25, 2
	for i := range total {
		e.note(t, alice, fmt.Sprintf("%s/notes/%d", alice, i), 100*24*time.Hour+time.Duration(i)*time.Minute, nil)
	}
	r := e.retention(retentionPeriod, 0, batch)
	next, n, err := r.Run(t.Context(), "")
	// One run has a bounded number of batches of batch statuses each.
	if err != nil || next == "" || n == 0 || n%batch != 0 || n >= total {
		t.Fatalf("Run = %q, %d, %v; want a cursor and part of the %d statuses in batches of %d", next, n, err, total, batch)
	}
	if rest := e.purgeAll(t, r); rest != total-n {
		t.Errorf("the later runs purged %d statuses, want %d", rest, total-n)
	}
	if posts, err := e.h.Statuses.FindPostCounter(t.Context(), aliceID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("post counter of alice = %+v, %v; want none", posts, err)
	}
}

func retentionRefetch(t *testing.T, e *retentionEnv) {
	const alice = "https://remote.example/users/alice"
	aliceID := e.actor(t, alice)
	e.follow(t, e.user(t, "reader"), aliceID)
	id := alice + "/notes/1"
	first := e.note(t, alice, id, 100*24*time.Hour, nil)
	if n := e.purgeAll(t, e.retention(retentionPeriod, 0, 0)); n != 1 {
		t.Fatalf("purged %d statuses, want 1", n)
	}
	// The same post arrives again: a deletion by retention does not keep it out.
	e.expect(t, remoteActivity(t, "Create", id+"/activity-again", alice, remoteNote(id, alice, remoteNow.Add(-100*24*time.Hour), nil)), appfederation.InboxHandled)
	again := e.post(t, id).Status
	if again.ID == first.ID {
		t.Errorf("the status came back with its purged ID %s", again.ID)
	}
	// Its author's Delete still deletes it into a tombstone.
	e.expect(t, remoteActivity(t, "Delete", id+"#delete", alice, id), appfederation.InboxHandled)
	if tomb, err := e.content.Tombstoned(t.Context(), id); err != nil || !tomb {
		t.Errorf("Tombstoned after the Delete = %v, %v; want true", tomb, err)
	}
	d, err := e.rh.Statuses.FindDeletionByURI(t.Context(), id)
	if err != nil || d.Origin != content.DeletedByRemote || d.StatusID != again.ID {
		t.Errorf("FindDeletionByURI = %+v, %v; want the author's deletion of %s", d, err, again.ID)
	}
	// A Delete of a purged post, which no status has now, leaves a tombstone for a late Create.
	other := alice + "/notes/2"
	e.note(t, alice, other, 100*24*time.Hour, nil)
	if n := e.purgeAll(t, e.retention(retentionPeriod, 0, 0)); n != 1 {
		t.Fatalf("purged %d statuses, want 1", n)
	}
	e.expect(t, remoteActivity(t, "Delete", other+"#delete", alice, other), appfederation.InboxTombstoned)
}

func retentionFederationRecords(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	local := e.user(t, "sender")
	type record struct {
		received          federation.IRI
		outboundID        string
		delivery, attempt string
	}
	day := 24 * time.Hour
	// write records the inbox and delivery rows of one age.
	write := func(name string, age time.Duration) record {
		t.Helper()
		at := remoteNow.Add(-age)
		e.clock.Set(at)
		defer e.clock.Set(remoteNow)
		r := record{received: federation.IRI("https://remote.example/activities/" + name)}
		act := federation.OutboundActivity{
			ID: e.ids.NewID(), Type: federation.ActivityCreate, ActorAccountID: local,
			ObjectKind: federation.OutboundStatus, ObjectID: e.ids.NewID(), CreatedAt: at,
		}
		act.ActivityURI = federation.IRI("https://" + contractHost + "/ap/activities/" + act.ID)
		r.outboundID = act.ID
		del := federation.Delivery{ID: e.ids.NewID(), OutboundActivityID: act.ID, InboxURL: "https://remote.example/inbox", Domain: "remote.example", CreatedAt: at}
		attempt := federation.DeliveryAttempt{ID: e.ids.NewID(), DeliveryID: del.ID, Outcome: federation.DeliverySucceeded, Scheme: "cavage", HTTPStatus: 202, AttemptedAt: at}
		r.delivery, r.attempt = del.ID, attempt.ID
		err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			if _, err := e.rh.Inbox.InsertReceivedActivity(ctx, tx, federation.ReceivedActivity{
				ActivityURI: r.received, ActorURI: "https://remote.example/users/alice", Type: "Like", Scheme: "cavage", Trust: federation.TrustSigned, ReceivedAt: at,
			}); err != nil {
				return err
			}
			if err := e.rh.Inbox.InsertReceivedDocument(ctx, tx, r.received, []byte(`{"id":"`+string(r.received)+`"}`)); err != nil {
				return err
			}
			if _, err := e.rh.Inbox.InsertRemoteObjectTombstone(ctx, tx, federation.RemoteObjectTombstone{
				URI: r.received + "/object", ActorURI: "https://remote.example/users/alice", ActivityURI: r.received, ReceivedAt: at,
			}); err != nil {
				return err
			}
			if _, err := e.rh.Inbox.InsertRemoteFollowTombstone(ctx, tx, federation.RemoteFollowTombstone{
				ActivityURI: r.received + "#undo", ActorURI: "https://remote.example/users/alice", ObjectURI: r.received + "/followee", ReceivedAt: at,
			}); err != nil {
				return err
			}
			if err := e.rh.Federation.InsertOutboundActivity(ctx, tx, act); err != nil {
				return err
			}
			if _, err := e.rh.Federation.InsertOutboundDocument(ctx, tx, act.ID, []byte(`{"id":"`+string(act.ActivityURI)+`"}`), at); err != nil {
				return err
			}
			if _, err := e.rh.Federation.InsertDelivery(ctx, tx, del); err != nil {
				return err
			}
			return e.rh.Federation.InsertDeliveryAttempt(ctx, tx, attempt)
		})
		if err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
		return r
	}
	fresh, week, month := write("fresh", day), write("week", 8*day), write("month", 31*day)
	// A stale tombstone that another actor left for the URI of a fresh one goes alone.
	if err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.rh.Inbox.InsertRemoteObjectTombstone(ctx, tx, federation.RemoteObjectTombstone{
			URI: fresh.received + "/object", ActorURI: "https://remote.example/users/bob", ActivityURI: fresh.received + "#bob", ReceivedAt: remoteNow.Add(-31 * day),
		})
		return err
	}); err != nil {
		t.Fatalf("record another actor's tombstone: %v", err)
	}
	purger := appfederation.NewPurger(appfederation.PurgeDeps{UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Purges, Clock: e.clock, Batch: 1})
	if _, err := purger.Run(ctx); err != nil {
		t.Fatalf("Run = %v", err)
	}
	type want struct{ document, activity, tombstone, followTombstone, outbound, attempt bool }
	for _, tc := range []struct {
		name string
		r    record
		want want
	}{
		{"fresh", fresh, want{true, true, true, true, true, true}},
		{"week", week, want{false, true, true, true, true, true}},
		{"month", month, want{false, false, false, false, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got want
			_, err := e.rh.Inbox.FindReceivedDocument(ctx, tc.r.received)
			got.document = err == nil
			_, err = e.rh.Inbox.FindReceivedActivity(ctx, tc.r.received)
			got.activity = err == nil
			got.tombstone, err = e.rh.Inbox.RemoteObjectTombstoned(ctx, tc.r.received+"/object", "https://remote.example/users/alice")
			if err != nil {
				t.Fatal(err)
			}
			err = e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				_, err := e.rh.Inbox.LiveRemoteFollowTombstone(ctx, tx, "https://remote.example/users/alice", tc.r.received+"/followee")
				return err
			})
			switch {
			case err == nil:
				got.followTombstone = true
			case !errors.Is(err, appfederation.ErrNotFound):
				t.Fatal(err)
			}
			_, err = e.rh.Federation.FindOutboundDocument(ctx, tc.r.outboundID)
			got.outbound = err == nil
			attempts, err := e.rh.Federation.ListDeliveryAttempts(ctx, tc.r.delivery)
			if err != nil {
				t.Fatal(err)
			}
			got.attempt = slices.ContainsFunc(attempts, func(a federation.DeliveryAttempt) bool { return a.ID == tc.r.attempt })
			if got != tc.want {
				t.Errorf("kept %+v, want %+v", got, tc.want)
			}
		})
	}
	if ok, err := e.rh.Inbox.RemoteObjectTombstoned(ctx, fresh.received+"/object", "https://remote.example/users/bob"); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned(another actor's stale tombstone) = %t, %v; want false", ok, err)
	}
}

func retentionProfiles(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	reader := e.user(t, "reader")
	mentioned := e.user(t, "mentioned")
	save := func(uri string, n int) appaccount.Actor {
		t.Helper()
		e.actors.serve(uri)
		in := e.actors.served[uri]
		in.Name = fmt.Sprintf("name %d", n)
		in.Document = fmt.Appendf(nil, `{"id":%q,"name":"name %d"}`, uri, n)
		a, err := e.remote.SaveRemoteActor(ctx, in)
		if err != nil {
			t.Fatalf("SaveRemoteActor(%s) = %v", uri, err)
		}
		return a
	}
	accounts := map[string]string{}
	for _, name := range []string{"unrelated", "followed", "follower", "poster"} {
		uri := "https://remote.example/users/" + name
		for n := 1; n <= 3; n++ {
			accounts[name] = save(uri, n).Account.ID
		}
	}
	e.follow(t, reader, accounts["followed"])
	e.follow(t, accounts["follower"], reader)
	e.note(t, "https://remote.example/users/poster", "https://remote.example/users/poster/notes/1", time.Hour, map[string]any{
		"cc":  []string{e.localIRI(mentioned)},
		"tag": []any{map[string]any{"type": "Mention", "href": e.localIRI(mentioned)}},
	})
	before, err := e.rh.Accounts.FindLatestRemoteSnapshot(ctx, accounts["unrelated"])
	if err != nil {
		t.Fatal(err)
	}

	pruner := appaccount.NewProfilePruner(appaccount.PruneDeps{
		UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Profiles, Relations: retentionRelations{e}, Batch: 1,
	})
	after := ""
	for range 20 {
		next, _, err := pruner.Run(ctx, after)
		if err != nil {
			t.Fatalf("Run(%q) = %v", after, err)
		}
		if next == "" {
			break
		}
		after = next
	}
	listed, err := e.rh.Profiles.ListRemoteAccountsWithPastProfiles(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{accounts["followed"], accounts["follower"], accounts["poster"]}
	slices.Sort(want)
	if !slices.Equal(listed, want) {
		t.Errorf("accounts with past profiles = %v, want %v (the related ones)", listed, want)
	}
	actor, err := e.remote.FindByURI(ctx, "https://remote.example/users/unrelated")
	if err != nil || actor.Profile == nil || actor.Profile.DisplayName != "name 3" {
		t.Errorf("the pruned account = %+v, %v; want its current profile kept", actor.Profile, err)
	}
	if latest, err := e.rh.Accounts.FindLatestRemoteSnapshot(ctx, accounts["unrelated"]); err != nil || latest.ID != before.ID {
		t.Errorf("latest snapshot = %+v, %v; want %s kept", latest, err, before.ID)
	}
	// A later fetch revises the profile again.
	save("https://remote.example/users/unrelated", 4)
	if actor, err := e.remote.FindByURI(ctx, "https://remote.example/users/unrelated"); err != nil || actor.Profile.DisplayName != "name 4" || actor.Profile.Seq != 4 {
		t.Errorf("after a new fetch = %+v, %v; want name 4 at seq 4", actor.Profile, err)
	}
}

// retentionRelations relates a remote account to local data as the composition root does: a
// follow either way, or a stored status.
type retentionRelations struct{ e *retentionEnv }

func (r retentionRelations) RelatedToLocal(ctx context.Context, accountID string) (bool, error) {
	c, err := r.e.graph.Counters(ctx, accountID)
	if err != nil || c.FollowersCount > 0 || c.FollowingCount > 0 {
		return err == nil, err
	}
	n, err := r.e.content.PostCount(ctx, accountID)
	return n > 0, err
}

// statusReader returns the reader of statuses on the harness, without media.
func (e *retentionEnv) statusReader() *appcontent.Reader {
	return appcontent.NewReader(appcontent.ReaderDeps{
		Statuses: e.rh.Statuses, Interactions: e.rh.Interactions, Accounts: e.accounts, Relationships: e.graph,
	})
}

// stubFollows stands in for the graph module: the remote authors that a local account follows.
type stubFollows map[string]bool

// AuthorFollowed implements appcontent.FollowedAuthors.
func (s stubFollows) AuthorFollowed(_ context.Context, accountID string) (bool, error) {
	return s[accountID], nil
}

func retentionRelayed(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	const alice, bob = "https://remote.example/users/alice", "https://remote.example/users/bob"
	aliceID, bobID := e.actor(t, alice), e.actor(t, bob)
	reader := e.user(t, "reader")
	e.follow(t, reader, aliceID)
	e.follow(t, reader, bobID)
	old := e.note(t, alice, alice+"/notes/old", 10*24*time.Hour, nil)
	oldRev := e.post(t, old.URI).Revision.ID
	older := e.note(t, alice, alice+"/notes/older", 20*24*time.Hour, nil)
	olderRev := e.post(t, older.URI).Revision.ID
	young := e.note(t, alice, alice+"/notes/young", 3*24*time.Hour, nil)
	ofFollowed := e.note(t, bob, bob+"/notes/old", 10*24*time.Hour, nil)
	favourited := e.note(t, alice, alice+"/notes/favourited", 10*24*time.Hour, nil)
	plain := e.note(t, alice, alice+"/notes/plain", 10*24*time.Hour, nil)
	err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, st := range []content.Status{old, older, young, ofFollowed, favourited} {
			if err := e.content.RecordRelayedStatus(ctx, tx, st.ID, NamedID("relay-1"), remoteNow); err != nil {
				return err
			}
		}
		// Recording again changes nothing.
		return e.content.RecordRelayedStatus(ctx, tx, old.ID, NamedID("relay-2"), remoteNow)
	})
	if err != nil {
		t.Fatalf("RecordRelayedStatus = %v", err)
	}
	if _, err := e.interactions.Favourite(ctx, reader, favourited.ID); err != nil {
		t.Fatal(err)
	}
	r := appcontent.NewRelayedRetention(appcontent.RelayedRetentionDeps{
		UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Statuses, Retention: e.rh.Retention, Notifications: e.notifications,
		Follows: stubFollows{bobID: true}, Clock: e.clock, Period: 7 * 24 * time.Hour, Batch: 1,
	})
	total, after := 0, ""
	for range 100 {
		next, n, err := r.Run(ctx, after)
		if err != nil {
			t.Fatalf("Run(%q) = %v", after, err)
		}
		total += n
		if after = next; next == "" {
			break
		}
	}
	if total != 2 {
		t.Errorf("purged %d relayed statuses, want 2", total)
	}
	e.purged(t, old, oldRev)
	e.purged(t, older, olderRev)
	for _, st := range []content.Status{young, ofFollowed, favourited, plain} {
		e.kept(t, st)
	}
	// The purge of every remote status leaves them to their own retention.
	if n := e.purgeAll(t, e.retention(retentionPeriod, 0, 0)); n != 0 {
		t.Errorf("the retention purge took %d statuses, want 0", n)
	}
}
