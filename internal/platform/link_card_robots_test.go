// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// agentWeb is a redirectingWeb that keeps who each request said it was.
type agentWeb struct {
	*redirectingWeb
	mu     sync.Mutex
	agents map[string][]kernel.HTTPAgent
}

func (w *agentWeb) Do(ctx context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	w.mu.Lock()
	w.agents[req.URL] = append(w.agents[req.URL], req.Agent)
	w.mu.Unlock()
	return w.redirectingWeb.Do(ctx, req)
}

// TestLinkCardRobotsThroughTheAPI runs robots.txt, the page's directives, and the setting through
// the real stores and the client API (ADR-0131), and checks that the fetches of a card name the
// crawler while the media proxy and the verification of a creator keep the server's name.
func TestLinkCardRobotsThroughTheAPI(t *testing.T) {
	t.Parallel()
	png := testkit.FixturePNG(64, 48, false)
	const actor = "https://social.example/users/ann"
	const webfinger = "https://social.example/.well-known/webfinger?resource=acct%3Aann%40social.example"
	web := &agentWeb{agents: map[string][]kernel.HTTPAgent{}, redirectingWeb: &redirectingWeb{
		redirects: map[string]string{"https://short.example/x": "https://site.example/private/moved"},
		pageWeb: &pageWeb{hits: map[string]int{}, pages: map[string]webPage{
			"https://site.example/robots.txt": {http.StatusOK, "text/plain", []byte(
				"User-agent: *\nDisallow: /private/\n\nUser-agent: my-linkcard\nDisallow: /private/\nDisallow: /cards-off\n")},
			"https://site.example/open":          htmlPage(cardPage("Open", "https://images.example/open.png")),
			"https://images.example/open.png":    {http.StatusOK, "image/png", png},
			"https://site.example/private/a":     htmlPage(cardPage("Private", "")),
			"https://site.example/private/moved": htmlPage(cardPage("Moved", "")),
			"https://site.example/cards-off":     htmlPage(cardPage("Off", "")),
			"https://images.example/robots.txt":  {http.StatusOK, "text/plain", []byte("User-agent: my-linkcard\nDisallow: /no/\n")},
			"https://site.example/refused-image": htmlPage(cardPage("Imageless", "https://images.example/no/a.png")),
			"https://images.example/no/a.png":    {http.StatusOK, "image/png", png},
			"https://site.example/quiet": htmlPage([]byte(`<head><meta property="og:title" content="Quiet"><meta property="og:description" content="words">
<meta name="robots" content="nosnippet, noimageindex"><meta property="og:image" content="https://images.example/open.png"></head>`)),
			webfinger: {http.StatusOK, "application/jrd+json", []byte(
				`{"subject":"acct:ann@social.example","links":[{"rel":"self","type":"application/activity+json","href":"` + actor + `"}]}`)},
			actor: {http.StatusOK, "application/activity+json", []byte(`{"@context":["https://www.w3.org/ns/activitystreams"],
"id":"` + actor + `","type":"Person","preferredUsername":"ann","inbox":"` + actor + `/inbox"}`)},
			"https://site.example/creator": htmlPage([]byte(`<head><meta property="og:title" content="By Ann"><meta name="fediverse:creator" content="@ann@social.example"></head>`)),
		}},
	}}
	c := newStatusesClientWith(t, nil, platform.AppOptions{HTTPClient: web}, "alice")
	e := &linkCardRulesEnv{t: t, c: c, web: web.pageWeb}

	open := e.read(e.post("https://site.example/open"))
	if open == nil || open.Title != "Open" || open.Image == nil {
		t.Fatalf("card of an allowed page = %+v", open)
	}
	for _, refused := range []string{"https://site.example/private/a", "https://site.example/cards-off"} {
		if card := e.read(e.post(refused)); card != nil || e.web.count(refused) != 0 {
			t.Errorf("card of %s = %+v after %d fetches", refused, card, e.web.count(refused))
		}
	}
	if card := e.read(e.post("https://short.example/x")); card != nil || e.web.count("https://site.example/private/moved") != 0 {
		t.Errorf("card through a redirect to a disallowed URL = %+v", card)
	}
	if card := e.read(e.post("https://site.example/refused-image")); card == nil || card.Image != nil || e.web.count("https://images.example/no/a.png") != 0 {
		t.Errorf("card with a disallowed image = %+v", card)
	}
	quiet := e.read(e.post("https://site.example/quiet"))
	if quiet == nil || quiet.Title != "Quiet" || quiet.Description != "" || quiet.Image != nil {
		t.Errorf("card of a page with directives = %+v", quiet)
	}
	if n := e.web.count("https://site.example/robots.txt"); n != 1 {
		t.Errorf("robots.txt fetched %d times, want once", n)
	}

	// The media proxy serves the kept image as the server, not as the crawler.
	u, err := url.Parse(open.Image.Url)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u.RequestURI(), nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET card image = %d", rec.Code)
	}

	// So does the verification of a creator, which is a federation lookup.
	if card := e.read(e.post("https://site.example/creator")); card == nil {
		t.Error("no card of the page with a creator")
	}

	web.mu.Lock()
	defer web.mu.Unlock()
	for u, want := range map[string]kernel.HTTPAgent{
		"https://site.example/robots.txt": kernel.HTTPAgentLinkCard,
		"https://site.example/open":       kernel.HTTPAgentLinkCard,
		"https://short.example/x":         kernel.HTTPAgentLinkCard,
		webfinger:                         kernel.HTTPAgentServer,
		actor:                             kernel.HTTPAgentServer,
	} {
		agents := web.agents[u]
		if len(agents) == 0 {
			t.Errorf("no request of %s", u)
		}
		for _, got := range agents {
			if got != want {
				t.Errorf("agent of %s = %v, want %v", u, got, want)
			}
		}
	}
	image := web.agents["https://images.example/open.png"]
	if len(image) != 2 || image[0] != kernel.HTTPAgentLinkCard || image[1] != kernel.HTTPAgentServer {
		t.Errorf("agents of the image = %v, want the crawler's fetch, then the media proxy's", image)
	}
}

// TestLinkCardRobotsSetting turns link_cards_ignore_robots_txt on through the settings: robots.txt
// is no longer asked and a URL it refused gets its card.
func TestLinkCardRobotsSetting(t *testing.T) {
	t.Parallel()
	e := newLinkCardRulesEnv(t, map[string]webPage{
		"https://site.example/robots.txt": {http.StatusOK, "text/plain", []byte("User-agent: *\nDisallow: /\n")},
		"https://site.example/a":          htmlPage(cardPage("Page", "")),
		"https://site.example/b":          htmlPage(cardPage("Other", "")),
	}, nil)
	st := e.post("https://site.example/a")
	if card := e.read(st); card != nil {
		t.Fatalf("card that robots.txt disallows = %+v", card)
	}
	set := func(value string) {
		t.Helper()
		if _, err := e.c.app.Instance.SetSetting(t.Context(), instance.SetSettingInput{Key: domaininstance.SettingLinkCardsIgnoreRobotsTxt, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	set("true")
	if card := e.read(st); card == nil || card.Title != "Page" {
		t.Errorf("card with the setting on = %+v", card)
	}
	if n := e.web.count("https://site.example/robots.txt"); n != 1 {
		t.Errorf("robots.txt fetched %d times, want only the first, before the setting", n)
	}
	set("false")
	if card := e.read(e.post("https://site.example/b")); card != nil {
		t.Errorf("card with the setting off again = %+v", card)
	}
}
