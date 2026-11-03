// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The billing imports (ADR-0068 §9, FR-904): the admin operations on /admin/billing_imports
// (manage_finance, ADR-0058 server administration).

// BillingImports runs the use cases of the billing imports. *appfinance.Service implements it.
type BillingImports interface {
	ImportStatuses(ctx context.Context) ([]appfinance.ImportStatus, error)
	RequestImport(ctx context.Context, source, byUserID string) error
}

// requireBillingImports checks the manage_finance permission and that the use cases are
// configured; it returns the problem to answer otherwise, and the viewer.
func (s *Server) requireBillingImports(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageFinance); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.BillingImports == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

func newMoney(m finance.Money) Money { return Money{Amount: m.Decimal(), Currency: string(m.Currency)} }

func newBillingImportRun(r appfinance.Run) BillingImportRun {
	outcome := BillingImportRunOutcomeSucceeded
	if !r.Succeeded() {
		outcome = BillingImportRunOutcomeFailed
	}
	return BillingImportRun{
		Outcome: outcome, ErrorKind: BillingImportRunErrorKind(r.ErrorKind), Requests: r.Requests, BytesBilled: r.BytesBilled,
		OccurredAt: r.OccurredAt.UTC(),
	}
}

// newBillingImport presents the state of a source's imports to the administrators.
func newBillingImport(st appfinance.ImportStatus) BillingImport {
	out := BillingImport{
		Source: BillingSourceName(st.Source), Provider: st.Provider, TimeZone: st.Location.String(),
		Method:        BillingImportMethod(st.Method),
		RequestsToday: st.RequestsToday, RequestsThisMonth: st.RequestsThisMonth, BytesBilledThisMonth: st.BytesBilledThisMonth,
		Months: make([]BillingImportMonth, 0, len(st.Months)),
	}
	if st.LastRun != nil {
		r := newBillingImportRun(*st.LastRun)
		out.LastRun = &r
	}
	if st.LastSuccess != nil {
		at := st.LastSuccess.OccurredAt.UTC()
		out.LastSuccessAt = &at
	}
	if st.Caps != nil {
		perDay, perMonth := st.Caps.PerDay, st.Caps.PerMonth
		out.MaxRequestsPerDay, out.MaxRequestsPerMonth = &perDay, &perMonth
	}
	for _, m := range st.Months {
		month := BillingImportMonth{Month: m.Month.String(), Kind: BillingImportMonthKind(m.Kind), Totals: make([]Money, 0, len(m.Totals))}
		for _, t := range m.Totals {
			month.Totals = append(month.Totals, newMoney(t))
		}
		out.Months = append(out.Months, month)
	}
	return out
}

// ListBillingImports serves GET /api/v1/admin/billing_imports.
func (s *Server) ListBillingImports(ctx context.Context, _ ListBillingImportsRequestObject) (ListBillingImportsResponseObject, error) {
	_, p, err := s.requireBillingImports(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return ListBillingImportsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	list, err := s.o.BillingImports.ImportStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list billing imports: %w", err)
	}
	out := BillingImportList{Items: make([]BillingImport, 0, len(list))}
	for _, st := range list {
		out.Items = append(out.Items, newBillingImport(st))
	}
	cache := cacheNoStore
	return ListBillingImports200JSONResponse{Body: out, Headers: ListBillingImports200ResponseHeaders{CacheControl: &cache}}, nil
}

// RunBillingImport serves POST /api/v1/admin/billing_imports/{source}/runs.
func (s *Server) RunBillingImport(ctx context.Context, req RunBillingImportRequestObject) (RunBillingImportResponseObject, error) {
	v, p, err := s.requireBillingImports(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RunBillingImportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.BillingImports.RequestImport(ctx, string(req.Source), v.UserID)
	switch {
	case errors.Is(err, appfinance.ErrUnknownSource):
		return RunBillingImportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appfinance.ErrImportTooSoon):
		l := s.Localizer(ctx)
		now := time.Now().UTC()
		wait := int(now.Truncate(time.Hour).Add(time.Hour).Sub(now).Seconds()) + 1
		return RunBillingImportdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusTooManyRequests, ProblemTypeBlank, l.ServerErrorsRateLimited(), l.ServerErrorsRateLimitedDetail(wait))), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: run billing import of %s: %w", req.Source, err)
	}
	return RunBillingImport202Response{}, nil
}
