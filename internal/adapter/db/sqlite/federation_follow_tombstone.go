// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InsertRemoteFollowTombstone implements federation.InboxRepository.
func (r *FederationRepository) InsertRemoteFollowTombstone(ctx context.Context, tx uow.Tx, t federation.RemoteFollowTombstone) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteFollowTombstone(ctx, sqlitegen.InsertRemoteFollowTombstoneParams{
		ActivityUri: string(t.ActivityURI), ActorUri: string(t.ActorURI), ObjectUri: string(t.ObjectURI),
		ReceivedAt: toMillis(t.ReceivedAt),
	})
	return n == 1, wrap(err, "insert remote follow tombstone")
}

// LiveRemoteFollowTombstone implements federation.InboxRepository.
func (r *FederationRepository) LiveRemoteFollowTombstone(ctx context.Context, tx uow.Tx, actorURI, objectURI federation.IRI) (federation.RemoteFollowTombstone, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.RemoteFollowTombstone{}, err
	}
	row, err := q.GetLiveRemoteFollowTombstone(ctx, sqlitegen.GetLiveRemoteFollowTombstoneParams{
		ActorUri: string(actorURI), ObjectUri: string(objectURI),
	})
	if err != nil {
		return federation.RemoteFollowTombstone{}, notFound(err, appfederation.ErrNotFound, "follow tombstone of "+string(actorURI))
	}
	return federation.RemoteFollowTombstone{
		ActivityURI: federation.IRI(row.ActivityUri), ActorURI: federation.IRI(row.ActorUri),
		ObjectURI: federation.IRI(row.ObjectUri), ReceivedAt: fromMillis(row.ReceivedAt),
	}, nil
}
