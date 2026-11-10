// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"errors"
	"slices"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The thread mutes (ADR-0158 §8, data-model §4.7): a local account mutes the conversation of a
// status it may see (content's conversations, ADR-0154), and the notifications of
// notification.ThreadMuted types about the statuses of that conversation are not created for it
// from then on, its own threads included; those created before stay, and unmuting does not bring
// back what happened meanwhile. Timelines still show the conversation. The merge of a
// conversation moves its mutes to the conversation it was merged into, in the transaction that
// merges, so a mute follows the thread whichever side of the merge it was on.

// ThreadMuteRepository is the persistence of the thread mutes (conversation_mutes, data-model
// §4.7), part of Repository.
type ThreadMuteRepository interface {
	// InsertConversationMute inserts a mute and reports false, writing nothing, when the account
	// already mutes the conversation.
	InsertConversationMute(ctx context.Context, tx uow.Tx, m notification.ConversationMute) (bool, error)
	// DeleteConversationMute deletes the account's mute of the conversation, if any, and reports
	// whether there was one.
	DeleteConversationMute(ctx context.Context, tx uow.Tx, accountID, conversationID string) (bool, error)
	// ConversationMutersAmong returns the accounts among accountIDs that mute the conversation,
	// with one query.
	ConversationMutersAmong(ctx context.Context, tx uow.Tx, conversationID string, accountIDs []string) (map[string]bool, error)
	// ListConversationMutes returns the mutes of the accounts among accountIDs of the
	// conversations among conversationIDs, in no order, with one query, outside any transaction.
	ListConversationMutes(ctx context.Context, accountIDs, conversationIDs []string) ([]notification.ConversationMute, error)
	// MergeConversationMutes moves the mutes of the merged conversation to the one it was merged
	// into; an account that mutes both keeps one mute of the latter. Moving them again changes
	// nothing.
	MergeConversationMutes(ctx context.Context, tx uow.Tx, mergedID, intoID string) error
}

// Conversations is the part of the content module's public API that the thread mutes read: the
// conversation of a status, the one a merged conversation ended in, and the lock that keeps a
// conversation from being merged while a mute of it is written. *appcontent.Service implements
// it.
type Conversations interface {
	ConversationOfTx(ctx context.Context, tx uow.Tx, statusID string) (appcontent.Conversation, error)
	FinalConversationTx(ctx context.Context, tx uow.Tx, id string) (appcontent.Conversation, error)
	LockConversationOfTx(ctx context.Context, tx uow.Tx, statusID string) (appcontent.Conversation, error)
	ConversationsOf(ctx context.Context, statusIDs []string) (map[string]appcontent.Conversation, error)
}

// ThreadMuteDeps are the ports of the thread mutes' use cases.
type ThreadMuteDeps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    ThreadMuteRepository
	Conversations Conversations
	Clock         kernel.Clock
}

// ThreadMutes runs the use cases of a local account's thread mutes: muting and unmuting the
// conversation of a status, and reading which statuses' conversations accounts mute. The caller
// checks that the account may see the status it mutes.
type ThreadMutes struct {
	d ThreadMuteDeps
}

// NewThreadMutes returns ThreadMutes on d.
func NewThreadMutes(d ThreadMuteDeps) *ThreadMutes { return &ThreadMutes{d: d} }

// Mute makes accountID mute the conversation of statusID. Muting again changes nothing. A status
// that is not stored fails with appcontent.ErrNotFound.
func (s *ThreadMutes) Mute(ctx context.Context, accountID, statusID string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		c, err := s.d.Conversations.LockConversationOfTx(ctx, tx, statusID)
		if err != nil {
			return err
		}
		_, err = s.d.Repository.InsertConversationMute(ctx, tx, notification.ConversationMute{
			AccountID: accountID, ConversationID: c.ID, CreatedAt: s.d.Clock.Now(),
		})
		return err
	})
}

// Unmute ends accountID's mute of the conversation of statusID. Without one it changes nothing.
// A status that is not stored fails with appcontent.ErrNotFound.
func (s *ThreadMutes) Unmute(ctx context.Context, accountID, statusID string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		c, err := s.d.Conversations.LockConversationOfTx(ctx, tx, statusID)
		if err != nil {
			return err
		}
		_, err = s.d.Repository.DeleteConversationMute(ctx, tx, accountID, c.ID)
		return err
	})
}

// MutedAmong returns, by account among accountIDs, the statuses among statusIDs whose
// conversation the account mutes, with two queries whatever their numbers. An account that mutes
// none of them is left out, as is a status that is not stored.
func (s *ThreadMutes) MutedAmong(ctx context.Context, accountIDs, statusIDs []string) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	if len(accountIDs) == 0 || len(statusIDs) == 0 {
		return out, nil
	}
	of, err := s.d.Conversations.ConversationsOf(ctx, statusIDs)
	if err != nil || len(of) == 0 {
		return out, err
	}
	byConversation := map[string][]string{}
	var ids []string
	for _, statusID := range statusIDs {
		c, ok := of[statusID]
		if !ok {
			continue
		}
		if _, seen := byConversation[c.ID]; !seen {
			ids = append(ids, c.ID)
		}
		if !slices.Contains(byConversation[c.ID], statusID) {
			byConversation[c.ID] = append(byConversation[c.ID], statusID)
		}
	}
	mutes, err := s.d.Repository.ListConversationMutes(ctx, uniqueIDs(accountIDs), ids)
	if err != nil {
		return nil, err
	}
	for _, m := range mutes {
		for _, statusID := range byConversation[m.ConversationID] {
			if out[m.AccountID] == nil {
				out[m.AccountID] = map[string]bool{}
			}
			out[m.AccountID][statusID] = true
		}
	}
	return out, nil
}

// uniqueIDs returns ids without repeats, in their first order.
func uniqueIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// registerThreadMuteProjector registers the projector that moves the mutes of a merged
// conversation to the one it was merged into (ADR-0158 §8), in the transaction of the merge.
func registerThreadMuteProjector(r *uow.Projectors, repo Repository) {
	r.Register(appcontent.EventConversationMerged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		m := e.(appcontent.ConversationMerged).Merge
		return repo.MergeConversationMutes(ctx, tx, m.MergedConversationID, m.IntoConversationID)
	})
}

// threadMuters returns the recipients among ids that mute the conversation of statusID, inside
// tx (ADR-0158 §4, §8). It reads the conversation that the status belongs to and its mutes, and,
// when none of those recipients mutes it, the conversation that it may have been merged into by a
// transaction that committed between the two reads, so that a merge that moves the mutes does
// not let a notification through. Without Conversations, or for a status without a conversation,
// nobody mutes it.
func (p *projector) threadMuters(ctx context.Context, tx uow.Tx, statusID string, ids []string) (map[string]bool, error) {
	if p.d.Conversations == nil || statusID == "" || len(ids) == 0 {
		return nil, nil
	}
	c, err := p.d.Conversations.ConversationOfTx(ctx, tx, statusID)
	if errors.Is(err, appcontent.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	muters, err := p.d.Repository.ConversationMutersAmong(ctx, tx, c.ID, ids)
	if err != nil || len(muters) == len(ids) {
		return muters, err
	}
	final, err := p.d.Conversations.FinalConversationTx(ctx, tx, c.ID)
	if err != nil || final.ID == c.ID {
		return muters, err
	}
	rest := slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return muters[id] })
	more, err := p.d.Repository.ConversationMutersAmong(ctx, tx, final.ID, rest)
	if err != nil {
		return nil, err
	}
	for id := range more {
		muters[id] = true
	}
	return muters, nil
}

// threadMuted reports whether recipientID mutes the conversation of statusID (threadMuters).
func (p *projector) threadMuted(ctx context.Context, tx uow.Tx, recipientID, statusID string) (bool, error) {
	muters, err := p.threadMuters(ctx, tx, statusID, []string{recipientID})
	return muters[recipientID], err
}

// threadOf returns the status whose conversation's mutes keep n from being created inside notify
// (ADR-0158 §8): its status for a notification.ThreadMuted type, "" for the others. A quote's is
// the quoted status, which n does not hold, so the projector of quotes checks it itself.
func threadOf(n notification.Notification) string {
	if n.Type == notification.TypeQuote || !notification.ThreadMuted(n.Type) {
		return ""
	}
	return n.StatusID
}
