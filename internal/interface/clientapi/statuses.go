// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// Statuses runs the posting use cases of local statuses. *appcontent.Service implements it.
type Statuses interface {
	Create(ctx context.Context, in appcontent.CreateInput) (appcontent.Post, error)
	// Replay returns the post that the author posted with an Idempotency-Key; ErrNotFound when none.
	Replay(ctx context.Context, authorAccountID, key string) (appcontent.Post, error)
	Edit(ctx context.Context, in appcontent.EditInput) (appcontent.Post, error)
	Delete(ctx context.Context, accountID, statusID string) (content.Deletion, error)
	SetQuotePolicy(ctx context.Context, accountID, statusID string, policy content.QuotePolicy) (content.StatusQuotePolicy, error)
}

// OutboxFetcher starts fetching the recent posts of a remote account whose profile a viewer
// opens (ADR-0044), and reports whether it did; the conditions are its own.
// *federation.StatusHandlers implements it.
type OutboxFetcher interface {
	FetchOutbox(ctx context.Context, viewerAccountID, accountID string) (bool, error)
}

// ThreadFetcher starts fetching the missing ancestors and the replies of a status from their
// servers, without waiting for them. *federation.StatusHandlers implements it.
type ThreadFetcher interface {
	FetchThread(ctx context.Context, statusID string) error
}

// StatusReader presents statuses to a viewer. *appcontent.Reader implements it.
type StatusReader interface {
	View(ctx context.Context, viewer appcontent.Viewer, statusID string) (appcontent.StatusView, error)
	Thread(ctx context.Context, viewer appcontent.Viewer, statusID string) (appcontent.Thread, error)
	History(ctx context.Context, viewer appcontent.Viewer, statusID string) ([]appcontent.RevisionView, error)
	Source(ctx context.Context, accountID, statusID string) (content.Revision, error)
	Bookmarks(ctx context.Context, accountID, before string, limit int) (appcontent.BookmarkPage, error)
	Reactions(ctx context.Context, viewer appcontent.Viewer, statusID, emoji, before string, limit int) (appcontent.ReactedPage, error)
	ReactionEmoji(ctx context.Context, e appcontent.ReactionEmoji) (appcontent.EmojiView, error)
	Quotes(ctx context.Context, accountID, statusID, before string, limit int) (appcontent.QuotesPage, error)
}

// Interactions runs boosts, favourites, bookmarks, and pins. *appcontent.Interactions
// implements it.
type Interactions interface {
	Boost(ctx context.Context, accountID, statusID string) (content.Boost, error)
	Unboost(ctx context.Context, accountID, statusID string) error
	Favourite(ctx context.Context, accountID, statusID string) (content.Favourite, error)
	Unfavourite(ctx context.Context, accountID, statusID string) error
	React(ctx context.Context, accountID, statusID, emoji string) (content.Reaction, error)
	Unreact(ctx context.Context, accountID, statusID string) error
	Bookmark(ctx context.Context, accountID, statusID string) error
	Unbookmark(ctx context.Context, accountID, statusID string) error
	Pin(ctx context.Context, accountID, statusID string) error
	Unpin(ctx context.Context, accountID, statusID string) error
	VotePoll(ctx context.Context, accountID, statusID, pollID string, choices []int) ([]content.PollVote, error)
	RevokeQuote(ctx context.Context, accountID, quotedStatusID, quotingStatusID string) error
}

// defaultLimit is the page size of collections without a limit parameter.
const defaultLimit = 20

// contentViewer returns the viewer of ctx for the content module: the authenticated local
// account, or an anonymous visitor. A token without read:direct does not see direct posts.
func contentViewer(ctx context.Context) appcontent.Viewer {
	if v, ok := ViewerFrom(ctx); ok {
		return localContentViewer(v)
	}
	return appcontent.Viewer{Kind: appcontent.ViewerAnonymous}
}

// localContentViewer returns the content viewer of an authenticated request, which sees direct
// posts only when its token grants read:direct (ADR-0090 §4). The visibility itself is still
// decided by the single policy function.
func localContentViewer(v Viewer) appcontent.Viewer {
	return appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: v.AccountID, DirectDenied: !v.Scope.Grants(identity.ScopeReadDirect)}
}

// statusCache returns the Cache-Control of a successful response about statuses. An anonymous
// visitor sees only what anyone may see (public and unlisted statuses), so shared caches may
// store anonymous responses; responses to a user depend on the user.
func statusCache(ctx context.Context) *string {
	c := cacheNoStore
	if _, ok := ViewerFrom(ctx); !ok {
		c = cachePublic
	}
	return &c
}

// problem is the default response of any operation; each operation converts it to its own
// default response type, which has the same fields.
type problem struct {
	Body       Problem
	Headers    ProblemResponseHeaders
	StatusCode int
}

func (s *Server) problem(ctx context.Context, status int, typ, title, detail string) problem {
	body, headers := s.NewProblem(ctx, status, typ, title, detail)
	return problem{Body: body, Headers: headers, StatusCode: status}
}

// statusProblem returns the problem that err of a statuses use case means to the client, or
// false for an unexpected error, which the handler returns as a 500.
func (s *Server) statusProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (problem, bool) {
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail), true
	}
	var tooLong *appcontent.TextTooLongError
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return s.problem(ctx, http.StatusNotFound, ProblemTypeBlank, l.ServerErrorsNotFound(), ""), true
	case errors.Is(err, appcontent.ErrAccountUnavailable), errors.Is(err, appcontent.ErrActorUnavailable),
		errors.Is(err, appcontent.ErrRemoteAccount):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeAccountUnavailable, l.ServerErrorsAccountUnavailable(), ""), true
	case errors.Is(err, appcontent.ErrPostingRestricted):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeBlank, l.ServerErrorsForbidden(), l.ServerErrorsStatusPostingRestricted()), true
	case errors.Is(err, appcontent.ErrInteractionRestricted):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeBlank, l.ServerErrorsForbidden(), l.ServerErrorsInteractionRestricted()), true
	case errors.Is(err, appcontent.ErrNotAuthor):
		return s.problem(ctx, http.StatusForbidden, ProblemTypeBlank, l.ServerErrorsForbidden(), l.ServerErrorsStatusNotAuthor()), true
	case errors.Is(err, appcontent.ErrAlreadyDeleted):
		return s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), l.ServerErrorsStatusAlreadyDeleted()), true
	case errors.As(err, &tooLong):
		return unprocessable(l.ServerErrorsStatusTextTooLong(tooLong.Max))
	case errors.Is(err, appcontent.ErrEmptyText):
		return unprocessable(l.ServerErrorsStatusEmptyText())
	case errors.Is(err, appcontent.ErrInvalidText):
		return unprocessable(l.ServerErrorsStatusInvalidText())
	case errors.Is(err, appcontent.ErrInvalidLanguage):
		return unprocessable(l.ServerErrorsStatusInvalidLanguage())
	case errors.Is(err, content.ErrInvalidIdempotencyKey):
		return unprocessable(l.ServerErrorsInvalidValue("Idempotency-Key"))
	case errors.Is(err, content.ErrInvalidVisibility):
		return unprocessable("")
	case errors.Is(err, appcontent.ErrReplyTargetNotFound):
		return unprocessable(l.ServerErrorsStatusReplyNotFound())
	case errors.Is(err, appcontent.ErrQuoteTargetNotFound):
		return unprocessable(l.ServerErrorsStatusQuoteNotFound())
	case errors.Is(err, appcontent.ErrQuoteNotAllowed):
		return unprocessable(l.ServerErrorsStatusQuoteNotAllowed())
	case errors.Is(err, appcontent.ErrInvalidQuotePolicy):
		return unprocessable(l.ServerErrorsInvalidValue("quote_policy"))
	case errors.Is(err, appcontent.ErrTooManyMedia):
		return unprocessable(l.ServerErrorsStatusTooManyMedia(appcontent.MaxAttachments))
	case errors.Is(err, appcontent.ErrMediaNotFound):
		return unprocessable(l.ServerErrorsStatusMediaNotFound())
	case errors.Is(err, appcontent.ErrMediaNotReady):
		return unprocessable(l.ServerErrorsStatusMediaNotReady())
	case errors.Is(err, appcontent.ErrInvalidMediaDescription):
		return unprocessable(l.ServerErrorsStatusInvalidMediaDescription(appcontent.MaxMediaDescription))
	case errors.Is(err, appcontent.ErrMissingMediaDescription):
		return unprocessable(l.ServerErrorsStatusMissingMediaDescription())
	case errors.Is(err, content.ErrNotBoostable):
		return unprocessable(l.ServerErrorsStatusNotBoostable())
	case errors.Is(err, content.ErrNotPinnable):
		return unprocessable(l.ServerErrorsStatusNotPinnable())
	case errors.Is(err, content.ErrPinLimit):
		return unprocessable(l.ServerErrorsStatusPinLimit(content.MaxPins))
	case errors.Is(err, content.ErrInvalidReaction):
		return unprocessable(l.ServerErrorsStatusInvalidReaction())
	case errors.Is(err, content.ErrNoPoll):
		return unprocessable(l.ServerErrorsStatusPollMissing())
	case errors.Is(err, content.ErrPollChanged):
		return s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), l.ServerErrorsStatusPollChanged()), true
	case errors.Is(err, content.ErrPollClosed):
		return unprocessable(l.ServerErrorsStatusPollClosed())
	case errors.Is(err, content.ErrAlreadyVoted):
		return unprocessable(l.ServerErrorsStatusPollAlreadyVoted())
	case errors.Is(err, content.ErrInvalidPollChoices):
		return unprocessable(l.ServerErrorsStatusPollInvalidChoices())
	case errors.Is(err, content.ErrInvalidPoll):
		return unprocessable(l.ServerErrorsStatusPollInvalid(i18n.ServerErrorsStatusPollInvalidArgs{Length: strconv.Itoa(content.MaxPollOptionLength), Max: strconv.Itoa(content.MaxLocalPollOptions), Min: strconv.Itoa(content.MinPollOptions)}))
	case errors.Is(err, appcontent.ErrPollWithMedia):
		return unprocessable(l.ServerErrorsStatusPollWithMedia())
	case errors.Is(err, appcontent.ErrPollNotEditable):
		return unprocessable(l.ServerErrorsStatusPollNotEditable())
	}
	return problem{}, false
}

// authorProblem returns the problem of an edit or deletion that failed with err. Another
// account's status is a 403 only when the viewer may see it, and a 404 otherwise, so that the
// answer does not reveal a status the viewer may not see.
func (s *Server) authorProblem(ctx context.Context, v Viewer, statusID string, err error) (problem, bool) {
	if errors.Is(err, appcontent.ErrNotAuthor) {
		_, viewErr := s.o.StatusReader.View(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: v.AccountID}, statusID)
		if errors.Is(viewErr, appcontent.ErrNotFound) {
			err = viewErr
		} else if viewErr != nil {
			return problem{}, false
		}
	}
	return s.statusProblem(ctx, err)
}

// viewStatus returns the status as the viewer of ctx sees it, for the response of a command.
func (s *Server) viewStatus(ctx context.Context, statusID string) (Status, error) {
	viewer := contentViewer(ctx)
	// The response to a command shows the post it acted on, the author's own direct post
	// included, so write:statuses alone can send one. Another account's direct post stays
	// hidden from a token without read:direct.
	viewer.DirectDenied = false
	view, err := s.o.StatusReader.View(ctx, viewer, statusID)
	if err != nil {
		return Status{}, err
	}
	if v, ok := ViewerFrom(ctx); ok && contentViewer(ctx).DirectDenied &&
		view.Status.Visibility == appcontent.VisibilityDirect && view.Status.AuthorAccountID != v.AccountID {
		return Status{}, appcontent.ErrNotFound
	}
	return s.presentStatus(ctx, view)
}

// presentStatus presents one status as the Status schema, with its author's images.
func (s *Server) presentStatus(ctx context.Context, view appcontent.StatusView) (Status, error) {
	parts, err := s.statusParts(ctx, []appcontent.StatusView{view})
	if err != nil {
		return Status{}, err
	}
	return s.newStatus(view, parts), nil
}

// presentStatuses presents views in order as the Status schema, reading the images of their
// authors together.
func (s *Server) presentStatuses(ctx context.Context, views []appcontent.StatusView) ([]Status, error) {
	parts, err := s.statusParts(ctx, views)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(views))
	for _, v := range views {
		out = append(out, s.newStatus(v, parts))
	}
	return out, nil
}

// statusParts reads the parts of the authors of views, of the statuses they quote, and of more
// accounts, and the profile pages of the accounts that they mention, together
// (Server.accountParts).
func (s *Server) statusParts(ctx context.Context, views []appcontent.StatusView, more ...appaccount.Actor) (accountParts, error) {
	actors := slices.Clone(more)
	var mentioned []appaccount.Actor
	var emojis []string
	add := func(v appcontent.StatusView) {
		actors = append(actors, v.Author)
		if v.Card != nil && v.Card.Author != nil {
			actors = append(actors, *v.Card.Author)
		}
		mentioned = append(mentioned, v.Mentions...)
		emojis = append(emojis, v.Revision.EmojiIDs...)
	}
	for _, v := range views {
		add(v)
		if v.Quote != nil && v.Quote.Status != nil {
			add(*v.Quote.Status)
		}
	}
	return s.readAccountParts(ctx, actors, mentioned, emojis)
}

// newStatus presents a status as the Status schema; parts holds its author's images and the
// profile pages of its author and mentions (statusParts).
func (s *Server) newStatus(v appcontent.StatusView, parts accountParts) Status {
	st, rev := v.Status, v.Revision
	out := Status{
		Id: st.ID, Uri: st.URI, Account: s.newAccount(v.Author, parts),
		SpoilerText: rev.SpoilerText, Sensitive: rev.Sensitive, Visibility: Visibility(st.Visibility),
		Language: rev.Language, CreatedAt: st.PublishedAt,
		MediaAttachments: newMediaAttachments(v.Attachments),
		Mentions:         make([]StatusMention, 0, len(v.Mentions)),
		Tags:             make([]StatusTag, 0, len(v.Tags)),
		Emojis:           parts.customEmojis(rev.EmojiIDs),
		RepliesCount:     int(v.Counters.RepliesCount), BoostsCount: int(v.Counters.BoostsCount),
		FavouritesCount: int(v.Counters.FavouritesCount),
		Reactions:       newStatusReactions(v.Reactions, v.Relation != nil),
	}
	// A local status's page is derived from its author's current username (ADR-0053) and served
	// by the web client's origin.
	if u := appcontent.StatusPageURL(s.o.Host, st, v.Author); u != "" {
		if st.IsLocal {
			u = s.pageURL(u)
		}
		out.Url = &u
	}
	if len(rev.Bodies) > 0 {
		out.Content = rev.Bodies[0].ContentHTML
	}
	if out.Language == "" {
		out.Language = appcontent.UndeterminedLanguage
	}
	if p := v.InReplyTo; p != nil {
		out.InReplyToId, out.InReplyToAccountId = &p.ID, &p.AuthorAccountID
	}
	if v.Edited() {
		at := rev.RevisedAt
		out.EditedAt = &at
	}
	for _, m := range v.Mentions {
		a := s.newAccount(m, parts)
		out.Mentions = append(out.Mentions, StatusMention{Id: a.Id, Username: a.Username, Acct: a.Acct, Url: a.Url})
	}
	for _, t := range v.Tags {
		out.Tags = append(out.Tags, StatusTag{Name: t.Name, Url: s.origin() + "/tags/" + url.PathEscape(t.NameNormalized)})
	}
	if r := v.Relation; r != nil {
		out.Favourited, out.Boosted, out.Bookmarked = &r.Favourited, &r.Boosted, &r.Bookmarked
		if r.Author {
			out.Pinned = &r.Pinned
		}
	}
	if v.Card != nil {
		out.Card = s.newLinkCard(*v.Card, parts)
	}
	if v.Poll != nil {
		p := newPoll(*v.Poll, v.Relation != nil)
		out.Poll = &p
	}
	if v.Relation != nil {
		out.Quotable = &v.Quotable
	}
	if v.QuotePolicy != "" {
		policy := QuotePolicy(v.QuotePolicy)
		out.QuotePolicy = &policy
	}
	if q := v.Quote; q != nil {
		out.Quote = &StatusQuote{State: QuoteState(q.State)}
		if q.Status != nil {
			quoted := s.newStatus(*q.Status, parts)
			out.Quote.QuotedStatus = &quoted
		}
	}
	return out
}

// newPoll presents the poll of a status; withViewer adds the authenticated user's vote.
func newPoll(v appcontent.PollView, withViewer bool) Poll {
	out := Poll{
		Id: v.Poll.ID, ExpiresAt: v.Poll.ClosesAt, Expired: v.Closed, Multiple: v.Poll.Multiple,
		VotesCount: int(v.VotesCount()), Options: make([]PollOption, 0, len(v.Poll.Options)),
	}
	if v.VotersCount != nil {
		n := int(*v.VotersCount)
		out.VotersCount = &n
	}
	for i, o := range v.Poll.Options {
		opt := PollOption{Title: o.Title}
		if !v.TotalsHidden {
			var votes int
			if i < len(v.Votes) {
				votes = int(v.Votes[i])
			}
			opt.VotesCount = &votes
		}
		out.Options = append(out.Options, opt)
	}
	if withViewer {
		own := slices.Clone(v.OwnVotes)
		if own == nil {
			own = []int{}
		}
		voted := len(own) > 0
		out.Voted, out.OwnVotes = &voted, &own
	}
	return out
}

// newLinkCard presents the card of a status (ADR-0062 §8); parts holds its author's images.
func (s *Server) newLinkCard(v appcontent.LinkCardView, parts accountParts) *LinkCard {
	c := v.Card
	out := &LinkCard{
		Url: c.CardURL, Title: c.Title, Description: c.Description, SiteName: c.SiteName,
		AuthorName: optionalString(c.AuthorName), PublishedAt: c.PublishedAt, Language: optionalString(c.Language),
	}
	if img := v.Image; img != nil {
		out.Image = &LinkCardImage{
			Url: img.URL, Width: img.Width, Height: img.Height,
			Blurhash: optionalString(img.Blurhash), Description: optionalString(img.Description),
		}
	}
	if v.Author != nil {
		a := s.newAccount(*v.Author, parts)
		out.Author = &a
	}
	return out
}

// optionalString returns nil for "", so that an empty value is omitted.
func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// viewImage presents the image of a MediaView, which the reader resolved already.
type viewImage struct{ img appcontent.Image }

func (v viewImage) Image(appcontent.Media) (appcontent.Image, bool) { return v.img, true }

// newMediaAttachments presents the attachments of a revision as MediaAttachment (media.go),
// with the revision's copy of each description.
func newMediaAttachments(in []appcontent.MediaView) []MediaAttachment {
	out := make([]MediaAttachment, 0, len(in))
	for _, m := range in {
		var a MediaAttachment
		if m.Media.Remote != nil {
			a = newRemoteAttachment(m.Media, m.Image)
		} else {
			a = newMediaAttachment(m.Media, viewImage{m.Image})
		}
		a.Description = nil
		if m.Description != "" {
			a.Description = &m.Description
		}
		out = append(out, a)
	}
	return out
}

func mediaInput(in *StatusMediaInput) []appcontent.MediaInput {
	if in == nil {
		return nil
	}
	out := make([]appcontent.MediaInput, 0, len(*in))
	for _, m := range *in {
		item := appcontent.MediaInput{MediaID: knownID(m.Id)}
		if m.Description != nil {
			item.Description = *m.Description
		}
		out = append(out, item)
	}
	return out
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// CreateStatus serves POST /api/v1/statuses.
func (s *Server) CreateStatus(ctx context.Context, req CreateStatusRequestObject) (CreateStatusResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	// A retry with the key of a status already posted returns that status, before the rate limit and
	// the other checks of a new post, which the first request passed.
	if key := deref(req.Params.IdempotencyKey); key != "" {
		post, err := s.o.Statuses.Replay(ctx, v.AccountID, key)
		if err == nil {
			return s.createdStatus(ctx, post.Status.ID)
		}
		if !errors.Is(err, appcontent.ErrNotFound) {
			if p, ok := s.statusProblem(ctx, err); ok {
				return CreateStatusdefaultApplicationProblemPlusJSONResponse(p), nil
			}
			return nil, err
		}
	}
	if p, h, limited := s.LimitProblem(ctx, PostLimit, v.AccountID); limited {
		return CreateStatusdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	if p, err := s.refuseMoved(ctx, v.AccountID); err != nil {
		return nil, err
	} else if p != nil {
		return CreateStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	body := req.Body
	if body.QuotedStatusId != nil {
		if p, err := s.quotableRemotely(ctx, knownID(*body.QuotedStatusId)); err != nil {
			return nil, err
		} else if p != nil {
			return CreateStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
		}
	}
	post, err := s.o.Statuses.Create(ctx, appcontent.CreateInput{
		TextInput: appcontent.TextInput{
			Text: body.Text, SpoilerText: deref(body.SpoilerText), Sensitive: deref(body.Sensitive),
			Language: deref(body.Language), Media: mediaInput(body.Media),
		},
		AuthorAccountID: v.AccountID, Visibility: content.Visibility(body.Visibility), InReplyToID: knownID(deref(body.InReplyToId)),
		ClientID: v.ClientID, IdempotencyKey: deref(req.Params.IdempotencyKey), Poll: pollInput(body.Poll),
		QuotedStatusID: knownID(deref(body.QuotedStatusId)), QuotePolicy: content.QuotePolicy(deref(body.QuotePolicy)),
	})
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return CreateStatusdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return s.createdStatus(ctx, post.Status.ID)
}

// createdStatus answers POST /api/v1/statuses with the status posted.
func (s *Server) createdStatus(ctx context.Context, statusID string) (CreateStatusResponseObject, error) {
	out, err := s.viewStatus(ctx, statusID)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return CreateStatusdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return CreateStatus200JSONResponse{Body: out, Headers: CreateStatus200ResponseHeaders{CacheControl: statusCache(ctx)}}, nil
}

// quotableRemotely refuses a quote of a remote status while the federation mode does not send
// posts out, since the QuoteRequest cannot go to its server (ADR-0076). A status that the viewer
// cannot see is left to Create, which answers it as any quote of one.
func (s *Server) quotableRemotely(ctx context.Context, statusID string) (*problem, error) {
	if ok, err := s.federationAllows(ctx, remoteQuoteFlows...); err != nil || ok {
		return nil, err
	}
	view, err := s.o.StatusReader.View(ctx, contentViewer(ctx), statusID)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	case view.Status.IsLocal:
		return nil, nil
	}
	p := s.federationRestricted(ctx)
	return &p, nil
}

// pollInput maps the poll of a new status, nil for none.
func pollInput(p *PollInput) *content.PollInput {
	if p == nil {
		return nil
	}
	return &content.PollInput{
		Options: p.Options, Duration: time.Duration(p.ExpiresIn) * time.Second,
		Multiple: deref(p.Multiple), HideTotals: deref(p.HideTotals),
	}
}

// GetStatus serves GET /api/v1/statuses/{id}.
func (s *Server) GetStatus(ctx context.Context, req GetStatusRequestObject) (GetStatusResponseObject, error) {
	view, err := s.o.StatusReader.View(ctx, contentViewer(ctx), req.Id)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return GetStatusdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	body, err := s.presentStatus(ctx, view)
	if err != nil {
		return nil, err
	}
	return GetStatus200JSONResponse{
		Body:    body,
		Headers: GetStatus200ResponseHeaders{CacheControl: statusCache(ctx)},
	}, nil
}

// EditStatus serves PUT /api/v1/statuses/{id}.
func (s *Server) EditStatus(ctx context.Context, req EditStatusRequestObject) (EditStatusResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	body := req.Body
	_, err = s.o.Statuses.Edit(ctx, appcontent.EditInput{
		TextInput: appcontent.TextInput{
			Text: body.Text, SpoilerText: deref(body.SpoilerText), Sensitive: deref(body.Sensitive),
			Language: deref(body.Language), Media: mediaInput(body.Media),
		},
		AccountID: v.AccountID, StatusID: req.Id, DetachQuote: deref(body.RemoveQuote),
	})
	if err == nil {
		var out Status
		if out, err = s.viewStatus(ctx, req.Id); err == nil {
			return EditStatus200JSONResponse{Body: out, Headers: EditStatus200ResponseHeaders{CacheControl: statusCache(ctx)}}, nil
		}
	}
	if p, ok := s.authorProblem(ctx, v, req.Id, err); ok {
		return EditStatusdefaultApplicationProblemPlusJSONResponse(p), nil
	}
	return nil, err
}

// DeleteStatus serves DELETE /api/v1/statuses/{id}.
func (s *Server) DeleteStatus(ctx context.Context, req DeleteStatusRequestObject) (DeleteStatusResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.o.Statuses.Delete(ctx, v.AccountID, req.Id); err != nil {
		if p, ok := s.authorProblem(ctx, v, req.Id, err); ok {
			return DeleteStatusdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return DeleteStatus204Response{}, nil
}

// GetStatusContext serves GET /api/v1/statuses/{id}/context.
func (s *Server) GetStatusContext(ctx context.Context, req GetStatusContextRequestObject) (GetStatusContextResponseObject, error) {
	thread, err := s.o.StatusReader.Thread(ctx, contentViewer(ctx), req.Id)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return GetStatusContextdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	if s.o.Threads != nil {
		// The thread shows what is stored now; the fetched parts show the next time.
		if err := s.o.Threads.FetchThread(ctx, req.Id); err != nil {
			s.o.Logger.WarnContext(ctx, "fetch remote thread", slog.String("status_id", req.Id), slog.Any("error", err))
		}
	}
	parts, err := s.statusParts(ctx, slices.Concat(thread.Ancestors, thread.Descendants))
	if err != nil {
		return nil, err
	}
	out := StatusContext{Ancestors: make([]Status, 0, len(thread.Ancestors)), Descendants: make([]Status, 0, len(thread.Descendants))}
	for _, v := range thread.Ancestors {
		out.Ancestors = append(out.Ancestors, s.newStatus(v, parts))
	}
	for _, v := range thread.Descendants {
		out.Descendants = append(out.Descendants, s.newStatus(v, parts))
	}
	return GetStatusContext200JSONResponse{
		Body: out, Headers: GetStatusContext200ResponseHeaders{CacheControl: statusCache(ctx)},
	}, nil
}

// GetStatusHistory serves GET /api/v1/statuses/{id}/history.
func (s *Server) GetStatusHistory(ctx context.Context, req GetStatusHistoryRequestObject) (GetStatusHistoryResponseObject, error) {
	viewer := contentViewer(ctx)
	revs, err := s.o.StatusReader.History(ctx, viewer, req.Id)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return GetStatusHistorydefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	var emojiIDs []string
	for _, r := range revs {
		emojiIDs = append(emojiIDs, r.Revision.EmojiIDs...)
	}
	parts, err := s.readAccountParts(ctx, nil, nil, emojiIDs)
	if err != nil {
		return nil, err
	}
	out := StatusHistory{Items: make([]StatusEdit, 0, len(revs))}
	for _, r := range revs {
		e := StatusEdit{
			SpoilerText: r.Revision.SpoilerText, Sensitive: r.Revision.Sensitive, CreatedAt: r.Revision.RevisedAt,
			MediaAttachments: newMediaAttachments(r.Attachments), Emojis: parts.customEmojis(r.Revision.EmojiIDs),
		}
		if len(r.Revision.Bodies) > 0 {
			e.Content = r.Revision.Bodies[0].ContentHTML
		}
		out.Items = append(out.Items, e)
	}
	return GetStatusHistory200JSONResponse{
		Body: out, Headers: GetStatusHistory200ResponseHeaders{CacheControl: statusCache(ctx)},
	}, nil
}

// GetStatusSource serves GET /api/v1/statuses/{id}/source.
func (s *Server) GetStatusSource(ctx context.Context, req GetStatusSourceRequestObject) (GetStatusSourceResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	rev, err := s.o.StatusReader.Source(ctx, v.AccountID, req.Id)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return GetStatusSourcedefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	out := StatusSource{Id: req.Id, Text: rev.Source.Text, SpoilerText: rev.SpoilerText, Language: rev.Language}
	if out.Language == "" {
		out.Language = appcontent.UndeterminedLanguage
	}
	return GetStatusSource200JSONResponse{Body: out, Headers: GetStatusSource200ResponseHeaders{CacheControl: statusCache(ctx)}}, nil
}

// ListBookmarks serves GET /api/v1/bookmarks.
func (s *Server) ListBookmarks(ctx context.Context, req ListBookmarksRequestObject) (ListBookmarksResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	limit := defaultLimit
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if !validCursor(req.Params.Cursor) {
		return ListBookmarksdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
	}
	page, err := s.o.StatusReader.Bookmarks(ctx, v.AccountID, deref(req.Params.Cursor), limit)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return ListBookmarksdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	items, err := s.presentStatuses(ctx, page.Statuses)
	if err != nil {
		return nil, err
	}
	out := StatusPage{Items: items}
	if page.Next != "" {
		out.NextCursor = &page.Next
	}
	return ListBookmarks200JSONResponse{Body: out, Headers: ListBookmarks200ResponseHeaders{CacheControl: statusCache(ctx)}}, nil
}

// act runs an interaction of the viewer with a status and returns the status afterwards, or
// the problem of the failure.
func (s *Server) act(ctx context.Context, statusID string, run func(accountID string) error) (StatusAfterActionJSONResponse, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return StatusAfterActionJSONResponse{}, nil, err
	}
	err = run(v.AccountID)
	if err == nil {
		var out Status
		if out, err = s.viewStatus(ctx, statusID); err == nil {
			return StatusAfterActionJSONResponse{Body: out, Headers: StatusAfterActionResponseHeaders{CacheControl: statusCache(ctx)}}, nil, nil
		}
	}
	if p, ok := s.statusProblem(ctx, err); ok {
		return StatusAfterActionJSONResponse{}, &p, nil
	}
	return StatusAfterActionJSONResponse{}, nil, err
}

// actUnmoved is act for the actions that a moved account cannot do (ADR-0089): boosting,
// reacting, and voting, refused with ProblemTypeAccountMoved.
func (s *Server) actUnmoved(ctx context.Context, statusID string, run func(accountID string) error) (StatusAfterActionJSONResponse, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return StatusAfterActionJSONResponse{}, nil, err
	}
	if p, err := s.refuseMoved(ctx, v.AccountID); err != nil || p != nil {
		return StatusAfterActionJSONResponse{}, p, err
	}
	return s.act(ctx, statusID, run)
}

func ignoreResult[T any](_ T, err error) error { return err }

// BoostStatus serves POST /api/v1/statuses/{id}/boost.
func (s *Server) BoostStatus(ctx context.Context, req BoostStatusRequestObject) (BoostStatusResponseObject, error) {
	res, p, err := s.actUnmoved(ctx, req.Id, func(a string) error { return ignoreResult(s.o.Interactions.Boost(ctx, a, req.Id)) })
	if p != nil {
		return BoostStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return BoostStatus200JSONResponse{res}, err
}

// VoteInPoll serves POST /api/v1/statuses/{id}/poll/votes.
func (s *Server) VoteInPoll(ctx context.Context, req VoteInPollRequestObject) (VoteInPollResponseObject, error) {
	var pollID string
	var choices []int
	if req.Body != nil {
		pollID, choices = knownID(req.Body.PollId), req.Body.Choices
	}
	res, p, err := s.actUnmoved(ctx, req.Id, func(a string) error {
		return ignoreResult(s.o.Interactions.VotePoll(ctx, a, req.Id, pollID, choices))
	})
	if p != nil {
		return VoteInPolldefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return VoteInPoll200JSONResponse{res}, err
}

// UnboostStatus serves DELETE /api/v1/statuses/{id}/boost.
func (s *Server) UnboostStatus(ctx context.Context, req UnboostStatusRequestObject) (UnboostStatusResponseObject, error) {
	res, p, err := s.act(ctx, req.Id, func(a string) error { return s.o.Interactions.Unboost(ctx, a, req.Id) })
	if p != nil {
		return UnboostStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnboostStatus200JSONResponse{res}, err
}

// FavouriteStatus serves POST /api/v1/statuses/{id}/favourite.
func (s *Server) FavouriteStatus(ctx context.Context, req FavouriteStatusRequestObject) (FavouriteStatusResponseObject, error) {
	res, p, err := s.actUnmoved(ctx, req.Id, func(a string) error { return ignoreResult(s.o.Interactions.Favourite(ctx, a, req.Id)) })
	if p != nil {
		return FavouriteStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return FavouriteStatus200JSONResponse{res}, err
}

// UnfavouriteStatus serves DELETE /api/v1/statuses/{id}/favourite.
func (s *Server) UnfavouriteStatus(ctx context.Context, req UnfavouriteStatusRequestObject) (UnfavouriteStatusResponseObject, error) {
	res, p, err := s.act(ctx, req.Id, func(a string) error { return s.o.Interactions.Unfavourite(ctx, a, req.Id) })
	if p != nil {
		return UnfavouriteStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnfavouriteStatus200JSONResponse{res}, err
}

// BookmarkStatus serves POST /api/v1/statuses/{id}/bookmark.
func (s *Server) BookmarkStatus(ctx context.Context, req BookmarkStatusRequestObject) (BookmarkStatusResponseObject, error) {
	res, p, err := s.act(ctx, req.Id, func(a string) error { return s.o.Interactions.Bookmark(ctx, a, req.Id) })
	if p != nil {
		return BookmarkStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return BookmarkStatus200JSONResponse{res}, err
}

// UnbookmarkStatus serves DELETE /api/v1/statuses/{id}/bookmark.
func (s *Server) UnbookmarkStatus(ctx context.Context, req UnbookmarkStatusRequestObject) (UnbookmarkStatusResponseObject, error) {
	res, p, err := s.act(ctx, req.Id, func(a string) error { return s.o.Interactions.Unbookmark(ctx, a, req.Id) })
	if p != nil {
		return UnbookmarkStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnbookmarkStatus200JSONResponse{res}, err
}

// PinStatus serves POST /api/v1/statuses/{id}/pin.
func (s *Server) PinStatus(ctx context.Context, req PinStatusRequestObject) (PinStatusResponseObject, error) {
	res, p, err := s.act(ctx, req.Id, func(a string) error { return s.o.Interactions.Pin(ctx, a, req.Id) })
	if p != nil {
		return PinStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return PinStatus200JSONResponse{res}, err
}

// UnpinStatus serves DELETE /api/v1/statuses/{id}/pin.
func (s *Server) UnpinStatus(ctx context.Context, req UnpinStatusRequestObject) (UnpinStatusResponseObject, error) {
	res, p, err := s.act(ctx, req.Id, func(a string) error { return s.o.Interactions.Unpin(ctx, a, req.Id) })
	if p != nil {
		return UnpinStatusdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnpinStatus200JSONResponse{res}, err
}

// SetStatusQuotePolicy serves PUT /api/v1/statuses/{id}/quote_policy.
func (s *Server) SetStatusQuotePolicy(ctx context.Context, req SetStatusQuotePolicyRequestObject) (SetStatusQuotePolicyResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	_, err = s.o.Statuses.SetQuotePolicy(ctx, v.AccountID, req.Id, content.QuotePolicy(req.Body.QuotePolicy))
	if err == nil {
		var out Status
		if out, err = s.viewStatus(ctx, req.Id); err == nil {
			return SetStatusQuotePolicy200JSONResponse{StatusAfterActionJSONResponse{Body: out, Headers: StatusAfterActionResponseHeaders{CacheControl: statusCache(ctx)}}}, nil
		}
	}
	if p, ok := s.authorProblem(ctx, v, req.Id, err); ok {
		return SetStatusQuotePolicydefaultApplicationProblemPlusJSONResponse(p), nil
	}
	return nil, err
}

// ListStatusQuotes serves GET /api/v1/statuses/{id}/quotes.
func (s *Server) ListStatusQuotes(ctx context.Context, req ListStatusQuotesRequestObject) (ListStatusQuotesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	limit := defaultLimit
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if !validCursor(req.Params.Cursor) {
		return ListStatusQuotesdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
	}
	page, err := s.o.StatusReader.Quotes(ctx, v.AccountID, req.Id, deref(req.Params.Cursor), limit)
	if err != nil {
		if p, ok := s.statusProblem(ctx, err); ok {
			return ListStatusQuotesdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	items, err := s.presentStatuses(ctx, page.Statuses)
	if err != nil {
		return nil, err
	}
	out := StatusPage{Items: items}
	if page.Next != "" {
		out.NextCursor = &page.Next
	}
	return ListStatusQuotes200JSONResponse{StatusPageJSONResponse{Body: out, Headers: StatusPageResponseHeaders{CacheControl: statusCache(ctx)}}}, nil
}

// RevokeStatusQuote serves POST /api/v1/statuses/{id}/quotes/{quote_id}/revoke.
func (s *Server) RevokeStatusQuote(ctx context.Context, req RevokeStatusQuoteRequestObject) (RevokeStatusQuoteResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.o.Interactions.RevokeQuote(ctx, v.AccountID, req.Id, req.QuoteId); err != nil {
		if p, ok := s.authorProblem(ctx, v, req.Id, err); ok {
			return RevokeStatusQuotedefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	return RevokeStatusQuote204Response{}, nil
}
