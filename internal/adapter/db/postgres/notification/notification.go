// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// Repository implements notification.Repository on PostgreSQL.
type Repository struct {
	db *moduleDB
}

var _ appnotification.Repository = (*Repository)(nil)

// NewRepository returns the notification module's repository on d.
func NewRepository(d *postgres.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// AddErrorGroupNotice implements notification.Repository.
func (r *Repository) AddErrorGroupNotice(ctx context.Context, tx uow.Tx, hour time.Time) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.AddErrorGroupNotice(ctx, hour)
	return n, postgres.Wrap(err, "add error group notice")
}

// ErrorGroupNotices implements notification.Repository.
func (r *Repository) ErrorGroupNotices(ctx context.Context, hour time.Time) (int64, error) {
	n, err := r.db.reader().GetErrorGroupNotices(ctx, hour)
	return n, postgres.Wrap(err, "get error group notices")
}

// DeleteErrorGroupNoticesBefore implements notification.Repository.
func (r *Repository) DeleteErrorGroupNoticesBefore(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.db.reader().DeleteErrorGroupNoticesBefore(ctx, before)
	return n, postgres.Wrap(err, "delete error group notices")
}

// InsertNotification implements notification.Repository.
func (r *Repository) InsertNotification(ctx context.Context, tx uow.Tx, n notification.Notification) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertNotification(ctx, notificationgen.InsertNotificationParams{
		ID: n.ID, RecipientAccountID: n.RecipientAccountID, Type: string(n.Type),
		SourceAccountID: postgres.ToNullString(n.SourceAccountID), SourceDomain: postgres.ToNullString(n.SourceDomain), StatusID: postgres.ToNullString(n.StatusID),
		ModerationActionID: postgres.ToNullString(n.ModerationActionID), ReportID: postgres.ToNullString(n.ReportID),
		SubjectUserID: postgres.ToNullString(n.SubjectUserID), SeveranceID: postgres.ToNullString(n.SeveranceID), AppealID: postgres.ToNullString(n.AppealID),
		CustomEmojiRequestID: postgres.ToNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: postgres.ToNullString(n.SupporterSubscriptionID),
		ReactionEmoji: postgres.ToNullString(n.ReactionEmoji), ReactionCustomEmojiID: postgres.ToNullString(n.ReactionCustomEmojiID),
		AccountMoveID: postgres.ToNullString(n.AccountMoveID), AccountMoveOutcome: postgres.ToNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: postgres.ToNullString(n.ServiceClosureEventID), ServiceClosureChange: postgres.ToNullString(string(n.ServiceClosureChange)), VoiceRoomID: postgres.ToNullString(n.VoiceRoomID),
		ErrorGroupChange: postgres.ToNullString(string(n.ErrorGroupChange)), ErrorGroupID: postgres.ToNullString(n.ErrorGroupID), ErrorGroupHour: postgres.ToNullTime(n.ErrorGroupHour),
		AnnouncementID: postgres.ToNullString(n.AnnouncementID), HookAppealID: postgres.ToNullString(n.HookAppealID),
		AutoDeleteLimitNoticeID: postgres.ToNullString(n.AutoDeleteLimitNoticeID), DraftScheduleID: postgres.ToNullString(n.DraftScheduleID), DedupeKey: n.DedupeKey, CreatedAt: n.CreatedAt,
	})
	if err != nil {
		return false, postgres.Wrap(err, "insert notification")
	}
	return rows == 1, nil
}

// Notification implements notification.Repository.
func (r *Repository) Notification(ctx context.Context, tx uow.Tx, id string) (notification.Notification, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return notification.Notification{}, err
	}
	row, err := q.GetNotification(ctx, id)
	if err != nil {
		return notification.Notification{}, postgres.NotFound(err, appnotification.ErrNotFound, "notification "+id)
	}
	return notificationFrom(row), nil
}

// DismissByDedupeKey implements notification.Repository.
func (r *Repository) DismissByDedupeKey(ctx context.Context, tx uow.Tx, recipientAccountID, dedupeKey string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	_, err = q.DismissNotificationByDedupeKey(ctx, notificationgen.DismissNotificationByDedupeKeyParams{
		DismissedAt: at, RecipientAccountID: recipientAccountID, DedupeKey: dedupeKey,
	})
	return postgres.Wrap(err, "dismiss notification by dedupe key")
}

// InsertDismissal implements notification.Repository.
func (r *Repository) InsertDismissal(ctx context.Context, tx uow.Tx, d notification.Dismissal) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertNotificationDismissal(ctx, notificationgen.InsertNotificationDismissalParams{
		NotificationID: d.NotificationID, DismissedAt: d.DismissedAt,
	})
	if err != nil {
		return false, postgres.Wrap(err, "insert notification dismissal")
	}
	return rows == 1, nil
}

// AdvanceReadMarker implements notification.Repository.
func (r *Repository) AdvanceReadMarker(ctx context.Context, tx uow.Tx, m notification.ReadMarker) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.AdvanceNotificationReadMarker(ctx, notificationgen.AdvanceNotificationReadMarkerParams{
		AccountID: m.AccountID, LastReadID: m.LastReadID,
	})
	if err != nil {
		return false, postgres.Wrap(err, "advance notification read marker")
	}
	return rows == 1, nil
}

// UpsertExclusion implements notification.Repository.
func (r *Repository) UpsertExclusion(ctx context.Context, tx uow.Tx, x notification.Exclusion) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.UpsertNotificationExclusion(ctx, notificationgen.UpsertNotificationExclusionParams{
		ViewerAccountID: x.ViewerAccountID, TargetAccountID: x.TargetAccountID, Reason: string(x.Reason), ExpiresAt: x.ExpiresAt,
	}), "upsert notification exclusion")
}

// DeleteExclusion implements notification.Repository.
func (r *Repository) DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason notification.ExclusionReason) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteNotificationExclusion(ctx, notificationgen.DeleteNotificationExclusionParams{
		ViewerAccountID: viewerAccountID, TargetAccountID: targetAccountID, Reason: string(reason),
	}), "delete notification exclusion")
}

// InsertDomainExclusion implements notification.Repository.
func (r *Repository) InsertDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertNotificationDomainExclusion(ctx, notificationgen.InsertNotificationDomainExclusionParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "insert notification domain exclusion")
}

// DeleteDomainExclusion implements notification.Repository.
func (r *Repository) DeleteDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteNotificationDomainExclusion(ctx, notificationgen.DeleteNotificationDomainExclusionParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "delete notification domain exclusion")
}

// DomainExcluded implements notification.Repository.
func (r *Repository) DomainExcluded(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	excluded, err := q.DomainExcluded(ctx, notificationgen.DomainExcludedParams{ViewerAccountID: viewerAccountID, Domain: domain})
	return excluded, postgres.Wrap(err, "check notification domain exclusion")
}

// RecordDeletedStatuses implements notification.Repository.
func (r *Repository) RecordDeletedStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	if len(statusIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	list := statusIDs
	rows, err := q.NotifiedStatusesAmong(ctx, list)
	if err != nil {
		return postgres.Wrap(err, "list the notified statuses")
	}
	var notified []string
	for _, id := range rows {
		if id != nil {
			notified = append(notified, *id)
		}
	}
	if len(notified) == 0 {
		return nil
	}
	notifiedList := notified
	return postgres.Wrap(q.InsertNotificationDeletedStatuses(ctx, notifiedList), "insert notification deleted statuses")
}

// InsertDeletedStatus implements notification.Repository.
func (r *Repository) InsertDeletedStatus(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertNotificationDeletedStatus(ctx, statusID), "insert notification deleted status")
}

// ListNotifications implements notification.Repository.
func (r *Repository) ListNotifications(ctx context.Context, recipientAccountID string, types []notification.Type, hidden appnotification.Hidden, before string, limit int, now time.Time) ([]notification.Notification, error) {
	n, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListNotifications(ctx, notificationgen.ListNotificationsParams{
		RecipientAccountID: recipientAccountID, Before: postgres.ToNullString(before), AllTypes: len(types) == 0, Types: typeNames(types), HiddenTypes: typeNames(hidden.Types), ShownAnnouncements: idList(hidden.ShownAnnouncements), Now: now, PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list notifications")
	}
	out := make([]notification.Notification, len(rows))
	for i, row := range rows {
		out[i] = notificationFrom(row)
	}
	return out, nil
}

// ListNotificationsAfter implements notification.Repository.
func (r *Repository) ListNotificationsAfter(ctx context.Context, recipientAccountID string, types []notification.Type, hidden appnotification.Hidden, after string, limit int, now time.Time) ([]notification.Notification, error) {
	n, err := postgres.ToInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListNotificationsAfter(ctx, notificationgen.ListNotificationsAfterParams{
		RecipientAccountID: recipientAccountID, After: after, AllTypes: len(types) == 0, Types: typeNames(types), HiddenTypes: typeNames(hidden.Types), ShownAnnouncements: idList(hidden.ShownAnnouncements), Now: now, PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list notifications after")
	}
	out := make([]notification.Notification, len(rows))
	for i, row := range rows {
		out[i] = notificationFrom(row)
	}
	return out, nil
}

// NotificationListed implements notification.Repository.
func (r *Repository) NotificationListed(ctx context.Context, recipientAccountID, id string, hidden appnotification.Hidden, now time.Time) (bool, error) {
	ok, err := r.db.reader().NotificationListed(ctx, notificationgen.NotificationListedParams{
		ID: id, RecipientAccountID: recipientAccountID, HiddenTypes: typeNames(hidden.Types), ShownAnnouncements: idList(hidden.ShownAnnouncements), Now: now,
	})
	return ok, postgres.Wrap(err, "notification listed")
}

// CountNotificationsAfter implements notification.Repository.
func (r *Repository) CountNotificationsAfter(ctx context.Context, recipientAccountID, after string, hidden appnotification.Hidden, now time.Time) (int64, error) {
	n, err := r.db.reader().CountNotificationsAfter(ctx, notificationgen.CountNotificationsAfterParams{
		RecipientAccountID: recipientAccountID, After: postgres.ToNullString(after), HiddenTypes: typeNames(hidden.Types), ShownAnnouncements: idList(hidden.ShownAnnouncements), Now: now,
	})
	return n, postgres.Wrap(err, "count notifications")
}

// FindReadMarker implements notification.Repository.
func (r *Repository) FindReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error) {
	row, err := r.db.reader().GetNotificationReadMarker(ctx, accountID)
	if err != nil {
		return notification.ReadMarker{}, postgres.NotFound(err, appnotification.ErrNotFound, "notification read marker of "+accountID)
	}
	return notification.ReadMarker{AccountID: row.AccountID, LastReadID: row.LastReadID}, nil
}

// SaveNotificationSettings implements notification.Repository.
func (r *Repository) SaveNotificationSettings(ctx context.Context, tx uow.Tx, settings []notification.Setting, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, s := range settings {
		err := q.UpsertNotificationSetting(ctx, notificationgen.UpsertNotificationSettingParams{
			AccountID: s.AccountID, NotificationGroup: string(s.Group), Channel: string(s.Channel),
			Enabled: s.Enabled, UpdatedAt: at,
		})
		if err != nil {
			return postgres.Wrap(err, "upsert notification setting")
		}
	}
	return nil
}

// NotificationSettingsTx implements notification.Repository.
func (r *Repository) NotificationSettingsTx(ctx context.Context, tx uow.Tx, accountID string) ([]notification.Setting, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	return notificationSettings(q.ListNotificationSettings(ctx, accountID))
}

// NotificationSettings implements notification.Repository.
func (r *Repository) NotificationSettings(ctx context.Context, accountID string) ([]notification.Setting, error) {
	return notificationSettings(r.db.reader().ListNotificationSettings(ctx, accountID))
}

func notificationSettings(rows []notificationgen.NotificationSetting, err error) ([]notification.Setting, error) {
	if err != nil {
		return nil, postgres.Wrap(err, "list notification settings")
	}
	out := make([]notification.Setting, len(rows))
	for i, row := range rows {
		out[i] = notification.Setting{
			AccountID: row.AccountID, Group: notification.Group(row.NotificationGroup), Channel: notification.Channel(row.Channel),
			Enabled: row.Enabled,
		}
	}
	return out, nil
}

func notificationFrom(row notificationgen.Notification) notification.Notification {
	return notification.Notification{
		ID: row.ID, RecipientAccountID: row.RecipientAccountID, Type: notification.Type(row.Type),
		SourceAccountID: postgres.FromNullString(row.SourceAccountID), SourceDomain: postgres.FromNullString(row.SourceDomain), StatusID: postgres.FromNullString(row.StatusID),
		ModerationActionID: postgres.FromNullString(row.ModerationActionID), ReportID: postgres.FromNullString(row.ReportID),
		SubjectUserID: postgres.FromNullString(row.SubjectUserID), SeveranceID: postgres.FromNullString(row.SeveranceID), AppealID: postgres.FromNullString(row.AppealID),
		CustomEmojiRequestID: postgres.FromNullString(row.CustomEmojiRequestID), SupporterSubscriptionID: postgres.FromNullString(row.SupporterSubscriptionID),
		ReactionEmoji: postgres.FromNullString(row.ReactionEmoji), ReactionCustomEmojiID: postgres.FromNullString(row.ReactionCustomEmojiID),
		AccountMoveID: postgres.FromNullString(row.AccountMoveID), AccountMoveOutcome: notification.MoveOutcome(postgres.FromNullString(row.AccountMoveOutcome)),
		ServiceClosureEventID: postgres.FromNullString(row.ServiceClosureEventID), ServiceClosureChange: notification.ClosureChange(postgres.FromNullString(row.ServiceClosureChange)), VoiceRoomID: postgres.FromNullString(row.VoiceRoomID),
		ErrorGroupChange: notification.ErrorGroupChange(postgres.FromNullString(row.ErrorGroupChange)), ErrorGroupID: postgres.FromNullString(row.ErrorGroupID), ErrorGroupHour: postgres.FromNullTime(row.ErrorGroupHour),
		AnnouncementID: postgres.FromNullString(row.AnnouncementID), HookAppealID: postgres.FromNullString(row.HookAppealID),
		AutoDeleteLimitNoticeID: postgres.FromNullString(row.AutoDeleteLimitNoticeID), DraftScheduleID: postgres.FromNullString(row.DraftScheduleID), DedupeKey: row.DedupeKey, CreatedAt: postgres.UTC(row.CreatedAt),
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

// idList returns ids, empty rather than nil, for a uuid[] argument.
func idList(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

// InsertNotificationsFor implements notification.Repository.
func (r *Repository) InsertNotificationsFor(ctx context.Context, tx uow.Tx, n notification.Notification, ids, recipients []string) (map[string]bool, error) {
	out := make(map[string]bool, len(recipients))
	if len(recipients) == 0 {
		return out, nil
	}
	if len(ids) != len(recipients) {
		return nil, fmt.Errorf("insert notifications: %d IDs for %d recipients", len(ids), len(recipients))
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	arg := notificationgen.InsertNotificationsForParams{
		Type:            string(n.Type),
		SourceAccountID: postgres.ToNullString(n.SourceAccountID), SourceDomain: postgres.ToNullString(n.SourceDomain), StatusID: postgres.ToNullString(n.StatusID),
		ModerationActionID: postgres.ToNullString(n.ModerationActionID), ReportID: postgres.ToNullString(n.ReportID),
		SubjectUserID: postgres.ToNullString(n.SubjectUserID), SeveranceID: postgres.ToNullString(n.SeveranceID), AppealID: postgres.ToNullString(n.AppealID),
		CustomEmojiRequestID: postgres.ToNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: postgres.ToNullString(n.SupporterSubscriptionID),
		ReactionEmoji: postgres.ToNullString(n.ReactionEmoji), ReactionCustomEmojiID: postgres.ToNullString(n.ReactionCustomEmojiID),
		AccountMoveID: postgres.ToNullString(n.AccountMoveID), AccountMoveOutcome: postgres.ToNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: postgres.ToNullString(n.ServiceClosureEventID), ServiceClosureChange: postgres.ToNullString(string(n.ServiceClosureChange)), VoiceRoomID: postgres.ToNullString(n.VoiceRoomID),
		ErrorGroupChange: postgres.ToNullString(string(n.ErrorGroupChange)), ErrorGroupID: postgres.ToNullString(n.ErrorGroupID), ErrorGroupHour: postgres.ToNullTime(n.ErrorGroupHour),
		AnnouncementID: postgres.ToNullString(n.AnnouncementID), HookAppealID: postgres.ToNullString(n.HookAppealID),
		AutoDeleteLimitNoticeID: postgres.ToNullString(n.AutoDeleteLimitNoticeID), DraftScheduleID: postgres.ToNullString(n.DraftScheduleID), DedupeKey: n.DedupeKey, CreatedAt: n.CreatedAt,
	}
	for i, id := range ids {
		arg.Ids, arg.RecipientAccountIds = append(arg.Ids, id), append(arg.RecipientAccountIds, recipients[i])
	}
	inserted, err := q.InsertNotificationsFor(ctx, arg)
	if err != nil {
		return nil, postgres.Wrap(err, "insert notifications")
	}
	for _, id := range inserted {
		out[id] = true
	}
	return out, nil
}

// InsertNotificationsForEach implements notification.Repository.
func (r *Repository) InsertNotificationsForEach(ctx context.Context, tx uow.Tx, ns []notification.Notification) (map[string]bool, error) {
	out := make(map[string]bool, len(ns))
	if len(ns) == 0 {
		return out, nil
	}
	n := ns[0]
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	arg := notificationgen.InsertNotificationsForEachParams{
		Type:               string(n.Type),
		StatusID:           postgres.ToNullString(n.StatusID),
		ModerationActionID: postgres.ToNullString(n.ModerationActionID), ReportID: postgres.ToNullString(n.ReportID),
		SubjectUserID: postgres.ToNullString(n.SubjectUserID), AppealID: postgres.ToNullString(n.AppealID),
		CustomEmojiRequestID: postgres.ToNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: postgres.ToNullString(n.SupporterSubscriptionID),
		ReactionEmoji: postgres.ToNullString(n.ReactionEmoji), ReactionCustomEmojiID: postgres.ToNullString(n.ReactionCustomEmojiID),
		AccountMoveID: postgres.ToNullString(n.AccountMoveID), AccountMoveOutcome: postgres.ToNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: postgres.ToNullString(n.ServiceClosureEventID), ServiceClosureChange: postgres.ToNullString(string(n.ServiceClosureChange)), VoiceRoomID: postgres.ToNullString(n.VoiceRoomID),
		ErrorGroupChange: postgres.ToNullString(string(n.ErrorGroupChange)), ErrorGroupID: postgres.ToNullString(n.ErrorGroupID), ErrorGroupHour: postgres.ToNullTime(n.ErrorGroupHour),
		AnnouncementID: postgres.ToNullString(n.AnnouncementID), HookAppealID: postgres.ToNullString(n.HookAppealID),
		AutoDeleteLimitNoticeID: postgres.ToNullString(n.AutoDeleteLimitNoticeID), DraftScheduleID: postgres.ToNullString(n.DraftScheduleID),
	}
	for _, m := range ns {
		arg.Ids, arg.RecipientAccountIds = append(arg.Ids, m.ID), append(arg.RecipientAccountIds, m.RecipientAccountID)
		arg.SeveranceIds, arg.DedupeKeys = append(arg.SeveranceIds, m.SeveranceID), append(arg.DedupeKeys, m.DedupeKey)
		arg.SourceAccountIds, arg.SourceDomains = append(arg.SourceAccountIds, m.SourceAccountID), append(arg.SourceDomains, m.SourceDomain)
		arg.CreatedAts = append(arg.CreatedAts, m.CreatedAt)
	}
	inserted, err := q.InsertNotificationsForEach(ctx, arg)
	if err != nil {
		return nil, postgres.Wrap(err, "insert notifications")
	}
	for _, id := range inserted {
		out[id] = true
	}
	return out, nil
}

// DomainExcludedAmong implements notification.Repository.
func (r *Repository) DomainExcludedAmong(ctx context.Context, tx uow.Tx, viewerAccountIDs []string, domain string) (map[string]bool, error) {
	out := make(map[string]bool, len(viewerAccountIDs))
	if len(viewerAccountIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.DomainsExcludedAmong(ctx, notificationgen.DomainsExcludedAmongParams{ViewerAccountIds: viewerAccountIDs, Domain: domain})
	if err != nil {
		return nil, postgres.Wrap(err, "check notification domain exclusions")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// NotificationSettingsOfTx implements notification.Repository.
func (r *Repository) NotificationSettingsOfTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string][]notification.Setting, error) {
	out := make(map[string][]notification.Setting, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	all, err := notificationSettings(q.ListNotificationSettingsOf(ctx, accountIDs))
	if err != nil {
		return nil, err
	}
	for _, st := range all {
		out[st.AccountID] = append(out[st.AccountID], st)
	}
	return out, nil
}
