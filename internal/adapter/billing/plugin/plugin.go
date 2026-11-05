// SPDX-License-Identifier: AGPL-3.0-only

// Package plugin is the finance.BillingSource that delegates to an operator's plugin (ADR-0146 §11): one source per
// destination, which aggregates its providers itself. What the port asks with no call, its name, time zone, and method,
// is fixed or comes from the settings, so that the start depends on no call.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	billingv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/billing/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/billing/v1/billingv1connect"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// Source is the finance.BillingSource of a plugin.
type Source struct {
	conn     *client.Conn
	client   billingv1connect.BillingServiceClient
	location *time.Location
}

// New returns the source that reaches its plugin through conn. location is the time zone of the plugin's billing months
// (plugin_billing_location). It makes no call.
func New(conn *client.Conn, location *time.Location) (*Source, error) {
	if location == nil {
		return nil, errors.New("billing plugin: no time zone")
	}
	return &Source{conn: conn, client: billingv1connect.NewBillingServiceClient(conn.RPC), location: location}, nil
}

// Close closes the idle connections to the plugin.
func (s *Source) Close() error {
	s.conn.Close()
	return nil
}

// Name implements finance.BillingSource.
func (s *Source) Name() string { return appfinance.SourcePlugin }

// Location implements finance.BillingSource.
func (s *Source) Location() *time.Location { return s.location }

// Method implements finance.BillingSource.
func (s *Source) Method() string { return appfinance.MethodPlugin }

// Fetch implements finance.BillingSource. No request allowed means no call, so that a plugin that ignores max_requests
// still bills nothing. A transport failure reports no requests, since the server cannot know them. The errors say which
// line is wrong and hold nothing of the plugin's text (ADR-0146 §10).
func (s *Source) Fetch(ctx context.Context, req appfinance.FetchRequest) (appfinance.Snapshot, error) {
	if req.MaxRequests <= 0 {
		return appfinance.Snapshot{}, fmt.Errorf("%w: no request allowed", appfinance.ErrRequestCap)
	}
	months := make([]string, len(req.Months))
	for i, m := range req.Months {
		months[i] = m.String()
	}
	maxRequests := min(req.MaxRequests, math.MaxInt32)
	res, err := s.client.Fetch(ctx, &billingv1.FetchRequest{Months: months, MaxRequests: int32(maxRequests)})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return appfinance.Snapshot{}, fmt.Errorf("billing plugin: %w", ctxErr)
		}
		return appfinance.Snapshot{}, fmt.Errorf("%w: %w", appfinance.ErrBillingUnavailable, err)
	}
	snap := appfinance.Snapshot{Requests: max(0, int(res.GetRequests())), BytesBilled: max(0, res.GetBytesBilled())}
	if failure := outcomeError(res.GetOutcome()); failure != nil {
		return snap, failure
	}
	switch {
	case res.GetRequests() < 0 || res.GetBytesBilled() < 0:
		return snap, fmt.Errorf("%w: negative usage", appfinance.ErrInvalidBillingResponse)
	case int(res.GetRequests()) > maxRequests:
		return snap, fmt.Errorf("%w: %d requests, more than the %d allowed", appfinance.ErrInvalidBillingResponse, res.GetRequests(), maxRequests)
	}
	lines, err := costLines(res.GetLines(), req.Months)
	if err != nil {
		return snap, err
	}
	snap.Lines = lines
	return snap, nil
}

// outcomeError returns the error of an outcome, nil for OUTCOME_OK. An outcome the server does not know, or none, is
// never a success (ADR-0146 §10).
func outcomeError(o billingv1.Outcome) error {
	var failure error
	switch o {
	case billingv1.Outcome_OUTCOME_OK:
		return nil
	case billingv1.Outcome_OUTCOME_PERMISSION:
		failure = appfinance.ErrBillingPermission
	case billingv1.Outcome_OUTCOME_NOT_ENABLED:
		failure = appfinance.ErrBillingNotEnabled
	case billingv1.Outcome_OUTCOME_NOT_FOUND:
		failure = appfinance.ErrBillingNotFound
	case billingv1.Outcome_OUTCOME_THROTTLED:
		failure = appfinance.ErrBillingThrottled
	case billingv1.Outcome_OUTCOME_REQUEST_CAP:
		failure = appfinance.ErrRequestCap
	case billingv1.Outcome_OUTCOME_BYTES_CAP:
		failure = appfinance.ErrBytesCap
	case billingv1.Outcome_OUTCOME_INVALID_RESPONSE:
		failure = appfinance.ErrInvalidBillingResponse
	case billingv1.Outcome_OUTCOME_UNAVAILABLE:
		failure = appfinance.ErrBillingUnavailable
	default:
		return fmt.Errorf("%w: the outcome is %d", appfinance.ErrInvalidBillingResponse, int32(o))
	}
	return fmt.Errorf("%w: the plugin's outcome", failure)
}

var chargeTypes = map[billingv1.ChargeType]finance.ChargeType{
	billingv1.ChargeType_CHARGE_TYPE_USAGE:      finance.ChargeUsage,
	billingv1.ChargeType_CHARGE_TYPE_CREDIT:     finance.ChargeCredit,
	billingv1.ChargeType_CHARGE_TYPE_REFUND:     finance.ChargeRefund,
	billingv1.ChargeType_CHARGE_TYPE_TAX:        finance.ChargeTax,
	billingv1.ChargeType_CHARGE_TYPE_SUPPORT:    finance.ChargeSupport,
	billingv1.ChargeType_CHARGE_TYPE_FEE:        finance.ChargeFee,
	billingv1.ChargeType_CHARGE_TYPE_ADJUSTMENT: finance.ChargeAdjustment,
	billingv1.ChargeType_CHARGE_TYPE_OTHER:      finance.ChargeOther,
}

var categories = map[billingv1.Category]finance.Category{
	billingv1.Category_CATEGORY_COMPUTE:  finance.CategoryCompute,
	billingv1.Category_CATEGORY_DATABASE: finance.CategoryDatabase,
	billingv1.Category_CATEGORY_STORAGE:  finance.CategoryStorage,
	billingv1.Category_CATEGORY_NETWORK:  finance.CategoryNetwork,
	billingv1.Category_CATEGORY_DOMAIN:   finance.CategoryDomain,
	billingv1.Category_CATEGORY_EMAIL:    finance.CategoryEmail,
	billingv1.Category_CATEGORY_SOFTWARE: finance.CategorySoftware,
	billingv1.Category_CATEGORY_OTHER:    finance.CategoryOther,
}

// costLines checks what the plugin returned against the contract of the port, since its output is not trusted
// (ADR-0146 §10): anything else fails the whole response with ErrInvalidBillingResponse.
func costLines(in []*billingv1.CostLine, months []finance.YearMonth) ([]appfinance.CostLine, error) {
	asked := make(map[finance.YearMonth]bool, len(months))
	for _, m := range months {
		asked[m] = true
	}
	type key struct {
		month   finance.YearMonth
		service string
		charge  finance.ChargeType
	}
	seen := make(map[key]bool, len(in))
	out := make([]appfinance.CostLine, 0, len(in))
	for i, l := range in {
		line, why := costLine(l, asked)
		if why != "" {
			return nil, fmt.Errorf("%w: line %d: %s", appfinance.ErrInvalidBillingResponse, i, why)
		}
		k := key{line.Month, line.Service, line.ChargeType}
		if seen[k] {
			return nil, fmt.Errorf("%w: line %d: a month, service, and charge type that came before", appfinance.ErrInvalidBillingResponse, i)
		}
		seen[k] = true
		out = append(out, line)
	}
	return out, nil
}

// costLine returns the line of l, or what is wrong with it.
func costLine(l *billingv1.CostLine, asked map[finance.YearMonth]bool) (appfinance.CostLine, string) {
	month, err := finance.ParseYearMonth(l.GetMonth())
	if err != nil {
		return appfinance.CostLine{}, "the month is not YYYY-MM"
	}
	if !asked[month] {
		return appfinance.CostLine{}, "a month that was not asked for"
	}
	service := l.GetService()
	if service == "" || !utf8.ValidString(service) || utf8.RuneCountInString(service) > appfinance.MaxServiceLength || strings.ContainsFunc(service, unicode.IsControl) {
		return appfinance.CostLine{}, fmt.Sprintf("the service is empty, longer than %d characters, or holds control characters", appfinance.MaxServiceLength)
	}
	charge, ok := chargeTypes[l.GetChargeType()]
	if !ok {
		return appfinance.CostLine{}, "an unknown charge type"
	}
	category, ok := categories[l.GetCategory()]
	if !ok {
		return appfinance.CostLine{}, "an unknown category"
	}
	start, err := finance.ParseDate(l.GetCoveredStart())
	if err != nil {
		return appfinance.CostLine{}, "the first covered day is not YYYY-MM-DD"
	}
	end, err := finance.ParseDate(l.GetCoveredEnd())
	if err != nil {
		return appfinance.CostLine{}, "the last covered day is not YYYY-MM-DD"
	}
	covered, whole := finance.DateRange{Start: start, End: end}, month.Range()
	switch {
	case !covered.Valid() || !whole.Contains(start) || !whole.Contains(end):
		return appfinance.CostLine{}, "the covered days are not a range inside the month"
	case l.GetFinal() && covered != whole:
		return appfinance.CostLine{}, "a final line must cover the whole month"
	}
	currency, err := finance.ParseCurrency(l.GetCurrency())
	if err != nil {
		return appfinance.CostLine{}, "the currency is not an ISO 4217 code"
	}
	if _, err := finance.MinorUnitsOf(l.GetAmount(), currency); err != nil {
		return appfinance.CostLine{}, "the amount is not a decimal in major units"
	}
	return appfinance.CostLine{
		Month: month, Covered: covered, Service: service, ChargeType: charge, Category: category,
		Amount: l.GetAmount(), Currency: l.GetCurrency(), Final: l.GetFinal(),
	}, ""
}
