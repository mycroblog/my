// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AccountDomains finds the domain of an account. *appaccount.Service implements it.
type AccountDomains interface {
	AccountDomain(ctx context.Context, id string) (string, error)
	AccountDomainTx(ctx context.Context, tx uow.Tx, id string) (string, error)
}

// AccountPolicy is the RestrictionPolicy of data-model §5.3 as far as moderation decides it: the
// restrictions in effect on an account are those of its own actions (account_restrictions) and
// those of its domain and the parent domains (domain_restrictions), read at the time of each
// call. The other modules decide through it, never from the moderation tables: the content
// module through Content, the identity module through LoginRestricted and Banned, the graph module
// through InteractionRestricted, and the notification module through
// UnsolicitedReachRestricted.
type AccountPolicy struct {
	repo     Repository
	accounts AccountDomains
	clock    kernel.Clock
}

// NewAccountPolicy returns the AccountPolicy on repo.
func NewAccountPolicy(repo Repository, accounts AccountDomains, clock kernel.Clock) *AccountPolicy {
	return &AccountPolicy{repo: repo, accounts: accounts, clock: clock}
}

// Restrictions returns the restrictions in effect on an account, in restriction order. An
// unknown account has those of its own actions only.
func (p *AccountPolicy) Restrictions(ctx context.Context, accountID string) ([]moderation.Restriction, error) {
	now := p.clock.Now()
	rs, err := p.repo.AccountRestrictionsInEffect(ctx, accountID, now)
	if err != nil {
		return nil, err
	}
	domain, err := p.accounts.AccountDomain(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return rs, nil
	case err != nil:
		return nil, err
	}
	domains := moderation.DomainAndParents(domain)
	if len(domains) == 0 {
		return rs, nil
	}
	ds, err := p.repo.DomainRestrictionsInEffect(ctx, domains, now)
	if err != nil {
		return nil, err
	}
	rs = append(rs, ds...)
	slices.Sort(rs)
	return slices.Compact(rs), nil
}

// accountDomainBatch is an AccountDomains that finds the domains of many accounts at once, as
// *appaccount.Service does.
type accountDomainBatch interface {
	AccountDomains(ctx context.Context, ids []string) (map[string]string, error)
}

// RestrictionsOf returns the restrictions in effect on each of accountIDs, by account, as
// Restrictions returns those of one, reading the accounts' own restrictions, their domains, and
// the restrictions of those domains and their parents with one query each.
func (p *AccountPolicy) RestrictionsOf(ctx context.Context, accountIDs []string) (map[string][]moderation.Restriction, error) {
	out := make(map[string][]moderation.Restriction, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	now := p.clock.Now()
	own, err := p.repo.AccountRestrictionsInEffectOf(ctx, accountIDs, now)
	if err != nil {
		return nil, err
	}
	domainOf := map[string]string{}
	if b, ok := p.accounts.(accountDomainBatch); ok {
		if domainOf, err = b.AccountDomains(ctx, accountIDs); err != nil {
			return nil, err
		}
	} else {
		for _, id := range accountIDs {
			d, err := p.accounts.AccountDomain(ctx, id)
			switch {
			case errors.Is(err, appaccount.ErrNotFound):
			case err != nil:
				return nil, err
			default:
				domainOf[id] = d
			}
		}
	}
	var domains []string
	for _, d := range domainOf {
		for _, dd := range moderation.DomainAndParents(d) {
			if !slices.Contains(domains, dd) {
				domains = append(domains, dd)
			}
		}
	}
	byDomain, err := p.repo.DomainRestrictionsInEffectOf(ctx, domains, now)
	if err != nil {
		return nil, err
	}
	for _, id := range accountIDs {
		rs := slices.Clone(own[id])
		if d, ok := domainOf[id]; ok {
			for _, dd := range moderation.DomainAndParents(d) {
				rs = append(rs, byDomain[dd]...)
			}
		}
		slices.Sort(rs)
		out[id] = slices.Compact(rs)
	}
	return out, nil
}

func (p *AccountPolicy) has(ctx context.Context, accountID string, r moderation.Restriction) (bool, error) {
	rs, err := p.Restrictions(ctx, accountID)
	return slices.Contains(rs, r), err
}

// LoginRestricted reports whether an account may not log in or use the API (the login
// restriction: disable_login, suspend, ban).
func (p *AccountPolicy) LoginRestricted(ctx context.Context, accountID string) (bool, error) {
	return p.has(ctx, accountID, moderation.RestrictLogin)
}

// banPage is how many actions in effect Banned reads at a time.
const banPage = 100

// Banned reports whether a ban is in effect on an account: issued and not lifted by an
// overturned appeal (a ban never expires). Its account signs in to the server-rendered screens
// until the ban deletes it, so that the user can appeal (FR-707).
func (p *AccountPolicy) Banned(ctx context.Context, accountID string) (bool, error) {
	now := kernel.Normalize(p.clock.Now())
	q := ActionQuery{AccountID: accountID, InEffectAt: &now}
	for {
		rs, err := p.repo.ListActions(ctx, q, banPage)
		if err != nil {
			return false, err
		}
		if slices.ContainsFunc(rs, func(r moderation.ActionRecord) bool { return r.Action.Kind == moderation.ActionBan }) {
			return true, nil
		}
		if len(rs) < banPage {
			return false, nil
		}
		q.Before = rs[len(rs)-1].Action.ID
	}
}

// InteractionRestricted reports whether an account may not follow, favourite, boost, or reply
// (the interact restriction: restrict_interaction, suspend).
func (p *AccountPolicy) InteractionRestricted(ctx context.Context, accountID string) (bool, error) {
	return p.has(ctx, accountID, moderation.RestrictInteract)
}

// ProfileHidden reports whether an account's profile is shown to nobody but itself and the
// moderators (the profile_view restriction: suspend, or a suspended domain).
func (p *AccountPolicy) ProfileHidden(ctx context.Context, accountID string) (bool, error) {
	return p.has(ctx, accountID, moderation.RestrictProfileView)
}

// UnsolicitedReachRestricted reports whether an account's notifications reach only the accounts
// that follow it (the unsolicited_reach restriction: limit, or a limited domain).
func (p *AccountPolicy) UnsolicitedReachRestricted(ctx context.Context, accountID string) (bool, error) {
	return p.has(ctx, accountID, moderation.RestrictUnsolicitedReach)
}

// Content returns the policy as the content module's RestrictionPolicy.
func (p *AccountPolicy) Content() appcontent.RestrictionPolicy { return contentPolicy{p} }

type contentPolicy struct{ p *AccountPolicy }

// Restrictions implements appcontent.RestrictionPolicy.
func (c contentPolicy) Restrictions(ctx context.Context, accountID string) (appcontent.Restrictions, error) {
	rs, err := c.p.Restrictions(ctx, accountID)
	if err != nil {
		return appcontent.Restrictions{}, err
	}
	return contentRestrictions(rs), nil
}

// RestrictionsOf returns the content restrictions of many accounts, by account, as Restrictions
// returns those of one (AccountPolicy.RestrictionsOf).
func (c contentPolicy) RestrictionsOf(ctx context.Context, accountIDs []string) (map[string]appcontent.Restrictions, error) {
	all, err := c.p.RestrictionsOf(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]appcontent.Restrictions, len(all))
	for id, rs := range all {
		out[id] = contentRestrictions(rs)
	}
	return out, nil
}

// contentRestrictions are the restrictions that the content module applies, of those in effect.
func contentRestrictions(rs []moderation.Restriction) appcontent.Restrictions {
	return appcontent.Restrictions{
		Post:           slices.Contains(rs, moderation.RestrictPost),
		Interact:       slices.Contains(rs, moderation.RestrictInteract),
		MediaSensitive: slices.Contains(rs, moderation.RestrictMediaSensitive),
		Author: appcontent.AuthorRestrictions{
			ProfileView:              slices.Contains(rs, moderation.RestrictProfileView),
			VisibilityCeilingPrivate: slices.Contains(rs, moderation.RestrictVisibilityCeilingPrivate),
			PublicDiscovery:          slices.Contains(rs, moderation.RestrictPublicDiscovery),
		},
	}
}
