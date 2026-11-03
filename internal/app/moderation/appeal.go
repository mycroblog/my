// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The appeals (FR-707, ADR-0038, data-model §4.3): a local account appeals an action on it once,
// within the instance's appeal window from the action, and a moderator upholds or overturns the
// appeal. Overturning lifts the action in the same transaction (LiftAccountAction's effects; the
// statuses that delete_statuses deleted stay deleted). A ban, which nothing else lifts (FR-703),
// is appealed while its account awaits deletion (moderation.AppealDeadline), and overturning it
// also cancels that deletion. The account is told of the decision
// (appeal_decided) and the moderators of a new appeal (admin_appeal) by the notification module,
// from the events here.

var (
	// ErrNotAppealable reports an appeal of an action that cannot be appealed any more, or the
	// overturning of one: a ban whose account is deleted.
	ErrNotAppealable = errors.New("moderation: the action cannot be appealed")
	// ErrAppealWindowClosed reports an appeal after the appeal window of the action.
	ErrAppealWindowClosed = errors.New("moderation: the appeal window of the action has closed")
	// ErrAlreadyAppealed reports a second appeal of one action.
	ErrAlreadyAppealed = errors.New("moderation: the action is already appealed")
	// ErrAppealDecided reports a decision on an appeal that was decided otherwise.
	ErrAppealDecided = errors.New("moderation: the appeal is already decided")
)

// Events of the appeals.
const (
	// EventAppealFiled follows a new appeal, in its transaction.
	EventAppealFiled = "moderation.appeal_filed"
	// EventAppealDecided follows the decision on an appeal, in its transaction.
	EventAppealDecided = "moderation.appeal_decided"
)

// AppealFiled carries a new appeal. The notification module turns it into admin_appeal.
type AppealFiled struct{ Appeal moderation.Appeal }

// EventType implements uow.Event.
func (AppealFiled) EventType() string { return EventAppealFiled }

// AppealDecided carries an appeal and its decision. The notification module turns it into the
// account's appeal_decided.
type AppealDecided struct {
	Appeal   moderation.Appeal
	Decision moderation.AppealDecision
}

// EventType implements uow.Event.
func (AppealDecided) EventType() string { return EventAppealDecided }

// AppealWindow reads how long after an action the account may appeal it: the instance setting
// appeal_window_days. The composition root implements it on the instance module.
type AppealWindow interface {
	AppealWindow(ctx context.Context) (time.Duration, error)
}

// AppealRepository is the persistence port of the appeals, part of Repository.
type AppealRepository interface {
	// InsertAppeal fails with ErrAlreadyRecorded when the action already has an appeal.
	InsertAppeal(ctx context.Context, tx uow.Tx, a moderation.Appeal) error
	// InsertAppealDecision fails with ErrAlreadyRecorded when the appeal is already decided.
	InsertAppealDecision(ctx context.Context, tx uow.Tx, d moderation.AppealDecision) error
	UpsertAppealStatus(ctx context.Context, tx uow.Tx, st moderation.AppealStatus) error
	// Appeal returns an appeal with its decision inside tx.
	Appeal(ctx context.Context, tx uow.Tx, id string) (moderation.AppealRecord, error)
	// FindAppeal returns an appeal with its decision.
	FindAppeal(ctx context.Context, id string) (moderation.AppealRecord, error)
	// AppealsOfActions returns the appeals of the actions, with their decisions, in no order.
	AppealsOfActions(ctx context.Context, actionIDs []string) ([]moderation.AppealRecord, error)
	// ListAppeals returns up to limit appeals with their decisions, newest first: those in
	// q.State (any state when empty) whose ID is below q.Before (from the newest when empty).
	ListAppeals(ctx context.Context, q AppealQuery, limit int) ([]moderation.AppealRecord, error)
	// CountAppeals counts the appeals in a state.
	CountAppeals(ctx context.Context, state moderation.AppealState) (int64, error)
}

// AppealQuery selects the appeals of a list.
type AppealQuery struct {
	State  moderation.AppealState // "" for any
	Before string                 // an appeal ID; "" for the first page
}

// OwnAppeal is an appeal as the account that filed it sees it: its text, where it stands, and
// the reason the moderators gave with their decision. It never names the moderator.
type OwnAppeal struct {
	ID          string
	Text        string
	State       moderation.AppealState
	SubmittedAt time.Time
	DecidedAt   *time.Time // nil while pending
	// DecisionReason is the moderators' explanation of the decision; "" for none.
	DecisionReason string
}

func ownAppeal(r moderation.AppealRecord) *OwnAppeal {
	a := &OwnAppeal{ID: r.Appeal.ID, Text: r.Appeal.Text, State: r.State(), SubmittedAt: r.Appeal.SubmittedAt}
	if d := r.Decision; d != nil {
		t := d.DecidedAt
		a.DecidedAt, a.DecisionReason = &t, d.PublicReason
	}
	return a
}

// appealWindow returns the appeal window of the instance.
func (s *Service) appealWindow(ctx context.Context) (time.Duration, error) {
	if s.d.AppealWindow == nil {
		return moderation.DefaultAppealWindow, nil
	}
	return s.d.AppealWindow.AppealWindow(ctx)
}

// targetViews presents the records as the account they are on sees them, with their appeals
// and whether each can be appealed at now.
func (s *Service) targetViews(ctx context.Context, rs []moderation.ActionRecord, now time.Time) ([]TargetActionView, error) {
	out := make([]TargetActionView, len(rs))
	if len(rs) == 0 {
		return out, nil
	}
	window, err := s.appealWindow(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.Action.ID
	}
	appeals, err := s.d.Repository.AppealsOfActions(ctx, ids)
	if err != nil {
		return nil, err
	}
	byAction := make(map[string]moderation.AppealRecord, len(appeals))
	for _, a := range appeals {
		byAction[a.Appeal.ActionID] = a
	}
	deleted := map[string]bool{}
	for i, r := range rs {
		v := targetView(r, now)
		deadline := moderation.AppealDeadline(r.Action, window)
		v.AppealDeadline = &deadline
		v.Appealable = !now.After(deadline)
		if v.Appealable && r.Action.Kind == moderation.ActionBan {
			id := r.Action.TargetAccountID
			if _, ok := deleted[id]; !ok {
				if deleted[id], err = s.accountDeleted(ctx, id); err != nil {
					return nil, err
				}
			}
			v.Appealable = !deleted[id]
		}
		if a, ok := byAction[r.Action.ID]; ok {
			v.Appeal, v.Appealable = ownAppeal(a), false
		}
		out[i] = v
	}
	return out, nil
}

// FileAppeal records accountID's appeal of an action on it, pending, and emits AppealFiled. An
// unknown action or one on another account fails with ErrNotFound, a ban with ErrNotAppealable,
// an action whose appeal window has closed with ErrAppealWindowClosed, a second appeal of the
// action with ErrAlreadyAppealed, and an empty, too long, or invalid text with
// moderation.ErrInvalidAppeal and a *moderation.TextError. It returns the action as the account
// sees it, with the appeal.
func (s *Service) FileAppeal(ctx context.Context, accountID, actionID, text string) (TargetActionView, error) {
	rec, err := s.d.Repository.FindActionRecord(ctx, actionID)
	if err != nil {
		return TargetActionView{}, err
	}
	a := rec.Action
	if a.TargetAccountID != accountID {
		return TargetActionView{}, fmt.Errorf("%w: action %s is on another account", ErrNotFound, actionID)
	}
	if a.Kind == moderation.ActionBan {
		gone, err := s.accountDeleted(ctx, accountID)
		if err != nil {
			return TargetActionView{}, err
		}
		if gone {
			return TargetActionView{}, fmt.Errorf("%w: the account of ban %s is deleted", ErrNotAppealable, a.ID)
		}
	}
	now := kernel.Normalize(s.d.Clock.Now())
	window, err := s.appealWindow(ctx)
	if err != nil {
		return TargetActionView{}, err
	}
	if now.After(moderation.AppealDeadline(a, window)) {
		return TargetActionView{}, fmt.Errorf("%w: action %s", ErrAppealWindowClosed, a.ID)
	}
	appeal := moderation.Appeal{ID: s.d.IDs.NewID(), ActionID: a.ID, AccountID: accountID, Text: text, SubmittedAt: now}
	if err := appeal.Validate(); err != nil {
		return TargetActionView{}, err
	}
	repo := s.d.Repository
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.InsertAppeal(ctx, tx, appeal); err != nil {
			return err
		}
		if err := repo.UpsertAppealStatus(ctx, tx, moderation.AppealStatus{AppealID: appeal.ID, State: moderation.AppealPending, Since: now}); err != nil {
			return err
		}
		return tx.Emit(ctx, AppealFiled{Appeal: appeal})
	})
	if errors.Is(err, ErrAlreadyRecorded) {
		return TargetActionView{}, fmt.Errorf("%w: action %s", ErrAlreadyAppealed, a.ID)
	}
	if err != nil {
		return TargetActionView{}, err
	}
	return s.OwnAction(ctx, accountID, actionID)
}

// accountDeleted reports whether an account is deleted, or gone.
func (s *Service) accountDeleted(ctx context.Context, accountID string) (bool, error) {
	a, err := s.d.Accounts.FindByID(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return true, nil
	case err != nil:
		return false, err
	}
	return a.Deleted(), nil
}

// OwnAppealAction returns the action that accountID's appeal appealID appeals, as the account
// sees it; an unknown appeal, or one of another account, fails with ErrNotFound.
func (s *Service) OwnAppealAction(ctx context.Context, accountID, appealID string) (TargetActionView, error) {
	r, err := s.d.Repository.FindAppeal(ctx, appealID)
	if err != nil {
		return TargetActionView{}, err
	}
	if r.Appeal.AccountID != accountID {
		return TargetActionView{}, fmt.Errorf("%w: appeal %s is of another account", ErrNotFound, appealID)
	}
	return s.OwnAction(ctx, accountID, r.Appeal.ActionID)
}

// AppealView is an appeal with its decision and the action it appeals, as the moderators see
// them.
type AppealView struct {
	moderation.AppealRecord
	Action AccountActionView
}

func (s *Service) appealView(ctx context.Context, r moderation.AppealRecord) (AppealView, error) {
	a, err := s.AccountAction(ctx, r.Appeal.ActionID)
	if err != nil {
		return AppealView{}, fmt.Errorf("moderation: the action of appeal %s: %w", r.Appeal.ID, err)
	}
	return AppealView{AppealRecord: r, Action: a}, nil
}

// Appeal returns an appeal with its decision and action; it fails with ErrNotFound for an
// unknown ID.
func (s *Service) Appeal(ctx context.Context, id string) (AppealView, error) {
	r, err := s.d.Repository.FindAppeal(ctx, id)
	if err != nil {
		return AppealView{}, err
	}
	return s.appealView(ctx, r)
}

// AppealFilter selects the appeals that Appeals lists.
type AppealFilter struct {
	// State keeps the appeals in one state; "" keeps every state.
	State moderation.AppealState
	// Before is the ID of the last appeal of the previous page; "" starts from the newest.
	Before string
}

// Appeals lists up to limit appeals with their decisions and actions, newest first. It fails
// with ErrInvalidQuery for an unknown state.
func (s *Service) Appeals(ctx context.Context, f AppealFilter, limit int) ([]AppealView, error) {
	if f.State != "" && !f.State.Valid() {
		return nil, fmt.Errorf("%w: appeal state %q", ErrInvalidQuery, f.State)
	}
	rs, err := s.d.Repository.ListAppeals(ctx, AppealQuery(f), limit)
	if err != nil {
		return nil, err
	}
	out := make([]AppealView, len(rs))
	for i, r := range rs {
		if out[i], err = s.appealView(ctx, r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CountPendingAppeals counts the appeals that await the moderators (GET /api/v1/admin/counts).
func (s *Service) CountPendingAppeals(ctx context.Context) (int64, error) {
	return s.d.Repository.CountAppeals(ctx, moderation.AppealPending)
}

// DecideAppeal records a moderator's decision on a pending appeal, its state, and its audit log
// entry in one transaction, and emits AppealDecided. Overturning also lifts the action in that
// transaction, with LiftAccountAction's effects and audit log entry, a ban included; an action
// lifted meanwhile stays as it is. Overturning a ban first cancels the deletion of its account
// (Lifecycles.CancelDeletionByModeration, audited as account_deletion_canceled); a ban whose
// account is deleted fails with ErrNotAppealable, and without Lifecycles with
// ErrUnsupportedAction. Deciding a decided appeal the same way again records nothing; deciding it
// otherwise fails with ErrAppealDecided. An unknown appeal fails with ErrNotFound, and an
// unknown decision or an invalid reason with moderation.ErrInvalidAppeal (with a
// *moderation.TextError for the reason). byUserID is the moderator, "" for the CLI, who fails with
// ErrTargetOutranksActor when not ranking above the account's user.
func (s *Service) DecideAppeal(ctx context.Context, appealID string, decision moderation.AppealDecisionKind, publicReason, byUserID string) (AppealView, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	d := moderation.AppealDecision{AppealID: appealID, Decision: decision, PublicReason: publicReason, DecidedByUserID: byUserID, DecidedAt: now}
	if err := d.Validate(); err != nil {
		return AppealView{}, err
	}
	repo := s.d.Repository
	// An appeal and its action never change once filed, so they are read before the transaction.
	rec, err := repo.FindAppeal(ctx, appealID)
	if err != nil {
		return AppealView{}, err
	}
	switch target, err := s.d.Accounts.FindByID(ctx, rec.Appeal.AccountID); {
	case errors.Is(err, appaccount.ErrNotFound):
	case err != nil:
		return AppealView{}, err
	default:
		if err := s.checkRank(ctx, byUserID, target); err != nil {
			return AppealView{}, err
		}
	}
	label, err := s.accountLabel(ctx, rec.Appeal.AccountID)
	if err != nil {
		return AppealView{}, err
	}
	// The IDs follow the order of the records: the canceled deletion, the lift, the decision.
	cancelID, cancelAuditID, liftAuditID, auditID := s.d.IDs.NewID(), s.d.IDs.NewID(), s.d.IDs.NewID(), s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		err := repo.InsertAppealDecision(ctx, tx, d)
		if errors.Is(err, ErrAlreadyRecorded) {
			cur, err := repo.Appeal(ctx, tx, appealID)
			switch {
			case err != nil:
				return err
			case cur.Decision != nil && cur.Decision.Decision == decision:
				return nil
			}
			return fmt.Errorf("%w: appeal %s is %s", ErrAppealDecided, appealID, cur.State())
		}
		if err != nil {
			return err
		}
		if err := repo.UpsertAppealStatus(ctx, tx, moderation.AppealStatus{AppealID: appealID, State: decision.State(), Since: now}); err != nil {
			return err
		}
		if decision == moderation.AppealDecisionOverturned {
			a, err := repo.Action(ctx, tx, rec.Appeal.ActionID)
			if err != nil {
				return err
			}
			if a.Kind == moderation.ActionBan {
				if err := s.cancelBanDeletion(ctx, tx, a, label, cancelID, cancelAuditID, byUserID); err != nil {
					return err
				}
			}
			lift := moderation.Lift{ActionID: a.ID, LiftedByUserID: byUserID, LiftedAt: now}
			if err := s.liftAction(ctx, tx, a, lift, label, liftAuditID); err != nil {
				return err
			}
		}
		if err := tx.Emit(ctx, AppealDecided{Appeal: rec.Appeal, Decision: d}); err != nil {
			return err
		}
		action := moderation.AuditAppealUpheld
		if decision == moderation.AppealDecisionOverturned {
			action = moderation.AuditAppealOverturned
		}
		// The decision's row is keyed by the appeal's ID.
		return s.emitAudit(ctx, tx, moderation.AuditEntry{
			ID: auditID, Action: action, TargetKind: moderation.AuditTargetAppeal,
			TargetID: appealID, TargetLabel: label, SourceEventID: appealID, OccurredAt: now,
		}, byUserID)
	})
	if err != nil {
		return AppealView{}, err
	}
	return s.Appeal(ctx, appealID)
}

// cancelBanDeletion cancels inside tx the deletion that ban a scheduled for its account, with
// the audit log entry of the cancellation; an account whose deletion is not scheduled any more
// is left as it is.
func (s *Service) cancelBanDeletion(ctx context.Context, tx uow.Tx, a moderation.Action, label, eventID, auditID, byUserID string) error {
	if s.d.Lifecycles == nil {
		return fmt.Errorf("%w: overturning ban %s", ErrUnsupportedAction, a.ID)
	}
	canceled, err := s.d.Lifecycles.CancelDeletionByModeration(ctx, tx, a.TargetAccountID, a.ID, eventID)
	if errors.Is(err, appaccount.ErrAlreadyDeleted) {
		return fmt.Errorf("%w: the account of ban %s is deleted", ErrNotAppealable, a.ID)
	}
	if err != nil || !canceled {
		return err
	}
	return s.emitAudit(ctx, tx, moderation.AuditEntry{
		ID: auditID, Action: moderation.AuditAccountDeletionCanceled, TargetKind: moderation.AuditTargetAccount,
		TargetID: a.TargetAccountID, TargetLabel: label, SourceEventID: eventID, OccurredAt: kernel.Normalize(s.d.Clock.Now()),
	}, byUserID)
}
