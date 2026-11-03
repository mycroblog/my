// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// set reports whether an optional field of a Relationship is present and true.
func set(b *bool) bool { return b != nil && *b }

// TestFirstPartyOnlyOperationsRefuseThirdPartyTokens checks through the real API that a
// third-party token gets the 403 of a first-party-only operation from each operation of ADR-0143,
// whatever its scopes, that the first-party client still reads them, and that the reads the ADR
// keeps open stay open.
func TestFirstPartyOnlyOperationsRefuseThirdPartyTokens(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice")
	const missing = "00000000-0000-7000-8000-000000000000"
	app := c.appToken("alice", "read write")

	reads := []string{"/exports", "/csv_exports/following", "/imports", "/invites", "/moderation/actions"}
	refused := []struct {
		name, method, path string
		body               any
	}{
		{"listDataExports", http.MethodGet, "/exports", nil},
		{"requestDataExport", http.MethodPost, "/exports", nil},
		{"downloadDataExport", http.MethodGet, "/exports/" + missing + "/archive", nil},
		{"exportCsv", http.MethodGet, "/csv_exports/following", nil},
		{"listDataImports", http.MethodGet, "/imports", nil},
		{"startDataImport", http.MethodPost, "/imports", map[string]any{"kind": "following", "mode": "merge", "data": "a"}},
		{"previewDataImport", http.MethodPost, "/imports/preview", map[string]any{"kind": "following", "mode": "merge", "data": "a"}},
		{"updateAutoDeletePolicy", http.MethodPut, "/preferences/auto_delete", map[string]any{"enabled": false}},
		{"previewAutoDeletePolicy", http.MethodPost, "/preferences/auto_delete/preview", map[string]any{"enabled": false}},
		{"addAccountAlias", http.MethodPost, "/accounts/aliases", map[string]any{"acct": "bob@remote.example"}},
		{"removeAccountAlias", http.MethodPost, "/accounts/aliases/remove", map[string]any{"acct": "bob@remote.example"}},
		{"listOwnInvites", http.MethodGet, "/invites", nil},
		{"createOwnInvite", http.MethodPost, "/invites", map[string]any{}},
		{"revokeOwnInvite", http.MethodPost, "/invites/" + missing + "/revoke", nil},
		{"listOwnModerationActions", http.MethodGet, "/moderation/actions", nil},
		{"appealModerationAction", http.MethodPost, "/moderation/actions/" + missing + "/appeal", map[string]any{"text": "a"}},
	}
	for _, op := range refused {
		res := c.do(app, op.method, op.path, op.body, nil)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s with a third-party token = %d, want 403", op.name, res.StatusCode)
			continue
		}
		var p clientapi.Problem
		if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if p.Type != clientapi.ProblemTypeFirstPartyOnly {
			t.Errorf("%s problem = %q, want %q", op.name, p.Type, clientapi.ProblemTypeFirstPartyOnly)
		}
	}
	for _, path := range reads {
		if res := c.do("alice", http.MethodGet, path, nil, nil); res.StatusCode != http.StatusOK {
			t.Errorf("GET %s with the first-party client = %d, want 200", path, res.StatusCode)
		}
	}
	narrow := c.appToken("alice", "read:accounts")
	for _, path := range []string{"/preferences/auto_delete", "/accounts/aliases", "/accounts/verify_credentials"} {
		if res := c.do(narrow, http.MethodGet, path, nil, nil); res.StatusCode != http.StatusOK {
			t.Errorf("GET %s with read:accounts = %d, want 200", path, res.StatusCode)
		}
	}
}

// reportForModerators has alice report bob, which notifies carol, the only other administrator,
// and returns that notification's ID from carol's first-party list.
func reportForModerators(t *testing.T, c *statusesClient) string {
	t.Helper()
	if res := c.do("alice", http.MethodPost, "/reports", map[string]any{"account_id": c.ids["bob"], "category": "spam"}, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	var page clientapi.NotificationPage
	if res := c.do("carol", http.MethodGet, "/notifications", nil, &page); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /notifications = %d", res.StatusCode)
	}
	if len(page.Items) != 1 || page.Items[0].Type != clientapi.NotificationTypeAdminReport {
		t.Fatalf("carol's notifications = %+v, want one admin_report", page.Items)
	}
	return page.Items[0].Id
}

// mention has bob mention carol.
func mention(t *testing.T, c *statusesClient) {
	t.Helper()
	if _, err := c.app.Content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: c.ids["bob"], TextInput: appcontent.TextInput{Text: "@carol hello"}, Visibility: domcontent.VisibilityPublic,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

// TestModeratorNotificationsStayWithFirstPartyClients checks ADR-0143 on a moderator's tokens:
// the list and the unread count of a third-party token leave the moderator notifications out,
// also when it asks for their type, and those of the first-party client keep them.
func TestModeratorNotificationsStayWithFirstPartyClients(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	reportForModerators(t, c)
	mention(t, c)
	app := c.appToken("carol", "read:notifications read:statuses")

	list := func(user, query string) []clientapi.NotificationType {
		t.Helper()
		var page clientapi.NotificationPage
		if res := c.do(user, http.MethodGet, "/notifications"+query, nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /notifications%s as %s = %d", query, user, res.StatusCode)
		}
		types := []clientapi.NotificationType{}
		for _, n := range page.Items {
			types = append(types, n.Type)
		}
		return types
	}
	unread := func(user string) int64 {
		t.Helper()
		var m clientapi.NotificationMarker
		if res := c.do(user, http.MethodGet, "/notifications/marker", nil, &m); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /notifications/marker as %s = %d", user, res.StatusCode)
		}
		return m.UnreadCount
	}
	if got := list(app, ""); !slices.Equal(got, []clientapi.NotificationType{clientapi.NotificationTypeMention}) {
		t.Errorf("the third-party list = %v, want the mention alone", got)
	}
	if got := list(app, "?types=admin_report"); len(got) != 0 {
		t.Errorf("the third-party list of admin_report = %v, want none", got)
	}
	if got := list("carol", ""); !slices.Equal(got, []clientapi.NotificationType{clientapi.NotificationTypeMention, clientapi.NotificationTypeAdminReport}) {
		t.Errorf("the first-party list = %v, want the mention and the admin_report", got)
	}
	if got := unread(app); got != 1 {
		t.Errorf("the third-party unread count = %d, want 1", got)
	}
	if got := unread("carol"); got != 2 {
		t.Errorf("the first-party unread count = %d, want 2", got)
	}
}

// TestModeratorNotificationsStayOutOfThirdPartyStreams opens the stream with a moderator's
// third-party token: the mention that follows a moderator notification is the first notification
// it gets.
func TestModeratorNotificationsStayOutOfThirdPartyStreams(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	app := c.appToken("carol", "read")
	srv := httptest.NewServer(c.h)
	t.Cleanup(srv.Close)
	ctx, disconnect := context.WithCancel(t.Context())
	defer disconnect()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+clientapi.BasePath+"/streaming", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.tokens[app])
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /streaming: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /streaming = %d", res.StatusCode)
	}
	events := readEvents(bufio.NewReader(res.Body))
	deadline := time.Now().Add(5 * time.Second)
	for c.app.Streams.Len() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the stream did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}
	reportForModerators(t, c)
	mention(t, c)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("the stream ended before a notification")
			}
			if ev.name != "notification" {
				continue
			}
			var n clientapi.Notification
			if err := json.Unmarshal(ev.data, &n); err != nil {
				t.Fatal(err)
			}
			if n.Type != clientapi.NotificationTypeMention {
				t.Fatalf("the first notification of the stream = %s, want the mention", n.Type)
			}
			return
		case <-time.After(5 * time.Second):
			t.Fatal("no notification within 5 seconds")
		}
	}
}

// TestModeratorAlertsDroppedFromThirdPartyPush registers a third-party Web Push subscription that
// asks for a moderator type on the inline queue: the answer leaves the type out and the moderator
// notification sends no message.
func TestModeratorAlertsDroppedFromThirdPartyPush(t *testing.T) {
	t.Parallel()
	service := &pushService{status: http.StatusCreated}
	c := newStatusesClientWith(t, map[string]string{
		"MY_QUEUE": "inline", "MY_VAPID_PRIVATE_KEY": "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw",
	}, platform.AppOptions{HTTPClient: service}, "alice", "bob", "carol")
	app := c.appToken("carol", "push")
	ua, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	var sub clientapi.PushSubscription
	body := map[string]any{
		"endpoint": "https://push.example.net/send/1",
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef")),
		},
		"alerts": []string{"admin_report", "mention"},
	}
	if res := c.do(app, http.MethodPut, "/push/subscription", body, &sub); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /push/subscription = %d", res.StatusCode)
	}
	if !slices.Equal(sub.Alerts, []clientapi.NotificationType{clientapi.NotificationTypeMention}) {
		t.Errorf("alerts = %v, want the mention alone", sub.Alerts)
	}
	reportForModerators(t, c)
	if got := len(service.sent()); got != 0 {
		t.Errorf("%d messages after a moderator notification, want none", got)
	}
	mention(t, c)
	if got := len(service.sent()); got != 1 {
		t.Errorf("%d messages after a mention, want 1", got)
	}
}

// TestRelationshipFieldsFollowTheScopes checks that a Relationship holds the fields of the lists
// that the token's scopes cover, in the read and in the answers of the changes (M5 of the audit).
func TestRelationshipFieldsFollowTheScopes(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	bob := c.ids["bob"]
	follows := []string{"following", "requested", "followed_by", "requested_by", "show_boosts", "notify"}
	blocks := []string{"blocking"}
	mutes := []string{"muting", "muting_notifications"}
	wantFields := func(name string, got map[string]any, groups ...[]string) {
		t.Helper()
		want := []string{"id"}
		for _, g := range groups {
			want = append(want, g...)
		}
		slices.Sort(want)
		fields := make([]string, 0, len(got))
		for k := range got {
			fields = append(fields, k)
		}
		slices.Sort(fields)
		if !slices.Equal(fields, want) {
			t.Errorf("%s: fields %v, want %v", name, fields, want)
		}
	}
	read := func(user string) map[string]any {
		t.Helper()
		var list struct {
			Items []map[string]any `json:"items"`
		}
		if res := c.do(user, http.MethodGet, "/accounts/relationships?id="+bob, nil, &list); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /accounts/relationships as %s = %d", user, res.StatusCode)
		}
		rs := list.Items
		if len(rs) != 1 {
			t.Fatalf("relationships as %s = %v", user, rs)
		}
		return rs[0]
	}
	// The state is there to hide: alice blocks and mutes bob.
	if res := c.do("alice", http.MethodPost, "/accounts/"+bob+"/mute", map[string]any{}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("mute = %d", res.StatusCode)
	}
	if res := c.do("alice", http.MethodPost, "/accounts/"+bob+"/block", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("block = %d", res.StatusCode)
	}

	for _, tc := range []struct {
		scope  string
		groups [][]string
	}{
		{"read:follows", [][]string{follows}},
		{"read:follows read:blocks", [][]string{follows, blocks}},
		{"read:follows read:mutes", [][]string{follows, mutes}},
		{"read", [][]string{follows, blocks, mutes}},
		{"follow", [][]string{follows, blocks, mutes}},
	} {
		wantFields(tc.scope, read(c.appToken("alice", tc.scope)), tc.groups...)
	}
	wantFields("the first-party client", read("alice"), follows, blocks, mutes)
	if r := read(c.appToken("alice", "read:follows read:blocks read:mutes")); r["blocking"] != true || r["muting"] != true {
		t.Errorf("the state with every scope = %v", r)
	}

	// The answer of a change shows the list that its own scope covers.
	var unblocked map[string]any
	if res := c.do(c.appToken("alice", "write:blocks"), http.MethodDelete, "/accounts/"+bob+"/block", nil, &unblocked); res.StatusCode != http.StatusOK {
		t.Fatalf("unblock = %d", res.StatusCode)
	}
	wantFields("write:blocks", unblocked, blocks)
	var muted map[string]any
	if res := c.do(c.appToken("alice", "write:mutes"), http.MethodPost, "/accounts/"+bob+"/mute", map[string]any{}, &muted); res.StatusCode != http.StatusOK {
		t.Fatalf("mute = %d", res.StatusCode)
	}
	wantFields("write:mutes", muted, mutes)
	var followed map[string]any
	if res := c.do(c.appToken("alice", "write:follows"), http.MethodPost, "/accounts/"+bob+"/follow", nil, &followed); res.StatusCode != http.StatusOK {
		t.Fatalf("follow = %d", res.StatusCode)
	}
	wantFields("write:follows", followed, follows)
	if followed["following"] != true {
		t.Errorf("the answer of the follow = %v", followed)
	}
}

// TestReadFavouritesIsNotAScope checks that the scope without an operation is gone: an app that
// asks for it gets invalid_scope, and read still works without it.
func TestReadFavouritesIsNotAScope(t *testing.T) {
	t.Parallel()
	if slices.Contains(domain.KnownScopes(), "read:favourites") {
		t.Error("KnownScopes holds read:favourites")
	}
	c := newStatusesClient(t, "alice")
	const redirect = "https://app.example/callback"
	reg, err := c.app.Identity.RegisterOAuthApp(t.Context(), c.userIDs["alice"], identity.OAuthAppInput{
		Name: "App", RedirectURIs: []string{redirect}, Scope: "read",
	})
	if err != nil {
		t.Fatalf("RegisterOAuthApp: %v", err)
	}
	if _, err := c.app.Identity.ValidateAuthorization(t.Context(), identity.AuthorizationRequest{
		ResponseType: "code", ClientID: reg.Client.ClientID, RedirectURI: redirect, Scope: "read:favourites",
		CodeChallenge: domain.S256Challenge("verifier"), CodeChallengeMethod: domain.CodeChallengeMethodS256,
	}); err == nil {
		t.Error("the authorization request for read:favourites passed")
	}
}
