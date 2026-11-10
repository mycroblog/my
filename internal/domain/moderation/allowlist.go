// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

// The allowlist of federation (ADR-0159 §1-§3): while the instance setting federation_scope is
// allowlist, a domain federates only when it, or a parent domain, is allowed, and a domain
// without an allow has the restrictions of a domain suspension. The actions on domains still
// apply to the allowed ones: an action on an allowed domain restricts it as it would under open.

// ErrInvalidDomainAllow reports an allow that breaks its invariants.
var ErrInvalidDomainAllow = errors.New("moderation: invalid domain allow")

// MaxDomainAllowComment is the longest comment of an allow, in characters.
const MaxDomainAllowComment = 5000

// MaxDomainAllows is how many domains the allowlist holds at most, as many as an import of
// domain actions reads.
const MaxDomainAllows = 5000

// DomainAllow is a domain that federates while federation_scope is allowlist (domain_allows); it
// covers the domain's subdomains.
type DomainAllow struct {
	// Domain is a canonical host (IDNA A-labels).
	Domain string
	// Comment is a note for the moderators, never published; "" for none.
	Comment string
	// CreatedByUserID is the user who allowed the domain, "" for the CLI.
	CreatedByUserID string
	CreatedAt       time.Time
}

// Validate checks the invariants of an allow: a canonical domain and a comment of at most
// MaxDomainAllowComment characters.
func (a DomainAllow) Validate() error {
	switch {
	case !ValidDomain(a.Domain):
		return fmt.Errorf("%w: domain %q", ErrInvalidDomainAllow, a.Domain)
	case !validText(a.Comment) || utf8.RuneCountInString(a.Comment) > MaxDomainAllowComment:
		return fmt.Errorf("%w: comment longer than %d characters", ErrInvalidDomainAllow, MaxDomainAllowComment)
	}
	return nil
}

// FederationScope is the value of the instance setting federation_scope (ADR-0159 §2): open
// federates with every domain that no action suspends, allowlist only with the allowed ones.
type FederationScope string

// Values of FederationScope.
const (
	ScopeOpen      FederationScope = "open"
	ScopeAllowlist FederationScope = "allowlist"
)

// CutoffCause is why the allowlist cut domains off (allowlist_cutoffs.cause).
type CutoffCause string

// Causes of a cutoff.
const (
	// CutoffScopeSwitched: federation_scope became allowlist, which cuts off every domain that
	// is not allowed.
	CutoffScopeSwitched CutoffCause = "scope_switched"
	// CutoffAllowRemoved: the allow of a domain was removed while federation_scope is allowlist,
	// which cuts the domain off unless a parent domain is allowed.
	CutoffAllowRemoved CutoffCause = "allow_removed"
)

// AllowlistCutoff is the event that the allowlist cut domains off (allowlist_cutoffs): the
// follows between the local accounts and the accounts of those domains end, in both directions
// and without telling them, as a domain suspension ends them (FR-717).
type AllowlistCutoff struct {
	ID    string
	Cause CutoffCause
	// Domain is the domain whose allow was removed, for CutoffAllowRemoved only.
	Domain string
	// IssuedByUserID is the user who switched the scope or removed the allow, "" for the CLI.
	IssuedByUserID string
	OccurredAt     time.Time
}

// Validate checks the invariants of a cutoff: an ID, a known cause, and a domain exactly for
// CutoffAllowRemoved.
func (c AllowlistCutoff) Validate() error {
	switch {
	case c.ID == "":
		return fmt.Errorf("%w: a cutoff needs an ID", ErrInvalidDomainAllow)
	case c.Cause != CutoffScopeSwitched && c.Cause != CutoffAllowRemoved:
		return fmt.Errorf("%w: cause %q", ErrInvalidDomainAllow, c.Cause)
	case (c.Cause == CutoffAllowRemoved) != (c.Domain != ""):
		return fmt.Errorf("%w: a cutoff names a domain exactly when an allow was removed", ErrInvalidDomainAllow)
	case c.Domain != "" && !ValidDomain(c.Domain):
		return fmt.Errorf("%w: domain %q", ErrInvalidDomainAllow, c.Domain)
	}
	return nil
}

// Allowed reports whether a canonical host federates under the allowlist allowed, the domains of
// the allows: the host or one of its parent domains is among them. The local domain "" is always
// allowed.
func Allowed(domain string, allowed []string) bool {
	parents := DomainAndParents(domain)
	if len(parents) == 0 {
		return true
	}
	return slices.ContainsFunc(parents, func(d string) bool { return slices.Contains(allowed, d) })
}

// CutOffRestrictions are the restrictions of a domain that the allowlist cuts off: those of a
// domain suspension (ADR-0159 §2).
func CutOffRestrictions() []Restriction { return DomainSuspend.Restrictions() }
