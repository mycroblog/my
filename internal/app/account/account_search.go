// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/domain/account"
)

// The search of accounts for the autocomplete of mentions and the account search (ADR-0092 §1,
// FR-119, FR-306, FR-307): the accounts with a search term that starts with the query, in the
// order exact match, match from the start of a name, followed by the viewer, local, most
// followers. The infix terms of discoverable local accounts make the query match the middle of
// their names; a local account that is not discoverable matches only its whole username, or a
// viewer that follows it.

// Limits of the prefix search of accounts.
const (
	DefaultSearchLimit = 8
	MaxSearchLimit     = 20
	// searchCandidates is how many terms one search reads from the start of the range.
	searchCandidates = 100
)

// ErrInvalidSearchQuery reports a query that is empty or longer than MaxSearchQueryLength
// after its leading @.
var ErrInvalidSearchQuery = errors.New("account: invalid search query")

// SearchRelation is what the graph module tells about a viewer and a candidate of the search:
// Hidden when either blocks the other or the viewer mutes the candidate now, Following when the
// viewer actively follows it, and the number of its active followers.
type SearchRelation struct {
	Hidden    bool
	Following bool
	Followers int64
}

// SearchDeps are the dependencies of PrefixSearch. The composition root passes the graph and
// moderation modules as functions: the account module does not import them, since they import
// the account module.
type SearchDeps struct {
	Repository SearchTermRepository
	// Relations returns the relations of viewerID with the accounts ids; an account without an
	// entry has none.
	Relations func(ctx context.Context, viewerID string, ids []string) (map[string]SearchRelation, error)
	// DomainSuspended reports whether a domain, or a parent of it, is suspended. Nil suspends
	// none.
	DomainSuspended func(ctx context.Context, domain string) (bool, error)
}

// PrefixSearch is the prefix search of accounts.
type PrefixSearch struct {
	d SearchDeps
}

// NewPrefixSearch returns the search on d.
func NewPrefixSearch(d SearchDeps) *PrefixSearch { return &PrefixSearch{d: d} }

// NormalizeSearchQuery returns the form of a query that the search compares with the terms:
// without its leading @, under NormalizeSearchText, its white space collapsed to single spaces.
// It fails with ErrInvalidSearchQuery when the query without the @ is empty or longer than
// MaxSearchQueryLength characters.
func NormalizeSearchQuery(q string) (string, error) {
	q = strings.TrimPrefix(q, "@")
	if q == "" || len([]rune(q)) > MaxSearchQueryLength {
		return "", ErrInvalidSearchQuery
	}
	n := cutRunes(strings.Join(strings.Fields(NormalizeSearchText(q)), " "), MaxSearchQueryLength)
	if n == "" {
		return "", ErrInvalidSearchQuery
	}
	return n, nil
}

// Search returns the IDs of up to limit accounts (DefaultSearchLimit when zero or less, at most
// MaxSearchLimit) whose search term starts with q, for viewerID. It reads the first
// searchCandidates terms of the range in term order, leaves out the accounts that the viewer
// blocks, mutes, or is blocked by, those of a suspended domain, and those that match only by an
// exact-username term that is not equal to q when the viewer does not follow them, and orders
// the rest: an exact match of the username (or username@domain) first, then the matches from
// the start of a name (not only an infix), then the accounts that the viewer follows, then the
// local ones, then those with more followers, and then in term order. The search does not read
// indexable (ADR-0092 §1).
func (s *PrefixSearch) Search(ctx context.Context, viewerID, q string, limit int) ([]string, error) {
	n, err := NormalizeSearchQuery(q)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	limit = min(limit, MaxSearchLimit)
	terms, err := s.d.Repository.SearchTerms(ctx, n, n+"\U0010FFFF", searchCandidates)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		id, domain                     string
		exact, prefix, unlisted, local bool
		rel                            SearchRelation
	}
	var cands []*candidate
	byID := map[string]*candidate{}
	for _, t := range terms {
		c, ok := byID[t.AccountID]
		if !ok {
			c = &candidate{id: t.AccountID, domain: t.Domain, local: t.IsLocal}
			byID[t.AccountID] = c
			cands = append(cands, c)
		}
		switch t.Kind {
		case account.SearchTermUsername, account.SearchTermExactUsername:
			c.exact = c.exact || t.Term == n
			c.prefix = true
			c.unlisted = t.Kind == account.SearchTermExactUsername
		case account.SearchTermDisplayName:
			c.prefix = true
		}
	}
	if len(cands) == 0 {
		return []string{}, nil
	}

	suspended := map[string]bool{}
	if s.d.DomainSuspended != nil {
		for _, c := range cands {
			if c.local {
				continue
			}
			if _, seen := suspended[c.domain]; seen {
				continue
			}
			if suspended[c.domain], err = s.d.DomainSuspended(ctx, c.domain); err != nil {
				return nil, err
			}
		}
	}
	ids := make([]string, len(cands))
	for i, c := range cands {
		ids[i] = c.id
	}
	rels, err := s.d.Relations(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	kept := cands[:0]
	for _, c := range cands {
		c.rel = rels[c.id]
		if c.rel.Hidden || !c.local && suspended[c.domain] || c.unlisted && !c.exact && !c.rel.Following {
			continue
		}
		kept = append(kept, c)
	}

	slices.SortStableFunc(kept, func(a, b *candidate) int {
		for _, k := range [][2]bool{{a.exact, b.exact}, {a.prefix, b.prefix}, {a.rel.Following, b.rel.Following}, {a.local, b.local}} {
			if k[0] != k[1] {
				if k[0] {
					return -1
				}
				return 1
			}
		}
		switch {
		case a.rel.Followers > b.rel.Followers:
			return -1
		case a.rel.Followers < b.rel.Followers:
			return 1
		}
		return 0
	})
	out := make([]string, 0, min(limit, len(kept)))
	for _, c := range kept[:min(limit, len(kept))] {
		out = append(out, c.id)
	}
	return out, nil
}
