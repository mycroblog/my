// SPDX-License-Identifier: AGPL-3.0-only

package fakeplugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"connectrpc.com/connect/v2"

	billingv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/billing/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/billing/v1/billingv1connect"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

type billingServer struct {
	billingv1connect.UnimplementedBillingServiceHandler

	o Options
}

// Fetch implements billingv1connect.BillingServiceHandler.
func (s *billingServer) Fetch(ctx context.Context, req *billingv1.FetchRequest) (*billingv1.FetchResponse, error) {
	switch {
	case s.o.Billing == nil:
		return nil, connect.NewError(connect.CodeUnimplemented, "this fake has no billing data")
	case s.o.Block:
		<-ctx.Done()
		return nil, ctx.Err()
	case s.o.Broken:
		return &billingv1.FetchResponse{}, nil
	}
	return fetchBilling(ctx, s.o.Billing, req.GetMonths(), req.GetMaxRequests())
}

// fetchBilling answers a call from a port's source, as a plugin that holds its providers does.
func fetchBilling(ctx context.Context, src appfinance.BillingSource, months []string, maxRequests int32) (*billingv1.FetchResponse, error) {
	req := appfinance.FetchRequest{MaxRequests: int(maxRequests)}
	for _, m := range months {
		month, err := finance.ParseYearMonth(m)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, "bad month")
		}
		req.Months = append(req.Months, month)
	}
	snap, err := src.Fetch(ctx, req)
	if err != nil && errors.Is(err, context.Canceled) {
		return nil, connect.NewError(connect.CodeCanceled, "canceled")
	}
	res := &billingv1.FetchResponse{
		Outcome:     billingv1.Outcome(billingv1.Outcome_value["OUTCOME_"+outcomeName(err)]),
		Requests:    int32(snap.Requests), //nolint:gosec // a test's small count
		BytesBilled: snap.BytesBilled,
	}
	for _, l := range snap.Lines {
		res.Lines = append(res.Lines, &billingv1.CostLine{
			Month: l.Month.String(), CoveredStart: l.Covered.Start.String(), CoveredEnd: l.Covered.End.String(),
			Service:    l.Service,
			ChargeType: billingv1.ChargeType(billingv1.ChargeType_value["CHARGE_TYPE_"+strings.ToUpper(string(l.ChargeType))]),
			Category:   billingv1.Category(billingv1.Category_value["CATEGORY_"+strings.ToUpper(string(l.Category))]),
			Amount:     l.Amount, Currency: l.Currency, Final: l.Final,
		})
	}
	return res, nil
}

func outcomeName(err error) string {
	if err == nil {
		return "OK"
	}
	return strings.ToUpper(string(appfinance.ImportErrorKindOf(err)))
}

// serveBillingJSON adds the billing kind of the fake of net/http and encoding/json alone.
func serveBillingJSON(mux *http.ServeMux, o Options) {
	if o.Billing == nil {
		return
	}
	mux.HandleFunc("POST "+billingv1connect.BillingServiceFetchProcedure, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "send JSON", http.StatusUnsupportedMediaType)
			return
		}
		// An int32 is a JSON number, and fields at their zero value may be missing.
		var req struct {
			Months      []string `json:"months"`
			MaxRequests int32    `json:"maxRequests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		if o.Block {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if o.Broken {
			_, _ = w.Write([]byte("{}"))
			return
		}
		res, err := fetchBilling(r.Context(), o.Billing, req.Months, req.MaxRequests)
		if err != nil {
			http.Error(w, "failed", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(billingJSON(res))
	})
}

// billingJSON writes a response in proto3's JSON mapping: a decimal string for an int64, a name for an enum, and fields at
// their zero value left out.
func billingJSON(res *billingv1.FetchResponse) map[string]any {
	out := map[string]any{"outcome": res.GetOutcome().String(), "extra": "a field the server does not know, which it ignores"}
	if res.GetRequests() != 0 {
		out["requests"] = res.GetRequests()
	}
	if res.GetBytesBilled() != 0 {
		out["bytesBilled"] = strconv.FormatInt(res.GetBytesBilled(), 10)
	}
	lines := make([]map[string]any, 0, len(res.GetLines()))
	for _, l := range res.GetLines() {
		line := map[string]any{
			"month": l.GetMonth(), "coveredStart": l.GetCoveredStart(), "coveredEnd": l.GetCoveredEnd(), "service": l.GetService(),
			"chargeType": l.GetChargeType().String(), "category": l.GetCategory().String(), "amount": l.GetAmount(), "currency": l.GetCurrency(),
		}
		if l.GetFinal() {
			line["final"] = true
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		out["lines"] = lines
	}
	return out
}
