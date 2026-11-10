// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mycroblog/my/internal/domain/federation"
)

func TestAsArticle(t *testing.T) {
	note := federation.NoteDocument{
		ObjectDocument: federation.ObjectDocument{ID: "https://social.example/ap/objects/1", Type: federation.ObjectNote},
		Content:        "<p>body</p>",
		ContentMap:     map[string]string{"en": "<p>body</p>"},
		Attachment:     []federation.AttachmentDocument{},
		Tag:            []federation.TagDocument{},
	}
	cover := &federation.ImageDocument{Type: federation.ImageType, MediaType: "image/png", URL: "https://files.social.example/c.png", Name: "a cover"}
	got := marshal(t, note.AsArticle(federation.ArticleParts{Title: "Title", Summary: "Summary", Cover: cover}))
	// A body in one language is sent once: its language is in nameMap and summaryMap.
	want := map[string]any{
		"id": "https://social.example/ap/objects/1", "type": "Article", "name": "Title", "summary": "Summary",
		"nameMap": map[string]any{"en": "Title"}, "summaryMap": map[string]any{"en": "Summary"},
		"sensitive": false, "content": "<p>body</p>", "attachment": []any{}, "tag": []any{},
		"image": map[string]any{"type": "Image", "mediaType": "image/png", "url": "https://files.social.example/c.png", "name": "a cover"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Article = %v, want %v", got, want)
	}

	// A body in several languages keeps contentMap and has no nameMap or summaryMap.
	multi := note
	multi.ContentMap = map[string]string{"en": "<p>body</p>", "ja": "<p>本文</p>"}
	got = marshal(t, multi.AsArticle(federation.ArticleParts{Title: "Title", Summary: "Summary"}))
	if !reflect.DeepEqual(got["contentMap"], map[string]any{"en": "<p>body</p>", "ja": "<p>本文</p>"}) || got["nameMap"] != nil || got["summaryMap"] != nil {
		t.Errorf("Article in two languages = %v", got)
	}

	// Without a summary or a cover, they are left out; a Note never has a name or an image, and
	// neither has a source (the Markdown is never sent).
	got = marshal(t, note.AsArticle(federation.ArticleParts{Title: "Title"}))
	for _, k := range []string{"source", "image", "summary", "summaryMap", "contentMap"} {
		if _, ok := got[k]; ok {
			t.Errorf("Article without %s has %v", k, got[k])
		}
	}
	got = marshal(t, note)
	for _, k := range []string{"name", "nameMap", "summaryMap", "source", "image"} {
		if _, ok := got[k]; ok {
			t.Errorf("Note has %s %v", k, got[k])
		}
	}
}

func marshal(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
