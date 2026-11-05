// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/domain/finance"
)

// The import of a cloud provider's usage charges (ADR-0068, FR-904). A BillingSource adapter
// (internal/adapter/billing/*) returns the charges of the billing months it is asked for; the
// scheduled job JobImportBilling records the charges that changed as revisions, and the totals
// spread the newest revision of each key over the days it covers. The contract test suite of
// the adapters is testkit.BillingSourceContract.

// BillingSource is the port of a provider's billing data (ADR-0068 §1). Its types are of the
// standard library and the finance domain only.
type BillingSource interface {
	// Name is the source's identifier: "aws", "gcp", or "plugin" (billing_sources).
	Name() string
	// Location is the time zone of the source's billing periods (UTC, America/Los_Angeles).
	Location() *time.Location
	// Method is how the source reads the provider's data: MethodCostExplorer, MethodBigQuery,
	// MethodBudget, or MethodPlugin.
	Method() string
	// Fetch returns the costs of the given billing months as the provider reports them now. It
	// makes at most req.MaxRequests billed requests and fails with ErrRequestCap rather than make
	// one more, keeping none of the lines it read. The Snapshot carries the requests made and the
	// bytes billed even when Fetch fails. The errors are the ones of ImportErrorKindOf.
	Fetch(ctx context.Context, req FetchRequest) (Snapshot, error)
}

// The methods of the billing sources (ADR-0068 §1, §7), which the screens of the imports show.
const (
	// MethodCostExplorer is the AWS Cost Explorer API, by service and charge type.
	MethodCostExplorer = "cost_explorer"
	// MethodBigQuery is the Cloud Billing export to BigQuery, by service and charge type.
	MethodBigQuery = "bigquery"
	// MethodBudget is the notifications of a Cloud Billing budget: one estimated total per
	// budget and month, without a breakdown by service.
	MethodBudget = "budget"
	// MethodPlugin is an operator's plugin, which aggregates its providers itself (ADR-0146 §11).
	MethodPlugin = "plugin"
)

// SourcePlugin is the name of the billing source of an operator's plugin: one per destination
// (ADR-0146 §11).
const SourcePlugin = "plugin"

// FetchRequest asks a BillingSource for the costs of Months, oldest first, at most 2, in the
// source's Location, with at most MaxRequests billed requests (0 forbids any).
type FetchRequest struct {
	Months      []finance.YearMonth
	MaxRequests int
}

// Snapshot is what a BillingSource reports: one line per (Month, Service, ChargeType), the billed
// requests made (Cost Explorer pages, BigQuery queries, a plugin's report), and the bytes billed
// (BigQuery's or a plugin's report, 0 for the other sources). A key that it does not return has no cost in its month.
type Snapshot struct {
	Lines       []CostLine
	Requests    int
	BytesBilled int64
}

// CostLine is the cost of a billing month, service, and charge type, with the offsetting lines,
// credits, and refunds of the key summed by the adapter (ADR-0067 §5).
type CostLine struct {
	// Month is the provider's billing month.
	Month finance.YearMonth
	// Covered is the days the amount covers, in Location, within Month: the whole month once it
	// is final, from its first day to the last day with data before.
	Covered finance.DateRange
	// Service is the provider's service name, such as "Amazon Simple Storage Service".
	Service    string
	ChargeType finance.ChargeType
	// Category is the ADR-0067 category that the adapter's table of known services gives,
	// finance.CategoryOther for the unknown ones and the taxes.
	Category finance.Category
	// Amount is a decimal in major units, unrounded, and may be negative.
	Amount string
	// Currency is an ISO 4217 code.
	Currency string
	// Final reports that the provider, or the rule of the source (ClosingDay), treats the month
	// as closed.
	Final bool
}

// Rules of the billing imports (ADR-0068 §2, §3). They are provisional until the check of real
// accounts (#882) and are not settings.
const (
	// RefetchPreviousMonthThrough is the last day of a month, in the source's Location, whose
	// imports fetch the previous month again whatever its state; after it, they fetch the previous
	// month only while it is not final.
	RefetchPreviousMonthThrough = 15
	// ClosingDay is the day of month M+1, in the source's Location, from which a source without a
	// mark of its own (Google Cloud) reports month M as final (ClosedByRule).
	ClosingDay = 5
)

// RefetchMonths returns the billing months that an import on today (in the source's Location)
// fetches, oldest first: the current month, and the previous one through
// RefetchPreviousMonthThrough or while previousFinal is false.
func RefetchMonths(today finance.Date, previousFinal bool) []finance.YearMonth {
	current := finance.YearMonthOf(today)
	if _, _, day := today.Date(); day <= RefetchPreviousMonthThrough || !previousFinal {
		return []finance.YearMonth{current.AddMonths(-1), current}
	}
	return []finance.YearMonth{current}
}

// ClosedByRule reports whether a source without a mark of its own treats month as final on today
// (in its Location): from ClosingDay of the next month on.
func ClosedByRule(month finance.YearMonth, today finance.Date) bool {
	next := month.AddMonths(1)
	closing := next.Range().Start.AddDays(ClosingDay - 1)
	return !today.Before(closing)
}

// ImportErrorKind classifies a failed attempt of an import (billing_import_runs.error_kind,
// ADR-0068 §9).
type ImportErrorKind string

// Kinds of failure, in the order of the schema's CHECK.
const (
	ImportErrorNone            ImportErrorKind = "none"
	ImportErrorPermission      ImportErrorKind = "permission"
	ImportErrorNotEnabled      ImportErrorKind = "not_enabled"
	ImportErrorNotFound        ImportErrorKind = "not_found"
	ImportErrorThrottled       ImportErrorKind = "throttled"
	ImportErrorRequestCap      ImportErrorKind = "request_cap"
	ImportErrorBytesCap        ImportErrorKind = "bytes_cap"
	ImportErrorInvalidResponse ImportErrorKind = "invalid_response"
	ImportErrorUnavailable     ImportErrorKind = "unavailable"
)

// ImportErrorKinds returns every kind of failure, ImportErrorNone first.
func ImportErrorKinds() []ImportErrorKind {
	return []ImportErrorKind{
		ImportErrorNone, ImportErrorPermission, ImportErrorNotEnabled, ImportErrorNotFound, ImportErrorThrottled,
		ImportErrorRequestCap, ImportErrorBytesCap, ImportErrorInvalidResponse, ImportErrorUnavailable,
	}
}

// Retried reports whether the job retries an attempt that failed so (ADR-0004 backoff):
// throttling and unavailability pass; the other failures are configuration or permission errors
// that the next scheduled import tries again.
func (k ImportErrorKind) Retried() bool {
	return k == ImportErrorThrottled || k == ImportErrorUnavailable
}

// The errors of BillingSource.Fetch. Adapters wrap the provider's error in one of them;
// ErrRequestCap is for FetchRequest.MaxRequests.
var (
	// ErrBillingPermission: the credentials may not read the billing data (AccessDeniedException,
	// 403).
	ErrBillingPermission = errors.New("finance: the billing data is not readable with these credentials")
	// ErrBillingNotEnabled: the provider's billing data is not enabled (DataUnavailableException,
	// no Linked Account Access).
	ErrBillingNotEnabled = errors.New("finance: the billing data is not enabled")
	// ErrBillingNotFound: the dataset, table, or subscription does not exist (404).
	ErrBillingNotFound = errors.New("finance: the billing data is not found")
	// ErrBillingThrottled: the provider throttles the requests (LimitExceededException, 429).
	ErrBillingThrottled = errors.New("finance: the billing requests are throttled")
	// ErrRequestCap: the call would make more billed requests than FetchRequest.MaxRequests.
	ErrRequestCap = errors.New("finance: the cap of billed requests is reached")
	// ErrBytesCap: the query would scan more than its maximum bytes billed.
	ErrBytesCap = errors.New("finance: the cap of billed bytes is reached")
	// ErrInvalidBillingResponse: the provider's answer is not usable.
	ErrInvalidBillingResponse = errors.New("finance: invalid billing response")
	// ErrBillingUnavailable: the provider did not answer (5xx, network).
	ErrBillingUnavailable = errors.New("finance: the billing data is unavailable")
)

var importErrors = []struct {
	err  error
	kind ImportErrorKind
}{
	{ErrBillingPermission, ImportErrorPermission},
	{ErrBillingNotEnabled, ImportErrorNotEnabled},
	{ErrBillingNotFound, ImportErrorNotFound},
	{ErrBillingThrottled, ImportErrorThrottled},
	{ErrRequestCap, ImportErrorRequestCap},
	{ErrBytesCap, ImportErrorBytesCap},
	{ErrInvalidBillingResponse, ImportErrorInvalidResponse},
	{ErrBillingUnavailable, ImportErrorUnavailable},
}

// ImportErrorKindOf classifies an error of BillingSource.Fetch: ImportErrorNone for nil, the kind
// of the error it wraps, and ImportErrorUnavailable for any other (a canceled context included),
// which the job retries.
func ImportErrorKindOf(err error) ImportErrorKind {
	if err == nil {
		return ImportErrorNone
	}
	for _, e := range importErrors {
		if errors.Is(err, e.err) {
			return e.kind
		}
	}
	return ImportErrorUnavailable
}
