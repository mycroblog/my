// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// statusesClient sends client API requests as the users of a real App, checking every response
// against the spec.
type statusesClient struct {
	t       *testing.T
	app     *platform.App
	h       http.Handler
	spec    *apitest.Spec
	tokens  map[string]string // access token by username
	ids     map[string]string // account ID by username
	userIDs map[string]string // user ID by username
}

func newStatusesClient(t *testing.T, users ...string) *statusesClient {
	t.Helper()
	return newStatusesClientWith(t, nil, platform.AppOptions{}, users...)
}

// newStatusesClientWith is newStatusesClient with more configuration variables and the
// options of the App, whose Logger it discards.
func newStatusesClientWith(t *testing.T, env map[string]string, opts platform.AppOptions, users ...string) *statusesClient {
	t.Helper()
	full := map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin}
	for k, v := range env {
		full[k] = v
	}
	cfg := loadConfig(t, full)
	opts.Logger = slog.New(slog.DiscardHandler)
	app, err := platform.NewApp(t.Context(), cfg, opts)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	c := &statusesClient{t: t, app: app, h: app.Handler(), spec: apitest.Load(t), tokens: map[string]string{}, ids: map[string]string{}, userIDs: map[string]string{}}
	ctx := t.Context()
	verifier := strings.Repeat("v", 43)
	redirect := webOrigin + platform.WebClientCallbackPath
	for _, name := range users {
		reg, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
			Username: name, Email: name + "@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("CreateAdmin(%s): %v", name, err)
		}
		a, err := app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
			ResponseType: "code", ClientID: identity.DefaultWebClientID, RedirectURI: redirect,
			CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
		})
		if err != nil {
			t.Fatal(err)
		}
		code, err := app.Identity.IssueAuthorizationCode(ctx, a, reg.User.ID)
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
		c.tokens[name], c.ids[name], c.userIDs[name] = tokens.AccessToken, reg.AccountID, reg.User.ID
	}
	return c
}

// do sends a request as user ("" for anonymous) and decodes a JSON body into out when it is
// not nil and the response is a success.
func (c *statusesClient) do(user, method, path string, body any, out any) *http.Response {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		r = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, clientapi.BasePath+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+c.tokens[user])
	}
	res := c.spec.Serve(c.t, c.h, req)
	if out != nil && res.StatusCode < 300 {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			c.t.Fatal(err)
		}
	}
	return res
}

func (c *statusesClient) post(user string, body map[string]any) clientapi.Status {
	c.t.Helper()
	var s clientapi.Status
	if res := c.do(user, http.MethodPost, "/statuses", body, &s); res.StatusCode != http.StatusCreated {
		c.t.Fatalf("POST /statuses %v as %s = %d", body, user, res.StatusCode)
	}
	return s
}

// problemShape is what a problem says, without the request ID that differs per request.
func problemShape(t *testing.T, res *http.Response) string {
	t.Helper()
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	p.RequestId = nil
	return fmt.Sprintf("%d %+v %s", res.StatusCode, p, res.Header.Get("Cache-Control"))
}

// TestStatusVisibilityThroughTheAPI fetches a status of each of the five visibilities as each
// kind of viewer: a status that the viewer may not see answers exactly like a missing one.
func TestStatusVisibilityThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "follower", "mentioned", "stranger", "blocked")
	ctx := t.Context()
	if _, err := c.app.Graph.Follow(ctx, c.ids["follower"], c.ids["alice"]); err != nil {
		t.Fatal(err)
	}
	if _, err := c.app.Graph.Block(ctx, c.ids["alice"], c.ids["blocked"]); err != nil {
		t.Fatal(err)
	}
	missing := problemShape(t, c.do("", http.MethodGet, "/statuses/0192f0c1-0000-7000-8000-000000000000", nil, nil))

	viewers := []string{"", "alice", "follower", "mentioned", "stranger", "blocked"}
	want := map[string][]bool{ // in the order of viewers
		"public":   {true, true, true, true, true, false},
		"local":    {false, true, true, true, true, false},
		"unlisted": {true, true, true, true, true, false},
		"private":  {false, true, true, true, false, false},
		"direct":   {false, true, false, true, false, false},
	}
	for vis, sees := range want {
		st := c.post("alice", map[string]any{"text": "hello @mentioned (" + vis + ")", "visibility": vis})
		if string(st.Visibility) != vis || len(st.Mentions) != 1 || st.Mentions[0].Id != c.ids["mentioned"] {
			t.Fatalf("posted %+v, want %s mentioning the mentioned account", st, vis)
		}
		for i, viewer := range viewers {
			for _, path := range []string{"", "/context", "/history"} {
				res := c.do(viewer, http.MethodGet, "/statuses/"+st.Id+path, nil, nil)
				switch {
				case sees[i] && res.StatusCode != http.StatusOK:
					t.Errorf("%s status%s as %q = %d, want 200", vis, path, viewer, res.StatusCode)
				case !sees[i]:
					if got := problemShape(t, res); got != missing {
						t.Errorf("%s status%s as %q = %s, want the answer to a missing status %s", vis, path, viewer, got, missing)
					}
				}
			}
		}
	}
}

// TestStatusesThroughTheAPI drives posting with media, replies and threads, edits and their
// history, interactions, bookmarks, and deletion through the client API on the real database.
func TestStatusesThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	ctx := t.Context()
	upload := func(user string) string {
		t.Helper()
		m, err := c.app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: c.ids[user], Content: bytes.NewReader(testkit.FixturePNG(64, 48, false))})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		return m.File.ID
	}
	process := func(id string) {
		t.Helper()
		c.app.Media.Process(ctx, id, "")
	}

	// Media must be the poster's own processed uploads.
	pending, bobs := upload("alice"), upload("bob")
	process(bobs)
	for _, id := range []string{pending, bobs, "0192f0c1-0000-7000-8000-000000000000"} {
		res := c.do("alice", http.MethodPost, "/statuses", map[string]any{"text": "pic", "visibility": "public", "media": []map[string]any{{"id": id}}}, nil)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("POST with media %s = %d, want 422", id, res.StatusCode)
		}
	}
	process(pending)
	root := c.post("alice", map[string]any{
		"text": "root #Thread", "visibility": "public", "spoiler_text": "cw",
		"media": []map[string]any{{"id": pending, "description": "a picture"}},
	})
	if len(root.MediaAttachments) != 1 || root.MediaAttachments[0].Variants == nil || root.MediaAttachments[0].State != "ready" || *root.MediaAttachments[0].Description != "a picture" ||
		root.MediaAttachments[0].Type != clientapi.MediaAttachmentTypeImage || len(root.Tags) != 1 || root.Tags[0].Name != "Thread" {
		t.Errorf("root = %+v", root)
	}

	// A thread: bob replies, alice replies to bob.
	reply := c.post("bob", map[string]any{"text": "a reply", "visibility": "public", "in_reply_to_id": root.Id})
	if reply.InReplyToId == nil || *reply.InReplyToId != root.Id || *reply.InReplyToAccountId != c.ids["alice"] {
		t.Errorf("reply = %+v", reply)
	}
	nested := c.post("alice", map[string]any{"text": "a nested reply", "visibility": "unlisted", "in_reply_to_id": reply.Id})
	var thread clientapi.StatusContext
	c.do("", http.MethodGet, "/statuses/"+root.Id+"/context", nil, &thread)
	if len(thread.Ancestors) != 0 || len(thread.Descendants) != 2 || thread.Descendants[0].Id != reply.Id || thread.Descendants[1].Id != nested.Id {
		t.Errorf("context of the root = %+v", thread)
	}
	c.do("bob", http.MethodGet, "/statuses/"+nested.Id+"/context", nil, &thread)
	if len(thread.Ancestors) != 2 || thread.Ancestors[0].Id != root.Id || thread.Ancestors[1].Id != reply.Id || len(thread.Descendants) != 0 {
		t.Errorf("context of the nested reply = %+v", thread)
	}
	if res := c.do("alice", http.MethodPost, "/statuses", map[string]any{"text": "x", "visibility": "public", "in_reply_to_id": "0192f0c1-0000-7000-8000-000000000000"}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("reply to a missing status = %d, want 422", res.StatusCode)
	}

	// Edits keep the history; only the author edits.
	var edited clientapi.Status
	if res := c.do("alice", http.MethodPut, "/statuses/"+root.Id, map[string]any{"text": "root, edited"}, &edited); res.StatusCode != http.StatusOK ||
		edited.EditedAt == nil || len(edited.MediaAttachments) != 0 || edited.SpoilerText != "" {
		t.Errorf("PUT = %d %+v", res.StatusCode, edited)
	}
	if res := c.do("bob", http.MethodPut, "/statuses/"+root.Id, map[string]any{"text": "mine"}, nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("PUT by another account = %d, want 403", res.StatusCode)
	}
	var history clientapi.StatusHistory
	c.do("", http.MethodGet, "/statuses/"+root.Id+"/history", nil, &history)
	if len(history.Items) != 2 || history.Items[0].SpoilerText != "cw" || len(history.Items[0].MediaAttachments) != 1 || !strings.Contains(history.Items[1].Content, "edited") {
		t.Errorf("history = %+v", history)
	}
	var source clientapi.StatusSource
	if res := c.do("alice", http.MethodGet, "/statuses/"+root.Id+"/source", nil, &source); res.StatusCode != http.StatusOK || source.Text != "root, edited" {
		t.Errorf("source = %d %+v", res.StatusCode, source)
	}
	if res := c.do("bob", http.MethodGet, "/statuses/"+root.Id+"/source", nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("source of another account's status = %d, want 404", res.StatusCode)
	}

	// Interactions answer with the status as the actor sees it, and repeat safely.
	var st clientapi.Status
	for range 2 {
		for _, path := range []string{"boost", "favourite", "bookmark"} {
			if res := c.do("bob", http.MethodPost, "/statuses/"+root.Id+"/"+path, nil, &st); res.StatusCode != http.StatusOK {
				t.Fatalf("POST %s = %d", path, res.StatusCode)
			}
		}
	}
	if !*st.Boosted || !*st.Favourited || !*st.Bookmarked || st.BoostsCount != 1 || st.FavouritesCount != 1 || st.RepliesCount != 1 || st.Pinned != nil {
		t.Errorf("after the interactions = %+v", st)
	}
	if res := c.do("alice", http.MethodPost, "/statuses/"+root.Id+"/pin", nil, &st); res.StatusCode != http.StatusOK || st.Pinned == nil || !*st.Pinned {
		t.Errorf("pin = %d %+v", res.StatusCode, st)
	}
	if res := c.do("bob", http.MethodPost, "/statuses/"+root.Id+"/pin", nil, nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("pin of another account's status = %d, want 403", res.StatusCode)
	}
	var page clientapi.StatusPage
	if res := c.do("bob", http.MethodGet, "/bookmarks", nil, &page); res.StatusCode != http.StatusOK || len(page.Items) != 1 || page.Items[0].Id != root.Id {
		t.Errorf("bookmarks = %d %+v", res.StatusCode, page)
	}
	for _, path := range []string{"boost", "favourite", "bookmark"} {
		if res := c.do("bob", http.MethodDelete, "/statuses/"+root.Id+"/"+path, nil, &st); res.StatusCode != http.StatusOK {
			t.Fatalf("DELETE %s = %d", path, res.StatusCode)
		}
	}
	if *st.Boosted || *st.Favourited || *st.Bookmarked || st.BoostsCount != 0 || st.FavouritesCount != 0 {
		t.Errorf("after undoing = %+v", st)
	}

	// Deletion: only by the author; afterwards the status is gone.
	if res := c.do("bob", http.MethodDelete, "/statuses/"+root.Id, nil, nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("DELETE by another account = %d, want 403", res.StatusCode)
	}
	if res := c.do("alice", http.MethodDelete, "/statuses/"+root.Id, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", res.StatusCode)
	}
	for _, user := range []string{"", "alice"} {
		if res := c.do(user, http.MethodGet, "/statuses/"+root.Id, nil, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET a deleted status as %q = %d, want 404", user, res.StatusCode)
		}
	}
	if res := c.do("alice", http.MethodDelete, "/statuses/"+root.Id, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("second DELETE = %d, want 404", res.StatusCode)
	}
}

// TestRequiredMediaDescriptionThroughTheAPI checks the instance setting require_media_description
// (FR-105): GET /instance reports it, and while it is on a post or an edit with an image without
// alt text is a 422 that says why, and one whose images have alt text succeeds.
func TestRequiredMediaDescriptionThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice")
	ctx := t.Context()
	upload := func() string {
		t.Helper()
		m, err := c.app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: c.ids["alice"], Content: bytes.NewReader(testkit.FixturePNG(64, 48, false))})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		c.app.Media.Process(ctx, m.File.ID, "")
		return m.File.ID
	}
	required := func() bool {
		t.Helper()
		var info clientapi.Instance
		if res := c.do("", http.MethodGet, "/instance", nil, &info); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /instance = %d", res.StatusCode)
		}
		return info.Limits.RequireMediaDescription
	}
	if required() {
		t.Error("require_media_description is on by default")
	}
	first := upload()
	root := c.post("alice", map[string]any{"text": "pic", "visibility": "public", "media": []map[string]any{{"id": first}}})

	if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingRequireMediaDescription, Value: "true"}); err != nil {
		t.Fatal(err)
	}
	if !required() {
		t.Error("GET /instance does not report require_media_description")
	}
	for _, req := range []struct{ method, path string }{{http.MethodPost, "/statuses"}, {http.MethodPut, "/statuses/" + root.Id}} {
		res := c.do("alice", req.method, req.path, map[string]any{"text": "pic", "visibility": "public", "media": []map[string]any{{"id": first, "description": " "}}}, nil)
		var p clientapi.Problem
		if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "alt text") {
			t.Errorf("%s %s without alt text = %d %+v, want a 422 that asks for alt text", req.method, req.path, res.StatusCode, p)
		}
	}
	described := c.post("alice", map[string]any{"text": "pic", "visibility": "public", "media": []map[string]any{{"id": upload(), "description": "a picture"}}})
	if len(described.MediaAttachments) != 1 {
		t.Errorf("post with alt text = %+v", described)
	}
}
