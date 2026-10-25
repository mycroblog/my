// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"slices"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/notification"
)

// stubClosureRecipients pages a fixed list of local accounts, two to a page.
type stubClosureRecipients struct{ ids []string }

func (r stubClosureRecipients) LocalUserAccountIDs(_ context.Context, after string, limit int) ([]string, string, error) {
	limit = min(limit, 2)
	var page []string
	for _, id := range r.ids {
		if id > after && len(page) < limit {
			page = append(page, id)
		}
	}
	next := ""
	if len(page) == limit && page[len(page)-1] != r.ids[len(r.ids)-1] {
		next = page[len(page)-1]
	}
	return page, next, nil
}

// stubAnnouncements has one announcement in effect.
type stubAnnouncements struct {
	id       string
	closesAt time.Time
}

func (a stubAnnouncements) AnnouncementInEffect(_ context.Context, id string) (time.Time, bool, error) {
	return a.closesAt, id == a.id, nil
}

// notificationServiceClosure checks the service closure notifications (ADR-0088 §1, §2): the
// announcement, the change of its dates, and the withdrawal each enqueue the job that tells
// every local user once with a service_closure, which names the event and what it did; and the
// job of the closing date tells the holders of manage_service_closure once with an
// admin_service_closure_due while the announcement is in effect and its date has passed.
func notificationServiceClosure(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.staff.set(alice, notification.PermissionManageServiceClosure)
	recipients := []string{alice, bob, carol}
	slices.Sort(recipients)
	announcement := e.ids.NewID()
	closesAt := e.clock.Now().Add(-time.Minute)
	outbox := &recordingOutbox{}
	d := appnotification.ClosureDeps{
		UnitOfWork: e.nh.UnitOfWork, Repository: e.nh.Notifications, Recipients: stubClosureRecipients{ids: recipients},
		Closures: stubAnnouncements{id: announcement, closesAt: closesAt}, IDs: e.ids, Clock: e.clock, Outbox: outbox,
		Moderators: appnotification.ModeratorDeps{Repository: e.nh.Notifications, Staff: e.staff, IDs: e.ids},
	}
	projectors, jobs := &uow.Projectors{}, &queue.JobHandlerRegistry{}
	appnotification.RegisterClosureProjectors(projectors, d)
	appnotification.RegisterClosureJobs(jobs, d)
	record := func(ev appinstance.ClosureRecorded) {
		t.Helper()
		if err := e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return projectors.Project(ctx, tx, ev) }); err != nil {
			t.Fatal(err)
		}
	}
	run := func() {
		t.Helper()
		for _, j := range outbox.take() {
			h, _, ok := jobs.Lookup(j.Type)
			if !ok {
				t.Fatalf("no handler of %s", j.Type)
			}
			// Each job runs twice: at least once delivery.
			for range 2 {
				if r := h(ctx, queue.Job{ID: e.ids.NewID(), Type: j.Type, Payload: j.Payload}); !r.IsDone() {
					t.Fatalf("%s = %v", j.Type, r.Err())
				}
			}
		}
	}
	at := e.clock.Now()
	record(appinstance.ClosureRecorded{
		Event:    instance.ClosureEvent{ID: announcement, Kind: instance.ClosureEventAnnounced, Schedule: instance.ClosureSchedule{ClosesAt: closesAt}, OccurredAt: at},
		Previous: instance.ClosureNone,
	})
	run()
	withdrawal := e.ids.NewID()
	record(appinstance.ClosureRecorded{Event: instance.ClosureEvent{ID: withdrawal, Kind: instance.ClosureEventWithdrawn, OccurredAt: at}, Previous: instance.ClosureAnnounced})
	run()

	for _, id := range recipients {
		ns := e.list(t, id, notification.TypeServiceClosure)
		if len(ns) != 2 || ns[0].ServiceClosureEventID != withdrawal || ns[0].ServiceClosureChange != notification.ClosureWithdrawn ||
			ns[1].ServiceClosureEventID != announcement || ns[1].ServiceClosureChange != notification.ClosureAnnounced || ns[1].SourceAccountID != "" {
			t.Errorf("service_closure of %s = %+v", id, ns)
		}
	}
	if ns := e.list(t, alice, notification.TypeAdminServiceClosureDue); len(ns) != 1 || ns[0].ServiceClosureEventID != announcement {
		t.Errorf("admin_service_closure_due of alice = %+v", ns)
	}
	if ns := e.list(t, bob, notification.TypeAdminServiceClosureDue); len(ns) != 0 {
		t.Errorf("admin_service_closure_due of bob, without the permission = %+v", ns)
	}
}
