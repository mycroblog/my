// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The use cases of a local account's own actions (FR-715, NFR-MOD-6): the account sees every
// action on it, in effect or ended, with what the moderators chose to tell it, and never the
// private note, the report, or who filed the report or issued the action (ADR-0058).

// EventAccountActionIssued follows every action on an account, in its transaction.
const EventAccountActionIssued = "moderation.account_action_issued"

// AccountActionIssued carries a new action on an account. The notification module turns one
// with NotifyTarget into the account's moderation_notice (data-model §4.7).
type AccountActionIssued struct{ Action moderation.Action }

// EventType implements uow.Event.
func (AccountActionIssued) EventType() string { return EventAccountActionIssued }

// TargetActionView is an action on an account as the account itself sees it: its kind, the
// reason meant for it, the statuses it acted on, whether it is in effect now, and its appeal
// (appeal.go). It has no private note, report, issuer, or lift reason.
type TargetActionView struct {
	ID             string
	Kind           moderation.ActionKind
	ReasonCategory moderation.Category
	PublicReason   string // "" for none
	// StatusIDs are the statuses that an action on statuses acted on; empty for an action on
	// the whole account.
	StatusIDs []string
	ExpiresAt *time.Time // nil: until lifted
	IssuedAt  time.Time
	LiftedAt  *time.Time // nil unless lifted
	State     moderation.ActionState
	// AppealDeadline is the last moment the action can be appealed (moderation.AppealDeadline);
	// it is set for every action.
	AppealDeadline *time.Time
	// Appealable reports whether the account can appeal the action now: no appeal yet, the
	// deadline not passed, and for a ban the account not deleted.
	Appealable bool
	// Appeal is the account's appeal of the action; nil for none.
	Appeal *OwnAppeal
}

func targetView(r moderation.ActionRecord, now time.Time) TargetActionView {
	a := r.Action
	v := TargetActionView{
		ID: a.ID, Kind: a.Kind, ReasonCategory: a.ReasonCategory, PublicReason: a.PublicReason,
		StatusIDs: append([]string{}, a.StatusIDs...), ExpiresAt: a.ExpiresAt, IssuedAt: a.IssuedAt, State: r.State(now),
	}
	if r.Lift != nil {
		t := r.Lift.LiftedAt
		v.LiftedAt = &t
	}
	return v
}

// OwnActionFilter selects the actions that OwnActions lists.
type OwnActionFilter struct {
	// Active keeps the actions in effect now: neither lifted nor expired.
	Active bool
	// Before is the ID of the last action of the previous page; "" starts from the newest.
	Before string
}

// OwnActions lists up to limit actions on accountID, newest first, as the account sees them.
// It lists the actions issued without notifying the account too: their restrictions apply to
// it all the same.
func (s *Service) OwnActions(ctx context.Context, accountID string, f OwnActionFilter, limit int) ([]TargetActionView, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	q := ActionQuery{AccountID: accountID, Before: f.Before}
	if f.Active {
		q.InEffectAt = &now
	}
	rs, err := s.d.Repository.ListActions(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	return s.targetViews(ctx, rs, now)
}

// OwnAction returns an action on accountID as the account sees it; an unknown action, or one
// on another account, fails with ErrNotFound.
func (s *Service) OwnAction(ctx context.Context, accountID, actionID string) (TargetActionView, error) {
	r, err := s.d.Repository.FindActionRecord(ctx, actionID)
	if err != nil {
		return TargetActionView{}, err
	}
	if r.Action.TargetAccountID != accountID {
		return TargetActionView{}, fmt.Errorf("%w: action %s is on another account", ErrNotFound, actionID)
	}
	vs, err := s.targetViews(ctx, []moderation.ActionRecord{r}, kernel.Normalize(s.d.Clock.Now()))
	if err != nil {
		return TargetActionView{}, err
	}
	return vs[0], nil
}
