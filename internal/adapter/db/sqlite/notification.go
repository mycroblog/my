// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// NotificationRepository implements notification.Repository on SQLite.
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
	n, err := q.AddErrorGroupNotice(ctx, toMillis(hour))
	return n, wrap(err, "add error group notice")
}

// ErrorGroupNotices implements notification.Repository.
func (r *NotificationRepository) ErrorGroupNotices(ctx context.Context, hour time.Time) (int64, error) {
	n, err := r.db.reader().GetErrorGroupNotices(ctx, toMillis(hour))
	return n, wrap(err, "get error group notices")
}

// DeleteErrorGroupNoticesBefore implements notification.Repository.
func (r *NotificationRepository) DeleteErrorGroupNoticesBefore(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.db.reader().DeleteErrorGroupNoticesBefore(ctx, toMillis(before))
	return n, wrap(err, "delete error group notices")
}

// InsertNotification implements notification.Repository.
func (r *NotificationRepository) InsertNotification(ctx context.Context, tx uow.Tx, n notification.Notification) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertNotification(ctx, sqlitegen.InsertNotificationParams{
		ID: n.ID, RecipientAccountID: n.RecipientAccountID, Type: string(n.Type),
		SourceAccountID: toNullString(n.SourceAccountID), SourceDomain: toNullString(n.SourceDomain), StatusID: toNullString(n.StatusID),
		ModerationActionID: toNullString(n.ModerationActionID), ReportID: toNullString(n.ReportID),
		SubjectUserID: toNullString(n.SubjectUserID), SeveranceID: toNullString(n.SeveranceID), AppealID: toNullString(n.AppealID),
		CustomEmojiRequestID: toNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: toNullString(n.SupporterSubscriptionID),
		ReactionEmoji: toNullString(n.ReactionEmoji), ReactionCustomEmojiID: toNullString(n.ReactionCustomEmojiID),
		AccountMoveID: toNullString(n.AccountMoveID), AccountMoveOutcome: toNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: toNullString(n.ServiceClosureEventID), ServiceClosureChange: toNullString(string(n.ServiceClosureChange)), VoiceRoomID: toNullString(n.VoiceRoomID),
		ErrorGroupChange: toNullString(string(n.ErrorGroupChange)), ErrorGroupID: toNullString(n.ErrorGroupID), ErrorGroupHour: toNullTime(n.ErrorGroupHour),
		DedupeKey: n.DedupeKey, CreatedAt: toMillis(n.CreatedAt),
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
	rows, err := q.InsertNotificationDismissal(ctx, sqlitegen.InsertNotificationDismissalParams{
		NotificationID: d.NotificationID, DismissedAt: toMillis(d.DismissedAt),
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
	rows, err := q.AdvanceNotificationReadMarker(ctx, sqlitegen.AdvanceNotificationReadMarkerParams{
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
	return wrap(q.UpsertNotificationExclusion(ctx, sqlitegen.UpsertNotificationExclusionParams{
		ViewerAccountID: x.ViewerAccountID, TargetAccountID: x.TargetAccountID, Reason: string(x.Reason), ExpiresAt: toNullMillis(x.ExpiresAt),
	}), "upsert notification exclusion")
}

// DeleteExclusion implements notification.Repository.
func (r *NotificationRepository) DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason notification.ExclusionReason) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteNotificationExclusion(ctx, sqlitegen.DeleteNotificationExclusionParams{
		ViewerAccountID: viewerAccountID, TargetAccountID: targetAccountID, Reason: string(reason),
	}), "delete notification exclusion")
}

// InsertDomainExclusion implements notification.Repository.
func (r *NotificationRepository) InsertDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertNotificationDomainExclusion(ctx, sqlitegen.InsertNotificationDomainExclusionParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "insert notification domain exclusion")
}

// DeleteDomainExclusion implements notification.Repository.
func (r *NotificationRepository) DeleteDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteNotificationDomainExclusion(ctx, sqlitegen.DeleteNotificationDomainExclusionParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "delete notification domain exclusion")
}

// DomainExcluded implements notification.Repository.
func (r *NotificationRepository) DomainExcluded(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	excluded, err := q.DomainExcluded(ctx, sqlitegen.DomainExcludedParams{ViewerAccountID: viewerAccountID, Domain: domain})
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
	rows, err := r.db.reader().ListNotifications(ctx, sqlitegen.ListNotificationsParams{
		RecipientAccountID: recipientAccountID, Before: before, Types: typeSet(types), HiddenTypes: typeSet(hidden), Now: toMillis(now), PageLimit: int64(limit),
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
	n, err := r.db.reader().CountNotificationsAfter(ctx, sqlitegen.CountNotificationsAfterParams{
		RecipientAccountID: recipientAccountID, After: after, HiddenTypes: typeSet(hidden), Now: toMillis(now),
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

func notificationFrom(row sqlitegen.Notification) notification.Notification {
	return notification.Notification{
		ID: row.ID, RecipientAccountID: row.RecipientAccountID, Type: notification.Type(row.Type),
		SourceAccountID: row.SourceAccountID.String, SourceDomain: row.SourceDomain.String, StatusID: row.StatusID.String,
		ModerationActionID: row.ModerationActionID.String, ReportID: row.ReportID.String,
		SubjectUserID: row.SubjectUserID.String, SeveranceID: row.SeveranceID.String, AppealID: row.AppealID.String,
		CustomEmojiRequestID: row.CustomEmojiRequestID.String, SupporterSubscriptionID: row.SupporterSubscriptionID.String,
		ReactionEmoji: row.ReactionEmoji.String, ReactionCustomEmojiID: row.ReactionCustomEmojiID.String,
		AccountMoveID: row.AccountMoveID.String, AccountMoveOutcome: notification.MoveOutcome(row.AccountMoveOutcome.String),
		ServiceClosureEventID: row.ServiceClosureEventID.String, ServiceClosureChange: notification.ClosureChange(row.ServiceClosureChange.String), VoiceRoomID: row.VoiceRoomID.String,
		ErrorGroupChange: notification.ErrorGroupChange(row.ErrorGroupChange.String), ErrorGroupID: row.ErrorGroupID.String, ErrorGroupHour: fromNullTime(row.ErrorGroupHour),
		DedupeKey: row.DedupeKey, CreatedAt: fromMillis(row.CreatedAt),
	}
}

// typeSet returns the types as the queries take them: each followed by a comma, after a leading
// comma (",follow,mention,"), or "" for none.
func typeSet(types []notification.Type) string {
	if len(types) == 0 {
		return ""
	}
	set := ","
	for _, t := range types {
		set += string(t) + ","
	}
	return set
}
