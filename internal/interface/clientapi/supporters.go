// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// The users' own subscriptions to the supporter plans (ADR-0072 §6, §8, §18; tag supporters,
// scopes read:supporters and write:supporters) and the supporter list (ADR-0071 §4): the
// members' choice to be listed (/preferences/supporter_listing) and the public list
// (/instance/supporters), which the instance setting supporter_list_visibility opens.

// The problem types of the subscriptions.
const (
	ProblemTypeSupporterIneligible        = "/api/v1/problems/supporter-ineligible"
	ProblemTypeSupporterSubscribed        = "/api/v1/problems/supporter-subscribed"
	ProblemTypeSupporterPlanUnavailable   = "/api/v1/problems/supporter-plan-unavailable"
	ProblemTypeSupporterSubscriptionState = "/api/v1/problems/supporter-subscription-state"
	ProblemTypeSupporterSamePlan          = "/api/v1/problems/supporter-same-plan"
)

// supporterReturnPath is the screen of the web client that the provider's checkout and portal
// return to; the query tells the screen what returned.
const supporterReturnPath = "/settings/supporter"

// supporterListRobots is the X-Robots-Tag of the supporter list, whatever the instance states
// to search engines (ADR-0071 §4, N1).
const supporterListRobots = "noindex, noarchive"

// OwnSupporters runs the users' own subscriptions and the supporter list:
// *appsupporters.Service implements it.
type OwnSupporters interface {
	OwnSubscription(ctx context.Context, userID string) (appsupporters.SubscriptionView, error)
	Eligibility(ctx context.Context, userID string) (appsupporters.Eligibility, error)
	StartCheckout(ctx context.Context, in appsupporters.CheckoutInput) (appsupporters.StartedCheckout, error)
	Returned(ctx context.Context, userID string) (appsupporters.SubscriptionView, error)
	ChangePlan(ctx context.Context, userID, planID string, interval supporters.Interval) (appsupporters.SubscriptionView, error)
	Cancel(ctx context.Context, userID string) (appsupporters.SubscriptionView, error)
	Resume(ctx context.Context, userID string) (appsupporters.SubscriptionView, error)
	CancelScheduledChange(ctx context.Context, userID string) (appsupporters.SubscriptionView, error)
	PortalURL(ctx context.Context, userID, returnURL string) (string, error)
	Listed(ctx context.Context, userID string) (bool, error)
	SetListed(ctx context.Context, userID string, listed bool) (bool, error)
	SupporterList(ctx context.Context, f appsupporters.ListingFilter) (appsupporters.SupporterList, error)
	// OwnSubscriptionsByID returns the user's subscriptions among ids, which the supporter
	// notifications present.
	OwnSubscriptionsByID(ctx context.Context, userID string, ids []string) ([]appsupporters.SubscriptionView, error)
}

// ownSupporters returns the use cases, or the 404 of a server without them.
func (s *Server) ownSupporters(ctx context.Context) (OwnSupporters, *problem) {
	if s.o.OwnSupporters == nil {
		p := s.notFoundResponse(ctx)
		return nil, &p
	}
	return s.o.OwnSupporters, nil
}

// webOrigin is the origin of the web client (Options.WebOrigin).
func (s *Server) webOrigin() string {
	if s.o.WebOrigin != "" {
		return s.o.WebOrigin
	}
	return s.origin()
}

// ownSupporterProblem maps the errors of the users' subscriptions; it reports false for any
// other error.
func (s *Server) ownSupporterProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	var ie *appsupporters.IneligibleError
	switch {
	case errors.Is(err, appsupporters.ErrDisabled), errors.Is(err, appsupporters.ErrNotMember):
		return s.notFoundResponse(ctx), true
	case errors.As(err, &ie):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeSupporterIneligible, l.ServerErrorsSupportersIneligible(),
			l.ServerErrorsSupportersIneligibleDetail(string(ie.Reason))), true
	case errors.Is(err, appsupporters.ErrLiveSubscription):
		return s.problem(ctx, http.StatusConflict, ProblemTypeSupporterSubscribed, l.ServerErrorsSupportersSubscribed(),
			l.ServerErrorsSupportersSubscribedDetail()), true
	case errors.Is(err, appsupporters.ErrPlanUnavailable), errors.Is(err, appsupporters.ErrInvalidPlan):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeSupporterPlanUnavailable, l.ServerErrorsSupportersPlanUnavailable(), ""), true
	case errors.Is(err, appsupporters.ErrSubscriptionState):
		return s.problem(ctx, http.StatusConflict, ProblemTypeSupporterSubscriptionState, l.ServerErrorsSupportersSubscriptionState(), ""), true
	case errors.Is(err, appsupporters.ErrServiceClosing):
		return s.serviceClosingProblem(ctx), true
	case errors.Is(err, appsupporters.ErrSameChoice):
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeSupporterSamePlan, l.ServerErrorsSupportersSamePlan(), ""), true
	}
	return s.supporterProblem(ctx, err)
}

func utcPtr(t time.Time) *time.Time {
	u := t.UTC()
	return &u
}

// newSupporterSubscription presents a user's own subscription.
func newSupporterSubscription(v appsupporters.SubscriptionView) SupporterSubscription {
	cur := v.Current
	out := SupporterSubscription{
		Id: v.ID, State: SupporterSubscriptionState(cur.State), PlanId: cur.PlanID, PlanName: v.Plan.Revision.Name,
		Interval: SupporterInterval(v.Price.Interval), Price: minorMoney(v.Price.AmountMinor, v.Price.Currency),
		StartedAt: v.CreatedAt.UTC(),
	}
	if !cur.PeriodEndsAt.IsZero() {
		out.PeriodEndsAt = utcPtr(cur.PeriodEndsAt)
	}
	if cur.Grace != nil {
		out.GraceEndsAt = utcPtr(cur.Grace.EndsAt)
	}
	if r := supporters.EndReasonOf(cur.State, cur.Reason); r != "" {
		reason := SupporterEndReason(r)
		out.EndReason = &reason
	}
	if sp, plan := v.ScheduledPrice, v.ScheduledPlan; sp != nil && plan != nil && cur.Scheduled != nil {
		out.ScheduledChange = &SupporterScheduledChange{
			PlanId: sp.PlanID, PlanName: plan.Revision.Name, Interval: SupporterInterval(sp.Interval),
			Price: minorMoney(sp.AmountMinor, sp.Currency), EffectiveAt: cur.Scheduled.EffectiveAt.UTC(),
		}
	}
	return out
}

// ownSupporterSubscription answers GET /supporter/subscription and its sync: the subscription
// that read returns (none for ErrNoLiveSubscription) and why the viewer may not subscribe.
func (s *Server) ownSupporterSubscription(ctx context.Context, read func(OwnSupporters, context.Context, string) (appsupporters.SubscriptionView, error)) (OwnSupporterSubscription, *problem, error) {
	sup, p := s.ownSupporters(ctx)
	if p != nil {
		return OwnSupporterSubscription{}, p, nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return OwnSupporterSubscription{}, nil, err
	}
	var out OwnSupporterSubscription
	sv, err := read(sup, ctx, v.UserID)
	switch {
	case err == nil:
		sub := newSupporterSubscription(sv)
		out.Subscription = &sub
	case errors.Is(err, appsupporters.ErrNoLiveSubscription):
	default:
		if pr, ok := s.ownSupporterProblem(ctx, err); ok {
			return OwnSupporterSubscription{}, &pr, nil
		}
		return OwnSupporterSubscription{}, nil, fmt.Errorf("clientapi: the subscription of %s: %w", v.UserID, err)
	}
	reason, err := sup.Eligibility(ctx, v.UserID)
	if err != nil {
		return OwnSupporterSubscription{}, nil, fmt.Errorf("clientapi: the eligibility of %s: %w", v.UserID, err)
	}
	if reason != "" {
		r := SupporterIneligibleReason(reason)
		out.IneligibleReason = &r
	}
	return out, nil, nil
}

// GetOwnSupporterSubscription serves GET /api/v1/supporter/subscription.
func (s *Server) GetOwnSupporterSubscription(ctx context.Context, _ GetOwnSupporterSubscriptionRequestObject) (GetOwnSupporterSubscriptionResponseObject, error) {
	out, p, err := s.ownSupporterSubscription(ctx, OwnSupporters.OwnSubscription)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return GetOwnSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return GetOwnSupporterSubscription200JSONResponse{Body: out, Headers: GetOwnSupporterSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// SyncOwnSupporterSubscription serves POST /api/v1/supporter/subscription/sync.
func (s *Server) SyncOwnSupporterSubscription(ctx context.Context, _ SyncOwnSupporterSubscriptionRequestObject) (SyncOwnSupporterSubscriptionResponseObject, error) {
	out, p, err := s.ownSupporterSubscription(ctx, OwnSupporters.Returned)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return SyncOwnSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return SyncOwnSupporterSubscription200JSONResponse{Body: out, Headers: SyncOwnSupporterSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// StartSupporterCheckout serves POST /api/v1/supporter/checkout.
func (s *Server) StartSupporterCheckout(ctx context.Context, req StartSupporterCheckoutRequestObject) (StartSupporterCheckoutResponseObject, error) {
	sup, p := s.ownSupporters(ctx)
	if p != nil {
		return StartSupporterCheckoutdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || !req.Body.Interval.Valid() {
		return StartSupporterCheckoutdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "interval")), nil
	}
	back := s.webOrigin() + supporterReturnPath
	co, err := sup.StartCheckout(ctx, appsupporters.CheckoutInput{
		UserID: v.UserID, PlanID: knownID(req.Body.PlanId), Interval: supporters.Interval(req.Body.Interval),
		SuccessURL: back + "?checkout=success", CancelURL: back + "?checkout=canceled",
	})
	if pr, ok := s.ownSupporterProblem(ctx, err); ok {
		return StartSupporterCheckoutdefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: start the checkout of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return StartSupporterCheckout200JSONResponse{
		Body:    SupporterCheckout{SubscriptionId: co.SubscriptionID, RedirectUrl: co.RedirectURL},
		Headers: StartSupporterCheckout200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// changeOwn runs an operation on the viewer's subscription and presents the result.
func (s *Server) changeOwn(ctx context.Context, op func(OwnSupporters, string) (appsupporters.SubscriptionView, error)) (SupporterSubscription, *problem, error) {
	sup, p := s.ownSupporters(ctx)
	if p != nil {
		return SupporterSubscription{}, p, nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return SupporterSubscription{}, nil, err
	}
	sv, err := op(sup, v.UserID)
	if errors.Is(err, appsupporters.ErrNoLiveSubscription) {
		err = appsupporters.ErrSubscriptionState
	}
	if pr, ok := s.ownSupporterProblem(ctx, err); ok {
		return SupporterSubscription{}, &pr, nil
	}
	if err != nil {
		return SupporterSubscription{}, nil, fmt.Errorf("clientapi: the subscription of %s: %w", v.UserID, err)
	}
	return newSupporterSubscription(sv), nil, nil
}

// ChangeOwnSupporterSubscription serves POST /api/v1/supporter/subscription/change.
func (s *Server) ChangeOwnSupporterSubscription(ctx context.Context, req ChangeOwnSupporterSubscriptionRequestObject) (ChangeOwnSupporterSubscriptionResponseObject, error) {
	if req.Body == nil || !req.Body.Interval.Valid() {
		return ChangeOwnSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "interval")), nil
	}
	out, p, err := s.changeOwn(ctx, func(sup OwnSupporters, userID string) (appsupporters.SubscriptionView, error) {
		return sup.ChangePlan(ctx, userID, knownID(req.Body.PlanId), supporters.Interval(req.Body.Interval))
	})
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return ChangeOwnSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return ChangeOwnSupporterSubscription200JSONResponse{Body: out, Headers: ChangeOwnSupporterSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// CancelOwnSupporterSubscription serves POST /api/v1/supporter/subscription/cancel.
func (s *Server) CancelOwnSupporterSubscription(ctx context.Context, _ CancelOwnSupporterSubscriptionRequestObject) (CancelOwnSupporterSubscriptionResponseObject, error) {
	out, p, err := s.changeOwn(ctx, func(sup OwnSupporters, userID string) (appsupporters.SubscriptionView, error) {
		return sup.Cancel(ctx, userID)
	})
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return CancelOwnSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return CancelOwnSupporterSubscription200JSONResponse{Body: out, Headers: CancelOwnSupporterSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// ResumeOwnSupporterSubscription serves POST /api/v1/supporter/subscription/resume.
func (s *Server) ResumeOwnSupporterSubscription(ctx context.Context, _ ResumeOwnSupporterSubscriptionRequestObject) (ResumeOwnSupporterSubscriptionResponseObject, error) {
	out, p, err := s.changeOwn(ctx, func(sup OwnSupporters, userID string) (appsupporters.SubscriptionView, error) {
		return sup.Resume(ctx, userID)
	})
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return ResumeOwnSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return ResumeOwnSupporterSubscription200JSONResponse{Body: out, Headers: ResumeOwnSupporterSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// CancelOwnSupporterScheduledChange serves POST
// /api/v1/supporter/subscription/cancel_scheduled_change.
func (s *Server) CancelOwnSupporterScheduledChange(ctx context.Context, _ CancelOwnSupporterScheduledChangeRequestObject) (CancelOwnSupporterScheduledChangeResponseObject, error) {
	out, p, err := s.changeOwn(ctx, func(sup OwnSupporters, userID string) (appsupporters.SubscriptionView, error) {
		return sup.CancelScheduledChange(ctx, userID)
	})
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return CancelOwnSupporterScheduledChangedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return CancelOwnSupporterScheduledChange200JSONResponse{Body: out, Headers: CancelOwnSupporterScheduledChange200ResponseHeaders{CacheControl: &cache}}, nil
}

// OpenSupporterPortal serves POST /api/v1/supporter/portal.
func (s *Server) OpenSupporterPortal(ctx context.Context, _ OpenSupporterPortalRequestObject) (OpenSupporterPortalResponseObject, error) {
	sup, p := s.ownSupporters(ctx)
	if p != nil {
		return OpenSupporterPortaldefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	url, err := sup.PortalURL(ctx, v.UserID, s.webOrigin()+supporterReturnPath+"?portal=returned")
	if pr, ok := s.ownSupporterProblem(ctx, err); ok {
		return OpenSupporterPortaldefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the portal of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return OpenSupporterPortal200JSONResponse{Body: SupporterPortal{Url: url}, Headers: OpenSupporterPortal200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetSupporterListing serves GET /api/v1/preferences/supporter_listing.
func (s *Server) GetSupporterListing(ctx context.Context, _ GetSupporterListingRequestObject) (GetSupporterListingResponseObject, error) {
	sup, p := s.ownSupporters(ctx)
	if p != nil {
		return GetSupporterListingdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	listed, err := sup.Listed(ctx, v.UserID)
	if pr, ok := s.ownSupporterProblem(ctx, err); ok {
		return GetSupporterListingdefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the supporter listing of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return GetSupporterListing200JSONResponse{Body: SupporterListing{Listed: listed}, Headers: GetSupporterListing200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdateSupporterListing serves PUT /api/v1/preferences/supporter_listing.
func (s *Server) UpdateSupporterListing(ctx context.Context, req UpdateSupporterListingRequestObject) (UpdateSupporterListingResponseObject, error) {
	sup, p := s.ownSupporters(ctx)
	if p != nil {
		return UpdateSupporterListingdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return UpdateSupporterListingdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	listed, err := sup.SetListed(ctx, v.UserID, req.Body.Listed)
	if pr, ok := s.ownSupporterProblem(ctx, err); ok {
		return UpdateSupporterListingdefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: set the supporter listing of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return UpdateSupporterListing200JSONResponse{Body: SupporterListing{Listed: listed}, Headers: UpdateSupporterListing200ResponseHeaders{CacheControl: &cache}}, nil
}

// supporterCursor is the position of a member in the supporter list, the cursor of its pages.
type supporterCursor struct {
	UserID string `json:"u"`
	Rank   int    `json:"r"`
	At     int64  `json:"t"`
}

func encodeSupporterCursor(l appsupporters.Listing) *Cursor {
	b, err := json.Marshal(supporterCursor{UserID: l.UserID, Rank: l.Rank, At: l.ListedAt.UnixMicro()})
	if err != nil {
		panic(err) // a struct of strings and numbers always marshals
	}
	c := base64.RawURLEncoding.EncodeToString(b)
	return &c
}

func decodeSupporterCursor(c string) (*appsupporters.Listing, bool) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return nil, false
	}
	var sc supporterCursor
	if err := json.Unmarshal(b, &sc); err != nil || !kernel.ValidID(sc.UserID) {
		return nil, false
	}
	return &appsupporters.Listing{UserID: sc.UserID, Rank: sc.Rank, ListedAt: time.UnixMicro(sc.At).UTC()}, true
}

// ListInstanceSupporters serves GET /api/v1/instance/supporters: the members who chose to be
// listed, as the instance setting supporter_list_visibility allows (ADR-0071 §4).
func (s *Server) ListInstanceSupporters(ctx context.Context, req ListInstanceSupportersRequestObject) (ListInstanceSupportersResponseObject, error) {
	var settings instance.Settings
	if s.o.Instance != nil {
		var err error
		if settings, err = s.o.Instance.Settings(ctx); err != nil {
			return nil, fmt.Errorf("clientapi: instance settings: %w", err)
		}
	}
	viewer := viewerOf(ctx)
	switch settings.SupporterListVisibility() {
	case instance.SupporterListPublic:
	case instance.SupporterListSignedIn:
		if viewer == nil {
			body, headers := s.NewProblem(ctx, http.StatusUnauthorized, ProblemTypeUnauthenticated, s.Localizer(ctx).ServerErrorsUnauthenticated(), "")
			challenge := "Bearer"
			headers.WWWAuthenticate = &challenge
			return ListInstanceSupportersdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusUnauthorized}, nil
		}
	default:
		return ListInstanceSupportersdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	sup, p := s.ownSupporters(ctx)
	if p != nil {
		return ListInstanceSupportersdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	n, ok := pageLimit(req.Params.Limit)
	if !ok {
		return ListInstanceSupportersdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "limit")), nil
	}
	f := appsupporters.ListingFilter{Limit: n}
	if req.Params.Cursor != nil {
		if f.After, ok = decodeSupporterCursor(*req.Params.Cursor); !ok {
			return ListInstanceSupportersdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
	}
	if req.Params.PlanId != nil {
		f.PlanID = knownID(*req.Params.PlanId)
	}
	list, err := sup.SupporterList(ctx, f)
	if pr, ok := s.ownSupporterProblem(ctx, err); ok {
		return ListInstanceSupportersdefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: the supporter list: %w", err)
	}
	page := SupporterListPage{Plans: make([]SupporterListPlan, 0, len(list.Plans)), Items: []SupporterListEntry{}}
	for _, pl := range list.Plans {
		page.Plans = append(page.Plans, SupporterListPlan{Id: pl.ID, Name: pl.Revision.Name})
	}
	var actors []appaccount.Actor
	var planIDs []string
	for _, m := range list.Members {
		if m.AccountID == "" {
			continue
		}
		a, ok, err := s.visibleAccount(ctx, viewer, m.AccountID)
		if err != nil {
			return nil, err
		}
		if ok {
			actors, planIDs = append(actors, a), append(planIDs, m.PlanID)
		}
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	for i, a := range accounts {
		page.Items = append(page.Items, SupporterListEntry{PlanId: planIDs[i], Account: a})
	}
	if list.Last != nil {
		page.NextCursor = encodeSupporterCursor(*list.Last)
	}
	// The list of the signed-in users is for them only; the public list varies by the token's
	// locale like any anonymous operation (client-api.md §2.4).
	cache, robots := cachePublic, supporterListRobots
	switch {
	case settings.SupporterListVisibility() == instance.SupporterListSignedIn:
		cache = cachePrivateNoStore
	case viewer != nil:
		cache = cachePrivate
	}
	return ListInstanceSupporters200JSONResponse{
		Body: page, Headers: ListInstanceSupporters200ResponseHeaders{CacheControl: &cache, XRobotsTag: &robots},
	}, nil
}
