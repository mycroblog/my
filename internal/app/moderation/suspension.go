// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// EventAccountSuspensionChanged follows every change of whether an account is suspended (the
// federation restriction of suspend and ban): an action that suspends it, and the lift or the
// expiry of its last suspension (moderation.SuspensionChange), of a local account and of a
// remote one alike. The federation module sends the Update of a local actor from it
// (federation.md §11.1, ADR-0058); when it starts the suspension of a remote account, the same
// transaction cuts the follows with the account (FR-717: relationship_severances.action_id is
// ActionID, severance.go).
const EventAccountSuspensionChanged = "moderation.account_suspension_changed"

// AccountSuspensionChanged carries an account's suspension after a change: Suspended is true
// when an action suspended the account, false when its last suspension ended. ActionID is the
// action that suspended the account when Suspended is true, and "" otherwise. ChangedAt is the
// time of the action, the lift, or the cleanup that removed the expired suspension.
type AccountSuspensionChanged struct {
	AccountID string
	Suspended bool
	ActionID  string
	ChangedAt time.Time
}

// EventType implements uow.Event.
func (AccountSuspensionChanged) EventType() string { return EventAccountSuspensionChanged }

// emitSuspension emits AccountSuspensionChanged in tx when the change from the rows before to
// the account's rows now changes its suspension, and reports whether the account became
// suspended. actionID is the action issued in tx, "" for a lift.
func (s *Service) emitSuspension(ctx context.Context, tx uow.Tx, accountID, actionID string, before []moderation.RestrictionRow, now time.Time) (started bool, err error) {
	after, err := s.d.Repository.AccountRestrictionRows(ctx, tx, accountID)
	if err != nil {
		return false, err
	}
	changed, suspended := moderation.SuspensionChange(before, after, now)
	if !changed {
		return false, nil
	}
	if !suspended {
		actionID = ""
	}
	return suspended, tx.Emit(ctx, AccountSuspensionChanged{AccountID: accountID, Suspended: suspended, ActionID: actionID, ChangedAt: now})
}

const (
	// JobExpireRestrictions removes the restriction rows of accounts and domains that have expired
	// and tells that they ended, and the end of the suspensions among them (FR-803,
	// federation.md §11.1); no payload.
	JobExpireRestrictions = "moderation.expire_restrictions"
	// ExpiryInterval is the period of the expiry schedule: an expired suspension reaches other
	// servers within it.
	ExpiryInterval = 5 * time.Minute
	// expiryBatch is the number of rows one expiry transaction removes; expiryMaxBatches bounds
	// the batches of one run, and the next run continues.
	expiryBatch      = 500
	expiryMaxBatches = 20
)

// RegisterJobs registers the moderation job handlers on reg (the expiry of restrictions, the
// severance of a suspended domain's follows, and the purge of the records past their
// retention) and the expiry and purge schedules on sched.
func (s *Service) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobSeverDomain, s.handleSeverDomain)
	reg.Register(JobExpireRestrictions, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.ExpireAccountRestrictions(ctx); err != nil {
			return queue.Retry(0, err)
		}
		if _, err := s.ExpireDomainRestrictions(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobExpireRestrictions, Every: ExpiryInterval, Job: queue.NewJob{Type: JobExpireRestrictions}})
	s.registerPurgeJob(reg, sched)
}

// ExpireAccountRestrictions removes the restriction rows of accounts that have expired and
// returns how many it removed. An expired row already restricts nothing (readers check the
// expiry), so removing it changes no restriction in effect; for each account whose last
// suspension it removes, it emits AccountSuspensionChanged in the same transaction, so that the
// end of an expired suspension is sent once, at the first run after the expiry, and for each
// account it removes a row of, AccountRestrictionsExpired. Each batch is
// one short transaction, and one run removes a bounded number of rows; the next run continues.
// A row that a lift removes first is told by the lift.
func (s *Service) ExpireAccountRestrictions(ctx context.Context) (int, error) {
	removed := 0
	for range expiryMaxBatches {
		var n int
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			now := kernel.Normalize(s.d.Clock.Now())
			rows, err := s.d.Repository.DeleteExpiredAccountRestrictions(ctx, tx, now, expiryBatch)
			if err != nil {
				return err
			}
			n = len(rows)
			for _, id := range rowTargets(rows) {
				if err := tx.Emit(ctx, AccountRestrictionsExpired{AccountID: id}); err != nil {
					return err
				}
			}
			byAccount := map[string][]moderation.RestrictionRow{}
			var accounts []string
			for _, r := range rows {
				if r.Restriction != moderation.RestrictFederation {
					continue
				}
				if _, ok := byAccount[r.Target]; !ok {
					accounts = append(accounts, r.Target)
				}
				byAccount[r.Target] = append(byAccount[r.Target], r)
			}
			for _, id := range accounts {
				after, err := s.d.Repository.AccountRestrictionRows(ctx, tx, id)
				if err != nil {
					return err
				}
				changed, suspended := moderation.SuspensionChange(slices.Concat(after, byAccount[id]), after, now)
				if !changed {
					continue
				}
				if err := tx.Emit(ctx, AccountSuspensionChanged{AccountID: id, Suspended: suspended, ChangedAt: now}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return removed, err
		}
		removed += n
		if n < expiryBatch {
			break
		}
	}
	return removed, nil
}

// ExpireDomainRestrictions is ExpireAccountRestrictions for the rows of domains: for each domain it
// removes a row of, it emits DomainRestrictionsExpired in the same transaction.
func (s *Service) ExpireDomainRestrictions(ctx context.Context) (int, error) {
	removed := 0
	for range expiryMaxBatches {
		var n int
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			rows, err := s.d.Repository.DeleteExpiredDomainRestrictions(ctx, tx, kernel.Normalize(s.d.Clock.Now()), expiryBatch)
			if err != nil {
				return err
			}
			n = len(rows)
			for _, domain := range rowTargets(rows) {
				if err := tx.Emit(ctx, DomainRestrictionsExpired{Domain: domain}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return removed, err
		}
		removed += n
		if n < expiryBatch {
			break
		}
	}
	return removed, nil
}

// rowTargets returns the distinct targets of rows, sorted.
func rowTargets(rows []moderation.RestrictionRow) []string {
	targets := make([]string, 0, len(rows))
	for _, r := range rows {
		targets = append(targets, r.Target)
	}
	slices.Sort(targets)
	return slices.Compact(targets)
}
