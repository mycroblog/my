// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/federation"
)

// asArticle returns the Note of a local article as its Article (ADR-0160 §9, federation.md
// §7.4): the title, the stored summary (made from the body when the author left it empty, so an
// Article always has one unless its body has no text), and the cover as image; the Markdown is not
// sent (federation.AsArticle). note was built by statusNote from a revision that has no mentions,
// content warning, attachments, or poll, so its tag holds only the editor's hashtags and the
// custom emoji. A revision of a post returns note as it is.
func asArticle(note federation.NoteDocument, rev appcontent.Revision, cover *appcontent.MediaView) federation.NoteDocument {
	a := rev.Article
	if a == nil {
		return note
	}
	parts := federation.ArticleParts{Title: a.Title, Summary: a.Summary}
	if cover != nil {
		parts.Cover = coverOf(*cover)
	}
	return note.AsArticle(parts)
}

// coverOf returns the image document of the cover of an article: the URL and type of its large
// variant, moving for an animation as attachments are (ADR-0005 §8), with its alternative text
// as name; nil when the file has no image.
func coverOf(m appcontent.MediaView) *federation.ImageDocument {
	large := m.Image.Primary()
	if large.URL == "" {
		return nil
	}
	return &federation.ImageDocument{Type: federation.ImageType, MediaType: large.ContentType, URL: large.URL, Name: m.Description}
}
