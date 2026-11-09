// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	"github.com/mycroblog/my/internal/domain/content"
)

// SensitivitiesOverridden implements content.Repository.
func (r *StatusRepository) SensitivitiesOverridden(ctx context.Context, statusIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListStatusesSensitivityOverridden(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status sensitivity overrides")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// FindPollTalliesOf implements content.Repository.
func (r *StatusRepository) FindPollTalliesOf(ctx context.Context, pollIDs []string) (map[string][]content.PollTally, error) {
	out := map[string][]content.PollTally{}
	if len(pollIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(pollIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListPollTalliesOf(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list poll tallies")
	}
	for _, row := range rows {
		out[row.PollID] = append(out[row.PollID], content.PollTally{PollID: row.PollID, Position: int(row.Position), Votes: row.Votes, VotersCount: sqlite.FromNullInt64(row.VotersCount)})
	}
	return out, nil
}

// ListPollVotesIn implements content.Repository.
func (r *StatusRepository) ListPollVotesIn(ctx context.Context, pollIDs []string, accountID string) (map[string][]content.PollVote, error) {
	out := map[string][]content.PollVote{}
	if len(pollIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(pollIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListPollVotesOfAccountIn(ctx, contentgen.ListPollVotesOfAccountInParams{AccountID: accountID, PollIds: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list poll votes")
	}
	for _, row := range rows {
		out[row.PollID] = append(out[row.PollID], content.PollVote{
			ID: row.ID, PollID: row.PollID, AccountID: row.AccountID, Position: int(row.Position), URI: row.Uri.String, VotedAt: sqlite.FromMillis(row.VotedAt),
		})
	}
	return out, nil
}
