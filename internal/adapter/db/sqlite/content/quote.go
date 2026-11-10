// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var _ appcontent.QuoteRepository = (*StatusRepository)(nil)

// InsertQuote implements content.QuoteRepository.
func (r *StatusRepository) InsertQuote(ctx context.Context, tx uow.Tx, q content.Quote) (bool, error) {
	qs, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := qs.InsertStatusQuote(ctx, contentgen.InsertStatusQuoteParams{StatusID: q.StatusID, QuotedUri: q.QuotedURI, Form: string(q.Form)})
	return n > 0, sqlite.Wrap(err, "insert status quote")
}

// FindQuote implements content.QuoteRepository.
func (r *StatusRepository) FindQuote(ctx context.Context, statusID string) (content.Quote, error) {
	row, err := r.db.reader().GetStatusQuote(ctx, statusID)
	if err != nil {
		return content.Quote{}, sqlite.NotFound(err, appcontent.ErrNotFound, "quote of "+statusID)
	}
	return quoteFrom(row), nil
}

// FindQuoteTx implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteTx(ctx context.Context, tx uow.Tx, statusID string) (content.Quote, error) {
	d, err := r.db.in(tx)
	if err != nil {
		return content.Quote{}, err
	}
	return (&StatusRepository{db: d}).FindQuote(ctx, statusID)
}

// ListQuotesAfter implements content.QuoteRepository.
func (r *StatusRepository) ListQuotesAfter(ctx context.Context, tx uow.Tx, after string, limit int) ([]content.Quote, error) {
	qs, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := qs.ListStatusQuotesAfter(ctx, contentgen.ListStatusQuotesAfterParams{After: after, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list status quotes")
	}
	out := make([]content.Quote, 0, len(rows))
	for _, row := range rows {
		out = append(out, quoteFrom(row))
	}
	return out, nil
}

// FindQuoteState implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteState(ctx context.Context, statusID string) (content.QuoteState, error) {
	row, err := r.db.reader().GetStatusQuoteState(ctx, statusID)
	if err != nil {
		return content.QuoteState{}, sqlite.NotFound(err, appcontent.ErrNotFound, "quote state of "+statusID)
	}
	return quoteStateOf(quoteStateRow(row)), nil
}

// FindQuoteStateTx implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteStateTx(ctx context.Context, tx uow.Tx, statusID string) (content.QuoteState, error) {
	d, err := r.db.in(tx)
	if err != nil {
		return content.QuoteState{}, err
	}
	return (&StatusRepository{db: d}).FindQuoteState(ctx, statusID)
}

// ListQuoteStatesByAuthorization implements content.QuoteRepository.
func (r *StatusRepository) ListQuoteStatesByAuthorization(ctx context.Context, uri string) ([]content.QuoteState, error) {
	rows, err := r.db.reader().ListQuoteStatesByAuthorization(ctx, sqlite.ToNullString(uri))
	if err != nil {
		return nil, sqlite.Wrap(err, "list quote states by authorization")
	}
	out := make([]content.QuoteState, 0, len(rows))
	for _, row := range rows {
		out = append(out, quoteStateOf(quoteStateRow(row)))
	}
	return out, nil
}

func quoteFrom(row contentgen.StatusQuote) content.Quote {
	return content.Quote{StatusID: row.StatusID, QuotedURI: row.QuotedUri, Form: content.QuoteForm(row.Form)}
}

// quoteStateRow is a row of the queries that derive the state of a quote.
type quoteStateRow struct {
	StatusID         string
	Form             string
	Detached         int64
	State            string
	AuthorizationUri sql.NullString
}

// quoteStateOf derives the state of a quote from its row (content.QuoteStateFrom).
func quoteStateOf(row quoteStateRow) content.QuoteState {
	q := content.Quote{StatusID: row.StatusID, Form: content.QuoteForm(row.Form)}
	var req *content.InteractionState
	if row.State != "" {
		req = &content.InteractionState{State: content.InteractionRequestState(row.State), AuthorizationURI: row.AuthorizationUri.String}
	}
	return content.QuoteStateFrom(q, row.Detached != 0, req)
}

// ListQuotesOf implements content.QuoteRepository.
func (r *StatusRepository) ListQuotesOf(ctx context.Context, uri, before string, limit int) ([]content.Quote, error) {
	rows, err := r.db.reader().ListStatusQuotesOf(ctx, contentgen.ListStatusQuotesOfParams{QuotedUri: uri, Before: before, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list quotes of a status")
	}
	out := make([]content.Quote, 0, len(rows))
	for _, row := range rows {
		out = append(out, quoteFrom(row))
	}
	return out, nil
}
