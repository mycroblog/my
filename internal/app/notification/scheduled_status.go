// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// registerScheduledStatusProjector registers the projector of the scheduled posts that could not
// be published (ADR-0156 §8): a scheduled_status_failed to the author, in the transaction that
// records the failure, once per schedule. It comes from the instance and has no source account;
// its Web Push message says why in plain words. It is never emailed.
func registerScheduledStatusProjector(r *uow.Projectors, p *projector) {
	r.Register(appcontent.EventDraftScheduleFailed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		f := e.(appcontent.DraftScheduleFailed)
		return p.notifyWith(ctx, tx, notification.Notification{
			RecipientAccountID: f.AccountID, Type: notification.TypeScheduledStatusFailed, DraftScheduleID: f.Schedule.ID,
			DedupeKey: notification.ScheduledStatusFailedKey(f.Schedule.ID), CreatedAt: f.Outcome.OccurredAt,
		}, func(job *pushJob) {
			job.DraftFailureReason = string(f.Outcome.FailureReason)
		})
	})
}
