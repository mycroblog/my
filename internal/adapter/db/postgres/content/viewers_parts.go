// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// The reads of many viewers' interactions with the statuses of a page, each as the read of one
// viewer does for every viewer it names.

// FindInteractionsOfAccounts implements content.InteractionRepository.
func (r *InteractionRepository) FindInteractionsOfAccounts(ctx context.Context, accountIDs, statusIDs []string) (map[string]appcontent.AccountInteractions, error) {
	out := make(map[string]appcontent.AccountInteractions, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = appcontent.AccountInteractions{
			Favourites: map[string]content.Favourite{}, BoostEdges: map[string]content.BoostEdge{}, Bookmarks: map[string]content.Bookmark{},
			Pins: map[string]content.Pin{}, Reactions: map[string][]content.Reaction{},
		}
	}
	if len(accountIDs) == 0 || len(statusIDs) == 0 {
		return out, nil
	}
	accounts, statuses := accountIDs, statusIDs
	q := r.db.reader()
	favourites, err := q.ListFavouritesOfAccountsOn(ctx, contentgen.ListFavouritesOfAccountsOnParams{AccountIds: accounts, StatusIds: statuses})
	if err != nil {
		return nil, postgres.Wrap(err, "list favourites of accounts")
	}
	for _, row := range favourites {
		out[row.AccountID].Favourites[row.StatusID] = favouriteFrom(row)
	}
	edges, err := q.ListBoostEdgesOfAccountsOn(ctx, contentgen.ListBoostEdgesOfAccountsOnParams{AccountIds: accounts, StatusIds: statuses})
	if err != nil {
		return nil, postgres.Wrap(err, "list boost edges of accounts")
	}
	for _, row := range edges {
		out[row.BoosterAccountID].BoostEdges[row.StatusID] = content.BoostEdge(row)
	}
	bookmarks, err := q.ListBookmarksOfAccountsOn(ctx, contentgen.ListBookmarksOfAccountsOnParams{AccountIds: accounts, StatusIds: statuses})
	if err != nil {
		return nil, postgres.Wrap(err, "list bookmarks of accounts")
	}
	for _, row := range bookmarks {
		out[row.AccountID].Bookmarks[row.StatusID] = bookmarkFrom(row)
	}
	pins, err := q.ListStatusPinsOfAccountsOn(ctx, contentgen.ListStatusPinsOfAccountsOnParams{AccountIds: accounts, StatusIds: statuses})
	if err != nil {
		return nil, postgres.Wrap(err, "list pins of accounts")
	}
	for _, row := range pins {
		out[row.AccountID].Pins[row.StatusID] = pinFrom(row)
	}
	reactions, err := q.ListReactionsOfAccountsOn(ctx, contentgen.ListReactionsOfAccountsOnParams{AccountIds: accounts, StatusIds: statuses})
	if err != nil {
		return nil, postgres.Wrap(err, "list reactions of accounts")
	}
	for _, row := range reactions {
		out[row.AccountID].Reactions[row.StatusID] = append(out[row.AccountID].Reactions[row.StatusID], reactionFrom(row))
	}
	return out, nil
}

// MentionsOfAccountsIn implements content.InteractionRepository.
func (r *InteractionRepository) MentionsOfAccountsIn(ctx context.Context, statusIDs, accountIDs []string) (map[string]map[string]bool, error) {
	out := make(map[string]map[string]bool, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = map[string]bool{}
	}
	if len(accountIDs) == 0 || len(statusIDs) == 0 {
		return out, nil
	}
	accounts, statuses := accountIDs, statusIDs
	rows, err := r.db.reader().ListMentionsOfAccountsIn(ctx, contentgen.ListMentionsOfAccountsInParams{AccountIds: accounts, StatusIds: statuses})
	if err != nil {
		return nil, postgres.Wrap(err, "list the mentions of accounts")
	}
	for _, row := range rows {
		out[row.AccountID][row.StatusID] = true
	}
	return out, nil
}

// ListPollVotesOfAccountsIn implements content.Repository.
func (r *StatusRepository) ListPollVotesOfAccountsIn(ctx context.Context, pollIDs, accountIDs []string) (map[string]map[string][]content.PollVote, error) {
	out := make(map[string]map[string][]content.PollVote, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = map[string][]content.PollVote{}
	}
	if len(pollIDs) == 0 || len(accountIDs) == 0 {
		return out, nil
	}
	accounts, polls := accountIDs, pollIDs
	rows, err := r.db.reader().ListPollVotesOfAccountsIn(ctx, contentgen.ListPollVotesOfAccountsInParams{AccountIds: accounts, PollIds: polls})
	if err != nil {
		return nil, postgres.Wrap(err, "list poll votes of accounts")
	}
	for _, row := range rows {
		out[row.AccountID][row.PollID] = append(out[row.AccountID][row.PollID], content.PollVote{
			ID: row.ID, PollID: row.PollID, AccountID: row.AccountID, Position: int(row.Position), URI: postgres.FromNullString(row.Uri), VotedAt: postgres.UTC(row.VotedAt),
		})
	}
	return out, nil
}
