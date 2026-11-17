// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
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
	visible     map[string]bool
	err         error
	deactivated bool
}

func (stubAudience) GrantActive(context.Context, string) (bool, error) { return true, nil }
func (a stubAudience) Deactivated(context.Context, string) (bool, error) {
	return a.deactivated, nil
}
func (stubAudience) Locale(context.Context, string) (string, error) { return "en", nil }
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
		{"RecipientDeactivated", mention, stubAudience{deactivated: true, visible: map[string]bool{statusID: true}}, stubListed{listed: map[string]bool{notified: true}}, 0, false},
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

func TestAnnouncementPath(t *testing.T) {
	const id = "01a18bbc-4663-71ea-a2b8-f3f169979c0a"
	if got, want := AnnouncementPath(id), "/announcements/"+id; got != want {
		t.Errorf("AnnouncementPath = %q, want %q", got, want)
	}
}

// TestDescribeScheduledStatusFailed checks that the Web Push message of a scheduled post that
// could not be published says why in plain words, for each reason (ADR-0156 §8).
func TestDescribeScheduledStatusFailed(t *testing.T) {
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, loc := range []string{"en", "ja"} {
		l := b.Localizer(loc)
		seen := map[string]bool{}
		// The reasons of content.DraftFailureReason.
		for _, reason := range []string{"restricted", "reply_unavailable", "quote_unavailable", "media_unavailable", "invalid", "rejected"} {
			p := DescribeNotification(l, Wording{Type: notification.TypeScheduledStatusFailed, DraftFailureReason: reason}, "example.com")
			if !strings.HasPrefix(p.Title, "example.com") || seen[p.Title] || p.Path != "/notifications" {
				t.Errorf("%s, %s: %+v; want a title of its own that names the server, to the notifications", loc, reason, p)
			}
			seen[p.Title] = true
		}
	}
}

// TestPushPathOfEveryType checks that the Web Push message and the email of each notification
// type lead to the web client's page of it, as its notificationPath decides for the lists
// (web/app-components/src/paths.ts): the post, the account, the follow requests, what waits for
// approval, the announcement, and the notifications for a kind whose screen the client lacks.
func TestPushPathOfEveryType(t *testing.T) {
	const (
		me     = "me"
		bob    = "bob@remote.example"
		carol  = "carol"
		status = "01890a5d-ac96-774b-bcce-b30209900010"
		ann    = "01890a5d-ac96-774b-bcce-b30209900011"
	)
	byBob := "/@" + bob + "/status/" + status
	mine := "/@" + me + "/status/" + status
	want := map[notification.Type]string{
		notification.TypeFollow:                     "/@" + bob,
		notification.TypeFollowRequest:              "/notifications/follow_requests",
		notification.TypeFollowAccepted:             "/@" + bob,
		notification.TypeMention:                    byBob,
		notification.TypeStatus:                     byBob,
		notification.TypeBoost:                      mine,
		notification.TypeFavourite:                  mine,
		notification.TypeReaction:                   mine,
		notification.TypeQuote:                      byBob,
		notification.TypePollClosed:                 byBob,
		notification.TypeReplyRequest:               "/notifications/waiting",
		notification.TypeBoostRequest:               "/notifications/waiting",
		notification.TypeQuoteRequest:               "/notifications/waiting",
		notification.TypeInteractionAccepted:        byBob,
		notification.TypeInteractionRejected:        byBob,
		notification.TypeAccountMoved:               "/@" + carol,
		notification.TypeAnnouncement:               "/announcements/" + ann,
		notification.TypeModerationNotice:           "/notifications",
		notification.TypeReportResolved:             "/notifications",
		notification.TypeAdminReport:                "/notifications",
		notification.TypeAdminSignUp:                "/notifications",
		notification.TypeRelationshipsSevered:       "/notifications",
		notification.TypeAppealDecided:              "/notifications",
		notification.TypeAdminAppeal:                "/notifications",
		notification.TypeAdminWebhookDisabled:       "/notifications",
		notification.TypeAdminCustomEmojiRequest:    "/notifications",
		notification.TypeCustomEmojiRequestDecided:  "/notifications",
		notification.TypeSupporterPaymentFailed:     "/notifications",
		notification.TypeSupporterSubscriptionEnded: "/notifications",
		notification.TypeSupporterPlanRetired:       "/notifications",
		notification.TypeAdminAccountMoved:          "/notifications",
		notification.TypeServiceClosure:             "/notifications",
		notification.TypeAdminServiceClosureDue:     "/notifications",
		notification.TypeSupporterServiceClosure:    "/notifications",
		notification.TypeVoiceRoomStarted:           "/notifications",
		notification.TypeVoiceRoomInvited:           "/notifications",
		notification.TypeAdminErrorGroup:            "/notifications",
		notification.TypeAdminHookFailing:           "/notifications",
		notification.TypeAdminHookAppeal:            "/notifications",
		notification.TypeHookAppealDecided:          "/notifications",
		notification.TypeAutoDeleteLimit:            "/notifications",
		notification.TypeScheduledStatusFailed:      "/notifications",
		notification.TypeAdminDomainListFailing:     "/notifications",
		notification.TypeEventInvited:               "/notifications",
		notification.TypeEventScheduleConfirmed:     "/notifications",
		notification.TypeEventChanged:               "/notifications",
		notification.TypeEventCancelled:             "/notifications",
		notification.TypeEventReminder:              "/notifications",
		notification.TypeEventStarted:               "/notifications",
		notification.TypeEventRoomUnavailable:       "/notifications",
	}
	d := describer{audience: stubAudience{}, host: "example.com"}
	for _, typ := range notification.Types {
		t.Run(string(typ), func(t *testing.T) {
			path, ok := want[typ]
			if !ok {
				t.Fatalf("no page for %s: add the web client's page of it", typ)
			}
			job := pushJob{
				NotificationID: "n", AccountID: me, Type: typ, SourceAccountID: bob, StatusID: status,
				ReportID: "r", AppealID: "a", VoiceRoomID: "v", ErrorGroupID: "e", HookAppealID: "h",
				AnnouncementID: ann, GatheringID: "g",
			}
			if typ == notification.TypeAccountMoved {
				job.MoveTargetAccountID = carol
			}
			p, _, err := d.describe(t.Context(), job)
			if err != nil {
				t.Fatal(err)
			}
			if p.Path != path {
				t.Errorf("path = %q, want %q", p.Path, path)
			}
		})
	}
}

// TestPushPathOfAccountMovedWithoutTarget checks that an account_moved whose job does not name
// the account moved to, as one enqueued before it did, leads to the moved account.
func TestPushPathOfAccountMovedWithoutTarget(t *testing.T) {
	d := describer{audience: stubAudience{}, host: "example.com"}
	p, _, err := d.describe(t.Context(), pushJob{NotificationID: "n", AccountID: "me", Type: notification.TypeAccountMoved, SourceAccountID: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != "/@bob" {
		t.Errorf("path = %q, want /@bob", p.Path)
	}
}
