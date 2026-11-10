// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// Articles are long posts written in Markdown (ADR-0160): a status with a title, a summary, and a
// cover, whose body is rendered once, when it is written, through the ArticleRenderer. They take
// the same paths as posts for their visibility, edits, interactions, reports, and deletion.

var (
	// ErrArticlesNotPermitted reports a new article by an account that may not write one: the
	// instance setting article_authoring is permission and the account lacks create_articles.
	ErrArticlesNotPermitted = errors.New("content: the account may not write articles")
	// ErrNotArticle reports an edit as an article of a status that is a post.
	ErrNotArticle = errors.New("content: the status is not an article")
	// ErrArticleEditedAsPost reports an edit as a post of a status that is an article.
	ErrArticleEditedAsPost = errors.New("content: the status is an article")
	// ErrArticleDirect reports an article with the visibility direct, which articles never have.
	ErrArticleDirect = errors.New("content: an article cannot be direct")
	// ErrInvalidArticleTitle reports a title that is empty, longer than content.MaxArticleTitle,
	// or not valid text.
	ErrInvalidArticleTitle = errors.New("content: the title of the article is not valid")
	// ErrInvalidArticleSummary reports a summary longer than content.MaxArticleSummary or not
	// valid text.
	ErrInvalidArticleSummary = errors.New("content: the summary of the article is not valid")
	// ErrArticleTooLong reports a body whose HTML is longer than max_article_bytes.
	ErrArticleTooLong = errors.New("content: the body of the article is too long")
	// ErrTooManyArticleImages reports a body with more images than max_article_images.
	ErrTooManyArticleImages = errors.New("content: the body of the article has too many images")
	// ErrTooManyHashtags reports more hashtags of the editor than content.MaxArticleTags.
	ErrTooManyHashtags = errors.New("content: the article has too many hashtags")
)

// ArticleTooLongError is ErrArticleTooLong with the size of the body's HTML and the limit.
type ArticleTooLongError struct {
	Bytes, Max int
}

func (e *ArticleTooLongError) Error() string {
	return fmt.Sprintf("%v: %d bytes, at most %d", ErrArticleTooLong, e.Bytes, e.Max)
}

// Unwrap makes errors.Is(err, ErrArticleTooLong) hold.
func (e *ArticleTooLongError) Unwrap() error { return ErrArticleTooLong }

// TooManyArticleImagesError is ErrTooManyArticleImages with the count and the limit.
type TooManyArticleImagesError struct {
	Images, Max int
}

func (e *TooManyArticleImagesError) Error() string {
	return fmt.Sprintf("%v: %d images, at most %d", ErrTooManyArticleImages, e.Images, e.Max)
}

// Unwrap makes errors.Is(err, ErrTooManyArticleImages) hold.
func (e *TooManyArticleImagesError) Unwrap() error { return ErrTooManyArticleImages }

// ArticleMediaScheme starts the destination of a Markdown image that names an upload of the
// author by its ID: ![alt text](media:ID). Only those images, of uploads that finished processing,
// become images; every other image becomes a link (ADR-0160 §4).
const ArticleMediaScheme = "media:"

// ArticleRenderer renders the Markdown of an article (ADR-0160 §4): CommonMark with the tables,
// strikethrough, and autolinks of GitHub Flavored Markdown, no raw HTML, headings one level down,
// and the article allowlist. internal/adapter/markdown implements it; it does no I/O.
type ArticleRenderer interface {
	// ArticleReferences returns what source refers to that the renderer needs resolved: the
	// destinations of its images and the mentions in its text, outside code.
	ArticleReferences(source string) ArticleReferences
	// RenderArticle renders source with what ArticleReferences named resolved.
	RenderArticle(source string, r ArticleResolution) ArticleRendering
}

// ArticleReferences are the images and mentions of an article's Markdown.
type ArticleReferences struct {
	// Images are the destinations of the images, each once, in order.
	Images []string
	// Mentions are the handles mentioned in the text, each once, in order.
	Mentions []MentionHandle
}

// ArticleResolution is what the references of an article resolve to.
type ArticleResolution struct {
	// Images are the images to show, by destination; every other image becomes a link to its
	// destination with its alt text as the text.
	Images map[string]ArticleImageSource
	// Mentions are the resolved mentions; one without a target stays text.
	Mentions map[MentionHandle]MentionTarget
}

// ArticleImageSource is an upload that the body of an article shows: the large still variant.
type ArticleImageSource struct {
	MediaID       string
	URL           string
	Width, Height int
}

// ArticleRendering is the rendered body of an article.
type ArticleRendering struct {
	// HTML is the body, sanitized with the article allowlist.
	HTML string
	// Images are the uploads that HTML shows, each once, in order of their first use, with the alt
	// text of that use.
	Images []content.ArticleImage
	// Text is the text of the body without its headings, code, and the alt text of images, from
	// which a summary is made.
	Text string
}

// ArticleLimits are the instance settings that bound an article (ADR-0160 §5).
type ArticleLimits struct {
	// MaxBytes is max_article_bytes, MaxImages max_article_images.
	MaxBytes, MaxImages int
}

// defaultArticleLimits are the defaults of the settings, used without Deps.ArticleLimits.
var defaultArticleLimits = ArticleLimits{MaxBytes: 100000, MaxImages: 20}

// ArticleInput is the content of an article or of an edit of one.
type ArticleInput struct {
	// Title is plain text: 1 to content.MaxArticleTitle characters, surrounding white space
	// dropped.
	Title string
	// Summary is plain text of up to content.MaxArticleSummary characters; empty makes one from
	// the body (content.ArticleSummaryFromBody).
	Summary string
	// Cover is the cover image, an upload of the author that finished processing; nil for none.
	Cover *MediaInput
	// Text is the body in Markdown. It must not be blank.
	Text string
	// Tags are the hashtags the editor gives the article, without "#": at most
	// content.MaxArticleTags. The #name in the body are text.
	Tags []string
	// Language is a BCP 47 tag; "" means the author's default post language.
	Language string
	// Sensitive folds the body and the cover behind the title and the summary.
	Sensitive bool
}

// CreateArticleInput describes a new local article.
type CreateArticleInput struct {
	ArticleInput
	AuthorAccountID string
	// Visibility is any but direct; the author's visibility ceiling may narrow it.
	Visibility content.Visibility
	// ClientID is the OAuth client that posts, "" when none is recorded.
	ClientID string
	// IdempotencyKey is the client API's Idempotency-Key, as CreateInput's.
	IdempotencyKey string
	// InteractionPolicy is who may reply to, boost, and quote the article.
	InteractionPolicy InteractionPolicyChange
}

// EditArticleInput describes an edit of a local article.
type EditArticleInput struct {
	ArticleInput
	// AccountID is the account editing; it must be the author.
	AccountID string
	StatusID  string
}

// ArticlePreview is the body of an article as publishing it would store it, without storing it
// (POST /articles/preview).
type ArticlePreview struct {
	HTML string
	// Summary is the summary that publishing stores: the one given, or one made from the body.
	Summary       string
	SummaryOrigin content.SummaryOrigin
	// Bytes is the size of HTML and Images the number of images, with their limits.
	Bytes, MaxBytes   int
	Images, MaxImages int
}

// CreateArticle posts a local article (ADR-0160). It checks what Create checks of a post, the
// author's permission to write articles (ErrArticlesNotPermitted), the title, summary, and
// hashtags, and the limits of the body (an *ArticleTooLongError, a *TooManyArticleImagesError);
// an article is never direct (ErrArticleDirect). The body is rendered once, here, and stored with
// its Markdown. Its mentions are links that notify nobody, and its hashtags are those of Tags.
// An Idempotency-Key works as Create's.
func (s *Service) CreateArticle(ctx context.Context, in CreateArticleInput) (Post, error) {
	if in.Visibility.Valid() && !content.ArticleVisibility(in.Visibility) {
		return Post{}, ErrArticleDirect
	}
	if err := s.checkNewVisibility(ctx, in.Visibility); err != nil {
		return Post{}, err
	}
	if err := content.ValidateIdempotencyKey(in.IdempotencyKey); err != nil {
		return Post{}, err
	}
	author, err := s.localActive(ctx, in.AuthorAccountID)
	if err != nil {
		return Post{}, err
	}
	if in.IdempotencyKey != "" {
		if post, err := s.replay(ctx, in.AuthorAccountID, in.IdempotencyKey); !errors.Is(err, ErrNotFound) {
			return post, err
		}
	}
	restrictions, err := s.postingRestrictions(ctx, in.AuthorAccountID)
	if err != nil {
		return Post{}, err
	}
	if err := s.mayWriteArticles(ctx, author); err != nil {
		return Post{}, err
	}
	body, err := s.prepareArticle(ctx, author, in.ArticleInput)
	if err != nil {
		return Post{}, err
	}
	interaction, err := s.newInteractionSettings(ctx, author, in.InteractionPolicy)
	if err != nil {
		return Post{}, err
	}
	id := s.d.IDs.NewID()
	now := s.d.Clock.Now()
	st := content.Status{
		ID: id, URI: content.StatusURI(s.d.Host, id), AuthorAccountID: in.AuthorAccountID, IsLocal: true,
		Visibility: content.EffectiveVisibility(in.Visibility, restrictions.Author), PublishedAt: now,
	}
	rev := body.revision(s.d.IDs.NewID(), id, 1, now)
	verdicts, err := s.attachmentVerdicts(ctx, in.AuthorAccountID, reviewedMedia(rev), nil)
	if err != nil {
		return Post{}, err
	}
	p := PreparedCreate{
		st: st, local: content.LocalStatus{StatusID: id, AuthorAccountID: in.AuthorAccountID, ClientID: in.ClientID, IdempotencyKey: in.IdempotencyKey},
		rev: rev, tags: body.tags(s.d.IDs, now), interaction: interaction, media: verdicts, now: now,
	}
	return s.commitCreate(ctx, author, p, in.IdempotencyKey, nil)
}

// EditArticle records a new revision of a local article (FR-109), checked as CreateArticle checks
// a new one; the visibility never changes, and the permission to write articles is not needed.
// A status that is a post fails with ErrNotArticle, and the other failures are Edit's.
func (s *Service) EditArticle(ctx context.Context, in EditArticleInput) (Post, error) {
	author, err := s.localActive(ctx, in.AccountID)
	if err != nil {
		return Post{}, err
	}
	st, err := s.ownStatus(ctx, in.AccountID, in.StatusID)
	if err != nil {
		return Post{}, err
	}
	current, err := s.d.Repository.FindCurrentRevision(ctx, st.ID)
	if err != nil {
		return Post{}, err
	}
	if current.Article == nil {
		return Post{}, fmt.Errorf("status %s: %w", st.ID, ErrNotArticle)
	}
	if _, err := s.postingRestrictions(ctx, in.AccountID); err != nil {
		return Post{}, err
	}
	body, err := s.prepareArticle(ctx, author, in.ArticleInput)
	if err != nil {
		return Post{}, err
	}
	return s.commitEdit(ctx, author, st, current, body, false)
}

// PreviewArticle renders the body of an article of the local account accountID as publishing it
// would, without storing anything, with the summary that publishing would store for summary, and
// the size and images of the body with their limits. A body over the limits still renders.
func (s *Service) PreviewArticle(ctx context.Context, accountID, text, summary string) (ArticlePreview, error) {
	author, err := s.localActive(ctx, accountID)
	if err != nil {
		return ArticlePreview{}, err
	}
	if !content.ValidText(text) {
		return ArticlePreview{}, ErrInvalidText
	}
	summary, err = validSummary(summary)
	if err != nil {
		return ArticlePreview{}, err
	}
	limits, err := s.articleLimits(ctx)
	if err != nil {
		return ArticlePreview{}, err
	}
	r, err := s.renderArticle(ctx, author, text)
	if err != nil {
		return ArticlePreview{}, err
	}
	out := ArticlePreview{
		HTML: r.HTML, Summary: summary, SummaryOrigin: content.SummaryByAuthor,
		Bytes: len(r.HTML), MaxBytes: limits.MaxBytes, Images: len(r.Images), MaxImages: limits.MaxImages,
	}
	if summary == "" {
		out.Summary, out.SummaryOrigin = content.ArticleSummaryFromBody(r.Text), content.SummaryFromBody
	}
	return out, nil
}

// mayWriteArticles fails with ErrArticlesNotPermitted unless author may write new articles.
func (s *Service) mayWriteArticles(ctx context.Context, author appaccount.Actor) error {
	if s.d.MayWriteArticles == nil {
		return nil
	}
	ok, err := s.d.MayWriteArticles(ctx, author.Local.UserID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("account %s: %w", author.Account.ID, ErrArticlesNotPermitted)
	}
	return nil
}

// articleLimits returns the current limits of an article.
func (s *Service) articleLimits(ctx context.Context) (ArticleLimits, error) {
	if s.d.ArticleLimits == nil {
		return defaultArticleLimits, nil
	}
	return s.d.ArticleLimits(ctx)
}

// prepareArticle validates an article against its limits and the instance settings, resolves the
// images and mentions of its body, and renders it. It runs before the transaction, since
// resolving a remote handle may fetch it.
func (s *Service) prepareArticle(ctx context.Context, author appaccount.Actor, in ArticleInput) (preparedText, error) {
	title := strings.TrimSpace(in.Title)
	if !content.ValidText(title) || title == "" || utf8.RuneCountInString(title) > content.MaxArticleTitle {
		return preparedText{}, ErrInvalidArticleTitle
	}
	summary, err := validSummary(in.Summary)
	if err != nil {
		return preparedText{}, err
	}
	if !content.ValidText(in.Text) {
		return preparedText{}, ErrInvalidText
	}
	if strings.TrimSpace(in.Text) == "" {
		return preparedText{}, ErrEmptyText
	}
	hashtags, err := articleHashtags(in.Tags)
	if err != nil {
		return preparedText{}, err
	}
	language, err := s.language(ctx, author, in.Language)
	if err != nil {
		return preparedText{}, err
	}
	requireDescription, err := s.requireDescription(ctx)
	if err != nil {
		return preparedText{}, err
	}
	article := &content.ArticleRevision{Title: title, Summary: summary, SummaryOrigin: content.SummaryByAuthor}
	if c := in.Cover; c != nil {
		if err := checkDescription(c.MediaID, c.Description, requireDescription); err != nil {
			return preparedText{}, err
		}
		if err := s.checkMedia(ctx, author.Account.ID, c.MediaID, false); err != nil {
			return preparedText{}, err
		}
		article.Cover = &content.ArticleCover{MediaID: c.MediaID, Description: c.Description}
	}
	limits, err := s.articleLimits(ctx)
	if err != nil {
		return preparedText{}, err
	}
	r, err := s.renderArticle(ctx, author, in.Text)
	if err != nil {
		return preparedText{}, err
	}
	if n := len(r.HTML); n > limits.MaxBytes {
		return preparedText{}, &ArticleTooLongError{Bytes: n, Max: limits.MaxBytes}
	}
	if n := len(r.Images); n > limits.MaxImages {
		return preparedText{}, &TooManyArticleImagesError{Images: n, Max: limits.MaxImages}
	}
	for _, img := range r.Images {
		if err := checkDescription(img.MediaID, img.Description, requireDescription); err != nil {
			return preparedText{}, err
		}
	}
	article.Images = r.Images
	if summary == "" {
		article.Summary, article.SummaryOrigin = content.ArticleSummaryFromBody(r.Text), content.SummaryFromBody
	}
	var emojis []string
	if s.d.LocalEmojis != nil {
		if emojis, err = s.d.LocalEmojis(ctx, title, article.Summary, in.Text); err != nil {
			return preparedText{}, err
		}
	}
	return preparedText{
		in:       TextInput{Text: in.Text, Sensitive: in.Sensitive, Language: in.Language},
		language: language,
		html:     r.HTML,
		hashtags: hashtags,
		emojis:   emojis,
		format:   content.SourceTextMarkdown,
		article:  article,
	}, nil
}

// renderArticle resolves the images and mentions of the Markdown source of author's article and
// renders it. The images are the author's own uploads that finished processing, named by
// ArticleMediaScheme; the mentions resolve as those of a post do but are not recorded (ADR-0160
// §7). It may do network I/O, so it never runs inside a transaction.
func (s *Service) renderArticle(ctx context.Context, author appaccount.Actor, source string) (ArticleRendering, error) {
	if s.d.ArticleRenderer == nil {
		return ArticleRendering{}, errors.New("content: no article renderer")
	}
	refs := s.d.ArticleRenderer.ArticleReferences(source)
	res := ArticleResolution{Images: map[string]ArticleImageSource{}}
	for _, dest := range refs.Images {
		id, ok := strings.CutPrefix(dest, ArticleMediaScheme)
		if !ok || id == "" || s.d.ArticleImage == nil {
			continue
		}
		img, ok, err := s.d.ArticleImage(ctx, author.Account.ID, id)
		if err != nil {
			return ArticleRendering{}, err
		}
		if ok {
			res.Images[dest] = img
		}
	}
	var err error
	if res.Mentions, _, err = s.resolveMentions(ctx, refs.Mentions); err != nil {
		return ArticleRendering{}, err
	}
	return s.d.ArticleRenderer.RenderArticle(source, res), nil
}

// validSummary returns the summary of an article without its surrounding white space, failing
// with ErrInvalidArticleSummary when it is not valid text or too long.
func validSummary(summary string) (string, error) {
	summary = strings.TrimSpace(summary)
	if !content.ValidText(summary) || utf8.RuneCountInString(summary) > content.MaxArticleSummary {
		return "", ErrInvalidArticleSummary
	}
	return summary, nil
}

// articleHashtags returns the hashtags of names, each a hashtag name with or without "#"
// (ParseHashtag, ErrInvalidHashtag), each once by its normalized name: at most
// content.MaxArticleTags (ErrTooManyHashtags).
func articleHashtags(names []string) ([]Hashtag, error) {
	if len(names) > content.MaxArticleTags {
		return nil, fmt.Errorf("%w: %d hashtags, at most %d", ErrTooManyHashtags, len(names), content.MaxArticleTags)
	}
	var out []Hashtag
	for _, name := range names {
		h, ok := ParseHashtag(strings.TrimSpace(name))
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidHashtag, name)
		}
		if !containsTag(out, h.NameNormalized) {
			out = append(out, h)
		}
	}
	return out, nil
}

func containsTag(tags []Hashtag, normalized string) bool {
	for _, t := range tags {
		if t.NameNormalized == normalized {
			return true
		}
	}
	return false
}

// requireDescription returns the instance setting require_media_description.
func (s *Service) requireDescription(ctx context.Context) (bool, error) {
	if s.d.RequireMediaDescription == nil {
		return false, nil
	}
	return s.d.RequireMediaDescription(ctx)
}

// checkDescription checks the alt text of a media file as attachments checks it.
func checkDescription(mediaID, d string, required bool) error {
	if !content.ValidText(d) || utf8.RuneCountInString(d) > MaxMediaDescription {
		return fmt.Errorf("media %s: %w", mediaID, ErrInvalidMediaDescription)
	}
	if required && strings.TrimSpace(d) == "" {
		return fmt.Errorf("media %s: %w", mediaID, ErrMissingMediaDescription)
	}
	return nil
}

// reviewedMedia returns the media files of rev that the decision hook media.check acts on: its
// attachments, and the cover and body images of an article.
func reviewedMedia(rev content.Revision) []content.Attachment {
	a := rev.Article
	if a == nil {
		return rev.Attachments
	}
	out := append([]content.Attachment(nil), rev.Attachments...)
	if a.Cover != nil {
		out = append(out, content.Attachment{MediaID: a.Cover.MediaID, Description: a.Cover.Description})
	}
	for _, img := range a.Images {
		out = append(out, content.Attachment{MediaID: img.MediaID, Description: img.Description})
	}
	return out
}
