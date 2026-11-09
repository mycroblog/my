// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

// RemoteActorContract verifies the repository methods that the remote actor use cases use on a
// database adapter, by those methods alone, the port contract of ADR-0153 §1: a remote account
// gets its subtype once and is found by its URI inside and outside a transaction; its snapshots
// are found newest first; its head is upserted, found, counted a failed fetch, listed when stale
// in the order of its last fetch with paging, listed whole and as the newest snapshots make it,
// and deleted for a rebuild; the profile pages of the current snapshots and the distinct shared
// inboxes of the actors not deleted are listed; handle checks are found newest first, and the
// handle projection is upserted, found, and taken from another holder; a remote profile revision
// is current with the digest of its profile; keys are found by fingerprint revoked or not, and the current
// one is the newest without a revocation, which is recorded once; the newest remote deletion says
// who initiated it; and a remote server is recorded once with its newest NodeInfo
// observation. The use cases on top of it are RemoteActorFlows, which internal/flowtest/federation
// runs once on SQLite.
func RemoteActorContract(t *testing.T, newHarness RemoteActorFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *remoteActorRepo)
	}{
		{"StoresRemoteAccountsAndSnapshots", remoteActorRepoAccounts},
		{"KeepsTheHeadsOfRemoteActors", remoteActorRepoHeads},
		{"ListsProfileURLsAndSharedInboxes", remoteActorRepoInboxes},
		{"ChecksHandles", remoteActorRepoHandles},
		{"KeepsTheSourceOfRemoteProfiles", remoteActorRepoProfiles},
		{"RotatesKeys", remoteActorRepoKeys},
		{"RecordsRemoteDeletions", remoteActorRepoDeletions},
		{"RecordsInstancesAndNodeInfo", remoteActorRepoInstances},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))
			tc.run(t, &remoteActorRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type remoteActorRepo struct {
	h     RemoteActorHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *remoteActorRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// remote inserts a remote account named name on remote.example with its subtype.
func (r *remoteActorRepo) remote(t *testing.T, name string) account.Account {
	t.Helper()
	a := account.Account{
		ID: r.ids.NewID(), URI: "https://remote.example/users/" + name, Domain: "remote.example",
		ActorType: account.ActorPerson, CreatedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Accounts.InsertAccount(ctx, tx, a); err != nil {
			return err
		}
		_, err := r.h.Accounts.InsertRemoteAccount(ctx, tx, account.RemoteAccount{AccountID: a.ID, FirstSeenAt: a.CreatedAt})
		return err
	})
	return a
}

// snapshot inserts a snapshot of a, fetched now, whose shared inbox is shared ("" for none).
func (r *remoteActorRepo) snapshot(t *testing.T, a account.Account, shared string) account.RemoteSnapshot {
	t.Helper()
	id := r.ids.NewID()
	followers := int64(3)
	s := account.RemoteSnapshot{
		ID: id, AccountID: a.ID, PreferredUsername: "user", ProfileURL: a.URI + "/profile/" + id,
		InboxURL: a.URI + "/inbox", SharedInboxURL: shared, OutboxURL: a.URI + "/outbox",
		FollowersURL: a.URI + "/followers", FollowingURL: a.URI + "/following", FeaturedURL: a.URI + "/featured",
		FollowersTotal: &followers, DocumentDigest: []byte(id), FetchedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertRemoteSnapshot(ctx, tx, s) })
	return s
}

// head points the head of s's actor at s, fetched at s.FetchedAt.
func (r *remoteActorRepo) head(t *testing.T, s account.RemoteSnapshot) account.RemoteHead {
	t.Helper()
	h := account.RemoteHead{
		AccountID: s.AccountID, SnapshotID: s.ID, InboxURL: s.InboxURL, SharedInboxURL: s.SharedInboxURL, LastFetchedAt: s.FetchedAt,
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.UpsertRemoteHead(ctx, tx, h) })
	return h
}

// revision inserts the profile revision seq of a, made from snapshot, and points its profile head
// at it.
func (r *remoteActorRepo) revision(t *testing.T, a account.Account, seq int, snapshot string, digest []byte) account.ProfileRevision {
	t.Helper()
	rev := account.ProfileRevision{
		ID: r.ids.NewID(), AccountID: a.ID, Seq: seq, DisplayName: "rev", NoteHTML: "<p>hi</p>", RevisedAt: r.clock.Now(),
		Remote: &account.RemoteProfileSource{SnapshotID: snapshot, HTMLPolicyVersion: 7, ProfileDigest: digest},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Accounts.InsertProfileRevision(ctx, tx, rev); err != nil {
			return err
		}
		return r.h.Accounts.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: a.ID, RevisionID: rev.ID, Seq: seq})
	})
	return rev
}

func sameRemoteSnapshot(a, b account.RemoteSnapshot) bool {
	return a.ID == b.ID && a.AccountID == b.AccountID && a.PreferredUsername == b.PreferredUsername &&
		a.ProfileURL == b.ProfileURL && a.InboxURL == b.InboxURL && a.SharedInboxURL == b.SharedInboxURL &&
		a.OutboxURL == b.OutboxURL && a.FollowersURL == b.FollowersURL && a.FollowingURL == b.FollowingURL &&
		a.FeaturedURL == b.FeaturedURL && bytes.Equal(a.DocumentDigest, b.DocumentDigest) && a.FetchedAt.Equal(b.FetchedAt) &&
		a.FollowersTotal != nil && b.FollowersTotal != nil && *a.FollowersTotal == *b.FollowersTotal &&
		a.FollowingTotal == nil && b.FollowingTotal == nil && a.StatusesTotal == nil && b.StatusesTotal == nil
}

func sameRemoteHead(a, b account.RemoteHead) bool {
	return a.AccountID == b.AccountID && a.SnapshotID == b.SnapshotID && a.InboxURL == b.InboxURL &&
		a.SharedInboxURL == b.SharedInboxURL && a.LastFetchedAt.Equal(b.LastFetchedAt) &&
		a.ConsecutiveFetchFailures == b.ConsecutiveFetchFailures
}

func remoteHeadIDs(heads []account.RemoteHead) []string {
	ids := make([]string, len(heads))
	for i, h := range heads {
		ids[i] = h.AccountID
	}
	return ids
}

func remoteActorRepoAccounts(t *testing.T, r *remoteActorRepo) {
	ctx := t.Context()
	a := r.remote(t, "alice")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if again, err := r.h.Accounts.InsertRemoteAccount(ctx, tx, account.RemoteAccount{AccountID: a.ID, FirstSeenAt: r.clock.Now()}); err != nil || again {
			t.Errorf("InsertRemoteAccount again = %v, %v; want false", again, err)
		}
		if got, err := r.h.Accounts.AccountByURI(ctx, tx, a.URI); err != nil || got.ID != a.ID || got.Domain != a.Domain {
			t.Errorf("AccountByURI = %+v, %v; want %+v", got, err, a)
		}
		if _, err := r.h.Accounts.AccountByURI(ctx, tx, "https://remote.example/users/unknown"); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("AccountByURI(unknown) = %v, want ErrNotFound", err)
		}
		if _, err := r.h.Accounts.LatestRemoteSnapshot(ctx, tx, a.ID); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("LatestRemoteSnapshot without one = %v, want ErrNotFound", err)
		}
		return nil
	})
	if got, err := r.h.Accounts.FindAccountByURI(ctx, a.URI); err != nil || got.ID != a.ID {
		t.Errorf("FindAccountByURI = %+v, %v; want %s", got, err, a.ID)
	}
	r.snapshot(t, a, "")
	r.clock.Advance(time.Minute)
	newest := r.snapshot(t, a, "https://remote.example/inbox")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.LatestRemoteSnapshot(ctx, tx, a.ID); err != nil || !sameRemoteSnapshot(got, newest) {
			t.Errorf("LatestRemoteSnapshot = %+v, %v; want %+v", got, err, newest)
		}
		return nil
	})
	if got, err := r.h.Accounts.FindLatestRemoteSnapshot(ctx, a.ID); err != nil || !sameRemoteSnapshot(got, newest) {
		t.Errorf("FindLatestRemoteSnapshot = %+v, %v; want %+v", got, err, newest)
	}
	if _, err := r.h.Accounts.FindLatestRemoteSnapshot(ctx, r.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindLatestRemoteSnapshot(unknown) = %v, want ErrNotFound", err)
	}
}

func remoteActorRepoHeads(t *testing.T, r *remoteActorRepo) {
	ctx := t.Context()
	accounts := []account.Account{r.remote(t, "a"), r.remote(t, "b"), r.remote(t, "c")}
	// The heads are fetched in the order c, a, b, a minute apart.
	var heads [3]account.RemoteHead
	for _, i := range []int{2, 0, 1} {
		heads[i] = r.head(t, r.snapshot(t, accounts[i], ""))
		r.clock.Advance(time.Minute)
	}
	if got, err := r.h.Accounts.FindRemoteHead(ctx, accounts[0].ID); err != nil || !sameRemoteHead(got, heads[0]) {
		t.Errorf("FindRemoteHead = %+v, %v; want %+v", got, err, heads[0])
	}
	unfetched := account.Account{ID: r.ids.NewID(), URI: "https://remote.example/users/new", Domain: "remote.example", ActorType: account.ActorPerson, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertAccount(ctx, tx, unfetched) })
	if _, err := r.h.Accounts.FindRemoteHead(ctx, unfetched.ID); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindRemoteHead without a head = %v, want ErrNotFound", err)
	}
	if got, err := r.h.Accounts.FindRemoteHeads(ctx, []string{accounts[0].ID, accounts[2].ID, unfetched.ID}); err != nil || len(got) != 2 ||
		!sameRemoteHead(got[accounts[0].ID], heads[0]) || !sameRemoteHead(got[accounts[2].ID], heads[2]) {
		t.Errorf("FindRemoteHeads = %+v, %v; want the heads of a and c only", got, err)
	}

	// A failed fetch of b counts in its head and moves its last fetch after the others.
	failedAt := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if ok, err := r.h.Accounts.RecordRemoteFetchFailure(ctx, tx, accounts[1].ID, failedAt); err != nil || !ok {
			t.Errorf("RecordRemoteFetchFailure = %v, %v; want true", ok, err)
		}
		if ok, err := r.h.Accounts.RecordRemoteFetchFailure(ctx, tx, unfetched.ID, failedAt); err != nil || ok {
			t.Errorf("RecordRemoteFetchFailure without a head = %v, %v; want false", ok, err)
		}
		got, err := r.h.Accounts.FindRemoteHeadTx(ctx, tx, accounts[1].ID)
		want := heads[1]
		want.LastFetchedAt, want.ConsecutiveFetchFailures = failedAt, 1
		if err != nil || !sameRemoteHead(got, want) {
			t.Errorf("FindRemoteHeadTx after a failure = %+v, %v; want %+v", got, err, want)
		}
		heads[1] = want
		return nil
	})

	// Stale before failedAt: c then a; b was fetched (and failed) at failedAt.
	stale, err := r.h.Accounts.ListStaleRemoteHeads(ctx, failedAt, account.RemoteHead{}, 10)
	if want := []string{accounts[2].ID, accounts[0].ID}; err != nil || !slices.Equal(remoteHeadIDs(stale), want) {
		t.Errorf("ListStaleRemoteHeads = %v, %v; want %v by last fetch", remoteHeadIDs(stale), err, want)
	}
	page, err := r.h.Accounts.ListStaleRemoteHeads(ctx, failedAt.Add(time.Second), account.RemoteHead{}, 2)
	if want := []string{accounts[2].ID, accounts[0].ID}; err != nil || !slices.Equal(remoteHeadIDs(page), want) {
		t.Errorf("the first page of stale heads = %v, %v; want %v", remoteHeadIDs(page), err, want)
	}
	if len(page) == 2 {
		next, err := r.h.Accounts.ListStaleRemoteHeads(ctx, failedAt.Add(time.Second), page[1], 2)
		if want := []string{accounts[1].ID}; err != nil || !slices.Equal(remoteHeadIDs(next), want) || !sameRemoteHead(next[0], heads[1]) {
			t.Errorf("the page after %s = %+v, %v; want %v", page[1].AccountID, next, err, want)
		}
	}

	wantIDs := []string{accounts[0].ID, accounts[1].ID, accounts[2].ID}
	slices.Sort(wantIDs)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		listed, err := r.h.Accounts.ListRemoteHeads(ctx, tx)
		if err != nil || !slices.Equal(remoteHeadIDs(listed), wantIDs) {
			t.Errorf("ListRemoteHeads = %v, %v; want %v by account ID", remoteHeadIDs(listed), err, wantIDs)
		}
		// The newest snapshots make the heads with no failures, fetched when they were.
		rebuilt, err := r.h.Accounts.ListLatestRemoteSnapshotHeads(ctx, tx)
		if err != nil || !slices.Equal(remoteHeadIDs(rebuilt), wantIDs) {
			t.Errorf("ListLatestRemoteSnapshotHeads = %v, %v; want %v", remoteHeadIDs(rebuilt), err, wantIDs)
		}
		for _, h := range rebuilt {
			if h.AccountID == accounts[1].ID {
				if h.ConsecutiveFetchFailures != 0 || h.SnapshotID == "" || h.LastFetchedAt.Equal(failedAt) {
					t.Errorf("the head of b from its snapshot = %+v, want its snapshot's fetch and no failures", h)
				}
			}
		}
		if err := r.h.Accounts.DeleteAllRemoteHeads(ctx, tx); err != nil {
			return err
		}
		if listed, err := r.h.Accounts.ListRemoteHeads(ctx, tx); err != nil || len(listed) != 0 {
			t.Errorf("ListRemoteHeads after DeleteAllRemoteHeads = %v, %v; want none", remoteHeadIDs(listed), err)
		}
		return nil
	})
}

func remoteActorRepoInboxes(t *testing.T, r *remoteActorRepo) {
	ctx := t.Context()
	const sharedA, sharedB = "https://a.example/inbox", "https://b.example/inbox"
	alice, bob, carol, gone, none := r.remote(t, "alice"), r.remote(t, "bob"), r.remote(t, "carol"), r.remote(t, "gone"), r.remote(t, "none")
	old := r.snapshot(t, alice, sharedA)
	r.clock.Advance(time.Minute)
	current := r.snapshot(t, alice, sharedA)
	r.head(t, current)
	r.head(t, r.snapshot(t, bob, sharedB))
	r.head(t, r.snapshot(t, carol, sharedA))
	r.head(t, r.snapshot(t, gone, "https://gone.example/inbox"))
	r.head(t, r.snapshot(t, none, ""))
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Accounts.UpsertLifecycleState(ctx, tx, account.LifecycleState{AccountID: gone.ID, State: account.StateDeleted, Since: r.clock.Now()})
	})

	urls, err := r.h.Accounts.ListRemoteProfileURLs(ctx, []string{alice.ID, bob.ID, r.ids.NewID()})
	if err != nil || len(urls) != 2 || urls[alice.ID] != current.ProfileURL || urls[alice.ID] == old.ProfileURL || urls[bob.ID] == "" {
		t.Errorf("ListRemoteProfileURLs = %v, %v; want the current snapshots' pages of alice and bob", urls, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.ListRemoteProfileURLsTx(ctx, tx, []string{alice.ID}); err != nil || len(got) != 1 || got[alice.ID] != current.ProfileURL {
			t.Errorf("ListRemoteProfileURLsTx = %v, %v; want alice's current page", got, err)
		}
		return nil
	})

	if got, err := r.h.Accounts.ListKnownSharedInboxes(ctx, "", 10); err != nil || !slices.Equal(got, []string{sharedA, sharedB}) {
		t.Errorf("ListKnownSharedInboxes = %v, %v; want the distinct inboxes of the actors not deleted, in order", got, err)
	}
	if got, err := r.h.Accounts.ListKnownSharedInboxes(ctx, sharedA, 1); err != nil || !slices.Equal(got, []string{sharedB}) {
		t.Errorf("the page after %s = %v, %v; want [%s]", sharedA, got, err, sharedB)
	}
	if n, err := r.h.Accounts.CountKnownSharedInboxes(ctx); err != nil || n != 2 {
		t.Errorf("CountKnownSharedInboxes = %d, %v; want 2", n, err)
	}
}

func remoteActorRepoHandles(t *testing.T, r *remoteActorRepo) {
	ctx := t.Context()
	alice, bob := r.remote(t, "alice"), r.remote(t, "bob")
	check := func(a account.Account, username string, outcome account.HandleOutcome) account.HandleVerification {
		v := account.HandleVerification{ID: r.ids.NewID(), AccountID: a.ID, Username: username, Domain: "remote.example", Outcome: outcome, CheckedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertHandleVerification(ctx, tx, v) })
		r.clock.Advance(time.Minute)
		return v
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.h.Accounts.LatestHandleVerification(ctx, tx, alice.ID); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("LatestHandleVerification without one = %v, want ErrNotFound", err)
		}
		return nil
	})
	check(alice, "Alice", account.HandleMismatch)
	newest := check(alice, "Alice", account.HandleVerified)
	bobs := check(bob, "bob", account.HandleVerified)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.LatestHandleVerification(ctx, tx, alice.ID); err != nil || got.ID != newest.ID || !got.SameResult(newest) || !got.CheckedAt.Equal(newest.CheckedAt) {
			t.Errorf("LatestHandleVerification = %+v, %v; want %+v", got, err, newest)
		}
		latest, err := r.h.Accounts.ListLatestHandleVerifications(ctx, tx)
		if err != nil || len(latest) != 2 || latest[0].ID != newest.ID || latest[1].ID != bobs.ID {
			t.Errorf("ListLatestHandleVerifications = %+v, %v; want the newest of each, by check ID", latest, err)
		}
		return nil
	})

	h, _ := newest.Handle()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.UpsertHandle(ctx, tx, h) })
	if got, err := r.h.Accounts.FindHandle(ctx, "alice", "remote.example"); err != nil || got != h {
		t.Errorf("FindHandle = %+v, %v; want %+v", got, err, h)
	}
	if got, err := r.h.Accounts.FindHandleOf(ctx, alice.ID); err != nil || got != h {
		t.Errorf("FindHandleOf = %+v, %v; want %+v", got, err, h)
	}
	// The holder keeps its handle; another account's claim takes it.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.DeleteOtherHandle(ctx, tx, h) })
	if _, err := r.h.Accounts.FindHandle(ctx, "alice", "remote.example"); err != nil {
		t.Errorf("FindHandle after DeleteOtherHandle of its holder = %v, want it kept", err)
	}
	claim := h
	claim.AccountID = bob.ID
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.DeleteOtherHandle(ctx, tx, claim) })
	if _, err := r.h.Accounts.FindHandle(ctx, "alice", "remote.example"); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindHandle after another account's DeleteOtherHandle = %v, want ErrNotFound", err)
	}
	bh, _ := bobs.Handle()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Accounts.UpsertHandle(ctx, tx, bh); err != nil {
			return err
		}
		return r.h.Accounts.DeleteHandle(ctx, tx, bob.ID)
	})
	if _, err := r.h.Accounts.FindHandleOf(ctx, bob.ID); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindHandleOf after DeleteHandle = %v, want ErrNotFound", err)
	}
}

func remoteActorRepoProfiles(t *testing.T, r *remoteActorRepo) {
	ctx := t.Context()
	a := r.remote(t, "alice")
	s := r.snapshot(t, a, "")
	r.head(t, s)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.h.Accounts.RemoteProfileDigest(ctx, tx, a.ID); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("RemoteProfileDigest without a revision = %v, want ErrNotFound", err)
		}
		return nil
	})
	r.revision(t, a, 1, s.ID, []byte("first"))
	r.clock.Advance(time.Minute)
	rev := r.revision(t, a, 2, s.ID, []byte("second"))
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.h.Accounts.CurrentProfile(ctx, tx, a.ID)
		if err != nil || got.ID != rev.ID || got.Seq != 2 || got.NoteHTML != rev.NoteHTML {
			t.Errorf("CurrentProfile = %+v, %v; want %+v", got, err, rev)
		}
		if d, err := r.h.Accounts.RemoteProfileDigest(ctx, tx, a.ID); err != nil || !bytes.Equal(d, []byte("second")) {
			t.Errorf("RemoteProfileDigest = %q, %v; want the current revision's", d, err)
		}
		return nil
	})
	if d, err := r.h.Accounts.FindRemoteProfileDigest(ctx, a.ID); err != nil || !bytes.Equal(d, []byte("second")) {
		t.Errorf("FindRemoteProfileDigest = %q, %v; want the current revision's", d, err)
	}
}

func remoteActorRepoKeys(t *testing.T, r *remoteActorRepo) {
	ctx := t.Context()
	a := r.remote(t, "alice")
	keyID := a.URI + "#main-key"
	insert := func(fingerprint string) account.Key {
		k := account.Key{
			ID: r.ids.NewID(), AccountID: a.ID, KeyID: keyID, Algorithm: account.KeyRSASHA256,
			PublicKeyPEM: "-----BEGIN PUBLIC KEY-----\n" + fingerprint + "\n-----END PUBLIC KEY-----\n",
			Fingerprint:  []byte(fingerprint), CreatedAt: r.clock.Now(),
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertKey(ctx, tx, k, nil) })
		r.clock.Advance(time.Minute)
		return k
	}
	same := func(got, want account.Key) bool {
		return got.ID == want.ID && got.AccountID == want.AccountID && got.KeyID == want.KeyID && got.Algorithm == want.Algorithm &&
			got.PublicKeyPEM == want.PublicKeyPEM && bytes.Equal(got.Fingerprint, want.Fingerprint) && got.CreatedAt.Equal(want.CreatedAt)
	}
	old := insert("old")
	rotated := insert("rotated")
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.CurrentKey(ctx, tx, keyID); err != nil || !same(got, rotated) {
			t.Errorf("CurrentKey = %+v, %v; want the newest %+v", got, err, rotated)
		}
		if revoked, err := r.h.Accounts.InsertKeyRevocation(ctx, tx, account.KeyRevocation{AccountKeyID: old.ID, RevokedAt: r.clock.Now()}); err != nil || !revoked {
			t.Errorf("InsertKeyRevocation = %v, %v; want true", revoked, err)
		}
		if revoked, err := r.h.Accounts.InsertKeyRevocation(ctx, tx, account.KeyRevocation{AccountKeyID: old.ID, RevokedAt: r.clock.Now()}); err != nil || revoked {
			t.Errorf("InsertKeyRevocation again = %v, %v; want false", revoked, err)
		}
		if got, err := r.h.Accounts.KeyByFingerprint(ctx, tx, keyID, []byte("old")); err != nil || !same(got, old) {
			t.Errorf("KeyByFingerprint of a revoked key = %+v, %v; want %+v", got, err, old)
		}
		if _, err := r.h.Accounts.KeyByFingerprint(ctx, tx, keyID, []byte("unknown")); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("KeyByFingerprint(unknown) = %v, want ErrNotFound", err)
		}
		if active, err := r.h.Accounts.ListActiveKeys(ctx, tx, a.ID); err != nil || len(active) != 1 || !same(active[0], rotated) {
			t.Errorf("ListActiveKeys = %+v, %v; want only %s", active, err, rotated.ID)
		}
		if keys, err := r.h.Accounts.ListKeysTx(ctx, tx, a.ID); err != nil || len(keys) != 2 || keys[0].ID != old.ID {
			t.Errorf("ListKeysTx = %+v, %v; want both, oldest first", keys, err)
		}
		// Revoking the newest leaves no current key.
		if _, err := r.h.Accounts.InsertKeyRevocation(ctx, tx, account.KeyRevocation{AccountKeyID: rotated.ID, RevokedAt: r.clock.Now()}); err != nil {
			return err
		}
		if _, err := r.h.Accounts.CurrentKey(ctx, tx, keyID); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("CurrentKey with every key revoked = %v, want ErrNotFound", err)
		}
		return nil
	})
	if _, err := r.h.Accounts.FindCurrentKey(ctx, keyID); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindCurrentKey with every key revoked = %v, want ErrNotFound", err)
	}
	fresh := insert("fresh")
	if got, err := r.h.Accounts.FindCurrentKey(ctx, keyID); err != nil || !same(got, fresh) {
		t.Errorf("FindCurrentKey = %+v, %v; want %+v", got, err, fresh)
	}
	if keys, err := r.h.Accounts.ListKeys(ctx, a.ID); err != nil || len(keys) != 3 || keys[2].ID != fresh.ID {
		t.Errorf("ListKeys = %+v, %v; want the 3, oldest first", keys, err)
	}
}

func remoteActorRepoDeletions(t *testing.T, r *remoteActorRepo) {
	ctx := t.Context()
	a := r.remote(t, "alice")
	event := func(by account.Initiator) {
		e := account.LifecycleEvent{ID: r.ids.NewID(), AccountID: a.ID, Kind: account.LifecycleRemoteDeleted, InitiatedBy: by, OccurredAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertLifecycleEvent(ctx, tx, e) })
		r.clock.Advance(time.Minute)
	}
	if _, err := r.h.Accounts.NewestRemoteDeletion(ctx, nil, a.ID); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("NewestRemoteDeletion without one = %v, want ErrNotFound", err)
	}
	event(account.InitiatedBySystem)
	event(account.InitiatedByRemote)
	if by, err := r.h.Accounts.NewestRemoteDeletion(ctx, nil, a.ID); err != nil || by != account.InitiatedByRemote {
		t.Errorf("NewestRemoteDeletion = %q, %v; want remote", by, err)
	}
	event(account.InitiatedBySystem)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if by, err := r.h.Accounts.NewestRemoteDeletion(ctx, tx, a.ID); err != nil || by != account.InitiatedBySystem {
			t.Errorf("NewestRemoteDeletion in a transaction = %q, %v; want system", by, err)
		}
		return nil
	})
}

func remoteActorRepoInstances(t *testing.T, r *remoteActorRepo) {
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if ok, err := r.h.Instances.InsertRemoteInstance(ctx, tx, "mastodon.example", r.clock.Now()); err != nil || !ok {
			t.Errorf("InsertRemoteInstance = %v, %v; want true", ok, err)
		}
		if ok, err := r.h.Instances.InsertRemoteInstance(ctx, tx, "mastodon.example", r.clock.Now().Add(time.Hour)); err != nil || ok {
			t.Errorf("InsertRemoteInstance again = %v, %v; want false", ok, err)
		}
		if _, err := r.h.Instances.LatestNodeInfoObservation(ctx, tx, "mastodon.example"); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("LatestNodeInfoObservation without one = %v, want ErrNotFound", err)
		}
		return nil
	})
	var newest federation.NodeInfoObservation
	for _, version := range []string{"4.2.0", "4.3.1"} {
		newest = federation.NodeInfoObservation{ID: r.ids.NewID(), Domain: "mastodon.example", SoftwareName: "mastodon", SoftwareVersion: version, ObservedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Instances.InsertNodeInfoObservation(ctx, tx, newest)
		})
		r.clock.Advance(time.Hour)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.h.Instances.LatestNodeInfoObservation(ctx, tx, "mastodon.example")
		if err != nil || got.ID != newest.ID || !got.SameSoftware(newest) || got.Domain != newest.Domain || !got.ObservedAt.Equal(newest.ObservedAt) {
			t.Errorf("LatestNodeInfoObservation = %+v, %v; want %+v", got, err, newest)
		}
		if _, err := r.h.Instances.LatestNodeInfoObservation(ctx, tx, "unseen.example"); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("LatestNodeInfoObservation(unseen) = %v, want ErrNotFound", err)
		}
		return nil
	})
}
