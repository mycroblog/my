// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// replayFlushRows is how many entries a replayBatch holds before it writes them.
const replayFlushRows = 1000

// replayBatch is the Repository through which RebuildProjections applies the projectors to the
// replayed events (ADR-0172 §4): it holds the rows that the replay of statuses, boosts, follows,
// and exclusions writes, which go to tables that the rebuild emptied, and writes them a few
// statements at a time instead of a round trip each. It holds an entry's tags and direct
// recipients as new, since a replay derives each entry once. Every other method of the
// projectors writes what it holds first, then calls the repository.
type replayBatch struct {
	Repository
	tx uow.Tx

	entries    []Entry
	known      map[string]Entry
	recipients []DirectRecipient
	tags       []TagEntry
	usage      map[string]int64
	follows    map[[2]string]FollowEdge
	exclusions map[Exclusion]bool
}

func newReplayBatch(repo Repository) *replayBatch {
	return &replayBatch{
		Repository: repo, known: map[string]Entry{}, usage: map[string]int64{},
		follows: map[[2]string]FollowEdge{}, exclusions: map[Exclusion]bool{},
	}
}

// hold remembers the transaction of the replay, which writes what is held.
func (b *replayBatch) hold(ctx context.Context, tx uow.Tx) error {
	if b.tx != nil && b.tx != tx {
		if err := b.flush(ctx); err != nil {
			return err
		}
	}
	b.tx = tx
	return nil
}

// flush writes what b holds.
func (b *replayBatch) flush(ctx context.Context) error {
	if b.tx == nil {
		return nil
	}
	if len(b.entries) > 0 || len(b.recipients) > 0 || len(b.tags) > 0 || len(b.usage) > 0 {
		if err := b.InsertReplayed(ctx, b.tx, b.entries, b.recipients, b.tags, b.usage); err != nil {
			return err
		}
		b.entries, b.recipients, b.tags, b.usage = nil, nil, nil, map[string]int64{}
	}
	if len(b.follows) > 0 {
		es := make([]FollowEdge, 0, len(b.follows))
		for _, e := range b.follows {
			es = append(es, e)
		}
		if err := b.Repository.UpsertFollowEdges(ctx, b.tx, es); err != nil {
			return err
		}
		b.follows = map[[2]string]FollowEdge{}
	}
	if len(b.exclusions) > 0 {
		xs := make([]Exclusion, 0, len(b.exclusions))
		for x := range b.exclusions {
			xs = append(xs, x)
		}
		if err := b.Repository.UpsertExclusions(ctx, b.tx, xs); err != nil {
			return err
		}
		b.exclusions = map[Exclusion]bool{}
	}
	return nil
}

// before writes what b holds before another method reads or writes the feed tables.
func (b *replayBatch) before(ctx context.Context, tx uow.Tx) error {
	if err := b.hold(ctx, tx); err != nil {
		return err
	}
	return b.flush(ctx)
}

func (b *replayBatch) InsertEntry(ctx context.Context, tx uow.Tx, e Entry) (bool, error) {
	if err := b.hold(ctx, tx); err != nil {
		return false, err
	}
	if _, ok := b.known[e.ID]; ok {
		return false, nil
	}
	b.known[e.ID] = e
	b.entries = append(b.entries, e)
	if len(b.entries) >= replayFlushRows {
		return true, b.flush(ctx)
	}
	return true, nil
}

func (b *replayBatch) Entry(ctx context.Context, tx uow.Tx, id string) (Entry, error) {
	if e, ok := b.known[id]; ok {
		return e, nil
	}
	if err := b.before(ctx, tx); err != nil {
		return Entry{}, err
	}
	return b.Repository.Entry(ctx, tx, id)
}

func (b *replayBatch) ReplaceDirectRecipients(ctx context.Context, tx uow.Tx, entryID string, accountIDs []string) error {
	if err := b.hold(ctx, tx); err != nil {
		return err
	}
	for _, id := range accountIDs {
		b.recipients = append(b.recipients, DirectRecipient{EntryID: entryID, RecipientAccountID: id})
	}
	return nil
}

func (b *replayBatch) ReplaceTags(ctx context.Context, tx uow.Tx, e Entry, tagIDs []string, revisedAt time.Time) error {
	if err := b.hold(ctx, tx); err != nil {
		return err
	}
	counts := CountsTagUsage(e.Visibility)
	seen := map[string]bool{}
	for _, id := range tagIDs {
		if seen[id] { // a tag listed twice is one entry and counts once
			continue
		}
		seen[id] = true
		b.tags = append(b.tags, TagEntry{TagID: id, EntryID: e.ID, AuthorAccountID: e.AuthorAccountID, Visibility: e.Visibility, IsLocal: e.IsLocal, RevisedAt: revisedAt})
		if counts {
			b.usage[id]++
		}
	}
	return nil
}

func (b *replayBatch) UpsertFollowEdge(ctx context.Context, tx uow.Tx, e FollowEdge) error {
	if err := b.hold(ctx, tx); err != nil {
		return err
	}
	b.follows[[2]string{e.FollowerAccountID, e.FolloweeAccountID}] = e
	return nil
}

func (b *replayBatch) UpsertExclusion(ctx context.Context, tx uow.Tx, x Exclusion) error {
	if err := b.hold(ctx, tx); err != nil {
		return err
	}
	b.exclusions[x] = true
	return nil
}

// The other methods of the projectors write what b holds first.

func (b *replayBatch) UpsertFollowEdges(ctx context.Context, tx uow.Tx, es []FollowEdge) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.UpsertFollowEdges(ctx, tx, es)
}

func (b *replayBatch) UpsertExclusions(ctx context.Context, tx uow.Tx, xs []Exclusion) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.UpsertExclusions(ctx, tx, xs)
}

func (b *replayBatch) UpdateRevision(ctx context.Context, tx uow.Tx, statusID, language string, seq int) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.UpdateRevision(ctx, tx, statusID, language, seq)
}

func (b *replayBatch) DeleteEntry(ctx context.Context, tx uow.Tx, id string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	delete(b.known, id)
	return b.Repository.DeleteEntry(ctx, tx, id)
}

func (b *replayBatch) DeleteStatusEntries(ctx context.Context, tx uow.Tx, statusID string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	b.forgetStatuses(statusID)
	return b.Repository.DeleteStatusEntries(ctx, tx, statusID)
}

func (b *replayBatch) SetReplyTargets(ctx context.Context, tx uow.Tx, parentStatusID, parentAccountID string, replyStatusIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	for _, id := range replyStatusIDs {
		if e, ok := b.known[id]; ok && e.Kind == timeline.KindPost {
			e.InReplyToStatusID, e.InReplyToAccountID = parentStatusID, parentAccountID
			b.known[id] = e
		}
	}
	return b.Repository.SetReplyTargets(ctx, tx, parentStatusID, parentAccountID, replyStatusIDs)
}

func (b *replayBatch) ClearReplyTargetsTo(ctx context.Context, tx uow.Tx, parentStatusIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	gone := map[string]bool{}
	for _, id := range parentStatusIDs {
		gone[id] = true
	}
	for id, e := range b.known {
		if gone[e.InReplyToStatusID] {
			e.InReplyToStatusID, e.InReplyToAccountID = "", ""
			b.known[id] = e
		}
	}
	return b.Repository.ClearReplyTargetsTo(ctx, tx, parentStatusIDs)
}

func (b *replayBatch) DeleteStatusEntriesOf(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	b.forgetStatuses(statusIDs...)
	return b.Repository.DeleteStatusEntriesOf(ctx, tx, statusIDs)
}

// forgetStatuses drops the known entries of the statuses, which a deletion took away.
func (b *replayBatch) forgetStatuses(statusIDs ...string) {
	gone := map[string]bool{}
	for _, id := range statusIDs {
		gone[id] = true
	}
	for id, e := range b.known {
		if gone[e.StatusID] {
			delete(b.known, id)
		}
	}
}

func (b *replayBatch) DeleteFollowEdge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteFollowEdge(ctx, tx, followerAccountID, followeeAccountID)
}

func (b *replayBatch) DeleteFollowEdges(ctx context.Context, tx uow.Tx, followerIDs, followeeIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteFollowEdges(ctx, tx, followerIDs, followeeIDs)
}

func (b *replayBatch) DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason timeline.ExclusionReason) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteExclusion(ctx, tx, viewerAccountID, targetAccountID, reason)
}

func (b *replayBatch) InsertDomainMute(ctx context.Context, tx uow.Tx, m DomainMute) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.InsertDomainMute(ctx, tx, m)
}

func (b *replayBatch) DeleteDomainMute(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteDomainMute(ctx, tx, viewerAccountID, domain)
}

func (b *replayBatch) DeleteListMembersOfFollow(ctx context.Context, tx uow.Tx, ownerAccountID, memberAccountID string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteListMembersOfFollow(ctx, tx, ownerAccountID, memberAccountID)
}

func (b *replayBatch) DeleteListMembersOfFollows(ctx context.Context, tx uow.Tx, followerIDs, followeeIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteListMembersOfFollows(ctx, tx, followerIDs, followeeIDs)
}

func (b *replayBatch) DeleteListMembersWithoutFollow(ctx context.Context, tx uow.Tx) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteListMembersWithoutFollow(ctx, tx)
}

func (b *replayBatch) AddConversationEntry(ctx context.Context, tx uow.Tx, conversationID, entryID string, accountIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.AddConversationEntry(ctx, tx, conversationID, entryID, accountIDs)
}

func (b *replayBatch) DeleteConversationEntries(ctx context.Context, tx uow.Tx, entryIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteConversationEntries(ctx, tx, entryIDs)
}

func (b *replayBatch) MergeConversations(ctx context.Context, tx uow.Tx, mergedID, intoID string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.MergeConversations(ctx, tx, mergedID, intoID)
}

func (b *replayBatch) ReplaceConversationEntryParties(ctx context.Context, tx uow.Tx, entryID string, accountIDs []string) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.ReplaceConversationEntryParties(ctx, tx, entryID, accountIDs)
}

func (b *replayBatch) UpsertAuthorRestriction(ctx context.Context, tx uow.Tx, r AuthorRestriction) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.UpsertAuthorRestriction(ctx, tx, r)
}

func (b *replayBatch) DeleteAuthorRestriction(ctx context.Context, tx uow.Tx, accountID string, r timeline.Restriction) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteAuthorRestriction(ctx, tx, accountID, r)
}

func (b *replayBatch) UpsertDomainRestriction(ctx context.Context, tx uow.Tx, r DomainRestriction) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.UpsertDomainRestriction(ctx, tx, r)
}

func (b *replayBatch) DeleteDomainRestriction(ctx context.Context, tx uow.Tx, domain string, r timeline.Restriction) error {
	if err := b.before(ctx, tx); err != nil {
		return err
	}
	return b.Repository.DeleteDomainRestriction(ctx, tx, domain, r)
}
