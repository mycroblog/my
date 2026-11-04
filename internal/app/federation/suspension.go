// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// RegisterSuspensionProjectors subscribes the federation module to the moderation module's
// suspensions of local accounts (federation.md §11.1, ADR-0058): each change of a local
// account's suspension (moderation.account_suspension_changed: suspend or ban, and the lift or
// the expiry of the last one) records, in the transaction of the action, an Update of the actor
// and the JobFanOut that delivers it to the actor's followers and every known shared inbox,
// where a Delete of the actor goes. The fan-out builds the actor's document as it is then:
// emptied and marked suspended while a suspension is in effect, the profile otherwise
// (FanOutDeps.Suspensions). A rolled-back action records nothing. d.Accounts tells local
// accounts from remote ones; the suspension of a remote account sends nothing here, and neither
// does a change on an account that its owner deactivated, which stays hidden either way
// (ADR-0089 §17).
func RegisterSuspensionProjectors(r *uow.Projectors, d RecorderDeps) {
	rec := recorder{d: d}
	r.Register(appmoderation.EventAccountSuspensionChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appmoderation.AccountSuspensionChanged)
		if !ok {
			return nil
		}
		local, err := d.Accounts.IsLocal(ctx, tx, ev.AccountID)
		if err != nil || !local {
			return err
		}
		if d.Hidden != nil {
			deactivated, err := d.Hidden.DeactivatedTx(ctx, tx, ev.AccountID)
			if err != nil || deactivated {
				return err
			}
		}
		return rec.recordWith(ctx, tx, federation.OutboundActivity{
			Type: federation.ActivityUpdate, ActorAccountID: ev.AccountID,
			ObjectKind: federation.OutboundAccount, ObjectID: ev.AccountID, CreatedAt: ev.ChangedAt,
		}, fanOutPayload{SharedInboxes: true})
	})
}
