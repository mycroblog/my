// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// Notifications runs the use cases of the viewer's own notifications (FR-205).
// *appnotification.Service implements it.
type Notifications interface {
	List(ctx context.Context, accountID string, q appnotification.Query) ([]notification.Notification, error)
	ReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error)
	UnreadCount(ctx context.Context, accountID string) (int64, error)
	MarkRead(ctx context.Context, accountID, lastReadID string) (notification.ReadMarker, error)
	Dismiss(ctx context.Context, accountID, notificationID string) error
}

// Severances reads the follows that a local account lost to a suspension (FR-717).
// *appmoderation.Service implements it.
type Severances interface {
	SeveranceOf(ctx context.Context, accountID, id string) (moderation.SeveranceRecord, error)
}

// Page sizes of collections: the default and maximum of the spec's Limit parameter.
const (
	defaultPageLimit = 20
	maxPageLimit     = 40
)

// pageLimit returns the page size that the limit parameter asks for, or false when it is out of
// the spec's range (the generated code does not check it).
func pageLimit(limit *Limit) (int, bool) {
	if limit == nil {
		return defaultPageLimit, true
	}
	return *limit, *limit >= 1 && *limit <= maxPageLimit
}

// invalidParameter returns the 400 problem of a parameter whose value the handler rejects.
func (s *Server) invalidParameter(ctx context.Context, name string) (Problem, ProblemResponseHeaders) {
	l := s.Localizer(ctx)
	return s.NewProblem(ctx, http.StatusBadRequest, ProblemTypeBlank, l.ServerErrorsBadRequest(), l.ServerErrorsInvalidParameter(name))
}

// notFound returns the 404 problem of a resource that does not exist or is not the viewer's.
func (s *Server) notFound(ctx context.Context) (Problem, ProblemResponseHeaders) {
	return s.NewProblem(ctx, http.StatusNotFound, ProblemTypeBlank, s.Localizer(ctx).ServerErrorsNotFound(), "")
}

// ListNotifications serves GET /api/v1/notifications: a page of the viewer's notifications,
// newest first. The cursor is the ID of the last notification of the previous page.
func (s *Server) ListNotifications(ctx context.Context, req ListNotificationsRequestObject) (ListNotificationsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	limit, ok := pageLimit(req.Params.Limit)
	if !ok {
		body, headers := s.invalidParameter(ctx, "limit")
		return ListNotificationsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusBadRequest}, nil
	}
	if !validCursor(req.Params.Cursor) {
		body, headers := s.invalidParameter(ctx, "cursor")
		return ListNotificationsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusBadRequest}, nil
	}
	q := appnotification.Query{Limit: limit + 1} // one more tells whether a next page exists
	if req.Params.Cursor != nil {
		q.Before = *req.Params.Cursor
	}
	if req.Params.Types != nil {
		for _, t := range *req.Params.Types {
			q.Types = append(q.Types, notification.Type(t))
		}
	}
	ns, err := s.o.Notifications.List(ctx, v.AccountID, q)
	if errors.Is(err, notification.ErrInvalidType) {
		body, headers := s.invalidParameter(ctx, "types")
		return ListNotificationsdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusBadRequest}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: list notifications of %s: %w", v.AccountID, err)
	}
	page := NotificationPage{Items: []Notification{}}
	if len(ns) > limit {
		ns = ns[:limit]
		next := ns[limit-1].ID
		page.NextCursor = &next
	}
	items, err := s.newNotifications(ctx, v, ns)
	if err != nil {
		return nil, err
	}
	page.Items = append(page.Items, items...)
	cache := cacheNoStore
	return ListNotifications200JSONResponse{Body: page, Headers: ListNotifications200ResponseHeaders{CacheControl: &cache}}, nil
}

// newNotifications presents accountID's notifications ns, in order, as the Notification schema.
// The posts that they are about are read together through the content module, which applies the
// visibility policy: a post that the user may not see is left out of its notification. The source
// accounts are read once each; a source account that no longer exists is left out. A moderation
// notice carries its action as the user sees it (moderation.go), and a report_resolved the
// user's report, which tells only that it is resolved. The account of an
// admin_sign_up is the local account of the user who signed up, and a relationships_severed
// carries the follows that the user lost, with the suspended account as its account.
func (s *Server) newNotifications(ctx context.Context, viewer Viewer, ns []notification.Notification) ([]Notification, error) {
	accountID := viewer.AccountID
	severances, err := s.notifiedSeverances(ctx, accountID, ns)
	if err != nil {
		return nil, err
	}
	moves, err := s.notifiedMoves(ctx, ns)
	if err != nil {
		return nil, err
	}
	presented, err := s.presentedAccounts(ctx, ns, severances, moves)
	if err != nil {
		return nil, err
	}
	var statusIDs []string
	seen := map[string]bool{}
	for _, n := range ns {
		if n.StatusID != "" && !seen[n.StatusID] {
			seen[n.StatusID] = true
			statusIDs = append(statusIDs, n.StatusID)
		}
	}
	var views []appcontent.StatusView
	if len(statusIDs) > 0 {
		var err error
		views, err = s.o.TimelineReader.Views(ctx, localContentViewer(viewer), statusIDs)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the statuses of the notifications of %s: %w", accountID, err)
		}
	}
	// The source accounts, read once each; nil for one that no longer exists.
	sources := map[string]*appaccount.Actor{}
	var found []appaccount.Actor
	for i, n := range ns {
		id := presented[i]
		if _, ok := sources[id]; ok || id == "" {
			continue
		}
		actor, err := s.o.Accounts.FindByID(ctx, id)
		switch {
		case err == nil:
			sources[id] = &actor
			found = append(found, actor)
		case errors.Is(err, appaccount.ErrNotFound):
			sources[id] = nil
		default:
			return nil, fmt.Errorf("clientapi: the account %s of notification %s: %w", id, n.ID, err)
		}
	}
	reports, err := s.notifiedReports(ctx, accountID, ns)
	if err != nil {
		return nil, err
	}
	var actionIDs []string
	for _, n := range ns {
		if n.ModerationActionID != "" {
			actionIDs = append(actionIDs, n.ModerationActionID)
		}
	}
	actions, err := s.noticeActions(ctx, accountID, actionIDs)
	if err != nil {
		return nil, err
	}
	appealed, err := s.appealActions(ctx, accountID, ns)
	if err != nil {
		return nil, err
	}
	emojiRequests, err := s.notifiedCustomEmojiRequests(ctx, ns)
	if err != nil {
		return nil, err
	}
	supporterSubs, err := s.notifiedSupporterSubscriptions(ctx, ns)
	if err != nil {
		return nil, err
	}
	parts, err := s.statusParts(ctx, views, found...)
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]Status, len(views))
	for _, v := range views {
		statuses[v.Status.ID] = s.newStatus(v, parts)
	}
	accounts := make(map[string]Account, len(found))
	for id, a := range sources {
		if a != nil {
			accounts[id] = s.newAccount(*a, parts)
		}
	}
	out := make([]Notification, 0, len(ns))
	for i, n := range ns {
		item := Notification{Id: n.ID, Type: NotificationType(n.Type), CreatedAt: n.CreatedAt}
		if st, ok := statuses[n.StatusID]; ok {
			item.Status = &st
		}
		if a, ok := accounts[presented[i]]; ok {
			item.Account = &a
		}
		if r, ok := reports[n.ReportID]; ok {
			item.Report = &r
		}
		if a, ok := actions[n.ModerationActionID]; ok {
			item.ModerationAction = &a
		}
		if a, ok := appealed[n.AppealID]; ok && n.Type == notification.TypeAppealDecided {
			item.ModerationAction = &a
		}
		if n.Type == notification.TypeAdminAppeal {
			id := n.AppealID
			item.AppealId = &id
		}
		if n.Type == notification.TypeAdminReport {
			id := n.ReportID
			item.ReportId = &id
		}
		if r, ok := emojiRequests[n.CustomEmojiRequestID]; ok {
			item.CustomEmojiRequest = &r
		}
		if sub, ok := supporterSubs[n.SupporterSubscriptionID]; ok {
			item.SupporterSubscription = &sub
		}
		if sv, ok := severances[n.SeveranceID]; ok {
			item.RelationshipSeverance = newRelationshipSeverance(sv)
		}
		if m, ok := moves[n.AccountMoveID]; ok {
			item.AccountMove = &m.view
			if n.AccountMoveOutcome != "" {
				outcome := AccountMoveOutcome(n.AccountMoveOutcome)
				item.AccountMove.Outcome = &outcome
			}
		}
		if item.Reaction, err = s.reactionEmojiOf(ctx, n); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// presentedAccounts returns, for each of ns, the ID of the account that its account property
// presents: the source account, for an admin_sign_up the local account of the user who signed
// up, for a relationships_severed the suspended account of severances, and for an
// admin_account_moved the account that moved; "" for none.
func (s *Server) presentedAccounts(ctx context.Context, ns []notification.Notification, severances map[string]moderation.SeveranceRecord, moves map[string]notifiedMove) ([]string, error) {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.SourceAccountID
		if sv, ok := severances[n.SeveranceID]; ok {
			out[i] = sv.TargetAccountID
		}
		if m, ok := moves[n.AccountMoveID]; ok && n.Type == notification.TypeAdminAccountMoved {
			out[i] = m.move.AccountID
		}
		if n.Type != notification.TypeAdminSignUp || n.SubjectUserID == "" || s.o.UserAccounts == nil {
			continue
		}
		la, err := s.o.UserAccounts.LocalAccountOfUser(ctx, n.SubjectUserID)
		switch {
		case err == nil:
			out[i] = la.AccountID
		case !errors.Is(err, appaccount.ErrNotFound):
			return nil, fmt.Errorf("clientapi: the account of user %s of notification %s: %w", n.SubjectUserID, n.ID, err)
		}
	}
	return out, nil
}

// notifiedReports presents, by ID, accountID's own reports that the report_resolved
// notifications ns tell of; a report that no longer exists is left out.
func (s *Server) notifiedReports(ctx context.Context, accountID string, ns []notification.Notification) (map[string]Report, error) {
	if s.o.FiledReports == nil {
		return nil, nil
	}
	var recs []moderation.ReportRecord
	seen := map[string]bool{}
	for _, n := range ns {
		if n.Type != notification.TypeReportResolved || n.ReportID == "" || seen[n.ReportID] {
			continue
		}
		seen[n.ReportID] = true
		rec, err := s.o.FiledReports.ReportFiledBy(ctx, accountID, n.ReportID)
		switch {
		case errors.Is(err, appmoderation.ErrNotFound):
			continue
		case err != nil:
			return nil, fmt.Errorf("clientapi: the report %s of notification %s: %w", n.ReportID, n.ID, err)
		}
		recs = append(recs, rec)
	}
	items, err := s.newReports(ctx, recs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Report, len(items))
	for _, r := range items {
		out[r.Id] = r
	}
	return out, nil
}

// notifiedCustomEmojiRequests presents, by ID, the custom emoji requests that the
// admin_custom_emoji_request and custom_emoji_request_decided notifications ns tell of, read
// together; one that no longer exists is left out.
func (s *Server) notifiedCustomEmojiRequests(ctx context.Context, ns []notification.Notification) (map[string]CustomEmojiRequest, error) {
	out := map[string]CustomEmojiRequest{}
	if s.o.CustomEmojiRequests == nil {
		return out, nil
	}
	var ids []string
	for _, n := range ns {
		if n.CustomEmojiRequestID != "" {
			ids = append(ids, n.CustomEmojiRequestID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	reqs, err := s.o.CustomEmojiRequests.RequestsByID(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the custom emoji requests of the notifications: %w", err)
	}
	for _, r := range reqs {
		out[r.Request.ID] = newCustomEmojiRequest(r)
	}
	return out, nil
}

// notifiedSupporterSubscriptions presents, by ID, the viewer's supporter subscriptions that the
// supporter notifications ns tell of, as they are now; one that no longer exists is left out.
func (s *Server) notifiedSupporterSubscriptions(ctx context.Context, ns []notification.Notification) (map[string]SupporterSubscription, error) {
	out := map[string]SupporterSubscription{}
	var ids []string
	for _, n := range ns {
		if n.SupporterSubscriptionID != "" {
			ids = append(ids, n.SupporterSubscriptionID)
		}
	}
	if len(ids) == 0 || s.o.OwnSupporters == nil {
		return out, nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return out, nil //nolint:nilerr // without a viewer there is no subscription of theirs to present
	}
	subs, err := s.o.OwnSupporters.OwnSubscriptionsByID(ctx, v.UserID, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the supporter subscriptions of the notifications: %w", err)
	}
	for _, sub := range subs {
		out[sub.ID] = newSupporterSubscription(sub)
	}
	return out, nil
}

// notifiedSeverances returns, by ID, accountID's severances that the relationships_severed
// notifications ns tell of; one that no longer exists is left out.
func (s *Server) notifiedSeverances(ctx context.Context, accountID string, ns []notification.Notification) (map[string]moderation.SeveranceRecord, error) {
	out := map[string]moderation.SeveranceRecord{}
	if s.o.Severances == nil {
		return out, nil
	}
	for _, n := range ns {
		if n.Type != notification.TypeRelationshipsSevered || n.SeveranceID == "" {
			continue
		}
		if _, ok := out[n.SeveranceID]; ok {
			continue
		}
		sv, err := s.o.Severances.SeveranceOf(ctx, accountID, n.SeveranceID)
		switch {
		case errors.Is(err, appmoderation.ErrNotFound):
			continue
		case err != nil:
			return nil, fmt.Errorf("clientapi: the severance %s of notification %s: %w", n.SeveranceID, n.ID, err)
		}
		out[n.SeveranceID] = sv
	}
	return out, nil
}

// notifiedMove is a move that a notification tells of, with its target presented.
type notifiedMove struct {
	move appaccount.Move
	view AccountMove
}

// notifiedMoves returns, by ID, the moves that the account_moved and admin_account_moved
// notifications ns tell of, each with its target; a move that no longer exists is left out.
func (s *Server) notifiedMoves(ctx context.Context, ns []notification.Notification) (map[string]notifiedMove, error) {
	out := map[string]notifiedMove{}
	if s.o.Moves == nil {
		return out, nil
	}
	for _, n := range ns {
		if n.AccountMoveID == "" {
			continue
		}
		if _, ok := out[n.AccountMoveID]; ok {
			continue
		}
		m, err := s.o.Moves.FindMove(ctx, n.AccountMoveID)
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
			continue
		case err != nil:
			return nil, fmt.Errorf("clientapi: the move %s of notification %s: %w", n.AccountMoveID, n.ID, err)
		}
		target, err := s.presentedTarget(ctx, m.TargetAccountID)
		if err != nil {
			return nil, err
		}
		out[n.AccountMoveID] = notifiedMove{move: m, view: AccountMove{Target: target}}
	}
	return out, nil
}

// newRelationshipSeverance presents sv as the RelationshipSeverance schema.
func newRelationshipSeverance(sv moderation.SeveranceRecord) *RelationshipSeverance {
	out := &RelationshipSeverance{
		Kind: RelationshipSeveranceKindAccountSuspension, FollowersLost: sv.FollowersLost, FollowingLost: sv.FollowingLost,
	}
	if sv.DomainActionID != "" {
		domain := sv.Domain
		out.Kind, out.Domain = RelationshipSeveranceKindDomainSuspension, &domain
	}
	return out
}

// GetNotificationMarker serves GET /api/v1/notifications/marker: the viewer's read marker and
// unread count.
func (s *Server) GetNotificationMarker(ctx context.Context, _ GetNotificationMarkerRequestObject) (GetNotificationMarkerResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	m, err := s.o.Notifications.ReadMarker(ctx, v.AccountID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the notification marker of %s: %w", v.AccountID, err)
	}
	body, err := s.newNotificationMarker(ctx, v.AccountID, m)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return GetNotificationMarker200JSONResponse{Body: body, Headers: GetNotificationMarker200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdateNotificationMarker serves PUT /api/v1/notifications/marker: it moves the viewer's read
// marker forward to last_read_id, never back.
func (s *Server) UpdateNotificationMarker(ctx context.Context, req UpdateNotificationMarkerRequestObject) (UpdateNotificationMarkerResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		body, headers := s.invalidParameter(ctx, "last_read_id")
		return UpdateNotificationMarkerdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusBadRequest}, nil
	}
	m, err := s.o.Notifications.MarkRead(ctx, v.AccountID, knownID(req.Body.LastReadId))
	if errors.Is(err, appnotification.ErrNotFound) {
		body, headers := s.notFound(ctx)
		return UpdateNotificationMarkerdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: mark the notifications of %s read: %w", v.AccountID, err)
	}
	body, err := s.newNotificationMarker(ctx, v.AccountID, m)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return UpdateNotificationMarker200JSONResponse{Body: body, Headers: UpdateNotificationMarker200ResponseHeaders{CacheControl: &cache}}, nil
}

// newNotificationMarker presents accountID's marker m, with its unread count, as the
// NotificationMarker schema.
func (s *Server) newNotificationMarker(ctx context.Context, accountID string, m notification.ReadMarker) (NotificationMarker, error) {
	n, err := s.o.Notifications.UnreadCount(ctx, accountID)
	if err != nil {
		return NotificationMarker{}, fmt.Errorf("clientapi: the unread notifications of %s: %w", accountID, err)
	}
	out := NotificationMarker{UnreadCount: n}
	if m.LastReadID != "" {
		id := m.LastReadID
		out.LastReadId = &id
	}
	return out, nil
}

// DismissNotification serves POST /api/v1/notifications/{id}/dismiss.
func (s *Server) DismissNotification(ctx context.Context, req DismissNotificationRequestObject) (DismissNotificationResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.Notifications.Dismiss(ctx, v.AccountID, req.Id)
	if errors.Is(err, appnotification.ErrNotFound) {
		body, headers := s.notFound(ctx)
		return DismissNotificationdefaultApplicationProblemPlusJSONResponse{Body: body, Headers: headers, StatusCode: http.StatusNotFound}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: dismiss notification %s of %s: %w", req.Id, v.AccountID, err)
	}
	return DismissNotification204Response{}, nil
}
