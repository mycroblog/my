// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"math"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// registerAutoDeleteProjector registers the projector of the notices of the server maximum of the
// automatic deletion (ADR-0151 §7): an auto_delete_limit to the account that the maximum newly
// binds, in the transaction that records the notice, once per notice. Its Web Push message and
// its email tell the maximum and the days of the grace period as plain numbers (the namespaces
// that Go formats take no date), and link to the settings of the automatic deletion, whose
// preview shows what goes.
func registerAutoDeleteProjector(r *uow.Projectors, p *projector) {
	r.Register(appcontent.EventAutoDeleteLimitNoticed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		n := e.(appcontent.AutoDeleteLimitNoticed).Notice
		daysLeft := int(math.Ceil(n.EnforceFrom.Sub(n.NoticedAt).Hours() / 24))
		return p.notifyWith(ctx, tx, notification.Notification{
			RecipientAccountID: n.AccountID, Type: notification.TypeAutoDeleteLimit, AutoDeleteLimitNoticeID: n.ID,
			DedupeKey: notification.AutoDeleteLimitKey(n.ID), CreatedAt: n.NoticedAt,
		}, func(job *pushJob) {
			job.AutoDeleteMaxAgeDays, job.AutoDeleteDaysLeft = n.MaxAgeDays, daysLeft
		})
	})
}
