// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"slices"

	appgraph "github.com/mycroblog/my/internal/app/graph"
)

// relationshipsWithBatch is a Relationships that reads the relationships of many accounts with one
// target at once, as *appgraph.Service does.
type relationshipsWithBatch interface {
	RelationshipsWith(ctx context.Context, targetID string, accountIDs []string) (map[string]appgraph.Relationship, error)
}

// ViewsFor is Views of the statuses of ids for each of viewers, local signed-in viewers, by
// viewer account, with a number of queries that does not grow with the viewers: the statuses and
// everything about them that is the same for every viewer are read once (Reader.page), and what
// each viewer did with them, their votes, the statuses that mention them, and their relationships
// with the authors are read for all the viewers together.
func (r *Reader) ViewsFor(ctx context.Context, viewers []Viewer, ids []string) (map[string][]StatusView, error) {
	out := make(map[string][]StatusView, len(viewers))
	if len(viewers) == 0 {
		return out, nil
	}
	base, err := r.page(ctx, Viewer{Kind: ViewerAnonymous}, ids)
	if err != nil {
		return nil, err
	}
	statuses, okS := base.d.Statuses.(*pageStatuses)
	interactions, okI := base.d.Interactions.(*pageInteractions)
	relationships, okR := base.d.Relationships.(*pageRelationships)
	batch, okB := r.d.Relationships.(relationshipsWithBatch)
	if !okS || !okI || !okR || !okB {
		for _, v := range viewers {
			views, err := r.Views(ctx, v, ids)
			if err != nil {
				return nil, err
			}
			out[v.AccountID] = views
		}
		return out, nil
	}
	accounts := make([]string, len(viewers))
	for i, v := range viewers {
		accounts[i] = v.AccountID
	}
	live := interactions.page
	votes, err := r.d.Statuses.ListPollVotesOfAccountsIn(ctx, statuses.polls, accounts)
	if err != nil {
		return nil, err
	}
	mentioned, err := r.d.Interactions.MentionsOfAccountsIn(ctx, live, accounts)
	if err != nil {
		return nil, err
	}
	own, err := r.d.Interactions.FindInteractionsOfAccounts(ctx, accounts, live)
	if err != nil {
		return nil, err
	}
	var authors []string
	for _, id := range live {
		if a := statuses.statuses[id].AuthorAccountID; !slices.Contains(authors, a) {
			authors = append(authors, a)
		}
	}
	rels := map[[2]string]appgraph.Relationship{}
	for _, author := range authors {
		with, err := batch.RelationshipsWith(ctx, author, accounts)
		if err != nil {
			return nil, err
		}
		for account, rel := range with {
			rels[[2]string{account, author}] = rel
		}
	}
	for _, v := range viewers {
		vs := *statuses
		vs.voter, vs.votes = v.AccountID, votes[v.AccountID]
		vi := *interactions
		vi.viewer, vi.mentioned, vi.own = v.AccountID, mentioned[v.AccountID], own[v.AccountID]
		vr := &pageRelationships{Relationships: relationships.Relationships, of: map[[2]string]appgraph.Relationship{}}
		for _, author := range authors {
			vr.of[[2]string{v.AccountID, author}] = rels[[2]string{v.AccountID, author}]
		}
		p := *base
		p.d.Statuses, p.d.Interactions, p.d.Relationships = &vs, &vi, vr
		p.aud.statuses, p.aud.interactions, p.aud.relationships = &vs, &vi, vr
		views := make([]StatusView, 0, len(ids))
		for _, id := range ids {
			view, err := p.View(ctx, v, id)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			views = append(views, view)
		}
		out[v.AccountID] = views
	}
	return out, nil
}
