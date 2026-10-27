// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
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
	n, err := qs.InsertStatusQuote(ctx, sqlitegen.InsertStatusQuoteParams{StatusID: q.StatusID, QuotedUri: q.QuotedURI, Form: string(q.Form)})
	return n > 0, wrap(err, "insert status quote")
}

// FindQuote implements content.QuoteRepository.
func (r *StatusRepository) FindQuote(ctx context.Context, statusID string) (content.Quote, error) {
	row, err := r.db.reader().GetStatusQuote(ctx, statusID)
	if err != nil {
		return content.Quote{}, notFound(err, appcontent.ErrNotFound, "quote of "+statusID)
	}
	return quoteFrom(row), nil
}

// FindQuoteTx implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteTx(ctx context.Context, tx uow.Tx, statusID string) (content.Quote, error) {
	d, err := r.db.in(tx)
	if err != nil {
		return content.Quote{}, err
	}
	return d.StatusRepository().FindQuote(ctx, statusID)
}

// ListQuotesAfter implements content.QuoteRepository.
func (r *StatusRepository) ListQuotesAfter(ctx context.Context, tx uow.Tx, after string, limit int) ([]content.Quote, error) {
	qs, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := qs.ListStatusQuotesAfter(ctx, sqlitegen.ListStatusQuotesAfterParams{After: after, PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list status quotes")
	}
	out := make([]content.Quote, 0, len(rows))
	for _, row := range rows {
		out = append(out, quoteFrom(row))
	}
	return out, nil
}

// InsertQuoteEvent implements content.QuoteRepository.
func (r *StatusRepository) InsertQuoteEvent(ctx context.Context, tx uow.Tx, e content.QuoteEvent) error {
	qs, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(qs.InsertStatusQuoteEvent(ctx, sqlitegen.InsertStatusQuoteEventParams{
		ID: e.ID, StatusID: e.StatusID, Kind: string(e.Kind), AuthorizationUri: toNullString(e.AuthorizationURI), OccurredAt: toMillis(e.OccurredAt),
	}), "insert status quote event")
}

// ListQuoteEvents implements content.QuoteRepository.
func (r *StatusRepository) ListQuoteEvents(ctx context.Context, tx uow.Tx, statusID string) ([]content.QuoteEvent, error) {
	qs, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := qs.ListStatusQuoteEvents(ctx, statusID)
	if err != nil {
		return nil, wrap(err, "list status quote events")
	}
	out := make([]content.QuoteEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, content.QuoteEvent{
			ID: row.ID, StatusID: row.StatusID, Kind: content.QuoteEventKind(row.Kind),
			AuthorizationURI: row.AuthorizationUri.String, OccurredAt: fromMillis(row.OccurredAt),
		})
	}
	return out, nil
}

// UpsertQuoteState implements content.QuoteRepository.
func (r *StatusRepository) UpsertQuoteState(ctx context.Context, tx uow.Tx, s content.QuoteState) error {
	qs, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(qs.UpsertStatusQuoteState(ctx, sqlitegen.UpsertStatusQuoteStateParams{
		StatusID: s.StatusID, State: string(s.Approval), AuthorizationUri: toNullString(s.AuthorizationURI),
	}), "upsert status quote state")
}

// FindQuoteState implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteState(ctx context.Context, statusID string) (content.QuoteState, error) {
	row, err := r.db.reader().GetStatusQuoteState(ctx, statusID)
	if err != nil {
		return content.QuoteState{}, notFound(err, appcontent.ErrNotFound, "quote state of "+statusID)
	}
	return quoteStateFrom(row), nil
}

// FindQuoteStateTx implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteStateTx(ctx context.Context, tx uow.Tx, statusID string) (content.QuoteState, error) {
	d, err := r.db.in(tx)
	if err != nil {
		return content.QuoteState{}, err
	}
	return d.StatusRepository().FindQuoteState(ctx, statusID)
}

// ListQuoteStatesByAuthorization implements content.QuoteRepository.
func (r *StatusRepository) ListQuoteStatesByAuthorization(ctx context.Context, uri string) ([]content.QuoteState, error) {
	rows, err := r.db.reader().ListStatusQuoteStatesByAuthorization(ctx, toNullString(uri))
	if err != nil {
		return nil, wrap(err, "list status quote states by authorization")
	}
	return quoteStatesFrom(rows), nil
}

// ListQuoteStates implements content.QuoteRepository.
func (r *StatusRepository) ListQuoteStates(ctx context.Context) ([]content.QuoteState, error) {
	rows, err := r.db.reader().ListAllStatusQuoteStates(ctx)
	if err != nil {
		return nil, wrap(err, "list status quote states")
	}
	return quoteStatesFrom(rows), nil
}

// DeleteAllQuoteStates implements content.QuoteRepository.
func (r *StatusRepository) DeleteAllQuoteStates(ctx context.Context, tx uow.Tx) error {
	qs, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(qs.DeleteAllStatusQuoteStates(ctx), "delete status quote states")
}

func quoteFrom(row sqlitegen.StatusQuote) content.Quote {
	return content.Quote{StatusID: row.StatusID, QuotedURI: row.QuotedUri, Form: content.QuoteForm(row.Form)}
}

func quoteStateFrom(row sqlitegen.StatusQuoteState) content.QuoteState {
	return content.QuoteState{StatusID: row.StatusID, Approval: content.QuoteApproval(row.State), AuthorizationURI: row.AuthorizationUri.String}
}

func quoteStatesFrom(rows []sqlitegen.StatusQuoteState) []content.QuoteState {
	out := make([]content.QuoteState, 0, len(rows))
	for _, row := range rows {
		out = append(out, quoteStateFrom(row))
	}
	return out
}

// ListQuotesOf implements content.QuoteRepository.
func (r *StatusRepository) ListQuotesOf(ctx context.Context, uri, before string, limit int) ([]content.Quote, error) {
	rows, err := r.db.reader().ListStatusQuotesOf(ctx, sqlitegen.ListStatusQuotesOfParams{QuotedUri: uri, Before: before, PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list quotes of a status")
	}
	out := make([]content.Quote, 0, len(rows))
	for _, row := range rows {
		out = append(out, quoteFrom(row))
	}
	return out, nil
}

// UpsertQuotePolicy implements content.QuoteRepository.
func (r *StatusRepository) UpsertQuotePolicy(ctx context.Context, tx uow.Tx, p content.StatusQuotePolicy) error {
	qs, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(qs.UpsertStatusQuotePolicy(ctx, sqlitegen.UpsertStatusQuotePolicyParams{
		StatusID: p.StatusID, Policy: string(p.Policy), UpdatedAt: toMillis(p.UpdatedAt),
	}), "upsert status quote policy")
}

// DeleteQuotePolicy implements content.QuoteRepository.
func (r *StatusRepository) DeleteQuotePolicy(ctx context.Context, tx uow.Tx, statusID string) error {
	qs, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(qs.DeleteStatusQuotePolicy(ctx, statusID), "delete status quote policy")
}

// FindQuotePolicy implements content.QuoteRepository.
func (r *StatusRepository) FindQuotePolicy(ctx context.Context, statusID string) (content.StatusQuotePolicy, error) {
	row, err := r.db.reader().GetStatusQuotePolicy(ctx, statusID)
	if err != nil {
		return content.StatusQuotePolicy{}, notFound(err, appcontent.ErrNotFound, "quote policy of "+statusID)
	}
	return content.StatusQuotePolicy{StatusID: row.StatusID, Policy: content.QuotePolicy(row.Policy), UpdatedAt: fromMillis(row.UpdatedAt)}, nil
}

// InsertQuoteAuthorization implements content.QuoteRepository.
func (r *StatusRepository) InsertQuoteAuthorization(ctx context.Context, tx uow.Tx, a content.QuoteAuthorization) (bool, error) {
	qs, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := qs.InsertQuoteAuthorization(ctx, sqlitegen.InsertQuoteAuthorizationParams{
		ID: a.ID, QuotingUri: a.QuotingURI, QuotingAccountID: a.QuotingAccountID, QuotedStatusID: a.QuotedStatusID, CreatedAt: toMillis(a.CreatedAt),
	})
	return n > 0, wrap(err, "insert quote authorization")
}

// FindQuoteAuthorization implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteAuthorization(ctx context.Context, id string) (content.QuoteAuthorization, error) {
	row, err := r.db.reader().GetQuoteAuthorization(ctx, id)
	if err != nil {
		return content.QuoteAuthorization{}, notFound(err, appcontent.ErrNotFound, "quote authorization "+id)
	}
	return quoteAuthorizationFrom(sqlitegen.GetQuoteAuthorizationForRow(row)), nil
}

// FindQuoteAuthorizationFor implements content.QuoteRepository.
func (r *StatusRepository) FindQuoteAuthorizationFor(ctx context.Context, quotingURI, quotedStatusID string) (content.QuoteAuthorization, error) {
	row, err := r.db.reader().GetQuoteAuthorizationFor(ctx, sqlitegen.GetQuoteAuthorizationForParams{QuotingUri: quotingURI, QuotedStatusID: quotedStatusID})
	if err != nil {
		return content.QuoteAuthorization{}, notFound(err, appcontent.ErrNotFound, "quote authorization of "+quotingURI)
	}
	return quoteAuthorizationFrom(row), nil
}

// InsertQuoteAuthorizationRevocation implements content.QuoteRepository.
func (r *StatusRepository) InsertQuoteAuthorizationRevocation(ctx context.Context, tx uow.Tx, authorizationID string, at time.Time) (bool, error) {
	qs, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := qs.InsertQuoteAuthorizationRevocation(ctx, sqlitegen.InsertQuoteAuthorizationRevocationParams{AuthorizationID: authorizationID, RevokedAt: toMillis(at)})
	return n > 0, wrap(err, "insert quote authorization revocation")
}

func quoteAuthorizationFrom(row sqlitegen.GetQuoteAuthorizationForRow) content.QuoteAuthorization {
	return content.QuoteAuthorization{
		ID: row.ID, QuotingURI: row.QuotingUri, QuotingAccountID: row.QuotingAccountID, QuotedStatusID: row.QuotedStatusID,
		CreatedAt: fromMillis(row.CreatedAt), RevokedAt: fromNullMillis(row.RevokedAt),
	}
}
