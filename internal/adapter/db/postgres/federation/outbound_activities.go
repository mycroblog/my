// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InsertOutboundDocuments implements federation.Repository.
func (r *Repository) InsertOutboundDocuments(ctx context.Context, tx uow.Tx, documents map[string][]byte, builtAt time.Time) error {
	if len(documents) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	p := federationgen.InsertOutboundActivityDocumentsParams{BuiltAt: builtAt}
	for _, id := range slices.Sorted(maps.Keys(documents)) {
		p.OutboundActivityIds, p.Documents = append(p.OutboundActivityIds, id), append(p.Documents, documents[id])
	}
	return postgres.Wrap(q.InsertOutboundActivityDocuments(ctx, p), "insert outbound activity documents")
}

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
