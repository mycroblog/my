// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domaincontent "github.com/mycroblog/my/internal/domain/content"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// pageWeb is a kernel.HTTPClient that serves fixed responses by URL, as the web would, and
// counts the requests of each URL.
type pageWeb struct {
	mu    sync.Mutex
	pages map[string]webPage
	hits  map[string]int
}

type webPage struct {
	status      int
	contentType string
	body        []byte
}

func (w *pageWeb) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hits[req.URL]++
	p, ok := w.pages[req.URL]
	if !ok {
		p = webPage{status: http.StatusNotFound, contentType: "text/plain"}
	}
	return &kernel.HTTPResponse{
		StatusCode: p.status, URL: req.URL, Header: http.Header{"Content-Type": {p.contentType}},
		Body: io.NopCloser(bytes.NewReader(p.body)),
	}, nil
}

func (w *pageWeb) count(u string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hits[u]
}

// cardPage is a page whose head declares a card with an image.
func cardPage(title, image string) []byte {
	return []byte(`<!doctype html><html lang="en"><head><title>Fallback</title>
<meta property="og:title" content="` + title + `">
<meta property="og:description" content="A &amp; B">
<meta property="og:site_name" content="News">
<meta property="og:image" content="` + image + `">
<meta property="og:image:alt" content="A chart">
</head><body><meta property="og:title" content="ignored"></body></html>`)
}

// fetchLinkCard runs the fetch that the job of a status would, ending at once what a host's limits
// hold it back for, which the tests of the limits cover.
func fetchLinkCard(t *testing.T, app *platform.App, statusID string) {
	t.Helper()
	for {
		_, err := app.LinkCards.Fetch(t.Context(), statusID)
		var postponed *appcontent.LinkCardPostponed
		if !errors.As(err, &postponed) {
			if err != nil {
				t.Fatalf("Fetch(%s): %v", statusID, err)
			}
			return
		}
		if err := app.KV.Delete(t.Context(), appcontent.LinkCardHostNotBeforeNamespace, postponed.Host); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLinkCardsThroughTheAPI posts statuses with links, runs the fetch of their cards, and reads
// them through the client API (ADR-0062): the card of the first link with its image behind the
// media proxy, none for a status with attachments, a page without a title, or the disabled
// setting, and the domain actions on the card's host.
func TestLinkCardsThroughTheAPI(t *testing.T) {
	t.Parallel()
	png := testkit.FixturePNG(64, 48, false)
	web := &pageWeb{hits: map[string]int{}, pages: map[string]webPage{
		"https://news.example/a":        {http.StatusOK, "text/html; charset=utf-8", cardPage("Big news", "/a.png")},
		"https://news.example/a.png":    {http.StatusOK, "image/png", png},
		"https://untitled.example/":     {http.StatusOK, "text/html", []byte("<html><head></head><body>x</body></html>")},
		"https://media.example/a":       {http.StatusOK, "text/html", cardPage("Pictures", "https://media.example/a.png")},
		"https://media.example/a.png":   {http.StatusOK, "image/png", png},
		"https://blocked.example/a":     {http.StatusOK, "text/html", cardPage("Blocked", "")},
		"https://svg.example/a":         {http.StatusOK, "text/html", cardPage("Vector", "/a.svg")},
		"https://svg.example/a.svg":     {http.StatusOK, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
		"https://example.test/@alice/1": {http.StatusOK, "text/html", cardPage("Self", "")},
	}}
	c := newStatusesClientWith(t, nil, platform.AppOptions{HTTPClient: web}, "alice")
	ctx := t.Context()
	post := func(body map[string]any) clientapi.Status {
		t.Helper()
		if _, ok := body["visibility"]; !ok {
			body["visibility"] = "public"
		}
		return c.post("alice", body)
	}
	fetch := func(st clientapi.Status) clientapi.Status {
		t.Helper()
		fetchLinkCard(t, c.app, st.Id)
		var got clientapi.Status
		if res := c.do("alice", http.MethodGet, "/statuses/"+st.Id, nil, &got); res.StatusCode != http.StatusOK {
			t.Fatalf("GET status = %d", res.StatusCode)
		}
		return got
	}

	st := post(map[string]any{"text": "#news @alice https://example.test/@alice/1 read https://news.example/a#top and https://media.example/a"})
	if st.Card != nil {
		t.Errorf("card before the fetch = %+v", st.Card)
	}
	got := fetch(st)
	card := got.Card
	if card == nil || card.Url != "https://news.example/a" || card.Title != "Big news" || card.Description != "A & B" ||
		card.SiteName != "News" || deref(card.Language) != "en" || card.Image == nil {
		t.Fatalf("card = %+v", card)
	}
	img := card.Image
	if img.Width != 64 || img.Height != 48 || deref(img.Blurhash) == "" || deref(img.Description) != "A chart" ||
		!strings.HasPrefix(img.Url, "https://example.test/media/proxy/") || !strings.Contains(img.Url, "/small?") {
		t.Errorf("image = %+v", img)
	}
	u, err := url.Parse(img.Url)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u.RequestURI(), nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/webp" {
		t.Errorf("GET card image = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if n := web.count("https://example.test/@alice/1"); n != 0 {
		t.Errorf("fetched a URL of this instance %d times", n)
	}

	// A remote status with the same link shares the card: nothing is fetched again. Its
	// mentions are not links of a card.
	var remoteID string
	err = c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a, err := c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: "https://remote.example/users/bob", Domain: "remote.example", Type: "Person", PreferredUsername: "bob",
			InboxURL: "https://remote.example/users/bob/inbox", Document: []byte(`{}`),
		})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		p, err := c.app.Content.IngestRemoteStatus(ctx, tx, appcontent.RemoteStatusInput{
			URI: "https://remote.example/statuses/1", AuthorAccountID: a.ID, AuthorDomain: "remote.example",
			Visibility: domaincontent.VisibilityPublic, PublishedAt: now, RevisedAt: now,
			Bodies: []appcontent.RemoteBody{{Language: "en", HTML: `<p><span class="h-card"><a href="https://remote.example/@carol" class="u-url mention">@<span>carol</span></a></span> <a href="https://news.example/a">news</a></p>`}},
		})
		remoteID = p.Status.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := fetch(clientapi.Status{Id: remoteID}); got.Card == nil || got.Card.Title != "Big news" || web.count("https://news.example/a") != 1 {
		t.Errorf("card of the remote status = %+v after %d fetches", got.Card, web.count("https://news.example/a"))
	}

	// Another status with the same link shares the card: nothing is fetched again.
	again := post(map[string]any{"text": "https://NEWS.example:443/a", "visibility": "direct"})
	if got := fetch(again); got.Card == nil || got.Card.Title != "Big news" || web.count("https://news.example/a") != 1 {
		t.Errorf("shared card = %+v after %d fetches", got.Card, web.count("https://news.example/a"))
	}

	// No card for a status with attachments, for a page without a title, and for an SVG image.
	m, err := c.app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: c.ids["alice"], Content: bytes.NewReader(png)})
	if err != nil {
		t.Fatal(err)
	}
	c.app.Media.Process(ctx, m.File.ID, "")
	withMedia := post(map[string]any{"text": "https://media.example/a", "media": []map[string]any{{"id": m.File.ID}}})
	if got := fetch(withMedia); got.Card != nil || web.count("https://media.example/a") != 0 {
		t.Errorf("card of a status with attachments = %+v", got.Card)
	}
	if got := fetch(post(map[string]any{"text": "https://untitled.example"})); got.Card != nil {
		t.Errorf("card of a page without a title = %+v", got.Card)
	}
	if got := fetch(post(map[string]any{"text": "https://svg.example/a"})); got.Card == nil || got.Card.Image != nil {
		t.Errorf("card with an SVG image = %+v", got.Card)
	}

	// An edit that drops the link drops the card.
	var edited clientapi.Status
	if res := c.do("alice", http.MethodPut, "/statuses/"+st.Id, map[string]any{"text": "no links"}, &edited); res.StatusCode != http.StatusOK || edited.Card != nil {
		t.Errorf("edit = %d, card %+v", res.StatusCode, edited.Card)
	}

	// reject_media leaves the card without its image; suspend hides the card and stops fetching.
	pictures := fetch(post(map[string]any{"text": "https://media.example/a"}))
	if pictures.Card == nil || pictures.Card.Image == nil {
		t.Fatalf("card = %+v", pictures.Card)
	}
	if _, err := c.app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "media.example", Kind: moderation.DomainRejectMedia}); err != nil {
		t.Fatal(err)
	}
	if got := fetch(pictures); got.Card == nil || got.Card.Image != nil {
		t.Errorf("card under reject_media = %+v", got.Card)
	}
	if _, err := c.app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "blocked.example", Kind: moderation.DomainSuspend}); err != nil {
		t.Fatal(err)
	}
	if got := fetch(post(map[string]any{"text": "https://blocked.example/a"})); got.Card != nil || web.count("https://blocked.example/a") != 0 {
		t.Errorf("card of a suspended domain = %+v after %d fetches", got.Card, web.count("https://blocked.example/a"))
	}

	// link_cards off hides every card.
	if _, err := c.app.Instance.SetSetting(ctx, instance.SetSettingInput{Key: domaininstance.SettingLinkCards, Value: "false"}); err != nil {
		t.Fatal(err)
	}
	if got := fetch(again); got.Card != nil {
		t.Errorf("card with link_cards off = %+v", got.Card)
	}
}

// TestLinkCardsRefuseInternalAddresses posts links to a server on the loopback address, by its
// address and by a name, and checks that the SSRF guard of the HTTPClient (ADR-0019 §1) keeps
// the fetch of their cards from reaching it.
func TestLinkCardsRefuseInternalAddresses(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(cardPage("Internal", ""))
	}))
	t.Cleanup(srv.Close)
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]
	c := newStatusesClient(t, "alice")
	for _, link := range []string{srv.URL + "/admin", "https://localhost" + port + "/admin", "http://127.0.0.1" + port + "/admin", "https://[::1]" + port + "/"} {
		st := c.post("alice", map[string]any{"text": "see " + link, "visibility": "public"})
		fetchLinkCard(t, c.app, st.Id)
		var got clientapi.Status
		if res := c.do("alice", http.MethodGet, "/statuses/"+st.Id, nil, &got); res.StatusCode != http.StatusOK || got.Card != nil {
			t.Errorf("status linking %s = %d, card %+v", link, res.StatusCode, got.Card)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the internal server got %d requests", n)
	}
}

// TestLinkCardCreator reads the fediverse:creator of pages (ADR-0062 §5): the account is the
// card's author when its actor lists the page's domain among its attributionDomains, and no one
// otherwise.
func TestLinkCardCreator(t *testing.T) {
	t.Parallel()
	actor := "https://social.example/users/ann"
	page := func(title string) []byte {
		return []byte(`<html><head><meta property="og:title" content="` + title + `">
<meta name="fediverse:creator" content="@ann@social.example"></head></html>`)
	}
	web := &pageWeb{hits: map[string]int{}, pages: map[string]webPage{
		"https://social.example/.well-known/webfinger?resource=acct%3Aann%40social.example": {http.StatusOK, "application/jrd+json", []byte(
			`{"subject":"acct:ann@social.example","links":[{"rel":"self","type":"application/activity+json","href":"` + actor + `"}]}`)},
		actor: {http.StatusOK, "application/activity+json", []byte(`{"@context":["https://www.w3.org/ns/activitystreams",
{"toot":"http://joinmastodon.org/ns#","attributionDomains":{"@id":"toot:attributionDomains","@type":"@id"}}],
"id":"` + actor + `","type":"Person","preferredUsername":"ann","inbox":"` + actor + `/inbox","attributionDomains":["blog.example"]}`)},
		"https://www.blog.example/post": {http.StatusOK, "text/html", page("Verified")},
		"https://other.example/post":    {http.StatusOK, "text/html", page("Unverified")},
	}}
	c := newStatusesClientWith(t, nil, platform.AppOptions{HTTPClient: web}, "alice")
	for link, want := range map[string]string{"https://www.blog.example/post": "ann@social.example", "https://other.example/post": ""} {
		st := c.post("alice", map[string]any{"text": link, "visibility": "public"})
		fetchLinkCard(t, c.app, st.Id)
		var got clientapi.Status
		if res := c.do("alice", http.MethodGet, "/statuses/"+st.Id, nil, &got); res.StatusCode != http.StatusOK || got.Card == nil {
			t.Fatalf("status linking %s = %d, card %+v", link, res.StatusCode, got.Card)
		}
		var acct string
		if got.Card.Author != nil {
			acct = got.Card.Author.Acct
		}
		if acct != want {
			t.Errorf("author of the card of %s = %q, want %q", link, acct, want)
		}
	}
}
