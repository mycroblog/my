// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
)

// UnreadNotificationStatuses implements notification.Repository.
func (r *Repository) UnreadNotificationStatuses(ctx context.Context, recipientAccountID, after string, hidden appnotification.Hidden, now time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUnreadNotificationStatusIDs(ctx, notificationgen.ListUnreadNotificationStatusIDsParams{
		RecipientAccountID: recipientAccountID, After: postgres.ToNullString(after), HiddenTypes: typeNames(hidden.Types),
		ShownAnnouncements: idList(hidden.ShownAnnouncements), Now: now, Lim: int32(limit),
	})
	return ids, postgres.Wrap(err, "list the statuses of unread notifications")
}
