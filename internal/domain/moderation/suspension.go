// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"slices"
	"time"
)

// FederationSuspended reports whether the restriction rows of one account suspend its
// federation at now: a row of RestrictFederation (suspend, ban) is in effect.
func FederationSuspended(rows []RestrictionRow, now time.Time) bool {
	return slices.ContainsFunc(rows, func(r RestrictionRow) bool {
		return r.Restriction == RestrictFederation && InEffect(r.EffectiveUntil, now)
	})
}

// SuspensionChange compares the restriction rows of one account before and after a change
// (an action, a lift, or the cleanup of expired rows) and tells whether the account's
// suspension changed in a way that other servers must learn (federation.md §11.1, ADR-0058):
//
//   - The account became suspended: no RestrictFederation row was in effect before, and one is
//     after.
//   - The account stopped being suspended: a RestrictFederation row was removed, and none is in
//     effect after. The removed row may have expired already: the expiry is told when its row
//     is removed, by the lift or by the cleanup, whichever comes first, and only once.
//
// changed is false when there is nothing to tell; suspended is the state after the change.
func SuspensionChange(before, after []RestrictionRow, now time.Time) (changed, suspended bool) {
	was, is := FederationSuspended(before, now), FederationSuspended(after, now)
	if is {
		return !was, true
	}
	removed := slices.ContainsFunc(before, func(b RestrictionRow) bool {
		return b.Restriction == RestrictFederation && !slices.ContainsFunc(after, func(a RestrictionRow) bool {
			return a.Restriction == b.Restriction && a.SourceActionID == b.SourceActionID
		})
	})
	return removed, false
}
