// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"
	"slices"
	"sync"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds what a page of statuses reads once for all of them, so that presenting a page
// makes a number of round trips to the database that does not grow with each status. The page's
// Reader decides and presents each status as View does, reading through what was read for the
// page first.

// page returns a Reader for the statuses of ids that viewer sees. It reads together, for all the
// statuses: the statuses, their deletions, current revisions, counters, and hashtags, the accounts
// of their authors and of the accounts they mention, and, for a signed-in viewer, the viewer's
// relationships with the authors and the statuses that mention the viewer. Anything else, and
// anything of a status outside the page (a quoted status, the status replied to), is read once,
// the first time a status needs it.
func (r *Reader) page(ctx context.Context, viewer Viewer, ids []string) (*Reader, error) {
	statuses, err := r.d.Statuses.FindStatuses(ctx, ids)
	if err != nil {
		return nil, err
	}
	live := make([]string, 0, len(statuses))
	for _, id := range ids {
		if _, ok := statuses[id]; ok && !slices.Contains(live, id) {
			live = append(live, id)
		}
	}
	page := &pageStatuses{Repository: r.d.Statuses, statuses: statuses, ids: ids}
	if page.deletions, err = r.d.Statuses.FindDeletions(ctx, live); err != nil {
		return nil, err
	}
	if page.revisions, err = r.d.Statuses.FindCurrentRevisions(ctx, live); err != nil {
		return nil, err
	}
	if page.counters, err = r.d.Statuses.FindCountersOf(ctx, live); err != nil {
		return nil, err
	}
	var accountIDs, tagIDs []string
	for _, id := range live {
		accountIDs = append(accountIDs, statuses[id].AuthorAccountID)
	}
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
	relationships := &pageRelationships{Relationships: r.d.Relationships, of: map[[2]string]appgraph.Relationship{}}
	interactions := &pageInteractions{InteractionRepository: r.d.Interactions}
	if viewer.Kind == ViewerLocal && viewer.AccountID != "" {
		authors := make([]string, 0, len(live))
		for _, id := range live {
			authors = append(authors, statuses[id].AuthorAccountID)
		}
		if err := relationships.load(ctx, viewer.AccountID, authors); err != nil {
			return nil, err
		}
		interactions.viewer, interactions.page = viewer.AccountID, live
		if interactions.mentioned, err = r.d.Interactions.MentionedIn(ctx, live, viewer.AccountID); err != nil {
			return nil, err
		}
	}
	p := *r
	p.d.Statuses, p.d.Accounts, p.d.Relationships, p.d.Interactions = page, accounts, relationships, interactions
	p.aud.statuses, p.aud.accounts, p.aud.relationships, p.aud.interactions = page, accounts, relationships, interactions
	return &p, nil
}

// pageStatuses is the status repository with the parts of the statuses of a page read
// beforehand. A part that a status of the page does not have fails with ErrNotFound, as its read
// does.
type pageStatuses struct {
	Repository
	// ids are the IDs of the page, statuses those that exist, and the others the parts of them.
	ids       []string
	statuses  map[string]content.Status
	deletions map[string]content.Deletion
	revisions map[string]content.Revision
	counters  map[string]content.Counters
	tags      map[string]content.Tag
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

// FindTag returns a hashtag of the page's statuses as it was read, and reads any other.
func (p *pageStatuses) FindTag(ctx context.Context, id string) (content.Tag, error) {
	if tag, ok := p.tags[id]; ok {
		return tag, nil
	}
	return p.Repository.FindTag(ctx, id)
}

// pageInteractions are the interactions with the statuses of a page that mention its viewer read
// beforehand.
type pageInteractions struct {
	InteractionRepository
	viewer    string
	page      []string
	mentioned map[string]bool
}

// Mentioned tells whether a status of the page mentions its viewer as it was read, and reads any
// other.
func (p *pageInteractions) Mentioned(ctx context.Context, statusID, accountID string) (bool, error) {
	if accountID == p.viewer && p.viewer != "" && slices.Contains(p.page, statusID) {
		return p.mentioned[statusID], nil
	}
	return p.InteractionRepository.Mentioned(ctx, statusID, accountID)
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
