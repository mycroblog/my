// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

// The appeals of a reject of local.status (ADR-0147 §9). The author sends the text they wanted to
// post, and a person reviews it: no plugin is called. Upholding leaves the decision as it is.
// Overturning cancels the decision and exempts the account from the hook in one transaction, and the
// author posts again: the post is not restored.

// Events of the appeals. The notification module tells the holders of manage_reports of a new
// appeal (admin_hook_appeal) and the author of its review (hook_appeal_decided), and the audit log
// records an upheld appeal (hook_appeal_upheld); an overturned one is recorded as the cancellation
// of its decision (hook_decision_overridden).
const (
	// EventAppealFiled follows a new appeal, in its transaction.
	EventAppealFiled = "hook.appeal_filed"
	// EventAppealDecided follows the review of an appeal, in its transaction.
	EventAppealDecided = "hook.appeal_decided"
)

// AppealFiled carries a new appeal.
type AppealFiled struct{ Appeal hook.Appeal }

// EventType implements uow.Event.
func (AppealFiled) EventType() string { return EventAppealFiled }

// AppealDecided carries an appeal and its review. AuditID identifies the audit log entry of an
// upheld appeal; Label is the target label of that entry, the decision's.
type AppealDecided struct {
	Appeal  hook.Appeal
	Review  hook.AppealDecision
	Label   string
	AuditID string
}

// EventType implements uow.Event.
func (AppealDecided) EventType() string { return EventAppealDecided }

// AppealInput is an author's appeal.
type AppealInput struct {
	// AccountID is the author; a decision on another account is not found.
	AccountID  string
	DecisionID string
	// StatusText is the text of the post the author tried to post.
	StatusText string
	// Comment is what the author adds for the moderators, "" for none.
	Comment string
}

// FileAppeal records an author's appeal of a reject of local.status. It fails with
// ErrAppealNotFound for a decision that is unknown or not the author's, with hook.ErrNotAppealable
// for one that is not a reject in force, with hook.ErrAppealWindowClosed after appeal_window_days,
// with ErrAlreadyAppealed for a second appeal, and with a hook.AppealTextError for its text.
func (s *Service) FileAppeal(ctx context.Context, in AppealInput) (hook.AppealRecord, error) {
	_, window, err := s.d.Settings.HookRetention(ctx)
	if err != nil {
		return hook.AppealRecord{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	a := hook.Appeal{
		ID: s.d.IDs.NewID(), DecisionID: in.DecisionID, AccountID: in.AccountID,
		StatusText: in.StatusText, Comment: strings.TrimSpace(in.Comment), SubmittedAt: now,
	}
	if err := a.Validate(); err != nil {
		return hook.AppealRecord{}, err
	}
	var out hook.AppealRecord
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.d.Repository.RecordTx(ctx, tx, in.DecisionID)
		switch {
		case errors.Is(err, ErrNotFound):
			return ErrAppealNotFound
		case err != nil:
			return err
		case rec.Decision.AccountID != in.AccountID:
			return ErrAppealNotFound
		}
		if err := rec.CheckAppealable(now, window); err != nil {
			return err
		}
		inserted, err := s.d.Repository.InsertAppeal(ctx, tx, a)
		if err != nil {
			return err
		}
		if !inserted {
			return ErrAlreadyAppealed
		}
		out = hook.AppealRecord{Appeal: a, Decision: rec}
		return tx.Emit(ctx, AppealFiled{Appeal: a})
	})
	if err != nil {
		return hook.AppealRecord{}, err
	}
	return out, nil
}

// OwnAppeal returns where an author's decision stands for its author: the decision with its
// appeal, or ErrAppealNotFound when it has none. A decision of another account is not found.
func (s *Service) OwnAppeal(ctx context.Context, accountID, decisionID string) (hook.AppealRecord, error) {
	rec, err := s.d.Repository.AppealOfDecision(ctx, decisionID)
	if err != nil {
		return hook.AppealRecord{}, err
	}
	if rec.Appeal.AccountID != accountID {
		return hook.AppealRecord{}, ErrAppealNotFound
	}
	return rec, nil
}

// OwnAppealByID returns an author's appeal by its ID, for the notification of its review, or
// ErrAppealNotFound when it is gone or not the author's.
func (s *Service) OwnAppealByID(ctx context.Context, accountID, appealID string) (hook.AppealRecord, error) {
	rec, err := s.d.Repository.Appeal(ctx, appealID)
	if err != nil {
		return hook.AppealRecord{}, err
	}
	if rec.Appeal.AccountID != accountID {
		return hook.AppealRecord{}, ErrAppealNotFound
	}
	return rec, nil
}

// AppealDeadline returns the last moment an author can appeal a decision made at decidedAt, with
// the current appeal_window_days.
func (s *Service) AppealDeadline(ctx context.Context, decidedAt time.Time) (time.Time, error) {
	_, window, err := s.d.Settings.HookRetention(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return hook.AppealDeadline(hook.Decision{DecidedAt: decidedAt}, window), nil
}

// Appeal returns an appeal with its decision and review, for a moderator, or ErrAppealNotFound.
func (s *Service) Appeal(ctx context.Context, appealID string) (hook.AppealRecord, error) {
	return s.d.Repository.Appeal(ctx, appealID)
}

// ListAppeals returns the appeals that q selects, newest first.
func (s *Service) ListAppeals(ctx context.Context, q AppealQuery) ([]hook.AppealRecord, error) {
	return s.d.Repository.ListAppeals(ctx, q)
}

// DecideAppeal records a moderator's review of an appeal and tells its author (AppealDecided).
// byUserID is the moderator, "" for the CLI. Upholding writes the audit log entry
// hook_appeal_upheld. Overturning cancels the decision (hook_decision_overrides, with its audit log entry) and
// exempts the account from the hook in the same transaction. A review is made once: the same
// outcome again changes nothing, and the other one fails with ErrAppealDecided. It fails with
// ErrAppealNotFound for an unknown appeal.
func (s *Service) DecideAppeal(ctx context.Context, appealID string, outcome hook.AppealOutcome, byUserID string) (hook.AppealRecord, error) {
	if !outcome.Valid() {
		return hook.AppealRecord{}, fmt.Errorf("hook: appeal outcome %q", outcome)
	}
	var out hook.AppealRecord
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.d.Repository.AppealTx(ctx, tx, appealID)
		if err != nil {
			return err
		}
		if rec.Review != nil {
			if rec.Review.Outcome != outcome {
				return ErrAppealDecided
			}
			out = rec
			return nil
		}
		now := kernel.Normalize(s.d.Clock.Now())
		review := hook.AppealDecision{AppealID: appealID, Outcome: outcome, ByUserID: byUserID, DecidedAt: now}
		inserted, err := s.d.Repository.InsertAppealDecision(ctx, tx, review)
		if err != nil {
			return err
		}
		if !inserted {
			// A concurrent review was recorded first.
			return ErrAppealDecided
		}
		rec.Review = &review
		if outcome == hook.AppealOverturned {
			if rec.Decision.Override == nil {
				if rec.Decision, err = s.OverrideTx(ctx, tx, rec.Decision, byUserID); err != nil {
					return err
				}
			}
			err := s.d.Repository.InsertExemption(ctx, tx, hook.Exemption{
				Hook: rec.Decision.Decision.Hook, AccountID: rec.Appeal.AccountID, ExemptedAt: now,
			})
			if err != nil {
				return err
			}
		}
		out = rec
		return tx.Emit(ctx, AppealDecided{
			Appeal: rec.Appeal, Review: review, Label: decisionLabel(rec.Decision.Decision.Hook, rec.Decision.Decision.SubjectKey),
			AuditID: s.d.IDs.NewID(),
		})
	})
	if err != nil {
		return hook.AppealRecord{}, err
	}
	return out, nil
}
