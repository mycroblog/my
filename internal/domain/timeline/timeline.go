// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"strings"
	"time"
)

// Kind says what a feed entry shows: a status as posted, or a boost of one.
type Kind string

// Entry kinds (feed_entries.kind).
const (
	KindPost  Kind = "post"
	KindBoost Kind = "boost"
)

// ExclusionReason says why a viewer does not see an account's entries (feed_exclusions.reason).
type ExclusionReason string

// Exclusion reasons.
const (
	// ExcludedBlock: the viewer blocks the account.
	ExcludedBlock ExclusionReason = "block"
	// ExcludedBlockedBy: the account blocks the viewer.
	ExcludedBlockedBy ExclusionReason = "blocked_by"
	// ExcludedMute: the viewer mutes the account, until the exclusion expires.
	ExcludedMute ExclusionReason = "mute"
)

// Restriction is a moderation restriction of an account or a domain that bears on timelines
// (feed_author_restrictions and feed_domain_restrictions).
type Restriction string

// Restrictions that timelines apply.
const (
	// RestrictProfileView hides the account's entries from everyone but itself and moderators.
	RestrictProfileView Restriction = "profile_view"
	// RestrictVisibilityCeilingPrivate makes every post of the account followers-only.
	RestrictVisibilityCeilingPrivate Restriction = "visibility_ceiling_private"
	// RestrictPublicDiscovery keeps the account's posts off the local and hashtag timelines.
	RestrictPublicDiscovery Restriction = "public_discovery"
)

// UndeterminedLanguage is the language of an entry whose language is unknown. Language filters
// always let such entries through (FR-208).
const UndeterminedLanguage = "und"

// The read window of the home timeline (data-model §6.1, ADR-0013). Fan-out-on-read bounds each
// home query to the entries newer than the cursor minus a window, so that the rows a page reads
// stay proportional to recent activity. When a page is not filled within the window, the query
// runs again with a window HomeWindowGrowth times wider, up to HomeWindowWidenings times, and
// finally without a bound, so that a quiet home timeline still reaches its oldest entries.
const (
	// HomeWindow is the first window. Seven days covers the reading habit of most accounts on a
	// small instance; the measurement of ADR-0013 may revise it.
	HomeWindow = 7 * 24 * time.Hour
	// HomeWindowGrowth is the factor by which each widening multiplies the window.
	HomeWindowGrowth = 4
	// HomeWindowWidenings is how many times a window widens before the query drops the bound.
	HomeWindowWidenings = 3
)

// HomeWindows returns the bounded windows of the home timeline, narrowest first: HomeWindow
// and its HomeWindowWidenings widenings. After the last of them, the query is unbounded.
func HomeWindows() []time.Duration {
	ws := []time.Duration{HomeWindow}
	for range HomeWindowWidenings {
		ws = append(ws, ws[len(ws)-1]*HomeWindowGrowth)
	}
	return ws
}

// DomainAndParents returns domain followed by each of its parent domains, from the nearest:
// "a.b.example" gives a.b.example, b.example, and example. A domain restriction covers its
// subdomains, so these are the domains whose restrictions apply to an account on domain. The
// local domain "" has none.
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
