// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestNotificationSettingsThroughTheAPI drives the notification settings on the real database
// (FR-210): the rows an administrator gets with their defaults, a change that is saved, the
// changes that fail with 422, and a kind turned off in the app that creates no notification.
func TestNotificationSettingsThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	settings := func(user string) map[clientapi.NotificationGroup]clientapi.NotificationSettingRow {
		t.Helper()
		var got clientapi.NotificationSettings
		if res := c.do(user, http.MethodGet, "/preferences/notifications", nil, &got); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /preferences/notifications as %s = %d", user, res.StatusCode)
		}
		out := map[clientapi.NotificationGroup]clientapi.NotificationSettingRow{}
		for _, r := range got.Rows {
			out[r.Group] = r
		}
		return out
	}
	change := func(user string, group clientapi.NotificationGroup, channel string, on bool) *http.Response {
		t.Helper()
		body := map[string]any{"changes": []map[string]any{{"group": group, "channel": channel, "enabled": on}}}
		return c.do(user, http.MethodPatch, "/preferences/notifications", body, nil)
	}

	alice := settings("alice")
	if r := alice["mention"]; r.Section != clientapi.NotificationSettingRowSectionOrdinary || !r.InApp.Enabled || r.Email.Enabled || r.InApp.Fixed || r.Email.Fixed {
		t.Errorf("alice's mention row = %+v, want in the app and not by email, both chosen", r)
	}
	if r := alice["moderation"]; !r.InApp.Fixed || !r.InApp.Enabled || r.Email.Fixed || r.Email.Enabled || len(r.Types) != 5 {
		t.Errorf("alice's moderation row = %+v, want always in the app and not by email", r)
	}
	// Every user of this server is an administrator, who gets the moderator rows; the supporter
	// program is off, so nobody gets its row.
	if r, ok := alice["admin_report"]; !ok || r.Section != clientapi.NotificationSettingRowSectionModerator || !r.InApp.Enabled || !r.Email.Enabled {
		t.Errorf("the administrator's admin_report row = %+v (%v), want both on", r, ok)
	}
	if _, ok := alice["supporter"]; ok {
		t.Error("the supporter row is offered while the program is off")
	}

	if res := change("alice", "mention", "email", true); res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH mention email = %d", res.StatusCode)
	}
	if r := settings("alice")["mention"]; !r.Email.Enabled {
		t.Errorf("mention row after the change = %+v, want email on", r)
	}
	for _, bad := range []struct {
		group   clientapi.NotificationGroup
		channel string
	}{{"moderation", "in_app"}, {"service_closure", "email"}, {"supporter", "email"}} {
		if res := change("alice", bad.group, bad.channel, false); res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %s %s = %d, want 422", bad.group, bad.channel, res.StatusCode)
		}
	}

	// Follows turned off in the app create no notification.
	if res := change("alice", "follow", "in_app", false); res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH follow in_app = %d", res.StatusCode)
	}
	if res := c.do("bob", http.MethodPost, "/accounts/"+c.ids["alice"]+"/follow", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("POST follow = %d", res.StatusCode)
	}
	var page clientapi.NotificationPage
	if res := c.do("alice", http.MethodGet, "/notifications", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 0 {
		t.Errorf("alice's notifications = %d %+v, want none", res.StatusCode, page.Items)
	}
}
