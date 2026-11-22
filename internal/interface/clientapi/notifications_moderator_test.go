// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// moderatorNotifications is fakeNotifications whose Get leaves the admin_report notifications out
// when asked to leave the moderator notifications out, as appnotification.Service does.
type moderatorNotifications struct{ *fakeNotifications }

func (f moderatorNotifications) Get(ctx context.Context, accountID, id string, withoutModerator bool) (notification.Notification, error) {
	n, err := f.fakeNotifications.Get(ctx, accountID, id, withoutModerator)
	if err == nil && withoutModerator && n.Type == notification.TypeAdminReport {
		return notification.Notification{}, appnotification.ErrNotFound
	}
	return n, err
}

// TestThirdPartyClientsDoNotReadModeratorNotifications checks that a third-party client, which
// does not get the moderator notifications, can neither mark one read nor dismiss one: both answer
// 404 and change nothing, while the first-party client does both.
func TestThirdPartyClientsDoNotReadModeratorNotifications(t *testing.T) {
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	f := &fakeNotifications{dismissed: map[string]bool{}, markers: map[string]string{}, ns: []notification.Notification{
		{ID: notificationID(1), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminReport, ReportID: notificationID(7), CreatedAt: at},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Notifications = moderatorNotifications{f} })
	e.auth.tokens["third-party"] = thirdParty(bearer(identity.ScopeWriteNotifications, aliceAccountID))
	path := "/notifications/" + notificationID(1)
	for _, action := range []string{"/read", "/dismiss"} {
		if res := e.send(t, http.MethodPost, path+action, "third-party", ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s by a third-party client = %d, want 404", action, res.StatusCode)
		}
	}
	if !f.unread(aliceAccountID, notificationID(1)) || f.dismissed[notificationID(1)] {
		t.Errorf("the moderator notification changed: read %v, dismissed %v", !f.unread(aliceAccountID, notificationID(1)), f.dismissed[notificationID(1)])
	}
	for _, action := range []string{"/read", "/dismiss"} {
		if res := e.send(t, http.MethodPost, path+action, "write", ""); res.StatusCode != http.StatusNoContent {
			t.Errorf("POST %s by the first-party client = %d, want 204", action, res.StatusCode)
		}
	}
}
