// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// issueAccessToken runs the authorization code flow of the first-party web client for userID
// and returns its access token.
func issueAccessToken(t *testing.T, app *platform.App, userID, scope string) string {
	t.Helper()
	ctx := t.Context()
	verifier := strings.Repeat("v", 43)
	redirect := webOrigin + platform.WebClientCallbackPath
	a, err := app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
		ResponseType: "code", ClientID: identity.DefaultWebClientID, RedirectURI: redirect, Scope: scope,
		CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
	})
	if err != nil {
		t.Fatalf("ValidateAuthorization(%q): %v", scope, err)
	}
	code, err := app.Identity.IssueAuthorizationCode(ctx, a, userID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := app.Identity.ExchangeAuthorizationCode(ctx, identity.CodeExchange{
		ClientCredentials: identity.ClientCredentials{ClientID: identity.DefaultWebClientID},
		Code:              code, RedirectURI: redirect, CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tokens.AccessToken
}

// TestClientAPINotifications drives the notification endpoints on the real database: the
// notifications that a follow and a favourite create, with their source account, the type
// filter, the read marker and unread count, one notification, and dismissal.
func TestClientAPINotifications(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h := app.Handler()
	spec := apitest.Load(t)
	ctx := t.Context()

	alice, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
		Username: "bob", Password: "correct horse battery staple", DisplayName: "Bob", Locale: "en",
	})
	if err != nil {
		t.Fatalf("RegisterLocalUser: %v", err)
	}
	if _, err := app.Graph.Follow(ctx, bob.AccountID, alice.AccountID); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	p, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: alice.AccountID, TextInput: appcontent.TextInput{Text: "hi"}, Visibility: domcontent.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := app.Interactions.Favourite(ctx, bob.AccountID, p.Status.ID); err != nil {
		t.Fatalf("Favourite: %v", err)
	}

	token := issueAccessToken(t, app, alice.User.ID, "read:notifications write:notifications")
	send := func(method, path, body string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		return spec.Serve(t, h, req)
	}
	list := func(path string) clientapi.NotificationPage {
		t.Helper()
		res := send(http.MethodGet, path, "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", path, res.StatusCode)
		}
		var page clientapi.NotificationPage
		if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	marker := func(res *http.Response) clientapi.NotificationMarker {
		t.Helper()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("marker = %d", res.StatusCode)
		}
		var m clientapi.NotificationMarker
		if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	page := list("/api/v1/notifications")
	if len(page.Items) != 2 || page.NextCursor != nil {
		t.Fatalf("notifications = %+v, want two on one page", page)
	}
	fav, follow := page.Items[0], page.Items[1]
	if fav.Type != "favourite" || fav.Status == nil || fav.Status.Id != p.Status.ID || fav.Account == nil || fav.Account.Username != "bob" {
		t.Errorf("favourite = %+v", fav)
	}
	if follow.Type != "follow" || follow.Status != nil || follow.Account == nil || follow.Account.Id != bob.AccountID {
		t.Errorf("follow = %+v", follow)
	}
	if page := list("/api/v1/notifications?limit=1"); len(page.Items) != 1 || page.NextCursor == nil {
		t.Errorf("first page of one = %+v, want a next cursor", page)
	} else if next := list("/api/v1/notifications?limit=1&cursor=" + *page.NextCursor); len(next.Items) != 1 || next.Items[0].Id != follow.Id {
		t.Errorf("second page = %+v, want the follow", next)
	}
	if page := list("/api/v1/notifications?types=follow"); len(page.Items) != 1 || page.Items[0].Id != follow.Id {
		t.Errorf("follow notifications = %+v", page)
	}

	if m := marker(send(http.MethodGet, "/api/v1/notifications/marker", "")); m.LastReadId != nil || m.UnreadCount != 2 {
		t.Errorf("marker = %+v, want 2 unread", m)
	}
	// The favourite, read while the older follow is unread, stays read on the next list.
	if res := send(http.MethodPost, "/api/v1/notifications/"+fav.Id+"/read", ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("read the favourite = %d, want 204", res.StatusCode)
	}
	if page := list("/api/v1/notifications"); len(page.Items) != 2 || !page.Items[0].Read || page.Items[1].Read {
		t.Errorf("notifications after reading the favourite = %+v, want it read and the follow not", page)
	}
	if m := marker(send(http.MethodGet, "/api/v1/notifications/marker", "")); m.LastReadId != nil || m.UnreadCount != 1 {
		t.Errorf("marker after reading the favourite = %+v, want 1 unread", m)
	}
	if m := marker(send(http.MethodPut, "/api/v1/notifications/marker", `{"last_read_id":"`+fav.Id+`"}`)); m.LastReadId == nil || *m.LastReadId != fav.Id || m.UnreadCount != 0 {
		t.Errorf("marker after reading all = %+v", m)
	}
	if m := marker(send(http.MethodPut, "/api/v1/notifications/marker", `{"last_read_id":"`+follow.Id+`"}`)); *m.LastReadId != fav.Id {
		t.Errorf("marker moved back to %s", *m.LastReadId)
	}

	// One notification reads as the list shows it.
	if res := send(http.MethodGet, "/api/v1/notifications/"+fav.Id, ""); res.StatusCode != http.StatusOK {
		t.Errorf("GET the favourite = %d, want 200", res.StatusCode)
	} else {
		var got clientapi.Notification
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Id != fav.Id || got.Type != "favourite" || got.Status == nil || got.Status.Id != p.Status.ID || got.Account == nil || got.Account.Id != bob.AccountID {
			t.Errorf("favourite = %+v", got)
		}
	}

	if res := send(http.MethodPost, "/api/v1/notifications/"+follow.Id+"/dismiss", ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("dismiss = %d", res.StatusCode)
	}
	if res := send(http.MethodGet, "/api/v1/notifications/"+follow.Id, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET the dismissed follow = %d, want 404", res.StatusCode)
	}
	if res := send(http.MethodGet, "/api/v1/notifications/"+p.Status.ID, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET an unknown notification = %d, want 404", res.StatusCode)
	}
	if page := list("/api/v1/notifications"); len(page.Items) != 1 || page.Items[0].Id != fav.Id {
		t.Errorf("notifications after dismissing the follow = %+v", page)
	}
	if res := send(http.MethodPost, "/api/v1/notifications/"+p.Status.ID+"/dismiss", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("dismissing an unknown notification = %d, want 404", res.StatusCode)
	}
}
