// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// ListRestrictions implements timeline.Repository.
func (r *Repository) ListRestrictions(ctx context.Context, tx uow.Tx) ([]apptimeline.AuthorRestriction, []apptimeline.DomainRestriction, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, nil, err
	}
	authorRows, err := q.ListAllFeedAuthorRestrictions(ctx)
	if err != nil {
		return nil, nil, postgres.Wrap(err, "list feed author restrictions")
	}
	domainRows, err := q.ListAllFeedDomainRestrictions(ctx)
	if err != nil {
		return nil, nil, postgres.Wrap(err, "list feed domain restrictions")
	}
	authors := make([]apptimeline.AuthorRestriction, len(authorRows))
	for i, row := range authorRows {
		authors[i] = apptimeline.AuthorRestriction{
			AccountID: row.AccountID, Restriction: timeline.Restriction(row.Restriction), EffectiveUntil: postgres.UTCPtr(row.EffectiveUntil),
		}
	}
	domains := make([]apptimeline.DomainRestriction, len(domainRows))
	for i, row := range domainRows {
		domains[i] = apptimeline.DomainRestriction{
			Domain: row.Domain, Restriction: timeline.Restriction(row.Restriction), EffectiveUntil: postgres.UTCPtr(row.EffectiveUntil),
		}
	}
	return authors, domains, nil
}

// DeleteRestrictions implements timeline.Repository.
func (r *Repository) DeleteRestrictions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "empty feed restrictions",
		(*timelinegen.Queries).DeleteAllFeedAuthorRestrictions, (*timelinegen.Queries).DeleteAllFeedDomainRestrictions)
}
