// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The cases of RetentionContract on the outbound activities, the deletions by retention, and the
// observations of remote servers (data-model §4.8, §8).

const retentionDay = 24 * time.Hour

// outboundActivity stores an activity of the given age with one delivery and a fan-out completion,
// and returns the IDs of the activity and of the delivery.
func (e *retentionEnv) outboundActivity(t *testing.T, typ federation.ActivityType, kind federation.OutboundObjectKind, actor, object string, age time.Duration) (activityID, deliveryID string) {
	t.Helper()
	ctx := t.Context()
	at := e.clock.Now().Add(-age)
	now := e.clock.Now()
	e.clock.Set(at)
	defer e.clock.Set(now)
	act := federation.OutboundActivity{
		ID: e.ids.NewID(), Type: typ, ActorAccountID: actor, ObjectKind: kind, ObjectID: object, CreatedAt: at,
	}
	act.ActivityURI = federation.IRI("https://" + contractHost + "/ap/activities/" + act.ID)
	del := federation.Delivery{ID: e.ids.NewID(), OutboundActivityID: act.ID, InboxURL: "https://remote.example/inbox", Domain: "remote.example", CreatedAt: at}
	err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.rh.Federation.InsertOutboundActivity(ctx, tx, act); err != nil {
			return err
		}
		if _, err := e.rh.Federation.InsertDelivery(ctx, tx, del); err != nil {
			return err
		}
		return e.rh.Federation.InsertFanOutCompletion(ctx, tx, act.ID, at)
	})
	if err != nil {
		t.Fatalf("record %s: %v", typ, err)
	}
	return act.ID, del.ID
}

// retentionOutboundActivities checks what the purge keeps of the outbound activities: the deliveries
// of a status's Create until the status or its actor has been deleted for 30 days, and everything
// else for 30 days.
func retentionOutboundActivities(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	sender := e.user(t, "sender")
	gone, closing := e.user(t, "gone"), e.user(t, "closing")
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
		a, d := e.outboundActivity(t, typ, kind, actor, object, age)
		rows = append(rows, row{name, a, d, kept})
		return a
	}
	add("an old Like", false, federation.ActivityLike, federation.OutboundFavourite, sender, e.ids.NewID(), old)
	add("a recent Like", true, federation.ActivityLike, federation.OutboundFavourite, sender, e.ids.NewID(), recent)
	add("the Create of a live status", true, create, status, sender, e.ids.NewID(), 200*retentionDay)
	add("the old Update of a live status", false, federation.ActivityUpdate, status, sender, e.ids.NewID(), old)

	deleted := e.ids.NewID()
	add("the Create of a status deleted long ago", false, create, status, sender, deleted, 200*retentionDay)
	add("the old Delete of that status", false, del, status, sender, deleted, old)

	justDeleted := e.ids.NewID()
	add("the Create of a status deleted just now", true, create, status, sender, justDeleted, 200*retentionDay)
	add("the recent Delete of that status", true, del, status, sender, justDeleted, recent)

	add("the Create of an actor deleted long ago", false, create, status, gone, e.ids.NewID(), 200*retentionDay)
	add("the old Delete of that actor", false, del, acct, gone, gone, old)

	add("the Create of an actor the closure deleted", true, create, status, closing, e.ids.NewID(), 200*retentionDay)
	closure := add("the closure's old Delete of the actor", true, del, acct, closing, closing, old)
	err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := e.rh.Federation.InsertClosureDelete(ctx, tx, federation.ClosureDelete{AccountID: closing, OutboundActivityID: closure, RecordedAt: e.clock.Now()})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	purger := appfederation.NewPurger(appfederation.PurgeDeps{UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Purges, Clock: e.clock, Batch: 1})
	for range 2 { // safe to repeat
		if _, err := purger.Run(ctx); err != nil {
			t.Fatalf("Run = %v", err)
		}
	}
	for _, r := range rows {
		_, err := e.rh.Federation.FindOutboundActivity(ctx, r.activity)
		if got := err == nil; got != r.kept || (err != nil && !errors.Is(err, appfederation.ErrNotFound)) {
			t.Errorf("%s: FindOutboundActivity = %v, want kept = %t", r.name, err, r.kept)
		}
		_, err = e.rh.Federation.FindDelivery(ctx, r.delivery)
		if got := err == nil; got != r.kept || (err != nil && !errors.Is(err, appfederation.ErrNotFound)) {
			t.Errorf("%s: FindDelivery = %v, want kept = %t", r.name, err, r.kept)
		}
	}
}

// retentionObservations checks that the observations of a server go after 90 days, except the
// newest of each server.
func retentionObservations(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	now := e.clock.Now()
	record := func(domain string, age time.Duration) (nodeinfo, event string) {
		t.Helper()
		at := now.Add(-age)
		e.clock.Set(at)
		defer e.clock.Set(now)
		nodeinfo, event = e.ids.NewID(), e.ids.NewID()
		err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			if err := e.rh.Instances.InsertNodeInfoObservation(ctx, tx, federation.NodeInfoObservation{
				ID: nodeinfo, Domain: domain, SoftwareName: "mastodon", SoftwareVersion: "4", ObservedAt: at,
			}); err != nil {
				return err
			}
			return e.rh.Federation.InsertAvailabilityEvent(ctx, tx, federation.AvailabilityEvent{
				ID: event, Domain: domain, Kind: federation.MarkedUnavailable, Reason: federation.ReasonFailureDays, OccurredAt: at,
			})
		})
		if err != nil {
			t.Fatalf("record %s: %v", domain, err)
		}
		return nodeinfo, event
	}
	record("busy.example", 100*retentionDay)
	_, midEvent := record("busy.example", 91*retentionDay)
	_, newEvent := record("busy.example", retentionDay)
	_, quietEvent := record("quiet.example", 100*retentionDay)

	purger := appfederation.NewPurger(appfederation.PurgeDeps{UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Purges, Clock: e.clock})
	if _, err := purger.Run(ctx); err != nil {
		t.Fatalf("Run = %v", err)
	}
	events, err := e.rh.Federation.ListAvailabilityEvents(ctx, "busy.example", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != newEvent {
		t.Errorf("events of the busy server = %+v, want only %s (not %s)", events, newEvent, midEvent)
	}
	events, err = e.rh.Federation.ListAvailabilityEvents(ctx, "quiet.example", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != quietEvent {
		t.Errorf("events of the quiet server = %+v, want its newest kept", events)
	}
	// The purge took the old NodeInfo observations of the busy server too.
	err = e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		n, err := e.rh.Purges.DeleteNodeInfoObservationsBefore(ctx, tx, now.Add(-appfederation.InstanceObservationRetention), 10)
		if err == nil && n != 0 {
			t.Errorf("DeleteNodeInfoObservationsBefore after the run = %d, want 0", n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range []string{"busy.example", "quiet.example"} {
		err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := e.rh.Instances.LatestNodeInfoObservation(ctx, tx, domain)
			return err
		})
		if err != nil {
			t.Errorf("the newest NodeInfo observation of %s: %v", domain, err)
		}
	}
}

// retentionDeletions checks that a deletion by retention goes after 30 days, unless a local
// interaction still names the status: its readers tell from the row that the status is gone.
func retentionDeletions(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	statuses, _, _ := e.referencedStatuses(t)
	const maxPeriod = 100 * retentionDay
	if n := e.purgeAll(t, e.retention(retentionPeriod, maxPeriod, 0)); n == 0 {
		t.Fatal("nothing was purged")
	}
	wantDeletion := func(name string, want bool, when string) {
		t.Helper()
		_, err := e.rh.Statuses.FindDeletion(ctx, statuses[name].ID)
		if got := err == nil; got != want || (err != nil && !errors.Is(err, appcontent.ErrNotFound)) {
			t.Errorf("%s: FindDeletion(%s) = %v, want found = %t", when, name, err, want)
		}
	}
	wantDeletion("unrelated", true, "right after the purge")

	e.clock.Advance(31 * retentionDay)
	e.purgeAll(t, e.retention(retentionPeriod, maxPeriod, 1))
	wantDeletion("unrelated", false, "31 days later")
	wantDeletion("boost", true, "31 days later")
	wantDeletion("favourite", true, "31 days later")
	wantDeletion("bookmark", true, "31 days later")
	if tombstoned, err := e.content.Tombstoned(ctx, statuses["unrelated"].URI); err != nil || tombstoned {
		t.Errorf("Tombstoned(unrelated) = %t, %v; want it to be fetchable again", tombstoned, err)
	}
}

// retentionHandleVerifications checks that the checks of a remote handle go after 90 days, except
// the newest of the account, which rebuilds account_handles.
func retentionHandleVerifications(t *testing.T, e *retentionEnv) {
	ctx := t.Context()
	accountID := e.actor(t, "https://remote.example/users/checked")
	now := e.clock.Now()
	check := func(age time.Duration) string {
		t.Helper()
		at := now.Add(-age)
		e.clock.Set(at)
		defer e.clock.Set(now)
		v := account.HandleVerification{
			ID: e.ids.NewID(), AccountID: accountID, Username: "checked", Domain: "remote.example",
			Outcome: account.HandleVerified, CheckedAt: at,
		}
		err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.rh.Accounts.InsertHandleVerification(ctx, tx, v) })
		if err != nil {
			t.Fatalf("InsertHandleVerification = %v", err)
		}
		return v.ID
	}
	check(100 * retentionDay)
	newest := check(95 * retentionDay) // the newest of the account, however old

	pruner := appaccount.NewProfilePruner(appaccount.PruneDeps{
		UnitOfWork: e.rh.UnitOfWork, Repository: e.rh.Profiles, Relations: retentionRelations{e}, Clock: e.clock,
	})
	if _, _, err := pruner.Run(ctx, ""); err != nil {
		t.Fatalf("Run = %v", err)
	}
	err := e.rh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		last, err := e.rh.Accounts.LatestHandleVerification(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if last.ID != newest {
			t.Errorf("LatestHandleVerification = %s, want %s kept", last.ID, newest)
		}
		// The run took every check it could: older than 90 days and not the newest.
		n, err := e.rh.Profiles.DeleteHandleVerificationsBefore(ctx, tx, now.Add(-appaccount.HandleVerificationRetention), 10)
		if err == nil && n != 0 {
			t.Errorf("DeleteHandleVerificationsBefore after the run = %d, want 0", n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
