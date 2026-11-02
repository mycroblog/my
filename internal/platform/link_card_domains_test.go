// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domaininstance "github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// redirectingWeb is a pageWeb that follows redirects by URL, asking the request's CheckRedirect
// before each, as the HTTPClient does.
type redirectingWeb struct {
	*pageWeb
	redirects map[string]string
}

func (w *redirectingWeb) Do(ctx context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	to, ok := w.redirects[req.URL]
	if !ok {
		return w.pageWeb.Do(ctx, req)
	}
	if req.CheckRedirect != nil {
		if err := req.CheckRedirect(ctx, to); err != nil {
			return nil, err
		}
	}
	next := *req
	next.URL = to
	return w.pageWeb.Do(ctx, &next)
}

// linkCardRulesEnv posts statuses, fetches their cards, and sets the rules of the crawler.
type linkCardRulesEnv struct {
	t   *testing.T
	c   *statusesClient
	web *pageWeb
}

func (e *linkCardRulesEnv) post(link string) clientapi.Status {
	e.t.Helper()
	return e.c.post("alice", map[string]any{"text": link, "visibility": "public"})
}

// read returns the card of a status after running the fetch that its job would.
func (e *linkCardRulesEnv) read(st clientapi.Status) *clientapi.LinkCard {
	e.t.Helper()
	fetchLinkCard(e.t, e.c.app, st.Id)
	var got clientapi.Status
	if res := e.c.do("alice", http.MethodGet, "/statuses/"+st.Id, nil, &got); res.StatusCode != http.StatusOK {
		e.t.Fatalf("GET status = %d", res.StatusCode)
	}
	return got.Card
}

func (e *linkCardRulesEnv) rule(domain string, mode domaininstance.LinkCardDomainMode) {
	e.t.Helper()
	if _, err := e.c.app.Instance.SetLinkCardDomainRule(e.t.Context(), domaininstance.LinkCardDomainRule{Domain: domain, Mode: mode}, ""); err != nil {
		e.t.Fatal(err)
	}
}

func (e *linkCardRulesEnv) remove(domain string) {
	e.t.Helper()
	if err := e.c.app.Instance.RemoveLinkCardDomainRule(e.t.Context(), domain, ""); err != nil {
		e.t.Fatal(err)
	}
}

func newLinkCardRulesEnv(t *testing.T, pages map[string]webPage, redirects map[string]string) *linkCardRulesEnv {
	t.Helper()
	web := &pageWeb{hits: map[string]int{}, pages: pages}
	opts := platform.AppOptions{HTTPClient: &redirectingWeb{pageWeb: web, redirects: redirects}}
	return &linkCardRulesEnv{t: t, c: newStatusesClientWith(t, nil, opts, "alice"), web: web}
}

func htmlPage(body []byte) webPage { return webPage{http.StatusOK, "text/html", body} }

// TestLinkCardRuleStopsFetching checks no_fetch: nothing is fetched from the domain or its
// subdomains, kept cards hide at once and return when the rule goes, and a skipped fetch leaves no
// failure memory, so that the URL gets its card the next time a post needs it.
func TestLinkCardRuleStopsFetching(t *testing.T) {
	t.Parallel()
	png := testkit.FixturePNG(64, 48, false)
	e := newLinkCardRulesEnv(t, map[string]webPage{
		"https://news.example/kept":      htmlPage(cardPage("Kept", "/kept.png")),
		"https://news.example/kept.png":  {http.StatusOK, "image/png", png},
		"https://news.example/later":     htmlPage(cardPage("Later", "")),
		"https://sub.news.example/later": htmlPage(cardPage("Sub", "")),
		"https://other.example/a":        htmlPage(cardPage("Other", "")),
	}, nil)

	kept := e.post("https://news.example/kept")
	if card := e.read(kept); card == nil || card.Image == nil {
		t.Fatalf("card before the rule = %+v", card)
	}

	e.rule("news.example", domaininstance.LinkCardNoFetch)
	if card := e.read(kept); card != nil {
		t.Errorf("kept card under no_fetch = %+v", card)
	}
	skipped := e.post("https://news.example/later")
	if card := e.read(skipped); card != nil || e.web.count("https://news.example/later") != 0 {
		t.Errorf("card under no_fetch = %+v after %d fetches", card, e.web.count("https://news.example/later"))
	}
	sub := e.post("https://sub.news.example/later")
	if card := e.read(sub); card != nil || e.web.count("https://sub.news.example/later") != 0 {
		t.Errorf("card of a subdomain = %+v after %d fetches", card, e.web.count("https://sub.news.example/later"))
	}
	if card := e.read(e.post("https://other.example/a")); card == nil {
		t.Error("the rule of a domain hid the card of another")
	}

	// The rule bears only on link cards: the domain stays federated and its media are fetched.
	ctx := t.Context()
	if suspended, err := e.c.app.ModerationPolicy.FederationSuspended(ctx, "news.example"); err != nil || suspended {
		t.Errorf("FederationSuspended = %v, %v", suspended, err)
	}
	if rejected, err := e.c.app.ModerationPolicy.RejectsMedia(ctx, "news.example"); err != nil || rejected {
		t.Errorf("RejectsMedia = %v, %v", rejected, err)
	}

	e.remove("news.example")
	card := e.read(kept)
	if card == nil || card.Title != "Kept" || card.Image == nil {
		t.Errorf("kept card after the rule = %+v", card)
	}
	if n := e.web.count("https://news.example/kept"); n != 1 {
		t.Errorf("the kept card was fetched %d times, want once: removing a rule refetches nothing", n)
	}
	// No failure memory: the same URL fetches at once.
	if card := e.read(skipped); card == nil || card.Title != "Later" {
		t.Errorf("card of the skipped URL after the rule = %+v", card)
	}
}

// TestLinkCardRuleStopsImages checks no_images: cards have no image, and no image is fetched,
// when the page's host or the image's host is covered; kept cards lose their image at once and get
// it back when the rule goes; the media proxy does not serve it meanwhile.
func TestLinkCardRuleStopsImages(t *testing.T) {
	t.Parallel()
	png := testkit.FixturePNG(64, 48, false)
	e := newLinkCardRulesEnv(t, map[string]webPage{
		"https://news.example/kept":        htmlPage(cardPage("Kept", "/kept.png")),
		"https://news.example/kept.png":    {http.StatusOK, "image/png", png},
		"https://news.example/new":         htmlPage(cardPage("New", "/new.png")),
		"https://news.example/new.png":     {http.StatusOK, "image/png", png},
		"https://www.page.example/a":       htmlPage(cardPage("Page", "https://cdn.other.example/a.png")),
		"https://cdn.other.example/a.png":  {http.StatusOK, "image/png", png},
		"https://free.example/a":           htmlPage(cardPage("Free", "https://cdn.images.example/b.png")),
		"https://cdn.images.example/b.png": {http.StatusOK, "image/png", png},
	}, nil)
	kept := e.post("https://news.example/kept")
	before := e.read(kept)
	if before == nil || before.Image == nil {
		t.Fatalf("card before the rule = %+v", before)
	}
	proxied := func() int {
		t.Helper()
		u, err := url.Parse(before.Image.Url)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		e.c.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u.RequestURI(), nil))
		return rec.Code
	}
	if code := proxied(); code != http.StatusOK {
		t.Fatalf("GET card image = %d", code)
	}

	e.rule("news.example", domaininstance.LinkCardNoImages)
	if card := e.read(kept); card == nil || card.Title != "Kept" || card.Image != nil {
		t.Errorf("kept card under no_images = %+v", card)
	}
	if code := proxied(); code != http.StatusNotFound {
		t.Errorf("GET card image under no_images = %d, want 404", code)
	}
	if card := e.read(e.post("https://news.example/new")); card == nil || card.Title != "New" || card.Image != nil ||
		e.web.count("https://news.example/new.png") != 0 {
		t.Errorf("new card under no_images = %+v after %d image fetches", card, e.web.count("https://news.example/new.png"))
	}

	// The rule on the page's parent stops an image on another host.
	e.rule("page.example", domaininstance.LinkCardNoImages)
	if card := e.read(e.post("https://www.page.example/a")); card == nil || card.Image != nil || e.web.count("https://cdn.other.example/a.png") != 0 {
		t.Errorf("card of a covered page = %+v after %d image fetches", card, e.web.count("https://cdn.other.example/a.png"))
	}
	// The rule on the image's parent stops it on a page elsewhere, and no_fetch on it does too.
	e.rule("images.example", domaininstance.LinkCardNoFetch)
	if card := e.read(e.post("https://free.example/a")); card == nil || card.Title != "Free" || card.Image != nil ||
		e.web.count("https://cdn.images.example/b.png") != 0 {
		t.Errorf("card of a covered image = %+v after %d image fetches", card, e.web.count("https://cdn.images.example/b.png"))
	}

	e.remove("news.example")
	if card := e.read(kept); card == nil || card.Image == nil {
		t.Errorf("kept card after the rule = %+v", card)
	}
	if code := proxied(); code != http.StatusOK {
		t.Errorf("GET card image after the rule = %d", code)
	}
}

// TestLinkCardRuleCoversRedirects checks that a redirect into a covered domain ends the fetch,
// with no failure memory for the page and no image for the card.
func TestLinkCardRuleCoversRedirects(t *testing.T) {
	t.Parallel()
	png := testkit.FixturePNG(64, 48, false)
	e := newLinkCardRulesEnv(t, map[string]webPage{
		"https://stop.example/page":    htmlPage(cardPage("Moved", "")),
		"https://site.example/a":       htmlPage(cardPage("Site", "https://site.example/a.png")),
		"https://images.example/a.png": {http.StatusOK, "image/png", png},
	}, map[string]string{
		"https://short.example/x":    "https://stop.example/page",
		"https://site.example/a.png": "https://images.example/a.png",
	})
	e.rule("stop.example", domaininstance.LinkCardNoFetch)
	e.rule("images.example", domaininstance.LinkCardNoImages)

	short := e.post("https://short.example/x")
	if card := e.read(short); card != nil || e.web.count("https://stop.example/page") != 0 {
		t.Errorf("card through a redirect = %+v after %d fetches", card, e.web.count("https://stop.example/page"))
	}
	if card := e.read(e.post("https://site.example/a")); card == nil || card.Image != nil || e.web.count("https://images.example/a.png") != 0 {
		t.Errorf("card with a redirected image = %+v after %d fetches", card, e.web.count("https://images.example/a.png"))
	}
	e.remove("stop.example")
	if card := e.read(short); card == nil || card.Title != "Moved" {
		t.Errorf("card through the redirect after the rule = %+v", card)
	}
}

// TestLinkCardRuleIsAudited checks the entries that the changes of a rule record.
func TestLinkCardRuleIsAudited(t *testing.T) {
	t.Parallel()
	e := newLinkCardRulesEnv(t, nil, nil)
	e.rule("news.example", domaininstance.LinkCardNoFetch)
	e.rule("news.example", domaininstance.LinkCardNoImages)
	e.remove("news.example")
	e.remove("news.example") // Removing a rule that is gone records nothing.
	got, err := e.c.app.Moderation.AuditLog(t.Context(), appmoderation.AuditQuery{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []moderation.AuditAction{
		moderation.AuditLinkCardDomainRuleRemoved, moderation.AuditLinkCardDomainImagesStopped, moderation.AuditLinkCardDomainFetchStopped,
	}
	var actions []moderation.AuditAction
	for _, entry := range got {
		if entry.TargetKind != moderation.AuditTargetLinkCardDomain {
			continue
		}
		actions = append(actions, entry.Action)
		if entry.TargetID != "news.example" || entry.TargetLabel != "news.example" {
			t.Errorf("entry = %+v, want the domain as its target", entry)
		}
	}
	if len(actions) != len(want) {
		t.Fatalf("actions = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Errorf("actions = %v, want %v", actions, want)
		}
	}
}
