// SPDX-License-Identifier: AGPL-3.0-only

// Package finance holds the use cases of the instance's costs (ADR-0067, FR-901, FR-902): the
// cost items that are recorded with their schedule, label, prices, and end, their cancellation,
// and the totals by week, month, and year that the domain computes from them on every read. The
// adapters of Repository live in internal/adapter/db/*; their contract test suite is
// testkit.FinanceRepositoryContract.
package finance

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// ErrNotFound reports an unknown or canceled cost item, or an income movement that is not
// recorded.
var ErrNotFound = errors.New("finance: not found")

// LabelRevision is one revision of a cost item's label (cost_item_label_revisions).
type LabelRevision struct {
	ID                 string
	CostItemID         string
	Label              finance.Label
	ProviderNormalized string
	RevisedByUserID    string // "" for the CLI
	RevisedAt          time.Time
}

// PriceRevision is one revision of a cost item's price (cost_item_price_revisions).
type PriceRevision struct {
	CostItemID      string
	Revision        finance.PriceRevision
	RevisedByUserID string // "" for the CLI
}

// EndChange is a change of a cost item's end (cost_item_end_changes).
type EndChange struct {
	ID              string
	CostItemID      string
	EndsOn          *finance.Date // nil for no end
	ChangedByUserID string        // "" for the CLI
	ChangedAt       time.Time
}

// Cancellation is the cancellation of a cost item (cost_item_cancellations).
type Cancellation struct {
	ID               string
	CostItemID       string
	CanceledByUserID string // "" for the CLI
	CanceledAt       time.Time
}

// Head is the current label and end of a cost item that is not canceled (cost_item_heads).
type Head struct {
	CostItemID      string
	LabelRevisionID string
	EndsOn          *finance.Date // nil for no end
}

// Current is the head of a cost item with its start date and current name, read in the
// transaction of a change.
type Current struct {
	Head     Head
	StartsOn finance.Date
	// Name is the name of the current label, which labels the change in the audit log.
	Name string
}

// Repository is the persistence port of the finance module.
type Repository interface {
	// InsertItem inserts the cost_items row of item (its ID, recurrence, start, creator, and
	// creation time); the label revision, price revision, and head that complete it are inserted
	// in the same transaction.
	InsertItem(ctx context.Context, tx uow.Tx, item finance.CostItem) error
	InsertLabelRevision(ctx context.Context, tx uow.Tx, r LabelRevision) error
	InsertPriceRevision(ctx context.Context, tx uow.Tx, r PriceRevision) error
	InsertEndChange(ctx context.Context, tx uow.Tx, c EndChange) error
	InsertCancellation(ctx context.Context, tx uow.Tx, c Cancellation) error
	// UpsertHead sets the head of a cost item, and DeleteHead removes it (the projectors of
	// CostItemChanged and CostItemCanceled).
	UpsertHead(ctx context.Context, tx uow.Tx, h Head) error
	DeleteHead(ctx context.Context, tx uow.Tx, costItemID string) error
	// CurrentForUpdate returns the head and start of a cost item in tx, locking the head on the
	// databases that lock rows; it fails with ErrNotFound for an unknown or canceled item.
	CurrentForUpdate(ctx context.Context, tx uow.Tx, costItemID string) (Current, error)
	// FindItem returns a cost item that is not canceled, with its current label and end and every
	// price revision; it fails with ErrNotFound otherwise.
	FindItem(ctx context.Context, id string) (finance.CostItem, error)
	// ListItems returns up to limit cost items that are not canceled, newest first, before the
	// item before ("" for the first page).
	ListItems(ctx context.Context, before string, limit int) ([]finance.CostItem, error)
	// AllItems returns every cost item that is not canceled, newest first: what the totals read.
	AllItems(ctx context.Context) ([]finance.CostItem, error)

	ImportRepository
	IncomeRepository
	ProjectionRepository
}

// Run is one attempt of a billing import (billing_import_runs): a success when ErrorKind is
// ImportErrorNone.
type Run struct {
	ID     string
	Source string
	// RunKey is the idempotency key of the attempt's job, which its retries share.
	RunKey      string
	ErrorKind   ImportErrorKind
	Requests    int
	BytesBilled int64
	OccurredAt  time.Time
}

// Reservation is the billed requests that an attempt of a billing import reserved before it
// called the provider (billing_import_reservations): ID is the attempt's Run.ID.
type Reservation struct {
	ID         string
	Source     string
	RunKey     string
	Requests   int
	ReservedAt time.Time
}

// Succeeded reports whether the attempt recorded what the source reported.
func (r Run) Succeeded() bool { return r.ErrorKind == ImportErrorNone }

// ImportedKey identifies the cost of a source's billing month, service, and charge type.
type ImportedKey struct {
	Source     string
	Month      finance.YearMonth
	Service    string
	ChargeType finance.ChargeType
}

// ImportedRevision is one revision of an imported cost (imported_cost_revisions): its amount in
// minor units spread over Covered, and its kind.
type ImportedRevision struct {
	ID       string
	Key      ImportedKey
	Category finance.Category
	Covered  finance.DateRange
	Amount   finance.Money
	Kind     finance.RevisionKind
	// RunID is the attempt that recorded it, and RunKey the key of its job.
	RunID      string
	RunKey     string
	RecordedAt time.Time
}

// ImportRepository is the part of Repository that records the billing imports (ADR-0068 §5).
type ImportRepository interface {
	// InsertRun records an attempt.
	InsertRun(ctx context.Context, tx uow.Tx, r Run) error
	// SumRuns returns the requests and the bytes billed of the attempts of source that occurred
	// in [from, until), counting the reservation of an attempt that is not recorded yet in place
	// of its requests.
	SumRuns(ctx context.Context, source string, from, until time.Time) (requests int, bytesBilled int64, err error)
	// SumRunsTx is SumRuns in tx.
	SumRunsTx(ctx context.Context, tx uow.Tx, source string, from, until time.Time) (requests int, bytesBilled int64, err error)
	// LockSource makes the transactions that reserve requests for source run one at a time: it
	// locks the source in tx on the databases that run write transactions at the same time.
	LockSource(ctx context.Context, tx uow.Tx, source string) error
	// InsertReservation records the requests that an attempt reserved before it calls the
	// provider. Until the attempt (the Run of the same ID) is recorded, SumRuns counts the
	// reservation in place of its requests.
	InsertReservation(ctx context.Context, tx uow.Tx, r Reservation) error
	// ImportRunning reports in tx whether an attempt of source reserved requests at or after
	// since and is not recorded yet.
	ImportRunning(ctx context.Context, tx uow.Tx, source string, since time.Time) (bool, error)
	// RunKeySucceeded reports in tx whether an attempt of source with runKey succeeded.
	RunKeySucceeded(ctx context.Context, tx uow.Tx, source, runKey string) (bool, error)
	// RunKeyUsed reports whether an attempt of source recorded runKey.
	RunKeyUsed(ctx context.Context, source, runKey string) (bool, error)
	// LastRun returns the newest attempt of source, or its newest success when succeeded is
	// true; it fails with ErrNotFound when there is none.
	LastRun(ctx context.Context, source string, succeeded bool) (Run, error)
	// ImportedHeads returns the newest revision of every key, of source unless it is "" and of
	// months unless it is empty, ordered by source, month, service, and charge type.
	ImportedHeads(ctx context.Context, source string, months []finance.YearMonth) ([]ImportedRevision, error)
	// ImportedHeadsForUpdate returns the newest revision of every key of source and months in tx,
	// locking the heads on the databases that lock rows.
	ImportedHeadsForUpdate(ctx context.Context, tx uow.Tx, source string, months []finance.YearMonth) ([]ImportedRevision, error)
	// InsertImportedRevision records a revision and reports whether it did: an attempt of the
	// same job (RunKey) that already wrote the key writes nothing.
	InsertImportedRevision(ctx context.Context, tx uow.Tx, r ImportedRevision) (bool, error)
	// UpsertImportedHead makes r the newest revision of its key (imported_cost_heads).
	UpsertImportedHead(ctx context.Context, tx uow.Tx, r ImportedRevision) error
}

// ProjectionRepository is the part of Repository that rebuilds the finance projection
// (data-model §9).
type ProjectionRepository interface {
	// RebuildHeads empties cost_item_heads and derives it from the cost items that are not
	// canceled, with the newest label revision and end change of each.
	RebuildHeads(ctx context.Context, tx uow.Tx) error
	// RebuildImportedHeads empties imported_cost_heads and derives it from the newest revision
	// of each key.
	RebuildImportedHeads(ctx context.Context, tx uow.Tx) error
}
