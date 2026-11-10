// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/notification"
)

// stubFilters judges the statuses of verdicts.
type stubFilters map[string]StatusVerdict

func (f stubFilters) FilterStatuses(_ context.Context, _ string, ids []string) (map[string]StatusVerdict, error) {
	out := map[string]StatusVerdict{}
	for _, id := range ids {
		if v, ok := f[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

// TestPushAppliesKeywordFilters checks that a hide filter drops the message of a notification
// and that a warn filter words it by the filter's title.
func TestPushAppliesKeywordFilters(t *testing.T) {
	const (
		alice    = "01890a5d-ac96-774b-bcce-b30209900001"
		bob      = "01890a5d-ac96-774b-bcce-b30209900002"
		notified = "01890a5d-ac96-774b-bcce-b30209900003"
		hidden   = "01890a5d-ac96-774b-bcce-b30209900004"
		warned   = "01890a5d-ac96-774b-bcce-b30209900005"
	)
	sub := notification.PushSubscription{ID: "sub", AccountID: alice, TokenFamilyID: "family", Alerts: []notification.Type{notification.TypeMention}}
	filters := stubFilters{hidden: {Hidden: true}, warned: {Warned: []string{"Spoilers", "Other"}}}
	audience := stubAudience{visible: map[string]bool{hidden: true, warned: true}}
	for _, tc := range []struct {
		status string
		sent   int
		title  string
	}{
		{hidden, 0, ""},
		{warned, 1, "“Spoilers”"},
	} {
		sender := &recordingPushSender{}
		s := NewPushService(PushDeps{
			Repository: &stubPushRepository{sub: sub}, Sender: sender, Audience: audience,
			Notifications: stubListed{listed: map[string]bool{notified: true}}, Host: "social.example", Filters: filters,
		})
		s.push(t.Context(), pushJob{SubscriptionID: "sub", NotificationID: notified, AccountID: alice, Type: notification.TypeMention, SourceAccountID: bob, StatusID: tc.status})
		if len(sender.sent) != tc.sent {
			t.Fatalf("%s: %d messages sent, want %d", tc.status, len(sender.sent), tc.sent)
		}
		if tc.sent == 0 {
			continue
		}
		var p PushPayload
		if err := json.Unmarshal(sender.sent[0].Payload, &p); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(p.Title, tc.title) || strings.Contains(p.Title, "Other") || !strings.Contains(p.Title, bob) {
			t.Errorf("title = %q, want who mentioned and the first filter", p.Title)
		}
	}
}

// unreadRepository counts the unread notifications and lists their statuses as given.
type unreadRepository struct {
	Repository
	count    int64
	statuses []string
}

func (r unreadRepository) FindReadMarker(context.Context, string) (notification.ReadMarker, error) {
	return notification.ReadMarker{}, ErrNotFound
}

func (r unreadRepository) CountNotificationsAfter(context.Context, string, string, Hidden, time.Time) (int64, error) {
	return r.count, nil
}

func (r unreadRepository) UnreadNotificationStatuses(_ context.Context, _, _ string, _ Hidden, _ time.Time, limit int) ([]string, error) {
	if limit != maxFilteredUnread {
		return nil, nil
	}
	return r.statuses, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// TestUnreadCountLeavesOutHiddenNotifications checks that the unread count does not count the
// notifications whose status a hide filter matches, once per notification.
func TestUnreadCountLeavesOutHiddenNotifications(t *testing.T) {
	repo := unreadRepository{count: 5, statuses: []string{"hidden", "warned", "hidden", "plain"}}
	filters := stubFilters{"hidden": {Hidden: true}, "warned": {Warned: []string{"x"}}}
	s := NewService(Deps{Repository: repo, Clock: fixedClock{time.Now()}, Filters: filters})
	if n, err := s.UnreadCount(t.Context(), "alice", true); err != nil || n != 3 {
		t.Errorf("UnreadCount = %d, %v; want 3", n, err)
	}
	s = NewService(Deps{Repository: repo, Clock: fixedClock{time.Now()}})
	if n, err := s.UnreadCount(t.Context(), "alice", true); err != nil || n != 5 {
		t.Errorf("UnreadCount without filters = %d, %v; want 5", n, err)
	}
}
