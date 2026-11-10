// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// mentionQueries counts, by operation, the database round trips of one post by alice that
// mentions n local accounts. Every query is one round trip (one call in pg_stat_statements).
func mentionQueries(t *testing.T, n int) map[string]int {
	t.Helper()
	tracer := testkit.NewTracer()
	users := []string{"alice"}
	var mentions []string
	for i := range n {
		users = append(users, fmt.Sprintf("u%d", i))
		mentions = append(mentions, fmt.Sprintf("@u%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
	was := ops(tracer)
	c.post("alice", map[string]any{"text": "hello " + strings.Join(mentions, " "), "visibility": "public"})
	out := map[string]int{}
	for k, v := range ops(tracer) {
		if d := v - was[k]; d > 0 {
			out[k] = d
		}
	}
	for _, u := range users[1:] {
		var page clientapi.NotificationPage
		if res := c.do(u, http.MethodGet, "/notifications?types[]=mention", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("%s has %d mention notifications (%d), want 1", u, len(page.Items), res.StatusCode)
		}
	}
	return out
}

// ops counts the queries a tracer recorded by operation.
func ops(tracer *testkit.Tracer) map[string]int {
	out := map[string]int{}
	for _, sp := range tracer.Spans() {
		if op := sp.Attrs["db.operation.name"]; op != "" {
			out[op]++
		}
	}
	return out
}

// notificationOps are the operations that notifying the recipients of a status makes: reading the
// recipients, their relations with the author, their settings and domain exclusions, and writing
// the notifications and their messages. Resolving the mentions of the text is #2695.
var notificationOps = []string{
	"ListAccountsByIDs", "ListLifecycleStatesByIDs", "GetBlockEdge", "GetActiveMute", "GetFollowEdge",
	"ListBlockEdgesFrom", "ListBlockEdgesTo", "ListActiveMutesTo", "ListFollowEdgesTo",
	"DomainExcluded", "DomainsExcludedAmong", "ListNotificationSettings", "ListNotificationSettingsOf",
	"InsertNotification", "InsertNotificationsFor", "ListPushSubscriptionIDsForAlert", "ListPushSubscriptionIDsForAlertOf",
}

// TestMentionNotificationQueries checks that notifying the accounts that a status mentions makes a
// number of round trips that does not grow with them, and notifies each of them.
func TestMentionNotificationQueries(t *testing.T) {
	t.Parallel()
	count := func(byOp map[string]int) int {
		n := 0
		for _, op := range notificationOps {
			n += byOp[op]
		}
		return n
	}
	small, large := count(mentionQueries(t, 5)), count(mentionQueries(t, 15))
	t.Logf("%d queries to notify 5 mentioned accounts, %d to notify 15", small, large)
	if large > small {
		t.Errorf("%d queries to notify 15 mentioned accounts, more than the %d for 5", large, small)
	}
}

// subscriberQueries counts the database round trips of the jobs that notify the n subscribers of
// alice of her new post, and checks that each of them gets the status notification.
func subscriberQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	users := []string{"alice"}
	for i := range n {
		users = append(users, fmt.Sprintf("s%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
	for _, u := range users[1:] {
		if res := c.do(u, http.MethodPost, "/accounts/"+c.ids["alice"]+"/subscribe", nil, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("%s subscribes = %d", u, res.StatusCode)
		}
	}
	c.post("alice", map[string]any{"text": "news", "visibility": "public"})
	h, _, _ := c.app.Handlers.Lookup(appnotification.JobNotifyPostSubscribers)
	count := 0
	done := map[string]bool{}
	for {
		var todo []queue.PendingJob
		for _, j := range mustJobs(t, c, appnotification.JobNotifyPostSubscribers) {
			if !done[j.ID] {
				todo = append(todo, j)
			}
		}
		if len(todo) == 0 {
			break
		}
		for _, j := range todo {
			done[j.ID] = true
			was := ops(tracer)
			if res := h(t.Context(), queue.Job{ID: j.ID, Type: j.Type, Payload: j.Payload}); res.Err() != nil {
				t.Fatalf("%s: %v", j.Type, res.Err())
			}
			for k, v := range ops(tracer) {
				count += v - was[k]
			}
		}
	}
	for _, u := range users[1:] {
		var page clientapi.NotificationPage
		if res := c.do(u, http.MethodGet, "/notifications?types[]=status", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("%s has %d status notifications (%d), want 1", u, len(page.Items), res.StatusCode)
		}
	}
	return count
}

// TestSubscriberNotificationQueries checks that notifying the subscribers of a post, a batch at a
// time, makes a number of round trips that does not grow with the subscribers of the batch.
func TestSubscriberNotificationQueries(t *testing.T) {
	t.Parallel()
	small, large := subscriberQueries(t, 10), subscriberQueries(t, 50)
	t.Logf("%d queries to notify 10 subscribers, %d to notify 50", small, large)
	if large > small {
		t.Errorf("%d queries to notify 50 subscribers, more than the %d for 10", large, small)
	}
}

// voiceStartedQueries counts the database round trips of the jobs that tell the n followers of a
// host that the host opened a room, and checks that each of them is told.
func voiceStartedQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), Tracer: tracer})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &fanOutEnv{accountsEnv: &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, app: app}
	host := e.user("host")
	followers := e.members("follower", n)
	for _, f := range followers {
		e.want(e.do(http.MethodPost, "/accounts/"+host.id+"/follow", f.token, nil, nil), http.StatusOK, "follow")
	}
	e.drain()
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Talk", "visibility": "followers"}, &clientapi.VoiceRoomJoin{}), http.StatusCreated, "open")
	count := 0
	done := map[string]bool{}
	for {
		var todo []queue.PendingJob
		for _, j := range e.pending(voice.JobNotifyStarted) {
			if !done[j.ID] {
				todo = append(todo, j)
			}
		}
		if len(todo) == 0 {
			break
		}
		for _, j := range todo {
			done[j.ID] = true
			was := ops(tracer)
			e.handle(j)
			for k, v := range ops(tracer) {
				count += v - was[k]
			}
		}
	}
	for _, f := range followers {
		if got := e.count(f, clientapi.NotificationTypeVoiceRoomStarted); got != 1 {
			t.Fatalf("voice_room_started notifications = %d, want 1", got)
		}
	}
	return count
}

// TestVoiceStartedNotificationQueries checks that telling the followers of a host that a room
// opened makes a number of round trips that does not grow with them.
func TestVoiceStartedNotificationQueries(t *testing.T) {
	t.Parallel()
	small, large := voiceStartedQueries(t, 5), voiceStartedQueries(t, 25)
	t.Logf("%d queries to tell 5 followers, %d to tell 25", small, large)
	if large > small {
		t.Errorf("%d queries to tell 25 followers, more than the %d for 5", large, small)
	}
}

// TestMentionQueries checks that posting a status that mentions local accounts makes a number of
// round trips that does not grow with its mentions: the handles are resolved, the mentions written,
// and their accounts notified together.
func TestMentionQueries(t *testing.T) {
	t.Parallel()
	total := func(byOp map[string]int) int {
		n := 0
		for _, c := range byOp {
			n += c
		}
		return n
	}
	small, large := total(mentionQueries(t, 5)), total(mentionQueries(t, 15))
	t.Logf("%d queries to post with 5 mentions, %d with 15", small, large)
	if large > small {
		t.Errorf("%d queries to post with 15 mentions, more than the %d with 5", large, small)
	}
}
