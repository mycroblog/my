// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.RetentionRepository = (*StatusRepository)(nil)

// ListRemoteStatusesToPurge implements content.RetentionRepository.
func (r *StatusRepository) ListRemoteStatusesToPurge(ctx context.Context, tx uow.Tx, pq appcontent.PurgeQuery) ([]content.Status, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	n, err := postgres.ToInt32(pq.Limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRemoteStatusesToPurge(ctx, contentgen.ListRemoteStatusesToPurgeParams{
		After:              postgres.ToNullString(pq.After),
		IDBefore:           purgeIDBound(pq),
		AnyBefore:          pq.Before,
		UnreferencedBefore: pq.UnreferencedBefore,
		BatchLimit:         n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list remote statuses to purge")
	}
	out := make([]content.Status, 0, len(rows))
	for _, row := range rows {
		out = append(out, statusFrom(row))
	}
	return out, nil
}

// ListRelayedStatusesToPurge implements content.RetentionRepository.
func (r *StatusRepository) ListRelayedStatusesToPurge(ctx context.Context, tx uow.Tx, pq appcontent.RelayedPurgeQuery) ([]content.Status, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	n, err := postgres.ToInt32(pq.Limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRelayedStatusesToPurge(ctx, contentgen.ListRelayedStatusesToPurgeParams{
		After: postgres.ToNullString(pq.After), IDBefore: kernel.UUIDv7Floor(pq.Before.Add(time.Millisecond)), RelayedBefore: pq.Before, BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list relayed statuses to purge")
	}
	out := make([]content.Status, 0, len(rows))
	for _, row := range rows {
		out = append(out, statusFrom(row))
	}
	return out, nil
}

// InsertRelayedStatus implements content.Repository.
func (r *StatusRepository) InsertRelayedStatus(ctx context.Context, tx uow.Tx, statusID, relayID string, receivedAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRelayedStatus(ctx, contentgen.InsertRelayedStatusParams{StatusID: statusID, RelayID: relayID, ReceivedAt: receivedAt})
	return postgres.Wrap(err, "insert relayed status")
}

// purgeIDBound is the UUIDv7 floor just past the later of the query's times: the ID of a remote
// status is stamped with its publication time, so no status published before them has a greater
// ID, and the scan of the primary key stops there. A zero Before counts as no time.
func purgeIDBound(pq appcontent.PurgeQuery) string {
	t := pq.UnreferencedBefore
	if pq.Before.After(t) {
		t = pq.Before
	}
	return kernel.UUIDv7Floor(t.Add(time.Millisecond))
}

// PurgeStatuses implements content.RetentionRepository.
func (r *StatusRepository) PurgeStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids := statusIDs
	// Children before their parents: the revisions' rows, the polls', then the boosts, the
	// quotes, and the statuses.
	steps := []func(context.Context, []string) error{
		q.PurgeStatusSensitivityOverrides, q.PurgeStatusRevisionBodies, q.PurgeStatusRevisionSources, q.PurgeStatusRevisionAttachments,
		q.PurgeStatusRevisionMentions, q.PurgeStatusRevisionTags, q.PurgeStatusRevisionEmojis,
		q.PurgeRemoteStatusRevisions, q.PurgePollTallyObservationCounts, q.PurgePollTallyObservations,
		q.PurgePollVotes, q.PurgePollTallies, q.PurgePollOptions, q.PurgePolls, q.PurgeStatusRevisions,
		q.PurgeRemoteBoostRetractions, q.PurgeRemoteBoosts, q.PurgeRelayedStatuses, q.PurgeStatusQuoteStates,
		q.PurgeStatusQuoteEvents, q.PurgeStatusQuotes, q.PurgeStatusInteractionPolicies, q.PurgeHeldReplies,
		q.PurgeQuoteAuthorizationRevocations, q.PurgeQuoteAuthorizations, q.PurgeStatusConversations, q.PurgeStatuses,
	}
	for _, step := range steps {
		if err := step(ctx, ids); err != nil {
			return postgres.Wrap(err, "purge statuses")
		}
	}
	return nil
}

// DeleteRetentionDeletionsBefore implements content.RetentionRepository.
func (r *StatusRepository) DeleteRetentionDeletionsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteRetentionDeletionsBefore(ctx, contentgen.DeleteRetentionDeletionsBeforeParams{DeletedBefore: t, BatchLimit: batch})
	return n, postgres.Wrap(err, "delete retention deletions")
}
