// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds what a page of statuses reads once for all of them, so that presenting a page
// makes a number of round trips to the database that does not grow with each status. The page's
// Reader decides and presents each status as View does, reading through what was read for the
// page first.

// page returns a Reader for the statuses of ids that viewer sees. It reads the statuses, then the
// statuses they reply to and quote, and then, for all of them together: their deletions, current
// revisions, counters, hashtags, reaction counts, quotes and interaction policies, the accounts of their
// authors and of the accounts they mention, and, for a signed-in viewer, the viewer's
// relationships with the authors, the statuses that mention the viewer, and what the viewer did
// with them. Anything else is read once, the first time a status needs it.
func (r *Reader) page(ctx context.Context, viewer Viewer, ids []string) (*Reader, error) {
	statuses, err := r.d.Statuses.FindStatuses(ctx, ids)
	if err != nil {
		return nil, err
	}
	page := &pageStatuses{Repository: r.d.Statuses, statuses: statuses, byURI: map[string]string{}}
	quotes := &pageQuotes{QuoteRepository: r.d.Quotes}
	var quoting []string
	for _, id := range ids {
		if _, ok := statuses[id]; ok && !slices.Contains(quoting, id) {
			quoting = append(quoting, id)
		}
	}
	if r.d.Quotes != nil {
		quotes.quoting = quoting
		if quotes.quotes, err = r.d.Quotes.FindQuotes(ctx, quoting); err != nil {
			return nil, err
		}
		var quoted []string
		for id := range quotes.quotes {
			quoted = append(quoted, id)
		}
		if quotes.states, err = r.d.Quotes.FindQuoteStates(ctx, quoted); err != nil {
			return nil, err
		}
	}
	// The statuses replied to and quoted, which the page presents or decides about too.
	var uris []string
	for _, id := range quoting {
		if u := statuses[id].InReplyToURI; u != "" {
			uris = append(uris, u)
		}
		if q, ok := quotes.quotes[id]; ok {
			uris = append(uris, q.QuotedURI)
		}
	}
	referred, err := r.d.Statuses.FindStatusesByURIs(ctx, uris)
	if err != nil {
		return nil, err
	}
	for _, u := range uris {
		page.byURI[u] = "" // asked for: missing unless found
	}
	page.ids = slices.Clone(ids)
	for u, st := range referred {
		page.byURI[u] = st.ID
		statuses[st.ID] = st
		page.ids = append(page.ids, st.ID)
	}
	live := make([]string, 0, len(statuses))
	for _, id := range page.ids {
		if _, ok := statuses[id]; ok && !slices.Contains(live, id) {
			live = append(live, id)
		}
	}
	if page.deletions, err = r.d.Statuses.FindDeletions(ctx, live); err != nil {
		return nil, err
	}
	var localReplies []string
	for _, id := range live {
		if st := statuses[id]; st.IsLocal && st.InReplyToURI != "" {
			localReplies = append(localReplies, id)
		}
	}
	page.heldAmong = localReplies
	if page.held, err = r.d.Statuses.HeldRepliesAmong(ctx, nil, localReplies); err != nil {
		return nil, err
	}
	if page.revisions, err = r.d.Statuses.FindCurrentRevisions(ctx, live); err != nil {
		return nil, err
	}
	if page.counters, err = r.d.Statuses.FindCountersOf(ctx, live); err != nil {
		return nil, err
	}
	if page.sensitive, err = r.d.Statuses.SensitivitiesOverridden(ctx, live); err != nil {
		return nil, err
	}
	for _, rev := range page.revisions {
		if rev.Poll != nil {
			page.polls = append(page.polls, rev.Poll.ID)
		}
	}
	if page.tallies, err = r.d.Statuses.FindPollTalliesOf(ctx, page.polls); err != nil {
		return nil, err
	}
	if viewer.Kind == ViewerLocal && viewer.AccountID != "" {
		page.voter = viewer.AccountID
		if page.votes, err = r.d.Statuses.ListPollVotesIn(ctx, page.polls, viewer.AccountID); err != nil {
			return nil, err
		}
	}
	if r.d.Quotes != nil {
		quotes.page = live
		if quotes.policies, err = r.d.Quotes.FindInteractionPolicies(ctx, live); err != nil {
			return nil, err
		}
		if viewer.Kind == ViewerLocal && viewer.AccountID != "" {
			quotes.viewer = viewer.AccountID
			for _, id := range live {
				quotes.uris = append(quotes.uris, statuses[id].URI)
			}
			if quotes.own, err = r.d.Quotes.ListOwnInteractions(ctx, viewer.AccountID, quotes.uris); err != nil {
				return nil, err
			}
		}
	}
	var accountIDs, authors, tagIDs []string
	for _, id := range live {
		authors = append(authors, statuses[id].AuthorAccountID)
	}
	accountIDs = slices.Clone(authors)
	for _, rev := range page.revisions {
		accountIDs = append(accountIDs, rev.MentionedAccountIDs...)
		tagIDs = append(tagIDs, rev.TagIDs...)
	}
	if page.tags, err = r.d.Statuses.FindTags(ctx, tagIDs); err != nil {
		return nil, err
	}
	accounts := &pageAccounts{Accounts: r.d.Accounts, actors: map[string]*appaccount.Actor{}}
	if err := accounts.load(ctx, accountIDs); err != nil {
		return nil, err
	}
	restrictions := &pageRestrictions{RestrictionPolicy: r.d.Restrictions, of: map[string]Restrictions{}}
	if err := restrictions.load(ctx, authors); err != nil {
		return nil, err
	}
	relationships := &pageRelationships{Relationships: r.d.Relationships, of: map[[2]string]appgraph.Relationship{}}
	interactions := &pageInteractions{InteractionRepository: r.d.Interactions, page: live}
	if interactions.counts, err = r.d.Interactions.ListReactionCountsOf(ctx, live, content.MaxStatusReactions); err != nil {
		return nil, err
	}
	if viewer.Kind == ViewerLocal && viewer.AccountID != "" {
		if err := relationships.load(ctx, viewer.AccountID, authors); err != nil {
			return nil, err
		}
		interactions.viewer = viewer.AccountID
		if interactions.mentioned, err = r.d.Interactions.MentionedIn(ctx, live, viewer.AccountID); err != nil {
			return nil, err
		}
		if interactions.own, err = r.d.Interactions.FindAccountInteractions(ctx, viewer.AccountID, live); err != nil {
			return nil, err
		}
	}
	p := *r
	p.d.Statuses, p.d.Accounts, p.d.Relationships, p.d.Interactions = page, accounts, relationships, interactions
	p.aud.statuses, p.aud.accounts, p.aud.relationships, p.aud.interactions = page, accounts, relationships, interactions
	p.d.Restrictions, p.aud.restrictions = restrictions, restrictions
	if r.d.LinkCards != nil {
		if p.d.LinkCards, err = r.d.LinkCards.page(ctx, page.revisions, accounts); err != nil {
			return nil, err
		}
	}
	// The custom emoji of the reactions, which each status's reactions show.
	var emojiIDs []string
	for _, counts := range interactions.counts {
		for _, c := range counts {
			emojiIDs = append(emojiIDs, c.Emoji.CustomEmojiID)
		}
	}
	p.reactionEmoji = map[string]*ReactionCustomEmoji{}
	if err := (emojiCache{r: &p, seen: p.reactionEmoji}).load(ctx, emojiIDs); err != nil {
		return nil, err
	}
	if r.d.Media != nil {
		var mediaIDs []string
		for _, rev := range page.revisions {
			for _, at := range rev.Attachments {
				mediaIDs = append(mediaIDs, at.MediaID)
			}
		}
		if p.d.Media, err = r.d.Media.page(ctx, mediaIDs); err != nil {
			return nil, err
		}
	}
	if r.d.Quotes != nil {
		p.d.Quotes = quotes
	}
	return &p, nil
}

// pageStatuses is the status repository with the parts of the statuses of a page read
// beforehand. A part that a status of the page does not have fails with ErrNotFound, as its read
// does.
type pageStatuses struct {
	Repository
	// ids are the IDs of the page (with the statuses its statuses reply to and quote), statuses
	// those that exist, and the others the parts of them. byURI are the statuses asked for by URI,
	// "" for one that does not exist.
	ids       []string
	byURI     map[string]string
	statuses  map[string]content.Status
	deletions map[string]content.Deletion
	// held are the held replies among the local replies heldAmong of the page.
	held      map[string]bool
	heldAmong []string
	revisions map[string]content.Revision
	counters  map[string]content.Counters
	tags      map[string]content.Tag
	sensitive map[string]bool
	// polls are the polls of the page's revisions, with their tallies, and the votes of voter,
	// the page's viewer ("" for a visitor).
	polls   []string
	tallies map[string][]content.PollTally
	voter   string
	votes   map[string][]content.PollVote
}

// SensitivityOverridden tells whether moderation marked a status of the page sensitive as it was
// read, and reads of any other.
func (p *pageStatuses) SensitivityOverridden(ctx context.Context, statusID string) (bool, error) {
	if slices.Contains(p.ids, statusID) {
		return p.sensitive[statusID], nil
	}
	return p.Repository.SensitivityOverridden(ctx, statusID)
}

// FindPollTallies returns the tallies of a poll of the page as they were read, and reads any
// other's.
func (p *pageStatuses) FindPollTallies(ctx context.Context, pollID string) ([]content.PollTally, error) {
	if slices.Contains(p.polls, pollID) {
		return p.tallies[pollID], nil
	}
	return p.Repository.FindPollTallies(ctx, pollID)
}

// ListPollVotes returns the viewer's votes in a poll of the page as they were read, and reads any
// other's.
func (p *pageStatuses) ListPollVotes(ctx context.Context, pollID, accountID string) ([]content.PollVote, error) {
	if accountID == p.voter && p.voter != "" && slices.Contains(p.polls, pollID) {
		return p.votes[pollID], nil
	}
	return p.Repository.ListPollVotes(ctx, pollID, accountID)
}

// pagePart returns the part of statusID from m when statusID is a status of the page: found, or
// ErrNotFound; ok is false for a status outside the page.
func pagePart[T any](p *pageStatuses, m map[string]T, statusID, what string) (part T, ok bool, err error) {
	if !slices.Contains(p.ids, statusID) {
		return part, false, nil
	}
	if v, found := m[statusID]; found {
		return v, true, nil
	}
	return part, true, fmt.Errorf("%s of %s: %w", what, statusID, ErrNotFound)
}

// FindStatus returns a status of the page as it was read for the page, and reads any other.
func (p *pageStatuses) FindStatus(ctx context.Context, id string) (content.Status, error) {
	if st, ok, err := pagePart(p, p.statuses, id, "status"); ok {
		return st, err
	}
	return p.Repository.FindStatus(ctx, id)
}

// FindStatusByURI returns a status that the page asked for by URI as it was read, and reads any
// other.
func (p *pageStatuses) FindStatusByURI(ctx context.Context, uri string) (content.Status, error) {
	if id, ok := p.byURI[uri]; ok {
		if st, found := p.statuses[id]; found {
			return st, nil
		}
		return content.Status{}, fmt.Errorf("status %s: %w", uri, ErrNotFound)
	}
	return p.Repository.FindStatusByURI(ctx, uri)
}

// HeldRepliesAmong answers the held replies among the local replies of the page from what the page
// read, and reads the others.
func (p *pageStatuses) HeldRepliesAmong(ctx context.Context, tx uow.Tx, ids []string) (map[string]bool, error) {
	if tx != nil || slices.ContainsFunc(ids, func(id string) bool { return !slices.Contains(p.heldAmong, id) && p.statuses[id].IsLocal }) {
		return p.Repository.HeldRepliesAmong(ctx, tx, ids)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if p.held[id] {
			out[id] = true
		}
	}
	return out, nil
}

// FindDeletion returns the deletion of a status of the page as it was read, and reads any other.
func (p *pageStatuses) FindDeletion(ctx context.Context, statusID string) (content.Deletion, error) {
	if d, ok, err := pagePart(p, p.deletions, statusID, "deletion"); ok {
		return d, err
	}
	return p.Repository.FindDeletion(ctx, statusID)
}

// FindCurrentRevision returns the current revision of a status of the page as it was read, and
// reads any other.
func (p *pageStatuses) FindCurrentRevision(ctx context.Context, statusID string) (content.Revision, error) {
	if rev, ok, err := pagePart(p, p.revisions, statusID, "head"); ok {
		return rev, err
	}
	return p.Repository.FindCurrentRevision(ctx, statusID)
}

// FindCounters returns the counters of a status of the page as they were read, and reads any
// other's.
func (p *pageStatuses) FindCounters(ctx context.Context, statusID string) (content.Counters, error) {
	if c, ok, err := pagePart(p, p.counters, statusID, "counters"); ok {
		return c, err
	}
	return p.Repository.FindCounters(ctx, statusID)
}

// FindTags returns the hashtags of the page's statuses as they were read, and reads the others
// together.
func (p *pageStatuses) FindTags(ctx context.Context, ids []string) (map[string]content.Tag, error) {
	out := make(map[string]content.Tag, len(ids))
	var missing []string
	for _, id := range ids {
		if tag, ok := p.tags[id]; ok {
			out[id] = tag
		} else {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	read, err := p.Repository.FindTags(ctx, missing)
	if err != nil {
		return nil, err
	}
	maps.Copy(out, read)
	return out, nil
}

// FindTag returns a hashtag of the page's statuses as it was read, and reads any other.
func (p *pageStatuses) FindTag(ctx context.Context, id string) (content.Tag, error) {
	if tag, ok := p.tags[id]; ok {
		return tag, nil
	}
	return p.Repository.FindTag(ctx, id)
}

// pageInteractions are the interactions with the statuses of a page read beforehand: their
// reaction counts, and, for a signed-in viewer, which of them mention the viewer and what the
// viewer did with them.
type pageInteractions struct {
	InteractionRepository
	viewer    string
	page      []string
	mentioned map[string]bool
	counts    map[string][]content.ReactionCount
	own       AccountInteractions
}

// viewers tells whether the read of accountID's interaction with statusID was made for the page.
func (p *pageInteractions) viewers(accountID, statusID string) bool {
	return p.viewer != "" && accountID == p.viewer && slices.Contains(p.page, statusID)
}

// Mentioned tells whether a status of the page mentions its viewer as it was read, and reads any
// other.
func (p *pageInteractions) Mentioned(ctx context.Context, statusID, accountID string) (bool, error) {
	if p.viewers(accountID, statusID) {
		return p.mentioned[statusID], nil
	}
	return p.InteractionRepository.Mentioned(ctx, statusID, accountID)
}

// ownPart returns the viewer's interaction with a status of the page from m, failing with
// ErrNotFound as its read does when there is none; ok is false for a read the page did not make.
func ownPart[T any](p *pageInteractions, m map[string]T, accountID, statusID, what string) (part T, ok bool, err error) {
	if !p.viewers(accountID, statusID) {
		return part, false, nil
	}
	if v, found := m[statusID]; found {
		return v, true, nil
	}
	return part, true, fmt.Errorf("%s of %s on %s: %w", what, accountID, statusID, ErrNotFound)
}

func (p *pageInteractions) FindFavourite(ctx context.Context, accountID, statusID string) (content.Favourite, error) {
	if f, ok, err := ownPart(p, p.own.Favourites, accountID, statusID, "favourite"); ok {
		return f, err
	}
	return p.InteractionRepository.FindFavourite(ctx, accountID, statusID)
}

func (p *pageInteractions) FindBoostEdge(ctx context.Context, accountID, statusID string) (content.BoostEdge, error) {
	if e, ok, err := ownPart(p, p.own.BoostEdges, accountID, statusID, "boost edge"); ok {
		return e, err
	}
	return p.InteractionRepository.FindBoostEdge(ctx, accountID, statusID)
}

func (p *pageInteractions) FindBookmark(ctx context.Context, accountID, statusID string) (content.Bookmark, error) {
	if b, ok, err := ownPart(p, p.own.Bookmarks, accountID, statusID, "bookmark"); ok {
		return b, err
	}
	return p.InteractionRepository.FindBookmark(ctx, accountID, statusID)
}

func (p *pageInteractions) FindPin(ctx context.Context, accountID, statusID string) (content.Pin, error) {
	if pin, ok, err := ownPart(p, p.own.Pins, accountID, statusID, "pin"); ok {
		return pin, err
	}
	return p.InteractionRepository.FindPin(ctx, accountID, statusID)
}

func (p *pageInteractions) ListAccountStatusReactions(ctx context.Context, accountID, statusID string) ([]content.Reaction, error) {
	if p.viewers(accountID, statusID) {
		return p.own.Reactions[statusID], nil
	}
	return p.InteractionRepository.ListAccountStatusReactions(ctx, accountID, statusID)
}

// ListReactionCounts returns the reaction counts of a status of the page as they were read, and
// reads any other's, or any with another limit.
func (p *pageInteractions) ListReactionCounts(ctx context.Context, statusID string, limit int) ([]content.ReactionCount, error) {
	if limit == content.MaxStatusReactions && slices.Contains(p.page, statusID) {
		return p.counts[statusID], nil
	}
	return p.InteractionRepository.ListReactionCounts(ctx, statusID, limit)
}

// pageAccounts are the accounts with those of a page read beforehand, and every other one
// remembered once read, for one page. A nil entry is an account that does not exist.
type pageAccounts struct {
	Accounts
	mu     sync.Mutex
	actors map[string]*appaccount.Actor
}

// load reads the accounts of ids that are not known yet, together.
func (p *pageAccounts) load(ctx context.Context, ids []string) error {
	p.mu.Lock()
	var unknown []string
	for _, id := range ids {
		if _, ok := p.actors[id]; !ok && id != "" {
			unknown = append(unknown, id)
			p.actors[id] = nil // listed once, even when ids repeats it
		}
	}
	p.mu.Unlock()
	if len(unknown) == 0 {
		return nil
	}
	found, err := p.Accounts.FindByIDs(ctx, unknown)
	if err != nil {
		p.mu.Lock()
		for _, id := range unknown {
			delete(p.actors, id)
		}
		p.mu.Unlock()
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, a := range found {
		p.actors[id] = &a
	}
	return nil
}

// FindByID returns the account of id from the page, reading it once when the page has not read it.
func (p *pageAccounts) FindByID(ctx context.Context, id string) (appaccount.Actor, error) {
	p.mu.Lock()
	a, ok := p.actors[id]
	p.mu.Unlock()
	if !ok {
		if err := p.load(ctx, []string{id}); err != nil {
			return appaccount.Actor{}, err
		}
		p.mu.Lock()
		a = p.actors[id]
		p.mu.Unlock()
	}
	if a == nil {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, appaccount.ErrNotFound)
	}
	return *a, nil
}

// FindByIDs returns the accounts of ids from the page, reading those it has not read together.
func (p *pageAccounts) FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error) {
	if err := p.load(ctx, ids); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]appaccount.Actor, len(ids))
	for _, id := range ids {
		if a := p.actors[id]; a != nil {
			out[id] = *a
		}
	}
	return out, nil
}

// relationshipBatch is a Relationships that reads the relationships of one account with many
// others at once, as *appgraph.Service does.
type relationshipBatch interface {
	Relationships(ctx context.Context, accountID string, targetIDs []string) (map[string]appgraph.Relationship, error)
}

// pageRelationships are the relationships with those of a page's viewer read beforehand, and every
// other one remembered once read, for one page.
type pageRelationships struct {
	Relationships
	mu sync.Mutex
	of map[[2]string]appgraph.Relationship
}

// load reads the relationships of accountID with targetIDs that are not known yet, together when
// the Relationships can, one by one otherwise.
func (p *pageRelationships) load(ctx context.Context, accountID string, targetIDs []string) error {
	p.mu.Lock()
	var unknown []string
	for _, id := range targetIDs {
		if _, ok := p.of[[2]string{accountID, id}]; !ok && id != "" && !slices.Contains(unknown, id) {
			unknown = append(unknown, id)
		}
	}
	p.mu.Unlock()
	if len(unknown) == 0 {
		return nil
	}
	found := map[string]appgraph.Relationship{}
	if b, ok := p.Relationships.(relationshipBatch); ok {
		var err error
		if found, err = b.Relationships(ctx, accountID, unknown); err != nil {
			return err
		}
	} else {
		for _, id := range unknown {
			rel, err := p.Relationships.Relationship(ctx, accountID, id)
			if err != nil {
				return err
			}
			found[id] = rel
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, rel := range found {
		p.of[[2]string{accountID, id}] = rel
	}
	return nil
}

// Relationship returns the relationship of accountID with targetID from the page, reading it once
// when the page has not read it.
func (p *pageRelationships) Relationship(ctx context.Context, accountID, targetID string) (appgraph.Relationship, error) {
	key := [2]string{accountID, targetID}
	p.mu.Lock()
	rel, ok := p.of[key]
	p.mu.Unlock()
	if ok {
		return rel, nil
	}
	rel, err := p.Relationships.Relationship(ctx, accountID, targetID)
	if err != nil {
		return appgraph.Relationship{}, err
	}
	p.mu.Lock()
	p.of[key] = rel
	p.mu.Unlock()
	return rel, nil
}

// pageQuotes is the quote repository with the quotes, their states, and the interaction policies
// of the statuses of a page read beforehand.
type pageQuotes struct {
	QuoteRepository
	// quoting are the statuses whose quotes were read: those of the page itself, not the ones it
	// refers to. page are all of them, whose interaction policies were read.
	quoting  []string
	page     []string
	quotes   map[string]content.Quote
	states   map[string]content.QuoteState
	policies map[string]content.InteractionPolicy
	// viewer and uris are the account and the statuses whose own interactions own holds.
	viewer string
	uris   []string
	own    []OwnInteraction
}

// ListOwnInteractions answers the interactions of the page's viewer with statuses of the page
// from what the page read, and reads the others.
func (p *pageQuotes) ListOwnInteractions(ctx context.Context, accountID string, uris []string) ([]OwnInteraction, error) {
	if accountID != p.viewer || p.viewer == "" || slices.ContainsFunc(uris, func(u string) bool { return !slices.Contains(p.uris, u) }) {
		return p.QuoteRepository.ListOwnInteractions(ctx, accountID, uris)
	}
	var out []OwnInteraction
	for _, o := range p.own {
		if slices.Contains(uris, o.InteractingURI) && o.Kind == content.InteractionReply || slices.Contains(uris, o.TargetURI) && o.Kind == content.InteractionAnnounce {
			out = append(out, o)
		}
	}
	return out, nil
}

// quotePart returns the part of a status of the page from m, failing with ErrNotFound as its read
// does when there is none; ok is false for a status outside the page.
func quotePart[T any](p *pageQuotes, m map[string]T, statusID, what string) (part T, ok bool, err error) {
	if m == nil || !slices.Contains(p.page, statusID) {
		return part, false, nil
	}
	if v, found := m[statusID]; found {
		return v, true, nil
	}
	return part, true, fmt.Errorf("%s of %s: %w", what, statusID, ErrNotFound)
}

func (p *pageQuotes) FindQuote(ctx context.Context, statusID string) (content.Quote, error) {
	if slices.Contains(p.quoting, statusID) {
		if q, ok := p.quotes[statusID]; ok {
			return q, nil
		}
		return content.Quote{}, fmt.Errorf("quote of %s: %w", statusID, ErrNotFound)
	}
	return p.QuoteRepository.FindQuote(ctx, statusID)
}

func (p *pageQuotes) FindQuoteState(ctx context.Context, statusID string) (content.QuoteState, error) {
	if _, quoting := p.quotes[statusID]; quoting {
		if st, ok, err := quotePart(p, p.states, statusID, "quote state"); ok {
			return st, err
		}
	}
	return p.QuoteRepository.FindQuoteState(ctx, statusID)
}

func (p *pageQuotes) FindInteractionPolicy(ctx context.Context, statusID string) (content.InteractionPolicy, error) {
	if p.policies != nil && slices.Contains(p.page, statusID) {
		return p.policies[statusID], nil
	}
	return p.QuoteRepository.FindInteractionPolicy(ctx, statusID)
}

// restrictionBatch is a RestrictionPolicy that reads the restrictions of many accounts at once, as
// the moderation module's does.
type restrictionBatch interface {
	RestrictionsOf(ctx context.Context, accountIDs []string) (map[string]Restrictions, error)
}

// pageRestrictions are the restrictions of the authors of a page read beforehand, and every other
// account's remembered once read, for one page.
type pageRestrictions struct {
	RestrictionPolicy
	mu sync.Mutex
	of map[string]Restrictions
}

// load reads the restrictions of the accounts of ids that are not known yet, together when the
// policy can, one by one otherwise.
func (p *pageRestrictions) load(ctx context.Context, ids []string) error {
	p.mu.Lock()
	var unknown []string
	for _, id := range ids {
		if _, ok := p.of[id]; !ok && !slices.Contains(unknown, id) {
			unknown = append(unknown, id)
		}
	}
	p.mu.Unlock()
	if len(unknown) == 0 {
		return nil
	}
	found := map[string]Restrictions{}
	if b, ok := p.RestrictionPolicy.(restrictionBatch); ok {
		var err error
		if found, err = b.RestrictionsOf(ctx, unknown); err != nil {
			return err
		}
	} else {
		for _, id := range unknown {
			rs, err := p.RestrictionPolicy.Restrictions(ctx, id)
			if err != nil {
				return err
			}
			found[id] = rs
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range unknown {
		p.of[id] = found[id]
	}
	return nil
}

// Restrictions returns the restrictions of accountID from the page, reading them once when the
// page has not read them.
func (p *pageRestrictions) Restrictions(ctx context.Context, accountID string) (Restrictions, error) {
	if err := p.load(ctx, []string{accountID}); err != nil {
		return Restrictions{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.of[accountID], nil
}
