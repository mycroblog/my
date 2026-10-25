// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"slices"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// FederationPolicy reads the restrictions of domains and remote accounts that bear on
// federation (federation.md §11) from the moderation projections, domain_restrictions and
// account_restrictions, at the time of each call. A domain's restrictions cover its subdomains.
// It implements the federation module's DomainPolicy and AccountPolicy and the content module's
// MediaFetchPolicy.
//
// Callers pass canonical hosts (federation.CanonicalHost); the local domain "" has no
// restrictions.
type FederationPolicy struct {
	repo  Repository
	clock kernel.Clock
}

// NewFederationPolicy returns the FederationPolicy on repo.
func NewFederationPolicy(repo Repository, clock kernel.Clock) *FederationPolicy {
	return &FederationPolicy{repo: repo, clock: clock}
}

// DomainRestrictions returns the restrictions in effect on a domain and its parents, in
// restriction order.
func (p *FederationPolicy) DomainRestrictions(ctx context.Context, domain string) ([]moderation.Restriction, error) {
	domains := moderation.DomainAndParents(domain)
	if len(domains) == 0 {
		return nil, nil
	}
	return p.repo.DomainRestrictionsInEffect(ctx, domains, p.clock.Now())
}

func (p *FederationPolicy) domainHas(ctx context.Context, domain string, r moderation.Restriction) (bool, error) {
	rs, err := p.DomainRestrictions(ctx, domain)
	return slices.Contains(rs, r), err
}

// FederationSuspended reports whether a domain, or one of its parents, is suspended: nothing is
// received from, delivered to, or fetched from its servers.
func (p *FederationPolicy) FederationSuspended(ctx context.Context, domain string) (bool, error) {
	return p.domainHas(ctx, domain, moderation.RestrictFederation)
}

// AccountFederationSuspended reports whether an account has the federation restriction
// (suspend, ban): a remote account's activities are dropped.
func (p *FederationPolicy) AccountFederationSuspended(ctx context.Context, accountID string) (bool, error) {
	rs, err := p.repo.AccountRestrictionsInEffect(ctx, accountID, p.clock.Now())
	return slices.Contains(rs, moderation.RestrictFederation), err
}

// AccountLimited reports whether an account has the restrictions of a limit (the visibility
// ceiling that only a limit imposes), which the target of its move does not inherit (ADR-0089).
func (p *FederationPolicy) AccountLimited(ctx context.Context, accountID string) (bool, error) {
	rs, err := p.repo.AccountRestrictionsInEffect(ctx, accountID, p.clock.Now())
	return slices.Contains(rs, moderation.RestrictVisibilityCeilingPrivate), err
}

// RejectsMedia reports whether the media of a domain, or of a parent domain, are not fetched
// (reject_media).
func (p *FederationPolicy) RejectsMedia(ctx context.Context, domain string) (bool, error) {
	return p.domainHas(ctx, domain, moderation.RestrictMediaFetch)
}

// RejectsReports reports whether the reports of a domain, or of a parent domain, are dropped
// (reject_reports).
func (p *FederationPolicy) RejectsReports(ctx context.Context, domain string) (bool, error) {
	return p.domainHas(ctx, domain, moderation.RestrictReportIntake)
}

// DiscoveryRestricted reports whether a relay must not bring in the posts of an author (ADR-0061):
// the domain, or a parent, or the account (accountID, "" for an account that is not stored) has
// the federation or the public_discovery restriction, so that neither the federated timeline nor
// search would show them.
func (p *FederationPolicy) DiscoveryRestricted(ctx context.Context, domain, accountID string) (bool, error) {
	keeps := func(rs []moderation.Restriction) bool {
		return slices.Contains(rs, moderation.RestrictFederation) || slices.Contains(rs, moderation.RestrictPublicDiscovery)
	}
	rs, err := p.DomainRestrictions(ctx, domain)
	if err != nil || keeps(rs) {
		return err == nil, err
	}
	if accountID == "" {
		return false, nil
	}
	rs, err = p.repo.AccountRestrictionsInEffect(ctx, accountID, p.clock.Now())
	return keeps(rs), err
}
