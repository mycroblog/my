// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Articles are long posts written in Markdown (ADR-0160): a status that is an article has an
// Article row, and each of its revisions an ArticleRevision. Whether a status is an article is
// decided when it is created and never changes.

// Article is the article subtype of a status (status_articles).
type Article struct {
	StatusID        string
	AuthorAccountID string
}

// SummaryOrigin says where the summary of an article's revision comes from.
type SummaryOrigin string

// Summary origins (status_revision_articles.summary_origin).
const (
	// SummaryByAuthor is a summary the author wrote.
	SummaryByAuthor SummaryOrigin = "author"
	// SummaryFromBody is a summary made from the body, because the author left it empty
	// (ArticleSummaryFromBody).
	SummaryFromBody SummaryOrigin = "body"
)

// ArticleRevision is the article child of a revision: its title, its summary, its cover image,
// and the images of the author's own uploads that its body shows.
type ArticleRevision struct {
	Title         string
	Summary       string
	SummaryOrigin SummaryOrigin
	// Cover is the cover image, nil for none.
	Cover *ArticleCover
	// Images are the media files the body shows as images, in order of their first use, each
	// once; Position runs from 0.
	Images []ArticleImage
}

// ArticleCover is the cover image of an article's revision, with its alt text.
type ArticleCover struct {
	MediaID     string
	Description string
}

// ArticleImage is a media file that the body of an article's revision shows, with the alt text
// of its first use.
type ArticleImage struct {
	Position    int
	MediaID     string
	Description string
}

// Limits of an article (ADR-0160 §5).
const (
	// MaxArticleTitle is the most characters (code points) of a title; a title is never empty.
	MaxArticleTitle = 200
	// MaxArticleSummary is the most characters of a summary.
	MaxArticleSummary = 500
	// MaxArticleTags is the most hashtags the editor gives an article.
	MaxArticleTags = 10
	// ArticleSummaryLength is the most characters that a summary made from the body keeps
	// before its ellipsis.
	ArticleSummaryLength = 200
)

// ArticleVisibility reports whether an article may have visibility v: any but direct.
func ArticleVisibility(v Visibility) bool { return v.Valid() && v != VisibilityDirect }

// sentenceEnds are the characters that end a sentence, where a summary made from the body is cut
// first.
const sentenceEnds = ".!?。．！？"

// ArticleSummaryFromBody returns the summary of an article whose author left it empty, made from
// text, the text of its rendered body (without headings, code, and the alt text of images): its
// white space collapsed, and past ArticleSummaryLength characters cut at the last end of a
// sentence or word in them, with "…" after it (ADR-0160 §6). A cut that would keep less than
// half of the length cuts at the length instead.
func ArticleSummaryFromBody(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= ArticleSummaryLength {
		return text
	}
	runes := []rune(text)
	cut := 0
	for i := ArticleSummaryLength - 1; i >= ArticleSummaryLength/2 && cut == 0; i-- {
		if strings.ContainsRune(sentenceEnds, runes[i]) {
			cut = i + 1
		}
	}
	for i := ArticleSummaryLength; i >= ArticleSummaryLength/2 && cut == 0; i-- {
		if unicode.IsSpace(runes[i]) {
			cut = i
		}
	}
	if cut == 0 {
		cut = ArticleSummaryLength
	}
	return strings.TrimRightFunc(string(runes[:cut]), unicode.IsSpace) + "…"
}
