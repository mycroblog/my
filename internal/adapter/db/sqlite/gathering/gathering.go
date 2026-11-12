// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/gatheringgen"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// Repository implements appgathering.Repository (ADR-0161 §13).
type Repository struct {
	db *moduleDB
}

var _ appgathering.Repository = (*Repository)(nil)

// NewRepository returns the gathering module's repository on this database.
func NewRepository(d *sqlite.DB) *Repository { return &Repository{db: &moduleDB{d}} }

// q returns the queries in tx, or on the reader when tx is nil.
func (r *Repository) q(tx uow.Tx) (*gatheringgen.Queries, error) {
	if tx == nil {
		return r.db.reader(), nil
	}
	return r.db.queries(tx)
}

// InsertGathering implements appgathering.Repository.
func (r *Repository) InsertGathering(ctx context.Context, tx uow.Tx, g appgathering.Gathering) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertGathering(ctx, gatheringgen.InsertGatheringParams{
		ID: g.ID, OrganizerAccountID: g.OrganizerAccountID, CreatedAt: toTime(g.CreatedAt),
	}), "insert gathering")
}

// InsertRevision implements appgathering.Repository.
func (r *Repository) InsertRevision(ctx context.Context, tx uow.Tx, rev appgathering.Revision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	s := rev.Settings
	if err := q.InsertGatheringRevision(ctx, gatheringgen.InsertGatheringRevisionParams{
		ID: rev.ID, GatheringID: rev.GatheringID, Title: s.Title, Description: s.Description, Visibility: string(s.Visibility),
		TimeZone: s.TimeZone, PlaceDisclosure: string(s.PlaceDisclosure), Capacity: toInt(s.Capacity),
		RevisedByAccountID: rev.RevisedByAccountID, RevisedAt: toTime(rev.RevisedAt),
	}); err != nil {
		return wrap(err, "insert gathering revision")
	}
	for i, p := range s.Places {
		if err := q.InsertGatheringRevisionPlace(ctx, gatheringgen.InsertGatheringRevisionPlaceParams{
			RevisionID: rev.ID, Position: toInt(i), Kind: string(p.Kind), Name: p.Name, Address: p.Address, Url: p.URL,
			Speakers: string(p.Speakers),
		}); err != nil {
			return wrap(err, "insert gathering place")
		}
	}
	return nil
}

// InsertConfirmation implements appgathering.Repository.
func (r *Repository) InsertConfirmation(ctx context.Context, tx uow.Tx, c appgathering.Confirmation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertGatheringScheduleConfirmation(ctx, gatheringgen.InsertGatheringScheduleConfirmationParams{
		ID: c.ID, GatheringID: c.GatheringID, StartsAt: toTime(c.Schedule.StartsAt), EndsAt: toTime(c.Schedule.EndsAt),
		ConfirmedByAccountID: c.ConfirmedByAccountID, ConfirmedAt: toTime(c.ConfirmedAt),
	}), "insert gathering confirmation")
}

// InsertInvitation implements appgathering.Repository.
func (r *Repository) InsertInvitation(ctx context.Context, tx uow.Tx, i appgathering.Invitation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertGatheringInvitation(ctx, gatheringgen.InsertGatheringInvitationParams{
		ID: i.ID, GatheringID: i.GatheringID, InviteeAccountID: i.InviteeAccountID, InvitedByAccountID: i.InvitedByAccountID,
		InvitedAt: toTime(i.InvitedAt),
	})
	return n > 0, wrap(err, "insert gathering invitation")
}

// InsertInvitationRevocation implements appgathering.Repository.
func (r *Repository) InsertInvitationRevocation(ctx context.Context, tx uow.Tx, rev appgathering.InvitationRevocation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertGatheringInvitationRevocation(ctx, gatheringgen.InsertGatheringInvitationRevocationParams{
		InvitationID: rev.InvitationID, RevokedByAccountID: rev.RevokedByAccountID, RevokedAt: toTime(rev.RevokedAt),
	})
	return n > 0, wrap(err, "insert gathering invitation revocation")
}

// InsertRSVP implements appgathering.Repository.
func (r *Repository) InsertRSVP(ctx context.Context, tx uow.Tx, a appgathering.RSVP) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertGatheringRSVP(ctx, gatheringgen.InsertGatheringRSVPParams{
		ID: a.ID, GatheringID: a.GatheringID, AccountID: a.AccountID, Answer: string(a.Answer), AnsweredAt: toTime(a.AnsweredAt),
	}), "insert gathering rsvp")
}

// InsertApproval implements appgathering.Repository.
func (r *Repository) InsertApproval(ctx context.Context, tx uow.Tx, a appgathering.Approval) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertGatheringRSVPApproval(ctx, gatheringgen.InsertGatheringRSVPApprovalParams{
		ID: a.ID, GatheringID: a.GatheringID, AccountID: a.AccountID, Change: string(a.Change),
		ChangedByAccountID: a.ChangedByAccountID, ChangedAt: toTime(a.ChangedAt),
	}), "insert gathering approval")
}

// InsertCoorganizerChange implements appgathering.Repository.
func (r *Repository) InsertCoorganizerChange(ctx context.Context, tx uow.Tx, c appgathering.CoorganizerChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertGatheringCoorganizerChange(ctx, gatheringgen.InsertGatheringCoorganizerChangeParams{
		ID: c.ID, GatheringID: c.GatheringID, AccountID: c.AccountID, Change: string(c.Change), ChangedAt: toTime(c.ChangedAt),
	}), "insert gathering co-organizer change")
}

// InsertCancellation implements appgathering.Repository.
func (r *Repository) InsertCancellation(ctx context.Context, tx uow.Tx, c appgathering.Cancellation) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertGatheringCancellation(ctx, gatheringgen.InsertGatheringCancellationParams{
		GatheringID: c.GatheringID, Reason: string(c.Reason), CancelledByAccountID: toRef(c.CancelledByAccountID), Note: c.Note,
		CancelledAt: toTime(c.CancelledAt),
	})
	return n > 0, wrap(err, "insert gathering cancellation")
}

// recordOf returns the record of a row of GetGathering, without its places and co-organizers.
func recordOf(row gatheringgen.GetGatheringRow) appgathering.Record {
	rec := appgathering.Record{
		Gathering: appgathering.Gathering{ID: row.ID, OrganizerAccountID: row.OrganizerAccountID, CreatedAt: fromTime(row.CreatedAt)},
		Head: appgathering.Head{
			GatheringID: row.ID, OrganizerAccountID: row.OrganizerAccountID, RevisionID: row.RevisionID, ConfirmationID: row.ConfirmationID,
			State: gathering.State(row.State), GoingCount: fromInt(row.GoingCount),
			Schedule: gathering.Schedule{StartsAt: fromTime(row.StartsAt), EndsAt: fromTime(row.EndsAt)},
		},
		Revision: appgathering.Revision{
			ID: row.RevisionID, GatheringID: row.ID, RevisedByAccountID: row.RevisedByAccountID, RevisedAt: fromTime(row.RevisedAt),
			Settings: gathering.Settings{
				Title: row.Title, Description: row.Description, Visibility: gathering.Visibility(row.Visibility), TimeZone: row.TimeZone,
				PlaceDisclosure: gathering.PlaceDisclosure(row.PlaceDisclosure), Capacity: fromInt(row.Capacity),
			},
		},
		Confirmation: appgathering.Confirmation{
			ID: row.ConfirmationID, GatheringID: row.ID, ConfirmedByAccountID: row.ConfirmedByAccountID, ConfirmedAt: fromTime(row.ConfirmedAt),
			Schedule: gathering.Schedule{StartsAt: fromTime(row.StartsAt), EndsAt: fromTime(row.EndsAt)},
		},
		Cancellation: cancellationOf(row),
		Revisions:    int(row.RevisionCount),
	}
	if row.ConfirmationID == "" {
		// An unscheduled event polls its date: the head's times span its candidates.
		rec.Confirmation = appgathering.Confirmation{}
	}
	return rec
}

// withParts reads the places of the current revisions and the co-organizers of recs.
func withParts(ctx context.Context, q *gatheringgen.Queries, recs []appgathering.Record) error {
	if len(recs) == 0 {
		return nil
	}
	revisions := make([]string, len(recs))
	gatherings := make([]string, len(recs))
	for i, r := range recs {
		revisions[i], gatherings[i] = r.Revision.ID, r.ID
	}
	revList, err := listOf(revisions)
	if err != nil {
		return err
	}
	places, err := q.ListGatheringRevisionPlaces(ctx, revList)
	if err != nil {
		return wrap(err, "list gathering places")
	}
	byRevision := map[string][]gathering.Place{}
	for _, p := range places {
		byRevision[p.RevisionID] = append(byRevision[p.RevisionID], gathering.Place{
			Kind: gathering.PlaceKind(p.Kind), Name: p.Name, Address: p.Address, URL: p.Url, Speakers: gathering.Speakers(p.Speakers),
		})
	}
	gList, err := listOf(gatherings)
	if err != nil {
		return err
	}
	cos, err := q.ListGatheringCoorganizers(ctx, gList)
	if err != nil {
		return wrap(err, "list gathering co-organizers")
	}
	byGathering := map[string][]appgathering.Coorganizer{}
	for _, c := range cos {
		byGathering[c.GatheringID] = append(byGathering[c.GatheringID], appgathering.Coorganizer{AccountID: c.AccountID, ChangeID: c.ChangeID})
	}
	for i := range recs {
		recs[i].Revision.Settings.Places = byRevision[recs[i].Revision.ID]
		recs[i].Coorganizers = byGathering[recs[i].ID]
	}
	return nil
}

// FindGathering implements appgathering.Repository.
func (r *Repository) FindGathering(ctx context.Context, tx uow.Tx, id string) (appgathering.Record, error) {
	q, err := r.q(tx)
	if err != nil {
		return appgathering.Record{}, err
	}
	row, err := q.GetGathering(ctx, id)
	if err != nil {
		return appgathering.Record{}, notFound(err, appgathering.ErrNotFound, "gathering")
	}
	recs := []appgathering.Record{recordOf(row)}
	if err := withParts(ctx, q, recs); err != nil {
		return appgathering.Record{}, err
	}
	return recs[0], nil
}

// UpcomingGatherings implements appgathering.Repository.
func (r *Repository) UpcomingGatherings(ctx context.Context, tx uow.Tx, now time.Time, after *appgathering.ListKey, limit int) ([]appgathering.Record, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	arg := gatheringgen.ListUpcomingGatheringsParams{Now: toTime(now), RowLimit: toInt(limit)}
	if after != nil {
		arg.AfterID, arg.AfterStartsAt = after.GatheringID, toTime(after.StartsAt)
	}
	rows, err := q.ListUpcomingGatherings(ctx, arg)
	if err != nil {
		return nil, wrap(err, "list upcoming gatherings")
	}
	recs := make([]appgathering.Record, len(rows))
	for i, row := range rows {
		recs[i] = recordOf(gatheringgen.GetGatheringRow(row))
	}
	if err := withParts(ctx, q, recs); err != nil {
		return nil, err
	}
	return recs, nil
}

func attendeeOf(a gatheringgen.GatheringAttendee) appgathering.Attendee {
	return appgathering.Attendee{
		GatheringID: a.GatheringID, AccountID: a.AccountID, RSVPID: a.RsvpID, Answer: gathering.Answer(a.Answer), Approved: fromBool(a.Approved),
	}
}

// FindAttendee implements appgathering.Repository.
func (r *Repository) FindAttendee(ctx context.Context, tx uow.Tx, gatheringID, accountID string) (appgathering.Attendee, error) {
	q, err := r.q(tx)
	if err != nil {
		return appgathering.Attendee{}, err
	}
	a, err := q.GetGatheringAttendee(ctx, gatheringgen.GetGatheringAttendeeParams{GatheringID: gatheringID, AccountID: accountID})
	if err != nil {
		return appgathering.Attendee{}, notFound(err, appgathering.ErrNotFound, "gathering attendee")
	}
	return attendeeOf(a), nil
}

// Attendees implements appgathering.Repository.
func (r *Repository) Attendees(ctx context.Context, tx uow.Tx, gatheringID, afterRSVPID string, limit int) ([]appgathering.Attendee, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringAttendees(ctx, gatheringgen.ListGatheringAttendeesParams{GatheringID: gatheringID, AfterID: afterRSVPID, RowLimit: toInt(limit)})
	if err != nil {
		return nil, wrap(err, "list gathering attendees")
	}
	out := make([]appgathering.Attendee, len(rows))
	for i, row := range rows {
		out[i] = attendeeOf(row)
	}
	return out, nil
}

// FindInvitation implements appgathering.Repository.
func (r *Repository) FindInvitation(ctx context.Context, tx uow.Tx, gatheringID, accountID string) (appgathering.InvitationRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return appgathering.InvitationRecord{}, err
	}
	row, err := q.GetGatheringInvitation(ctx, gatheringgen.GetGatheringInvitationParams{GatheringID: gatheringID, InviteeAccountID: accountID})
	if err != nil {
		return appgathering.InvitationRecord{}, notFound(err, appgathering.ErrNotFound, "gathering invitation")
	}
	return appgathering.InvitationRecord{
		Invitation: appgathering.Invitation{
			ID: row.ID, GatheringID: row.GatheringID, InviteeAccountID: row.InviteeAccountID, InvitedByAccountID: row.InvitedByAccountID,
			InvitedAt: fromTime(row.InvitedAt),
		},
		Revoked: fromBool(row.Revoked),
	}, nil
}

// Invitations implements appgathering.Repository.
func (r *Repository) Invitations(ctx context.Context, tx uow.Tx, gatheringID, afterID string, limit int) ([]appgathering.Invitation, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringInvitations(ctx, gatheringgen.ListGatheringInvitationsParams{GatheringID: gatheringID, AfterID: afterID, RowLimit: toInt(limit)})
	if err != nil {
		return nil, wrap(err, "list gathering invitations")
	}
	out := make([]appgathering.Invitation, len(rows))
	for i, row := range rows {
		out[i] = appgathering.Invitation{
			ID: row.ID, GatheringID: row.GatheringID, InviteeAccountID: row.InviteeAccountID, InvitedByAccountID: row.InvitedByAccountID,
			InvitedAt: fromTime(row.InvitedAt),
		}
	}
	return out, nil
}

// ViewerFacts implements appgathering.Repository.
func (r *Repository) ViewerFacts(ctx context.Context, tx uow.Tx, accountID string, gatheringIDs []string) (map[string]appgathering.ViewerFacts, error) {
	out := make(map[string]appgathering.ViewerFacts, len(gatheringIDs))
	if len(gatheringIDs) == 0 {
		return out, nil
	}
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	list, err := listOf(gatheringIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringViewerFacts(ctx, gatheringgen.ListGatheringViewerFactsParams{AccountID: accountID, GatheringIds: list})
	if err != nil {
		return nil, wrap(err, "list gathering viewer facts")
	}
	for _, row := range rows {
		out[row.GatheringID] = appgathering.ViewerFacts{Invited: row.Invited, Answer: gathering.Answer(row.Answer), Approved: fromBool(row.Approved), Voted: row.Voted}
	}
	return out, nil
}

// EraseRevisions implements appgathering.Repository.
func (r *Repository) EraseRevisions(ctx context.Context, tx uow.Tx, accountID string, limit int) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	ids, err := q.ListGatheringRevisionsToErase(ctx, gatheringgen.ListGatheringRevisionsToEraseParams{AccountID: accountID, RowLimit: toInt(limit)})
	if err != nil {
		return 0, wrap(err, "list gathering revisions to erase")
	}
	for _, id := range ids {
		if err := q.EraseGatheringRevision(ctx, id); err != nil {
			return 0, wrap(err, "erase gathering revision")
		}
		if err := q.DeleteGatheringRevisionPlaces(ctx, id); err != nil {
			return 0, wrap(err, "delete gathering places")
		}
	}
	return len(ids), nil
}

// RefreshHead implements appgathering.ProjectionRepository.
func (r *Repository) RefreshHead(ctx context.Context, tx uow.Tx, gatheringID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteGatheringHead(ctx, gatheringID); err != nil {
		return wrap(err, "delete gathering head")
	}
	return wrap(q.InsertGatheringHead(ctx, gatheringID), "derive gathering head")
}

// RefreshAttendee implements appgathering.ProjectionRepository.
func (r *Repository) RefreshAttendee(ctx context.Context, tx uow.Tx, gatheringID, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteGatheringAttendee(ctx, gatheringgen.DeleteGatheringAttendeeParams{GatheringID: gatheringID, AccountID: accountID}); err != nil {
		return wrap(err, "delete gathering attendee")
	}
	return wrap(q.InsertGatheringAttendee(ctx, gatheringgen.InsertGatheringAttendeeParams{GatheringID: gatheringID, AccountID: accountID}),
		"derive gathering attendee")
}

// RefreshCoorganizers implements appgathering.ProjectionRepository.
func (r *Repository) RefreshCoorganizers(ctx context.Context, tx uow.Tx, gatheringID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteGatheringCoorganizers(ctx, gatheringID); err != nil {
		return wrap(err, "delete gathering co-organizers")
	}
	return wrap(q.InsertGatheringCoorganizers(ctx, gatheringID), "derive gathering co-organizers")
}

// RebuildProjections implements appgathering.ProjectionRepository: the attendees first, which
// the heads count.
func (r *Repository) RebuildProjections(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild gathering projections",
		(*gatheringgen.Queries).DeleteAllGatheringAttendees, (*gatheringgen.Queries).RebuildGatheringAttendees,
		(*gatheringgen.Queries).DeleteAllGatheringHeads, (*gatheringgen.Queries).RebuildGatheringHeads,
		(*gatheringgen.Queries).DeleteAllGatheringCoorganizers, (*gatheringgen.Queries).RebuildGatheringCoorganizers)
}
