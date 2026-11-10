// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

var _ appnotification.PolicyRepository = (*Repository)(nil)

// SavePolicyChoices implements notification.PolicyRepository.
func (r *Repository) SavePolicyChoices(ctx context.Context, tx uow.Tx, choices []notification.PolicyChoice) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, c := range choices {
		err := q.UpsertNotificationPolicy(ctx, notificationgen.UpsertNotificationPolicyParams{
			AccountID: c.AccountID, PolicyCondition: string(c.Condition), PolicyAction: string(c.Action), UpdatedAt: c.UpdatedAt,
		})
		if err != nil {
			return postgres.Wrap(err, "upsert notification policy")
		}
	}
	return nil
}

// PolicyChoices implements notification.PolicyRepository.
func (r *Repository) PolicyChoices(ctx context.Context, accountID string) ([]notification.PolicyChoice, error) {
	rows, err := r.db.reader().ListNotificationPolicies(ctx, accountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list notification policies")
	}
	out := make([]notification.PolicyChoice, len(rows))
	for i, row := range rows {
		out[i] = policyChoiceFrom(row)
	}
	return out, nil
}

// PolicyChoicesOfTx implements notification.PolicyRepository.
func (r *Repository) PolicyChoicesOfTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string][]notification.PolicyChoice, error) {
	out := make(map[string][]notification.PolicyChoice, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListNotificationPoliciesOf(ctx, accountIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list notification policies")
	}
	for _, row := range rows {
		out[row.AccountID] = append(out[row.AccountID], policyChoiceFrom(row))
	}
	return out, nil
}

func policyChoiceFrom(row notificationgen.NotificationPolicy) notification.PolicyChoice {
	return notification.PolicyChoice{
		AccountID: row.AccountID, Condition: notification.Condition(row.PolicyCondition), Action: notification.Action(row.PolicyAction),
		UpdatedAt: row.UpdatedAt,
	}
}

// InsertFilterings implements notification.PolicyRepository.
func (r *Repository) InsertFilterings(ctx context.Context, tx uow.Tx, ns []notification.Notification) error {
	if len(ns) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	arg := notificationgen.InsertNotificationFilteringsParams{}
	for _, n := range ns {
		arg.NotificationIds = append(arg.NotificationIds, n.ID)
		arg.RecipientAccountIds = append(arg.RecipientAccountIds, n.RecipientAccountID)
		arg.SourceAccountIds = append(arg.SourceAccountIds, n.SourceAccountID)
	}
	return postgres.Wrap(q.InsertNotificationFilterings(ctx, arg), "insert notification filterings")
}

// SourceAcceptedTx implements notification.PolicyRepository.
func (r *Repository) SourceAcceptedTx(ctx context.Context, tx uow.Tx, recipientAccountID, sourceAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.NotificationSourceAccepted(ctx, notificationgen.NotificationSourceAcceptedParams{
		RecipientAccountID: recipientAccountID, SourceAccountID: sourceAccountID,
	})
	return ok, postgres.Wrap(err, "notification source accepted")
}

// InsertAcceptedSource implements notification.PolicyRepository.
func (r *Repository) InsertAcceptedSource(ctx context.Context, tx uow.Tx, a notification.AcceptedSource) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertNotificationAcceptedSource(ctx, notificationgen.InsertNotificationAcceptedSourceParams{
		RecipientAccountID: a.RecipientAccountID, SourceAccountID: a.SourceAccountID, CreatedAt: a.CreatedAt,
	}), "insert notification accepted source")
}

// InsertRequestDismissal implements notification.PolicyRepository.
func (r *Repository) InsertRequestDismissal(ctx context.Context, tx uow.Tx, d notification.RequestDismissal) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertNotificationRequestDismissal(ctx, notificationgen.InsertNotificationRequestDismissalParams{
		ID: d.ID, RecipientAccountID: d.RecipientAccountID, SourceAccountID: d.SourceAccountID,
		LastNotificationID: d.LastNotificationID, DismissedAt: d.DismissedAt,
	}), "insert notification request dismissal")
}

// RefreshRequests implements notification.PolicyRepository.
func (r *Repository) RefreshRequests(ctx context.Context, tx uow.Tx, pairs []appnotification.RequestPair) error {
	if len(pairs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return refreshRequests(ctx, q, pairs)
}

func refreshRequests(ctx context.Context, q *notificationgen.Queries, pairs []appnotification.RequestPair) error {
	var recipients, sources []string
	for _, p := range pairs {
		recipients, sources = append(recipients, p.RecipientAccountID), append(sources, p.SourceAccountID)
	}
	err := q.DeleteNotificationRequestsOf(ctx, notificationgen.DeleteNotificationRequestsOfParams{RecipientAccountIds: recipients, SourceAccountIds: sources})
	if err != nil {
		return postgres.Wrap(err, "delete notification requests")
	}
	return postgres.Wrap(q.InsertNotificationRequestsOf(ctx, notificationgen.InsertNotificationRequestsOfParams{
		RecipientAccountIds: recipients, SourceAccountIds: sources,
	}), "derive notification requests")
}

// RebuildRequests implements notification.PolicyRepository.
func (r *Repository) RebuildRequests(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllNotificationRequests(ctx); err != nil {
		return postgres.Wrap(err, "empty notification requests")
	}
	return postgres.Wrap(q.RebuildNotificationRequests(ctx), "rebuild notification requests")
}

// RequestTx implements notification.PolicyRepository.
func (r *Repository) RequestTx(ctx context.Context, tx uow.Tx, recipientAccountID, sourceAccountID string) (notification.Request, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return notification.Request{}, err
	}
	row, err := q.GetNotificationRequestTx(ctx, notificationgen.GetNotificationRequestTxParams{
		RecipientAccountID: recipientAccountID, SourceAccountID: sourceAccountID,
	})
	if err != nil {
		return notification.Request{}, postgres.NotFound(err, appnotification.ErrNotFound, "notification request of "+sourceAccountID)
	}
	return requestFrom(row), nil
}

// Requests implements notification.PolicyRepository.
func (r *Repository) Requests(ctx context.Context, recipientAccountID, before string, limit int, now time.Time) ([]notification.Request, error) {
	n, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListNotificationRequests(ctx, notificationgen.ListNotificationRequestsParams{
		RecipientAccountID: recipientAccountID, Before: postgres.ToNullString(before), Now: now, PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list notification requests")
	}
	out := make([]notification.Request, len(rows))
	for i, row := range rows {
		out[i] = requestFrom(notificationgen.NotificationRequest{
			RecipientAccountID: row.RecipientAccountID, SourceAccountID: row.SourceAccountID,
			NotificationsCount: row.NotificationsCount, LastNotificationID: row.LastNotificationID,
		})
		out[i].Last = notificationFrom(row.Notification)
	}
	return out, nil
}

// Request implements notification.PolicyRepository.
func (r *Repository) Request(ctx context.Context, recipientAccountID, sourceAccountID string, now time.Time) (notification.Request, error) {
	row, err := r.db.reader().GetNotificationRequest(ctx, notificationgen.GetNotificationRequestParams{
		RecipientAccountID: recipientAccountID, SourceAccountID: sourceAccountID, Now: now,
	})
	if err != nil {
		return notification.Request{}, postgres.NotFound(err, appnotification.ErrNotFound, "notification request of "+sourceAccountID)
	}
	out := requestFrom(notificationgen.NotificationRequest{
		RecipientAccountID: row.RecipientAccountID, SourceAccountID: row.SourceAccountID,
		NotificationsCount: row.NotificationsCount, LastNotificationID: row.LastNotificationID,
	})
	out.Last = notificationFrom(row.Notification)
	return out, nil
}

// RequestSummary implements notification.PolicyRepository.
func (r *Repository) RequestSummary(ctx context.Context, recipientAccountID string, now time.Time) (notification.RequestSummary, error) {
	row, err := r.db.reader().SummarizeNotificationRequests(ctx, notificationgen.SummarizeNotificationRequestsParams{
		RecipientAccountID: recipientAccountID, Now: now,
	})
	if err != nil {
		return notification.RequestSummary{}, postgres.Wrap(err, "summarize notification requests")
	}
	return notification.RequestSummary{Accounts: row.Accounts, Notifications: row.Notifications}, nil
}

// RequestNotifications implements notification.PolicyRepository.
func (r *Repository) RequestNotifications(ctx context.Context, recipientAccountID, sourceAccountID, before string, limit int) ([]notification.Notification, error) {
	n, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListNotificationRequestNotifications(ctx, notificationgen.ListNotificationRequestNotificationsParams{
		RecipientAccountID: recipientAccountID, SourceAccountID: sourceAccountID, Before: postgres.ToNullString(before), PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list notification request notifications")
	}
	out := make([]notification.Notification, len(rows))
	for i, row := range rows {
		out[i] = notificationFrom(row)
	}
	return out, nil
}

func requestFrom(row notificationgen.NotificationRequest) notification.Request {
	return notification.Request{
		RecipientAccountID: row.RecipientAccountID, SourceAccountID: row.SourceAccountID,
		NotificationsCount: row.NotificationsCount, LastNotificationID: row.LastNotificationID,
	}
}

// erasePolicyOf erases the rows of the policy whose recipient or source is accountID, a bounded
// batch of each, and returns the most rows it deleted from one table.
func erasePolicyOf(ctx context.Context, q *notificationgen.Queries, accountID string, lim int32) (int64, error) {
	if err := q.EraseNotificationPoliciesOf(ctx, accountID); err != nil {
		return 0, postgres.Wrap(err, "erase notification policies")
	}
	accepted, err := q.EraseNotificationAcceptedSourcesOf(ctx, notificationgen.EraseNotificationAcceptedSourcesOfParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase notification accepted sources")
	}
	dismissals, err := q.EraseNotificationRequestDismissalsOf(ctx, notificationgen.EraseNotificationRequestDismissalsOfParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase notification request dismissals")
	}
	requests, err := q.EraseNotificationRequestsOf(ctx, notificationgen.EraseNotificationRequestsOfParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase notification requests")
	}
	return max(accepted, dismissals, requests), nil
}

// filteredPairsOfStatuses returns the requests that hold filtered notifications about the
// statuses ids, which their erasure derives again.
func filteredPairsOfStatuses(ctx context.Context, q *notificationgen.Queries, ids []string) ([]appnotification.RequestPair, error) {
	rows, err := q.FilteredPairsOfStatuses(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list filtered pairs of statuses")
	}
	out := make([]appnotification.RequestPair, len(rows))
	for i, row := range rows {
		out[i] = appnotification.RequestPair{RecipientAccountID: row.RecipientAccountID, SourceAccountID: row.SourceAccountID}
	}
	return out, nil
}
