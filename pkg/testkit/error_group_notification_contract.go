// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"testing"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
	"github.com/mycroblog/my/internal/domain/notification"
)

// notificationErrorGroups checks the admin_error_group notifications (ADR-0103 §8): a new
// group and a regression tell the holders of manage_errors, at most ErrorGroupHourlyCap times
// an hour, and the rest of the hour fold into one notification whose count grows with them; a
// group that a web report without a session created notifies only when a report with one
// confirms it, unless it is muted by then, and a regression from a report without a session
// notifies nobody; a new hour starts the count again; the prune deletes the old hours.
func notificationErrorGroups(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.staff.set(alice, notification.PermissionManageErrors)
	project := func(ev uow.Event) {
		t.Helper()
		if err := e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.only.Project(ctx, tx, ev) }); err != nil {
			t.Fatal(err)
		}
	}
	group := func(unauthenticated bool) appdiagnostics.Group {
		return appdiagnostics.Group{ID: e.ids.NewID(), Fingerprint: e.ids.NewID(), Source: "web", Kind: "error", Unauthenticated: unauthenticated, FirstSeenAt: e.clock.Now()}
	}
	list := func() []notification.Notification {
		t.Helper()
		return e.list(t, alice, notification.TypeAdminErrorGroup)
	}
	// The hour starts now, so that every event below falls in it.
	e.clock.Set(notification.ErrorGroupHour(e.clock.Now()).Add(time.Hour))

	anonymous := group(true)
	project(appdiagnostics.GroupCreated{Group: anonymous})
	project(appdiagnostics.GroupRegressed{
		Group: anonymous, Unauthenticated: true,
		Event: appdiagnostics.Event{ID: e.ids.NewID(), GroupID: anonymous.ID, Kind: diagnostics.EventRegressed, OccurredAt: e.clock.Now()},
	})
	muted := group(true)
	project(appdiagnostics.GroupCreated{Group: muted})
	project(appdiagnostics.GroupConfirmed{Group: muted, Head: appdiagnostics.Head{GroupID: muted.ID, State: diagnostics.StateMuted}, ConfirmedAt: e.clock.Now()})
	if ns := list(); len(ns) != 0 {
		t.Fatalf("reports without a session or of a muted group notified: %+v", ns)
	}
	project(appdiagnostics.GroupConfirmed{Group: anonymous, Head: appdiagnostics.Head{GroupID: anonymous.ID, State: diagnostics.StateOpen}, ConfirmedAt: e.clock.Now()})
	ns := list()
	if len(ns) != 1 || ns[0].ErrorGroupChange != notification.ErrorGroupCreated || ns[0].ErrorGroupID != anonymous.ID || ns[0].SourceAccountID != "" {
		t.Fatalf("confirmed group = %+v", ns)
	}

	var created []appdiagnostics.Group
	for range notification.ErrorGroupHourlyCap - 2 {
		e.clock.Advance(time.Second)
		g := group(false)
		created = append(created, g)
		project(appdiagnostics.GroupCreated{Group: g})
	}
	regression := appdiagnostics.Event{ID: e.ids.NewID(), GroupID: created[0].ID, Kind: diagnostics.EventRegressed, OccurredAt: e.clock.Now()}
	project(appdiagnostics.GroupRegressed{Group: created[0], Event: regression})
	ns = list()
	if len(ns) != notification.ErrorGroupHourlyCap || ns[0].ErrorGroupChange != notification.ErrorGroupRegressed || ns[0].ErrorGroupID != created[0].ID {
		t.Fatalf("under the cap: %d notifications, newest %+v; want %d, the regression first", len(ns), ns[0], notification.ErrorGroupHourlyCap)
	}
	// Beyond the cap the hour folds into one notification.
	for range 3 {
		e.clock.Advance(time.Second)
		project(appdiagnostics.GroupCreated{Group: group(false)})
	}
	ns = list()
	if len(ns) != notification.ErrorGroupHourlyCap+1 {
		t.Fatalf("beyond the cap: %d notifications, want %d", len(ns), notification.ErrorGroupHourlyCap+1)
	}
	others := ns[0]
	if others.ErrorGroupChange != notification.ErrorGroupOthers || others.ErrorGroupID != "" ||
		!others.ErrorGroupHour.Equal(notification.ErrorGroupHour(e.clock.Now())) {
		t.Errorf("the others = %+v", others)
	}
	if n, err := e.svc.ErrorGroupOthers(ctx, others); err != nil || n != 3 {
		t.Errorf("ErrorGroupOthers = %d, %v; want 3", n, err)
	}
	if n, err := e.svc.ErrorGroupOthers(ctx, ns[1]); err != nil || n != 0 {
		t.Errorf("ErrorGroupOthers of a single group = %d, %v; want 0", n, err)
	}
	if ns := e.list(t, bob, notification.TypeAdminErrorGroup); len(ns) != 0 {
		t.Errorf("bob, without manage_errors, got %+v", ns)
	}
	// A new hour starts the count again.
	e.clock.Advance(time.Hour)
	next := group(false)
	project(appdiagnostics.GroupCreated{Group: next})
	if ns := list(); len(ns) != notification.ErrorGroupHourlyCap+2 || ns[0].ErrorGroupID != next.ID {
		t.Errorf("the next hour: %d notifications, newest %+v", len(ns), ns[0])
	}

	// The prune deletes the notices of the hours older than the retention and keeps the rest:
	// the first hour is past it, the next hour is just within it.
	e.clock.Advance(notification.ErrorGroupNoticeRetention)
	if n, err := e.svc.PruneErrorGroupNotices(ctx); err != nil || n != 1 {
		t.Errorf("PruneErrorGroupNotices = %d, %v; want 1", n, err)
	}
	if n, err := e.svc.ErrorGroupOthers(ctx, others); err != nil || n != 0 {
		t.Errorf("ErrorGroupOthers of a pruned hour = %d, %v; want 0", n, err)
	}
	if n, err := e.svc.PruneErrorGroupNotices(ctx); err != nil || n != 0 {
		t.Errorf("PruneErrorGroupNotices again = %d, %v; want 0", n, err)
	}
}
