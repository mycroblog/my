// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
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

// The cases of RetentionContract on the outbound activities, the handle checks, and the snapshots
// of remote actors (data-model §4.2, §4.8, §8).

// at runs fn with the clock set to age before now, so that the IDs fn makes are of that time.
func (r *retentionRepo) at(age time.Duration, fn func(at time.Time)) {
	now := r.clock.Now()
	at := now.Add(-age)
	r.clock.Set(at)
	defer r.clock.Set(now)
	fn(at)
}

// outbound stores an outbound activity of the given age with one delivery, one attempt, its
// document, and a fan-out completion, and returns the IDs of the activity and of the delivery.
func (r *retentionRepo) outbound(t *testing.T, typ federation.ActivityType, kind federation.OutboundObjectKind, actor, object string, age time.Duration) (activityID, deliveryID string) {
	t.Helper()
	r.at(age, func(at time.Time) {
		act := federation.OutboundActivity{
			ID: r.ids.NewID(), Type: typ, ActorAccountID: actor, ObjectKind: kind, ObjectID: object, CreatedAt: at,
		}
		act.ActivityURI = federation.IRI("https://" + testkit.ContractHost + "/ap/activities/" + act.ID)
		del := federation.Delivery{ID: r.ids.NewID(), OutboundActivityID: act.ID, InboxURL: "https://remote.example/inbox", Domain: "remote.example", CreatedAt: at}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := r.h.Federation.InsertOutboundActivity(ctx, tx, act); err != nil {
				return err
			}
			if _, err := r.h.Federation.InsertOutboundDocument(ctx, tx, act.ID, []byte(`{"type":"`+string(typ)+`"}`), at); err != nil {
				return err
			}
			if _, err := r.h.Federation.InsertDelivery(ctx, tx, del); err != nil {
				return err
			}
			if err := r.h.Federation.InsertDeliveryAttempt(ctx, tx, federation.DeliveryAttempt{
				ID: r.ids.NewID(), DeliveryID: del.ID, AttemptedAt: at, Outcome: federation.DeliverySucceeded, HTTPStatus: 202,
			}); err != nil {
				return err
			}
			return r.h.Federation.InsertFanOutCompletion(ctx, tx, act.ID, at)
		})
		activityID, deliveryID = act.ID, del.ID
	})
	return activityID, deliveryID
}

// purge runs del in batches of limit until it deletes nothing and returns how many it deleted,
// failing when a batch deletes more than limit.
func (r *retentionRepo) purge(t *testing.T, name string, del func(context.Context, uow.Tx, time.Time, int) (int64, error), before time.Time, limit int) int64 {
	t.Helper()
	var total int64
	for range 100 {
		var n int64
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			n, err = del(ctx, tx, before, limit)
			return err
		})
		if n > int64(limit) {
			t.Errorf("%s(limit %d) = %d", name, limit, n)
		}
		if n == 0 {
			return total
		}
		total += n
	}
	t.Fatalf("%s did not finish", name)
	return total
}

// retentionRepoOutbound checks DeleteSupersededStatusCreates and DeleteExpiredOutboundActivities:
// a status's Create goes, with its deliveries, attempts, document, and fan-out completion, once
// its status or its actor (not by the service closure) has a Delete older than the cutoff; every
// other activity goes once it is older than the cutoff, except the service closure's Deletes and a
// Delete whose Create is kept.
func retentionRepoOutbound(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	sender, gone, closing := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	const (
		create = federation.ActivityCreate
		del    = federation.ActivityDelete
		status = federation.OutboundStatus
		acct   = federation.OutboundAccount
	)
	old, recent := 31*retentionDay, retentionDay
	type row struct {
		name               string
		activity, delivery string
		kept               bool
	}
	var rows []row
	add := func(name string, kept bool, typ federation.ActivityType, kind federation.OutboundObjectKind, actor, object string, age time.Duration) string {
		t.Helper()
		a, d := r.outbound(t, typ, kind, actor, object, age)
		rows = append(rows, row{name, a, d, kept})
		return a
	}
	add("an old Like", false, federation.ActivityLike, federation.OutboundFavourite, sender, r.ids.NewID(), old)
	add("a recent Like", true, federation.ActivityLike, federation.OutboundFavourite, sender, r.ids.NewID(), recent)
	add("the Create of a live status", true, create, status, sender, r.ids.NewID(), 200*retentionDay)
	add("the old Update of a live status", false, federation.ActivityUpdate, status, sender, r.ids.NewID(), old)
	deleted := r.ids.NewID()
	add("the Create of a status deleted long ago", false, create, status, sender, deleted, 200*retentionDay)
	add("the old Delete of that status", false, del, status, sender, deleted, old)
	justDeleted := r.ids.NewID()
	add("the Create of a status deleted just now", true, create, status, sender, justDeleted, 200*retentionDay)
	add("the recent Delete of that status", true, del, status, sender, justDeleted, recent)
	add("the Create of an actor deleted long ago", false, create, status, gone, r.ids.NewID(), 200*retentionDay)
	add("the old Delete of that actor", false, del, acct, gone, gone, old)
	add("the Create of an actor the closure deleted", true, create, status, closing, r.ids.NewID(), 200*retentionDay)
	closure := add("the closure's old Delete of the actor", true, del, acct, closing, closing, old)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Federation.InsertClosureDelete(ctx, tx, federation.ClosureDelete{AccountID: closing, OutboundActivityID: closure, RecordedAt: r.clock.Now()})
		return err
	})

	cutoff := r.clock.Now().Add(-30 * retentionDay)
	// A Delete waits for its Create: before the superseded Creates go, the old Deletes of a status
	// and of an actor stay.
	if n := r.purge(t, "DeleteExpiredOutboundActivities", r.h.Purges.DeleteExpiredOutboundActivities, cutoff, 1); n != 2 {
		t.Errorf("DeleteExpiredOutboundActivities before the Creates go = %d, want 2 (the old Like and Update)", n)
	}
	if n := r.purge(t, "DeleteSupersededStatusCreates", r.h.Purges.DeleteSupersededStatusCreates, cutoff, 1); n != 2 {
		t.Errorf("DeleteSupersededStatusCreates = %d, want 2", n)
	}
	if n := r.purge(t, "DeleteExpiredOutboundActivities", r.h.Purges.DeleteExpiredOutboundActivities, cutoff, 1); n != 2 {
		t.Errorf("DeleteExpiredOutboundActivities after the Creates went = %d, want 2 (their Deletes)", n)
	}
	for _, row := range rows {
		_, err := r.h.Federation.FindOutboundActivity(ctx, row.activity)
		if got := err == nil; got != row.kept || (err != nil && !errors.Is(err, appfederation.ErrNotFound)) {
			t.Errorf("%s: FindOutboundActivity = %v, want kept = %t", row.name, err, row.kept)
		}
		_, err = r.h.Federation.FindDelivery(ctx, row.delivery)
		if got := err == nil; got != row.kept || (err != nil && !errors.Is(err, appfederation.ErrNotFound)) {
			t.Errorf("%s: FindDelivery = %v, want kept = %t", row.name, err, row.kept)
		}
		_, err = r.h.Federation.FindOutboundDocument(ctx, row.activity)
		if got := err == nil; got != row.kept || (err != nil && !errors.Is(err, appfederation.ErrNotFound)) {
			t.Errorf("%s: FindOutboundDocument = %v, want kept = %t", row.name, err, row.kept)
		}
		if attempts, err := r.h.Federation.ListDeliveryAttempts(ctx, row.delivery); err != nil || (len(attempts) == 1) != row.kept {
			t.Errorf("%s: ListDeliveryAttempts = %+v, %v; want kept = %t", row.name, attempts, err, row.kept)
		}
	}
}

// retentionRepoOutboundBodies checks DeleteOutboundDocumentsBefore and DeleteDeliveryAttemptsBefore:
// the documents built and the attempts made before the cutoff go in bounded batches, and the
// activities and deliveries stay.
func retentionRepoOutboundBodies(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	sender := r.ids.NewID()
	var olds []string
	for range 3 {
		a, _ := r.outbound(t, federation.ActivityLike, federation.OutboundFavourite, sender, r.ids.NewID(), 31*retentionDay)
		olds = append(olds, a)
	}
	recent, recentDelivery := r.outbound(t, federation.ActivityLike, federation.OutboundFavourite, sender, r.ids.NewID(), retentionDay)
	cutoff := r.clock.Now().Add(-30 * retentionDay)

	if n := r.purge(t, "DeleteOutboundDocumentsBefore", r.h.Purges.DeleteOutboundDocumentsBefore, cutoff, 2); n != 3 {
		t.Errorf("DeleteOutboundDocumentsBefore = %d, want the 3 old documents", n)
	}
	if n := r.purge(t, "DeleteDeliveryAttemptsBefore", r.h.Purges.DeleteDeliveryAttemptsBefore, cutoff, 2); n != 3 {
		t.Errorf("DeleteDeliveryAttemptsBefore = %d, want the 3 old attempts", n)
	}
	for _, id := range olds {
		if _, err := r.h.Federation.FindOutboundDocument(ctx, id); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("FindOutboundDocument(old) = %v, want ErrNotFound", err)
		}
		if _, err := r.h.Federation.FindOutboundActivity(ctx, id); err != nil {
			t.Errorf("FindOutboundActivity(old) = %v, want the activity kept", err)
		}
	}
	if _, err := r.h.Federation.FindOutboundDocument(ctx, recent); err != nil {
		t.Errorf("FindOutboundDocument(recent) = %v, want it kept", err)
	}
	if attempts, err := r.h.Federation.ListDeliveryAttempts(ctx, recentDelivery); err != nil || len(attempts) != 1 {
		t.Errorf("ListDeliveryAttempts(recent) = %+v, %v; want the attempt kept", attempts, err)
	}
}

// retentionRepoHandleChecks checks DeleteHandleVerificationsBefore: the handle checks made before
// the cutoff go in bounded batches, except the newest of each account, however old.
func retentionRepoHandleChecks(t *testing.T, r *retentionRepo) {
	check := func(accountID string, age time.Duration) string {
		t.Helper()
		var id string
		r.at(age, func(at time.Time) {
			v := account.HandleVerification{
				ID: r.ids.NewID(), AccountID: accountID, Username: "checked", Domain: "remote.example",
				Outcome: account.HandleVerified, CheckedAt: at,
			}
			r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertHandleVerification(ctx, tx, v) })
			id = v.ID
		})
		return id
	}
	stale, fresh := r.ids.NewID(), r.ids.NewID()
	check(stale, 120*retentionDay)
	check(stale, 100*retentionDay)
	staleNewest := check(stale, 95*retentionDay)
	check(fresh, 100*retentionDay)
	freshNewest := check(fresh, retentionDay)

	cutoff := r.clock.Now().Add(-appaccount.HandleVerificationRetention)
	if n := r.purge(t, "DeleteHandleVerificationsBefore", r.h.Profiles.DeleteHandleVerificationsBefore, cutoff, 1); n != 3 {
		t.Errorf("DeleteHandleVerificationsBefore = %d, want 3 (all old checks but the newest of each account)", n)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for accountID, want := range map[string]string{stale: staleNewest, fresh: freshNewest} {
			if got, err := r.h.Accounts.LatestHandleVerification(ctx, tx, accountID); err != nil || got.ID != want {
				t.Errorf("LatestHandleVerification = %+v, %v; want %s kept", got, err, want)
			}
		}
		return nil
	})
}

// retentionRepoSnapshots checks the snapshots of PrunePastProfiles: a remote account with
// snapshots that neither its head nor a profile revision points to is listed, and pruning deletes
// those snapshots and keeps the others; an account without a head keeps every snapshot.
func retentionRepoSnapshots(t *testing.T, r *retentionRepo) {
	ctx := t.Context()
	// remote inserts a remote account with n snapshots, the last one its head when head is true,
	// and a current profile revision from the first snapshot.
	remote := func(name string, n int, head bool) (string, []account.RemoteSnapshot) {
		a := account.Account{
			ID: r.ids.NewID(), URI: "https://remote.example/users/" + name, Domain: "remote.example",
			ActorType: account.ActorPerson, CreatedAt: r.clock.Now(),
		}
		var snaps []account.RemoteSnapshot
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := r.h.Accounts.InsertAccount(ctx, tx, a); err != nil {
				return err
			}
			if _, err := r.h.Accounts.InsertRemoteAccount(ctx, tx, account.RemoteAccount{AccountID: a.ID, FirstSeenAt: a.CreatedAt}); err != nil {
				return err
			}
			for i := range n {
				s := account.RemoteSnapshot{
					ID: r.ids.NewID(), AccountID: a.ID, PreferredUsername: name, InboxURL: a.URI + "/inbox",
					DocumentDigest: []byte{byte(i)}, FetchedAt: r.clock.Now(),
				}
				if err := r.h.Accounts.InsertRemoteSnapshot(ctx, tx, s); err != nil {
					return err
				}
				snaps = append(snaps, s)
			}
			rev := account.ProfileRevision{
				ID: r.ids.NewID(), AccountID: a.ID, Seq: 1, DisplayName: name, RevisedAt: r.clock.Now(),
				Remote: &account.RemoteProfileSource{SnapshotID: snaps[0].ID, HTMLPolicyVersion: 1, ProfileDigest: []byte{1}},
			}
			if err := r.h.Accounts.InsertProfileRevision(ctx, tx, rev); err != nil {
				return err
			}
			if err := r.h.Accounts.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: a.ID, RevisionID: rev.ID, Seq: rev.Seq}); err != nil {
				return err
			}
			if !head {
				return nil
			}
			last := snaps[len(snaps)-1]
			return r.h.Accounts.UpsertRemoteHead(ctx, tx, account.RemoteHead{
				AccountID: a.ID, SnapshotID: last.ID, InboxURL: last.InboxURL, LastFetchedAt: last.FetchedAt,
			})
		})
		return a.ID, snaps
	}
	past, pastSnaps := remote("past", 3, true)
	remote("current", 2, true) // the first is the profile's, the second the head's
	headless, headlessSnaps := remote("headless", 2, false)

	listed, err := r.h.Profiles.ListRemoteAccountsWithPastProfiles(ctx, "", 10)
	if want := []string{past}; err != nil || !slices.Equal(listed, want) {
		t.Errorf("ListRemoteAccountsWithPastProfiles = %v, %v; want %v, the account with a past snapshot", listed, err, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.h.Profiles.PrunePastProfiles(ctx, tx, past), r.h.Profiles.PrunePastProfiles(ctx, tx, headless))
	})
	if listed, err := r.h.Profiles.ListRemoteAccountsWithPastProfiles(ctx, "", 10); err != nil || len(listed) != 0 {
		t.Errorf("ListRemoteAccountsWithPastProfiles after the pruning = %v, %v; want none", listed, err)
	}
	if latest, err := r.h.Accounts.FindLatestRemoteSnapshot(ctx, past); err != nil || latest.ID != pastSnaps[2].ID {
		t.Errorf("FindLatestRemoteSnapshot(pruned) = %+v, %v; want the head's %s kept", latest, err, pastSnaps[2].ID)
	}
	if digest, err := r.h.Accounts.FindRemoteProfileDigest(ctx, past); err != nil || !slices.Equal(digest, []byte{1}) {
		t.Errorf("FindRemoteProfileDigest(pruned) = %v, %v; want the current revision's, whose snapshot stays", digest, err)
	}
	if latest, err := r.h.Accounts.FindLatestRemoteSnapshot(ctx, headless); err != nil || latest.ID != headlessSnaps[1].ID {
		t.Errorf("FindLatestRemoteSnapshot(headless) = %+v, %v; want %s kept", latest, err, headlessSnaps[1].ID)
	}
}
