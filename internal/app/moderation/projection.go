// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// ProjectionRepository is the part of Repository that rebuilds the moderation projections and
// replays the restrictions for the projections of other modules (data-model §9).
type ProjectionRepository interface {
	// ListUnliftedActions returns the actions on accounts without a lift, expired ones
	// included, by ID, with their ID, target, kind, and end only.
	ListUnliftedActions(ctx context.Context, tx uow.Tx) ([]moderation.Action, error)
	// ListUnliftedDomainActions is ListUnliftedActions for the actions on domains, with their
	// ID, domain, kind, and end only.
	ListUnliftedDomainActions(ctx context.Context, tx uow.Tx) ([]moderation.DomainAction, error)
	// ListUnliftedActionStatuses returns the statuses of the actions of kind without a lift,
	// by action and status.
	ListUnliftedActionStatuses(ctx context.Context, tx uow.Tx, kind moderation.ActionKind) ([]ActionStatus, error)
	// ListAllAccountRestrictionRows returns every row of account_restrictions.
	ListAllAccountRestrictionRows(ctx context.Context, tx uow.Tx) ([]moderation.RestrictionRow, error)
	// ListAllDomainRestrictionRows returns every row of domain_restrictions.
	ListAllDomainRestrictionRows(ctx context.Context, tx uow.Tx) ([]moderation.RestrictionRow, error)
	DeleteAllAccountRestrictions(ctx context.Context, tx uow.Tx) error
	DeleteAllDomainRestrictions(ctx context.Context, tx uow.Tx) error
	// RebuildReportStates empties report_states and derives it again from the reports, their
	// assignments, resolutions, and reopenings.
	RebuildReportStates(ctx context.Context, tx uow.Tx) error
	// RebuildAppealStates empties appeal_states and derives it again from the appeals and their
	// decisions.
	RebuildAppealStates(ctx context.Context, tx uow.Tx) error
}

// ActionStatus is a status that an action on statuses names (moderation_action_statuses).
type ActionStatus struct {
	ActionID string
	StatusID string
}

// RebuildProjections empties account_restrictions, domain_restrictions, report_states, and
// appeal_states and derives them again, in one transaction, from the actions and their lifts,
// the reports and their events, and the appeals and their decisions: the rows that the commands
// write (data-model §9).
//
// An action that is not lifted imposes its restrictions until it expires. An expired row
// restricts nothing, and the job moderation.expire_restrictions removes it and tells what expired
// (ExpireAccountRestrictions, ExpireDomainRestrictions), which no event table records; so the
// rebuild keeps the expired rows that the tables hold and does not bring back those that the job
// removed.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository, clock kernel.Clock) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		now := kernel.Normalize(clock.Now())
		if err := rebuildAccountRestrictions(ctx, tx, repo, now); err != nil {
			return err
		}
		if err := rebuildDomainRestrictions(ctx, tx, repo, now); err != nil {
			return err
		}
		if err := repo.RebuildReportStates(ctx, tx); err != nil {
			return err
		}
		return repo.RebuildAppealStates(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("rebuild the moderation projections: %w", err)
	}
	return nil
}

func rebuildAccountRestrictions(ctx context.Context, tx uow.Tx, repo Repository, now time.Time) error {
	held, err := repo.ListAllAccountRestrictionRows(ctx, tx)
	if err != nil {
		return err
	}
	if err := repo.DeleteAllAccountRestrictions(ctx, tx); err != nil {
		return err
	}
	actions, err := repo.ListUnliftedActions(ctx, tx)
	if err != nil {
		return err
	}
	kept := heldRows(held)
	for _, a := range actions {
		for _, r := range a.Rows() {
			if !moderation.InEffect(r.EffectiveUntil, now) && !kept[rowKey(r)] {
				continue
			}
			if err := repo.InsertAccountRestriction(ctx, tx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func rebuildDomainRestrictions(ctx context.Context, tx uow.Tx, repo Repository, now time.Time) error {
	held, err := repo.ListAllDomainRestrictionRows(ctx, tx)
	if err != nil {
		return err
	}
	if err := repo.DeleteAllDomainRestrictions(ctx, tx); err != nil {
		return err
	}
	actions, err := repo.ListUnliftedDomainActions(ctx, tx)
	if err != nil {
		return err
	}
	kept := heldRows(held)
	for _, a := range actions {
		for _, r := range a.Rows() {
			if !moderation.InEffect(r.EffectiveUntil, now) && !kept[rowKey(r)] {
				continue
			}
			if err := repo.InsertDomainRestriction(ctx, tx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

// heldRows returns the keys of rows.
func heldRows(rows []moderation.RestrictionRow) map[string]bool {
	kept := make(map[string]bool, len(rows))
	for _, r := range rows {
		kept[rowKey(r)] = true
	}
	return kept
}

// rowKey identifies a restriction row by all its columns.
func rowKey(r moderation.RestrictionRow) string {
	until := "-"
	if r.EffectiveUntil != nil {
		until = strconv.FormatInt(r.EffectiveUntil.UnixMilli(), 10)
	}
	return strings.Join([]string{r.Target, string(r.Restriction), r.SourceActionID, until}, "\x00")
}

// ReplayEvents emits inside tx, for the rebuild of the projections that other modules derive
// from this module's events (data-model §9), the restrictions in effect at now:
// AccountRestrictionsChanged for each account and DomainRestrictionsChanged for each domain that
// an action without a lift names, in ID order, as the commands emit them after an action or a
// lift.
func ReplayEvents(ctx context.Context, tx uow.Tx, repo Repository, now time.Time, emit func(uow.Event) error) error {
	actions, err := repo.ListUnliftedActions(ctx, tx)
	if err != nil {
		return err
	}
	byAccount := map[string][]moderation.RestrictionRow{}
	for _, a := range actions {
		byAccount[a.TargetAccountID] = append(byAccount[a.TargetAccountID], a.Rows()...)
	}
	for _, id := range slices.Sorted(maps.Keys(byAccount)) {
		if err := emit(AccountRestrictionsChanged{AccountID: id, Effects: moderation.Merge(byAccount[id], now)}); err != nil {
			return err
		}
	}
	domainActions, err := repo.ListUnliftedDomainActions(ctx, tx)
	if err != nil {
		return err
	}
	byDomain := map[string][]moderation.RestrictionRow{}
	for _, a := range domainActions {
		byDomain[a.Domain] = append(byDomain[a.Domain], a.Rows()...)
	}
	for _, d := range slices.Sorted(maps.Keys(byDomain)) {
		if err := emit(DomainRestrictionsChanged{Domain: d, Effects: moderation.Merge(byDomain[d], now)}); err != nil {
			return err
		}
	}
	return nil
}

// ReplaySensitivityMarks calls mark inside tx for each status that a mark_statuses_sensitive
// action without a lift names, with the action: the statuses that the action shows as sensitive
// (Statuses.MarkSensitiveByModeration), for the rebuild of status_sensitivity_overrides.
func ReplaySensitivityMarks(ctx context.Context, tx uow.Tx, repo Repository, mark func(statusID, actionID string) error) error {
	marks, err := repo.ListUnliftedActionStatuses(ctx, tx, moderation.ActionMarkStatusesSensitive)
	if err != nil {
		return err
	}
	for _, m := range marks {
		if err := mark(m.StatusID, m.ActionID); err != nil {
			return err
		}
	}
	return nil
}
