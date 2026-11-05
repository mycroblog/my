// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeEmojiRequestsByID answers the custom emoji requests that notifications read.
type fakeEmojiRequestsByID struct {
	clientapi.CustomEmojiRequests
	reqs []appcontent.CustomEmojiRequestView
}

func (f fakeEmojiRequestsByID) RequestsByID(_ context.Context, ids []string) ([]appcontent.CustomEmojiRequestView, error) {
	var out []appcontent.CustomEmojiRequestView
	for _, r := range f.reqs {
		if slices.Contains(ids, r.Request.ID) {
			out = append(out, r)
		}
	}
	return out, nil
}

// TestNotificationObjectsNeedTheirScope checks what read:notifications shows of each object that a
// notification carries: the status always, an object with a scope of its own only with that scope,
// and a moderation action only to a first-party client.
func TestNotificationObjectsNeedTheirScope(t *testing.T) {
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	emojiRequestID := testkit.NamedID("emoji-request")
	severanceID := testkit.NamedID("severance")
	e, f, _ := newNotificationEnvWithViews(t, func(o *clientapi.Options) {
		o.FiledReports = &fakeFiledReports{recs: []moderation.ReportRecord{{
			Report: moderation.Report{
				ID: filedReportID(1), ReporterAccountID: aliceAccountID, TargetAccountID: bobAccountID, Category: moderation.CategorySpam,
				Comment: "spam", FiledAt: at,
			},
			Status: moderation.ReportStatus{ReportID: filedReportID(1), State: moderation.ReportResolved, Since: at},
		}}}
		o.Severances = fakeSeverances{severanceID: {
			Severance:       moderation.Severance{ID: severanceID, ActionID: testkit.NamedID("action"), AccountID: aliceAccountID, FollowersLost: 1},
			TargetAccountID: bobAccountID,
		}}
		o.OwnSupporters = &fakeOwnSupporters{sub: activeSubscription()}
		req := appcontent.CustomEmojiRequestView{}
		req.Request = content.CustomEmojiRequest{ID: emojiRequestID, Kind: content.RequestCreate, Shortcode: "blob", CreatedAt: at}
		o.CustomEmojiRequests = fakeEmojiRequestsByID{reqs: []appcontent.CustomEmojiRequestView{req}}
		o.Voice = &fakeVoiceRooms{}
	})
	add := func(i int, n notification.Notification) {
		n.ID, n.RecipientAccountID, n.CreatedAt = notificationID(i), aliceAccountID, at
		f.ns = append(f.ns, n)
	}
	add(10, notification.Notification{Type: notification.TypeReportResolved, ReportID: filedReportID(1)})
	add(11, notification.Notification{Type: notification.TypeRelationshipsSevered, SeveranceID: severanceID})
	add(12, notification.Notification{Type: notification.TypeSupporterPaymentFailed, SupporterSubscriptionID: ownSubID})
	add(13, notification.Notification{Type: notification.TypeCustomEmojiRequestDecided, CustomEmojiRequestID: emojiRequestID})
	add(14, notification.Notification{Type: notification.TypeVoiceRoomStarted, VoiceRoomID: voiceRoomID})

	for _, tc := range []struct {
		name  string
		token string
		// want lists the objects shown besides the status, which every token sees.
		want []string
	}{
		{"read:notifications alone", "narrow", nil},
		{"each read scope", "scoped", []string{"report", "severance", "supporter", "emoji", "voice"}},
		{"a first-party client", "first-party", []string{"action", "report", "severance", "supporter", "emoji", "voice"}},
		{"a first-party client with read:notifications alone", "first-party-narrow", []string{"action"}},
		{"another client with the broad read scope", "third-party-read", []string{"report", "severance", "supporter", "emoji", "voice"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e.auth.tokens["narrow"] = thirdParty(bearer(identity.ScopeReadNotifications+" "+identity.ScopeReadStatuses, aliceAccountID))
			e.auth.tokens["scoped"] = thirdParty(bearer(identity.Scope(identity.ScopeReadNotifications+" "+identity.ScopeReadStatuses+" "+identity.ScopeReadReports+" "+
				identity.ScopeReadFollows+" "+identity.ScopeReadSupporters+" "+identity.ScopeReadCustomEmojis+" "+identity.ScopeReadVoice), aliceAccountID))
			e.auth.tokens["first-party"] = bearer(identity.ScopeRead, aliceAccountID)
			e.auth.tokens["first-party-narrow"] = bearer(identity.ScopeReadNotifications+" "+identity.ScopeReadStatuses, aliceAccountID)
			e.auth.tokens["third-party-read"] = thirdParty(bearer(identity.ScopeRead, aliceAccountID))

			res := e.send(t, http.MethodGet, "/notifications", tc.token, "")
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.StatusCode)
			}
			got := []string{}
			status := false
			for _, n := range decode[clientapi.NotificationPage](t, res).Items {
				for name, shown := range map[string]bool{
					"action": n.ModerationAction != nil, "report": n.Report != nil, "severance": n.RelationshipSeverance != nil,
					"supporter": n.SupporterSubscription != nil, "emoji": n.CustomEmojiRequest != nil, "voice": n.VoiceRoom != nil,
				} {
					if shown {
						got = append(got, name)
					}
				}
				status = status || n.Status != nil
				if n.Type == clientapi.NotificationTypeVoiceRoomStarted && n.VoiceRoomId == nil {
					t.Errorf("voice_room_started lost its voice_room_id")
				}
			}
			if !status {
				t.Errorf("no notification shows its status")
			}
			slices.Sort(got)
			want := slices.Clone(tc.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("objects shown = %v, want %v", got, want)
			}
		})
	}
}
