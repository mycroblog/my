// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// Timelines reads the timelines (apptimeline.Service, FR-201 to FR-204).
type Timelines interface {
	Home(ctx context.Context, viewer appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error)
	Local(ctx context.Context, viewer appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error)
	Public(ctx context.Context, viewer appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error)
	Hashtag(ctx context.Context, viewer appcontent.Viewer, tagID string, q apptimeline.Query) (apptimeline.Page, error)
	Profile(ctx context.Context, viewer appcontent.Viewer, authorAccountID string, f apptimeline.ProfileFilter, q apptimeline.Query) (apptimeline.Page, error)
}

// TimelineReader reads the statuses that timelines list. *appcontent.Reader implements it.
type TimelineReader interface {
	// Views returns the statuses of ids that the viewer may see, in order.
	Views(ctx context.Context, viewer appcontent.Viewer, ids []string) ([]appcontent.StatusView, error)
	// Boosts returns the boosts of ids, in order, leaving out missing ones.
	Boosts(ctx context.Context, ids []string) ([]content.Boost, error)
	// Tag returns a hashtag by name, failing with appcontent.ErrNotFound when no status used it.
	Tag(ctx context.Context, name string) (content.Tag, error)
	// Pinned returns the statuses that accountID pins and the viewer may see, newest pin first.
	Pinned(ctx context.Context, viewer appcontent.Viewer, accountID string) ([]appcontent.StatusView, error)
}

// timelineViewer returns the viewer of the request in ctx for the content module, and the
// authenticated Viewer, nil for a visitor.
func timelineViewer(ctx context.Context) (appcontent.Viewer, *Viewer) {
	v, ok := ViewerFrom(ctx)
	if !ok {
		return contentViewer(ctx), nil
	}
	return contentViewer(ctx), &v
}

// statusItem is one item of a page of statuses: a post (BoostID "") or a boost of StatusID.
type statusItem struct {
	StatusID string
	BoostID  string
}

// statuses presents items in order as viewer sees them. An item whose status, boost, or
// booster is gone or hidden (since the page was read) is left out.
func (s *Server) statuses(ctx context.Context, viewer appcontent.Viewer, items []statusItem) ([]Status, error) {
	var statusIDs, boostIDs []string
	for _, it := range items {
		statusIDs = append(statusIDs, it.StatusID)
		if it.BoostID != "" {
			boostIDs = append(boostIDs, it.BoostID)
		}
	}
	views, err := s.o.TimelineReader.Views(ctx, viewer, statusIDs)
	if err != nil {
		return nil, err
	}
	boosts := map[string]content.Boost{}
	// The boosters, read once each; nil for one that no longer exists.
	boosters := map[string]*appaccount.Actor{}
	var found []appaccount.Actor
	if len(boostIDs) > 0 {
		bs, err := s.o.TimelineReader.Boosts(ctx, boostIDs)
		if err != nil {
			return nil, err
		}
		for _, b := range bs {
			boosts[b.ID] = b
			if _, ok := boosters[b.BoosterAccountID]; ok {
				continue
			}
			a, err := s.o.Accounts.FindByID(ctx, b.BoosterAccountID)
			switch {
			case err == nil:
				boosters[b.BoosterAccountID] = &a
				found = append(found, a)
			case errors.Is(err, appaccount.ErrNotFound):
				boosters[b.BoosterAccountID] = nil
			default:
				return nil, err
			}
		}
	}
	parts, err := s.statusParts(ctx, views, found...)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Status, len(views))
	for _, v := range views {
		byID[v.Status.ID] = s.newStatus(v, parts)
	}
	out := make([]Status, 0, len(items))
	for _, it := range items {
		st, ok := byID[it.StatusID]
		if !ok {
			continue
		}
		if it.BoostID == "" {
			out = append(out, st)
			continue
		}
		b, ok := boosts[it.BoostID]
		if !ok {
			continue
		}
		booster := boosters[b.BoosterAccountID]
		if booster == nil {
			continue
		}
		boosted := st
		out = append(out, Status{
			Id: b.ID, Uri: b.URI, CreatedAt: b.BoostedAt, Account: s.newAccount(*booster, parts),
			Visibility: st.Visibility, Language: st.Language, Boost: &boosted,
			MediaAttachments: []MediaAttachment{}, Mentions: []StatusMention{}, Tags: []StatusTag{}, Emojis: []CustomEmoji{},
			Reactions: []StatusReaction{},
		})
	}
	return out, nil
}

// timelineQuery is the query of a page; languages applies the viewer's timeline languages
// (FR-208).
func (s *Server) timelineQuery(ctx context.Context, v *Viewer, cursor *Cursor, limit *Limit, languages bool) (apptimeline.Query, error) {
	var q apptimeline.Query
	if cursor != nil {
		q.Before = *cursor
	}
	if limit != nil {
		q.Limit = *limit
	}
	if languages && v != nil && s.o.TimelineLanguages != nil {
		langs, err := s.o.TimelineLanguages(ctx, v.UserID)
		if err != nil {
			return q, fmt.Errorf("clientapi: the timeline languages of %s: %w", v.UserID, err)
		}
		q.Languages = langs
	}
	return q, nil
}

// statusPage presents a page of a timeline. An anonymous page is the same for every visitor,
// so shared caches may keep it briefly (ADR-0016); a viewer's page is not stored.
func (s *Server) statusPage(ctx context.Context, v *Viewer, items []statusItem, next string) (StatusPageJSONResponse, error) {
	cache := cachePublic
	if v != nil {
		cache = cacheNoStore
	}
	statuses, err := s.statuses(ctx, contentViewer(ctx), items)
	if err != nil {
		return StatusPageJSONResponse{}, err
	}
	body := StatusPage{Items: statuses}
	if next != "" {
		body.NextCursor = &next
	}
	return StatusPageJSONResponse{Body: body, Headers: StatusPageResponseHeaders{CacheControl: &cache}}, nil
}

// entryItems returns the items of the entries of a timeline page.
func entryItems(entries []apptimeline.Entry) []statusItem {
	items := make([]statusItem, 0, len(entries))
	for _, e := range entries {
		it := statusItem{StatusID: e.StatusID}
		if e.Kind == timeline.KindBoost {
			it.BoostID = e.ID
		}
		items = append(items, it)
	}
	return items
}

// GetHomeTimeline serves GET /api/v1/timelines/home.
func (s *Server) GetHomeTimeline(ctx context.Context, req GetHomeTimelineRequestObject) (GetHomeTimelineResponseObject, error) {
	cv, v := timelineViewer(ctx)
	if v == nil {
		return nil, errNoViewer
	}
	if p := s.plainPageProblem(ctx, req.Params.Cursor, req.Params.Limit); p != nil {
		return GetHomeTimelinedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q, err := s.timelineQuery(ctx, v, req.Params.Cursor, req.Params.Limit, true)
	if err != nil {
		return nil, err
	}
	p, err := s.o.Timelines.Home(ctx, cv, q)
	if err != nil {
		return nil, err
	}
	res, err := s.statusPage(ctx, v, entryItems(p.Entries), p.Next)
	return GetHomeTimeline200JSONResponse{res}, err
}

// GetLocalTimeline serves GET /api/v1/timelines/local. Visitors who are not logged in see only
// public posts; local posts need a logged-in viewer (ADR-0017). It is a 404 while the instance
// setting local_timeline_visibility is off, and a 401 for anonymous requests while it is signed_in
// (FR-202).
func (s *Server) GetLocalTimeline(ctx context.Context, req GetLocalTimelineRequestObject) (GetLocalTimelineResponseObject, error) {
	denied, err := s.timelineDenied(ctx, instance.Settings.LocalTimelineVisibility)
	if err != nil {
		return nil, err
	} else if denied != nil {
		return GetLocalTimelinedefaultApplicationProblemPlusJSONResponse(*denied), nil
	}
	cv, v := timelineViewer(ctx)
	if p := s.plainPageProblem(ctx, req.Params.Cursor, req.Params.Limit); p != nil {
		return GetLocalTimelinedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q, err := s.timelineQuery(ctx, v, req.Params.Cursor, req.Params.Limit, true)
	if err != nil {
		return nil, err
	}
	p, err := s.o.Timelines.Local(ctx, cv, q)
	if err != nil {
		return nil, err
	}
	res, err := s.statusPage(ctx, v, entryItems(p.Entries), p.Next)
	return GetLocalTimeline200JSONResponse{res}, err
}

// timelineDenied returns the problem that refuses a timeline to the request under the instance
// setting access (FR-202): a 404 while it is off, and a 401 for an anonymous request while it is
// signed_in. It returns nil when the request may read the timeline.
func (s *Server) timelineDenied(ctx context.Context, access func(instance.Settings) instance.Audience) (*problem, error) {
	var settings instance.Settings
	if s.o.Instance != nil {
		var err error
		if settings, err = s.o.Instance.Settings(ctx); err != nil {
			return nil, fmt.Errorf("clientapi: instance settings: %w", err)
		}
	}
	_, loggedIn := ViewerFrom(ctx)
	switch access(settings) {
	case instance.AudienceEveryone:
		return nil, nil
	case instance.AudienceSignedIn:
		if loggedIn {
			return nil, nil
		}
		body, headers := s.NewProblem(ctx, http.StatusUnauthorized, ProblemTypeUnauthenticated, s.Localizer(ctx).ServerErrorsUnauthenticated(), "")
		challenge := "Bearer"
		headers.WWWAuthenticate = &challenge
		return &problem{Body: body, Headers: headers, StatusCode: http.StatusUnauthorized}, nil
	default:
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
}

// GetPublicTimeline serves GET /api/v1/timelines/public: the public posts of local and remote
// accounts. It is a 404 while the instance setting federated_timeline_visibility is off, and a 401
// for anonymous requests while it is signed_in (FR-202).
func (s *Server) GetPublicTimeline(ctx context.Context, req GetPublicTimelineRequestObject) (GetPublicTimelineResponseObject, error) {
	denied, err := s.timelineDenied(ctx, instance.Settings.FederatedTimelineVisibility)
	if err != nil {
		return nil, err
	} else if denied != nil {
		return GetPublicTimelinedefaultApplicationProblemPlusJSONResponse(*denied), nil
	}
	cv, v := timelineViewer(ctx)
	if p := s.plainPageProblem(ctx, req.Params.Cursor, req.Params.Limit); p != nil {
		return GetPublicTimelinedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q, err := s.timelineQuery(ctx, v, req.Params.Cursor, req.Params.Limit, true)
	if err != nil {
		return nil, err
	}
	p, err := s.o.Timelines.Public(ctx, cv, q)
	if err != nil {
		return nil, err
	}
	res, err := s.statusPage(ctx, v, entryItems(p.Entries), p.Next)
	return GetPublicTimeline200JSONResponse{res}, err
}

// GetHashtagTimeline serves GET /api/v1/timelines/tag/{hashtag}. A hashtag that no status used
// has an empty timeline.
func (s *Server) GetHashtagTimeline(ctx context.Context, req GetHashtagTimelineRequestObject) (GetHashtagTimelineResponseObject, error) {
	cv, v := timelineViewer(ctx)
	if p := s.plainPageProblem(ctx, req.Params.Cursor, req.Params.Limit); p != nil {
		return GetHashtagTimelinedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q, err := s.timelineQuery(ctx, v, req.Params.Cursor, req.Params.Limit, true)
	if err != nil {
		return nil, err
	}
	var p apptimeline.Page
	tag, err := s.o.TimelineReader.Tag(ctx, req.Hashtag)
	switch {
	case err == nil:
		if p, err = s.o.Timelines.Hashtag(ctx, cv, tag.ID, q); err != nil {
			return nil, err
		}
	case !errors.Is(err, appcontent.ErrNotFound):
		return nil, err
	}
	res, err := s.statusPage(ctx, v, entryItems(p.Entries), p.Next)
	return GetHashtagTimeline200JSONResponse{res}, err
}

// ListAccountStatuses serves GET /api/v1/accounts/{id}/statuses: the account's profile
// timeline, or its pinned statuses in one page. An account that getAccount answers with a 404
// (unknown, not active, hidden by moderation, the instance actor, or blocking the viewer) is a
// 404 here too. The viewer's
// timeline languages do not apply: a profile shows the account in every language. The first
// page of a logged-in viewer may start fetching the recent posts of a remote account
// (Options.Outboxes, ADR-0044).
func (s *Server) ListAccountStatuses(ctx context.Context, req ListAccountStatusesRequestObject) (ListAccountStatusesResponseObject, error) {
	cv, v := timelineViewer(ctx)
	if p := s.plainPageProblem(ctx, req.Params.Cursor, req.Params.Limit); p != nil {
		return ListAccountStatusesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	account, ok, err := s.visibleAccount(ctx, v, req.Id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListAccountStatusesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	params := req.Params
	if params.Pinned != nil && *params.Pinned {
		cache := cachePublic
		if v != nil {
			cache = cacheNoStore
		}
		body := StatusPage{Items: []Status{}}
		// Every pinned status fits in the first page, so a cursor asks for an empty one.
		if params.Cursor == nil {
			views, err := s.o.TimelineReader.Pinned(ctx, cv, req.Id)
			if err != nil {
				return nil, err
			}
			if body.Items, err = s.presentStatuses(ctx, views); err != nil {
				return nil, err
			}
		}
		return ListAccountStatuses200JSONResponse{AccountStatusPageJSONResponse{
			Body: AccountStatusPage{Items: body.Items}, Headers: AccountStatusPageResponseHeaders{CacheControl: &cache},
		}}, nil
	}
	q, err := s.timelineQuery(ctx, v, params.Cursor, params.Limit, false)
	if err != nil {
		return nil, err
	}
	f := apptimeline.ProfileFilter{
		ExcludeReplies: params.ExcludeReplies != nil && *params.ExcludeReplies,
		ExcludeBoosts:  params.ExcludeBoosts != nil && *params.ExcludeBoosts,
	}
	p, err := s.o.Timelines.Profile(ctx, cv, req.Id, f, q)
	if err != nil {
		return nil, err
	}
	res, err := s.statusPage(ctx, v, entryItems(p.Entries), p.Next)
	if err != nil {
		return nil, err
	}
	out := AccountStatusPage{Items: res.Body.Items, NextCursor: res.Body.NextCursor}
	if params.Cursor == nil && v != nil && s.o.Outboxes != nil && !account.Account.IsLocal() {
		// The page shows what is stored now; the fetched posts show when it is read again.
		started, err := s.o.Outboxes.FetchOutbox(ctx, v.AccountID, req.Id)
		if err != nil {
			s.o.Logger.WarnContext(ctx, "fetch remote outbox", slog.String("account_id", req.Id), slog.Any("error", err))
		}
		if started {
			out.RemoteFetchStarted = &started
		}
	}
	return ListAccountStatuses200JSONResponse{AccountStatusPageJSONResponse{
		Body: out, Headers: AccountStatusPageResponseHeaders{CacheControl: res.Headers.CacheControl},
	}}, nil
}
