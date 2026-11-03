// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// fanOutEnv is a real server on SQLite whose wide fan-outs have jobs of two keys.
type fanOutEnv struct {
	*accountsEnv
	app *platform.App
}

func newFanOutEnv(t *testing.T) *fanOutEnv {
	t.Helper()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), FanOutSize: 2})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &fanOutEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, app: app}
}

func (e *fanOutEnv) member(name string) testUser {
	e.t.Helper()
	r, err := e.app.Identity.RegisterLocalUser(e.t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple"})
	if err != nil {
		e.t.Fatal(err)
	}
	return testUser{id: r.AccountID, token: issueTokens(e.t, e.app, r.User.ID, "").AccessToken}
}

// members returns n new accounts with names that start with prefix.
func (e *fanOutEnv) members(prefix string, n int) []testUser {
	e.t.Helper()
	out := make([]testUser, n)
	for i := range out {
		out[i] = e.member(fmt.Sprintf("%s%d", prefix, i))
	}
	return out
}

// pending returns the jobs of jobType waiting in the queue.
func (e *fanOutEnv) pending(jobType string) []queue.PendingJob {
	e.t.Helper()
	jobs, err := e.app.JobInspector.ListJobs(e.t.Context(), time.Now().Add(time.Hour), queue.JobFilter{Type: jobType}, queue.JobPosition{}, 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	return jobs
}

// handle runs the handler of j without the runner, so that j stays in the queue, as when its lease
// ended after it did its work.
func (e *fanOutEnv) handle(j queue.PendingJob) {
	e.t.Helper()
	h, _, ok := e.app.Handlers.Lookup(j.Type)
	if !ok {
		e.t.Fatalf("no handler of %s", j.Type)
	}
	if res := h(e.t.Context(), queue.Job{ID: j.ID, Type: j.Type, Payload: j.Payload}); res.Err() != nil {
		e.t.Fatalf("%s: %v", j.Type, res.Err())
	}
}

// drain runs every job.
func (e *fanOutEnv) drain() {
	e.t.Helper()
	if _, err := e.app.Runner.Drain(e.t.Context()); err != nil {
		e.t.Fatalf("Drain: %v", err)
	}
}

// count returns how many notifications of typ u has.
func (e *fanOutEnv) count(u testUser, typ clientapi.NotificationType) int {
	e.t.Helper()
	var page clientapi.NotificationPage
	e.want(e.do(http.MethodGet, "/notifications", u.token, nil, &page), http.StatusOK, "notifications")
	n := 0
	for _, item := range page.Items {
		if item.Type == typ {
			n++
		}
	}
	return n
}

// TestPostSubscribersWithinOneJob checks that up to the size of a job, the first job notifies the
// subscribers itself and enqueues nothing.
func TestPostSubscribersWithinOneJob(t *testing.T) {
	t.Parallel()
	for _, subscribers := range []int{1, 2} {
		t.Run(fmt.Sprint(subscribers), func(t *testing.T) {
			t.Parallel()
			e := newFanOutEnv(t)
			author := e.member("author")
			subs := e.members("sub", subscribers)
			for _, s := range subs {
				e.relate("subscribe", s, author, nil)
			}
			e.want(e.do(http.MethodPost, "/statuses", author.token, map[string]any{"text": "news", "visibility": "public"}, &clientapi.Status{}), http.StatusCreated, "post")
			jobs := e.pending(appnotification.JobNotifyPostSubscribers)
			if len(jobs) != 1 {
				t.Fatalf("jobs after the post = %d, want the first", len(jobs))
			}
			e.handle(jobs[0])
			if got := e.pending(appnotification.JobNotifyPostSubscribers); len(got) != 1 {
				t.Errorf("jobs after the first = %d, want no new ones", len(got))
			}
			for _, s := range subs {
				if n := e.count(s, clientapi.NotificationTypeStatus); n != 1 {
					t.Errorf("notifications = %d, want 1", n)
				}
			}
		})
	}
}

// TestPostSubscribersFanOut checks a post by an author with more subscribers than one job takes: the
// first job only enqueues ranges, running it again enqueues nothing more, a range run again notifies
// nobody twice, and a subscriber who leaves or arrives between the jobs is left out or reached.
func TestPostSubscribersFanOut(t *testing.T) {
	t.Parallel()
	e := newFanOutEnv(t)
	author := e.member("author")
	subs := e.members("sub", 5)
	for _, s := range subs {
		e.relate("subscribe", s, author, nil)
	}
	e.want(e.do(http.MethodPost, "/statuses", author.token, map[string]any{"text": "news", "visibility": "public"}, &clientapi.Status{}), http.StatusCreated, "post")

	jobs := e.pending(appnotification.JobNotifyPostSubscribers)
	if len(jobs) != 1 {
		t.Fatalf("jobs after the post = %d, want the first", len(jobs))
	}
	first := jobs[0]
	e.handle(first)
	notified := func() int {
		t.Helper()
		n := 0
		for _, s := range subs {
			n += e.count(s, clientapi.NotificationTypeStatus)
		}
		return n
	}
	if n := notified(); n != 0 {
		t.Errorf("the first job notified %d, want none", n)
	}
	jobs = e.pending(appnotification.JobNotifyPostSubscribers)
	if len(jobs) != 4 { // the first, and the ranges of 2, 2, and the rest
		t.Fatalf("jobs after the first = %d, want 4", len(jobs))
	}

	// The first job run again, as after its lease ended, enqueues no duplicates.
	e.handle(first)
	if got := e.pending(appnotification.JobNotifyPostSubscribers); len(got) != len(jobs) {
		t.Errorf("jobs after the first ran again = %d, want %d", len(got), len(jobs))
	}

	// A range run twice notifies its two subscribers once.
	var ranges []queue.PendingJob
	for _, j := range jobs {
		if j.ID != first.ID {
			ranges = append(ranges, j)
		}
	}
	for range 2 {
		e.handle(ranges[0])
		if n := notified(); n != 2 {
			t.Errorf("after a range: %d notified, want 2", n)
		}
	}

	// Between the jobs, one subscriber leaves and one arrives.
	left, arrived := subs[4], e.member("late")
	e.relate("unsubscribe", left, author, nil)
	e.relate("subscribe", arrived, author, nil)
	e.drain()

	if n := e.count(left, clientapi.NotificationTypeStatus); n != 0 {
		t.Errorf("the subscriber who left has %d notifications", n)
	}
	if n := e.count(arrived, clientapi.NotificationTypeStatus); n != 1 {
		t.Errorf("the subscriber who arrived has %d notifications, want 1", n)
	}
	for _, s := range subs[:4] {
		if n := e.count(s, clientapi.NotificationTypeStatus); n != 1 {
			t.Errorf("notifications = %d, want 1", n)
		}
	}
	if got := e.pending(appnotification.JobNotifyPostSubscribers); len(got) != 0 {
		t.Errorf("jobs left = %d", len(got))
	}
}

// TestAnnouncementFanOut checks that an announcement reaches every local user once through jobs of
// the size, also with the first job run again.
func TestAnnouncementFanOut(t *testing.T) {
	t.Parallel()
	e := newFanOutEnv(t)
	admin := e.user("admin")
	users := append(e.members("user", 5), admin)
	var a clientapi.AdminAnnouncement
	e.want(e.do(http.MethodPost, "/admin/announcements", admin.token, map[string]any{"title": "Hello", "text": "Hello"}, &a), http.StatusCreated, "create")
	e.want(e.do(http.MethodPost, "/admin/announcements/"+a.Id+"/publish", admin.token, nil, nil), http.StatusOK, "publish")

	jobs := e.pending(appnotification.JobNotifyAnnouncement)
	if len(jobs) != 1 {
		t.Fatalf("jobs after publishing = %d, want the first", len(jobs))
	}
	e.handle(jobs[0])
	wide := len(e.pending(appnotification.JobNotifyAnnouncement))
	if wide < 3 {
		t.Errorf("jobs after the first = %d, want ranges", wide)
	}
	e.handle(jobs[0])
	if got := len(e.pending(appnotification.JobNotifyAnnouncement)); got != wide {
		t.Errorf("jobs after the first ran again = %d, want %d", got, wide)
	}
	e.drain()
	for _, u := range users {
		if n := e.count(u, clientapi.NotificationTypeAnnouncement); n != 1 {
			t.Errorf("announcement notifications = %d, want 1", n)
		}
	}
}

// TestServiceClosureFanOut checks that the service closure notifications reach every local user once
// through jobs of the size.
func TestServiceClosureFanOut(t *testing.T) {
	t.Parallel()
	e := newFanOutEnv(t)
	admin := e.user("admin")
	users := append(e.members("user", 5), admin)
	closes := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Second)
	e.want(e.do(http.MethodPut, "/admin/closure/announcement", admin.token, map[string]any{"closes_at": closes}, &clientapi.AdminServiceClosure{}), http.StatusOK, "announce")

	jobs := e.pending(appnotification.JobServiceClosureFanout)
	if len(jobs) != 1 {
		t.Fatalf("jobs after the announcement = %d, want the first", len(jobs))
	}
	e.handle(jobs[0])
	if got := len(e.pending(appnotification.JobServiceClosureFanout)); got < 3 {
		t.Errorf("jobs after the first = %d, want ranges", got)
	}
	e.drain()
	for _, u := range users {
		if n := e.count(u, clientapi.NotificationTypeServiceClosure); n != 1 {
			t.Errorf("service closure notifications = %d, want 1", n)
		}
	}
}

// TestVoiceStartedFanOut checks that the start of a room reaches every follower once through jobs of
// the size, whose cursors go down.
func TestVoiceStartedFanOut(t *testing.T) {
	t.Parallel()
	e := newFanOutEnv(t)
	host := e.user("host")
	followers := e.members("follower", 5)
	for _, f := range followers {
		e.want(e.do(http.MethodPost, "/accounts/"+host.id+"/follow", f.token, nil, nil), http.StatusOK, "follow")
	}
	e.drain()
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Talk", "visibility": "followers"}, &clientapi.VoiceRoomJoin{}), http.StatusCreated, "open")

	jobs := e.pending(voice.JobNotifyStarted)
	if len(jobs) != 1 {
		t.Fatalf("jobs after opening = %d, want the first", len(jobs))
	}
	e.handle(jobs[0])
	if got := len(e.pending(voice.JobNotifyStarted)); got < 3 {
		t.Errorf("jobs after the first = %d, want ranges", got)
	}
	e.drain()
	for _, f := range followers {
		if n := e.count(f, clientapi.NotificationTypeVoiceRoomStarted); n != 1 {
			t.Errorf("voice_room_started notifications = %d, want 1", n)
		}
	}
}
