// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
)

// The direct messages by conversation (FR-212, data-model §6.4, ADR-0154 §6): the direct posts
// that a viewer sent or received, grouped by the conversation (content's status_conversations)
// they belong to, followed or not. The projectors keep two projections:
//   - feed_conversation_participants holds the parties of each direct post (its author and its
//     recipients) with its conversation;
//   - feed_conversations holds, for each party, each conversation and the party's newest direct
//     post there, which orders the list.
//
// The viewer's commands keep two current values, boundaries by entry ID: feed_conversation_reads
// (read up to the entry; a newer post by another account is unread) and feed_conversation_hides
// (hidden while no newer post arrives). content.conversation_merged moves the rows of the merged
// conversation to the one it was merged into, keeping the newer boundary of the two.

// ConversationParticipant is a party of a direct entry (feed_conversation_participants).
type ConversationParticipant struct {
	ConversationID string
	EntryID        string
	AccountID      string
}

// ConversationHead is the newest direct entry of an account in a conversation
// (feed_conversations).
type ConversationHead struct {
	AccountID      string
	ConversationID string
	LastEntryID    string
}

// ConversationRow is a conversation of a viewer's list with the viewer's boundaries.
type ConversationRow struct {
	ConversationID string
	// LastEntryID is the viewer's newest direct entry in the conversation, which orders the list.
	LastEntryID string
	// ReadEntryID is the viewer's read boundary, "" when the viewer never read it.
	ReadEntryID string
	// Hidden is true while the viewer's hide boundary is at or after LastEntryID.
	Hidden bool
}

// LastConversationEntries are the newest direct entries of a viewer in a conversation that the
// viewer's blocks, mutes, and the restrictions of their authors leave, as the home timeline
// leaves them.
type LastConversationEntries struct {
	ConversationID string
	LastEntryID    string
	// LastOtherEntryID is the newest of them by another account, "" for none.
	LastOtherEntryID string
}

// ConversationsQuery selects a page of a viewer's conversations, without the hidden ones, by
// LastEntryID newest first.
type ConversationsQuery struct {
	ViewerAccountID string
	// Before is the LastEntryID of the last row of the previous page; "" starts at the newest.
	Before string
	Limit  int
}

// ConversationRepository is the part of Repository that keeps the direct messages by
// conversation. Every method touches its rows with a number of statements that does not grow
// with the parties or the conversations it is given.
type ConversationRepository interface {
	// AddConversationEntry records accountIDs as the parties of the direct entry entryID in the
	// conversation conversationID, and makes it the last entry of the conversation for each of
	// them whose last one is older.
	AddConversationEntry(ctx context.Context, tx uow.Tx, conversationID, entryID string, accountIDs []string) error
	// ReplaceConversationEntryParties makes accountIDs the parties of the direct entry entryID,
	// in the conversation it is recorded in, and derives the last entries of that conversation
	// again. An entry with no parties recorded is left alone.
	ReplaceConversationEntryParties(ctx context.Context, tx uow.Tx, entryID string, accountIDs []string) error
	// DeleteConversationEntries takes the entries out of their conversations and derives the last
	// entries of those conversations again.
	DeleteConversationEntries(ctx context.Context, tx uow.Tx, entryIDs []string) error
	// MergeConversations moves the participants, last entries, read boundaries, and hide
	// boundaries of the conversation mergedID to intoID, keeping for each account the newer of
	// the two.
	MergeConversations(ctx context.Context, tx uow.Tx, mergedID, intoID string) error
	// MarkConversationRead sets the read boundary of accountID in conversationID to its last
	// entry there, keeping a newer one, and HideConversation does the same with the hide
	// boundary; both report false, writing nothing, when the account has no direct entry there.
	MarkConversationRead(ctx context.Context, tx uow.Tx, accountID, conversationID string) (bool, error)
	HideConversation(ctx context.Context, tx uow.Tx, accountID, conversationID string) (bool, error)

	// ListConversations returns up to q.Limit rows of a viewer's list.
	ListConversations(ctx context.Context, q ConversationsQuery) ([]ConversationRow, error)
	// ConversationsAmong returns the rows of a viewer among conversationIDs, hidden ones
	// included, by LastEntryID newest first.
	ConversationsAmong(ctx context.Context, viewerAccountID string, conversationIDs []string) ([]ConversationRow, error)
	// LastVisibleConversationEntries returns the last entries of a viewer in each of
	// conversationIDs that has one, by conversation, with one query.
	LastVisibleConversationEntries(ctx context.Context, viewerAccountID string, conversationIDs []string, now time.Time) (map[string]LastConversationEntries, error)
	// ConversationParticipants returns, by conversation, the other parties of the viewer's
	// direct entries in each of conversationIDs, ordered by ID, with one query.
	ConversationParticipants(ctx context.Context, viewerAccountID string, conversationIDs []string) (map[string][]string, error)
	// EntriesByID returns the entries among ids that exist, ordered by ID.
	EntriesByID(ctx context.Context, ids []string) ([]Entry, error)
}

// ConversationResolver finds the conversation that an ID names now: content's
// Service.ResolveConversation, which follows the merges (ADR-0154 §3).
type ConversationResolver interface {
	ResolveConversation(ctx context.Context, id string) (appcontent.ResolvedConversation, error)
}

// Conversation is a conversation of a viewer's list of direct messages.
type Conversation struct {
	// ID is the conversation's ID, that of the conversation it ended in when it was merged.
	ID string
	// LastEntry is the newest direct post of the conversation that the viewer may see.
	LastEntry Entry
	// AccountIDs are the other accounts of the direct posts the viewer sent or received there.
	AccountIDs []string
	// Unread is true when a post by another account that the viewer may see is newer than the
	// viewer's read boundary.
	Unread bool
}

// ConversationPage is one page of a viewer's list of direct messages.
type ConversationPage struct {
	// Conversations are newest first. A page may hold fewer than the limit even when more
	// follow, when a conversation has no post left that the viewer may see.
	Conversations []Conversation
	// Next is the Before of the next page, "" when this page is the last.
	Next string
}

// directParties returns the parties of a direct post: its author and the accounts it mentions.
func directParties(authorAccountID string, mentioned []string) []string {
	return uniq(append([]string{authorAccountID}, mentioned...))
}

// registerConversationProjectors registers the projectors of the direct messages by
// conversation. They run in the projectors of the statuses (RegisterProjectors), after
// feed_entries and feed_direct_recipients.
func registerConversationProjectors(r *uow.Projectors, repo Repository) {
	r.Register(appcontent.EventConversationMerged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		m := e.(appcontent.ConversationMerged).Merge
		return repo.MergeConversations(ctx, tx, m.MergedConversationID, m.IntoConversationID)
	})
}

// Conversations returns a page of the viewer's direct messages by conversation (FR-212), by
// their newest direct post, newest first, without those the viewer hid that no newer post
// brought back. Each conversation shows its newest post that the viewer may see, with the blocks,
// mutes, and restrictions of authors and domains of the home timeline (CanView in
// ContextDirectMessages),
// the previous one when that is hidden, and is left out when none is left. The page reads the
// conversations, their last entries, their participants, and the facts of their entries with a
// number of queries that does not grow with the page.
func (s *Service) Conversations(ctx context.Context, viewer appcontent.Viewer, q Query) (ConversationPage, error) {
	if !loggedIn(viewer) {
		return ConversationPage{}, ErrLoginRequired
	}
	limit := q.limit()
	rows, err := s.d.Repository.ListConversations(ctx, ConversationsQuery{ViewerAccountID: viewer.AccountID, Before: q.Before, Limit: limit + 1})
	if err != nil {
		return ConversationPage{}, err
	}
	var p ConversationPage
	if len(rows) > limit {
		rows = rows[:limit]
		p.Next = rows[limit-1].LastEntryID
	}
	if p.Conversations, err = s.presentConversations(ctx, viewer, rows); err != nil {
		return ConversationPage{}, err
	}
	return p, nil
}

// Conversation returns the conversation id of the viewer's list, hidden or not, or the one it
// ended in when it was merged (ADR-0154 §6). It fails with ErrNotFound when the conversation does
// not exist, the viewer has no direct post in it, or the viewer may see none of them.
func (s *Service) Conversation(ctx context.Context, viewer appcontent.Viewer, id string) (Conversation, error) {
	if !loggedIn(viewer) {
		return Conversation{}, ErrLoginRequired
	}
	final, err := s.resolveConversation(ctx, id)
	if err != nil {
		return Conversation{}, err
	}
	c, ok, err := s.conversationOf(ctx, viewer, final, true)
	if err != nil {
		return Conversation{}, err
	}
	if !ok {
		return Conversation{}, fmt.Errorf("conversation %s: %w", id, ErrNotFound)
	}
	return c, nil
}

// StreamConversation returns the conversation id as the viewer's list shows it, for the stream:
// false when the list does not show it (it is hidden, or the viewer may see none of its posts).
func (s *Service) StreamConversation(ctx context.Context, viewer appcontent.Viewer, id string) (Conversation, bool, error) {
	if !loggedIn(viewer) {
		return Conversation{}, false, ErrLoginRequired
	}
	final, err := s.resolveConversation(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Conversation{}, false, nil
	}
	if err != nil {
		return Conversation{}, false, err
	}
	return s.conversationOf(ctx, viewer, final, false)
}

// ReadConversation marks the conversation id read up to its newest direct post of the viewer,
// on every device of the viewer, and returns it as Conversation does. The streams of the viewer
// receive the conversation again.
func (s *Service) ReadConversation(ctx context.Context, viewer appcontent.Viewer, id string) (Conversation, error) {
	if err := s.conversationCommand(ctx, viewer, id, s.d.Repository.MarkConversationRead); err != nil {
		return Conversation{}, err
	}
	return s.Conversation(ctx, viewer, id)
}

// HideConversation takes the conversation id out of the viewer's list until a newer direct post
// arrives in it. Hiding it again changes nothing.
func (s *Service) HideConversation(ctx context.Context, viewer appcontent.Viewer, id string) error {
	return s.conversationCommand(ctx, viewer, id, s.d.Repository.HideConversation)
}

// conversationCommand moves a boundary of the viewer in the conversation id (or the one it ended
// in) with set, and signals the viewer's streams once it committed.
func (s *Service) conversationCommand(ctx context.Context, viewer appcontent.Viewer, id string,
	set func(ctx context.Context, tx uow.Tx, accountID, conversationID string) (bool, error),
) error {
	if !loggedIn(viewer) {
		return ErrLoginRequired
	}
	final, err := s.resolveConversation(ctx, id)
	if err != nil {
		return err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := set(ctx, tx, viewer.AccountID, final)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("conversation %s: %w", id, ErrNotFound)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if s.d.Stream != nil {
		s.d.Stream.Publish(stream.Event{Kind: stream.KindConversation, AccountID: viewer.AccountID, ConversationID: final})
	}
	return nil
}

// resolveConversation returns the ID of the conversation that id names now (ADR-0154 §3);
// ErrNotFound when none has the ID.
func (s *Service) resolveConversation(ctx context.Context, id string) (string, error) {
	if s.d.Conversations == nil {
		return id, nil
	}
	r, err := s.d.Conversations.ResolveConversation(ctx, id)
	if errors.Is(err, appcontent.ErrNotFound) {
		return "", fmt.Errorf("conversation %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", err
	}
	return r.Final.ID, nil
}

// conversationOf returns the viewer's conversation id as presentConversations shows it, the
// hidden one too when withHidden.
func (s *Service) conversationOf(ctx context.Context, viewer appcontent.Viewer, id string, withHidden bool) (Conversation, bool, error) {
	rows, err := s.d.Repository.ConversationsAmong(ctx, viewer.AccountID, []string{id})
	if err != nil {
		return Conversation{}, false, err
	}
	rows = slices.DeleteFunc(rows, func(r ConversationRow) bool { return r.Hidden && !withHidden })
	cs, err := s.presentConversations(ctx, viewer, rows)
	if err != nil || len(cs) == 0 {
		return Conversation{}, false, err
	}
	return cs[0], true, nil
}

// presentConversations returns the conversations of rows, in order, that keep a post the viewer
// may see: their last visible entries, the facts of those entries, and the participants, each
// read once for all the rows.
func (s *Service) presentConversations(ctx context.Context, viewer appcontent.Viewer, rows []ConversationRow) ([]Conversation, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ConversationID
	}
	now := s.d.Clock.Now()
	repo := s.d.Repository
	last, err := repo.LastVisibleConversationEntries(ctx, viewer.AccountID, ids, now)
	if err != nil {
		return nil, err
	}
	entryIDs := make([]string, 0, len(last))
	for _, l := range last {
		entryIDs = append(entryIDs, l.LastEntryID)
	}
	if len(entryIDs) == 0 {
		return nil, nil
	}
	slices.Sort(entryIDs)
	entries, err := repo.EntriesByID(ctx, entryIDs)
	if err != nil {
		return nil, err
	}
	facts, err := s.facts(ctx, viewer.AccountID, now, entries)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Entry, len(entries))
	for _, e := range entries {
		if facts.canView(appcontent.ContextDirectMessages, viewer, e) {
			byID[e.ID] = e
		}
	}
	accounts, err := repo.ConversationParticipants(ctx, viewer.AccountID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(rows))
	for _, r := range rows {
		l, ok := last[r.ConversationID]
		if !ok {
			continue
		}
		e, ok := byID[l.LastEntryID]
		if !ok {
			continue // CanView denies what the query let through: show nothing of it
		}
		out = append(out, Conversation{
			ID: r.ConversationID, LastEntry: e, AccountIDs: accounts[r.ConversationID],
			Unread: l.LastOtherEntryID != "" && l.LastOtherEntryID > r.ReadEntryID,
		})
	}
	return out, nil
}
