// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
)

// The conversations as the modules that key rows by them read them inside their transactions:
// the thread mutes of the notification module (ADR-0158 §8).

// ConversationOfTx returns, inside tx, the conversation that a status belongs to now. A merge
// moves the statuses of the merged conversation, so it is never a merged conversation. A status
// that is not stored fails with ErrNotFound.
func (s *Service) ConversationOfTx(ctx context.Context, tx uow.Tx, statusID string) (Conversation, error) {
	of, err := s.d.Repository.StatusConversationsTx(ctx, tx, []string{statusID})
	if err != nil {
		return Conversation{}, err
	}
	c, ok := of[statusID]
	if !ok {
		return Conversation{}, fmt.Errorf("conversation of status %s: %w", statusID, ErrNotFound)
	}
	return c, nil
}

// FinalConversationTx returns, inside tx, the conversation that id names, or the one it ended in
// when it was merged (ADR-0154 §3). An ID that no conversation has fails with ErrNotFound.
func (s *Service) FinalConversationTx(ctx context.Context, tx uow.Tx, id string) (Conversation, error) {
	return s.finalConversationTx(ctx, tx, id)
}

// LockConversationOfTx returns, inside tx, the conversation that a status belongs to now, holding
// until tx ends the lock that a merge of that conversation takes (insertStatus), so that a row
// that tx keys by the conversation is written either before the merge, which then moves it, or
// after it, on the conversation that the status was moved to. A status that is not stored fails
// with ErrNotFound.
func (s *Service) LockConversationOfTx(ctx context.Context, tx uow.Tx, statusID string) (Conversation, error) {
	locked := map[string]bool{}
	for range maxConversationLockRounds {
		c, err := s.ConversationOfTx(ctx, tx, statusID)
		if err != nil || locked[c.ID] {
			return c, err
		}
		if err := s.d.Repository.LockStatusKeys(ctx, tx, []string{lockConversation + c.ID}); err != nil {
			return Conversation{}, err
		}
		locked[c.ID] = true
	}
	return Conversation{}, fmt.Errorf("status %s: %w", statusID, errConversationsMoving)
}
