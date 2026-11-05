// SPDX-License-Identifier: AGPL-3.0-only

// Package hook holds the use cases of the decision hooks (ADR-0147): asking a plugin about one
// subject before it is accepted, recording the answer, and the list, the cancellation, and the
// retention of the records.
//
// A module that owns a subject sees only Gate. It calls Decide before its transaction, carries the
// Result in, acts on Result.Effect with its own code, and calls LinkStatus and Applied in that
// transaction. A verdict never does anything here: this module owns no post, follow, or account.
//
// The adapters of Plugin live in internal/adapter/hook and pass testkit.HookPluginContract; those
// of Repository in internal/adapter/db/* and pass testkit.HookRepositoryContract.
package hook

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

// The types of the domain that other modules read, which may not import it.
type (
	// Name is hook.Name.
	Name = hook.Name
	// Outcome is hook.Outcome.
	Outcome = hook.Outcome
	// Record is hook.Record.
	Record = hook.Record
)

// The hooks.
const (
	InboundStatus = hook.InboundStatus
	InboundFollow = hook.InboundFollow
	SignupCheck   = hook.SignupCheck
	MediaCheck    = hook.MediaCheck
	LocalStatus   = hook.LocalStatus
)

// The effects a module acts on.
const (
	Allow           = hook.Allow
	Drop            = hook.Drop
	Flag            = hook.Flag
	MarkSensitive   = hook.MarkSensitive
	RequireApproval = hook.RequireApproval
	Reject          = hook.Reject
)

// Request is what a hook asks a plugin. Subject is the hook's own description of the subject, nil
// for the contract's probes; the adapter of Plugin knows each hook's type and sends what it holds.
type Request struct {
	Hook       hook.Name
	SubjectKey string
	Subject    any
}

// Answer is what a plugin returned. Outcome is checked against the hook's spec afterwards.
type Answer struct {
	Verdict hook.Outcome
	Reason  hook.Reason
	Note    string
}

// The failures of a Plugin. They carry no text of the plugin's.
var (
	// ErrPluginTimeout is a call past its deadline.
	ErrPluginTimeout = errors.New("hook: the plugin timed out")
	// ErrPluginFailed is a call that failed in any other way.
	ErrPluginFailed = errors.New("hook: the plugin failed")
	// ErrPluginInvalid is an answer outside the protocol's vocabulary, an empty one included.
	ErrPluginInvalid = errors.New("hook: the plugin's answer is not valid")
)

// Plugin is the port of one hook's plugin. Check runs outside any transaction and does not retry.
type Plugin interface {
	Check(ctx context.Context, r Request) (Answer, error)
}

// Settings reads the instance settings of the hooks at each use, so that a change applies at once.
// The composition root implements it on the instance module.
type Settings interface {
	HookMode(ctx context.Context, n hook.Name) (hook.Mode, error)
	HookOnFailure(ctx context.Context, n hook.Name) (hook.OnFailure, error)
	// HookRetention returns how long decisions stay, and how long an author may appeal.
	HookRetention(ctx context.Context) (decisions, appealWindow time.Duration, err error)
}

// HeldAccounts lists the accounts that a retention hold preserves; *moderation.Service implements it.
type HeldAccounts interface {
	HeldAccountIDs(ctx context.Context) ([]string, error)
}

// Statuses tells which statuses still have a row. A decision that points only at gone statuses can
// be removed, since the rebuild of the projection drops those statuses anyway (ADR-0147 §8).
type Statuses interface {
	ExistingStatusIDs(ctx context.Context, ids []string) ([]string, error)
}

// ListQuery filters the list of decisions. Without an Outcome the list leaves out Allow.
type ListQuery struct {
	Hook    hook.Name
	Mode    hook.Mode
	Outcome hook.Outcome
	// Before is the ID of the last decision of the previous page; "" starts at the newest.
	Before string
	Limit  int
}

// SummaryRow counts the decisions of one hook, mode, and outcome.
type SummaryRow struct {
	Hook         hook.Name
	Mode         hook.Mode
	Outcome      hook.Outcome
	Decisions    int64
	TotalLatency time.Duration
	MaxLatency   time.Duration
}

// ExpiredQuery reads one batch of the decisions past their retention, oldest first.
type ExpiredQuery struct {
	Before time.Time
	// AfterAt and AfterID are the position of the last batch; "" starts at the oldest.
	AfterAt time.Time
	AfterID string
	Limit   int
}

// Repository is the persistence port of the module.
type Repository interface {
	// InsertDecision records d and reports whether it did: false when the hook already has a
	// decision on the subject, which stands.
	InsertDecision(ctx context.Context, tx uow.Tx, d hook.Decision) (bool, error)
	// RecordBySubject returns the decision of a hook on a subject with its cancellation, outside
	// any transaction; it fails with ErrNotFound.
	RecordBySubject(ctx context.Context, n hook.Name, subjectKey string) (hook.Record, error)
	// RecordTx returns a decision with its cancellation inside tx; it fails with ErrNotFound.
	RecordTx(ctx context.Context, tx uow.Tx, id string) (hook.Record, error)
	// InsertDecisionStatus records that the decision had an effect on the status; a second
	// record does nothing.
	InsertDecisionStatus(ctx context.Context, tx uow.Tx, decisionID, statusID string) error
	// InsertOverride records a cancellation and reports whether it did: false when the decision
	// is already cancelled.
	InsertOverride(ctx context.Context, tx uow.Tx, o hook.Override) (bool, error)
	// ListRecords returns the decisions that q selects with their cancellations, newest first.
	ListRecords(ctx context.Context, q ListQuery) ([]hook.Record, error)
	// Summarize counts the decisions made at or after since.
	Summarize(ctx context.Context, since time.Time) ([]SummaryRow, error)
	// ListExpired returns one batch with the statuses of each decision in StatusIDs.
	ListExpired(ctx context.Context, q ExpiredQuery) ([]hook.Expired, error)
	// DecisionIDsOfAccount returns up to limit decisions of an account inside tx.
	DecisionIDsOfAccount(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]string, error)
	// DeleteDecisions deletes the decisions with their statuses and cancellations.
	DeleteDecisions(ctx context.Context, tx uow.Tx, ids []string) error
}

// ErrNotFound reports an unknown decision.
var ErrNotFound = errors.New("hook: decision not found")

// ErrNotOverridable reports a decision that a moderator cannot cancel from the list.
var ErrNotOverridable = errors.New("hook: the decision cannot be overridden")

// ErrAlreadyOverridden reports a decision that is already cancelled.
var ErrAlreadyOverridden = errors.New("hook: the decision is already overridden")

// ErrRetryLater reports a failed call of a hook whose failure behaviour is retry: no decision was
// recorded, the attempt counts in the metrics and the job's log, and the job's next run calls the
// plugin again. The caller returns it from the job.
var ErrRetryLater = errors.New("hook: the plugin failed; the job runs again")

// Call is one question of a module to a hook.
type Call struct {
	Hook hook.Name
	// SubjectKey identifies the subject for the hook; a job that runs again passes the same one.
	SubjectKey string
	// AccountID is the account the subject belongs to, "" when there is none. Holds and account
	// erasures go by it.
	AccountID string
	// Subject is the hook's description of the subject, for the plugin.
	Subject any
}

// Result is what Decide returns, which the module carries into its transaction.
type Result struct {
	// DecisionID is "" when no decision exists: the hook is not configured, or its mode is off.
	DecisionID string
	Hook       hook.Name
	SubjectKey string
	// Effect is what the module acts on: Allow unless the decision is in force.
	Effect hook.Outcome
	// Reason is the reason of the verdict, for a report the module files; its Note is the comment.
	Reason hook.Reason
	Note   string
}

// Gate is all that the modules that own a subject see of this module.
type Gate interface {
	// Configured reports whether the hook has a plugin. A module may skip building a costly
	// Subject when it has none.
	Configured(n hook.Name) bool
	// Decide returns the decision on c, calling the plugin only if none is recorded. Call it
	// before the transaction. A hook with no plugin costs one map lookup. It fails with
	// ErrRetryLater as above.
	Decide(ctx context.Context, c Call) (Result, error)
	// LinkStatus records inside tx that the decision had an effect on a status, the source of the
	// rebuild of status_sensitivity_overrides.
	LinkStatus(ctx context.Context, tx uow.Tx, decisionID, statusID string) error
	// Applied records inside tx that the module acted on r: an audit log entry with the system as
	// the actor when the effect leaves state, and nothing otherwise (ADR-0147 §11).
	Applied(ctx context.Context, tx uow.Tx, r Result) error
}

// Domain events of the module.
const (
	// EventDecisionApplied follows an automated effect that left state, in its transaction.
	EventDecisionApplied = "hook.decision_applied"
	// EventDecisionOverridden follows a moderator's cancellation, in its transaction.
	EventDecisionOverridden = "hook.decision_overridden"
	// EventFailing follows the opening of a hook's circuit breaker.
	EventFailing = "hook.failing"
)

// DecisionApplied says that a module acted on a decision and left state. AuditID identifies the
// audit log entry; Label is the target label of that entry.
type DecisionApplied struct {
	DecisionID string
	Label      string
	AuditID    string
	At         time.Time
}

// EventType implements uow.Event.
func (DecisionApplied) EventType() string { return EventDecisionApplied }

// DecisionOverridden says that a moderator cancelled a decision. ByUserID is "" for the CLI.
type DecisionOverridden struct {
	DecisionID string
	Label      string
	ByUserID   string
	AuditID    string
	At         time.Time
}

// EventType implements uow.Event.
func (DecisionOverridden) EventType() string { return EventDecisionOverridden }

// Failing says that calls of a hook keep failing, which the moderators are told once a day.
type Failing struct {
	Hook hook.Name
	At   time.Time
}

// EventType implements uow.Event.
func (Failing) EventType() string { return EventFailing }
