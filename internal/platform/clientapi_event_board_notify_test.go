// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"crypto/ecdh"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/gathering"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// eventBoardVAPIDKey is the server key of Web Push of the tests of the board's notifications.
const eventBoardVAPIDKey = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"

// eventBoardNotifyEnv is an eventBoardEnv that sends Web Push to a recorded push service and
// emails to a recording mailer.
type eventBoardNotifyEnv struct {
	*eventBoardEnv
	push *pushService
	mail *testkit.Mailer
}

func newEventBoardNotifyEnv(t *testing.T, users ...string) *eventBoardNotifyEnv {
	t.Helper()
	push, mail := &pushService{status: http.StatusCreated}, &testkit.Mailer{}
	clock := testkit.NewClock(eventBoardStart)
	c := newStatusesClientWith(t, map[string]string{
		"MY_QUEUE": "db", "MY_VAPID_PRIVATE_KEY": eventBoardVAPIDKey,
		"MY_MAIL": "smtp", "MY_MAIL_FROM": "noreply@example.test", "MY_MAIL_SMTP_HOST": "localhost", "MY_MAIL_SMTP_PORT": "2525",
	}, platform.AppOptions{Clock: clock, HTTPClient: push, Mailer: mail}, users...)
	return &eventBoardNotifyEnv{eventBoardEnv: &eventBoardEnv{statusesClient: c, clock: clock}, push: push, mail: mail}
}

// setGroup turns the channel of the notification settings' row group on or off for user.
func (e *eventBoardEnv) setGroup(user string, group clientapi.NotificationGroup, channel string, on bool) {
	e.t.Helper()
	body := map[string]any{"changes": []map[string]any{{"group": group, "channel": channel, "enabled": on}}}
	if res := e.send(user, http.MethodPatch, "/preferences/notifications", body, nil, nil); res.StatusCode != http.StatusOK {
		e.t.Fatalf("PATCH /preferences/notifications %s %s as %s = %s", group, channel, user, e.outcome(res))
	}
}

// boardNotices returns the event_board_posted notifications of user, newest first.
func (e *eventBoardEnv) boardNotices(user string) []domainnotification.Notification {
	e.t.Helper()
	page, err := e.app.Notifications.List(e.t.Context(), e.ids[user], appnotification.Query{Types: []domainnotification.Type{domainnotification.TypeEventBoardPosted}, Limit: 50})
	if err != nil {
		e.t.Fatal(err)
	}
	return page
}

// apiNotifications returns the notifications of user through the client API, newest first.
func (e *eventBoardEnv) apiNotifications(user string) []clientapi.Notification {
	e.t.Helper()
	var page clientapi.NotificationPage
	if res := e.send(user, http.MethodGet, "/notifications?limit=40", nil, nil, &page); res.StatusCode != http.StatusOK {
		e.t.Fatalf("GET /notifications as %s = %s", user, e.outcome(res))
	}
	return page.Items
}

// subscribePush subscribes the token of user to Web Push of the alerts, and returns the keys that
// open its messages.
func (e *eventBoardEnv) subscribePush(user string, alerts ...string) (*ecdh.PrivateKey, []byte) {
	e.t.Helper()
	ua, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		e.t.Fatal(err)
	}
	auth := []byte("0123456789abcdef")
	body := map[string]any{
		"endpoint": "https://push.example.net/send/" + user,
		"keys":     map[string]string{"p256dh": base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()), "auth": base64.RawURLEncoding.EncodeToString(auth)},
		"alerts":   alerts,
	}
	if res := e.send(user, http.MethodPut, "/push/subscription", body, nil, nil); res.StatusCode != http.StatusOK {
		e.t.Fatalf("PUT /push/subscription as %s = %s", user, e.outcome(res))
	}
	return ua, auth
}

// TestEventBoardPostNotifiesItsReaders checks ADR-0161 §11 and §17 for a new post on a board: the
// readers who answered going or maybe get one event_board_posted, with the writer, the post, and
// the event, and by Web Push; the writer, the organizer, a co-organizer who did not answer, who declined,
// an invitee, a stranger, a mentioned reader (who gets the mention), and a reader who turned the
// Events row off in the app get none; nor do the readers who, before the job runs, change to not
// going, block the organizer, or block the writer. An edit of the post notifies nobody again.
func TestEventBoardPostNotifiesItsReaders(t *testing.T) {
	t.Parallel()
	users := []string{"alice", "cora", "gina", "mona", "max", "dan", "ivy", "sam", "will", "ben", "kim", "olga"}
	e := newEventBoardNotifyEnv(t, users...)
	ctx := t.Context()
	ev := e.createEvent("readers", map[string]any{
		"places": []map[string]any{{"kind": "physical", "name": "Secretplace Cafe", "address": "1-2-3 Hiddenstreet"}},
	})
	e.cast(ev.Id)
	for _, name := range []string{"max", "will", "ben", "kim", "olga"} {
		e.answer(name, ev.Id, gathering.AnswerGoing)
	}
	e.drain()
	e.setGroup("olga", clientapi.NotificationGroupEvent, "in_app", false)
	ua, auth := e.subscribePush("mona", string(clientapi.NotificationTypeEventBoardPosted), string(clientapi.NotificationTypeMention))

	const body = "boardbodyneedle bring snacks"
	post := e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": body + " @max"})
	// The readers are notified by the job gathering.notify of the post.
	found := false
	for _, j := range e.pendingJobs() {
		if j.Type == appgathering.JobNotify && strings.HasPrefix(j.IdempotencyKey, "gathering.notify:board_posted:"+post.Id) {
			found = true
		}
	}
	if !found {
		t.Errorf("the post enqueues no %s keyed gathering.notify:board_posted:%s", appgathering.JobNotify, post.Id)
	}
	// Before the job runs: will no longer goes, ben blocks the organizer, kim blocks the writer.
	e.answer("will", ev.Id, gathering.AnswerDeclined)
	if _, err := e.app.Graph.Block(ctx, e.ids["ben"], e.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Graph.Block(ctx, e.ids["kim"], e.ids["gina"]); err != nil {
		t.Fatal(err)
	}
	e.drain()

	for _, user := range users {
		want := 0
		if user == "mona" {
			want = 1
		}
		got := e.boardNotices(user)
		if len(got) != want {
			t.Errorf("%s: %d event_board_posted %+v, want %d", user, len(got), got, want)
			continue
		}
		for _, n := range got {
			if n.StatusID != post.Id || n.SourceAccountID != e.ids["gina"] || n.GatheringID != ev.Id || n.DedupeKey != domainnotification.EventBoardPostedKey(post.Id) {
				t.Errorf("%s: event_board_posted %+v, want the post %s of gina on %s keyed %s", user, n, post.Id, ev.Id, domainnotification.EventBoardPostedKey(post.Id))
			}
		}
	}
	// Through the API, with the writer, the post, and the event.
	var notice *clientapi.Notification
	for _, n := range e.apiNotifications("mona") {
		if n.Type == clientapi.NotificationTypeEventBoardPosted {
			notice = &n
			break
		}
	}
	if notice == nil {
		t.Fatal("mona has no event_board_posted through the API")
	}
	if notice.Account == nil || notice.Account.Id != e.ids["gina"] || notice.Status == nil || notice.Status.Id != post.Id ||
		notice.EventId == nil || *notice.EventId != ev.Id || notice.Event == nil || notice.Event.Id != ev.Id {
		t.Errorf("mona's event_board_posted = %+v, want the account gina, the post %s, and the event %s", notice, post.Id, ev.Id)
	}
	// The mentioned reader gets the mention alone.
	mentioned := false
	for _, n := range e.apiNotifications("max") {
		switch {
		case n.Type == clientapi.NotificationTypeMention && n.Status != nil && n.Status.Id == post.Id:
			mentioned = true
		case n.Type == clientapi.NotificationTypeEventBoardPosted:
			t.Errorf("the mentioned reader gets %+v", n)
		}
	}
	if !mentioned {
		t.Error("the mentioned reader gets no mention of the post")
	}

	// Web Push: one message to mona's subscription, which opens the notification's page.
	var pushed []appnotification.PushPayload
	for _, r := range e.push.sent() {
		if r.URL == "https://push.example.net/send/mona" {
			pushed = append(pushed, openPushMessage(t, r.Body, ua, auth))
		}
	}
	if len(pushed) != 1 {
		t.Fatalf("%d push messages to mona %+v, want 1", len(pushed), pushed)
	}
	if p := pushed[0]; p.Type != domainnotification.TypeEventBoardPosted || p.NotificationID != notice.Id || p.Path != "/notifications/"+notice.Id ||
		p.Title == "" || strings.Contains(p.Title, "boardbodyneedle") || strings.Contains(p.Title, "Secretplace") {
		t.Errorf("the push message = %+v, want the event_board_posted %s, its page, and a title without the post and the place", p, notice.Id)
	}

	// An edit of the post is no new post.
	if res := e.send("gina", http.MethodPut, "/statuses/"+post.Id, map[string]any{"text": body + ", edited @max"}, nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("edit the board post = %s", e.outcome(res))
	}
	e.drain()
	if got := e.boardNotices("mona"); len(got) != 1 {
		t.Errorf("after an edit, mona has %d event_board_posted, want 1", len(got))
	}
}

// TestEventBoardPostOfACancelledEventNotifiesNobody checks that a post on a board whose event is
// cancelled before the job of its notifications runs notifies none of its readers (ADR-0161 §11).
func TestEventBoardPostOfACancelledEventNotifiesNobody(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina", "mona")
	ev := e.createEvent("readers", nil)
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	e.answer("mona", ev.Id, gathering.AnswerMaybe)
	e.drain()
	e.mustPostOnBoard("alice", ev.Id, map[string]any{"text": "see you"})
	e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": "I bring the cards"})
	if _, err := e.app.Events.Cancel(t.Context(), e.ids["alice"], ev.Id, ""); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	e.drain()
	for _, user := range []string{"alice", "gina", "mona"} {
		if got := e.boardNotices(user); len(got) != 0 {
			t.Errorf("%s: event_board_posted %+v after the cancellation, want none", user, got)
		}
	}

	// The same posts on an event that goes on notify its readers, so this test sees them.
	other := e.createEvent("readers", nil)
	e.answer("gina", other.Id, gathering.AnswerGoing)
	e.answer("mona", other.Id, gathering.AnswerMaybe)
	e.drain()
	post := e.mustPostOnBoard("alice", other.Id, map[string]any{"text": "see you"})
	e.drain()
	for _, user := range []string{"gina", "mona"} {
		if got := e.boardNotices(user); len(got) != 1 || got[0].StatusID != post.Id {
			t.Errorf("%s: event_board_posted %+v, want the post %s", user, got, post.Id)
		}
	}
}
