// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"strings"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// A received Article is stored as an article (ADR-0160 §10): its name is the title, its summary
// the summary (never a content warning), its first image the cover, and its body is sanitized
// with the allowlist of articles. The images of the body may be of any host: the first
// max_article_images of them are recorded as remote media files of the author, and the client
// API shows them through the media proxy (Reader.ArticleBody); any other image becomes a link.

// RemoteArticleSanitizer cleans the body of a received Article with the allowlist of articles
// (ADR-0160 §4, §10). internal/adapter/htmlsanitizer implements it; it does no I/O.
type RemoteArticleSanitizer interface {
	// SanitizeRemoteArticle returns html, the body as received, reduced to the allowlist of
	// articles. The images of the first maxImages distinct http and https sources stay images
	// with those sources; every other image becomes a link to its source with its alt text as the
	// text, or its alt text alone.
	SanitizeRemoteArticle(html string, maxImages int) SanitizedArticle
	// ShowArticleImages returns html, a body that SanitizeRemoteArticle returned, with the source
	// of each image replaced by src[source]; an image whose source src lacks becomes a link as
	// SanitizeRemoteArticle makes it.
	ShowArticleImages(html string, src map[string]string) string
}

// SanitizedArticle is the body of a received Article, sanitized.
type SanitizedArticle struct {
	HTML string
	// Images are the sources of the images that HTML keeps, each once, in order of their first
	// use, with the alt text of that use.
	Images []RemoteArticleImage
	// Text is the text of the body without its headings, code, and images, from which a summary
	// is made.
	Text string
}

// RemoteArticleImage is an image that the body of a received Article keeps.
type RemoteArticleImage struct {
	URL string
	Alt string // as received
}

// RemoteArticleInput is what a received Article has besides a post: its title and summary as
// received (plain text), and its cover, nil for none.
type RemoteArticleInput struct {
	Title   string
	Summary string
	Cover   *RemoteAttachmentInput
}

// remoteArticle builds the article child of rev, a revision of the received Article in, and
// replaces the bodies of rev with in's sanitized with the allowlist of articles. The cover and
// the images of the bodies are recorded in tx as remote media files of the author.
func (s *Service) remoteArticle(ctx context.Context, tx uow.Tx, in RemoteStatusInput, rev *content.Revision) error {
	switch {
	case s.d.RemoteArticles == nil:
		return errors.New("content: remote articles need an article sanitizer")
	case s.d.RemoteMedia == nil:
		return errors.New("content: remote articles need a remote media recorder")
	}
	limits := defaultArticleLimits
	if s.d.ArticleLimitsTx != nil {
		var err error
		if limits, err = s.d.ArticleLimitsTx(ctx, tx); err != nil {
			return err
		}
	}
	plain := s.d.Sanitizer.PlainText
	a := &content.ArticleRevision{
		Title:         truncateRunes(strings.TrimSpace(plain(in.Article.Title)), content.MaxArticleTitle),
		Summary:       truncateRunes(strings.TrimSpace(plain(in.Article.Summary)), content.MaxArticleSummary),
		SummaryOrigin: content.SummaryByAuthor,
	}
	var text string
	for i := range rev.Bodies {
		b := &rev.Bodies[i]
		if strings.TrimSpace(b.ContentHTML) == "" {
			continue
		}
		out := s.d.RemoteArticles.SanitizeRemoteArticle(b.ContentHTML, limits.MaxImages)
		b.ContentHTML = out.HTML
		if b.Language == rev.Language {
			text = out.Text
		}
		for _, img := range out.Images {
			if err := s.remoteArticleImage(ctx, tx, in.AuthorAccountID, img, a, limits.MaxImages); err != nil {
				return err
			}
		}
	}
	if a.Summary == "" {
		a.Summary, a.SummaryOrigin = content.ArticleSummaryFromBody(text), content.SummaryFromBody
	}
	if c := in.Article.Cover; c != nil && c.URL != "" {
		description := truncateRunes(strings.TrimSpace(plain(c.Description)), MaxMediaDescription)
		m, err := s.d.RemoteMedia.RecordRemoteMedia(ctx, tx, RemoteMediaInput{
			OwnerAccountID: in.AuthorAccountID, RemoteURL: c.URL, DeclaredContentType: c.ContentType,
			DeclaredMediaType: content.MediaTypeImage, Purpose: content.PurposeAttachment,
			Width: max(c.Width, 0), Height: max(c.Height, 0), Blurhash: c.Blurhash, Description: description,
		})
		if err != nil {
			return err
		}
		a.Cover = &content.ArticleCover{MediaID: m.MediaID, Description: description}
	}
	rev.Article = a
	return nil
}

// remoteArticleImage records img, an image of the body of a received Article of the account
// owner, as a remote media file, and adds it to the images of a unless a has it already (the
// bodies in other languages show the same images) or has maxImages of them. An image that a
// lacks is shown as a link (Reader.ArticleBody).
func (s *Service) remoteArticleImage(ctx context.Context, tx uow.Tx, owner string, img RemoteArticleImage, a *content.ArticleRevision, maxImages int) error {
	if len(a.Images) >= maxImages {
		return nil
	}
	description := truncateRunes(strings.TrimSpace(s.d.Sanitizer.PlainText(img.Alt)), MaxMediaDescription)
	m, err := s.d.RemoteMedia.RecordRemoteMedia(ctx, tx, RemoteMediaInput{
		OwnerAccountID: owner, RemoteURL: img.URL, DeclaredMediaType: content.MediaTypeImage,
		Purpose: content.PurposeAttachment, Description: description,
	})
	if err != nil {
		return err
	}
	for _, have := range a.Images {
		if have.MediaID == m.MediaID {
			return nil
		}
	}
	a.Images = append(a.Images, content.ArticleImage{Position: len(a.Images), MediaID: m.MediaID, Description: description})
	return nil
}

// ArticleBody returns the body of rev, a revision of an article, as clients show it: the HTML of
// its first body, where the images of a received article are shown through the media proxy
// (ADR-0160 §10), since the body keeps their sources as received. An image whose media file
// cannot be shown becomes a link to its source. A revision of a post, or one without a body, has
// none. The body of a received article with images needs RemoteArticles: the sources as received
// would make readers' browsers load them from their hosts.
func (r *Reader) ArticleBody(ctx context.Context, rev content.Revision) (string, error) {
	if rev.Article == nil || len(rev.Bodies) == 0 {
		return "", nil
	}
	html := rev.Bodies[0].ContentHTML
	if rev.Remote == nil || !strings.Contains(html, "<img") {
		return html, nil
	}
	if r.d.RemoteArticles == nil {
		return "", errors.New("content: remote articles need an article sanitizer")
	}
	images := make([]content.Attachment, 0, len(rev.Article.Images))
	for _, img := range rev.Article.Images {
		images = append(images, content.Attachment{MediaID: img.MediaID, Description: img.Description})
	}
	views, err := r.attachments(ctx, images)
	if err != nil {
		return "", err
	}
	src := make(map[string]string, len(views))
	for _, v := range views {
		if v.Media.Remote != nil && v.Image.HasStill() {
			src[v.Media.Remote.RemoteURL] = v.Image.Still.Large.URL
		}
	}
	return r.d.RemoteArticles.ShowArticleImages(html, src), nil
}

// sameKind reports whether a revision of the received post in may follow current, the current
// revision of its status: whether a status is an article never changes (ADR-0160 §1). It fails
// with ErrNotArticle for an Article whose status is a post, and with ErrArticleEditedAsPost for
// a post whose status is an article.
func sameKind(current content.Revision, in RemoteStatusInput) error {
	switch {
	case current.Article == nil && in.Article != nil:
		return ErrNotArticle
	case current.Article != nil && in.Article == nil:
		return ErrArticleEditedAsPost
	}
	return nil
}
