// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// InsertList implements timeline.Repository.
func (r *TimelineRepository) InsertList(ctx context.Context, tx uow.Tx, l apptimeline.List) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.InsertList(ctx, postgresgen.InsertListParams{ID: l.ID, OwnerAccountID: l.OwnerAccountID, CreatedAt: l.CreatedAt}); err != nil {
		return wrap(err, "insert list")
	}
	return wrap(q.UpsertListName(ctx, postgresgen.UpsertListNameParams{ListID: l.ID, Name: l.Name}), "insert list name")
}

// RenameList implements timeline.Repository.
func (r *TimelineRepository) RenameList(ctx context.Context, tx uow.Tx, id, name string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertListName(ctx, postgresgen.UpsertListNameParams{ListID: id, Name: name}), "rename list")
}

// DeleteList implements timeline.Repository.
func (r *TimelineRepository) DeleteList(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range []func(context.Context, string) error{q.DeleteListMembersOfList, q.DeleteListName, q.DeleteList} {
		if err := step(ctx, id); err != nil {
			return wrap(err, "delete list")
		}
	}
	return nil
}

// FindList implements timeline.Repository.
func (r *TimelineRepository) FindList(ctx context.Context, tx uow.Tx, id string) (apptimeline.List, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return apptimeline.List{}, err
	}
	return listFrom(q.GetList(ctx, id))
}

// List implements timeline.Repository.
func (r *TimelineRepository) List(ctx context.Context, id string) (apptimeline.List, error) {
	return listFrom(r.db.reader().GetList(ctx, id))
}

func listFrom(row postgresgen.GetListRow, err error) (apptimeline.List, error) {
	if err != nil {
		return apptimeline.List{}, notFound(err, apptimeline.ErrNotFound, "list")
	}
	return apptimeline.List{ID: row.ID, OwnerAccountID: row.OwnerAccountID, Name: row.Name, CreatedAt: utc(row.CreatedAt), MembersCount: int(row.MembersCount)}, nil
}

// CountLists implements timeline.Repository.
func (r *TimelineRepository) CountLists(ctx context.Context, tx uow.Tx, ownerAccountID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountListsOfOwner(ctx, ownerAccountID)
	return int(n), wrap(err, "count lists")
}

// Following implements timeline.Repository.
func (r *TimelineRepository) Following(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.FeedFollowEdgeExists(ctx, postgresgen.FeedFollowEdgeExistsParams{
		FollowerAccountID: followerAccountID, FolloweeAccountID: followeeAccountID,
	})
	return ok, wrap(err, "look up feed follow edge")
}

// InsertListMember implements timeline.Repository.
func (r *TimelineRepository) InsertListMember(ctx context.Context, tx uow.Tx, m apptimeline.ListMember) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertListMember(ctx, postgresgen.InsertListMemberParams{
		ListID: m.ListID, MemberAccountID: m.MemberAccountID, OwnerAccountID: m.OwnerAccountID, AddedAt: m.AddedAt,
	})
	return n == 1, wrap(err, "insert list member")
}

// DeleteListMember implements timeline.Repository.
func (r *TimelineRepository) DeleteListMember(ctx context.Context, tx uow.Tx, listID, memberAccountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteListMember(ctx, postgresgen.DeleteListMemberParams{ListID: listID, MemberAccountID: memberAccountID}), "delete list member")
}

// CountListMembers implements timeline.Repository.
func (r *TimelineRepository) CountListMembers(ctx context.Context, tx uow.Tx, listID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountListMembers(ctx, listID)
	return int(n), wrap(err, "count list members")
}

// DeleteListMembersOfFollow implements timeline.Repository.
func (r *TimelineRepository) DeleteListMembersOfFollow(ctx context.Context, tx uow.Tx, ownerAccountID, memberAccountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteListMembersOfFollow(ctx, postgresgen.DeleteListMembersOfFollowParams{
		OwnerAccountID: ownerAccountID, MemberAccountID: memberAccountID,
	}), "delete list members of a follow")
}

// DeleteListMembersWithoutFollow implements timeline.Repository.
func (r *TimelineRepository) DeleteListMembersWithoutFollow(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteListMembersWithoutFollow(ctx), "delete list members without a follow")
}

// ListsOf implements timeline.Repository.
func (r *TimelineRepository) ListsOf(ctx context.Context, ownerAccountID string) ([]apptimeline.List, error) {
	rows, err := r.db.reader().ListListsOfOwner(ctx, ownerAccountID)
	if err != nil {
		return nil, wrap(err, "list lists")
	}
	out := make([]apptimeline.List, len(rows))
	for i, row := range rows {
		out[i] = apptimeline.List{ID: row.ID, OwnerAccountID: row.OwnerAccountID, Name: row.Name, CreatedAt: utc(row.CreatedAt), MembersCount: int(row.MembersCount)}
	}
	return out, nil
}

// ListMembers implements timeline.Repository.
func (r *TimelineRepository) ListMembers(ctx context.Context, listID, after string, limit int) ([]string, error) {
	lim, err := toInt32(limit, "page limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListListMembers(ctx, postgresgen.ListListMembersParams{ListID: listID, After: toNullString(after), PageLimit: lim})
	return ids, wrap(err, "list list members")
}

// ListIDsWithMember implements timeline.Repository.
func (r *TimelineRepository) ListIDsWithMember(ctx context.Context, ownerAccountID, memberAccountID string) ([]string, error) {
	ids, err := r.db.reader().ListListIDsWithMember(ctx, postgresgen.ListListIDsWithMemberParams{
		OwnerAccountID: ownerAccountID, MemberAccountID: memberAccountID,
	})
	return ids, wrap(err, "list the lists with a member")
}

// AllListMembers implements timeline.Repository.
func (r *TimelineRepository) AllListMembers(ctx context.Context) ([]apptimeline.ListMember, error) {
	rows, err := r.db.reader().ListAllListMembers(ctx)
	if err != nil {
		return nil, wrap(err, "list all list members")
	}
	out := make([]apptimeline.ListMember, len(rows))
	for i, row := range rows {
		out[i] = apptimeline.ListMember{
			ListID: row.ListID, MemberAccountID: row.MemberAccountID, OwnerAccountID: row.OwnerAccountID, AddedAt: utc(row.AddedAt),
		}
	}
	return out, nil
}

// ListListEntries implements timeline.Repository.
func (r *TimelineRepository) ListListEntries(ctx context.Context, lq apptimeline.ListTimelineQuery) ([]apptimeline.Entry, error) {
	all, langs := languages(lq.Languages)
	limit, err := toInt32(lq.Limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListListEntries(ctx, postgresgen.ListListEntriesParams{
		ListID: lq.ListID, Viewer: lq.ViewerAccountID, Before: toNullString(lq.Before), WindowFloor: toNullString(lq.Floor), Now: lq.Now,
		AllLanguages: all, Languages: langs, PageLimit: limit,
	})
	return entriesFrom(rows, err, "list list entries")
}
