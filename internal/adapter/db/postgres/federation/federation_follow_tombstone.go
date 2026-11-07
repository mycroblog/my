// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InsertRemoteFollowTombstone implements federation.InboxRepository.
func (r *Repository) InsertRemoteFollowTombstone(ctx context.Context, tx uow.Tx, t federation.RemoteFollowTombstone) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteFollowTombstone(ctx, federationgen.InsertRemoteFollowTombstoneParams{
		ActivityUri: string(t.ActivityURI), ActorUri: string(t.ActorURI), ObjectUri: string(t.ObjectURI),
		ReceivedAt: t.ReceivedAt,
	})
	return n == 1, postgres.Wrap(err, "insert remote follow tombstone")
}

// InsertReceivedActivityAnswer implements federation.Repository.
func (r *Repository) InsertReceivedActivityAnswer(ctx context.Context, tx uow.Tx, activityURI federation.IRI, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReceivedActivityAnswer(ctx, federationgen.InsertReceivedActivityAnswerParams{ActivityUri: string(activityURI), AnsweredAt: at})
	return n == 1, postgres.Wrap(err, "insert received activity answer")
}

// DeleteReceivedActivityAnswersBefore implements federation.PurgeRepository.
func (r *Repository) DeleteReceivedActivityAnswersBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteReceivedActivityAnswersBefore(ctx, federationgen.DeleteReceivedActivityAnswersBeforeParams{AnsweredBefore: t, BatchLimit: batch})
	return n, postgres.Wrap(err, "delete received activity answers")
}

// LiveRemoteFollowTombstone implements federation.InboxRepository.
func (r *Repository) LiveRemoteFollowTombstone(ctx context.Context, tx uow.Tx, actorURI, objectURI federation.IRI) (federation.RemoteFollowTombstone, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.RemoteFollowTombstone{}, err
	}
	row, err := q.GetLiveRemoteFollowTombstone(ctx, federationgen.GetLiveRemoteFollowTombstoneParams{
		ActorUri: string(actorURI), ObjectUri: string(objectURI),
	})
	if err != nil {
		return federation.RemoteFollowTombstone{}, postgres.NotFound(err, appfederation.ErrNotFound, "follow tombstone of "+string(actorURI))
	}
	return federation.RemoteFollowTombstone{
		ActivityURI: federation.IRI(row.ActivityUri), ActorURI: federation.IRI(row.ActorUri),
		ObjectURI: federation.IRI(row.ObjectUri), ReceivedAt: postgres.UTC(row.ReceivedAt),
	}, nil
}
