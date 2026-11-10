// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// Notifications runs the use cases of the viewer's own notifications (FR-205).
// *appnotification.Service implements it.
type Notifications interface {
	List(ctx context.Context, accountID string, q appnotification.Query) ([]notification.Notification, error)
	ReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error)
	UnreadCount(ctx context.Context, accountID string, withoutModerator bool) (int64, error)
	MarkRead(ctx context.Context, accountID, lastReadID string) (notification.ReadMarker, error)
	Dismiss(ctx context.Context, accountID, notificationID string) error
	// ErrorGroupOthers returns how many error groups an admin_error_group of the others folds.
	ErrorGroupOthers(ctx context.Context, n notification.Notification) (int64, error)
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
	// One more tells whether a next page exists. The moderator notifications stay with the
	// first-party clients, as administration does, whatever the user's roles.
	q := appnotification.Query{Limit: limit + 1, WithoutModerator: !v.FirstParty}
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
// visibility policy: a post that the user may not see is left out of its notification. A
// notification whose post a hide filter of the notifications matches is left out, so the result
// may be shorter than ns. The source
// accounts are read once each; a source account that no longer exists is left out. A moderation
// notice carries its action as the user sees it (moderation.go), and a report_resolved the
// user's report, which tells only that it is resolved. The account of an
// admin_sign_up is the local account of the user who signed up, and a relationships_severed
// carries the follows that the user lost, with the suspended account as its account.
//
// read:notifications shows the status of a notification and tells what happened through its
// type; any other object comes only with the scope that reads it elsewhere, and a moderation
// action only to a first-party client, which alone may list them.
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
	limitNotices, err := s.notifiedAutoDeleteLimits(ctx, accountID, ns)
	if err != nil {
		return nil, err
	}
	schedules, err := s.notifiedDraftSchedules(ctx, ns)
	if err != nil {
		return nil, err
	}
	presented, err := s.presentedAccounts(ctx, ns, severances, moves)
	if err != nil {
		return nil, err
	}
	voiceRooms := map[string]*VoiceRoom{}
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
	// The source accounts, read together; nil for one that no longer exists.
	var sourceIDs []string
	for _, id := range presented {
		if id != "" && !slices.Contains(sourceIDs, id) {
			sourceIDs = append(sourceIDs, id)
		}
	}
	actors, err := s.o.Accounts.FindByIDs(ctx, sourceIDs)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the accounts of the notifications of %s: %w", accountID, err)
	}
	sources := make(map[string]*appaccount.Actor, len(sourceIDs))
	var found []appaccount.Actor
	for _, id := range sourceIDs {
		if a, ok := actors[id]; ok {
			sources[id] = &a
			found = append(found, a)
		} else {
			sources[id] = nil
		}
	}
	var reports map[string]Report
	if viewer.Scope.Grants(identity.ScopeReadReports) {
		if reports, err = s.notifiedReports(ctx, accountID, ns); err != nil {
			return nil, err
		}
	}
	var actions, appealed map[string]ModerationAction
	var hookAppeals map[string]OwnHookAppeal
	if viewer.FirstParty {
		var actionIDs []string
		for _, n := range ns {
			if n.ModerationActionID != "" {
				actionIDs = append(actionIDs, n.ModerationActionID)
			}
		}
		if actions, err = s.noticeActions(ctx, accountID, actionIDs); err != nil {
			return nil, err
		}
		if appealed, err = s.appealActions(ctx, accountID, ns); err != nil {
			return nil, err
		}
		if hookAppeals, err = s.notifiedHookAppeals(ctx, accountID, ns); err != nil {
			return nil, err
		}
	}
	var emojiRequests map[string]CustomEmojiRequest
	if viewer.Scope.Grants(identity.ScopeReadCustomEmojis) {
		if emojiRequests, err = s.notifiedCustomEmojiRequests(ctx, ns); err != nil {
			return nil, err
		}
	}
	var supporterSubs map[string]SupporterSubscription
	if viewer.Scope.Grants(identity.ScopeReadSupporters) {
		if supporterSubs, err = s.notifiedSupporterSubscriptions(ctx, ns); err != nil {
			return nil, err
		}
	}
	closures, err := s.notifiedClosures(ctx, ns)
	if err != nil {
		return nil, err
	}
	reactionEmojis, err := s.reactionEmojis(ctx, ns)
	if err != nil {
		return nil, err
	}
	announcements, err := s.notifiedAnnouncements(ctx, viewer.UserID, ns)
	if err != nil {
		return nil, err
	}
	errorGroupOthers, err := s.errorGroupOthers(ctx, ns)
	if err != nil {
		return nil, err
	}
	parts, err := s.statusParts(ctx, views, found...)
	if err != nil {
		return nil, err
	}
	// The keyword filters of the notifications: a hide filter leaves the notification of a status
	// out, a warn filter marks its status (ADR-0157 §7).
	filter, err := s.statusFilter(ctx, localContentViewer(viewer), timeline.FilterContextNotifications)
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]Status, len(views))
	hidden := map[string]bool{}
	for _, v := range views {
		st := s.newStatus(v, parts)
		if applyJudgement(&st, filter.Judge(v, ""), false) {
			statuses[v.Status.ID] = st
		} else {
			hidden[v.Status.ID] = true
		}
	}
	accounts := make(map[string]Account, len(found))
	for id, a := range sources {
		if a != nil {
			accounts[id] = s.newAccount(*a, parts)
		}
	}
	out := make([]Notification, 0, len(ns))
	for i, n := range ns {
		if hidden[n.StatusID] {
			continue
		}
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
		if n.Type == notification.TypeAdminHookAppeal {
			id := n.HookAppealID
			item.HookAppealId = &id
		}
		if a, ok := hookAppeals[n.HookAppealID]; ok && n.Type == notification.TypeHookAppealDecided {
			item.HookAppeal = &a
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
		if c, ok := closures[n.ServiceClosureEventID]; ok {
			if n.ServiceClosureChange != "" {
				change := ServiceClosureNoticeChange(n.ServiceClosureChange)
				c.Change = &change
			}
			item.ServiceClosure = &c
		}
		if sv, ok := severances[n.SeveranceID]; ok && viewer.Scope.Grants(identity.ScopeReadFollows) {
			item.RelationshipSeverance = newRelationshipSeverance(sv)
		}
		if m, ok := moves[n.AccountMoveID]; ok {
			item.AccountMove = &m.view
			if n.AccountMoveOutcome != "" {
				outcome := AccountMoveOutcome(n.AccountMoveOutcome)
				item.AccountMove.Outcome = &outcome
			}
		}
		item.Reaction = s.reactionEmojiOf(n, reactionEmojis)
		if n.ErrorGroupChange != "" {
			item.ErrorGroup = notifiedErrorGroup(n, errorGroupOthers)
		}
		if notice, ok := limitNotices[n.AutoDeleteLimitNoticeID]; ok && n.Type == notification.TypeAutoDeleteLimit {
			item.AutoDeleteLimit = &NotificationAutoDeleteLimit{MaxAgeDays: notice.MaxAgeDays, EnforceFrom: notice.EnforceFrom.UTC()}
		}
		if r, ok := schedules[n.DraftScheduleID]; ok && n.Type == notification.TypeScheduledStatusFailed && r.Outcome.FailureReason != "" {
			item.ScheduledStatusFailure = &NotificationScheduledStatusFailure{
				DraftId: r.Schedule.DraftID, Reason: DraftFailureReason(r.Outcome.FailureReason), PublishAt: r.Schedule.PublishAt.UTC(),
			}
		}
		if a, ok := announcements[n.AnnouncementID]; ok && n.AnnouncementID != "" {
			view := newAnnouncement(a)
			item.Announcement = &view
		}
		if n.VoiceRoomID != "" {
			id := n.VoiceRoomID
			item.VoiceRoomId = &id
			if viewer.Scope.Grants(identity.ScopeReadVoice) {
				if item.VoiceRoom, err = s.notifiedVoiceRoom(ctx, accountID, id, voiceRooms); err != nil {
					return nil, err
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// ClosureEvents reads the service closure events of the notifications. *appinstance.Service
// implements it.
type ClosureEvents interface {
	// ClosureEvent returns an event with its schedule; appinstance.ErrNotFound when there is none.
	ClosureEvent(ctx context.Context, id string) (instance.ClosureEvent, error)
}

// notifiedClosures presents, by event ID, the service closure events that ns tell of, with the
// dates of an announcement; an event that is gone is left out.
func (s *Server) notifiedClosures(ctx context.Context, ns []notification.Notification) (map[string]ServiceClosureNotice, error) {
	out := map[string]ServiceClosureNotice{}
	if s.o.ClosureEvents == nil {
		return out, nil
	}
	var ids []string
	for _, n := range ns {
		if id := n.ServiceClosureEventID; id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	events, err := s.closureEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the service closure events of the notifications: %w", err)
	}
	for id, e := range events {
		var v ServiceClosureNotice
		if e.Kind == instance.ClosureEventAnnounced {
			v.ClosesAt, v.ExportUntil = utcPtr(e.Schedule.ClosesAt), utcPtr(e.Schedule.ExportUntil)
		}
		out[id] = v
	}
	return out, nil
}

// closureEventBatch is a ClosureEvents that reads many events at once, as *appinstance.Service
// does.
type closureEventBatch interface {
	ClosureEvents(ctx context.Context, ids []string) (map[string]instance.ClosureEvent, error)
}

// closureEvents returns the events of ids that exist: together when ClosureEvents can, one by one
// otherwise.
func (s *Server) closureEvents(ctx context.Context, ids []string) (map[string]instance.ClosureEvent, error) {
	if b, ok := s.o.ClosureEvents.(closureEventBatch); ok {
		return b.ClosureEvents(ctx, ids)
	}
	out := make(map[string]instance.ClosureEvent, len(ids))
	for _, id := range ids {
		e, err := s.o.ClosureEvents.ClosureEvent(ctx, id)
		switch {
		case errors.Is(err, appinstance.ErrNotFound):
		case err != nil:
			return nil, err
		default:
			out[id] = e
		}
	}
	return out, nil
}

// presentedAccounts returns, for each of ns, the ID of the account that its account property
// presents: the source account, for an admin_sign_up the local account of the user who signed
// up, for a relationships_severed the suspended account of severances, and for an
// admin_account_moved the account that moved; "" for none.
func (s *Server) presentedAccounts(ctx context.Context, ns []notification.Notification, severances map[string]moderation.SeveranceRecord, moves map[string]notifiedMove) ([]string, error) {
	out := make([]string, len(ns))
	var users []string
	for i, n := range ns {
		out[i] = n.SourceAccountID
		if sv, ok := severances[n.SeveranceID]; ok {
			out[i] = sv.TargetAccountID
		}
		if m, ok := moves[n.AccountMoveID]; ok && n.Type == notification.TypeAdminAccountMoved {
			out[i] = m.move.AccountID
		}
		if n.Type == notification.TypeAdminSignUp && n.SubjectUserID != "" && !slices.Contains(users, n.SubjectUserID) {
			users = append(users, n.SubjectUserID)
		}
	}
	if len(users) == 0 || s.o.UserAccounts == nil {
		return out, nil
	}
	accounts, err := s.localAccountsOfUsers(ctx, users)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the accounts of the users of the sign-up notifications: %w", err)
	}
	for i, n := range ns {
		if n.Type != notification.TypeAdminSignUp || n.SubjectUserID == "" {
			continue
		}
		if la, ok := accounts[n.SubjectUserID]; ok {
			out[i] = la.AccountID
		}
	}
	return out, nil
}

// userAccountBatch is a UserAccounts that reads the local accounts of many users at once, as
// *appaccount.Service does.
type userAccountBatch interface {
	LocalAccountsOfUsers(ctx context.Context, userIDs []string) (map[string]account.LocalAccount, error)
}

// localAccountsOfUsers returns the local accounts of users that have one, by user ID: together when
// UserAccounts can, one by one otherwise.
func (s *Server) localAccountsOfUsers(ctx context.Context, users []string) (map[string]account.LocalAccount, error) {
	if b, ok := s.o.UserAccounts.(userAccountBatch); ok {
		return b.LocalAccountsOfUsers(ctx, users)
	}
	out := make(map[string]account.LocalAccount, len(users))
	for _, u := range users {
		la, err := s.o.UserAccounts.LocalAccountOfUser(ctx, u)
		switch {
		case err == nil:
			out[u] = la
		case !errors.Is(err, appaccount.ErrNotFound):
			return nil, err
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
	var ids []string
	for _, n := range ns {
		if n.Type == notification.TypeReportResolved && n.ReportID != "" && !slices.Contains(ids, n.ReportID) {
			ids = append(ids, n.ReportID)
		}
	}
	if len(ids) == 0 {
		return map[string]Report{}, nil
	}
	found, err := s.reportsFiledByID(ctx, accountID, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the reports of the notifications of %s: %w", accountID, err)
	}
	recs := make([]moderation.ReportRecord, 0, len(found))
	for _, id := range ids {
		if rec, ok := found[id]; ok {
			recs = append(recs, rec)
		}
	}
	items, err := s.newReports(ctx, Viewer{AccountID: accountID}, recs)
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
	var ids []string
	for _, n := range ns {
		if n.Type == notification.TypeRelationshipsSevered && n.SeveranceID != "" {
			ids = append(ids, n.SeveranceID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	found, err := s.severancesOf(ctx, accountID, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the severances of the notifications of %s: %w", accountID, err)
	}
	return found, nil
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
	var ids []string
	for _, n := range ns {
		if n.AccountMoveID != "" && !slices.Contains(ids, n.AccountMoveID) {
			ids = append(ids, n.AccountMoveID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	moves, err := s.findMoves(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the moves of the notifications: %w", err)
	}
	var targetIDs []string
	for _, m := range moves {
		targetIDs = append(targetIDs, m.TargetAccountID)
	}
	targets, err := s.presentedTargets(ctx, targetIDs)
	if err != nil {
		return nil, err
	}
	for id, m := range moves {
		out[id] = notifiedMove{move: m, view: AccountMove{Target: targets[m.TargetAccountID]}}
	}
	return out, nil
}

// moveBatch is a Moves that reads many moves at once, as *appaccount.Service does.
type moveBatch interface {
	FindMoves(ctx context.Context, ids []string) (map[string]appaccount.Move, error)
}

// findMoves returns the moves of ids that exist: together when Moves can, one by one otherwise.
func (s *Server) findMoves(ctx context.Context, ids []string) (map[string]appaccount.Move, error) {
	if b, ok := s.o.Moves.(moveBatch); ok {
		return b.FindMoves(ctx, ids)
	}
	return byOne(ids, func(id string) (appaccount.Move, error) { return s.o.Moves.FindMove(ctx, id) }, appaccount.ErrNotFound)
}

// presentedTargets presents the move targets of ids as presentedTarget presents one, reading the
// accounts and their parts together; a target that is missing or not active is nil.
func (s *Server) presentedTargets(ctx context.Context, ids []string) (map[string]*Account, error) {
	found, err := s.o.Accounts.FindByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the move targets: %w", err)
	}
	var active []appaccount.Actor
	for _, a := range found {
		if a.Lifecycle.Active() {
			active = append(active, a)
		}
	}
	parts, err := s.accountParts(ctx, active...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*Account, len(active))
	for _, a := range active {
		v := s.newAccount(a, parts)
		out[a.Account.ID] = &v
	}
	return out, nil
}

// newRelationshipSeverance presents sv as the RelationshipSeverance schema.
func newRelationshipSeverance(sv moderation.SeveranceRecord) *RelationshipSeverance {
	out := &RelationshipSeverance{
		Kind: RelationshipSeveranceKindAccountSuspension, FollowersLost: sv.FollowersLost, FollowingLost: sv.FollowingLost,
	}
	switch {
	case sv.DomainActionID != "":
		domain := sv.Domain
		out.Kind, out.Domain = RelationshipSeveranceKindDomainSuspension, &domain
	case sv.AllowlistCutoffID != "":
		out.Kind = RelationshipSeveranceKindAllowlistCutoff
		if sv.Domain != "" {
			domain := sv.Domain
			out.Domain = &domain
		}
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
	body, err := s.newNotificationMarker(ctx, v, m)
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
	body, err := s.newNotificationMarker(ctx, v, m)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return UpdateNotificationMarker200JSONResponse{Body: body, Headers: UpdateNotificationMarker200ResponseHeaders{CacheControl: &cache}}, nil
}

// newNotificationMarker presents the viewer's marker m, with its unread count, as the
// NotificationMarker schema.
func (s *Server) newNotificationMarker(ctx context.Context, v Viewer, m notification.ReadMarker) (NotificationMarker, error) {
	n, err := s.o.Notifications.UnreadCount(ctx, v.AccountID, !v.FirstParty)
	if err != nil {
		return NotificationMarker{}, fmt.Errorf("clientapi: the unread notifications of %s: %w", v.AccountID, err)
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

// notifiedErrorGroup presents what an admin_error_group tells of, with the count of the groups
// that one of the others folds.
func notifiedErrorGroup(n notification.Notification, others map[string]int64) *NotificationErrorGroup {
	out := &NotificationErrorGroup{Change: NotificationErrorGroupChange(n.ErrorGroupChange)}
	if n.ErrorGroupID != "" {
		id := n.ErrorGroupID
		out.GroupId = &id
	}
	if n.ErrorGroupChange == notification.ErrorGroupOthers {
		count := others[n.ID]
		out.Count = &count
	}
	return out
}

// errorGroupOtherCounts is a Notifications that counts the other error groups of many
// notifications at once, as *appnotification.Service does.
type errorGroupOtherCounts interface {
	ErrorGroupOthersOf(ctx context.Context, ns []notification.Notification) (map[string]int64, error)
}

// errorGroupOthers returns the counts of the other error groups that the notifications of ns tell
// of, by notification ID: together when Notifications can, one by one otherwise.
func (s *Server) errorGroupOthers(ctx context.Context, ns []notification.Notification) (map[string]int64, error) {
	if b, ok := s.o.Notifications.(errorGroupOtherCounts); ok {
		return b.ErrorGroupOthersOf(ctx, ns)
	}
	out := map[string]int64{}
	for _, n := range ns {
		if n.ErrorGroupChange != notification.ErrorGroupOthers {
			continue
		}
		count, err := s.o.Notifications.ErrorGroupOthers(ctx, n)
		if err != nil {
			return nil, err
		}
		out[n.ID] = count
	}
	return out, nil
}

// announcementBatch is an Announcements that reads many shown announcements at once, as
// *appinstance.Service does.
type announcementBatch interface {
	ShownAnnouncementsByID(ctx context.Context, userID string, ids []string) (map[string]appinstance.ReaderAnnouncement, error)
}

// notifiedAnnouncements returns the announcements that the notifications of ns tell of and the
// user is shown, by ID: together when Announcements can, one by one otherwise.
func (s *Server) notifiedAnnouncements(ctx context.Context, userID string, ns []notification.Notification) (map[string]appinstance.ReaderAnnouncement, error) {
	out := map[string]appinstance.ReaderAnnouncement{}
	if s.o.Announcements == nil {
		return out, nil
	}
	var ids []string
	for _, n := range ns {
		if id := n.AnnouncementID; id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	if b, ok := s.o.Announcements.(announcementBatch); ok {
		found, err := b.ShownAnnouncementsByID(ctx, userID, ids)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the announcements of the notifications: %w", err)
		}
		return found, nil
	}
	for _, id := range ids {
		a, ok, err := s.o.Announcements.ShownAnnouncement(ctx, userID, id)
		if err != nil {
			return nil, fmt.Errorf("clientapi: the announcement %s of a notification: %w", id, err)
		}
		if ok {
			out[id] = a
		}
	}
	return out, nil
}

// notifiedAutoDeleteLimits returns the notices of the server maximum of the automatic deletion
// that the auto_delete_limit notifications among ns tell the account of, by ID.
func (s *Server) notifiedAutoDeleteLimits(ctx context.Context, accountID string, ns []notification.Notification) (map[string]content.AutoDeleteLimitNotice, error) {
	var ids []string
	for _, n := range ns {
		if n.Type == notification.TypeAutoDeleteLimit {
			ids = append(ids, n.AutoDeleteLimitNoticeID)
		}
	}
	if len(ids) == 0 || s.o.AutoDelete == nil {
		return nil, nil
	}
	notices, err := s.o.AutoDelete.LimitNotices(ctx, accountID, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the auto delete notices of %s: %w", accountID, err)
	}
	return notices, nil
}

// DraftScheduleReader reads the schedules of drafts with their results, by ID
// (ADR-0156 §8); *appcontent.DraftService implements it. A schedule whose draft
// was deleted is absent.
type DraftScheduleReader interface {
	DraftSchedules(ctx context.Context, ids []string) (map[string]appcontent.DraftScheduleRecord, error)
}

// notifiedDraftSchedules returns the schedules that the scheduled_status_failed notifications
// among ns tell of, by ID. The notifications are the viewer's own, made for the author of the
// draft, so the schedules are the viewer's.
func (s *Server) notifiedDraftSchedules(ctx context.Context, ns []notification.Notification) (map[string]appcontent.DraftScheduleRecord, error) {
	var ids []string
	for _, n := range ns {
		if n.Type == notification.TypeScheduledStatusFailed {
			ids = append(ids, n.DraftScheduleID)
		}
	}
	if len(ids) == 0 || s.o.DraftSchedules == nil {
		return nil, nil
	}
	schedules, err := s.o.DraftSchedules.DraftSchedules(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the draft schedules of the notifications: %w", err)
	}
	return schedules, nil
}
