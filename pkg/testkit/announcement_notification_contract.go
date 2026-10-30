// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/notification"
)

// stubAnnouncementRecipients pages a fixed list of local accounts, two to a page, and leaves out
// the suspended ones.
type stubAnnouncementRecipients struct {
	stubClosureRecipients
	suspended []string
}

func (r stubAnnouncementRecipients) Notifiable(_ context.Context, _ uow.Tx, id string) (bool, error) {
	return !slices.Contains(r.suspended, id), nil
}

// stubShownAnnouncements holds announcements and answers, at the clock's now, which of them
// show.
type stubShownAnnouncements struct {
	mu    sync.Mutex
	clock kernel.Clock
	all   map[string]instance.Announcement
}

func (s *stubShownAnnouncements) put(a instance.Announcement) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all[a.ID] = a
}

func (s *stubShownAnnouncements) Announcement(_ context.Context, id string) (instance.Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.all[id]
	if !ok {
		return instance.Announcement{}, fmt.Errorf("announcement %s: %w", id, appinstance.ErrNotFound)
	}
	return a, nil
}

func (s *stubShownAnnouncements) ShownAnnouncementIDs(context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id, a := range s.all {
		if a.ShowsAt(s.clock.Now()) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// notificationAnnouncement checks the announcement notifications (ADR-0125 §2) with a fixed
// clock: scheduling an announcement enqueues its job at ShowsFrom; the job run before then
// notifies nobody and waits; once the window starts, every local user is notified once, however
// often the job runs, but a suspended one and one who turned announcements off in the app; and
// the notifications leave the list and the unread count once the announcement is withdrawn or
// its window ends.
func notificationAnnouncement(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol, dave := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol"), e.user(t, "dave")
	recipients := []string{alice, bob, carol, dave}
	slices.Sort(recipients)
	e.setting(t, dave, notification.GroupAnnouncement, notification.ChannelInApp, false)
	shown := &stubShownAnnouncements{clock: e.clock, all: map[string]instance.Announcement{}}
	outbox := &recordingOutbox{}
	d := appnotification.AnnouncementDeps{
		UnitOfWork: e.nh.UnitOfWork, Repository: e.nh.Notifications,
		Recipients:    stubAnnouncementRecipients{stubClosureRecipients: stubClosureRecipients{ids: recipients}, suspended: []string{carol}},
		Announcements: shown, IDs: e.ids, Clock: e.clock, Outbox: outbox,
	}
	svc := appnotification.NewService(appnotification.Deps{
		UnitOfWork: e.nh.UnitOfWork, Repository: e.nh.Notifications, Clock: e.clock, Staff: e.staff, Announcements: shown,
	})
	projectors, jobs := &uow.Projectors{}, &queue.JobHandlerRegistry{}
	appnotification.RegisterAnnouncementProjectors(projectors, d)
	appnotification.RegisterAnnouncementJobs(jobs, d)
	schedule := func(a instance.Announcement) queue.NewJob {
		t.Helper()
		shown.put(a)
		err := e.nh.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return projectors.Project(ctx, tx, appinstance.AnnouncementScheduled{AnnouncementID: a.ID, ShowsFrom: a.ShowsFrom()})
		})
		if err != nil {
			t.Fatal(err)
		}
		js := outbox.take()
		if len(js) != 1 || js[0].Type != appnotification.JobNotifyAnnouncement || !js[0].RunAt.Equal(a.ShowsFrom()) {
			t.Fatalf("jobs = %+v, want %s at %v", js, appnotification.JobNotifyAnnouncement, a.ShowsFrom())
		}
		return js[0]
	}
	run := func(j queue.NewJob) queue.Result {
		t.Helper()
		h, _, ok := jobs.Lookup(j.Type)
		if !ok {
			t.Fatalf("no handler of %s", j.Type)
		}
		return h(ctx, queue.Job{ID: e.ids.NewID(), Type: j.Type, Payload: j.Payload})
	}
	list := func(id string) []notification.Notification {
		t.Helper()
		ns, err := svc.List(ctx, id, appnotification.Query{Types: []notification.Type{notification.TypeAnnouncement}})
		if err != nil {
			t.Fatal(err)
		}
		return ns
	}
	unread := func(id string) int64 {
		t.Helper()
		n, err := svc.UnreadCount(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	// An announcement whose window starts in an hour.
	now := e.clock.Now()
	later := instance.Announcement{
		ID: e.ids.NewID(), State: instance.AnnouncementPublished, PublishedAt: now,
		Revision: instance.AnnouncementRevision{Title: "Maintenance", Body: "On Sunday.", StartsAt: now.Add(time.Hour)},
	}
	job := schedule(later)
	if r := run(job); !r.IsRetry() || r.After() != time.Hour {
		t.Fatalf("the job before the window = done %v, after %v, %v; want to wait an hour", r.IsDone(), r.After(), r.Err())
	}
	for _, id := range recipients {
		if ns := list(id); len(ns) != 0 {
			t.Errorf("announcements of %s before the window = %+v, want none", id, ns)
		}
	}
	e.clock.Advance(time.Hour)
	for range 2 { // at least once delivery
		if r := run(job); !r.IsDone() {
			t.Fatalf("the job in the window = %v", r.Err())
		}
	}
	for _, id := range []string{alice, bob} {
		if ns := list(id); len(ns) != 1 || ns[0].AnnouncementID != later.ID || ns[0].SourceAccountID != "" || unread(id) != 1 {
			t.Errorf("announcements of %s = %+v, want one", id, ns)
		}
	}
	if ns := list(carol); len(ns) != 0 {
		t.Errorf("announcements of the suspended carol = %+v, want none", ns)
	}
	if ns := list(dave); len(ns) != 0 {
		t.Errorf("announcements of dave, who turned them off = %+v, want none", ns)
	}

	// A revision that schedules it again (or a job left from before) notifies nobody twice.
	if r := run(job); !r.IsDone() || len(list(alice)) != 1 {
		t.Errorf("the job run again = %v, %+v", r.Err(), list(alice))
	}

	// A withdrawal hides the notifications from the list and the unread count.
	later.State = instance.AnnouncementWithdrawn
	shown.put(later)
	if ns := list(alice); len(ns) != 0 || unread(alice) != 0 {
		t.Errorf("announcements of alice after the withdrawal = %+v, %d unread; want none", ns, unread(alice))
	}
	if r := run(job); !r.IsDone() {
		t.Errorf("the job of a withdrawn announcement = %v", r.Err())
	}

	// So does the end of the window; a job that runs after the end notifies nobody.
	now = e.clock.Now()
	ending := instance.Announcement{
		ID: e.ids.NewID(), State: instance.AnnouncementPublished, PublishedAt: now,
		Revision: instance.AnnouncementRevision{Title: "Survey", Body: "Answer by Friday.", EndsAt: now.Add(time.Hour)},
	}
	if r := run(schedule(ending)); !r.IsDone() || len(list(bob)) != 1 {
		t.Fatalf("the job of the ending announcement = %v, %+v", r.Err(), list(bob))
	}
	e.clock.Advance(time.Hour)
	if ns := list(bob); len(ns) != 0 || unread(bob) != 0 {
		t.Errorf("announcements of bob after the end = %+v, want none", ns)
	}
	ended := instance.Announcement{
		ID: e.ids.NewID(), State: instance.AnnouncementPublished, PublishedAt: now,
		Revision: instance.AnnouncementRevision{Title: "Old", Body: "Gone.", EndsAt: now.Add(time.Minute)},
	}
	if r := run(schedule(ended)); !r.IsDone() {
		t.Errorf("the job of an ended announcement = %v", r.Err())
	}
	shown.put(instance.Announcement{ID: ended.ID, State: instance.AnnouncementPublished, PublishedAt: now, Revision: instance.AnnouncementRevision{Title: "Old", Body: "Gone."}})
	if ns := list(bob); len(ns) != 0 {
		t.Errorf("announcements of bob from an announcement that had ended when its job ran = %+v, want none", ns)
	}
}
