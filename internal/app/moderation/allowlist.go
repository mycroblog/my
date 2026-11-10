// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The allowlist of federation (ADR-0159 §1-§3). The allowed domains are resources of their own
// (domain_allows), not domain actions. While the instance setting federation_scope is allowlist,
// FederationPolicy answers for a domain that neither it nor a parent domain allows as for a
// suspended domain, so that every entry point of federation (the inbox, the delivery, the fetches,
// the media proxy, the relays) refuses it through the check it already makes. The switch to
// allowlist, and the removal of an allow while it is on, record an allowlist_cutoffs row and end
// the follows with the domains cut off in JobSeverAllowlist, as a domain suspension does (FR-717).

// DomainAllow is moderation.DomainAllow.
type DomainAllow = moderation.DomainAllow

// AllowlistCutoff is moderation.AllowlistCutoff.
type AllowlistCutoff = moderation.AllowlistCutoff

// FederationScope is moderation.FederationScope.
type FederationScope = moderation.FederationScope

// Values of FederationScope.
const (
	ScopeOpen      = moderation.ScopeOpen
	ScopeAllowlist = moderation.ScopeAllowlist
)

var (
	// ErrTooManyDomainAllows reports an allow beyond moderation.MaxDomainAllows.
	ErrTooManyDomainAllows = errors.New("moderation: the allowlist is full")
	// ErrInvalidDomainAllow is moderation.ErrInvalidDomainAllow.
	ErrInvalidDomainAllow = moderation.ErrInvalidDomainAllow
)

// FederationScopes reads the instance setting federation_scope, per call, so that a change applies
// at once. The instance module imports this one, so it is a port; nil is open.
type FederationScopes interface {
	FederationScope(ctx context.Context) (moderation.FederationScope, error)
	FederationScopeTx(ctx context.Context, tx uow.Tx) (moderation.FederationScope, error)
}

// AllowlistRepository is the part of Repository that holds the allowlist and its cutoffs.
type AllowlistRepository interface {
	// InsertDomainAllow inserts an allow and reports whether it did: a domain already allowed
	// keeps its row and inserts nothing.
	InsertDomainAllow(ctx context.Context, tx uow.Tx, a moderation.DomainAllow) (bool, error)
	// DeleteDomainAllow deletes the allow of a domain and returns it; ErrNotFound when there is
	// none.
	DeleteDomainAllow(ctx context.Context, tx uow.Tx, domain string) (moderation.DomainAllow, error)
	// FindDomainAllow returns the allow of a domain; ErrNotFound when there is none.
	FindDomainAllow(ctx context.Context, tx uow.Tx, domain string) (moderation.DomainAllow, error)
	// ListDomainAllows returns every allow, in domain order.
	ListDomainAllows(ctx context.Context) ([]moderation.DomainAllow, error)
	// CountDomainAllows counts the allows, inside tx.
	CountDomainAllows(ctx context.Context, tx uow.Tx) (int, error)
	// AllowedDomains returns which of domains are allowed (each a row of domain_allows), and
	// AllowedDomainsTx the same inside tx.
	AllowedDomains(ctx context.Context, domains []string) ([]string, error)
	AllowedDomainsTx(ctx context.Context, tx uow.Tx, domains []string) ([]string, error)
	InsertAllowlistCutoff(ctx context.Context, tx uow.Tx, c moderation.AllowlistCutoff) error
	// FindAllowlistCutoff returns a cutoff by ID; ErrNotFound when there is none.
	FindAllowlistCutoff(ctx context.Context, id string) (moderation.AllowlistCutoff, error)
}

// JobSeverAllowlist cuts the follows of the local accounts with the domains that a cutoff of the
// allowlist covers; its payload is severAllowlistPayload.
const JobSeverAllowlist = "moderation.sever_allowlist"

type severAllowlistPayload struct {
	queue.FanOutStep
	CutoffID string `json:"cutoff_id"`
}

// AllowDomainInput is an allow of a domain.
type AllowDomainInput struct {
	// Domain is a canonical host (federation.CanonicalHost); the allow covers its subdomains.
	Domain  string
	Comment string
	// ByUserID is the user who allows it, "" for the CLI.
	ByUserID string
}

// AllowDomain adds a domain to the allowlist and records it in the audit log, in one transaction.
// It returns the allow and whether it is new: a domain already allowed keeps its allow, and
// records nothing. It fails with moderation.ErrInvalidDomainAllow for a domain that is not a
// canonical host or a comment that is too long, with moderation.ErrOwnDomain for this instance's
// domain, and with ErrTooManyDomainAllows beyond moderation.MaxDomainAllows. The allow takes
// effect at once while federation_scope is allowlist; the follows that the allowlist ended do not
// come back.
func (s *Service) AllowDomain(ctx context.Context, in AllowDomainInput) (moderation.DomainAllow, bool, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	a := moderation.DomainAllow{Domain: in.Domain, Comment: in.Comment, CreatedByUserID: in.ByUserID, CreatedAt: now}
	if err := a.Validate(); err != nil {
		return moderation.DomainAllow{}, false, err
	}
	if s.ownDomain(in.Domain) {
		return moderation.DomainAllow{}, false, fmt.Errorf("%w: %q", moderation.ErrOwnDomain, in.Domain)
	}
	var created bool
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		existing, err := s.d.Repository.FindDomainAllow(ctx, tx, a.Domain)
		switch {
		case err == nil:
			a, created = existing, false
			return nil
		case !errors.Is(err, ErrNotFound):
			return err
		}
		n, err := s.d.Repository.CountDomainAllows(ctx, tx)
		if err != nil {
			return err
		}
		if n >= moderation.MaxDomainAllows {
			return ErrTooManyDomainAllows
		}
		if created, err = s.d.Repository.InsertDomainAllow(ctx, tx, a); err != nil || !created {
			return err
		}
		return s.emitAudit(ctx, tx, moderation.AuditEntry{
			ID: s.d.IDs.NewID(), Action: moderation.AuditDomainAllowAdded, TargetKind: moderation.AuditTargetDomainAllow,
			TargetID: a.Domain, TargetLabel: a.Domain, OccurredAt: now,
		}, in.ByUserID)
	})
	if err != nil {
		return moderation.DomainAllow{}, false, err
	}
	return a, created, nil
}

// RemoveDomainAllow removes a domain from the allowlist and records it in the audit log, and
// reports whether the domain had an allow. While federation_scope is allowlist and no parent
// domain is allowed, the domain is cut off at once: the same transaction records an
// allowlist_cutoffs row (the source of the audit log entry) and enqueues JobSeverAllowlist, which
// ends the follows with its accounts and those of its subdomains that no other allow covers.
func (s *Service) RemoveDomainAllow(ctx context.Context, domain, byUserID string) (bool, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	removed := false
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Repository.DeleteDomainAllow(ctx, tx, domain)
		switch {
		case errors.Is(err, ErrNotFound):
			return nil
		case err != nil:
			return err
		}
		removed = true
		entry := moderation.AuditEntry{
			ID: s.d.IDs.NewID(), Action: moderation.AuditDomainAllowRemoved, TargetKind: moderation.AuditTargetDomainAllow,
			TargetID: domain, TargetLabel: domain, OccurredAt: now,
		}
		scope, err := s.scopeTx(ctx, tx)
		if err != nil {
			return err
		}
		if scope == moderation.ScopeAllowlist {
			allowed, err := s.d.Repository.AllowedDomainsTx(ctx, tx, moderation.DomainAndParents(domain))
			if err != nil {
				return err
			}
			if !moderation.Allowed(domain, allowed) {
				c := moderation.AllowlistCutoff{ID: s.d.IDs.NewID(), Cause: moderation.CutoffAllowRemoved, Domain: domain, IssuedByUserID: byUserID, OccurredAt: now}
				if err := s.recordCutoff(ctx, tx, c); err != nil {
					return err
				}
				entry.SourceEventID = c.ID
			}
		}
		return s.emitAudit(ctx, tx, entry, byUserID)
	})
	return removed, err
}

// DomainAllows returns every allow, in domain order.
func (s *Service) DomainAllows(ctx context.Context) ([]moderation.DomainAllow, error) {
	return s.d.Repository.ListDomainAllows(ctx)
}

// FederationScope returns the current value of federation_scope, open without Deps.Scopes.
func (s *Service) FederationScope(ctx context.Context) (moderation.FederationScope, error) {
	if s.d.Scopes == nil {
		return moderation.ScopeOpen, nil
	}
	return s.d.Scopes.FederationScope(ctx)
}

func (s *Service) scopeTx(ctx context.Context, tx uow.Tx) (moderation.FederationScope, error) {
	if s.d.Scopes == nil {
		return moderation.ScopeOpen, nil
	}
	return s.d.Scopes.FederationScopeTx(ctx, tx)
}

// FederationScopeSwitchedTx cuts off, inside the transaction of the change of federation_scope
// from previous to scope by byUserID ("" for the CLI), the domains that the allowlist does not
// allow: a switch from open to allowlist records an allowlist_cutoffs row and enqueues
// JobSeverAllowlist. Any other change, and setting allowlist again, records nothing; switching back
// to open restores no follow.
func (s *Service) FederationScopeSwitchedTx(ctx context.Context, tx uow.Tx, previous, scope moderation.FederationScope, byUserID string) error {
	if scope != moderation.ScopeAllowlist || previous == moderation.ScopeAllowlist {
		return nil
	}
	return s.recordCutoff(ctx, tx, moderation.AllowlistCutoff{
		ID: s.d.IDs.NewID(), Cause: moderation.CutoffScopeSwitched, IssuedByUserID: byUserID,
		OccurredAt: kernel.Normalize(s.d.Clock.Now()),
	})
}

// recordCutoff inserts a cutoff and, when the Service cuts follows, enqueues JobSeverAllowlist
// for it, in tx.
func (s *Service) recordCutoff(ctx context.Context, tx uow.Tx, c moderation.AllowlistCutoff) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := s.d.Repository.InsertAllowlistCutoff(ctx, tx, c); err != nil {
		return err
	}
	if !s.severs() || s.d.Directory == nil || s.d.Outbox == nil {
		return nil
	}
	payload, err := json.Marshal(severAllowlistPayload{CutoffID: c.ID})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobSeverAllowlist, Payload: payload, IdempotencyKey: JobSeverAllowlist + ":" + c.ID})
}

// SeverAllowlist cuts the follows between the local accounts and the remote accounts that the
// cutoff cutoffID covers, without telling them, and records a severance of the cutoff for each
// local account that lost an active follow; it returns how many local accounts it recorded. A
// switch covers every remote account whose domain the allowlist does not allow, a removal those of
// the domain and its subdomains that no remaining allow covers. The allowlist and the scope are
// read when each local account is cut: once federation_scope is open again, or a domain is allowed
// again, nothing more of it is cut. It runs one short transaction per page of local accounts, so
// running it again changes nothing it did.
func (s *Service) SeverAllowlist(ctx context.Context, cutoffID string) (int, error) {
	recorded := 0
	err := s.severAllowlist(ctx, cutoffID, &recorded, func(fan queue.FanOut) error {
		return fan.Walk(ctx, queue.KeyRange{})
	})
	return recorded, err
}

func (s *Service) severAllowlist(ctx context.Context, cutoffID string, recorded *int, run func(queue.FanOut) error) error {
	if !s.severs() || s.d.Directory == nil {
		return fmt.Errorf("%w: severing follows without the graph and account modules", ErrUnsupportedAction)
	}
	domains, ok := s.d.Directory.(accountDomainsBatch)
	if !ok {
		return fmt.Errorf("%w: severing follows without the domains of the accounts", ErrUnsupportedAction)
	}
	c, err := s.d.Repository.FindAllowlistCutoff(ctx, cutoffID)
	if err != nil {
		return err
	}
	fan := queue.FanOut{
		Size: s.d.FanOutSize,
		Page: queue.PageByLastKey(s.d.Directory.LocalAccountIDs),
		Handle: func(ctx context.Context, locals []string) error {
			var created int
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				var err error
				created, err = s.severAllowlistOf(ctx, tx, domains, c, locals)
				return err
			})
			*recorded += created
			return err
		},
		Enqueue: func(ctx context.Context, steps []queue.FanOutStep) error {
			jobs := make([]queue.NewJob, 0, len(steps))
			for _, step := range steps {
				payload, err := json.Marshal(severAllowlistPayload{FanOutStep: step, CutoffID: c.ID})
				if err != nil {
					return err
				}
				jobs = append(jobs, queue.NewJob{Type: JobSeverAllowlist, Payload: payload, IdempotencyKey: queue.StepKey(JobSeverAllowlist+":"+c.ID, step)})
			}
			return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, jobs...) })
		},
	}
	return run(fan)
}

// cutOff returns, of the remote accounts of domainOf (account → canonical host), those that
// cutoff c covers while the allowlist is allowed: their domain is not allowed and, for a removal,
// is the removed domain or one of its subdomains.
func cutOff(c moderation.AllowlistCutoff, domainOf map[string]string, allowed []string) map[string]bool {
	out := map[string]bool{}
	for id, d := range domainOf {
		if d == "" || moderation.Allowed(d, allowed) {
			continue
		}
		if c.Cause == moderation.CutoffAllowRemoved && !slices.Contains(moderation.DomainAndParents(d), c.Domain) {
			continue
		}
		out[id] = true
	}
	return out
}

// remoteDomains returns the domains of accounts and their parents, each once.
func remoteDomains(domainOf map[string]string) []string {
	var all []string
	for _, d := range domainOf {
		for _, dd := range moderation.DomainAndParents(d) {
			if !slices.Contains(all, dd) {
				all = append(all, dd)
			}
		}
	}
	return all
}

// severAllowlistOf cuts, inside tx, the follows between locals and the accounts that cutoff c
// covers, and records their severances; it returns how many severances are new.
func (s *Service) severAllowlistOf(ctx context.Context, tx uow.Tx, domains accountDomainsBatch, c moderation.AllowlistCutoff, locals []string) (int, error) {
	scope, err := s.scopeTx(ctx, tx)
	if err != nil || scope != moderation.ScopeAllowlist {
		return 0, err
	}
	counterparts, err := s.d.Follows.CounterpartsOf(ctx, tx, locals)
	if err != nil {
		return 0, err
	}
	var all []string
	for _, local := range locals {
		all = append(all, counterparts[local]...)
	}
	if len(all) == 0 {
		return 0, nil
	}
	slices.Sort(all)
	domainOf, err := domains.AccountDomainsTx(ctx, tx, slices.Compact(all))
	if err != nil {
		return 0, err
	}
	allowed, err := s.d.Repository.AllowedDomainsTx(ctx, tx, remoteDomains(domainOf))
	if err != nil {
		return 0, err
	}
	covered := cutOff(c, domainOf, allowed)
	if len(covered) == 0 {
		return 0, nil
	}
	others := make(map[string][]string, len(locals))
	for _, local := range locals {
		for _, other := range counterparts[local] {
			if covered[other] {
				others[local] = append(others[local], other)
			}
		}
	}
	now := kernel.Normalize(s.d.Clock.Now())
	lost, err := s.d.Follows.SeverAmong(ctx, tx, others, appgraph.SeverBoth, false, now)
	if err != nil {
		return 0, err
	}
	return s.recordSeverances(ctx, tx, Severance{AllowlistCutoffID: c.ID}, locals, lost, now)
}

// handleSeverAllowlist runs JobSeverAllowlist.
func (s *Service) handleSeverAllowlist(ctx context.Context, j queue.Job) queue.Result {
	var p severAllowlistPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.CutoffID == "" {
		return queue.Fail(fmt.Errorf("moderation: payload of %s: %q", JobSeverAllowlist, j.Payload))
	}
	recorded := 0
	err := s.severAllowlist(ctx, p.CutoffID, &recorded, func(fan queue.FanOut) error {
		return fan.Run(ctx, p.FanOutStep)
	})
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrUnsupportedAction):
		return queue.Fail(err)
	case err != nil:
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// ActiveFollows reads the accepted follows of accounts, for the impact of the allowlist.
// *appgraph.Service implements it; a Deps.Follows that does not counts nothing.
type ActiveFollows interface {
	ActiveFollowsOf(ctx context.Context, tx uow.Tx, ids []string) ([]appgraph.ActiveFollow, error)
}

// ScopeImpact is what switching federation_scope to allowlist would end now (ADR-0159 §3): the
// accepted follows, in both directions, between the local accounts and the remote accounts whose
// domain the allowlist does not allow.
type ScopeImpact struct {
	// Scope is the current value of federation_scope.
	Scope moderation.FederationScope
	// Follows counts the follows that would end, LocalAccounts the local accounts that would lose
	// one, and Domains the domains of the remote accounts.
	Follows, LocalAccounts, Domains int
	// TopDomains are the domains with the most follows that would end, at most
	// ScopeImpactTopDomains, the most first and then by domain.
	TopDomains []DomainImpact
}

// DomainImpact is the follows with one domain that the allowlist would end.
type DomainImpact struct {
	Domain  string
	Follows int
}

// ScopeImpactTopDomains is how many domains ScopeImpact names.
const ScopeImpactTopDomains = 10

// scopeImpactPage is how many local accounts FederationScopeImpact reads at a time.
const scopeImpactPage = 500

// FederationScopeImpact counts what switching federation_scope to allowlist with the current
// allowlist would end, for the confirmation that the switch asks for. It reads every local account
// and its follows, a page at a time, and changes nothing; while the scope is already allowlist, it
// counts what is left, which is nothing once the cutoff has run.
func (s *Service) FederationScopeImpact(ctx context.Context) (ScopeImpact, error) {
	scope, err := s.FederationScope(ctx)
	if err != nil {
		return ScopeImpact{}, err
	}
	out := ScopeImpact{Scope: scope}
	follows, ok := s.d.Follows.(ActiveFollows)
	domains, okDomains := s.d.Directory.(accountDomainsBatch)
	if !ok || !okDomains {
		return out, nil
	}
	byDomain := map[string]int{}
	after := ""
	for {
		locals, err := s.d.Directory.LocalAccountIDs(ctx, after, scopeImpactPage)
		if err != nil {
			return ScopeImpact{}, err
		}
		if len(locals) == 0 {
			break
		}
		err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return s.countImpact(ctx, tx, follows, domains, locals, &out, byDomain)
		})
		if err != nil {
			return ScopeImpact{}, err
		}
		if len(locals) < scopeImpactPage {
			break
		}
		after = locals[len(locals)-1]
	}
	out.Domains = len(byDomain)
	for d, n := range byDomain {
		out.TopDomains = append(out.TopDomains, DomainImpact{Domain: d, Follows: n})
	}
	slices.SortFunc(out.TopDomains, func(a, b DomainImpact) int {
		return cmp.Or(cmp.Compare(b.Follows, a.Follows), cmp.Compare(a.Domain, b.Domain))
	})
	if len(out.TopDomains) > ScopeImpactTopDomains {
		out.TopDomains = out.TopDomains[:ScopeImpactTopDomains]
	}
	return out, nil
}

// countImpact adds to out and byDomain the accepted follows of locals that the allowlist would end.
func (s *Service) countImpact(ctx context.Context, tx uow.Tx, follows ActiveFollows, domains accountDomainsBatch, locals []string, out *ScopeImpact, byDomain map[string]int) error {
	fs, err := follows.ActiveFollowsOf(ctx, tx, locals)
	if err != nil || len(fs) == 0 {
		return err
	}
	isLocal := make(map[string]bool, len(locals))
	for _, l := range locals {
		isLocal[l] = true
	}
	var others []string
	for _, f := range fs {
		others = append(others, f.FollowerAccountID, f.FolloweeAccountID)
	}
	slices.Sort(others)
	domainOf, err := domains.AccountDomainsTx(ctx, tx, slices.Compact(others))
	if err != nil {
		return err
	}
	allowed, err := s.d.Repository.AllowedDomainsTx(ctx, tx, remoteDomains(domainOf))
	if err != nil {
		return err
	}
	covered := cutOff(moderation.AllowlistCutoff{Cause: moderation.CutoffScopeSwitched}, domainOf, allowed)
	affected := map[string]bool{}
	for _, f := range fs {
		var local, remote string
		switch {
		case isLocal[f.FollowerAccountID] && covered[f.FolloweeAccountID]:
			local, remote = f.FollowerAccountID, f.FolloweeAccountID
		case isLocal[f.FolloweeAccountID] && covered[f.FollowerAccountID]:
			local, remote = f.FolloweeAccountID, f.FollowerAccountID
		default:
			continue
		}
		out.Follows++
		affected[local] = true
		byDomain[domainOf[remote]]++
	}
	out.LocalAccounts += len(affected)
	return nil
}
