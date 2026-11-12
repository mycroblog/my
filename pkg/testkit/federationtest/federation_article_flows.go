// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/pkg/testkit"
)

// article returns the object of the stored document of the only activity of type typ about the
// article s, failing unless it is an Article.
func (e *fedEnv) article(t *testing.T, s content.Status, typ federation.ActivityType) map[string]any {
	t.Helper()
	doc := e.document(t, e.activity(t, federation.OutboundStatus, s.ID, typ).ID)
	obj, ok := doc["object"].(map[string]any)
	if !ok || obj["type"] != "Article" {
		t.Fatalf("%s document = %v, want an Article as its object", typ, doc)
	}
	return obj
}

// fedSendsTheArticle checks the Article of a local article (ADR-0160 §9):
// its title as name, its summary (the one made from the body when the author left it empty) as
// summary and never as a content warning, the rendered HTML once as content (a body in one
// language without contentMap, its language in nameMap and summaryMap) and never the Markdown as
// source, its cover as image, and only the editor's hashtags as tag; it goes to the inboxes of a
// post of the same visibility, and its Update and Delete follow the Create as a post's do.
func fedSendsTheArticle(t *testing.T, e *fedEnv) {
	a := e.audience()
	tag := content.Tag{ID: e.ids.NewID(), Name: "Essay", NameNormalized: "essay"}
	cover := appcontent.MediaView{
		Media: appcontent.Media{Result: &content.MediaProcessingResult{
			Outcome: content.MediaReady, MediaType: content.MediaTypeImage, Width: 1600, Height: 900,
		}},
		Description: "a mountain at dawn",
		Image: appcontent.Image{Still: appcontent.ImageVariants{
			Small: appcontent.ImageVariant{URL: "https://files.social.example/media/c/small.webp", ContentType: "image/webp", Width: 400, Height: 225},
			Large: appcontent.ImageVariant{URL: "https://files.social.example/media/c/large.jpg", ContentType: "image/jpeg", Width: 1600, Height: 900},
		}},
	}
	e.parts.mu.Lock()
	e.parts.tags[tag.ID] = tag
	e.parts.mu.Unlock()
	const (
		markdown = "## Morning\n\nWe climbed **early**."
		body     = "<h2>Morning</h2>\n<p>We climbed <strong>early</strong>.</p>"
	)
	post := e.post(t, a.author, content.VisibilityPublic)
	s := e.postWith(t, a.author, content.VisibilityPublic, func(r *content.Revision) {
		r.Bodies = []content.Body{{Language: "en", ContentHTML: body}}
		r.Source = &content.Source{Text: markdown, Format: content.SourceTextMarkdown}
		r.TagIDs = []string{tag.ID}
		r.Article = &content.ArticleRevision{
			Title: "A climb", Summary: "How we climbed the mountain.", SummaryOrigin: content.SummaryByAuthor,
			Cover: &content.ArticleCover{MediaID: "cover", Description: cover.Description},
		}
		e.parts.mu.Lock()
		e.parts.covers[r.ID] = cover
		e.parts.mu.Unlock()
	})
	e.drain(t)

	art := e.article(t, s, federation.ActivityCreate)
	e.accounts.mu.Lock()
	wantURL := content.StatusURL(testkit.ContractHost, e.accounts.actors[a.author].Local.Username, s.ID)
	e.accounts.mu.Unlock()
	if art["name"] != "A climb" || art["summary"] != "How we climbed the mountain." || art["content"] != body ||
		art["url"] != wantURL {
		t.Errorf("Article name = %v, summary = %v, content = %v, url = %v", art["name"], art["summary"], art["content"], art["url"])
	}
	// The body is sent once; nameMap and summaryMap say its language.
	if _, ok := art["contentMap"]; ok || !reflect.DeepEqual(art["nameMap"], map[string]any{"en": "A climb"}) ||
		!reflect.DeepEqual(art["summaryMap"], map[string]any{"en": "How we climbed the mountain."}) {
		t.Errorf("Article contentMap = %v, nameMap = %v, summaryMap = %v; want the language in nameMap and summaryMap only",
			art["contentMap"], art["nameMap"], art["summaryMap"])
	}
	// The summary is not a content warning: an article that is not sensitive says so.
	if art["sensitive"] != false {
		t.Errorf("sensitive = %v, want false", art["sensitive"])
	}
	// The Markdown is not sent: every copy of the body counts against Misskey's 64 KiB inbox
	// limit, and no major implementation reads text/markdown.
	if src, ok := art["source"]; ok {
		t.Errorf("source = %v, want none", src)
	}
	wantImage := map[string]any{"type": "Image", "mediaType": "image/jpeg", "url": cover.Image.Still.Large.URL, "name": cover.Description}
	if !reflect.DeepEqual(art["image"], wantImage) {
		t.Errorf("image = %v, want %v", art["image"], wantImage)
	}
	if got, want := tagsOf(art), [][3]string{{"Hashtag", "https://" + testkit.ContractHost + "/tags/essay", "#Essay"}}; !slices.Equal(got, want) {
		t.Errorf("tag = %v, want %v", got, want)
	}
	if got := art["attachment"]; !reflect.DeepEqual(got, []any{}) {
		t.Errorf("attachment = %v, want none", got)
	}
	created := e.inboxes(t, e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityCreate).ID)
	if want := e.inboxes(t, e.activity(t, federation.OutboundStatus, post.ID, federation.ActivityCreate).ID); !slices.Equal(created, want) {
		t.Errorf("Article inboxes = %v, want those of a public post %v", created, want)
	}

	// The author emptied the summary: the one made from the body is sent, and a sensitive
	// article stays sensitive with its summary showing.
	long := strings.Repeat("We climbed early and walked along the ridge. ", 8)
	made := content.ArticleSummaryFromBody(long)
	e.reviseWith(t, s, func(r *content.Revision) {
		r.Sensitive = true
		r.Bodies = []content.Body{{Language: "en", ContentHTML: "<p>" + long + "</p>"}}
		r.Source = &content.Source{Text: long, Format: content.SourceTextMarkdown}
		r.Article = &content.ArticleRevision{Title: "A climb, edited", Summary: made, SummaryOrigin: content.SummaryFromBody}
	})
	e.drain(t)
	update := e.article(t, s, federation.ActivityUpdate)
	if made == "" || update["summary"] != made || !reflect.DeepEqual(update["summaryMap"], map[string]any{"en": made}) || update["name"] != "A climb, edited" || update["sensitive"] != true ||
		update["updated"] != federation.FormatTime(e.clock.Now()) {
		t.Errorf("Update Article name = %v, summary = %v (want %q), sensitive = %v, updated = %v",
			update["name"], update["summary"], made, update["sensitive"], update["updated"])
	}
	if _, ok := update["image"]; ok {
		t.Errorf("Update image = %v, want none for a revision without a cover", update["image"])
	}

	e.delete(t, s)
	e.drain(t)
	del := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityDelete)
	if got := e.inboxes(t, del.ID); !slices.Equal(got, created) {
		t.Errorf("Delete inboxes = %v, want those of the Create %v", got, created)
	}
	if obj, _ := e.document(t, del.ID)["object"].(map[string]any); obj["type"] != "Tombstone" || obj["id"] != s.URI {
		t.Errorf("Delete object = %v, want the Tombstone of the article", obj)
	}
}
