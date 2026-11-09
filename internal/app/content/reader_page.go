// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"fmt"
	"sync"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds what a page of statuses reads once for all of them, so that presenting a page
// makes a number of round trips to the database that does not grow with each status. The page's
// Reader decides and presents each status as View does, reading through what was read for the
// page first.

// page returns a Reader for the statuses of ids: it reads the statuses and the accounts of their
// authors together, and reads each other account once, the first time a status needs it.
func (r *Reader) page(ctx context.Context, ids []string) (*Reader, error) {
	statuses, err := r.d.Statuses.FindStatuses(ctx, ids)
	if err != nil {
		return nil, err
	}
	authors := make([]string, 0, len(statuses))
	for _, st := range statuses {
		authors = append(authors, st.AuthorAccountID)
	}
	accounts := &pageAccounts{Accounts: r.d.Accounts, actors: map[string]*appaccount.Actor{}}
	if err := accounts.load(ctx, authors); err != nil {
		return nil, err
	}
	page := &pageStatuses{Repository: r.d.Statuses, statuses: statuses}
	for _, id := range ids {
		if _, ok := statuses[id]; !ok {
			page.missing = append(page.missing, id)
		}
	}
	p := *r
	p.d.Statuses, p.d.Accounts = page, accounts
	p.aud.statuses, p.aud.accounts = page, accounts
	return &p, nil
}

// pageStatuses is the status repository with the statuses of a page read beforehand.
type pageStatuses struct {
	Repository
	statuses map[string]content.Status
	// missing are the IDs of the page that have no status.
	missing []string
}

// FindStatus returns a status of the page as it was read for the page, and reads any other.
func (p *pageStatuses) FindStatus(ctx context.Context, id string) (content.Status, error) {
	if st, ok := p.statuses[id]; ok {
		return st, nil
	}
	for _, m := range p.missing {
		if m == id {
			return content.Status{}, fmt.Errorf("status %s: %w", id, ErrNotFound)
		}
	}
	return p.Repository.FindStatus(ctx, id)
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
