// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// RetentionContract verifies the repository methods of the purges of federated data on a database
// adapter, by those methods alone, the port contract of ADR-0153 §1: the remote statuses to purge
// are listed in ID order and in pages, past the period when nothing local refers to them and past
// the maximum regardless, never the deleted ones nor local ones; so are the statuses only a relay
// brought in; purging deleted statuses removes their rows and revisions and keeps their
// deletions; the deletions by retention of purged statuses go in bounded batches; and the records
// of received activities, their documents and answers, the tombstones, and the observations of
// servers (all but the newest of each server) go after their retention, in bounded batches; the
// Creates of deleted statuses and actors go with their deliveries, attempts, documents, and
// fan-out completions, and the other outbound activities after their retention, except the
// service closure's Deletes and a Delete whose Create stays; the outbound documents and the
// delivery attempts go after theirs; so do the handle checks, all but the newest of each account;
// and the remote accounts with past profile revisions or snapshots are listed in pages and pruned
// to their current revision, its snapshot, and the head's. The purges on top of it are
// RetentionFlows, which internal/flowtest/federation runs once on SQLite; the purge of remote
// custom emoji is checked there too.
func RetentionContract(t *testing.T, newHarness RetentionFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *retentionRepo)
	}{
		{"ListsTheRemoteStatusesToPurge", retentionRepoRemoteStatuses},
		{"ListsTheRelayedStatusesToPurge", retentionRepoRelayed},
		{"PurgesDeletedStatuses", retentionRepoPurgeStatuses},
		{"DeletesTheDeletionsByRetention", retentionRepoDeletions},
		{"DeletesTheRecordsOfTheInbox", retentionRepoInbox},
		{"DeletesTheObservationsOfServers", retentionRepoObservations},
		{"PrunesThePastProfilesOfRemoteAccounts", retentionRepoProfiles},
		{"PrunesThePastSnapshotsOfRemoteAccounts", retentionRepoSnapshots},
		{"PurgesTheOutboundActivities", retentionRepoOutbound},
		{"DeletesTheBodiesOfOutboundActivities", retentionRepoOutboundBodies},
		{"DeletesTheOldHandleChecks", retentionRepoHandleChecks},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &retentionRepo{h: h, clock: clock, ids: NewIDGenerator(clock)})
		})
	}
}

type retentionRepo struct {
	h     RetentionHarness
	clock *Clock
	ids   *IDGenerator
}

func (r *retentionRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// status inserts a status published age ago, with an ID of that time (as remote statuses are
// ingested), its first revision, and its head; it replies to inReplyTo ("" for none).
func (r *retentionRepo) status(t *testing.T, local bool, age time.Duration, inReplyTo string) content.Status {
	t.Helper()
	published := r.clock.Now().Add(-age)
	id := r.ids.NewIDAt(published)
	s := content.Status{
		ID: id, URI: "https://remote.example/notes/" + id, AuthorAccountID: r.ids.NewID(),
		Visibility: content.VisibilityPublic, InReplyToURI: inReplyTo, PublishedAt: published,
	}
	if local {
		s.IsLocal, s.URI = true, "https://"+contractHost+"/statuses/"+id
	}
	rev := content.Revision{ID: r.ids.NewID(), StatusID: id, Seq: 1, RevisedAt: s.PublishedAt, Bodies: []content.Body{{ContentHTML: "<p>hi</p>"}}}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Statuses.InsertStatus(ctx, tx, s),
			r.h.Statuses.InsertRevision(ctx, tx, rev),
			r.h.Statuses.UpsertHead(ctx, tx, content.Head{StatusID: id, RevisionID: rev.ID, Seq: 1, LastRevisedAt: rev.RevisedAt}),
		)
	})
	return s
}

// deleteStatus records the deletion of s with origin now.
func (r *retentionRepo) deleteStatus(t *testing.T, s content.Status, origin content.DeletionOrigin) content.Deletion {
	t.Helper()
	d := content.Deletion{StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: origin, DeletedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Statuses.InsertDeletion(ctx, tx, d) })
	return d
}

func statusIDs(statuses []content.Status) []string {
	ids := make([]string, len(statuses))
	for i, s := range statuses {
		ids[i] = s.ID
	}
	return ids
}

func retentionRepoRemoteStatuses(t *testing.T, r *retentionRepo) {
	const day = 24 * time.Hour
	old := r.status(t, false, 10*day, "")
	referenced := r.status(t, false, 10*day, "")
	r.status(t, true, 9*day, referenced.URI)
	recent := r.status(t, false, day, "")
	deleted := r.status(t, false, 10*day, "")
	r.deleteStatus(t, deleted, content.DeletedByRemote)
	older := r.status(t, false, 20*day, "")
	r.status(t, true, 20*day, "")

	list := func(q appcontent.PurgeQuery) []string {
		var got []content.Status
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			got, err = r.h.Retention.ListRemoteStatusesToPurge(ctx, tx, q)
			return err
		})
		return statusIDs(got)
	}
	period := r.clock.Now().Add(-5 * day)
	if got, want := list(appcontent.PurgeQuery{UnreferencedBefore: period, Limit: 10}), []string{older.ID, old.ID}; !slices.Equal(got, want) {
		t.Errorf("past the period = %v, want the unreferenced %v in ID order", got, want)
	}
	if got, want := list(appcontent.PurgeQuery{UnreferencedBefore: period, Limit: 1}), []string{older.ID}; !slices.Equal(got, want) {
		t.Errorf("the first page = %v, want %v", got, want)
	}
	if got, want := list(appcontent.PurgeQuery{After: older.ID, UnreferencedBefore: period, Limit: 1}), []string{old.ID}; !slices.Equal(got, want) {
		t.Errorf("the page after the first = %v, want %v", got, want)
	}
	if got, want := list(appcontent.PurgeQuery{UnreferencedBefore: period, Before: period, Limit: 10}), []string{older.ID, old.ID, referenced.ID}; !slices.Equal(got, want) {
		t.Errorf("past the maximum = %v, want the referenced one too: %v", got, want)
	}
	if got := list(appcontent.PurgeQuery{UnreferencedBefore: r.clock.Now().Add(-30 * day), Limit: 10}); len(got) != 0 {
		t.Errorf("before every status = %v, want none (recent %s)", got, recent.ID)
	}
}

func retentionRepoRelayed(t *testing.T, r *retentionRepo) {
	const day = 24 * time.Hour
	relayID := r.ids.NewID()
	relayed := func(age time.Duration) content.Status {
		s := r.status(t, false, age, "")
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Statuses.InsertRelayedStatus(ctx, tx, s.ID, relayID, s.PublishedAt)
		})
		return s
	}
	first := relayed(10 * day)
	referenced := relayed(10 * day)
	r.status(t, true, 9*day, referenced.URI)
	relayed(day)
	second := relayed(10 * day)
	r.status(t, false, 10*day, "")
	// Recording a relayed status again changes nothing.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Statuses.InsertRelayedStatus(ctx, tx, first.ID, relayID, r.clock.Now())
	})

	list := func(q appcontent.RelayedPurgeQuery) []string {
		var got []content.Status
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			got, err = r.h.Retention.ListRelayedStatusesToPurge(ctx, tx, q)
			return err
		})
		return statusIDs(got)
	}
	before := r.clock.Now().Add(-5 * day)
	if got, want := list(appcontent.RelayedPurgeQuery{Before: before, Limit: 10}), []string{first.ID, second.ID}; !slices.Equal(got, want) {
		t.Errorf("ListRelayedStatusesToPurge = %v, want the old unreferenced relayed statuses %v", got, want)
	}
	if got, want := list(appcontent.RelayedPurgeQuery{After: first.ID, Before: before, Limit: 1}), []string{second.ID}; !slices.Equal(got, want) {
		t.Errorf("the page after the first = %v, want %v", got, want)
	}
}

func retentionRepoPurgeStatuses(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	purged := r.status(t, false, time.Hour, "")
	kept := r.status(t, false, time.Hour, "")
	deletion := r.deleteStatus(t, purged, content.DeletedByRetention)
	rev, err := r.h.Statuses.FindCurrentRevision(ctx, purged.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Retention.PurgeStatuses(ctx, tx, []string{purged.ID})
	})

	if _, err := r.h.Statuses.FindStatus(ctx, purged.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindStatus(purged) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Statuses.FindRevision(ctx, rev.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("FindRevision(of the purged status) = %v, want ErrNotFound", err)
	}
	if got, err := r.h.Statuses.FindDeletion(ctx, purged.ID); err != nil || got.Origin != deletion.Origin || got.URI != deletion.URI {
		t.Errorf("FindDeletion(purged) = %+v, %v; want the deletion %+v kept", got, err, deletion)
	}
	if _, err := r.h.Statuses.FindStatus(ctx, kept.ID); err != nil {
		t.Errorf("FindStatus(another status) = %v, want it kept", err)
	}
}

func retentionRepoDeletions(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	purge := func(origin content.DeletionOrigin) content.Deletion {
		s := r.status(t, false, time.Hour, "")
		d := r.deleteStatus(t, s, origin)
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Retention.PurgeStatuses(ctx, tx, []string{s.ID})
		})
		return d
	}
	first, second := purge(content.DeletedByRetention), purge(content.DeletedByRetention)
	byRemote := purge(content.DeletedByRemote)
	unpurged := r.deleteStatus(t, r.status(t, false, time.Hour, ""), content.DeletedByRetention)
	r.clock.Advance(time.Hour)
	recent := purge(content.DeletedByRetention)

	cutoff := r.clock.Now().Add(-time.Minute)
	for i, want := range []int64{1, 1, 0} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			n, err := r.h.Retention.DeleteRetentionDeletionsBefore(ctx, tx, cutoff, 1)
			if err == nil && n != want {
				t.Errorf("DeleteRetentionDeletionsBefore #%d = %d, want %d", i+1, n, want)
			}
			return err
		})
	}
	for _, d := range []content.Deletion{first, second} {
		if _, err := r.h.Statuses.FindDeletion(ctx, d.StatusID); !errors.Is(err, appcontent.ErrNotFound) {
			t.Errorf("FindDeletion(an old deletion by retention) = %v, want ErrNotFound", err)
		}
	}
	for name, d := range map[string]content.Deletion{"by the remote server": byRemote, "of a status not purged": unpurged, "recent": recent} {
		if _, err := r.h.Statuses.FindDeletion(ctx, d.StatusID); err != nil {
			t.Errorf("FindDeletion(the deletion %s) = %v, want it kept", name, err)
		}
	}
}

func retentionRepoInbox(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	const actor = federation.IRI("https://remote.example/users/alice")
	received := func(n string) federation.IRI {
		uri := actor + "/activities/" + federation.IRI(n)
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if _, err := r.h.Inbox.InsertReceivedActivity(ctx, tx, federation.ReceivedActivity{
				ActivityURI: uri, ActorURI: actor, Type: "Create", Scheme: "rfc9421", Trust: federation.TrustSigned, ReceivedAt: r.clock.Now(),
			}); err != nil {
				return err
			}
			if err := r.h.Inbox.InsertReceivedDocument(ctx, tx, uri, []byte(`{"type":"Create"}`)); err != nil {
				return err
			}
			if _, err := r.h.Federation.InsertReceivedActivityAnswer(ctx, tx, uri, r.clock.Now()); err != nil {
				return err
			}
			if _, err := r.h.Inbox.InsertRemoteObjectTombstone(ctx, tx, federation.RemoteObjectTombstone{
				URI: actor + "/notes/" + federation.IRI(n), ActorURI: actor, ActivityURI: uri + "/delete", ReceivedAt: r.clock.Now(),
			}); err != nil {
				return err
			}
			_, err := r.h.Inbox.InsertRemoteFollowTombstone(ctx, tx, federation.RemoteFollowTombstone{
				ActivityURI: uri, ActorURI: actor, ObjectURI: "https://" + contractHost + "/users/" + federation.IRI(n), ReceivedAt: r.clock.Now(),
			})
			return err
		})
		return uri
	}
	first, second := received("1"), received("2")
	r.clock.Advance(time.Hour)
	recent := received("3")
	cutoff := r.clock.Now().Add(-time.Minute)

	type purge struct {
		name string
		fn   func(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	}
	// The records of activities go after their documents.
	for _, p := range []purge{
		{"DeleteReceivedDocumentsBefore", r.h.Purges.DeleteReceivedDocumentsBefore},
		{"DeleteReceivedActivityAnswersBefore", r.h.Purges.DeleteReceivedActivityAnswersBefore},
		{"DeleteReceivedActivitiesBefore", r.h.Purges.DeleteReceivedActivitiesBefore},
		{"DeleteRemoteObjectTombstonesBefore", r.h.Purges.DeleteRemoteObjectTombstonesBefore},
		{"DeleteRemoteFollowTombstonesBefore", r.h.Purges.DeleteRemoteFollowTombstonesBefore},
	} {
		for i, want := range []int64{1, 1, 0} {
			r.tx(t, func(ctx context.Context, tx uow.Tx) error {
				n, err := p.fn(ctx, tx, cutoff, 1)
				if err == nil && n != want {
					t.Errorf("%s #%d = %d, want %d", p.name, i+1, n, want)
				}
				return err
			})
		}
	}
	for _, uri := range []federation.IRI{first, second} {
		if _, err := r.h.Inbox.FindReceivedActivity(ctx, uri); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("FindReceivedActivity(%s) = %v, want ErrNotFound", uri, err)
		}
		if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, actor+"/notes/"+uri[len(uri)-1:], actor); err != nil || ok {
			t.Errorf("RemoteObjectTombstoned(an old tombstone) = %v, %v; want false", ok, err)
		}
	}
	if doc, err := r.h.Inbox.FindReceivedDocument(ctx, recent); err != nil || len(doc) == 0 {
		t.Errorf("FindReceivedDocument(recent) = %s, %v; want it kept", doc, err)
	}
	if ok, err := r.h.Inbox.RemoteObjectTombstoned(ctx, actor+"/notes/3", actor); err != nil || !ok {
		t.Errorf("RemoteObjectTombstoned(a recent tombstone) = %v, %v; want true", ok, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Inbox.LiveRemoteFollowTombstone(ctx, tx, actor, "https://"+contractHost+"/users/3"); err != nil || got.ActivityURI != recent {
			t.Errorf("LiveRemoteFollowTombstone(recent) = %+v, %v; want it kept", got, err)
		}
		return nil
	})
}

func retentionRepoObservations(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	domains := []string{"one.example", "two.example"}
	var observations []federation.NodeInfoObservation
	var events []federation.AvailabilityEvent
	for _, domain := range domains {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			_, err := r.h.Instances.InsertRemoteInstance(ctx, tx, domain, r.clock.Now())
			return err
		})
		for range 2 {
			o := federation.NodeInfoObservation{ID: r.ids.NewID(), Domain: domain, SoftwareName: "mastodon", SoftwareVersion: "4.3.0", ObservedAt: r.clock.Now()}
			e := federation.AvailabilityEvent{ID: r.ids.NewID(), Domain: domain, Kind: federation.MarkedUnavailable, Reason: federation.ReasonFailureDays, OccurredAt: r.clock.Now()}
			r.tx(t, func(ctx context.Context, tx uow.Tx) error {
				return errors.Join(r.h.Instances.InsertNodeInfoObservation(ctx, tx, o), r.h.Federation.InsertAvailabilityEvent(ctx, tx, e))
			})
			observations, events = append(observations, o), append(events, e)
			r.clock.Advance(time.Second)
		}
	}
	cutoff := r.clock.Now()
	for _, p := range []struct {
		name string
		fn   func(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	}{
		{"DeleteNodeInfoObservationsBefore", r.h.Purges.DeleteNodeInfoObservationsBefore},
		{"DeleteAvailabilityEventsBefore", r.h.Purges.DeleteAvailabilityEventsBefore},
	} {
		for i, want := range []int64{1, 1, 0} {
			r.tx(t, func(ctx context.Context, tx uow.Tx) error {
				n, err := p.fn(ctx, tx, cutoff, 1)
				if err == nil && n != want {
					t.Errorf("%s #%d = %d, want %d: all but the newest of each server", p.name, i+1, n, want)
				}
				return err
			})
		}
	}
	for i, domain := range domains {
		newest := observations[2*i+1]
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if got, err := r.h.Instances.LatestNodeInfoObservation(ctx, tx, domain); err != nil || got.ID != newest.ID {
				t.Errorf("LatestNodeInfoObservation(%s) = %+v, %v; want the newest %s kept", domain, got, err, newest.ID)
			}
			return nil
		})
		got, err := r.h.Federation.ListAvailabilityEvents(ctx, domain, "", 10)
		if err != nil || len(got) != 1 || got[0].ID != events[2*i+1].ID {
			t.Errorf("ListAvailabilityEvents(%s) = %+v, %v; want only the newest %s", domain, got, err, events[2*i+1].ID)
		}
	}
}

func retentionRepoProfiles(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	// remote inserts a remote account with seqs profile revisions, the last one its head unless
	// head is false.
	remote := func(name string, seqs int, head bool) (string, []account.ProfileRevision) {
		a := account.Account{
			ID: r.ids.NewID(), URI: "https://remote.example/users/" + name, Domain: "remote.example",
			ActorType: account.ActorPerson, CreatedAt: r.clock.Now(),
		}
		var revs []account.ProfileRevision
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := r.h.Accounts.InsertAccount(ctx, tx, a); err != nil {
				return err
			}
			if _, err := r.h.Accounts.InsertRemoteAccount(ctx, tx, account.RemoteAccount{AccountID: a.ID, FirstSeenAt: a.CreatedAt}); err != nil {
				return err
			}
			for seq := 1; seq <= seqs; seq++ {
				rev := account.ProfileRevision{ID: r.ids.NewID(), AccountID: a.ID, Seq: seq, DisplayName: name, RevisedAt: r.clock.Now()}
				if err := r.h.Accounts.InsertProfileRevision(ctx, tx, rev); err != nil {
					return err
				}
				revs = append(revs, rev)
			}
			if !head {
				return nil
			}
			last := revs[len(revs)-1]
			return r.h.Accounts.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: a.ID, RevisionID: last.ID, Seq: last.Seq})
		})
		return a.ID, revs
	}
	first, firstRevs := remote("first", 3, true)
	second, _ := remote("second", 2, true)
	remote("current", 1, true)
	headless, headlessRevs := remote("headless", 2, false)

	listed, err := r.h.Profiles.ListRemoteAccountsWithPastProfiles(ctx, "", 10)
	if want := []string{first, second}; err != nil || !slices.Equal(listed, want) {
		t.Errorf("ListRemoteAccountsWithPastProfiles = %v, %v; want %v in ID order", listed, err, want)
	}
	if page, err := r.h.Profiles.ListRemoteAccountsWithPastProfiles(ctx, first, 1); err != nil || !slices.Equal(page, []string{second}) {
		t.Errorf("the page after the first = %v, %v; want [%s]", page, err, second)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Profiles.PrunePastProfiles(ctx, tx, first),
			r.h.Profiles.PrunePastProfiles(ctx, tx, headless),
		)
	})
	listed, err = r.h.Profiles.ListRemoteAccountsWithPastProfiles(ctx, "", 10)
	if want := []string{second}; err != nil || !slices.Equal(listed, want) {
		t.Errorf("after PrunePastProfiles = %v, %v; want %v", listed, err, want)
	}
	if cur, err := r.h.Accounts.FindCurrentProfile(ctx, first); err != nil || cur.ID != firstRevs[2].ID {
		t.Errorf("FindCurrentProfile(pruned) = %+v, %v; want the current revision %s kept", cur, err, firstRevs[2].ID)
	}
	// An account without a head keeps everything: a head now makes its past revision listed.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		last := headlessRevs[len(headlessRevs)-1]
		return r.h.Accounts.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: headless, RevisionID: last.ID, Seq: last.Seq})
	})
	listed, err = r.h.Profiles.ListRemoteAccountsWithPastProfiles(ctx, "", 10)
	if want := []string{second, headless}; err != nil || !slices.Equal(listed, want) {
		t.Errorf("after the head of the account without one = %v, %v; want %v", listed, err, want)
	}
}
