// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
)

// The suggestions of accounts to follow (FR-308): the local accounts that the administrators
// feature (the instance setting featured_accounts), in their order, then the local accounts that
// posted most recently. Only discoverable, active local accounts are suggested, and never the
// viewer, the accounts it follows, or those it blocks, mutes, or is blocked by.

// Limits of the suggestions.
const (
	DefaultSuggestionLimit = 10
	MaxSuggestionLimit     = 40
	// SuggestionActivityWindow is how far back a public or local post makes an account recently
	// active.
	SuggestionActivityWindow = 30 * 24 * time.Hour
)

// SuggestionSource says why an account is suggested.
type SuggestionSource string

// Suggestion sources.
const (
	SuggestionFeatured       SuggestionSource = "featured"
	SuggestionRecentlyActive SuggestionSource = "recently_active"
)

// Suggestion is one suggested account.
type Suggestion struct {
	AccountID string
	Source    SuggestionSource
}

// SuggestionDeps are the dependencies of Suggestions. The composition root passes the instance,
// content, and graph modules as functions: the account module does not import them, since they
// import the account module.
type SuggestionDeps struct {
	Accounts *Service
	Clock    kernel.Clock
	// Featured returns the usernames of the featured local accounts, in order.
	Featured func(ctx context.Context) ([]string, error)
	// RecentPosters returns up to limit local accounts with a public or local post published at
	// since or later, the one that posted last first.
	RecentPosters func(ctx context.Context, since time.Time, limit int) ([]string, error)
	// Relations returns the relations of viewerID with the accounts ids, as for the search.
	Relations func(ctx context.Context, viewerID string, ids []string) (map[string]SearchRelation, error)
}

// Suggestions suggests accounts to follow.
type Suggestions struct {
	d SuggestionDeps
}

// NewSuggestions returns the suggestions on d.
func NewSuggestions(d SuggestionDeps) *Suggestions { return &Suggestions{d: d} }

// Suggest returns up to limit accounts (DefaultSuggestionLimit when zero or less, at most
// MaxSuggestionLimit) to suggest to viewerID: the featured accounts first, in their order, then
// the recently active ones, the one that posted last first, each account once.
func (s *Suggestions) Suggest(ctx context.Context, viewerID string, limit int) ([]Suggestion, error) {
	if limit <= 0 {
		limit = DefaultSuggestionLimit
	}
	limit = min(limit, MaxSuggestionLimit)

	var cands []Suggestion
	seen := map[string]bool{viewerID: true}
	add := func(id string, src SuggestionSource) {
		if !seen[id] {
			seen[id] = true
			cands = append(cands, Suggestion{AccountID: id, Source: src})
		}
	}
	if s.d.Featured != nil {
		names, err := s.d.Featured(ctx)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			h, err := s.d.Accounts.d.Repository.FindHandle(ctx, account.NormalizeUsername(name), account.LocalDomain)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			add(h.AccountID, SuggestionFeatured)
		}
	}
	if s.d.RecentPosters != nil {
		// Read more than limit: some of them are followed already or left out.
		ids, err := s.d.RecentPosters(ctx, s.d.Clock.Now().Add(-SuggestionActivityWindow), 3*limit+10)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			add(id, SuggestionRecentlyActive)
		}
	}
	if len(cands) == 0 {
		return []Suggestion{}, nil
	}

	ids := make([]string, len(cands))
	for i, c := range cands {
		ids[i] = c.AccountID
	}
	rels, err := s.d.Relations(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Suggestion, 0, limit)
	for _, c := range cands {
		if len(out) == limit {
			break
		}
		if r := rels[c.AccountID]; r.Hidden || r.Following {
			continue
		}
		ok, err := s.suggestable(ctx, c.AccountID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// suggestable reports whether an account is a discoverable, active local account of a person.
func (s *Suggestions) suggestable(ctx context.Context, id string) (bool, error) {
	a, err := s.d.Accounts.FindByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return a.Local != nil && !a.IsInstanceActor() && a.Lifecycle.Active() &&
		a.Profile != nil && a.Profile.Discoverable, nil
}
