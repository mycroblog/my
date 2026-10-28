// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var (
	// ErrNotFound reports that no row matches a lookup.
	ErrNotFound = errors.New("moderation: not found")
	// ErrAlreadyRecorded reports a second lift of one action.
	ErrAlreadyRecorded = errors.New("moderation: the action is already lifted")
)

// Repository is the persistence port of the moderation module: the actions on accounts and
// domains, their lifts, the restrictions they impose, and reports (data-model §4.3). The
// database adapters implement it and pass testkit.ModerationContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	TxReads

	// Events and resources, written by commands.

	InsertAction(ctx context.Context, tx uow.Tx, a moderation.Action) error
	// Action returns an action on an account by ID.
	Action(ctx context.Context, tx uow.Tx, id string) (moderation.Action, error)
	// InsertActionLift fails with ErrAlreadyRecorded when the action is already lifted.
	InsertActionLift(ctx context.Context, tx uow.Tx, l moderation.Lift) error
	InsertDomainAction(ctx context.Context, tx uow.Tx, a moderation.DomainAction) error
	// DomainAction returns an action on a domain by ID.
	DomainAction(ctx context.Context, tx uow.Tx, id string) (moderation.DomainAction, error)
	// InsertDomainActionLift fails with ErrAlreadyRecorded when the action is already lifted.
	InsertDomainActionLift(ctx context.Context, tx uow.Tx, l moderation.DomainLift) error
	// InsertReport inserts a report with its statuses and reports whether it did: a report
	// whose URI another report has inserts nothing.
	InsertReport(ctx context.Context, tx uow.Tx, r moderation.Report) (bool, error)

	// Projections, written only by this module's commands.

	// LockAccountRestrictions serializes inside tx the commands that change the restrictions of
	// an account: a second transaction waits until the first ends, so that the restrictions it
	// reads and projects are the committed ones (SELECT ... FOR UPDATE or an advisory lock where
	// the database needs it).
	LockAccountRestrictions(ctx context.Context, tx uow.Tx, accountID string) error
	// LockDomainRestrictions is LockAccountRestrictions for the restrictions of a domain.
	LockDomainRestrictions(ctx context.Context, tx uow.Tx, domain string) error
	InsertAccountRestriction(ctx context.Context, tx uow.Tx, r moderation.RestrictionRow) error
	// DeleteAccountRestrictionsOf deletes the rows that an action on an account imposes.
	DeleteAccountRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error
	// AccountRestrictionRows returns every row of an account, expired ones included.
	AccountRestrictionRows(ctx context.Context, tx uow.Tx, accountID string) ([]moderation.RestrictionRow, error)
	// DeleteExpiredAccountRestrictions deletes at most limit rows of any account that have
	// expired at now, the earliest expired first, and returns them.
	DeleteExpiredAccountRestrictions(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]moderation.RestrictionRow, error)
	InsertDomainRestriction(ctx context.Context, tx uow.Tx, r moderation.RestrictionRow) error
	// DeleteDomainRestrictionsOf deletes the rows that an action on a domain imposes.
	DeleteDomainRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error
	// DomainRestrictionRows returns every row of one domain (not its parents), expired ones
	// included.
	DomainRestrictionRows(ctx context.Context, tx uow.Tx, domain string) ([]moderation.RestrictionRow, error)
	UpsertReportStatus(ctx context.Context, tx uow.Tx, s moderation.ReportStatus) error
	InsertReportAssignment(ctx context.Context, tx uow.Tx, a moderation.ReportAssignment) error
	InsertReportResolution(ctx context.Context, tx uow.Tx, r moderation.ReportResolution) error
	// ReportStatusForUpdate returns the state of a report inside the transaction that changes
	// it, locking it where the database can.
	ReportStatusForUpdate(ctx context.Context, tx uow.Tx, reportID string) (moderation.ReportStatus, error)
	// HasOtherOpenReportOf reports inside tx whether a report of the target other than reportID
	// is open.
	HasOtherOpenReportOf(ctx context.Context, tx uow.Tx, targetAccountID, reportID string) (bool, error)

	// Lookups.

	// AccountRestrictionsInEffect returns the distinct restrictions of an account in effect at
	// now, in restriction order.
	AccountRestrictionsInEffect(ctx context.Context, accountID string, now time.Time) ([]moderation.Restriction, error)
	// DomainRestrictionsInEffect returns the distinct restrictions in effect at now of any of
	// the domains, in restriction order.
	DomainRestrictionsInEffect(ctx context.Context, domains []string, now time.Time) ([]moderation.Restriction, error)
	// FindReport returns a report with its statuses.
	FindReport(ctx context.Context, id string) (moderation.Report, error)
	// FindReportByURI returns the report of a received Flag with its statuses.
	FindReportByURI(ctx context.Context, uri string) (moderation.Report, error)
	FindReportStatus(ctx context.Context, reportID string) (moderation.ReportStatus, error)
	// CountReports counts the reports in a state.
	CountReports(ctx context.Context, state moderation.ReportState) (int64, error)
	// OldestReportFiledAt returns when the oldest report in a state was filed; it fails with
	// ErrNotFound when no report is in the state.
	OldestReportFiledAt(ctx context.Context, state moderation.ReportState) (time.Time, error)
	// ListReportsOf returns the reports of a target account with their statuses, oldest first.
	ListReportsOf(ctx context.Context, targetAccountID string) ([]moderation.Report, error)
	// ListReports returns up to limit reports with their statuses and states that q selects,
	// newest first (oldest first when q.OldestFirst): those past the report q.After in that
	// order (from the first when empty).
	ListReports(ctx context.Context, q ReportQuery, limit int) ([]moderation.ReportRecord, error)
	// ListReportsByReporter returns up to limit reports that reporterAccountID filed, with their
	// statuses and states, newest first: those whose ID is below before (from the newest when
	// empty).
	ListReportsByReporter(ctx context.Context, reporterAccountID, before string, limit int) ([]moderation.ReportRecord, error)
	// FindActionRecord returns an action on an account with its lift.
	FindActionRecord(ctx context.Context, id string) (moderation.ActionRecord, error)
	// ListActions returns up to limit actions on accounts with their lifts, newest first: those
	// on q.AccountID (every account when empty), only those in effect at q.InEffectAt when it
	// is set, whose ID is below q.Before (from the newest when empty).
	ListActions(ctx context.Context, q ActionQuery, limit int) ([]moderation.ActionRecord, error)
	// FindDomainActionRecord returns a domain action with its lift.
	FindDomainActionRecord(ctx context.Context, id string) (moderation.DomainActionRecord, error)
	// ListDomainActions returns up to limit domain actions with their lifts, newest first: those
	// of q.Domain (every domain when empty), of q.Kinds (every kind when empty), only those in
	// effect at q.InEffectAt when it is set, whose ID is below q.Before (from the newest when
	// empty).
	ListDomainActions(ctx context.Context, q DomainActionQuery, limit int) ([]moderation.DomainActionRecord, error)

	// The audit log (audit.go).
	AuditRepository
	// The appeals (appeal.go).
	AppealRepository
	// Notes on reports and their reopenings (report_notes.go).
	ReportNoteRepository
	// The follows that suspensions cut (severance.go).
	SeveranceRepository
	// The retention of the records and the holds (retention.go).
	RetentionRepository
	// The account management (account_admin.go).
	AccountAdminRepository
	// The sign-up block list (signup_block.go).
	SignupBlockRepository
	// The rebuild of the projections (projection.go).
	ProjectionRepository
}

// ReportQuery selects the reports of a list.
type ReportQuery struct {
	State          moderation.ReportState // "" for any
	Category       moderation.Category    // "" for any
	AssigneeUserID string                 // the user the reports are assigned to; "" for anyone
	Unassigned     bool                   // only the reports that nobody handles
	OldestFirst    bool                   // the oldest report first rather than the newest
	After          string                 // the last report ID of the previous page; "" for the first page
}

// ActionQuery selects the actions on accounts of a list.
type ActionQuery struct {
	AccountID  string     // "" for every account
	InEffectAt *time.Time // nil for every action, ended ones included
	Before     string     // an action ID; "" for the first page
}

// DomainActionQuery selects the domain actions of a list.
type DomainActionQuery struct {
	Domain     string                        // a canonical host; "" for every domain
	Kinds      []moderation.DomainActionKind // the kinds to keep; empty for every kind
	InEffectAt *time.Time                    // nil for every action, ended ones included
	Before     string                        // an action ID; "" for the first page
}

// TxReads are the reads of Repository that run in a transaction tx, with what tx has written, for the
// use cases that present what an event's transaction wrote (the data of the webhook messages,
// ADR-0060). Each one reads as the method of the same name without Tx does.
type TxReads interface {
	AccountRestrictionsInEffectTx(ctx context.Context, tx uow.Tx, accountID string, now time.Time) ([]moderation.Restriction, error)
	DomainRestrictionsInEffectTx(ctx context.Context, tx uow.Tx, domains []string, now time.Time) ([]moderation.Restriction, error)
	FindReportTx(ctx context.Context, tx uow.Tx, id string) (moderation.Report, error)
	FindReportStatusTx(ctx context.Context, tx uow.Tx, reportID string) (moderation.ReportStatus, error)
	FindActionRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.ActionRecord, error)
	FindDomainActionRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.DomainActionRecord, error)
	FindAppealTx(ctx context.Context, tx uow.Tx, id string) (moderation.AppealRecord, error)
}
