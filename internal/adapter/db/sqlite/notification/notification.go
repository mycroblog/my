// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// Repository implements notification.Repository on SQLite.
type Repository struct {
	db *moduleDB
}

var _ appnotification.Repository = (*Repository)(nil)

// NewRepository returns the notification module's repository on d.
func NewRepository(d *sqlite.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// AddErrorGroupNotice implements notification.Repository.
func (r *Repository) AddErrorGroupNotice(ctx context.Context, tx uow.Tx, hour time.Time) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.AddErrorGroupNotice(ctx, sqlite.ToMillis(hour))
	return n, sqlite.Wrap(err, "add error group notice")
}

// ErrorGroupNotices implements notification.Repository.
func (r *Repository) ErrorGroupNotices(ctx context.Context, hour time.Time) (int64, error) {
	n, err := r.db.reader().GetErrorGroupNotices(ctx, sqlite.ToMillis(hour))
	return n, sqlite.Wrap(err, "get error group notices")
}

// DeleteErrorGroupNoticesBefore implements notification.Repository.
func (r *Repository) DeleteErrorGroupNoticesBefore(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.db.reader().DeleteErrorGroupNoticesBefore(ctx, sqlite.ToMillis(before))
	return n, sqlite.Wrap(err, "delete error group notices")
}

// InsertNotification implements notification.Repository.
func (r *Repository) InsertNotification(ctx context.Context, tx uow.Tx, n notification.Notification) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertNotification(ctx, notificationgen.InsertNotificationParams{
		ID: n.ID, RecipientAccountID: n.RecipientAccountID, Type: string(n.Type),
		SourceAccountID: sqlite.ToNullString(n.SourceAccountID), SourceDomain: sqlite.ToNullString(n.SourceDomain), StatusID: sqlite.ToNullString(n.StatusID),
		ModerationActionID: sqlite.ToNullString(n.ModerationActionID), ReportID: sqlite.ToNullString(n.ReportID),
		SubjectUserID: sqlite.ToNullString(n.SubjectUserID), SeveranceID: sqlite.ToNullString(n.SeveranceID), AppealID: sqlite.ToNullString(n.AppealID),
		CustomEmojiRequestID: sqlite.ToNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: sqlite.ToNullString(n.SupporterSubscriptionID),
		ReactionEmoji: sqlite.ToNullString(n.ReactionEmoji), ReactionCustomEmojiID: sqlite.ToNullString(n.ReactionCustomEmojiID),
		AccountMoveID: sqlite.ToNullString(n.AccountMoveID), AccountMoveOutcome: sqlite.ToNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: sqlite.ToNullString(n.ServiceClosureEventID), ServiceClosureChange: sqlite.ToNullString(string(n.ServiceClosureChange)), VoiceRoomID: sqlite.ToNullString(n.VoiceRoomID),
		ErrorGroupChange: sqlite.ToNullString(string(n.ErrorGroupChange)), ErrorGroupID: sqlite.ToNullString(n.ErrorGroupID), ErrorGroupHour: sqlite.ToNullTime(n.ErrorGroupHour),
		AnnouncementID: sqlite.ToNullString(n.AnnouncementID), HookAppealID: sqlite.ToNullString(n.HookAppealID),
		AutoDeleteLimitNoticeID: sqlite.ToNullString(n.AutoDeleteLimitNoticeID), DedupeKey: n.DedupeKey, CreatedAt: sqlite.ToMillis(n.CreatedAt),
	})
	if err != nil {
		return false, sqlite.Wrap(err, "insert notification")
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
		return notification.Notification{}, sqlite.NotFound(err, appnotification.ErrNotFound, "notification "+id)
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
		DismissedAt: sqlite.ToMillis(at), RecipientAccountID: recipientAccountID, DedupeKey: dedupeKey,
	})
	return sqlite.Wrap(err, "dismiss notification by dedupe key")
}

// InsertDismissal implements notification.Repository.
func (r *Repository) InsertDismissal(ctx context.Context, tx uow.Tx, d notification.Dismissal) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	rows, err := q.InsertNotificationDismissal(ctx, notificationgen.InsertNotificationDismissalParams{
		NotificationID: d.NotificationID, DismissedAt: sqlite.ToMillis(d.DismissedAt),
	})
	if err != nil {
		return false, sqlite.Wrap(err, "insert notification dismissal")
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
		return false, sqlite.Wrap(err, "advance notification read marker")
	}
	return rows == 1, nil
}

// UpsertExclusion implements notification.Repository.
func (r *Repository) UpsertExclusion(ctx context.Context, tx uow.Tx, x notification.Exclusion) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertNotificationExclusion(ctx, notificationgen.UpsertNotificationExclusionParams{
		ViewerAccountID: x.ViewerAccountID, TargetAccountID: x.TargetAccountID, Reason: string(x.Reason), ExpiresAt: sqlite.ToNullMillis(x.ExpiresAt),
	}), "upsert notification exclusion")
}

// DeleteExclusion implements notification.Repository.
func (r *Repository) DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason notification.ExclusionReason) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteNotificationExclusion(ctx, notificationgen.DeleteNotificationExclusionParams{
		ViewerAccountID: viewerAccountID, TargetAccountID: targetAccountID, Reason: string(reason),
	}), "delete notification exclusion")
}

// InsertDomainExclusion implements notification.Repository.
func (r *Repository) InsertDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertNotificationDomainExclusion(ctx, notificationgen.InsertNotificationDomainExclusionParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "insert notification domain exclusion")
}

// DeleteDomainExclusion implements notification.Repository.
func (r *Repository) DeleteDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteNotificationDomainExclusion(ctx, notificationgen.DeleteNotificationDomainExclusionParams{
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
	return excluded, sqlite.Wrap(err, "check notification domain exclusion")
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
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	rows, err := q.NotifiedStatusesAmong(ctx, list)
	if err != nil {
		return sqlite.Wrap(err, "list the notified statuses")
	}
	var notified []string
	for _, id := range rows {
		if id.Valid {
			notified = append(notified, id.String)
		}
	}
	if len(notified) == 0 {
		return nil
	}
	notifiedList, err := sqlite.JSONList(notified)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertNotificationDeletedStatuses(ctx, notifiedList), "insert notification deleted statuses")
}

// InsertDeletedStatus implements notification.Repository.
func (r *Repository) InsertDeletedStatus(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertNotificationDeletedStatus(ctx, statusID), "insert notification deleted status")
}

// ListNotifications implements notification.Repository.
func (r *Repository) ListNotifications(ctx context.Context, recipientAccountID string, types []notification.Type, hidden appnotification.Hidden, before string, limit int, now time.Time) ([]notification.Notification, error) {
	rows, err := r.db.reader().ListNotifications(ctx, notificationgen.ListNotificationsParams{
		RecipientAccountID: recipientAccountID, Before: before, Types: typeSet(types), HiddenTypes: typeSet(hidden.Types), ShownAnnouncements: idSet(hidden.ShownAnnouncements), Now: sqlite.ToMillis(now), PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list notifications")
	}
	out := make([]notification.Notification, len(rows))
	for i, row := range rows {
		out[i] = notificationFrom(row)
	}
	return out, nil
}

// ListNotificationsAfter implements notification.Repository.
func (r *Repository) ListNotificationsAfter(ctx context.Context, recipientAccountID string, types []notification.Type, hidden appnotification.Hidden, after string, limit int, now time.Time) ([]notification.Notification, error) {
	rows, err := r.db.reader().ListNotificationsAfter(ctx, notificationgen.ListNotificationsAfterParams{
		RecipientAccountID: recipientAccountID, After: after, Types: typeSet(types), HiddenTypes: typeSet(hidden.Types), ShownAnnouncements: idSet(hidden.ShownAnnouncements), Now: sqlite.ToMillis(now), PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list notifications after")
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
		ID: id, RecipientAccountID: recipientAccountID, HiddenTypes: typeSet(hidden.Types), ShownAnnouncements: idSet(hidden.ShownAnnouncements), Now: sqlite.ToMillis(now),
	})
	return ok, sqlite.Wrap(err, "notification listed")
}

// CountNotificationsAfter implements notification.Repository.
func (r *Repository) CountNotificationsAfter(ctx context.Context, recipientAccountID, after string, hidden appnotification.Hidden, now time.Time) (int64, error) {
	n, err := r.db.reader().CountNotificationsAfter(ctx, notificationgen.CountNotificationsAfterParams{
		RecipientAccountID: recipientAccountID, After: after, HiddenTypes: typeSet(hidden.Types), ShownAnnouncements: idSet(hidden.ShownAnnouncements), Now: sqlite.ToMillis(now),
	})
	return n, sqlite.Wrap(err, "count notifications")
}

// FindReadMarker implements notification.Repository.
func (r *Repository) FindReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error) {
	row, err := r.db.reader().GetNotificationReadMarker(ctx, accountID)
	if err != nil {
		return notification.ReadMarker{}, sqlite.NotFound(err, appnotification.ErrNotFound, "notification read marker of "+accountID)
	}
	return notification.ReadMarker{AccountID: row.AccountID, LastReadID: row.LastReadID}, nil
}

func notificationFrom(row notificationgen.Notification) notification.Notification {
	return notification.Notification{
		ID: row.ID, RecipientAccountID: row.RecipientAccountID, Type: notification.Type(row.Type),
		SourceAccountID: row.SourceAccountID.String, SourceDomain: row.SourceDomain.String, StatusID: row.StatusID.String,
		ModerationActionID: row.ModerationActionID.String, ReportID: row.ReportID.String,
		SubjectUserID: row.SubjectUserID.String, SeveranceID: row.SeveranceID.String, AppealID: row.AppealID.String,
		CustomEmojiRequestID: row.CustomEmojiRequestID.String, SupporterSubscriptionID: row.SupporterSubscriptionID.String,
		ReactionEmoji: row.ReactionEmoji.String, ReactionCustomEmojiID: row.ReactionCustomEmojiID.String,
		AccountMoveID: row.AccountMoveID.String, AccountMoveOutcome: notification.MoveOutcome(row.AccountMoveOutcome.String),
		ServiceClosureEventID: row.ServiceClosureEventID.String, ServiceClosureChange: notification.ClosureChange(row.ServiceClosureChange.String), VoiceRoomID: row.VoiceRoomID.String,
		ErrorGroupChange: notification.ErrorGroupChange(row.ErrorGroupChange.String), ErrorGroupID: row.ErrorGroupID.String, ErrorGroupHour: sqlite.FromNullTime(row.ErrorGroupHour),
		AnnouncementID: row.AnnouncementID.String, HookAppealID: row.HookAppealID.String,
		AutoDeleteLimitNoticeID: row.AutoDeleteLimitNoticeID.String, DedupeKey: row.DedupeKey, CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}
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
			Enabled: sqlite.BoolToInt(s.Enabled), UpdatedAt: sqlite.ToMillis(at),
		})
		if err != nil {
			return sqlite.Wrap(err, "upsert notification setting")
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
		return nil, sqlite.Wrap(err, "list notification settings")
	}
	out := make([]notification.Setting, len(rows))
	for i, row := range rows {
		out[i] = notification.Setting{
			AccountID: row.AccountID, Group: notification.Group(row.NotificationGroup), Channel: notification.Channel(row.Channel),
			Enabled: row.Enabled != 0,
		}
	}
	return out, nil
}

// typeSet returns the types as the queries take them: each followed by a comma, after a leading
// comma (",follow,mention,"), or "" for none.
// idSet is typeSet for IDs.
func idSet(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return "," + strings.Join(ids, ",") + ","
}

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
	type row struct {
		ID        string `json:"id"`
		Recipient string `json:"recipient"`
	}
	rows := make([]row, len(ids))
	for i, id := range ids {
		rows[i] = row{ID: id, Recipient: recipients[i]}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	arg := notificationgen.InsertNotificationsForParams{
		Type:            string(n.Type),
		SourceAccountID: sqlite.ToNullString(n.SourceAccountID), SourceDomain: sqlite.ToNullString(n.SourceDomain), StatusID: sqlite.ToNullString(n.StatusID),
		ModerationActionID: sqlite.ToNullString(n.ModerationActionID), ReportID: sqlite.ToNullString(n.ReportID),
		SubjectUserID: sqlite.ToNullString(n.SubjectUserID), SeveranceID: sqlite.ToNullString(n.SeveranceID), AppealID: sqlite.ToNullString(n.AppealID),
		CustomEmojiRequestID: sqlite.ToNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: sqlite.ToNullString(n.SupporterSubscriptionID),
		ReactionEmoji: sqlite.ToNullString(n.ReactionEmoji), ReactionCustomEmojiID: sqlite.ToNullString(n.ReactionCustomEmojiID),
		AccountMoveID: sqlite.ToNullString(n.AccountMoveID), AccountMoveOutcome: sqlite.ToNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: sqlite.ToNullString(n.ServiceClosureEventID), ServiceClosureChange: sqlite.ToNullString(string(n.ServiceClosureChange)), VoiceRoomID: sqlite.ToNullString(n.VoiceRoomID),
		ErrorGroupChange: sqlite.ToNullString(string(n.ErrorGroupChange)), ErrorGroupID: sqlite.ToNullString(n.ErrorGroupID), ErrorGroupHour: sqlite.ToNullTime(n.ErrorGroupHour),
		AnnouncementID: sqlite.ToNullString(n.AnnouncementID), HookAppealID: sqlite.ToNullString(n.HookAppealID),
		AutoDeleteLimitNoticeID: sqlite.ToNullString(n.AutoDeleteLimitNoticeID), DedupeKey: n.DedupeKey, CreatedAt: sqlite.ToMillis(n.CreatedAt),
		Recipients: string(list),
	}
	inserted, err := q.InsertNotificationsFor(ctx, arg)
	if err != nil {
		return nil, sqlite.Wrap(err, "insert notifications")
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
	type row struct {
		ID              string `json:"id"`
		Recipient       string `json:"recipient"`
		SeveranceID     string `json:"severance_id"`
		DedupeKey       string `json:"dedupe_key"`
		SourceAccountID string `json:"source_account_id"`
		SourceDomain    string `json:"source_domain"`
		CreatedAt       int64  `json:"created_at"`
	}
	rows := make([]row, len(ns))
	for i, m := range ns {
		rows[i] = row{ID: m.ID, Recipient: m.RecipientAccountID, SeveranceID: m.SeveranceID, DedupeKey: m.DedupeKey,
			SourceAccountID: m.SourceAccountID, SourceDomain: m.SourceDomain, CreatedAt: sqlite.ToMillis(m.CreatedAt)}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	arg := notificationgen.InsertNotificationsForEachParams{
		Type:               string(n.Type),
		StatusID:           sqlite.ToNullString(n.StatusID),
		ModerationActionID: sqlite.ToNullString(n.ModerationActionID), ReportID: sqlite.ToNullString(n.ReportID),
		SubjectUserID: sqlite.ToNullString(n.SubjectUserID), AppealID: sqlite.ToNullString(n.AppealID),
		CustomEmojiRequestID: sqlite.ToNullString(n.CustomEmojiRequestID), SupporterSubscriptionID: sqlite.ToNullString(n.SupporterSubscriptionID),
		ReactionEmoji: sqlite.ToNullString(n.ReactionEmoji), ReactionCustomEmojiID: sqlite.ToNullString(n.ReactionCustomEmojiID),
		AccountMoveID: sqlite.ToNullString(n.AccountMoveID), AccountMoveOutcome: sqlite.ToNullString(string(n.AccountMoveOutcome)),
		ServiceClosureEventID: sqlite.ToNullString(n.ServiceClosureEventID), ServiceClosureChange: sqlite.ToNullString(string(n.ServiceClosureChange)), VoiceRoomID: sqlite.ToNullString(n.VoiceRoomID),
		ErrorGroupChange: sqlite.ToNullString(string(n.ErrorGroupChange)), ErrorGroupID: sqlite.ToNullString(n.ErrorGroupID), ErrorGroupHour: sqlite.ToNullTime(n.ErrorGroupHour),
		AnnouncementID: sqlite.ToNullString(n.AnnouncementID), HookAppealID: sqlite.ToNullString(n.HookAppealID),
		AutoDeleteLimitNoticeID: sqlite.ToNullString(n.AutoDeleteLimitNoticeID),
		Recipients:              string(list),
	}
	inserted, err := q.InsertNotificationsForEach(ctx, arg)
	if err != nil {
		return nil, sqlite.Wrap(err, "insert notifications")
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
	list, err := sqlite.JSONList(viewerAccountIDs)
	if err != nil {
		return nil, err
	}
	ids, err := q.DomainsExcludedAmong(ctx, notificationgen.DomainsExcludedAmongParams{ViewerAccountIds: list, Domain: domain})
	if err != nil {
		return nil, sqlite.Wrap(err, "check notification domain exclusions")
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
	list, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	all, err := notificationSettings(q.ListNotificationSettingsOf(ctx, list))
	if err != nil {
		return nil, err
	}
	for _, st := range all {
		out[st.AccountID] = append(out[st.AccountID], st)
	}
	return out, nil
}
