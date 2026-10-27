// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// InteractionRepository implements content.InteractionRepository on SQLite.
type InteractionRepository struct {
	db *DB
}

var _ appcontent.InteractionRepository = (*InteractionRepository)(nil)

// InteractionRepository returns the content module's repository of boosts, favourites,
// bookmarks, and pins on d.
func (d *DB) InteractionRepository() *InteractionRepository { return &InteractionRepository{db: d} }

// InsertBoost implements content.InteractionRepository.
func (r *InteractionRepository) InsertBoost(ctx context.Context, tx uow.Tx, b content.Boost) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertBoost(ctx, sqlitegen.InsertBoostParams{
		ID: b.ID, Uri: b.URI, BoosterAccountID: b.BoosterAccountID, StatusID: b.StatusID,
		Visibility: string(b.Visibility), IsLocal: boolToInt(b.IsLocal), BoostedAt: toMillis(b.BoostedAt),
	})
	if isUniqueViolation(err, "boosts.uri") {
		return fmt.Errorf("insert boost %s: %w", b.URI, appcontent.ErrBoostExists)
	}
	return wrap(err, "insert boost")
}

// InsertBoostRetraction implements content.InteractionRepository.
func (r *InteractionRepository) InsertBoostRetraction(ctx context.Context, tx uow.Tx, br content.BoostRetraction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertBoostRetraction(ctx, sqlitegen.InsertBoostRetractionParams{
		BoostID: br.BoostID, Origin: string(br.Origin), RetractedAt: toMillis(br.RetractedAt),
	})
	if isUniqueViolation(err, "boost_retractions.boost_id") {
		return fmt.Errorf("retract boost %s: %w", br.BoostID, appcontent.ErrBoostRetracted)
	}
	return wrap(err, "insert boost retraction")
}

// Boost implements content.InteractionRepository.
func (r *InteractionRepository) Boost(ctx context.Context, tx uow.Tx, id string) (content.Boost, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Boost{}, err
	}
	row, err := q.GetBoost(ctx, id)
	if err != nil {
		return content.Boost{}, notFound(err, appcontent.ErrNotFound, "boost "+id)
	}
	return boostFrom(row), nil
}

// BoostEdge implements content.InteractionRepository.
func (r *InteractionRepository) BoostEdge(ctx context.Context, tx uow.Tx, boosterAccountID, statusID string) (content.BoostEdge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.BoostEdge{}, err
	}
	return boostEdge(ctx, q, boosterAccountID, statusID)
}

func boostEdge(ctx context.Context, q *sqlitegen.Queries, boosterAccountID, statusID string) (content.BoostEdge, error) {
	row, err := q.GetBoostEdge(ctx, sqlitegen.GetBoostEdgeParams{BoosterAccountID: boosterAccountID, StatusID: statusID})
	if err != nil {
		return content.BoostEdge{}, notFound(err, appcontent.ErrNotFound, "boost edge of "+boosterAccountID+" on "+statusID)
	}
	return content.BoostEdge(row), nil
}

// InsertBoostEdge implements content.InteractionRepository.
func (r *InteractionRepository) InsertBoostEdge(ctx context.Context, tx uow.Tx, e content.BoostEdge) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertBoostEdge(ctx, sqlitegen.InsertBoostEdgeParams(e))
	if err != nil {
		return false, wrap(err, "insert boost edge")
	}
	return n == 1, nil
}

// DeleteBoostEdge implements content.InteractionRepository.
func (r *InteractionRepository) DeleteBoostEdge(ctx context.Context, tx uow.Tx, e content.BoostEdge) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteBoostEdge(ctx, sqlitegen.DeleteBoostEdgeParams(e))
	if err != nil {
		return false, wrap(err, "delete boost edge")
	}
	return n == 1, nil
}

// DeleteStatusBoostEdges implements content.InteractionRepository.
func (r *InteractionRepository) DeleteStatusBoostEdges(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteStatusBoostEdges(ctx, statusID), "delete boost edges of a status")
}

// RebuildBoostEdges implements content.InteractionRepository.
func (r *InteractionRepository) RebuildBoostEdges(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllBoostEdges(ctx); err != nil {
		return wrap(err, "rebuild boost edges")
	}
	return wrap(q.RebuildBoostEdges(ctx), "rebuild boost edges")
}

// ListBoostEdges implements content.InteractionRepository.
func (r *InteractionRepository) ListBoostEdges(ctx context.Context) ([]content.BoostEdge, error) {
	rows, err := r.db.reader().ListBoostEdges(ctx)
	if err != nil {
		return nil, wrap(err, "list boost edges")
	}
	out := make([]content.BoostEdge, len(rows))
	for i, row := range rows {
		out[i] = content.BoostEdge(row)
	}
	return out, nil
}

// AddBoosts implements content.InteractionRepository.
func (r *InteractionRepository) AddBoosts(ctx context.Context, tx uow.Tx, statusID string, delta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.AddStatusBoosts(ctx, sqlitegen.AddStatusBoostsParams{Delta: delta, StatusID: statusID}), "add boosts")
}

// AddFavourites implements content.InteractionRepository.
func (r *InteractionRepository) AddFavourites(ctx context.Context, tx uow.Tx, statusID string, delta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.AddStatusFavourites(ctx, sqlitegen.AddStatusFavouritesParams{Delta: delta, StatusID: statusID}), "add favourites")
}

// Mentioned implements content.InteractionRepository.
func (r *InteractionRepository) Mentioned(ctx context.Context, statusID, accountID string) (bool, error) {
	n, err := r.db.reader().CountStatusMention(ctx, sqlitegen.CountStatusMentionParams{StatusID: statusID, AccountID: accountID})
	if err != nil {
		return false, wrap(err, "find mention")
	}
	return n > 0, nil
}

// InsertFavourite implements content.InteractionRepository.
func (r *InteractionRepository) InsertFavourite(ctx context.Context, tx uow.Tx, f content.Favourite) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertFavourite(ctx, sqlitegen.InsertFavouriteParams{
		AccountID: f.AccountID, StatusID: f.StatusID, Uri: f.URI, CreatedAt: toMillis(f.CreatedAt), ID: f.ID,
	})
	if isUniqueViolation(err, "favourites.uri") {
		return false, fmt.Errorf("insert favourite %s: %w", f.URI, appcontent.ErrFavouriteURIUsed)
	}
	if err != nil {
		return false, wrap(err, "insert favourite")
	}
	return n == 1, nil
}

// DeleteFavourite implements content.InteractionRepository.
func (r *InteractionRepository) DeleteFavourite(ctx context.Context, tx uow.Tx, accountID, statusID string) (content.Favourite, bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Favourite{}, false, err
	}
	rows, err := q.DeleteFavourite(ctx, sqlitegen.DeleteFavouriteParams{AccountID: accountID, StatusID: statusID})
	if err != nil {
		return content.Favourite{}, false, wrap(err, "delete favourite")
	}
	if len(rows) == 0 {
		return content.Favourite{}, false, nil
	}
	return favouriteFrom(rows[0]), true, nil
}

// InsertBookmark implements content.InteractionRepository.
func (r *InteractionRepository) InsertBookmark(ctx context.Context, tx uow.Tx, b content.Bookmark) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertBookmark(ctx, sqlitegen.InsertBookmarkParams{
		AccountID: b.AccountID, StatusID: b.StatusID, CreatedAt: toMillis(b.CreatedAt), ID: b.ID,
	})
	if err != nil {
		return false, wrap(err, "insert bookmark")
	}
	return n == 1, nil
}

// DeleteBookmark implements content.InteractionRepository.
func (r *InteractionRepository) DeleteBookmark(ctx context.Context, tx uow.Tx, accountID, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteBookmark(ctx, sqlitegen.DeleteBookmarkParams{AccountID: accountID, StatusID: statusID})
	if err != nil {
		return false, wrap(err, "delete bookmark")
	}
	return n == 1, nil
}

// InsertPin implements content.InteractionRepository.
func (r *InteractionRepository) InsertPin(ctx context.Context, tx uow.Tx, p content.Pin) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertStatusPin(ctx, sqlitegen.InsertStatusPinParams{
		AccountID: p.AccountID, StatusID: p.StatusID, CreatedAt: toMillis(p.CreatedAt), ID: p.ID, MaxSlot: content.MaxPins,
	})
	if err != nil {
		return false, wrap(err, "insert status pin")
	}
	return n == 1, nil
}

// HasPin implements content.InteractionRepository.
func (r *InteractionRepository) HasPin(ctx context.Context, tx uow.Tx, accountID, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.StatusPinExists(ctx, sqlitegen.StatusPinExistsParams{AccountID: accountID, StatusID: statusID})
	return ok, wrap(err, "check status pin")
}

// DeletePin implements content.InteractionRepository.
func (r *InteractionRepository) DeletePin(ctx context.Context, tx uow.Tx, accountID, statusID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteStatusPin(ctx, sqlitegen.DeleteStatusPinParams{AccountID: accountID, StatusID: statusID})
	if err != nil {
		return false, wrap(err, "delete status pin")
	}
	return n == 1, nil
}

// DeletePinsOf implements content.InteractionRepository.
func (r *InteractionRepository) DeletePinsOf(ctx context.Context, tx uow.Tx, accountID string) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteStatusPinsOf(ctx, accountID)
	return n, wrap(err, "delete status pins")
}

// CountPins implements content.InteractionRepository.
func (r *InteractionRepository) CountPins(ctx context.Context, tx uow.Tx, accountID string) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountStatusPins(ctx, accountID)
	return n, wrap(err, "count status pins")
}

// FindBoost implements content.InteractionRepository.
func (r *InteractionRepository) FindBoost(ctx context.Context, id string) (content.Boost, error) {
	row, err := r.db.reader().GetBoost(ctx, id)
	if err != nil {
		return content.Boost{}, notFound(err, appcontent.ErrNotFound, "boost "+id)
	}
	return boostFrom(row), nil
}

// FindBoostByURI implements content.InteractionRepository.
func (r *InteractionRepository) FindBoostByURI(ctx context.Context, uri string) (content.Boost, error) {
	row, err := r.db.reader().GetBoostByURI(ctx, uri)
	if err != nil {
		return content.Boost{}, notFound(err, appcontent.ErrNotFound, "boost "+uri)
	}
	return boostFrom(row), nil
}

// FindBoostRetraction implements content.InteractionRepository.
func (r *InteractionRepository) FindBoostRetraction(ctx context.Context, boostID string) (content.BoostRetraction, error) {
	row, err := r.db.reader().GetBoostRetraction(ctx, boostID)
	if err != nil {
		return content.BoostRetraction{}, notFound(err, appcontent.ErrNotFound, "retraction of boost "+boostID)
	}
	return content.BoostRetraction{
		BoostID: row.BoostID, Origin: content.BoostRetractionOrigin(row.Origin), RetractedAt: fromMillis(row.RetractedAt),
	}, nil
}

// FindBoostEdge implements content.InteractionRepository.
func (r *InteractionRepository) FindBoostEdge(ctx context.Context, boosterAccountID, statusID string) (content.BoostEdge, error) {
	return boostEdge(ctx, r.db.reader(), boosterAccountID, statusID)
}

// FindFavourite implements content.InteractionRepository.
func (r *InteractionRepository) FindFavourite(ctx context.Context, accountID, statusID string) (content.Favourite, error) {
	row, err := r.db.reader().GetFavourite(ctx, sqlitegen.GetFavouriteParams{AccountID: accountID, StatusID: statusID})
	if err != nil {
		return content.Favourite{}, notFound(err, appcontent.ErrNotFound, "favourite of "+accountID+" on "+statusID)
	}
	return favouriteFrom(row), nil
}

// FindFavouriteByURI implements content.InteractionRepository.
func (r *InteractionRepository) FindFavouriteByURI(ctx context.Context, uri string) (content.Favourite, error) {
	row, err := r.db.reader().GetFavouriteByURI(ctx, uri)
	if err != nil {
		return content.Favourite{}, notFound(err, appcontent.ErrNotFound, "favourite "+uri)
	}
	return favouriteFrom(row), nil
}

// FindBookmark implements content.InteractionRepository.
func (r *InteractionRepository) FindBookmark(ctx context.Context, accountID, statusID string) (content.Bookmark, error) {
	row, err := r.db.reader().GetBookmark(ctx, sqlitegen.GetBookmarkParams{AccountID: accountID, StatusID: statusID})
	if err != nil {
		return content.Bookmark{}, notFound(err, appcontent.ErrNotFound, "bookmark of "+accountID+" on "+statusID)
	}
	return bookmarkFrom(row), nil
}

// FindPin implements content.InteractionRepository.
func (r *InteractionRepository) FindPin(ctx context.Context, accountID, statusID string) (content.Pin, error) {
	row, err := r.db.reader().GetStatusPin(ctx, sqlitegen.GetStatusPinParams{AccountID: accountID, StatusID: statusID})
	if err != nil {
		return content.Pin{}, notFound(err, appcontent.ErrNotFound, "pin of "+accountID+" on "+statusID)
	}
	return pinFrom(row), nil
}

// ListFavourites implements content.InteractionRepository.
func (r *InteractionRepository) ListFavourites(ctx context.Context, accountID, before string, limit int) ([]content.Favourite, error) {
	rows, err := r.db.reader().ListFavourites(ctx, sqlitegen.ListFavouritesParams{AccountID: accountID, Before: before, PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list favourites")
	}
	out := make([]content.Favourite, len(rows))
	for i, row := range rows {
		out[i] = favouriteFrom(row)
	}
	return out, nil
}

// ListBookmarks implements content.InteractionRepository.
func (r *InteractionRepository) ListBookmarks(ctx context.Context, accountID, before string, limit int) ([]content.Bookmark, error) {
	rows, err := r.db.reader().ListBookmarks(ctx, sqlitegen.ListBookmarksParams{AccountID: accountID, Before: before, PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list bookmarks")
	}
	out := make([]content.Bookmark, len(rows))
	for i, row := range rows {
		out[i] = bookmarkFrom(row)
	}
	return out, nil
}

// ListPins implements content.InteractionRepository.
func (r *InteractionRepository) ListPins(ctx context.Context, accountID string) ([]content.Pin, error) {
	rows, err := r.db.reader().ListStatusPins(ctx, accountID)
	if err != nil {
		return nil, wrap(err, "list status pins")
	}
	out := make([]content.Pin, len(rows))
	for i, row := range rows {
		out[i] = pinFrom(row)
	}
	return out, nil
}

func boostFrom(row sqlitegen.Boost) content.Boost {
	return content.Boost{
		ID: row.ID, URI: row.Uri, BoosterAccountID: row.BoosterAccountID, StatusID: row.StatusID,
		Visibility: content.Visibility(row.Visibility), IsLocal: row.IsLocal == 1, BoostedAt: fromMillis(row.BoostedAt),
	}
}

func favouriteFrom(row sqlitegen.Favourite) content.Favourite {
	return content.Favourite{ID: row.ID, AccountID: row.AccountID, StatusID: row.StatusID, URI: row.Uri, CreatedAt: fromMillis(row.CreatedAt)}
}

func bookmarkFrom(row sqlitegen.Bookmark) content.Bookmark {
	return content.Bookmark{ID: row.ID, AccountID: row.AccountID, StatusID: row.StatusID, CreatedAt: fromMillis(row.CreatedAt)}
}

func pinFrom(row sqlitegen.StatusPin) content.Pin {
	return content.Pin{ID: row.ID, AccountID: row.AccountID, StatusID: row.StatusID, CreatedAt: fromMillis(row.CreatedAt)}
}
