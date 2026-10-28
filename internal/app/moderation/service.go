// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var (
	// ErrUnsupportedAction reports an action on an account that this module does not issue: a
	// kind that does not apply to the account (login on a remote account), one whose port is
	// missing from Deps, or an action on the instance actor.
	ErrUnsupportedAction = errors.New("moderation: the action is not supported")
	// ErrInvalidReportReference reports an action that names a report that does not exist or
	// is of another account.
	ErrInvalidReportReference = errors.New("moderation: the report does not exist or is of another account")
	// ErrAccountNotFound reports an action or a report on an account that does not exist.
	ErrAccountNotFound = errors.New("moderation: the account does not exist")
	// ErrInvalidStatusReference reports an action on statuses that names a status that does
	// not exist, is deleted, or is of another account.
	ErrInvalidStatusReference = errors.New("moderation: the status does not exist or is of another account")
	// ErrNotLiftable reports the lift of a ban, which is not lifted (FR-703).
	ErrNotLiftable = errors.New("moderation: the action cannot be lifted")
)

// Report is moderation.Report, a report as other modules file it.
type Report = moderation.Report

// Action is moderation.Action, an action on an account as the events carry it.
type Action = moderation.Action

// DomainAction is moderation.DomainAction, an action on a domain as the events carry it.
type DomainAction = moderation.DomainAction

// Effect is moderation.Effect, a restriction in effect as the events carry it.
type Effect = moderation.Effect

// Restrictions that other modules look for in the events.
const (
	// RestrictProfileView is moderation.RestrictProfileView.
	RestrictProfileView = moderation.RestrictProfileView
	// RestrictLogin is moderation.RestrictLogin.
	RestrictLogin = moderation.RestrictLogin
)

// Domain events of the moderation module.
const (
	// EventDomainRestrictionsChanged follows every action on a domain and every lift of one,
	// with the restrictions in effect on the domain afterwards.
	EventDomainRestrictionsChanged = "moderation.domain_restrictions_changed"
	// EventAccountRestrictionsChanged is EventDomainRestrictionsChanged for an account.
	EventAccountRestrictionsChanged = "moderation.account_restrictions_changed"
	// EventReportFiled follows a new report.
	EventReportFiled = "moderation.report_filed"
	// EventReportResolved follows every resolution of a report, the first one and those after
	// a reopening.
	EventReportResolved = "moderation.report_resolved"
	// EventReportReopened follows the reopening of a resolved report.
	EventReportReopened = "moderation.report_reopened"
	// EventDomainActionIssued follows a new action on a domain, and EventDomainActionLifted its
	// lift.
	EventDomainActionIssued = "moderation.domain_action_issued"
	EventDomainActionLifted = "moderation.domain_action_lifted"
	// EventAccountActionLifted follows the lift of an action on an account.
	EventAccountActionLifted = "moderation.account_action_lifted"
)

// ReportReopened carries a reopened report and its reopening.
type ReportReopened struct {
	Report    moderation.Report
	Reopening moderation.ReportReopening
}

// EventType implements uow.Event.
func (ReportReopened) EventType() string { return EventReportReopened }

// DomainActionIssued carries a new action on a domain.
type DomainActionIssued struct{ Action moderation.DomainAction }

// EventType implements uow.Event.
func (DomainActionIssued) EventType() string { return EventDomainActionIssued }

// DomainActionLifted carries an action on a domain and its lift.
type DomainActionLifted struct {
	Action moderation.DomainAction
	Lift   moderation.DomainLift
}

// EventType implements uow.Event.
func (DomainActionLifted) EventType() string { return EventDomainActionLifted }

// AccountActionLifted carries an action on an account and its lift.
type AccountActionLifted struct {
	Action moderation.Action
	Lift   moderation.Lift
}

// EventType implements uow.Event.
func (AccountActionLifted) EventType() string { return EventAccountActionLifted }

// DomainRestrictionsChanged carries the restrictions in effect on one domain (not its parents)
// after an action or a lift: subscribers replace what they keep of the domain with Effects.
type DomainRestrictionsChanged struct {
	Domain  string
	Effects []moderation.Effect
}

// EventType implements uow.Event.
func (DomainRestrictionsChanged) EventType() string { return EventDomainRestrictionsChanged }

// AccountRestrictionsChanged is DomainRestrictionsChanged for an account.
type AccountRestrictionsChanged struct {
	AccountID string
	Effects   []moderation.Effect
}

// EventType implements uow.Event.
func (AccountRestrictionsChanged) EventType() string { return EventAccountRestrictionsChanged }

// ReportFiled carries a new report.
type ReportFiled struct {
	Report moderation.Report
	// FirstOpen reports that no other report of the target was open when the report was filed:
	// the moderators are notified of the first open report of an account only (data-model §4.7).
	FirstOpen bool
}

// EventType implements uow.Event.
func (ReportFiled) EventType() string { return EventReportFiled }

// ReportResolved carries a report and its resolution.
type ReportResolved struct {
	Report     moderation.Report
	Resolution moderation.ReportResolution
}

// EventType implements uow.Event.
func (ReportResolved) EventType() string { return EventReportResolved }

// Accounts is the part of the account module's public API that the moderation module reads.
// *appaccount.Service implements it.
type Accounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	// ExistingIDs returns, in ID order, the accounts among ids that exist.
	ExistingIDs(ctx context.Context, ids []string) ([]string, error)
}

// Statuses is the part of the content module's public API that the actions on statuses use.
// *appcontent.Service implements it.
type Statuses interface {
	// Find returns a status that is not deleted; it fails with appcontent.ErrNotFound.
	Find(ctx context.Context, statusID string) (appcontent.Post, error)
	DeleteByModeration(ctx context.Context, tx uow.Tx, st appcontent.Status, actionID string, at time.Time) error
	MarkSensitiveByModeration(ctx context.Context, tx uow.Tx, statusID, actionID string) error
	ClearSensitivityOf(ctx context.Context, tx uow.Tx, actionID string) error
}

// Lifecycles is the part of the account module's public API that a ban and an overturned
// appeal of one use.
// *appaccount.Service implements it.
type Lifecycles interface {
	ScheduleDeletionByModeration(ctx context.Context, tx uow.Tx, accountID, actionID string, due time.Time) error
	// CancelDeletionByModeration cancels the scheduled deletion of an account (an overturned
	// appeal of a ban) and reports whether one was scheduled; a deleted account fails with
	// appaccount.ErrAlreadyDeleted.
	CancelDeletionByModeration(ctx context.Context, tx uow.Tx, accountID, actionID, eventID string) (bool, error)
}

// Deps are the ports a Service uses.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Accounts   Accounts
	// Statuses carries out the actions on statuses; nil refuses them with ErrUnsupportedAction.
	Statuses Statuses
	// Lifecycles schedules the deletion of a banned account; nil refuses bans with
	// ErrUnsupportedAction.
	Lifecycles Lifecycles
	// AppealWindow reads how long an action can be appealed; nil means
	// moderation.DefaultAppealWindow.
	AppealWindow AppealWindow
	// Retention reads how long the moderation records are kept; nil means
	// DefaultRetentionPeriods.
	Retention RetentionSettings
	// Follows cuts the follows of the remote accounts and domains that are suspended (FR-717);
	// nil cuts none.
	Follows Follows
	// Directory finds the local accounts and the accounts of a suspended domain, and Outbox
	// enqueues the job that cuts their follows; without either, a domain suspension cuts none.
	Directory Directory
	Outbox    queue.Outbox
	// Emails reads the email address of a banned account's user for the block the ban makes of
	// it (AccountActionInput.BlockEmail); nil refuses such bans with ErrCannotBlockEmail.
	Emails Emails
	// CanonicalDomain returns a domain in IDNA A-labels, for the email_domain sign-up blocks
	// and the domains of the addresses they screen; nil only lowers the case.
	CanonicalDomain func(domain string) (string, error)
	Clock           kernel.Clock
	IDs             kernel.IDGenerator
	// Evidence presents the statuses of a report to the moderators (ReportStatuses); nil
	// presents none.
	Evidence Evidence
	// AuditTargets tells which roles, custom emoji, and webhooks named by the audit log still
	// exist (GoneAuditTargets); nil reports none of them gone.
	AuditTargets AuditTargets
}

// AuditTargets looks up the targets of the audit log that other modules own and delete: roles
// (identity), custom emoji (content), and webhooks (webhook). Those modules emit the audit log
// entries, so this module reaches them through this port.
type AuditTargets interface {
	// ExistingAuditTargets returns, in ID order, the targets among ids of kind that exist; kind
	// is moderation.AuditTargetRole, AuditTargetCustomEmoji, AuditTargetCustomEmojiRequest,
	// AuditTargetWebhook, or AuditTargetRelay.
	ExistingAuditTargets(ctx context.Context, kind moderation.AuditTargetKind, ids []string) ([]string, error)
}

// Service runs the moderation module's use cases: actions on domains and accounts, their lifts,
// and the reports received from other servers, and the moderators' lists of them and handling
// of the reports (admin.go).
type Service struct{ d Deps }

// NewService returns a Service on d.
func NewService(d Deps) *Service { return &Service{d: d} }

// DomainActionInput is a moderator's action on a domain.
type DomainActionInput struct {
	// Domain is a canonical host (federation.CanonicalHost); the action covers its subdomains.
	Domain        string
	Kind          moderation.DomainActionKind
	PublicComment string
	PrivateNote   string
	Obfuscate     bool
	// IssuedByUserID is the moderator; "" for the CLI.
	IssuedByUserID string
	// Duration is how long the action lasts, at most moderation.MaxDuration; zero lasts until
	// it is lifted.
	Duration time.Duration
}

// IssueDomainAction records an action on a domain and, in the same transaction, the
// restrictions it imposes on the domain and its subdomains (data-model §5.2), and emits
// DomainRestrictionsChanged and its audit log entry. A suspension also enqueues JobSeverDomain,
// which cuts the follows with the domain (SeverDomain).
func (s *Service) IssueDomainAction(ctx context.Context, in DomainActionInput) (moderation.DomainAction, error) {
	if !moderation.ValidDuration(in.Duration) {
		return moderation.DomainAction{}, fmt.Errorf("%w: duration %s", moderation.ErrInvalidAction, in.Duration)
	}
	now := kernel.Normalize(s.d.Clock.Now())
	a := moderation.DomainAction{
		ID: s.d.IDs.NewID(), Domain: in.Domain, Kind: in.Kind, PublicComment: in.PublicComment,
		PrivateNote: in.PrivateNote, Obfuscate: in.Obfuscate, IssuedByUserID: in.IssuedByUserID,
		ExpiresAt: expiry(now, in.Duration), IssuedAt: now,
	}
	if err := a.Validate(); err != nil {
		return moderation.DomainAction{}, err
	}
	auditID := s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.insertDomainAction(ctx, tx, a, auditID)
	})
	if err != nil {
		return moderation.DomainAction{}, err
	}
	return a, nil
}

// insertDomainAction writes a valid domain action inside tx with what IssueDomainAction
// describes: its restrictions, the events, the severance of a suspension, and its audit log
// entry auditID.
func (s *Service) insertDomainAction(ctx context.Context, tx uow.Tx, a moderation.DomainAction, auditID string) error {
	repo := s.d.Repository
	if err := repo.LockDomainRestrictions(ctx, tx, a.Domain); err != nil {
		return err
	}
	if err := repo.InsertDomainAction(ctx, tx, a); err != nil {
		return err
	}
	for _, r := range a.Rows() {
		if err := repo.InsertDomainRestriction(ctx, tx, r); err != nil {
			return err
		}
	}
	if err := s.emitDomain(ctx, tx, a.Domain, a.IssuedAt); err != nil {
		return err
	}
	if err := s.enqueueDomainSeverance(ctx, tx, a); err != nil {
		return err
	}
	if err := tx.Emit(ctx, DomainActionIssued{Action: a}); err != nil {
		return err
	}
	return s.emitAudit(ctx, tx, moderation.AuditEntry{
		ID: auditID, Action: moderation.AuditDomainActionIssued, TargetKind: moderation.AuditTargetDomain,
		TargetID: a.Domain, TargetLabel: a.Domain, SourceEventID: a.ID, OccurredAt: a.IssuedAt,
	}, a.IssuedByUserID)
}

// LiftDomainAction lifts an action on a domain: its restrictions end, while those of other
// actions on the domain stay, and the lift is audited. Lifting an action that is already lifted
// changes nothing.
func (s *Service) LiftDomainAction(ctx context.Context, actionID, liftedByUserID string) error {
	now := kernel.Normalize(s.d.Clock.Now())
	repo := s.d.Repository
	auditID := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a, err := repo.DomainAction(ctx, tx, actionID)
		if err != nil {
			return err
		}
		if err := repo.LockDomainRestrictions(ctx, tx, a.Domain); err != nil {
			return err
		}
		lift := moderation.DomainLift{ActionID: a.ID, LiftedByUserID: liftedByUserID, LiftedAt: now}
		err = repo.InsertDomainActionLift(ctx, tx, lift)
		if errors.Is(err, ErrAlreadyRecorded) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := repo.DeleteDomainRestrictionsOf(ctx, tx, a.ID); err != nil {
			return err
		}
		if err := s.emitDomain(ctx, tx, a.Domain, now); err != nil {
			return err
		}
		if err := tx.Emit(ctx, DomainActionLifted{Action: a, Lift: lift}); err != nil {
			return err
		}
		// The lift's row is keyed by the action's ID.
		return s.emitAudit(ctx, tx, moderation.AuditEntry{
			ID: auditID, Action: moderation.AuditDomainActionLifted, TargetKind: moderation.AuditTargetDomain,
			TargetID: a.Domain, TargetLabel: a.Domain, SourceEventID: a.ID, OccurredAt: now,
		}, liftedByUserID)
	})
}

func (s *Service) emitDomain(ctx context.Context, tx uow.Tx, domain string, now time.Time) error {
	rows, err := s.d.Repository.DomainRestrictionRows(ctx, tx, domain)
	if err != nil {
		return err
	}
	return tx.Emit(ctx, DomainRestrictionsChanged{Domain: domain, Effects: moderation.Merge(rows, now)})
}

// remoteActionKinds are the kinds of actions on a remote account whose only effect is the
// restrictions they impose (data-model §5.2: login and post mean nothing for a remote account).
var remoteActionKinds = []moderation.ActionKind{moderation.ActionLimit, moderation.ActionSuspend, moderation.ActionForceSensitive}

// localActionKinds are the kinds of actions on a local account: every kind (data-model §5.2).
// Besides their restrictions and the notice to the account, the actions on statuses delete or
// mark the statuses they name, and a ban schedules the deletion of the account.
var localActionKinds = []moderation.ActionKind{
	moderation.ActionWarn, moderation.ActionMarkStatusesSensitive, moderation.ActionDeleteStatuses,
	moderation.ActionForceSensitive, moderation.ActionLimit, moderation.ActionRestrictPosting,
	moderation.ActionRestrictInteraction, moderation.ActionDisableLogin, moderation.ActionSuspend, moderation.ActionBan,
}

// AccountActionInput is a moderator's action on an account.
type AccountActionInput struct {
	TargetAccountID string
	Kind            moderation.ActionKind
	ReasonCategory  moderation.Category
	PublicReason    string
	PrivateNote     string
	// ReportID is the report the action follows from; "" for none. The report must be of the
	// target account; the action resolves it as actioned.
	ReportID string
	// NotifyTarget tells a local target of the action (moderation_notice); it is ignored for a
	// remote account.
	NotifyTarget bool
	// IssuedByUserID is the moderator; "" for the CLI.
	IssuedByUserID string
	// Duration is how long the action lasts, at most moderation.MaxDuration; zero lasts until
	// it is lifted.
	Duration time.Duration
	// StatusIDs are the statuses of the target that an action on statuses (delete_statuses,
	// mark_statuses_sensitive) acts on: from 1 to moderation.MaxActionStatuses of them, and
	// none for the other kinds.
	StatusIDs []string
	// BlockEmail also blocks sign-ups with the email address of a banned account's user
	// (an email_hash block with severity reject, data-model §4.3), audited with the action;
	// only a ban of a local account takes it, and it fails with ErrCannotBlockEmail otherwise.
	BlockEmail bool
}

// IssueAccountAction records an action on an account and, in the same transaction, the
// restrictions it imposes (data-model §5.2) and its other effects, and emits
// AccountRestrictionsChanged and AccountActionIssued, and AccountSuspensionChanged when the
// action (suspend, ban) suspends an account that was not suspended; the suspension of a remote
// account also cuts its follows with the local accounts (severance.go). delete_statuses deletes the
// statuses it names (origin
// moderation), mark_statuses_sensitive shows them as sensitive until it is lifted, and ban
// schedules the deletion of the account moderation.BanDeletionDelay later. An action that
// follows from a report resolves the report as actioned in the same transaction, with the audit
// log entry of the resolution, unless the report is resolved already.
//
// A local account takes every kind, a remote one limit, suspend, and force_sensitive; another
// kind, the instance actor, or a kind whose port is missing from Deps fails with
// ErrUnsupportedAction, and an unknown kind or an invalid field with
// moderation.ErrInvalidAction (moderation.ErrActionNeverExpires for an end on a ban or an
// action on statuses, moderation.ErrInvalidActionStatuses for its statuses). An unknown account
// fails with ErrAccountNotFound, a report that does not exist or is of another account with
// ErrInvalidReportReference, and a status that does not exist, is deleted, or is of another
// account with ErrInvalidStatusReference.
func (s *Service) IssueAccountAction(ctx context.Context, in AccountActionInput) (moderation.Action, error) {
	if !in.Kind.Valid() {
		return moderation.Action{}, fmt.Errorf("%w: kind %q", moderation.ErrInvalidAction, in.Kind)
	}
	if !moderation.ValidDuration(in.Duration) {
		return moderation.Action{}, fmt.Errorf("%w: duration %s", moderation.ErrInvalidAction, in.Duration)
	}
	target, err := s.d.Accounts.FindByID(ctx, in.TargetAccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return moderation.Action{}, fmt.Errorf("%w: %s", ErrAccountNotFound, in.TargetAccountID)
	case err != nil:
		return moderation.Action{}, err
	case target.IsInstanceActor():
		return moderation.Action{}, fmt.Errorf("%w: %s is the instance actor", ErrUnsupportedAction, in.TargetAccountID)
	}
	kinds, where := remoteActionKinds, "remote"
	if target.Account.IsLocal() {
		kinds, where = localActionKinds, "local"
	} else {
		in.NotifyTarget = false
	}
	if !slices.Contains(kinds, in.Kind) || (in.Kind.OnStatuses() && s.d.Statuses == nil) ||
		(in.Kind == moderation.ActionBan && s.d.Lifecycles == nil) {
		return moderation.Action{}, fmt.Errorf("%w: %s on a %s account", ErrUnsupportedAction, in.Kind, where)
	}
	var report moderation.Report
	if in.ReportID != "" {
		r, err := s.d.Repository.FindReport(ctx, in.ReportID)
		report = r
		switch {
		case errors.Is(err, ErrNotFound):
			return moderation.Action{}, fmt.Errorf("%w: no report %s", ErrInvalidReportReference, in.ReportID)
		case err != nil:
			return moderation.Action{}, err
		case r.TargetAccountID != in.TargetAccountID:
			return moderation.Action{}, fmt.Errorf("%w: report %s is of another account", ErrInvalidReportReference, in.ReportID)
		}
	}
	now := kernel.Normalize(s.d.Clock.Now())
	a := moderation.Action{
		ID: s.d.IDs.NewID(), TargetAccountID: in.TargetAccountID, Kind: in.Kind, ReasonCategory: in.ReasonCategory,
		PublicReason: in.PublicReason, PrivateNote: in.PrivateNote, ReportID: in.ReportID, Issuer: moderation.IssuerModerator,
		IssuedByUserID: in.IssuedByUserID, NotifyTarget: in.NotifyTarget, ExpiresAt: expiry(now, in.Duration), IssuedAt: now,
	}
	if len(in.StatusIDs) > 0 {
		a.StatusIDs = slices.Sorted(slices.Values(in.StatusIDs))
	}
	if err := a.Validate(); err != nil {
		return moderation.Action{}, err
	}
	statuses, err := s.actionStatuses(ctx, a)
	if err != nil {
		return moderation.Action{}, err
	}
	var block *moderation.SignupBlock
	if in.BlockEmail {
		if a.Kind != moderation.ActionBan || target.Local == nil {
			return moderation.Action{}, fmt.Errorf("%w: only a ban of a local account blocks its email address", ErrCannotBlockEmail)
		}
		b, err := s.banEmailBlock(ctx, target.Local.UserID, a)
		if err != nil {
			return moderation.Action{}, err
		}
		block = &b
	}
	repo := s.d.Repository
	auditID, resolutionID, resolutionAuditID := s.d.IDs.NewID(), s.d.IDs.NewID(), s.d.IDs.NewID()
	blockAuditID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.LockAccountRestrictions(ctx, tx, a.TargetAccountID); err != nil {
			return err
		}
		before, err := repo.AccountRestrictionRows(ctx, tx, a.TargetAccountID)
		if err != nil {
			return err
		}
		if err := repo.InsertAction(ctx, tx, a); err != nil {
			return err
		}
		for _, r := range a.Rows() {
			if err := repo.InsertAccountRestriction(ctx, tx, r); err != nil {
				return err
			}
		}
		if err := s.carryOut(ctx, tx, a, statuses); err != nil {
			return err
		}
		if err := s.emitAccount(ctx, tx, a.TargetAccountID, now); err != nil {
			return err
		}
		started, err := s.emitSuspension(ctx, tx, a.TargetAccountID, a.ID, before, now)
		if err != nil {
			return err
		}
		if started && !target.Account.IsLocal() && s.severs() {
			if err := s.severAccountIn(ctx, tx, a, now); err != nil {
				return err
			}
		}
		if err := tx.Emit(ctx, AccountActionIssued{Action: a}); err != nil {
			return err
		}
		err = s.emitAudit(ctx, tx, moderation.AuditEntry{
			ID: auditID, Action: moderation.AuditAccountActionIssued, TargetKind: moderation.AuditTargetAccount,
			TargetID: a.TargetAccountID, TargetLabel: actorLabel(target), SourceEventID: a.ID, OccurredAt: now,
		}, in.IssuedByUserID)
		if err != nil {
			return err
		}
		if block != nil {
			if err := s.insertSignupBlock(ctx, tx, *block, blockAuditID); err != nil {
				return err
			}
		}
		if a.ReportID == "" {
			return nil
		}
		return s.resolveByAction(ctx, tx, report, a, actorLabel(target), resolutionID, resolutionAuditID)
	})
	if errors.Is(err, appcontent.ErrAlreadyDeleted) {
		return moderation.Action{}, fmt.Errorf("%w: a status was deleted meanwhile", ErrInvalidStatusReference)
	}
	if err != nil {
		return moderation.Action{}, err
	}
	return a, nil
}

// actionStatuses returns the statuses that an action on statuses names, read before its
// transaction: each must be a status of the target that is not deleted.
func (s *Service) actionStatuses(ctx context.Context, a moderation.Action) ([]appcontent.Status, error) {
	out := make([]appcontent.Status, 0, len(a.StatusIDs))
	for _, id := range a.StatusIDs {
		p, err := s.d.Statuses.Find(ctx, id)
		switch {
		case errors.Is(err, appcontent.ErrNotFound):
			return nil, fmt.Errorf("%w: no status %s", ErrInvalidStatusReference, id)
		case err != nil:
			return nil, err
		case p.Status.AuthorAccountID != a.TargetAccountID:
			return nil, fmt.Errorf("%w: status %s is of another account", ErrInvalidStatusReference, id)
		}
		out = append(out, p.Status)
	}
	return out, nil
}

// carryOut records inside tx the effects of a beyond its restrictions (data-model §5.2): the
// deletion or the sensitivity of its statuses, and the scheduled deletion of a banned account.
func (s *Service) carryOut(ctx context.Context, tx uow.Tx, a moderation.Action, statuses []appcontent.Status) error {
	switch a.Kind {
	case moderation.ActionDeleteStatuses:
		for _, st := range statuses {
			if err := s.d.Statuses.DeleteByModeration(ctx, tx, st, a.ID, a.IssuedAt); err != nil {
				return err
			}
		}
	case moderation.ActionMarkStatusesSensitive:
		for _, st := range statuses {
			if err := s.d.Statuses.MarkSensitiveByModeration(ctx, tx, st.ID, a.ID); err != nil {
				return err
			}
		}
	case moderation.ActionBan:
		return s.d.Lifecycles.ScheduleDeletionByModeration(ctx, tx, a.TargetAccountID, a.ID, a.IssuedAt.Add(moderation.BanDeletionDelay))
	}
	return nil
}

// LiftAccountAction lifts an action on an account: its restrictions end, while those of other
// actions on the account stay, and the statuses that a mark_statuses_sensitive action marked are
// no longer shown as sensitive by it; the statuses that delete_statuses deleted stay deleted.
// Lifting the account's last suspension emits AccountSuspensionChanged. Lifting an action that
// is already lifted changes nothing. A ban fails with ErrNotLiftable. An
// invalid reason fails with an error wrapping moderation.ErrInvalidAction and a
// *moderation.TextError.
func (s *Service) LiftAccountAction(ctx context.Context, actionID, liftedByUserID, reason string) error {
	now := kernel.Normalize(s.d.Clock.Now())
	lift := moderation.Lift{ActionID: actionID, LiftedByUserID: liftedByUserID, Reason: reason, LiftedAt: now}
	if err := lift.Validate(); err != nil {
		return err
	}
	repo := s.d.Repository
	// The target of an action never changes, so its label is read before the transaction.
	rec, err := repo.FindActionRecord(ctx, actionID)
	if err != nil {
		return err
	}
	label, err := s.accountLabel(ctx, rec.Action.TargetAccountID)
	if err != nil {
		return err
	}
	auditID := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a, err := repo.Action(ctx, tx, actionID)
		if err != nil {
			return err
		}
		if !a.Kind.Liftable() {
			return fmt.Errorf("%w: %s %s", ErrNotLiftable, a.Kind, a.ID)
		}
		return s.liftAction(ctx, tx, a, lift, label, auditID)
	})
}

// liftAction records lift of a inside tx with its effects and its audit log entry, as
// LiftAccountAction describes; it records nothing when a is already lifted. label is the
// target's display name for the audit log.
func (s *Service) liftAction(ctx context.Context, tx uow.Tx, a moderation.Action, lift moderation.Lift, label, auditID string) error {
	repo := s.d.Repository
	// The lifts and actions on one account run one after another, so that the restrictions that
	// this lift projects are those that the others committed.
	if err := repo.LockAccountRestrictions(ctx, tx, a.TargetAccountID); err != nil {
		return err
	}
	err := repo.InsertActionLift(ctx, tx, lift)
	if errors.Is(err, ErrAlreadyRecorded) {
		return nil
	}
	if err != nil {
		return err
	}
	before, err := repo.AccountRestrictionRows(ctx, tx, a.TargetAccountID)
	if err != nil {
		return err
	}
	if err := repo.DeleteAccountRestrictionsOf(ctx, tx, a.ID); err != nil {
		return err
	}
	if a.Kind == moderation.ActionMarkStatusesSensitive && s.d.Statuses != nil {
		if err := s.d.Statuses.ClearSensitivityOf(ctx, tx, a.ID); err != nil {
			return err
		}
	}
	if err := s.emitAccount(ctx, tx, a.TargetAccountID, lift.LiftedAt); err != nil {
		return err
	}
	if _, err := s.emitSuspension(ctx, tx, a.TargetAccountID, "", before, lift.LiftedAt); err != nil {
		return err
	}
	if err := tx.Emit(ctx, AccountActionLifted{Action: a, Lift: lift}); err != nil {
		return err
	}
	// The lift's row is keyed by the action's ID.
	return s.emitAudit(ctx, tx, moderation.AuditEntry{
		ID: auditID, Action: moderation.AuditAccountActionLifted, TargetKind: moderation.AuditTargetAccount,
		TargetID: a.TargetAccountID, TargetLabel: label, SourceEventID: a.ID, OccurredAt: lift.LiftedAt,
	}, lift.LiftedByUserID)
}

func (s *Service) emitAccount(ctx context.Context, tx uow.Tx, accountID string, now time.Time) error {
	rows, err := s.d.Repository.AccountRestrictionRows(ctx, tx, accountID)
	if err != nil {
		return err
	}
	return tx.Emit(ctx, AccountRestrictionsChanged{AccountID: accountID, Effects: moderation.Merge(rows, now)})
}

func expiry(now time.Time, d time.Duration) *time.Time {
	if d <= 0 {
		return nil
	}
	t := now.Add(d)
	return &t
}

// RemoteReportInput is a report received from another server in a Flag (federation.md §11).
type RemoteReportInput struct {
	// URI is the id of the Flag (or the URI synthesized for a Flag without one), the key that
	// makes filing the same Flag again do nothing.
	URI string
	// ReporterAccountID is the remote actor of the Flag, often the instance actor of its server.
	ReporterAccountID string
	// TargetAccountID is the local account reported.
	TargetAccountID string
	// StatusIDs are the target's statuses the Flag points at.
	StatusIDs []string
	// Comment is the Flag's content as plain text; it is cut to MaxReportComment characters.
	Comment string
}

// FileRemoteReport records the report of a received Flag, open, in the moderators' queue
// (FR-701), and emits ReportFiled. Filing a Flag whose URI already has a report changes nothing
// and returns that report; created reports whether this call filed it.
func (s *Service) FileRemoteReport(ctx context.Context, in RemoteReportInput) (r moderation.Report, created bool, err error) {
	if in.URI == "" {
		return moderation.Report{}, false, fmt.Errorf("%w: a received report needs a URI", moderation.ErrInvalidReport)
	}
	now := kernel.Normalize(s.d.Clock.Now())
	r = moderation.Report{
		ID: s.d.IDs.NewID(), URI: in.URI, ReporterAccountID: in.ReporterAccountID, TargetAccountID: in.TargetAccountID,
		Category: moderation.CategoryOther, Comment: moderation.TruncateComment(in.Comment), FiledAt: now,
		StatusIDs: uniqueSorted(in.StatusIDs), RuleIDs: []string{},
	}
	if err := r.Validate(); err != nil {
		return moderation.Report{}, false, err
	}
	repo := s.d.Repository
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := repo.InsertReport(ctx, tx, r)
		if err != nil || !ok {
			created = false
			return err
		}
		created = true
		if err := repo.UpsertReportStatus(ctx, tx, moderation.ReportStatus{ReportID: r.ID, State: moderation.ReportOpen, Since: now}); err != nil {
			return err
		}
		return emitReportFiled(ctx, tx, repo, r)
	})
	if err != nil {
		return moderation.Report{}, false, err
	}
	if !created {
		r, err = repo.FindReportByURI(ctx, in.URI)
		return r, false, err
	}
	return r, true, nil
}

// emitReportFiled emits ReportFiled for the report r just filed in tx.
func emitReportFiled(ctx context.Context, tx uow.Tx, repo Repository, r moderation.Report) error {
	other, err := repo.HasOtherOpenReportOf(ctx, tx, r.TargetAccountID, r.ID)
	if err != nil {
		return err
	}
	return tx.Emit(ctx, ReportFiled{Report: r, FirstOpen: !other})
}

// CountOpenReports counts the reports that await the moderators (GET /api/v1/admin/counts).
func (s *Service) CountOpenReports(ctx context.Context) (int64, error) {
	return s.d.Repository.CountReports(ctx, moderation.ReportOpen)
}

// OldestOpenReportFiledAt returns when the open report that has waited longest was filed, and
// nil when no report is open (GET /api/v1/admin/counts).
func (s *Service) OldestOpenReportFiledAt(ctx context.Context) (*time.Time, error) {
	t, err := s.d.Repository.OldestReportFiledAt(ctx, moderation.ReportOpen)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func uniqueSorted(ids []string) []string {
	out := append([]string{}, ids...)
	slices.Sort(out)
	return slices.Compact(out)
}

// Report returns a report with its statuses.
func (s *Service) Report(ctx context.Context, id string) (moderation.Report, error) {
	return s.d.Repository.FindReport(ctx, id)
}

// ReportStatus returns the state of a report.
func (s *Service) ReportStatus(ctx context.Context, id string) (moderation.ReportStatus, error) {
	return s.d.Repository.FindReportStatus(ctx, id)
}

// ReportsOf returns the reports of an account, oldest first.
func (s *Service) ReportsOf(ctx context.Context, targetAccountID string) ([]moderation.Report, error) {
	return s.d.Repository.ListReportsOf(ctx, targetAccountID)
}
