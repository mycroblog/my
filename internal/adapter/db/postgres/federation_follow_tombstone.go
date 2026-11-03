// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
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
	n, err := q.InsertRemoteFollowTombstone(ctx, postgresgen.InsertRemoteFollowTombstoneParams{
		ActivityUri: string(t.ActivityURI), ActorUri: string(t.ActorURI), ObjectUri: string(t.ObjectURI),
		ReceivedAt: t.ReceivedAt,
	})
	return n == 1, wrap(err, "insert remote follow tombstone")
}

// InsertReceivedActivityAnswer implements federation.Repository.
func (r *FederationRepository) InsertReceivedActivityAnswer(ctx context.Context, tx uow.Tx, activityURI federation.IRI, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReceivedActivityAnswer(ctx, postgresgen.InsertReceivedActivityAnswerParams{ActivityUri: string(activityURI), AnsweredAt: at})
	return n == 1, wrap(err, "insert received activity answer")
}

// DeleteReceivedActivityAnswersBefore implements federation.PurgeRepository.
func (r *FederationRepository) DeleteReceivedActivityAnswersBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := toInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteReceivedActivityAnswersBefore(ctx, postgresgen.DeleteReceivedActivityAnswersBeforeParams{AnsweredBefore: t, BatchLimit: batch})
	return n, wrap(err, "delete received activity answers")
}

// LiveRemoteFollowTombstone implements federation.InboxRepository.
func (r *FederationRepository) LiveRemoteFollowTombstone(ctx context.Context, tx uow.Tx, actorURI, objectURI federation.IRI) (federation.RemoteFollowTombstone, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.RemoteFollowTombstone{}, err
	}
	row, err := q.GetLiveRemoteFollowTombstone(ctx, postgresgen.GetLiveRemoteFollowTombstoneParams{
		ActorUri: string(actorURI), ObjectUri: string(objectURI),
	})
	if err != nil {
		return federation.RemoteFollowTombstone{}, notFound(err, appfederation.ErrNotFound, "follow tombstone of "+string(actorURI))
	}
	return federation.RemoteFollowTombstone{
		ActivityURI: federation.IRI(row.ActivityUri), ActorURI: federation.IRI(row.ActorUri),
		ObjectURI: federation.IRI(row.ObjectUri), ReceivedAt: utc(row.ReceivedAt),
	}, nil
}
