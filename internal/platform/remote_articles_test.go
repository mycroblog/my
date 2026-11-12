// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestInboxReadsRemoteArticlesInFull checks that a received Article is stored as an article on
// the real database with the real sanitizer (ADR-0160 §10): its body is sanitized with the
// allowlist of articles, its images are remote media files that the client API shows through the
// media proxy (the ones past max_article_images as links), its name is the title and its summary
// is made from the body; lists show its card without the body, and GET /statuses/{id} the body.
// Its mention notifies, its Update makes a revision, an Update that makes it a Note is dropped,
// and its Delete removes it.
func TestInboxReadsRemoteArticlesInFull(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	const actor = "https://remote.example/users/bob"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &timelineAPI{app: app, h: app.Handler(), spec: apitest.Load(t)}
	alice := e.user(t, "alice")
	var bobID string
	err = app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		bob, err := app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "bob",
			InboxURL: actor + "/inbox", FollowersURL: actor + "/followers",
			Keys:     []appaccount.RemoteKeyInput{{KeyID: actor + "#main-key", PublicKey: &key.PublicKey}},
			Document: []byte(`{"id":"` + actor + `"}`),
		})
		bobID = bob.ID
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingMaxArticleImages, Value: "1"}); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	aliceIRI := "https://" + cfg.Host + "/ap/actors/" + alice.accountID
	post := func(t *testing.T, activity map[string]any) {
		t.Helper()
		activity["@context"] = "https://www.w3.org/ns/activitystreams"
		body, err := json.Marshal(activity)
		if err != nil {
			t.Fatal(err)
		}
		req := &kernel.HTTPRequest{
			Method: http.MethodPost, URL: "https://" + cfg.Host + "/ap/inbox", Body: body,
			Header: http.Header{"Date": {time.Now().UTC().Format(http.TimeFormat)}, "Host": {cfg.Host}},
		}
		err = httpsig.New().Sign(req, appfederation.SigningKey{KeyID: actor + "#main-key", PrivateKey: key}, appfederation.SignOptions{
			Scheme: appfederation.SchemeCavage, Algorithm: appfederation.AlgRSAPKCS1SHA256,
			Components: []string{"(request-target)", "host", "date", "digest"},
		})
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		hr := httptest.NewRequest(http.MethodPost, req.URL, strings.NewReader(string(body)))
		for k, v := range req.Header {
			if k != "Host" {
				hr.Header[k] = v
			}
		}
		hr.Header.Set("Content-Type", "application/activity+json")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, hr)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("POST %s = %d %s, want 202", activity["id"], rec.Code, rec.Body)
		}
	}
	const uri = actor + "/articles/1"
	published := time.Now().UTC().Add(-time.Hour)
	article := func(typ, name string, updated time.Time) map[string]any {
		return map[string]any{
			"id": uri, "type": typ, "attributedTo": actor, "published": published.Format(time.RFC3339),
			"updated": updated.Format(time.RFC3339), "url": "https://remote.example/@bob/a-long-read",
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{actor + "/followers", aliceIRI},
			"name": name,
			"content": `<h1>Chapter one</h1><p onclick="x()">Hello <span class="h-card"><a href="` + aliceIRI + `" class="u-url mention">@alice</a></span>, ` +
				`this is the body.</p><p><img src="https://remote.example/files/diagram.png" alt="a diagram" width="640" height="480">` +
				`<img src="https://remote.example/files/second.png" alt="another"></p><script>alert(1)</script>` +
				`<iframe src="https://evil.example"></iframe><pre><code>code()</code></pre>`,
			"image": map[string]any{"type": "Image", "url": "https://remote.example/files/cover.webp", "mediaType": "image/webp", "name": "the cover"},
			"tag":   []any{map[string]any{"type": "Mention", "href": aliceIRI, "name": "@alice@" + cfg.Host}, map[string]any{"type": "Hashtag", "name": "#reading"}},
		}
	}
	post(t, map[string]any{"id": uri + "/create", "type": "Create", "actor": actor, "object": article("Article", "A long read", published)})

	// The inline queue processes the activity before the inbox answers.
	st, err := app.Content.FindStatusByURI(ctx, uri)
	if err != nil {
		t.Fatalf("FindStatusByURI(%s) = %v", uri, err)
	}
	p, err := app.Content.Find(ctx, st.ID)
	if err != nil {
		t.Fatalf("Find = %v", err)
	}
	a := p.Revision.Article
	if a == nil || a.Title != "A long read" || a.SummaryOrigin != domcontent.SummaryFromBody || a.Summary != "Hello @alice, this is the body." ||
		a.Cover == nil || a.Cover.Description != "the cover" || len(a.Images) != 1 || a.Images[0].Description != "a diagram" || p.Revision.SpoilerText != "" {
		t.Fatalf("article = %+v, spoiler %q; want the title, the summary from the body, the cover, and one image", a, p.Revision.SpoilerText)
	}
	stored := p.Revision.Bodies[0].ContentHTML
	for _, want := range []string{"<h2>Chapter one</h2>", `<img src="https://remote.example/files/diagram.png" alt="a diagram" width="640" height="480" loading="lazy">`,
		`<a href="https://remote.example/files/second.png" rel="nofollow noopener noreferrer" target="_blank">another</a>`, "<pre><code>code()</code></pre>"} {
		if !strings.Contains(stored, want) {
			t.Errorf("stored body %q lacks %s", stored, want)
		}
	}
	for _, bad := range []string{"<script", "<iframe", "onclick", "<h1"} {
		if strings.Contains(stored, bad) {
			t.Errorf("stored body %q keeps %s", stored, bad)
		}
	}
	ns, err := app.Notifications.List(ctx, alice.accountID, appnotification.Query{})
	if err != nil || len(ns) != 1 || ns[0].Type != notification.TypeMention || ns[0].StatusID != st.ID {
		t.Errorf("notifications of alice = %+v, %v; want the mention", ns, err)
	}

	getStatus := func(t *testing.T) clientapi.Status {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/statuses/"+st.ID, nil)
		req.Header.Set("Authorization", "Bearer "+alice.token)
		res := e.spec.Serve(t, e.h, req)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /statuses/%s = %d", st.ID, res.StatusCode)
		}
		var out clientapi.Status
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	proxy := "https://" + cfg.Host + "/media/proxy/"
	full := getStatus(t)
	if full.Article == nil || full.Article.Title != "A long read" || full.Article.Cover == nil || full.Article.Cover.Url == nil ||
		!strings.HasPrefix(*full.Article.Cover.Url, proxy) || full.SpoilerText != "" {
		t.Errorf("article = %+v, spoiler %q; want the title and the proxied cover", full.Article, full.SpoilerText)
	}
	if !strings.Contains(full.Content, `<img src="`+proxy) || strings.Contains(full.Content, `src="https://remote.example`) ||
		!strings.Contains(full.Content, "<h2>Chapter one</h2>") {
		t.Errorf("content = %q, want the body with its image through the media proxy", full.Content)
	}
	page := e.page(t, &alice, "/accounts/"+bobID+"/statuses")
	if len(page.Items) != 1 || page.Items[0].Article == nil || page.Items[0].Article.Title != "A long read" || page.Items[0].Content != "" {
		t.Errorf("profile statuses = %+v, want the card of the article without its body", page.Items)
	}

	post(t, map[string]any{"id": uri + "/update/1", "type": "Update", "actor": actor, "object": article("Article", "A longer read", published.Add(time.Minute))})
	if got := getStatus(t); got.Article == nil || got.Article.Title != "A longer read" || got.EditedAt == nil {
		t.Errorf("after the Update: article = %+v, edited_at = %v; want the new title", got.Article, got.EditedAt)
	}
	post(t, map[string]any{"id": uri + "/update/2", "type": "Update", "actor": actor, "object": article("Note", "", published.Add(2*time.Minute))})
	if got := getStatus(t); got.Article == nil || got.Article.Title != "A longer read" {
		t.Errorf("after an Update as a Note: article = %+v, want the article as it was", got.Article)
	}
	// Nor does an Update make a post an article.
	note := func(typ string, updated time.Time) map[string]any {
		n := article(typ, "A note", updated)
		n["id"] = actor + "/notes/1"
		return n
	}
	post(t, map[string]any{"id": actor + "/notes/1/create", "type": "Create", "actor": actor, "object": note("Note", published)})
	post(t, map[string]any{"id": actor + "/notes/1/update", "type": "Update", "actor": actor, "object": note("Article", published.Add(time.Minute))})
	if ns, err := app.Content.FindStatusByURI(ctx, actor+"/notes/1"); err != nil {
		t.Errorf("FindStatusByURI(the note) = %v", err)
	} else if np, err := app.Content.Find(ctx, ns.ID); err != nil || np.Revision.Article != nil || np.Revision.Seq != 1 {
		t.Errorf("the note after an Update as an Article = %+v, %v; want the note as it was", np.Revision, err)
	}

	post(t, map[string]any{"id": uri + "#delete", "type": "Delete", "actor": actor, "object": map[string]any{"id": uri, "type": "Tombstone"}})
	if _, err := app.Content.Find(ctx, st.ID); err == nil {
		t.Errorf("the article is still there after its Delete")
	}
}
