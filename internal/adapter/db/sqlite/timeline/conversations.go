// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// AddConversationEntry implements timeline.ConversationRepository.
func (r *Repository) AddConversationEntry(ctx context.Context, tx uow.Tx, conversationID, entryID string, accountIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return err
	}
	if err := q.InsertFeedConversationParticipants(ctx, timelinegen.InsertFeedConversationParticipantsParams{
		ConversationID: conversationID, EntryID: entryID, AccountIds: ids,
	}); err != nil {
		return sqlite.Wrap(err, "insert feed conversation participants")
	}
	return sqlite.Wrap(q.AdvanceFeedConversations(ctx, entryID), "advance feed conversations")
}

// ReplaceConversationEntryParties implements timeline.ConversationRepository.
func (r *Repository) ReplaceConversationEntryParties(ctx context.Context, tx uow.Tx, entryID string, accountIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	entries, err := sqlite.JSONList([]string{entryID})
	if err != nil {
		return err
	}
	convs, err := q.ListFeedConversationIDsOfEntries(ctx, entries)
	if err != nil {
		return sqlite.Wrap(err, "list the conversations of feed entries")
	}
	if len(convs) == 0 {
		return nil
	}
	if err := q.DeleteFeedConversationParticipantsOfEntries(ctx, entries); err != nil {
		return sqlite.Wrap(err, "delete feed conversation participants")
	}
	ids, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return err
	}
	if err := q.InsertFeedConversationParticipants(ctx, timelinegen.InsertFeedConversationParticipantsParams{
		ConversationID: convs[0], EntryID: entryID, AccountIds: ids,
	}); err != nil {
		return sqlite.Wrap(err, "insert feed conversation participants")
	}
	return refreshConversations(ctx, q, convs)
}

// DeleteConversationEntries implements timeline.ConversationRepository.
func (r *Repository) DeleteConversationEntries(ctx context.Context, tx uow.Tx, entryIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return deleteConversationEntries(ctx, q, entryIDs)
}

// deleteConversationEntries takes the entries out of their conversations and derives the last
// entries of those conversations again.
func deleteConversationEntries(ctx context.Context, q *timelinegen.Queries, entryIDs []string) error {
	if len(entryIDs) == 0 {
		return nil
	}
	entries, err := sqlite.JSONList(entryIDs)
	if err != nil {
		return err
	}
	convs, err := q.ListFeedConversationIDsOfEntries(ctx, entries)
	if err != nil {
		return sqlite.Wrap(err, "list the conversations of feed entries")
	}
	if len(convs) == 0 {
		return nil
	}
	if err := q.DeleteFeedConversationParticipantsOfEntries(ctx, entries); err != nil {
		return sqlite.Wrap(err, "delete feed conversation participants")
	}
	return refreshConversations(ctx, q, convs)
}

// refreshConversations derives the rows of feed_conversations of the conversations again.
func refreshConversations(ctx context.Context, q *timelinegen.Queries, conversationIDs []string) error {
	ids, err := sqlite.JSONList(conversationIDs)
	if err != nil {
		return err
	}
	if err := q.DeleteFeedConversationsOf(ctx, ids); err != nil {
		return sqlite.Wrap(err, "delete feed conversations")
	}
	return sqlite.Wrap(q.InsertFeedConversationsOf(ctx, ids), "insert feed conversations")
}

// MergeConversations implements timeline.ConversationRepository.
func (r *Repository) MergeConversations(ctx context.Context, tx uow.Tx, mergedID, intoID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.MoveFeedConversationParticipants(ctx, timelinegen.MoveFeedConversationParticipantsParams{IntoID: intoID, MergedID: mergedID}); err != nil {
		return sqlite.Wrap(err, "move feed conversation participants")
	}
	if err := q.MergeFeedConversations(ctx, timelinegen.MergeFeedConversationsParams{IntoID: intoID, MergedID: mergedID}); err != nil {
		return sqlite.Wrap(err, "merge feed conversations")
	}
	if err := q.DeleteFeedConversationsOfConversation(ctx, mergedID); err != nil {
		return sqlite.Wrap(err, "delete merged feed conversations")
	}
	if err := q.MergeFeedConversationReads(ctx, timelinegen.MergeFeedConversationReadsParams{IntoID: intoID, MergedID: mergedID}); err != nil {
		return sqlite.Wrap(err, "merge feed conversation reads")
	}
	if err := q.DeleteFeedConversationReadsOfConversation(ctx, mergedID); err != nil {
		return sqlite.Wrap(err, "delete merged feed conversation reads")
	}
	if err := q.MergeFeedConversationHides(ctx, timelinegen.MergeFeedConversationHidesParams{IntoID: intoID, MergedID: mergedID}); err != nil {
		return sqlite.Wrap(err, "merge feed conversation hides")
	}
	return sqlite.Wrap(q.DeleteFeedConversationHidesOfConversation(ctx, mergedID), "delete merged feed conversation hides")
}

// MarkConversationRead implements timeline.ConversationRepository.
func (r *Repository) MarkConversationRead(ctx context.Context, tx uow.Tx, accountID, conversationID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.MarkFeedConversationRead(ctx, timelinegen.MarkFeedConversationReadParams{AccountID: accountID, ConversationID: conversationID})
	return n > 0, sqlite.Wrap(err, "mark a feed conversation read")
}

// HideConversation implements timeline.ConversationRepository.
func (r *Repository) HideConversation(ctx context.Context, tx uow.Tx, accountID, conversationID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.HideFeedConversation(ctx, timelinegen.HideFeedConversationParams{AccountID: accountID, ConversationID: conversationID})
	return n > 0, sqlite.Wrap(err, "hide a feed conversation")
}

// ListConversations implements timeline.ConversationRepository.
func (r *Repository) ListConversations(ctx context.Context, cq apptimeline.ConversationsQuery) ([]apptimeline.ConversationRow, error) {
	rows, err := r.db.reader().ListFeedConversations(ctx, timelinegen.ListFeedConversationsParams{
		Viewer: cq.ViewerAccountID, Before: cq.Before, PageLimit: int64(cq.Limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list feed conversations")
	}
	out := make([]apptimeline.ConversationRow, len(rows))
	for i, row := range rows {
		out[i] = apptimeline.ConversationRow{ConversationID: row.ConversationID, LastEntryID: row.LastEntryID, ReadEntryID: row.ReadEntryID}
	}
	return out, nil
}

// ConversationsAmong implements timeline.ConversationRepository.
func (r *Repository) ConversationsAmong(ctx context.Context, viewerAccountID string, conversationIDs []string) ([]apptimeline.ConversationRow, error) {
	if len(conversationIDs) == 0 {
		return nil, nil
	}
	ids, err := sqlite.JSONList(conversationIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListFeedConversationsAmong(ctx, timelinegen.ListFeedConversationsAmongParams{Viewer: viewerAccountID, ConversationIds: ids})
	if err != nil {
		return nil, sqlite.Wrap(err, "list feed conversations")
	}
	out := make([]apptimeline.ConversationRow, len(rows))
	for i, row := range rows {
		out[i] = apptimeline.ConversationRow{
			ConversationID: row.ConversationID, LastEntryID: row.LastEntryID, ReadEntryID: row.ReadEntryID, Hidden: row.Hidden == 1,
		}
	}
	return out, nil
}

// LastVisibleConversationEntries implements timeline.ConversationRepository.
func (r *Repository) LastVisibleConversationEntries(ctx context.Context, viewerAccountID string, conversationIDs []string, now time.Time) (map[string]apptimeline.LastConversationEntries, error) {
	out := map[string]apptimeline.LastConversationEntries{}
	if len(conversationIDs) == 0 {
		return out, nil
	}
	ids, err := sqlite.JSONList(conversationIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLastVisibleFeedConversationEntries(ctx, timelinegen.ListLastVisibleFeedConversationEntriesParams{
		Viewer: viewerAccountID, ConversationIds: ids, Now: sqlite.ToMillis(now),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list the last feed conversation entries")
	}
	for _, row := range rows {
		out[row.ConversationID] = apptimeline.LastConversationEntries(row)
	}
	return out, nil
}

// ConversationParticipants implements timeline.ConversationRepository.
func (r *Repository) ConversationParticipants(ctx context.Context, viewerAccountID string, conversationIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(conversationIDs) == 0 {
		return out, nil
	}
	ids, err := sqlite.JSONList(conversationIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListFeedConversationParticipants(ctx, timelinegen.ListFeedConversationParticipantsParams{Viewer: viewerAccountID, ConversationIds: ids})
	if err != nil {
		return nil, sqlite.Wrap(err, "list feed conversation participants")
	}
	for _, row := range rows {
		out[row.ConversationID] = append(out[row.ConversationID], row.AccountID)
	}
	return out, nil
}

// EntriesByID implements timeline.ConversationRepository.
func (r *Repository) EntriesByID(ctx context.Context, ids []string) ([]apptimeline.Entry, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListFeedEntriesByID(ctx, list)
	return entriesFrom(rows, err, "list feed entries by ID")
}

// conversationSnapshot reads the projections of the direct messages by conversation into p.
func conversationSnapshot(ctx context.Context, q *timelinegen.Queries, p *apptimeline.Projection) error {
	parts, err := q.ListAllFeedConversationParticipants(ctx)
	if err != nil {
		return sqlite.Wrap(err, "list feed conversation participants")
	}
	for _, row := range parts {
		p.ConversationParticipants = append(p.ConversationParticipants, apptimeline.ConversationParticipant(row))
	}
	heads, err := q.ListAllFeedConversations(ctx)
	if err != nil {
		return sqlite.Wrap(err, "list feed conversations")
	}
	for _, row := range heads {
		p.ConversationHeads = append(p.ConversationHeads, apptimeline.ConversationHead(row))
	}
	return nil
}
