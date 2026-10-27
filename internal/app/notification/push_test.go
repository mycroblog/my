// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/notification"
)

type stubPushRepository struct {
	PushRepository
	sub notification.PushSubscription
}

func (r *stubPushRepository) FindPushSubscription(_ context.Context, id string) (notification.PushSubscription, error) {
	if id != r.sub.ID {
		return notification.PushSubscription{}, ErrNotFound
	}
	return r.sub, nil
}

type recordingPushSender struct{ sent []PushMessage }

func (s *recordingPushSender) ServerKey() string { return "key" }

func (s *recordingPushSender) Send(_ context.Context, _ notification.PushSubscription, m PushMessage) (PushResult, error) {
	s.sent = append(s.sent, m)
	return PushResult{Outcome: PushDelivered, StatusCode: 201}, nil
}

// stubAudience sees the statuses of visible.
type stubAudience struct {
	visible map[string]bool
	err     error
}

func (stubAudience) GrantActive(context.Context, string) (bool, error) { return true, nil }
func (stubAudience) Locale(context.Context, string) (string, error)    { return "en", nil }
func (stubAudience) Account(_ context.Context, id string) (PushAccount, error) {
	return PushAccount{Acct: id}, nil
}

func (a stubAudience) StatusVisible(_ context.Context, _, statusID string) (bool, error) {
	return a.visible[statusID], a.err
}

// stubListed lists the notifications of listed.
type stubListed struct {
	listed map[string]bool
	err    error
}

func (l stubListed) Listed(_ context.Context, _, id string) (bool, error) { return l.listed[id], l.err }

// TestPushRechecksAtSend checks that the push job sends a notification's message only while
// the recipient still sees the notification in the list and still may see its status.
func TestPushRechecksAtSend(t *testing.T) {
	const (
		alice    = "01890a5d-ac96-774b-bcce-b30209900001"
		bob      = "01890a5d-ac96-774b-bcce-b30209900002"
		notified = "01890a5d-ac96-774b-bcce-b30209900003"
		statusID = "01890a5d-ac96-774b-bcce-b30209900004"
	)
	sub := notification.PushSubscription{ID: "sub", AccountID: alice, TokenFamilyID: "family", Alerts: []notification.Type{notification.TypeMention, notification.TypeFollow}}
	mention := pushJob{SubscriptionID: "sub", NotificationID: notified, AccountID: alice, Type: notification.TypeMention, SourceAccountID: bob, StatusID: statusID}
	follow := pushJob{SubscriptionID: "sub", NotificationID: notified, AccountID: alice, Type: notification.TypeFollow, SourceAccountID: bob}
	failure := errors.New("unavailable")
	cases := []struct {
		name     string
		job      pushJob
		audience stubAudience
		listed   stubListed
		sent     int
		retry    bool
	}{
		{"Visible", mention, stubAudience{visible: map[string]bool{statusID: true}}, stubListed{listed: map[string]bool{notified: true}}, 1, false},
		{"StatusNoLongerVisible", mention, stubAudience{}, stubListed{listed: map[string]bool{notified: true}}, 0, false},
		{"NoLongerListed", mention, stubAudience{visible: map[string]bool{statusID: true}}, stubListed{}, 0, false},
		{"FollowNoLongerListed", follow, stubAudience{}, stubListed{}, 0, false},
		{"FollowListed", follow, stubAudience{}, stubListed{listed: map[string]bool{notified: true}}, 1, false},
		{"ListedFails", mention, stubAudience{visible: map[string]bool{statusID: true}}, stubListed{err: failure}, 0, true},
		{"VisibilityFails", mention, stubAudience{err: failure}, stubListed{listed: map[string]bool{notified: true}}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := &recordingPushSender{}
			s := NewPushService(PushDeps{
				Repository: &stubPushRepository{sub: sub}, Sender: sender, Audience: tc.audience, Notifications: tc.listed, Host: "social.example",
			})
			res := s.push(t.Context(), tc.job)
			if len(sender.sent) != tc.sent {
				t.Errorf("%d messages sent, want %d", len(sender.sent), tc.sent)
			}
			if retried := res != queue.Done(); retried != tc.retry {
				t.Errorf("result = %+v, want a retry: %v", res, tc.retry)
			}
		})
	}
}
