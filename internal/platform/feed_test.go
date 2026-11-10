// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/xml"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// atomDocument is what the test reads of an Atom feed.
type atomDocument struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated"`
	Links   []struct {
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Entries []struct {
		ID      string `xml:"id"`
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Author  struct {
			Name string `xml:"name"`
			URI  string `xml:"uri"`
		} `xml:"author"`
		Summary struct {
			Type string `xml:"type,attr"`
			Text string `xml:",chardata"`
		} `xml:"summary"`
		Content struct {
			Type string `xml:"type,attr"`
			Text string `xml:",chardata"`
		} `xml:"content"`
		Categories []struct {
			Term string `xml:"term,attr"`
		} `xml:"category"`
	} `xml:"entry"`
}

// TestAtomFeeds checks end to end which posts the Atom feeds of profiles and hashtags show (FR-213):
// only the local public posts that are not replies or boosts, of authors whose feed is on and who
// are neither restricted nor moved. Without a choice of its own, a feed follows indexable. Articles
// appear with their title and summary, and a profile's articles-only feed lists only its articles.
func TestAtomFeeds(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	build := fstest.MapFS{
		".vite/manifest.json":  {Data: []byte(`{"index.html": {"file": "assets/index-AAAA.js", "isEntry": true}}`)},
		"assets/index-AAAA.js": {Data: []byte("export {}")},
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), WebBuild: build})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ctx := t.Context()
	yes, no := true, false
	accounts := map[string]string{}
	// New accounts are not indexable on an instance without settings, so their feeds are off
	// until they choose.
	for name, in := range map[string]appaccount.ProfileUpdate{
		"alice": {Indexable: &yes},                    // follows indexable: on
		"bob":   {FeedEnabled: &yes},                  // not indexable, but chose the feed
		"carol": {Indexable: &yes, FeedEnabled: &no},  // indexable, but turned the feed off
		"dave":  {},                                   // follows indexable: off
		"erin":  {Indexable: &yes},                    // limited below
		"frank": {Indexable: &yes},                    // moved below
		"gwen":  {FeedEnabled: &no, Indexable: &no},   // turned it off, then indexable below
		"hank":  {Indexable: &yes, FeedEnabled: &yes}, // on, and stays on when not indexable below
		"ivy":   {Indexable: &no},                     // reads the hashtag's feed only
	} {
		reg, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: name, Password: "correct horse battery staple", DisplayName: strings.ToUpper(name[:1]) + name[1:], Locale: "en",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser(%s): %v", name, err)
		}
		accounts[name] = reg.AccountID
		if _, err := app.Accounts.UpdateProfile(ctx, reg.AccountID, in); err != nil {
			t.Fatalf("UpdateProfile(%s): %v", name, err)
		}
	}
	// A later change of indexable leaves an explicit choice as it is.
	if _, err := app.Accounts.UpdateProfile(ctx, accounts["gwen"], appaccount.ProfileUpdate{Indexable: &yes}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Accounts.UpdateProfile(ctx, accounts["hank"], appaccount.ProfileUpdate{Indexable: &no}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: accounts["erin"], Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam,
	}); err != nil {
		t.Fatalf("IssueAccountAction(limit): %v", err)
	}
	if _, err := app.Accounts.RecordLocalMove(ctx, appaccount.LocalMoveInput{AccountID: accounts["frank"], TargetAccountID: accounts["alice"]}); err != nil {
		t.Fatalf("RecordLocalMove: %v", err)
	}

	post := func(author string, vis domcontent.Visibility, in appcontent.TextInput, replyTo string) string {
		t.Helper()
		in.Language = "en"
		p, err := app.Content.Create(ctx, appcontent.CreateInput{AuthorAccountID: accounts[author], Visibility: vis, TextInput: in, InReplyToID: replyTo})
		if err != nil {
			t.Fatalf("Create(%s, %s): %v", author, in.Text, err)
		}
		return p.Status.ID
	}
	text := func(s string) appcontent.TextInput { return appcontent.TextInput{Text: s} }
	ids := map[string]string{}
	ids["first"] = post("alice", domcontent.VisibilityPublic, text("first #cats <b>post</b>"), "")
	for _, vis := range []domcontent.Visibility{domcontent.VisibilityLocal, domcontent.VisibilityUnlisted, domcontent.VisibilityPrivate, domcontent.VisibilityDirect} {
		post("alice", vis, text("a "+string(vis)+" post #cats"), "")
	}
	post("alice", domcontent.VisibilityPublic, text("a reply #cats"), ids["first"])
	ids["warned"] = post("alice", domcontent.VisibilityPublic, appcontent.TextInput{Text: "the hidden body #cats", SpoilerText: "spoilers"}, "")
	ids["bob"] = post("bob", domcontent.VisibilityPublic, text("bob's #cats"), "")
	if _, err := app.Interactions.Boost(ctx, accounts["alice"], ids["bob"]); err != nil {
		t.Fatalf("Boost: %v", err)
	}
	for _, name := range []string{"carol", "dave", "erin", "gwen"} {
		post(name, domcontent.VisibilityPublic, text(name+"'s #cats"), "")
	}
	ids["hank"] = post("hank", domcontent.VisibilityPublic, text("hank's #cats"), "")
	article := func(author string, vis domcontent.Visibility, title, summary string) string {
		t.Helper()
		p, err := app.Content.CreateArticle(ctx, appcontent.CreateArticleInput{
			AuthorAccountID: accounts[author], Visibility: vis,
			ArticleInput: appcontent.ArticleInput{
				Title: title, Summary: summary, Text: "## The body\n\nThe whole *body* of " + title + ".", Tags: []string{"cats"}, Language: "en",
			},
		})
		if err != nil {
			t.Fatalf("CreateArticle(%s, %s): %v", author, title, err)
		}
		return p.Status.ID
	}
	ids["article"] = article("alice", domcontent.VisibilityPublic, "A long read", "What the article is about")
	ids["summarized"] = article("alice", domcontent.VisibilityPublic, "Another read", "")
	article("alice", domcontent.VisibilityUnlisted, "An unlisted read", "")
	article("dave", domcontent.VisibilityPublic, "Dave's read", "")

	h := app.Handler()
	get := func(path string, header ...string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.Host+path, nil)
		req.Header.Set("Accept", "application/atom+xml")
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	read := func(path string) (atomDocument, *httptest.ResponseRecorder) {
		t.Helper()
		rec := get(path)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/atom+xml; charset=utf-8" {
			t.Fatalf("GET %s = %d %v:\n%s", path, rec.Code, rec.Header(), rec.Body)
		}
		var doc atomDocument
		if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("GET %s: %v\n%s", path, err, rec.Body)
		}
		if doc.ID == "" || doc.Title == "" || doc.Updated == "" {
			t.Errorf("GET %s: feed without id, title, or updated:\n%s", path, rec.Body)
		}
		for _, e := range doc.Entries {
			if e.ID == "" || e.Title == "" || e.Author.Name == "" {
				t.Errorf("GET %s: entry without id, title, or author:\n%s", path, rec.Body)
			}
			if _, err := time.Parse(time.RFC3339, e.Updated); err != nil {
				t.Errorf("GET %s: updated %q: %v", path, e.Updated, err)
			}
		}
		return doc, rec
	}
	entryIDs := func(doc atomDocument) []string {
		var out []string
		for _, e := range doc.Entries {
			out = append(out, e.ID[strings.LastIndex(e.ID, "/")+1:])
		}
		return out
	}

	alice, rec := read("/@alice.atom")
	if got, want := entryIDs(alice), []string{ids["summarized"], ids["article"], ids["warned"], ids["first"]}; !slices.Equal(got, want) {
		t.Fatalf("alice's feed = %v, want %v:\n%s", got, want, rec.Body)
	}
	if e := alice.Entries[1]; e.Title != "A long read" || e.Summary.Type != "text" || e.Summary.Text != "What the article is about" ||
		e.Content.Text != "" || len(e.Categories) != 1 || e.Categories[0].Term != "cats" || strings.Contains(rec.Body.String(), "h2") {
		t.Errorf("the article's entry = %+v, want its title, its summary, and its hashtag, without the body", e)
	}
	if e := alice.Entries[0]; e.Title != "Another read" || e.Summary.Text != "The whole body of Another read." {
		t.Errorf("the entry of an article without a summary = %+v, want the summary made from the body", e)
	}
	if e := alice.Entries[2]; e.Title != "spoilers" || e.Content.Type != "text" || !strings.Contains(e.Content.Text, "spoilers") ||
		strings.Contains(rec.Body.String(), "the hidden body") {
		t.Errorf("the entry behind a content warning = %+v, want the warning alone", e)
	}
	if e := alice.Entries[3]; e.Content.Type != "html" || !strings.Contains(e.Content.Text, "first") ||
		len(e.Categories) != 1 || e.Categories[0].Term != "cats" {
		t.Errorf("the first entry = %+v, want its HTML and its hashtag", e)
	}
	// A conditional request of the same feed is answered from its ETag.
	if rec := get("/@alice.atom", "If-None-Match", rec.Header().Get("ETag")); rec.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d, want 304", rec.Code)
	}
	if rec.Header().Get("Last-Modified") == "" {
		t.Errorf("feed without Last-Modified: %v", rec.Header())
	}
	if doc, _ := read("/@bob.atom"); !slices.Equal(entryIDs(doc), []string{ids["bob"]}) {
		t.Errorf("bob's feed = %v, want his post", entryIDs(doc))
	}
	if doc, _ := read("/@hank.atom"); !slices.Equal(entryIDs(doc), []string{ids["hank"]}) {
		t.Errorf("hank's feed = %v, want his post", entryIDs(doc))
	}
	for _, name := range []string{"carol", "dave", "erin", "frank", "gwen", "nobody"} {
		if rec := get("/@" + name + ".atom"); rec.Code != http.StatusNotFound {
			t.Errorf("GET /@%s.atom = %d, want 404", name, rec.Code)
		}
	}

	articles, rec := read("/@alice/articles.atom")
	if got, want := entryIDs(articles), []string{ids["summarized"], ids["article"]}; !slices.Equal(got, want) {
		t.Errorf("alice's articles feed = %v, want %v:\n%s", got, want, rec.Body)
	}
	if want := "https://" + cfg.Host + "/@alice/articles.atom"; articles.ID != want || !strings.HasPrefix(articles.Title, "Articles by Alice") {
		t.Errorf("alice's articles feed = %q, %q; want the ID %s and its own title", articles.ID, articles.Title, want)
	}
	if doc, _ := read("/@bob/articles.atom"); len(doc.Entries) != 0 {
		t.Errorf("bob's articles feed = %v, want none", entryIDs(doc))
	}
	for _, name := range []string{"carol", "dave", "nobody"} {
		if rec := get("/@" + name + "/articles.atom"); rec.Code != http.StatusNotFound {
			t.Errorf("GET /@%s/articles.atom = %d, want 404", name, rec.Code)
		}
	}

	tag, rec := read("/tags/cats.atom")
	if got, want := entryIDs(tag), []string{ids["summarized"], ids["article"], ids["hank"], ids["bob"], ids["warned"], ids["first"]}; !slices.Equal(got, want) {
		t.Errorf("the hashtag's feed = %v, want %v:\n%s", got, want, rec.Body)
	}
	if rec := get("/tags/dogs.atom"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /tags/dogs.atom = %d, want 404", rec.Code)
	}

	// The HTML pages link to the feeds that exist.
	const link = `<link rel="alternate" type="application/atom+xml" title="`
	page := func(path string) string {
		t.Helper()
		rec := get(path, "Accept", acceptBrowser)
		return rec.Body.String()
	}
	if body := page("/@alice"); !strings.Contains(body, link) || !strings.Contains(body, `href="https://`+cfg.Host+`/@alice.atom"`) {
		t.Errorf("alice's page has no link to her feed:\n%s", body)
	}
	if body := page("/@dave"); strings.Contains(body, "application/atom+xml") {
		t.Errorf("dave's page links to a feed:\n%s", body)
	}
	if body := page("/tags/cats"); !strings.Contains(body, `href="https://`+cfg.Host+`/tags/cats.atom"`) {
		t.Errorf("the hashtag's page has no link to its feed:\n%s", body)
	}
}
