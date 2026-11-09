// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InsertOutboundActivities implements federation.Repository.
func (r *Repository) InsertOutboundActivities(ctx context.Context, tx uow.Tx, as []federation.OutboundActivity) error {
	if len(as) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var p federationgen.InsertOutboundActivitiesParams
	for _, a := range as {
		p.Ids = append(p.Ids, a.ID)
		p.ActivityUris = append(p.ActivityUris, string(a.ActivityURI))
		p.Types = append(p.Types, string(a.Type))
		p.ActorAccountIds = append(p.ActorAccountIds, a.ActorAccountID)
		p.ObjectTypes = append(p.ObjectTypes, string(a.ObjectKind))
		p.ObjectIds = append(p.ObjectIds, a.ObjectID)
		p.CreatedAts = append(p.CreatedAts, a.CreatedAt)
	}
	return postgres.Wrap(q.InsertOutboundActivities(ctx, p), "insert outbound activities")
}
