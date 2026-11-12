// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"slices"
	"strings"
	"time"
)

// Restriction is a capability that a moderation action takes away
// (account_restrictions.restriction and domain_restrictions.restriction).
type Restriction string

// Restrictions.
const (
	// RestrictLogin forbids logging in and using the API.
	RestrictLogin Restriction = "login"
	// RestrictPost forbids posting, replying, boosting, and editing.
	RestrictPost Restriction = "post"
	// RestrictInteract forbids following, favouriting, boosting, and voting.
	RestrictInteract Restriction = "interact"
	// RestrictVisibilityCeilingPrivate caps the visibility of statuses at private.
	RestrictVisibilityCeilingPrivate Restriction = "visibility_ceiling_private"
	// RestrictPublicDiscovery keeps statuses out of the local, federated, and hashtag
	// timelines, search, and suggestions.
	RestrictPublicDiscovery Restriction = "public_discovery"
	// RestrictUnsolicitedReach keeps notifications and mentions from accounts that do not follow.
	RestrictUnsolicitedReach Restriction = "unsolicited_reach"
	// RestrictMediaSensitive shows every attachment as sensitive.
	RestrictMediaSensitive Restriction = "media_sensitive"
	// RestrictProfileView hides the profile and statuses from all but the owner and moderators.
	RestrictProfileView Restriction = "profile_view"
	// RestrictFederation stops federation: received activities are dropped, and nothing is
	// delivered (federation.md §1).
	RestrictFederation Restriction = "federation"
	// RestrictMediaFetch keeps media from being fetched and shown (a domain's reject_media).
	RestrictMediaFetch Restriction = "media_fetch"
	// RestrictReportIntake drops received reports (a domain's reject_reports).
	RestrictReportIntake Restriction = "report_intake"
)

// Valid reports whether r is one of the restrictions.
func (r Restriction) Valid() bool {
	switch r {
	case RestrictLogin, RestrictPost, RestrictInteract, RestrictVisibilityCeilingPrivate, RestrictPublicDiscovery,
		RestrictUnsolicitedReach, RestrictMediaSensitive, RestrictProfileView, RestrictFederation, RestrictMediaFetch,
		RestrictReportIntake:
		return true
	}
	return false
}

// InEffect reports whether a restriction that lasts until until (nil: until lifted) applies at
// now. An expired restriction needs no write to end: readers check it.
func InEffect(until *time.Time, now time.Time) bool {
	return until == nil || until.After(now)
}

// Effect is one restriction in effect on an account or a domain, merged over the actions that
// impose it: it lasts until Until, the latest end among them (nil: until lifted).
type Effect struct {
	Restriction Restriction
	Until       *time.Time
}

// Merge merges the restriction rows of one account or domain into the restrictions in effect at
// now, in restriction order: several actions imposing one restriction make one Effect that lasts
// as long as the longest of them (the union of the actions).
func Merge(rows []RestrictionRow, now time.Time) []Effect {
	byRestriction := map[Restriction]*Effect{}
	for _, r := range rows {
		if !InEffect(r.EffectiveUntil, now) {
			continue
		}
		e, ok := byRestriction[r.Restriction]
		switch {
		case !ok:
			byRestriction[r.Restriction] = &Effect{Restriction: r.Restriction, Until: r.EffectiveUntil}
		case e.Until == nil:
		case r.EffectiveUntil == nil || r.EffectiveUntil.After(*e.Until):
			e.Until = r.EffectiveUntil
		}
	}
	out := make([]Effect, 0, len(byRestriction))
	for _, e := range byRestriction {
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b Effect) int { return strings.Compare(string(a.Restriction), string(b.Restriction)) })
	return out
}

// RestrictionRow is one row of account_restrictions or domain_restrictions: a restriction that
// one action imposes on its target (an account ID or a domain) until EffectiveUntil (nil: until
// lifted).
type RestrictionRow struct {
	Target         string
	Restriction    Restriction
	SourceActionID string
	EffectiveUntil *time.Time
}

// DomainAndParents returns a domain followed by each of its parent domains, from the nearest:
// "a.b.example" gives a.b.example, b.example, and example. A domain action covers its
// subdomains, so these are the domains whose restrictions apply to a server.
// The empty domain (a local account) has none.
func DomainAndParents(domain string) []string {
	var out []string
	for domain != "" {
		out = append(out, domain)
		_, parent, ok := strings.Cut(domain, ".")
		if !ok {
			break
		}
		domain = parent
	}
	return out
}
