// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/gatheringgen"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The moderation of the events and the retention of their records
// (appgathering.ModerationRepository and appgathering.RecordsRepository).

// Revisions implements appgathering.Repository.
func (r *Repository) Revisions(ctx context.Context, tx uow.Tx, gatheringID string) ([]appgathering.Revision, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringRevisions(ctx, gatheringID)
	if err != nil {
		return nil, wrap(err, "list gathering revisions")
	}
	if len(rows) == 0 {
		return []appgathering.Revision{}, nil
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	list, err := listOf(ids)
	if err != nil {
		return nil, err
	}
	places, err := q.ListGatheringRevisionPlaces(ctx, list)
	if err != nil {
		return nil, wrap(err, "list gathering places")
	}
	byRevision := map[string][]gathering.Place{}
	for _, p := range places {
		byRevision[p.RevisionID] = append(byRevision[p.RevisionID], gathering.Place{
			Kind: gathering.PlaceKind(p.Kind), Name: p.Name, Address: p.Address, URL: p.Url, Speakers: gathering.Speakers(p.Speakers),
		})
	}
	out := make([]appgathering.Revision, len(rows))
	for i, row := range rows {
		out[i] = appgathering.Revision{
			ID: row.ID, GatheringID: row.GatheringID, RevisedByAccountID: row.RevisedByAccountID, RevisedAt: fromTime(row.RevisedAt),
			Settings: gathering.Settings{
				Title: row.Title, Description: row.Description, Visibility: gathering.Visibility(row.Visibility), TimeZone: row.TimeZone,
				PlaceDisclosure: gathering.PlaceDisclosure(row.PlaceDisclosure), Capacity: fromInt(row.Capacity),
				Places: byRevision[row.ID],
			},
		}
	}
	return out, nil
}

// OrganizedUpcoming implements appgathering.Repository.
func (r *Repository) OrganizedUpcoming(ctx context.Context, tx uow.Tx, accountID string, now time.Time, limit int) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListOrganizedUpcomingGatherings(ctx, gatheringgen.ListOrganizedUpcomingGatheringsParams{
		AccountID: accountID, Now: toTime(now), RowLimit: toInt(limit),
	})
	return ids, wrap(err, "list the upcoming gatherings of an organizer")
}

// GatheringsToPurge implements appgathering.Repository.
func (r *Repository) GatheringsToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, after string, limit int) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListGatheringsToPurge(ctx, gatheringgen.ListGatheringsToPurgeParams{
		After: after, EndedBefore: toTime(endedBefore), RowLimit: toInt(limit),
	})
	return ids, wrap(err, "list gatherings to purge")
}

// RecordAccounts implements appgathering.Repository.
func (r *Repository) RecordAccounts(ctx context.Context, tx uow.Tx, gatheringID string) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListGatheringRecordAccounts(ctx, gatheringID)
	return ids, wrap(err, "list the accounts of the gathering records")
}

// PurgeRecords implements appgathering.Repository.
func (r *Repository) PurgeRecords(ctx context.Context, tx uow.Tx, gatheringIDs []string) error {
	if len(gatheringIDs) == 0 {
		return nil
	}
	q, err := r.q(tx)
	if err != nil {
		return err
	}
	list, err := listOf(gatheringIDs)
	if err != nil {
		return err
	}
	steps := []struct {
		what string
		run  func(context.Context, []string) error
	}{
		{"approvals", q.PurgeGatheringRSVPApprovals},
		{"answers", q.PurgeGatheringRSVPs},
		{"attendees", q.PurgeGatheringAttendees},
		{"votes", q.PurgeGatheringVotes},
		{"invitation revocations", q.PurgeGatheringInvitationRevocations},
		{"invitations", q.PurgeGatheringInvitations},
	}
	for _, step := range steps {
		if err := step.run(ctx, list); err != nil {
			return wrap(err, "purge gathering "+step.what)
		}
	}
	return nil
}
