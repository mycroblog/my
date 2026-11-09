// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// FindAccountInteractions implements content.InteractionRepository: one query per kind, mapped as
// the reads of one status map their rows.
func (r *InteractionRepository) FindAccountInteractions(ctx context.Context, accountID string, statusIDs []string) (appcontent.AccountInteractions, error) {
	out := appcontent.AccountInteractions{
		Favourites: map[string]content.Favourite{}, BoostEdges: map[string]content.BoostEdge{}, Bookmarks: map[string]content.Bookmark{},
		Pins: map[string]content.Pin{}, Reactions: map[string][]content.Reaction{},
	}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return out, err
	}
	q := r.db.reader()
	favourites, err := q.ListFavouritesOn(ctx, contentgen.ListFavouritesOnParams{AccountID: accountID, StatusIds: list})
	if err != nil {
		return out, sqlite.Wrap(err, "list favourites of "+accountID)
	}
	for _, row := range favourites {
		out.Favourites[row.StatusID] = favouriteFrom(row)
	}
	edges, err := q.ListBoostEdgesOn(ctx, contentgen.ListBoostEdgesOnParams{AccountID: accountID, StatusIds: list})
	if err != nil {
		return out, sqlite.Wrap(err, "list boost edges of "+accountID)
	}
	for _, row := range edges {
		out.BoostEdges[row.StatusID] = content.BoostEdge(row)
	}
	bookmarks, err := q.ListBookmarksOn(ctx, contentgen.ListBookmarksOnParams{AccountID: accountID, StatusIds: list})
	if err != nil {
		return out, sqlite.Wrap(err, "list bookmarks of "+accountID)
	}
	for _, row := range bookmarks {
		out.Bookmarks[row.StatusID] = bookmarkFrom(row)
	}
	pins, err := q.ListStatusPinsOn(ctx, contentgen.ListStatusPinsOnParams{AccountID: accountID, StatusIds: list})
	if err != nil {
		return out, sqlite.Wrap(err, "list pins of "+accountID)
	}
	for _, row := range pins {
		out.Pins[row.StatusID] = pinFrom(row)
	}
	reactions, err := q.ListAccountReactionsOn(ctx, contentgen.ListAccountReactionsOnParams{AccountID: accountID, StatusIds: list})
	if err != nil {
		return out, sqlite.Wrap(err, "list reactions of an account")
	}
	for _, row := range reactions {
		out.Reactions[row.StatusID] = append(out.Reactions[row.StatusID], reactionFrom(row))
	}
	return out, nil
}

// ListReactionCountsOf implements content.InteractionRepository.
func (r *InteractionRepository) ListReactionCountsOf(ctx context.Context, statusIDs []string, limit int) (map[string][]content.ReactionCount, error) {
	out := map[string][]content.ReactionCount{}
	if len(statusIDs) == 0 || limit <= 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListReactionCountsOf(ctx, contentgen.ListReactionCountsOfParams{StatusIds: list, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list reaction counts")
	}
	for _, c := range reactionCountsFrom(rows) {
		out[c.StatusID] = append(out[c.StatusID], c)
	}
	return out, nil
}

// FindBoosts implements content.InteractionRepository.
func (r *InteractionRepository) FindBoosts(ctx context.Context, ids []string) (map[string]content.Boost, error) {
	out := map[string]content.Boost{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListBoostsByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list boosts")
	}
	for _, row := range rows {
		out[row.ID] = boostFrom(row)
	}
	return out, nil
}
