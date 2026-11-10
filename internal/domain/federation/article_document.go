// SPDX-License-Identifier: AGPL-3.0-only

package federation

// ArticleParts are what the Article of an article has besides the shared part of a Note: its
// title, its summary (the one its author wrote or the one made from its body, never a content
// warning), and its cover, nil for none.
type ArticleParts struct {
	Title   string
	Summary string
	Cover   *ImageDocument
}

// AsArticle returns n as the Article of an article with the parts a (ADR-0160 §9): the title as
// name, the summary as summary, and the cover as image. The rest of n (content, url, sensitive,
// tag, attachment, the addressing) stays as the Note has it.
//
// The body reaches the receiver once where it can: the Markdown it was written in is not sent as
// source, and a body in one language is sent as content alone, without contentMap, its language
// given by nameMap and summaryMap instead (Mastodon reads the language from contentMap, then
// nameMap, then summaryMap). A body in several languages keeps contentMap. Misskey refuses an
// inbox POST over 64 KiB, and every copy of the body counts.
func (n NoteDocument) AsArticle(a ArticleParts) NoteDocument {
	n.Type = ObjectArticle
	n.Name, n.Summary = a.Title, a.Summary
	n.NameMap, n.SummaryMap = nil, nil
	if len(n.ContentMap) == 1 {
		for lang := range n.ContentMap {
			n.NameMap = map[string]string{lang: a.Title}
			if a.Summary != "" {
				n.SummaryMap = map[string]string{lang: a.Summary}
			}
		}
		n.ContentMap = nil
	}
	n.Image = a.Cover
	return n
}
