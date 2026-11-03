// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestClientAPIAccountImages sets an avatar and a header on alice's profile on the real
// database and checks that every response embedding her account carries their URLs: her own
// account, her profile, a timeline, an account list, and a notification. bob has no images, and
// his account has none.
func TestClientAPIAccountImages(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()

	type user struct{ accountID, token string }
	users := map[string]user{}
	for _, name := range []string{"alice", "bob"} {
		r, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
			Username: name, Email: name + "@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("CreateAdmin(%s): %v", name, err)
		}
		users[name] = user{accountID: r.AccountID, token: issueAccessToken(t, app, r.User.ID, "read")}
	}
	alice, bob := users["alice"], users["bob"]

	image := func(w, h int, purpose domcontent.MediaPurpose) string {
		t.Helper()
		m, err := app.Media.Upload(ctx, appcontent.UploadInput{
			OwnerAccountID: alice.accountID, Content: bytes.NewReader(testkit.FixturePNG(w, h, false)), Purpose: purpose,
		})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if res := app.Media.Process(ctx, m.File.ID, purpose); !res.IsDone() {
			t.Fatalf("Process = %+v", res)
		}
		return m.File.ID
	}
	avatarID, headerID := image(64, 64, domcontent.PurposeAvatar), image(150, 50, domcontent.PurposeHeader)
	if _, err := app.Accounts.UpdateProfile(ctx, alice.accountID, account.ProfileUpdate{AvatarMediaID: &avatarID, HeaderMediaID: &headerID}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	small := func(id string) string {
		t.Helper()
		m, err := app.Media.Media(ctx, id)
		if err != nil || !m.Ready() {
			t.Fatalf("Media(%s) = %+v, %v", id, m.Result, err)
		}
		v, _ := m.Result.Variant(domcontent.SizeSmall, domcontent.MotionStill)
		return app.Media.URL(v)
	}
	wantAvatar, wantHeader := small(avatarID), small(headerID)

	if _, err := app.Graph.Follow(ctx, bob.accountID, alice.accountID); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if _, err := app.Graph.Follow(ctx, alice.accountID, bob.accountID); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if _, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: alice.accountID, TextInput: appcontent.TextInput{Text: "hi"}, Visibility: domcontent.VisibilityPublic,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	get := func(path, token string, out any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := spec.Serve(t, h, req)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", path, res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	check := func(what string, a *clientapi.Account) {
		t.Helper()
		switch {
		case a == nil:
			t.Errorf("%s: no account", what)
		case a.Id == alice.accountID && (smallURL(a.Avatar) != wantAvatar || smallURL(a.Header) != wantHeader):
			t.Errorf("%s: alice's images = %v %v, want %s %s", what, a.Avatar, a.Header, wantAvatar, wantHeader)
		case a.Id == bob.accountID && (a.Avatar != nil || a.Header != nil):
			t.Errorf("%s: bob's images = %v %v, want none", what, a.Avatar, a.Header)
		}
	}

	var own clientapi.CredentialAccount
	get("/accounts/verify_credentials", alice.token, &own)
	if smallURL(own.Avatar) != wantAvatar || smallURL(own.Header) != wantHeader || deref(own.Source.AvatarMediaId) != avatarID {
		t.Errorf("verify_credentials = %v %v %v", own.Avatar, own.Header, own.Source.AvatarMediaId)
	}
	if a := own.Avatar; a == nil || a.Large.Width == nil || *a.Large.Width != 64 || *a.Small.Width != 64 || a.Animated != nil {
		t.Errorf("avatar variants = %+v, want 64 × 64 still ones", a)
	}

	var profile, other clientapi.Account
	get("/accounts/"+alice.accountID, bob.token, &profile)
	check("getAccount", &profile)
	get("/accounts/"+bob.accountID, alice.token, &other)
	check("getAccount of bob", &other)

	var home clientapi.StatusPage
	get("/timelines/home", bob.token, &home)
	if len(home.Items) != 1 {
		t.Fatalf("home = %+v, want alice's post", home.Items)
	}
	check("home timeline", &home.Items[0].Account)

	var following clientapi.AccountPage
	get("/accounts/"+bob.accountID+"/following", bob.token, &following)
	if len(following.Items) != 1 {
		t.Fatalf("following = %+v, want alice", following.Items)
	}
	check("following", &following.Items[0])

	var notifications clientapi.NotificationPage
	get("/notifications", bob.token, &notifications)
	if len(notifications.Items) != 1 {
		t.Fatalf("notifications = %+v, want alice's follow", notifications.Items)
	}
	check("notification", notifications.Items[0].Account)
}

// smallURL returns the URL of the small variant of an image, "" for none.
func smallURL(v *clientapi.MediaVariants) string {
	if v == nil {
		return ""
	}
	return v.Small.Url
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
