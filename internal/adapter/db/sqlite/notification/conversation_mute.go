// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

var _ appnotification.ThreadMuteRepository = (*Repository)(nil)

// InsertConversationMute implements notification.ThreadMuteRepository.
func (r *Repository) InsertConversationMute(ctx context.Context, tx uow.Tx, m notification.ConversationMute) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertConversationMute(ctx, notificationgen.InsertConversationMuteParams{
		AccountID: m.AccountID, ConversationID: m.ConversationID, CreatedAt: sqlite.ToMillis(m.CreatedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert conversation mute")
}

// DeleteConversationMute implements notification.ThreadMuteRepository.
func (r *Repository) DeleteConversationMute(ctx context.Context, tx uow.Tx, accountID, conversationID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteConversationMute(ctx, notificationgen.DeleteConversationMuteParams{AccountID: accountID, ConversationID: conversationID})
	return n > 0, sqlite.Wrap(err, "delete conversation mute")
}

// ConversationMutersAmong implements notification.ThreadMuteRepository.
func (r *Repository) ConversationMutersAmong(ctx context.Context, tx uow.Tx, conversationID string, accountIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListConversationMuters(ctx, notificationgen.ListConversationMutersParams{ConversationID: conversationID, AccountIds: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list conversation muters")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// ListConversationMutes implements notification.ThreadMuteRepository.
func (r *Repository) ListConversationMutes(ctx context.Context, accountIDs, conversationIDs []string) ([]notification.ConversationMute, error) {
	if len(accountIDs) == 0 || len(conversationIDs) == 0 {
		return nil, nil
	}
	accounts, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	conversations, err := sqlite.JSONList(conversationIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListConversationMutesAmong(ctx, notificationgen.ListConversationMutesAmongParams{ConversationIds: conversations, AccountIds: accounts})
	if err != nil {
		return nil, sqlite.Wrap(err, "list conversation mutes")
	}
	out := make([]notification.ConversationMute, len(rows))
	for i, m := range rows {
		out[i] = notification.ConversationMute{AccountID: m.AccountID, ConversationID: m.ConversationID, CreatedAt: sqlite.FromMillis(m.CreatedAt)}
	}
	return out, nil
}

// MergeConversationMutes implements notification.ThreadMuteRepository.
func (r *Repository) MergeConversationMutes(ctx context.Context, tx uow.Tx, mergedID, intoID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.MoveConversationMutes(ctx, notificationgen.MoveConversationMutesParams{IntoConversationID: intoID, MergedConversationID: mergedID}); err != nil {
		return sqlite.Wrap(err, "move conversation mutes")
	}
	return sqlite.Wrap(q.DeleteMergedConversationMutes(ctx, mergedID), "delete merged conversation mutes")
}
