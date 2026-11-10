// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
)

// UnreadNotificationStatuses implements notification.Repository.
func (r *Repository) UnreadNotificationStatuses(ctx context.Context, recipientAccountID, after string, hidden appnotification.Hidden, now time.Time, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUnreadNotificationStatusIDs(ctx, notificationgen.ListUnreadNotificationStatusIDsParams{
		RecipientAccountID: recipientAccountID, After: after, HiddenTypes: typeSet(hidden.Types),
		ShownAnnouncements: idSet(hidden.ShownAnnouncements), Now: sqlite.ToMillis(now), Lim: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list the statuses of unread notifications")
}
