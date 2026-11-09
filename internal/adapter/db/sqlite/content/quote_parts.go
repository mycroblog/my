// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/domain/content"
)

// FindQuotes implements content.QuoteRepository.
func (r *StatusRepository) FindQuotes(ctx context.Context, statusIDs []string) (map[string]content.Quote, error) {
	out := map[string]content.Quote{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListQuotesOfStatuses(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list quotes")
	}
	for _, row := range rows {
		out[row.StatusID] = quoteFrom(row)
	}
	return out, nil
}

// FindQuoteStates implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteStates(ctx context.Context, statusIDs []string) (map[string]content.QuoteState, error) {
	out := map[string]content.QuoteState{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListQuoteStatesOfStatuses(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list quote states")
	}
	for _, row := range rows {
		out[row.StatusID] = quoteStateFrom(row)
	}
	return out, nil
}

// FindQuotePolicies implements content.QuoteRepository.
func (r *StatusRepository) FindQuotePolicies(ctx context.Context, statusIDs []string) (map[string]content.StatusQuotePolicy, error) {
	out := map[string]content.StatusQuotePolicy{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListQuotePoliciesOfStatuses(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list quote policies")
	}
	for _, row := range rows {
		out[row.StatusID] = content.StatusQuotePolicy{StatusID: row.StatusID, Policy: content.QuotePolicy(row.Policy), UpdatedAt: sqlite.FromMillis(row.UpdatedAt)}
	}
	return out, nil
}

// FindStatusesByURIs implements content.Repository.
func (r *StatusRepository) FindStatusesByURIs(ctx context.Context, uris []string) (map[string]content.Status, error) {
	out := map[string]content.Status{}
	if len(uris) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(uris)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStatusesByURIs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list statuses by URI")
	}
	for _, row := range rows {
		out[row.Uri] = statusFrom(row)
	}
	return out, nil
}
