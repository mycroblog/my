// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// Conversation is a thread of statuses (ADR-0154).
type Conversation = content.Conversation

// EventConversationMerged is emitted when a conversation is merged into another (ADR-0154 §3).
// The projections and thread mutes keyed by conversation follow it, moving the rows of the merged
// conversation to the one it is merged into.
const EventConversationMerged = "content.conversation_merged"

// ConversationMerged is emitted inside the transaction that stores the status whose reply link
// showed two conversations to be one thread, once per merge.
type ConversationMerged struct {
	Merge content.ConversationMerge
}

// EventType implements uow.Event.
func (ConversationMerged) EventType() string { return EventConversationMerged }

// Lock keys of the conversations (Repository.LockStatusKeys). A declared IRI is locked before the
// conversation is looked up by it, so that two posts that declare a new IRI make one
// conversation; the IDs of the conversations that a status joins or merges are locked before
// they are written, so that a status joining a conversation and the merge of that conversation
// do not miss each other.
const (
	lockConversationURI = "conversation-uri:"
	lockConversation    = "conversation:"
)

// maxConversationLockRounds bounds how many times insertStatus locks more conversations that
// concurrent merges moved its statuses to; more rounds than this fail.
const maxConversationLockRounds = 8

// errConversationsMoving reports conversations that kept moving while insertStatus locked them.
var errConversationsMoving = errors.New("content: the conversations of a status kept moving")

// insertStatus inserts st inside tx and records the conversation it belongs to (ADR-0154 §2):
// the conversation of declared, the IRI that a remote post declared (one of this server's
// conversations does not count), else the conversation of its stored parent, else a new
// conversation of this server. When the reply links show that st's conversation and its
// parent's, or st's and that of a stored reply to st, are one thread, the child's side is merged
// into the parent's (ADR-0154 §3), so that the thread ends up in one conversation whatever the
// order in which its statuses arrive. It reports whether st's parent was stored and the
// conversation that st ends in, after the merges.
//
// It takes the lock keys of st (statusLockKeys) before it inserts, then those of the
// conversations it reads, so that a parent and a reply stored at the same time see each other
// and a concurrent merge does not leave st in a merged conversation.
func (s *Service) insertStatus(ctx context.Context, tx uow.Tx, st content.Status, declared string) (insertedStatus, error) {
	repo := s.d.Repository
	if content.IsLocalConversationURI(s.d.Host, declared) {
		declared = ""
	}
	keys := statusLockKeys(st, false)
	if declared != "" {
		keys = append(keys, lockConversationURI+declared)
		slices.Sort(keys)
	}
	if err := repo.LockStatusKeys(ctx, tx, keys); err != nil {
		return insertedStatus{}, err
	}
	if err := repo.InsertStatus(ctx, tx, st); err != nil {
		return insertedStatus{}, err
	}
	var declaredConv *content.Conversation
	if declared != "" {
		c, err := repo.EnsureConversation(ctx, tx, content.Conversation{ID: s.d.IDs.NewID(), URI: declared})
		if err != nil {
			return insertedStatus{}, err
		}
		declaredConv = &c
	}
	var parent *content.Status
	if st.InReplyToURI != "" {
		p, err := repo.FindLiveStatusByURI(ctx, tx, st.InReplyToURI)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return insertedStatus{}, err
		case p.ID != st.ID: // a status that replies to itself is no parent of its own
			parent = &p
		}
	}
	replies, err := repo.ListLiveRepliesTo(ctx, tx, st.URI)
	if err != nil {
		return insertedStatus{}, err
	}
	related := make([]string, 0, len(replies)+1)
	if parent != nil {
		related = append(related, parent.ID)
	}
	for _, r := range replies {
		if r.ID != st.ID {
			related = append(related, r.ID)
		}
	}

	// Lock the conversations that st joins or merges, and read them again until none moved.
	locked := map[string]bool{}
	var of map[string]content.Conversation
	for round := 0; ; round++ {
		if of, err = repo.StatusConversationsTx(ctx, tx, related); err != nil {
			return insertedStatus{}, err
		}
		if declaredConv != nil {
			c, err := s.finalConversationTx(ctx, tx, declaredConv.ID)
			if err != nil {
				return insertedStatus{}, err
			}
			declaredConv = &c
		}
		var more []string
		for _, id := range related {
			if c, ok := of[id]; ok && !locked[c.ID] && !slices.Contains(more, c.ID) {
				more = append(more, c.ID)
			}
		}
		if declaredConv != nil && !locked[declaredConv.ID] && !slices.Contains(more, declaredConv.ID) {
			more = append(more, declaredConv.ID)
		}
		if len(more) == 0 {
			break
		}
		if round == maxConversationLockRounds {
			return insertedStatus{}, fmt.Errorf("status %s: %w", st.ID, errConversationsMoving)
		}
		slices.Sort(more)
		ks := make([]string, len(more))
		for i, id := range more {
			ks[i] = lockConversation + id
			locked[id] = true
		}
		if err := repo.LockStatusKeys(ctx, tx, ks); err != nil {
			return insertedStatus{}, err
		}
	}

	var parentConv *content.Conversation
	if parent != nil {
		if c, ok := of[parent.ID]; ok {
			parentConv = &c
		}
	}
	var conv content.Conversation
	switch {
	case declaredConv != nil:
		conv = *declaredConv
	case parentConv != nil:
		conv = *parentConv
	default:
		id := s.d.IDs.NewID()
		conv = content.Conversation{ID: id, URI: content.ConversationURI(s.d.Host, id), IsLocal: true}
		if conv, err = repo.EnsureConversation(ctx, tx, conv); err != nil {
			return insertedStatus{}, err
		}
	}
	if err := repo.InsertStatusConversation(ctx, tx, st.ID, conv.ID); err != nil {
		return insertedStatus{}, err
	}
	// The merges of this transaction, by merged conversation, for the replies that shared one.
	merged := map[string]string{}
	merge := func(from, into string) error {
		m := content.ConversationMerge{MergedConversationID: from, IntoConversationID: into, StatusID: st.ID, MergedAt: s.d.Clock.Now()}
		if err := repo.MergeConversation(ctx, tx, m); err != nil {
			return err
		}
		merged[from] = into
		return tx.Emit(ctx, ConversationMerged{Merge: m})
	}
	if parentConv != nil && parentConv.ID != conv.ID {
		if err := merge(conv.ID, parentConv.ID); err != nil {
			return insertedStatus{}, err
		}
		conv = *parentConv
	}
	for _, r := range replies {
		c, ok := of[r.ID]
		if r.ID == st.ID || !ok {
			continue
		}
		id := c.ID
		for merged[id] != "" {
			id = merged[id]
		}
		if id != conv.ID {
			if err := merge(id, conv.ID); err != nil {
				return insertedStatus{}, err
			}
		}
	}
	return insertedStatus{ParentStored: parent != nil, ConversationID: conv.ID}, nil
}

// insertedStatus is what insertStatus reports of the status it inserted.
type insertedStatus struct {
	// ParentStored is true when the status replies to a stored status.
	ParentStored bool
	// ConversationID is the conversation that the status belongs to once the merges are done.
	ConversationID string
}

// finalConversationTx returns, inside tx, the conversation that id names, or the one it ended in
// when it was merged (ADR-0154 §3).
func (s *Service) finalConversationTx(ctx context.Context, tx uow.Tx, id string) (content.Conversation, error) {
	repo := s.d.Repository
	for range content.MaxConversationMergeHops {
		into, err := repo.ConversationMergeTargetTx(ctx, tx, id)
		if errors.Is(err, ErrNotFound) {
			return repo.FindConversationTx(ctx, tx, id)
		}
		if err != nil {
			return content.Conversation{}, err
		}
		id = into
	}
	return content.Conversation{}, fmt.Errorf("conversation %s: %w", id, content.ErrConversationMergeChain)
}

// ResolvedConversation is a conversation as a lookup by ID finds it: the conversation the ID
// names, and the one it ended in, which is the same unless it was merged.
type ResolvedConversation struct {
	Named Conversation
	Final Conversation
}

// Merged reports whether the conversation that the ID names was merged into another.
func (r ResolvedConversation) Merged() bool { return r.Named.ID != r.Final.ID }

// ResolveConversation returns the conversation id and the one it ended in when it was merged
// (ADR-0154 §3, §6): a request that names a merged conversation is answered with the conversation
// it was merged into. An ID that no conversation has fails with ErrNotFound.
func (s *Service) ResolveConversation(ctx context.Context, id string) (ResolvedConversation, error) {
	named, err := s.d.Repository.FindConversation(ctx, id)
	if err != nil {
		return ResolvedConversation{}, err
	}
	out := ResolvedConversation{Named: named, Final: named}
	for range content.MaxConversationMergeHops {
		m, err := s.d.Repository.FindConversationMerge(ctx, out.Final.ID)
		if errors.Is(err, ErrNotFound) {
			return out, nil
		}
		if err != nil {
			return ResolvedConversation{}, err
		}
		if out.Final, err = s.d.Repository.FindConversation(ctx, m.IntoConversationID); err != nil {
			return ResolvedConversation{}, err
		}
	}
	return ResolvedConversation{}, fmt.Errorf("conversation %s: %w", id, content.ErrConversationMergeChain)
}

// ConversationsOf returns the conversation that each of statusIDs belongs to now, by status,
// with one query; a status that is not stored is left out. A merge moves the statuses of the
// merged conversation, so none of them is a merged conversation.
func (s *Service) ConversationsOf(ctx context.Context, statusIDs []string) (map[string]Conversation, error) {
	if len(statusIDs) == 0 {
		return map[string]Conversation{}, nil
	}
	return s.d.Repository.StatusConversations(ctx, statusIDs)
}

// FindConversationByURI returns the conversation with an IRI, as it was stored: a merged one is
// not followed to the one it was merged into.
func (s *Service) FindConversationByURI(ctx context.Context, uri string) (Conversation, error) {
	return s.d.Repository.FindConversationByURI(ctx, uri)
}

// FindConversationMerge returns the merge of a merged conversation; ErrNotFound means that it is
// not merged.
func (s *Service) FindConversationMerge(ctx context.Context, conversationID string) (content.ConversationMerge, error) {
	return s.d.Repository.FindConversationMerge(ctx, conversationID)
}

// ConversationPageSize is the number of statuses on one page of the collection of a conversation
// (ADR-0154 §9).
const ConversationPageSize = 40

// ConversationPage is one page of the statuses of a conversation that are not deleted, oldest
// first, before any decision about who may see them.
type ConversationPage struct {
	// Statuses are the statuses of the page; Revisions the current revision of each, by status.
	Statuses  []content.Status
	Revisions map[string]content.Revision
	// Next is the cursor of the next page, "" on the last page.
	Next string
}

// ConversationStatuses returns up to limit statuses of a conversation that are not deleted, in ID
// order after the cursor after ("" for the first page), with their current revisions, with a
// number of queries that does not grow with limit. The caller decides who may see each.
func (s *Service) ConversationStatuses(ctx context.Context, conversationID, after string, limit int) (ConversationPage, error) {
	sts, err := s.d.Repository.ListLiveConversationStatuses(ctx, conversationID, after, limit+1)
	if err != nil {
		return ConversationPage{}, err
	}
	var out ConversationPage
	if len(sts) > limit {
		sts = sts[:limit]
		out.Next = sts[limit-1].ID
	}
	ids := make([]string, len(sts))
	for i, st := range sts {
		ids[i] = st.ID
	}
	revs, err := s.d.Repository.FindCurrentRevisions(ctx, ids)
	if err != nil {
		return ConversationPage{}, err
	}
	// A status deleted between the two reads has no current revision; it is left out.
	for _, st := range sts {
		if _, ok := revs[st.ID]; ok {
			out.Statuses = append(out.Statuses, st)
		}
	}
	out.Revisions = revs
	return out, nil
}
