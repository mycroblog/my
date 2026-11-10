// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
)

// Search returns one page of what the viewer finds with q (ADR-0149 §8). The backend narrows the
// candidates to IDs, and the database decides what may be shown: each ID is read again and dropped
// when the status is missing or deleted, when it no longer qualifies (Eligible), when
// appcontent.CanView denies it in ContextSearch, or when the viewer mutes its author or the domain.
// An index that lags behind only finds less.
//
// Next is the backend's own cursor, so a page holds fewer than q.Limit statuses, or none, when
// results were dropped, and more may still follow it; only an empty Next ends the search. With
// search = none it queries nothing and finds nothing.
//
// It fails with ErrInvalidCursor for a cursor the backend did not issue, and with ErrBackendFailed
// when the backend cannot answer. Neither q nor anything the backend said is in the errors it
// returns, so that a query reaches no log (ADR-0149 §9).
func (s *Service) Search(ctx context.Context, viewer appcontent.Viewer, q Query) (Page, error) {
	if !s.Enabled() {
		return Page{}, nil
	}
	var found Page
	err := s.call(ctx, func(ctx context.Context) error {
		var err error
		found, err = s.d.Index.Search(ctx, q)
		return err
	})
	switch {
	case errors.Is(err, ErrInvalidCursor):
		return Page{}, ErrInvalidCursor
	case err != nil:
		return Page{}, backendFailure(err)
	}
	page := Page{Next: found.Next}
	if len(found.StatusIDs) == 0 {
		return page, nil
	}
	covered, err := s.Visibilities(ctx)
	if err != nil {
		return Page{}, err
	}
	candidates, err := s.candidates(ctx, found.StatusIDs, covered)
	if err != nil {
		return Page{}, err
	}
	if page.StatusIDs, err = s.d.Viewing.SearchVisible(ctx, viewer, candidates); err != nil {
		return Page{}, err
	}
	return page, nil
}

// backendFailure is ErrBackendFailed with how the call failed.
func backendFailure(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: timed out", ErrBackendFailed)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%w: canceled", ErrBackendFailed)
	default:
		return fmt.Errorf("%w: %T", ErrBackendFailed, err)
	}
}

// author is what a search reads once of each author of a page.
type author struct {
	indexable    bool
	restrictions appcontent.AuthorRestrictions
	domain       string
	domainRead   bool
}

// candidates reads the statuses of ids from the database and keeps those that exist and qualify.
func (s *Service) candidates(ctx context.Context, ids []string, covered []Visibility) ([]apptimeline.SearchCandidate, error) {
	sb, okS := s.d.Statuses.(statusBatch)
	ab, okA := s.d.Accounts.(accountBatch)
	if !okS || !okA {
		return s.candidatesByOne(ctx, ids, covered)
	}
	live, err := sb.LiveStatuses(ctx, ids)
	if err != nil {
		return nil, err
	}
	var authorIDs []string
	for _, id := range ids {
		if st, ok := live[id]; ok && !slices.Contains(authorIDs, st.AuthorAccountID) {
			authorIDs = append(authorIDs, st.AuthorAccountID)
		}
	}
	out := make([]apptimeline.SearchCandidate, 0, len(ids))
	if len(authorIDs) == 0 {
		return out, nil
	}
	indexable, err := ab.ProfilesIndexable(ctx, authorIDs)
	if err != nil {
		return nil, err
	}
	restrictions, err := sb.AuthorRestrictionsOf(ctx, authorIDs)
	if err != nil {
		return nil, err
	}
	// The domains of the remote authors of the hits that qualify.
	var remote []string
	for _, id := range ids {
		st, ok := live[id]
		if !ok || st.IsLocal || slices.Contains(remote, st.AuthorAccountID) {
			continue
		}
		if _, ok := Eligible(Candidate{Visibility: st.Visibility, Indexable: indexable[st.AuthorAccountID], Author: restrictions[st.AuthorAccountID]}, covered); ok {
			remote = append(remote, st.AuthorAccountID)
		}
	}
	domains := map[string]string{}
	if len(remote) > 0 {
		if domains, err = ab.AccountDomains(ctx, remote); err != nil {
			return nil, err
		}
	}
	for _, id := range ids {
		st, ok := live[id]
		if !ok {
			continue
		}
		a := st.AuthorAccountID
		if _, ok := Eligible(Candidate{Visibility: st.Visibility, Indexable: indexable[a], Author: restrictions[a]}, covered); !ok {
			continue
		}
		domain, known := domains[a]
		if !st.IsLocal && !known {
			continue // the author is gone
		}
		out = append(out, apptimeline.SearchCandidate{StatusID: st.ID, AuthorAccountID: a, AuthorDomain: domain, Visibility: st.Visibility})
	}
	return out, nil
}

// statusBatch is a StatusReader that reads many statuses and authors at once, as
// *appcontent.Service does.
type statusBatch interface {
	LiveStatuses(ctx context.Context, ids []string) (map[string]appcontent.Status, error)
	CurrentRevisions(ctx context.Context, ids []string) (map[string]appcontent.Revision, error)
	AuthorRestrictionsOf(ctx context.Context, accountIDs []string) (map[string]appcontent.AuthorRestrictions, error)
}

// accountBatch is an AccountReader that reads many accounts at once, as *appaccount.Service does.
type accountBatch interface {
	ProfilesIndexable(ctx context.Context, ids []string) (map[string]bool, error)
	AccountDomains(ctx context.Context, ids []string) (map[string]string, error)
}

// candidatesByOne is candidates reading each hit and author one by one.
func (s *Service) candidatesByOne(ctx context.Context, ids []string, covered []Visibility) ([]apptimeline.SearchCandidate, error) {
	out := make([]apptimeline.SearchCandidate, 0, len(ids))
	authors := map[string]*author{}
	for _, id := range ids {
		st, _, deleted, err := s.d.Statuses.ObjectStatus(ctx, id)
		switch {
		case errors.Is(err, appcontent.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		case deleted:
			continue
		}
		a, ok := authors[st.AuthorAccountID]
		if !ok {
			indexable, restrictions, err := s.authorState(ctx, st.AuthorAccountID)
			if err != nil {
				return nil, err
			}
			a = &author{indexable: indexable, restrictions: restrictions}
			authors[st.AuthorAccountID] = a
		}
		if _, ok := Eligible(Candidate{Visibility: st.Visibility, Indexable: a.indexable, Author: a.restrictions}, covered); !ok {
			continue
		}
		if !st.IsLocal && !a.domainRead {
			a.domain, err = s.d.Accounts.AccountDomain(ctx, st.AuthorAccountID)
			if errors.Is(err, appaccount.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			a.domainRead = true
		}
		out = append(out, apptimeline.SearchCandidate{
			StatusID: st.ID, AuthorAccountID: st.AuthorAccountID, AuthorDomain: a.domain, Visibility: st.Visibility,
		})
	}
	return out, nil
}
