// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// NotificationRepository implements notification.Repository on PostgreSQL.
type NotificationRepository struct {
	db *DB
}

var _ appnotification.Repository = (*NotificationRepository)(nil)

// NotificationRepository returns the notification module's repository on d.
func (d *DB) NotificationRepository() *NotificationRepository { return &NotificationRepository{db: d} }

// AddErrorGroupNotice implements notification.Repository.
func (r *NotificationRepository) AddErrorGroupNotice(ctx context.Context, tx uow.Tx, hour time.Time) (int64, error) {
	q, err := queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.AddErrorGroupNotice(ctx, hour)
	return n, wrap(err, "add error group notice")
}

// ErrorGroupNotices implements notification.Repository.
func (r *NotificationRepository) ErrorGroupNotices(ctx context.Context, hour time.Time) (int64, error) {
	n, err := r.db.reader().GetErrorGroupNotices(ctx, hour)
	return n, wrap(err, "get error group notices")
}

// InsertNotification implements notification.Repository.
func (r *NotificationRepository) InsertNotification(ctx context.Context, tx uow.Tx, n notification.Notification) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertNotification(ctx, postgresgen.InsertNotificationParams{
		ID: n.ID, RecipientAccountID: n.RecipientAccountID, Type: string(n.Type),
		SourceAccountID: toNullString(n.SourceAccountID), SourceDomain: toNullString(n.SourceDomain), StatusID: toNullString(n.StatusID),
		ModerationActionID: toNullString(n.ModerationActionID), ReportID: toNullString(n.ReportID),
		SubjectUserID: toNullString(n.SubjectUserID), SeveranceID: toNullString(n.SeveranceID), AppealID: toNullString(n.AppealID),
		CustomEmojiRequestID: toNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: toNullString(n.SupporterSubscriptionID),
		ReactionEmoji: toNullString(n.ReactionEmoji), ReactionCustomEmojiID: toNullString(n.ReactionCustomEmojiID),
		AccountMoveID: toNullString(n.AccountMoveID), AccountMoveOutcome: toNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: toNullString(n.ServiceClosureEventID), ServiceClosureChange: toNullString(string(n.ServiceClosureChange)), VoiceRoomID: toNullString(n.VoiceRoomID),
		ErrorGroupChange: toNullString(string(n.ErrorGroupChange)), ErrorGroupID: toNullString(n.ErrorGroupID), ErrorGroupHour: toNullTime(n.ErrorGroupHour),
		DedupeKey: n.DedupeKey, CreatedAt: n.CreatedAt,
	})
	if err != nil {
		return false, wrap(err, "insert notification")
	}
	return rows == 1, nil
}

// Notification implements notification.Repository.
func (r *NotificationRepository) Notification(ctx context.Context, tx uow.Tx, id string) (notification.Notification, error) {
	q, err := queries(tx)
	if err != nil {
		return notification.Notification{}, err
	}
	row, err := q.GetNotification(ctx, id)
	if err != nil {
		return notification.Notification{}, notFound(err, appnotification.ErrNotFound, "notification "+id)
	}
	return notificationFrom(row), nil
}

// InsertDismissal implements notification.Repository.
func (r *NotificationRepository) InsertDismissal(ctx context.Context, tx uow.Tx, d notification.Dismissal) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertNotificationDismissal(ctx, postgresgen.InsertNotificationDismissalParams{
		NotificationID: d.NotificationID, DismissedAt: d.DismissedAt,
	})
	if err != nil {
		return false, wrap(err, "insert notification dismissal")
	}
	return rows == 1, nil
}

// AdvanceReadMarker implements notification.Repository.
func (r *NotificationRepository) AdvanceReadMarker(ctx context.Context, tx uow.Tx, m notification.ReadMarker) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.AdvanceNotificationReadMarker(ctx, postgresgen.AdvanceNotificationReadMarkerParams{
		AccountID: m.AccountID, LastReadID: m.LastReadID,
	})
	if err != nil {
		return false, wrap(err, "advance notification read marker")
	}
	return rows == 1, nil
}

// UpsertExclusion implements notification.Repository.
func (r *NotificationRepository) UpsertExclusion(ctx context.Context, tx uow.Tx, x notification.Exclusion) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertNotificationExclusion(ctx, postgresgen.UpsertNotificationExclusionParams{
		ViewerAccountID: x.ViewerAccountID, TargetAccountID: x.TargetAccountID, Reason: string(x.Reason), ExpiresAt: x.ExpiresAt,
	}), "upsert notification exclusion")
}

// DeleteExclusion implements notification.Repository.
func (r *NotificationRepository) DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason notification.ExclusionReason) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteNotificationExclusion(ctx, postgresgen.DeleteNotificationExclusionParams{
		ViewerAccountID: viewerAccountID, TargetAccountID: targetAccountID, Reason: string(reason),
	}), "delete notification exclusion")
}

// InsertDomainExclusion implements notification.Repository.
func (r *NotificationRepository) InsertDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertNotificationDomainExclusion(ctx, postgresgen.InsertNotificationDomainExclusionParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "insert notification domain exclusion")
}

// DeleteDomainExclusion implements notification.Repository.
func (r *NotificationRepository) DeleteDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteNotificationDomainExclusion(ctx, postgresgen.DeleteNotificationDomainExclusionParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "delete notification domain exclusion")
}

// DomainExcluded implements notification.Repository.
func (r *NotificationRepository) DomainExcluded(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	excluded, err := q.DomainExcluded(ctx, postgresgen.DomainExcludedParams{ViewerAccountID: viewerAccountID, Domain: domain})
	return excluded, wrap(err, "check notification domain exclusion")
}

// InsertDeletedStatus implements notification.Repository.
func (r *NotificationRepository) InsertDeletedStatus(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertNotificationDeletedStatus(ctx, statusID), "insert notification deleted status")
}

// ListNotifications implements notification.Repository.
func (r *NotificationRepository) ListNotifications(ctx context.Context, recipientAccountID string, types, hidden []notification.Type, before string, limit int, now time.Time) ([]notification.Notification, error) {
	n, err := toInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListNotifications(ctx, postgresgen.ListNotificationsParams{
		RecipientAccountID: recipientAccountID, Before: toNullString(before), AllTypes: len(types) == 0, Types: typeNames(types), HiddenTypes: typeNames(hidden), Now: now, PageLimit: n,
	})
	if err != nil {
		return nil, wrap(err, "list notifications")
	}
	out := make([]notification.Notification, len(rows))
	for i, row := range rows {
		out[i] = notificationFrom(row)
	}
	return out, nil
}

// CountNotificationsAfter implements notification.Repository.
func (r *NotificationRepository) CountNotificationsAfter(ctx context.Context, recipientAccountID, after string, hidden []notification.Type, now time.Time) (int64, error) {
	n, err := r.db.reader().CountNotificationsAfter(ctx, postgresgen.CountNotificationsAfterParams{
		RecipientAccountID: recipientAccountID, After: toNullString(after), HiddenTypes: typeNames(hidden), Now: now,
	})
	return n, wrap(err, "count notifications")
}

// FindReadMarker implements notification.Repository.
func (r *NotificationRepository) FindReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error) {
	row, err := r.db.reader().GetNotificationReadMarker(ctx, accountID)
	if err != nil {
		return notification.ReadMarker{}, notFound(err, appnotification.ErrNotFound, "notification read marker of "+accountID)
	}
	return notification.ReadMarker{AccountID: row.AccountID, LastReadID: row.LastReadID}, nil
}

func notificationFrom(row postgresgen.Notification) notification.Notification {
	return notification.Notification{
		ID: row.ID, RecipientAccountID: row.RecipientAccountID, Type: notification.Type(row.Type),
		SourceAccountID: fromNullString(row.SourceAccountID), SourceDomain: fromNullString(row.SourceDomain), StatusID: fromNullString(row.StatusID),
		ModerationActionID: fromNullString(row.ModerationActionID), ReportID: fromNullString(row.ReportID),
		SubjectUserID: fromNullString(row.SubjectUserID), SeveranceID: fromNullString(row.SeveranceID), AppealID: fromNullString(row.AppealID),
		CustomEmojiRequestID: fromNullString(row.CustomEmojiRequestID), SupporterSubscriptionID: fromNullString(row.SupporterSubscriptionID),
		ReactionEmoji: fromNullString(row.ReactionEmoji), ReactionCustomEmojiID: fromNullString(row.ReactionCustomEmojiID),
		AccountMoveID: fromNullString(row.AccountMoveID), AccountMoveOutcome: notification.MoveOutcome(fromNullString(row.AccountMoveOutcome)),
		ServiceClosureEventID: fromNullString(row.ServiceClosureEventID), ServiceClosureChange: notification.ClosureChange(fromNullString(row.ServiceClosureChange)), VoiceRoomID: fromNullString(row.VoiceRoomID),
		ErrorGroupChange: notification.ErrorGroupChange(fromNullString(row.ErrorGroupChange)), ErrorGroupID: fromNullString(row.ErrorGroupID), ErrorGroupHour: fromNullTime(row.ErrorGroupHour),
		DedupeKey: row.DedupeKey, CreatedAt: utc(row.CreatedAt),
	}
}

// typeNames returns the types as the queries take them; none is an empty array.
func typeNames(types []notification.Type) []string {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return names
}
