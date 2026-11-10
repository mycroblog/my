// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// newArticleWorld returns a noteWorld whose status is an article of alice (ADR-0160 §9): a
// title, a summary, a body rendered from Markdown (which the Article does not carry), a cover,
// and a hashtag from the editor; no content warning, mention, or attachment.
func newArticleWorld() *noteWorld {
	w := newNoteWorld()
	body := `<h2>Morning</h2>` + "\n" + `<p>We climbed <strong>early</strong>, past <a href="https://example.org/ridge" rel="nofollow noopener noreferrer" target="_blank">the ridge</a>.</p>`
	w.rev.SpoilerText, w.rev.MentionedAccountIDs = "", nil
	w.rev.Bodies = []content.Body{{Language: "en", ContentHTML: body}}
	w.rev.Source = &content.Source{Text: "## Morning\n\nWe climbed **early**, past [the ridge](https://example.org/ridge).", Format: content.SourceTextMarkdown}
	w.rev.Article = &content.ArticleRevision{
		Title: "A climb", Summary: "How we climbed the mountain before the sun rose.", SummaryOrigin: content.SummaryByAuthor,
		Cover: &content.ArticleCover{MediaID: testkit.NamedID("cover"), Description: "A mountain at dawn"},
	}
	cover := w.parts.Attachments[0]
	cover.Description = "A mountain at dawn"
	w.parts.Cover, w.parts.Attachments, w.parts.Mentions = &cover, nil, nil
	return w
}

// TestArticleDocuments checks the Article of a local article, as its IRI and the Create's IRI
// serve it with the builder that the fan-out delivers too (federation.md §7.4), and that the
// outbox carries the same Article.
func TestArticleDocuments(t *testing.T) {
	s := newNoteServer(t, newArticleWorld(), nil)

	rec := s.get(t, "/ap/objects/"+noteStatusID, acceptMastodon)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the Article = %d %s", rec.Code, rec.Body)
	}
	checkGolden(t, "object-article.json", rec.Body.Bytes())
	own := decode(t, rec.Body.Bytes())
	delete(own, "@context")

	rec = s.get(t, "/ap/objects/"+noteStatusID+"/activity", acceptMastodon)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the Create = %d %s", rec.Code, rec.Body)
	}
	checkGolden(t, "activity-create-article.json", rec.Body.Bytes())
	if obj := decode(t, rec.Body.Bytes())["object"]; !reflect.DeepEqual(obj, own) {
		t.Errorf("Create object = %v, want %v", obj, own)
	}

	rec = s.get(t, "/ap/actors/"+aliceID+"/outbox?page=true", acceptMastodon)
	items, _ := decode(t, rec.Body.Bytes())["orderedItems"].([]any)
	if len(items) != 1 || !reflect.DeepEqual(items[0].(map[string]any)["object"], own) {
		t.Errorf("outbox items = %v, want the Create of %v", items, own)
	}
}
