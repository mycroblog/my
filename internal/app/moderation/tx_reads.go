// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The reads in a transaction: each answers as the method of the same name without Tx does, read
// in tx with what tx has written, for the use cases that present what an event's transaction
// wrote (the data of the webhook messages, ADR-0060).

// ReportRecordTx returns a report with its state as ReportRecord does, read in tx.
func (s *Service) ReportRecordTx(ctx context.Context, tx uow.Tx, id string) (moderation.ReportRecord, error) {
	r, err := s.d.Repository.FindReportTx(ctx, tx, id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	st, err := s.d.Repository.FindReportStatusTx(ctx, tx, id)
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	return moderation.ReportRecord{Report: r, Status: st}, nil
}

// AccountActionTx returns an action on an account as AccountAction does, read in tx.
func (s *Service) AccountActionTx(ctx context.Context, tx uow.Tx, id string) (AccountActionView, error) {
	r, err := s.d.Repository.FindActionRecordTx(ctx, tx, id)
	if err != nil {
		return AccountActionView{}, err
	}
	return s.accountView(r), nil
}

// AppealTx returns an appeal with its decision and action as Appeal does, read in tx.
func (s *Service) AppealTx(ctx context.Context, tx uow.Tx, id string) (AppealView, error) {
	r, err := s.d.Repository.FindAppealTx(ctx, tx, id)
	if err != nil {
		return AppealView{}, err
	}
	a, err := s.AccountActionTx(ctx, tx, r.Appeal.ActionID)
	if err != nil {
		return AppealView{}, fmt.Errorf("moderation: the action of appeal %s: %w", r.Appeal.ID, err)
	}
	return AppealView{AppealRecord: r, Action: a}, nil
}

// DomainActionTx returns an action on a domain as DomainAction does, read in tx.
func (s *Service) DomainActionTx(ctx context.Context, tx uow.Tx, id string) (DomainActionView, error) {
	r, err := s.d.Repository.FindDomainActionRecordTx(ctx, tx, id)
	if err != nil {
		return DomainActionView{}, err
	}
	return s.view(r), nil
}

// OwnRestrictionsTx returns the restrictions of an account's own actions as OwnRestrictions
// does, read in tx.
func (p *AccountPolicy) OwnRestrictionsTx(ctx context.Context, tx uow.Tx, accountID string) ([]moderation.Restriction, error) {
	return p.repo.AccountRestrictionsInEffectTx(ctx, tx, accountID, p.clock.Now())
}

// LoginRestrictedTx reports whether an account's own actions forbid it to log in as LoginRestricted
// does, read in tx.
func (p *AccountPolicy) LoginRestrictedTx(ctx context.Context, tx uow.Tx, accountID string) (bool, error) {
	rs, err := p.OwnRestrictionsTx(ctx, tx, accountID)
	return slices.Contains(rs, moderation.RestrictLogin), err
}

// RejectsMediaTx reports whether the media of a domain are rejected as RejectsMedia does, read
// in tx.
func (p *FederationPolicy) RejectsMediaTx(ctx context.Context, tx uow.Tx, domain string) (bool, error) {
	domains := moderation.DomainAndParents(domain)
	if len(domains) == 0 {
		return false, nil
	}
	rs, err := p.repo.DomainRestrictionsInEffectTx(ctx, tx, domains, p.clock.Now())
	if err != nil {
		return false, err
	}
	if rejectsMedia(rs) {
		return true, nil
	}
	return p.cutOffTx(ctx, tx, domain)
}

// cutOffTx reports whether the allowlist cuts off domain as cutOffAmong does, read in tx.
func (p *FederationPolicy) cutOffTx(ctx context.Context, tx uow.Tx, domain string) (bool, error) {
	if p.scopes == nil {
		return false, nil
	}
	scope, err := p.scopes.FederationScopeTx(ctx, tx)
	if err != nil || scope != moderation.ScopeAllowlist {
		return false, err
	}
	allowed, err := p.repo.AllowedDomainsTx(ctx, tx, moderation.DomainAndParents(domain))
	if err != nil {
		return false, err
	}
	return !moderation.Allowed(domain, allowed), nil
}

// SuspendedTx reports whether an account's own actions suspend it at the policy's now (the
// federation restriction of suspend and ban, moderation.FederationSuspended), read in tx: the
// account module's search terms leave out a suspended account and follow
// EventAccountSuspensionChanged (ADR-0092 §1).
func (p *AccountPolicy) SuspendedTx(ctx context.Context, tx uow.Tx, accountID string) (bool, error) {
	rows, err := p.repo.AccountRestrictionRows(ctx, tx, accountID)
	if err != nil {
		return false, err
	}
	return moderation.FederationSuspended(rows, p.clock.Now()), nil
}

// SuspendedAmongTx is SuspendedTx of each of ids, by account, with one query.
func (p *AccountPolicy) SuspendedAmongTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]bool, error) {
	rows, err := p.repo.AccountRestrictionRowsOf(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	now := p.clock.Now()
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = moderation.FederationSuspended(rows[id], now)
	}
	return out, nil
}

// AccountEffectsTx returns inside tx the restrictions that an account's own actions have in effect
// at the policy's now, each with its end, as moderation.account_restrictions_changed carries them.
func (p *AccountPolicy) AccountEffectsTx(ctx context.Context, tx uow.Tx, accountID string) ([]Effect, error) {
	rows, err := p.repo.AccountRestrictionRows(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	return moderation.Merge(rows, p.clock.Now()), nil
}

// accountDomainsBatch is an AccountDomains that finds the domains of many accounts at once, as
// *appaccount.Service does.
type accountDomainsBatch interface {
	AccountDomainsTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]string, error)
}

// UnsolicitedReachRestrictedAmongTx is UnsolicitedReachRestrictedTx of each of ids, by account,
// with one query for the accounts' restrictions, one for their domains, and one for the
// restrictions of the domains; without the batch read of the domains it reads them one by one.
func (p *AccountPolicy) UnsolicitedReachRestrictedAmongTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	b, ok := p.accounts.(accountDomainsBatch)
	if !ok {
		for _, id := range ids {
			restricted, err := p.UnsolicitedReachRestrictedTx(ctx, tx, id)
			if err != nil {
				return nil, err
			}
			out[id] = restricted
		}
		return out, nil
	}
	if len(ids) == 0 {
		return out, nil
	}
	now := p.clock.Now()
	own, err := p.repo.AccountRestrictionsInEffectOfTx(ctx, tx, ids, now)
	if err != nil {
		return nil, err
	}
	domains, err := b.AccountDomainsTx(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	var all []string
	for _, d := range domains {
		for _, dd := range moderation.DomainAndParents(d) {
			if !slices.Contains(all, dd) {
				all = append(all, dd)
			}
		}
	}
	ofDomains, err := p.repo.DomainRestrictionsInEffectOfTx(ctx, tx, all, now)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		rs := own[id]
		if d, found := domains[id]; found {
			for _, dd := range moderation.DomainAndParents(d) {
				rs = append(rs, ofDomains[dd]...)
			}
		}
		out[id] = slices.Contains(rs, moderation.RestrictUnsolicitedReach)
	}
	return out, nil
}

// UnsolicitedReachRestrictedTx is UnsolicitedReachRestricted inside tx.
func (p *AccountPolicy) UnsolicitedReachRestrictedTx(ctx context.Context, tx uow.Tx, accountID string) (bool, error) {
	now := p.clock.Now()
	rs, err := p.repo.AccountRestrictionsInEffectTx(ctx, tx, accountID, now)
	if err != nil {
		return false, err
	}
	domain, err := p.accounts.AccountDomainTx(ctx, tx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return slices.Contains(rs, moderation.RestrictUnsolicitedReach), nil
	case err != nil:
		return false, err
	}
	if domains := moderation.DomainAndParents(domain); len(domains) > 0 {
		ds, err := p.repo.DomainRestrictionsInEffectTx(ctx, tx, domains, now)
		if err != nil {
			return false, err
		}
		rs = append(rs, ds...)
	}
	return slices.Contains(rs, moderation.RestrictUnsolicitedReach), nil
}
