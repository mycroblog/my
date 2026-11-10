// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Articles writes the viewer's articles, the long posts in Markdown (ADR-0160 §8), and previews
// their bodies. *appcontent.Service implements it.
type Articles interface {
	CreateArticle(ctx context.Context, in appcontent.CreateArticleInput) (appcontent.Post, error)
	EditArticle(ctx context.Context, in appcontent.EditArticleInput) (appcontent.Post, error)
	PreviewArticle(ctx context.Context, accountID, text, summary string) (appcontent.ArticlePreview, error)
}

// articleProblem returns the problem of an error of the article use cases, or false for another
// error.
func (s *Server) articleProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (problem, bool) {
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail), true
	}
	var tooLong *appcontent.ArticleTooLongError
	var tooManyImages *appcontent.TooManyArticleImagesError
	switch {
	case errors.Is(err, appcontent.ErrArticlesNotPermitted):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeBlank, l.ServerErrorsForbidden(),
			l.ServerErrorsMissingPermissionDetail(string(identity.PermCreateArticles))), true
	case errors.Is(err, appcontent.ErrArticleDirect):
		return unprocessable(l.ServerErrorsStatusArticleDirect())
	case errors.Is(err, appcontent.ErrInvalidArticleTitle):
		return unprocessable(l.ServerErrorsStatusArticleTitleInvalid(content.MaxArticleTitle))
	case errors.Is(err, appcontent.ErrInvalidArticleSummary):
		return unprocessable(l.ServerErrorsStatusArticleSummaryInvalid(content.MaxArticleSummary))
	case errors.As(err, &tooLong):
		return unprocessable(l.ServerErrorsStatusArticleTooLong(tooLong.Max))
	case errors.As(err, &tooManyImages):
		return unprocessable(l.ServerErrorsStatusArticleTooManyImages(tooManyImages.Max))
	case errors.Is(err, appcontent.ErrTooManyHashtags):
		return unprocessable(l.ServerErrorsStatusArticleTooManyTags(content.MaxArticleTags))
	case errors.Is(err, appcontent.ErrInvalidHashtag):
		return unprocessable(l.ServerErrorsStatusInvalidHashtag())
	case errors.Is(err, appcontent.ErrNotArticle):
		return unprocessable(l.ServerErrorsStatusNotArticle())
	case errors.Is(err, appcontent.ErrArticleEditedAsPost):
		return unprocessable(l.ServerErrorsStatusArticleEditedAsPost())
	}
	return problem{}, false
}

// withArticleBody gives the Status of an article its body, which lists leave out (ADR-0160 §8):
// the operations that present one article call it. The images of a received article are shown
// through the media proxy (StatusReader.ArticleBody).
func (s *Server) withArticleBody(ctx context.Context, st *Status, v appcontent.StatusView) error {
	if v.Revision.Article == nil {
		return nil
	}
	body, err := s.o.StatusReader.ArticleBody(ctx, v.Revision)
	if err != nil {
		return err
	}
	st.Content = body
	return nil
}

// newArticleSource is what the author wrote of an article besides its body, nil for a post.
func newArticleSource(v appcontent.StatusView) *ArticleSource {
	a := v.Revision.Article
	if a == nil {
		return nil
	}
	out := &ArticleSource{Title: a.Title, Tags: make([]string, 0, len(v.Tags))}
	if a.SummaryOrigin == content.SummaryByAuthor {
		out.Summary = a.Summary
	}
	if c := a.Cover; c != nil {
		out.Cover = &StatusMediaItem{Id: c.MediaID}
		if c.Description != "" {
			d := c.Description
			out.Cover.Description = &d
		}
	}
	for _, t := range v.Tags {
		out.Tags = append(out.Tags, t.Name)
	}
	return out
}

// articleInput maps the content of an article of a request.
func articleInput(title string, summary *string, cover *ArticleCoverInput, text string, tags *ArticleTagsInput, language *StatusLanguageInput, sensitive *bool) appcontent.ArticleInput {
	in := appcontent.ArticleInput{
		Title: title, Summary: deref(summary), Text: text, Language: deref(language), Sensitive: deref(sensitive),
	}
	if tags != nil {
		in.Tags = *tags
	}
	if cover != nil {
		in.Cover = &appcontent.MediaInput{MediaID: knownID(cover.Id), Description: deref(cover.Description)}
	}
	return in
}

// articleView presents the article statusID with its body, as the answer of a write of it.
func (s *Server) articleView(ctx context.Context, statusID string) (Status, error) {
	view, err := s.o.StatusReader.View(ctx, contentViewer(ctx), statusID)
	if err != nil {
		return Status{}, err
	}
	out, err := s.presentStatus(ctx, view)
	if err != nil {
		return Status{}, err
	}
	if err := s.withArticleBody(ctx, &out, view); err != nil {
		return Status{}, err
	}
	return out, nil
}

// CreateArticle serves POST /api/v1/articles.
func (s *Server) CreateArticle(ctx context.Context, req CreateArticleRequestObject) (CreateArticleResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	fail := func(p problem) (CreateArticleResponseObject, error) {
		return CreateArticledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if s.o.Articles == nil {
		return fail(s.notFoundResponse(ctx))
	}
	key := deref(req.Params.IdempotencyKey)
	if key != "" {
		post, err := s.o.Statuses.Replay(ctx, v.AccountID, key)
		if err == nil {
			return s.createdArticle(ctx, post.Status.ID)
		}
		if !errors.Is(err, appcontent.ErrNotFound) {
			if p, ok := s.statusProblem(ctx, err); ok {
				return fail(p)
			}
			return nil, err
		}
	}
	if p, h, limited := s.LimitProblem(ctx, PostLimit, v.AccountID); limited {
		return CreateArticledefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	if p, err := s.refuseMoved(ctx, v.AccountID); err != nil {
		return nil, err
	} else if p != nil {
		return fail(*p)
	}
	if req.Body == nil {
		return fail(s.invalidParameterResponse(ctx, "body"))
	}
	b := req.Body
	post, err := s.o.Articles.CreateArticle(ctx, appcontent.CreateArticleInput{
		ArticleInput:    articleInput(b.Title, b.Summary, b.Cover, b.Text, b.Tags, b.Language, b.Sensitive),
		AuthorAccountID: v.AccountID, Visibility: content.Visibility(b.Visibility),
		ClientID: v.ClientID, IdempotencyKey: key, InteractionPolicy: interactionPolicyChange(b.InteractionPolicy),
	})
	if err != nil {
		if r, ok, err := s.rejectedProblem(ctx, err); ok {
			if err != nil {
				return nil, err
			}
			return CreateArticle422ApplicationProblemPlusJSONResponse{r}, nil
		}
		if p, ok := s.statusProblem(ctx, err); ok {
			return fail(p)
		}
		return nil, err
	}
	return s.createdArticle(ctx, post.Status.ID)
}

// createdArticle answers POST /api/v1/articles with the article posted.
func (s *Server) createdArticle(ctx context.Context, statusID string) (CreateArticleResponseObject, error) {
	out, err := s.articleView(ctx, statusID)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return CreateArticledefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return CreateArticle201JSONResponse{Body: out, Headers: CreateArticle201ResponseHeaders{CacheControl: statusCache(ctx)}}, nil
}

// EditArticle serves PUT /api/v1/articles/{id}.
func (s *Server) EditArticle(ctx context.Context, req EditArticleRequestObject) (EditArticleResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	fail := func(p problem) (EditArticleResponseObject, error) {
		return EditArticledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if s.o.Articles == nil {
		return fail(s.notFoundResponse(ctx))
	}
	if req.Body == nil {
		return fail(s.invalidParameterResponse(ctx, "body"))
	}
	b := req.Body
	_, err = s.o.Articles.EditArticle(ctx, appcontent.EditArticleInput{
		ArticleInput: articleInput(b.Title, b.Summary, b.Cover, b.Text, b.Tags, b.Language, b.Sensitive),
		AccountID:    v.AccountID, StatusID: req.Id,
	})
	if err == nil {
		var out Status
		if out, err = s.articleView(ctx, req.Id); err == nil {
			return EditArticle200JSONResponse{Body: out, Headers: EditArticle200ResponseHeaders{CacheControl: statusCache(ctx)}}, nil
		}
	}
	if errors.Is(err, appcontent.ErrRejectedByReview) {
		// An edit that local.status refused, as editStatus answers one (ADR-0147 §2).
		l := s.Localizer(ctx)
		return fail(s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeStatusRejected,
			l.ServerErrorsStatusEditRejected(), l.ServerErrorsStatusEditRejectedDetail()))
	}
	if p, ok := s.authorProblem(ctx, v, req.Id, err); ok {
		return fail(p)
	}
	return nil, err
}

// PreviewArticle serves POST /api/v1/articles/preview.
func (s *Server) PreviewArticle(ctx context.Context, req PreviewArticleRequestObject) (PreviewArticleResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	fail := func(p problem) (PreviewArticleResponseObject, error) {
		return PreviewArticledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if s.o.Articles == nil {
		return fail(s.notFoundResponse(ctx))
	}
	if req.Body == nil {
		return fail(s.invalidParameterResponse(ctx, "body"))
	}
	p, err := s.o.Articles.PreviewArticle(ctx, v.AccountID, req.Body.Text, deref(req.Body.Summary))
	if err != nil {
		if pr, ok := s.statusProblem(ctx, err); ok {
			return fail(pr)
		}
		return nil, err
	}
	cache := cacheNoStore
	return PreviewArticle200JSONResponse{
		Body: ArticlePreview{
			Content: p.HTML, Summary: p.Summary, SummaryFromBody: p.SummaryOrigin == content.SummaryFromBody,
			Bytes: p.Bytes, MaxBytes: p.MaxBytes, Images: p.Images, MaxImages: p.MaxImages,
		},
		Headers: PreviewArticle200ResponseHeaders{CacheControl: &cache},
	}, nil
}
