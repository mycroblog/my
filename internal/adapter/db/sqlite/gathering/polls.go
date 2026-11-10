// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/gatheringgen"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The date polls of the events (ADR-0161 §7): appgathering.PollRepository.

// InsertCandidate implements appgathering.PollRepository.
func (r *Repository) InsertCandidate(ctx context.Context, tx uow.Tx, c appgathering.Candidate) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertGatheringCandidate(ctx, gatheringgen.InsertGatheringCandidateParams{
		ID: c.ID, GatheringID: c.GatheringID, StartsAt: toTime(c.Schedule.StartsAt), EndsAt: toTime(c.Schedule.EndsAt),
		AddedByAccountID: c.AddedByAccountID, AddedAt: toTime(c.AddedAt),
	}), "insert gathering candidate")
}

// InsertCandidateWithdrawal implements appgathering.PollRepository.
func (r *Repository) InsertCandidateWithdrawal(ctx context.Context, tx uow.Tx, w appgathering.CandidateWithdrawal) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertGatheringCandidateWithdrawal(ctx, gatheringgen.InsertGatheringCandidateWithdrawalParams{
		CandidateID: w.CandidateID, WithdrawnByAccountID: w.WithdrawnByAccountID, WithdrawnAt: toTime(w.WithdrawnAt),
	})
	return n > 0, wrap(err, "insert gathering candidate withdrawal")
}

// FindCandidate implements appgathering.PollRepository.
func (r *Repository) FindCandidate(ctx context.Context, tx uow.Tx, gatheringID, id string) (appgathering.CandidateRecord, error) {
	q, err := r.q(tx)
	if err != nil {
		return appgathering.CandidateRecord{}, err
	}
	row, err := q.GetGatheringCandidate(ctx, gatheringgen.GetGatheringCandidateParams{GatheringID: gatheringID, ID: id})
	if err != nil {
		return appgathering.CandidateRecord{}, notFound(err, appgathering.ErrNotFound, "gathering candidate")
	}
	return appgathering.CandidateRecord{
		Candidate: appgathering.Candidate{
			ID: row.ID, GatheringID: row.GatheringID, AddedByAccountID: row.AddedByAccountID, AddedAt: fromTime(row.AddedAt),
			Schedule: gathering.Schedule{StartsAt: fromTime(row.StartsAt), EndsAt: fromTime(row.EndsAt)},
		},
		Withdrawn: row.Withdrawn, Confirmed: row.Confirmed,
	}, nil
}

// Candidates implements appgathering.PollRepository.
func (r *Repository) Candidates(ctx context.Context, tx uow.Tx, gatheringID, accountID string) ([]appgathering.CandidateTally, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringCandidates(ctx, gatheringgen.ListGatheringCandidatesParams{GatheringID: gatheringID, AccountID: accountID})
	if err != nil {
		return nil, wrap(err, "list gathering candidates")
	}
	out := make([]appgathering.CandidateTally, len(rows))
	for i, row := range rows {
		out[i] = appgathering.CandidateTally{
			Candidate: appgathering.Candidate{
				ID: row.ID, GatheringID: row.GatheringID, AddedByAccountID: row.AddedByAccountID, AddedAt: fromTime(row.AddedAt),
				Schedule: gathering.Schedule{StartsAt: fromTime(row.StartsAt), EndsAt: fromTime(row.EndsAt)},
			},
			Yes: fromInt(row.YesCount), Maybe: fromInt(row.MaybeCount), No: fromInt(row.NoCount),
			ViewerVote: gathering.Vote(row.ViewerVote), Confirmed: row.Confirmed,
		}
	}
	return out, nil
}

// SetVote implements appgathering.PollRepository.
func (r *Repository) SetVote(ctx context.Context, tx uow.Tx, b appgathering.Ballot) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertGatheringVote(ctx, gatheringgen.UpsertGatheringVoteParams{
		CandidateID: b.CandidateID, AccountID: b.AccountID, GatheringID: b.GatheringID, Vote: string(b.Vote), VotedAt: toTime(b.VotedAt),
	}), "upsert gathering vote")
}

// Ballots implements appgathering.PollRepository.
func (r *Repository) Ballots(ctx context.Context, tx uow.Tx, candidateID string, vote gathering.Vote, afterAccountID string, limit int) ([]appgathering.Ballot, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringVotes(ctx, gatheringgen.ListGatheringVotesParams{
		CandidateID: candidateID, VoteFilter: string(vote), AfterID: afterAccountID, RowLimit: toInt(limit),
	})
	if err != nil {
		return nil, wrap(err, "list gathering votes")
	}
	out := make([]appgathering.Ballot, len(rows))
	for i, row := range rows {
		out[i] = appgathering.Ballot{CandidateID: candidateID, AccountID: row.AccountID, Vote: gathering.Vote(row.Vote)}
	}
	return out, nil
}

// InsertConfirmationCandidate implements appgathering.PollRepository.
func (r *Repository) InsertConfirmationCandidate(ctx context.Context, tx uow.Tx, confirmationID, candidateID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertGatheringConfirmationCandidate(ctx, gatheringgen.InsertGatheringConfirmationCandidateParams{
		ConfirmationID: confirmationID, CandidateID: candidateID,
	}), "insert gathering confirmation candidate")
}

// PollAudience implements appgathering.PollRepository.
func (r *Repository) PollAudience(ctx context.Context, tx uow.Tx, gatheringID, after string, limit int) ([]string, error) {
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListGatheringPollAudience(ctx, gatheringgen.ListGatheringPollAudienceParams{
		GatheringID: gatheringID, AfterID: after, RowLimit: toInt(limit),
	})
	return ids, wrap(err, "list gathering poll audience")
}
