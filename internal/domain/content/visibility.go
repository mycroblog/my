// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
)

// Visibility is the audience a status is posted to (ADR-0017, FR-102). It never changes after
// posting (statuses.visibility).
type Visibility string

// Visibilities, from the widest audience to the narrowest.
const (
	VisibilityPublic   Visibility = "public"
	VisibilityLocal    Visibility = "local"
	VisibilityUnlisted Visibility = "unlisted"
	VisibilityPrivate  Visibility = "private"
	VisibilityDirect   Visibility = "direct"
)

// Visibilities lists every visibility from the widest audience to the narrowest.
func Visibilities() []Visibility {
	return []Visibility{VisibilityPublic, VisibilityLocal, VisibilityUnlisted, VisibilityPrivate, VisibilityDirect}
}

// ErrInvalidVisibility reports a value that is not one of the five visibilities.
var ErrInvalidVisibility = errors.New("invalid visibility")

// ParseVisibility returns the visibility named s.
func ParseVisibility(s string) (Visibility, error) {
	v := Visibility(s)
	if !v.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidVisibility, s)
	}
	return v, nil
}

// Valid reports whether v is one of the five visibilities.
func (v Visibility) Valid() bool { return v.rank() >= 0 }

// rank orders visibilities from the widest audience (0) to the narrowest (4); -1 means invalid.
func (v Visibility) rank() int {
	switch v {
	case VisibilityPublic:
		return 0
	case VisibilityLocal:
		return 1
	case VisibilityUnlisted:
		return 2
	case VisibilityPrivate:
		return 3
	case VisibilityDirect:
		return 4
	default:
		return -1
	}
}

// Narrower returns whichever of a and b has the narrower audience: min(a, b) in the order
// public > local > unlisted > private > direct. An invalid value is treated as narrowest of all and
// returned as is, so callers fail closed.
func Narrower(a, b Visibility) Visibility {
	if !a.Valid() {
		return a
	}
	if !b.Valid() || b.rank() > a.rank() {
		return b
	}
	return a
}

// VisibilityCeiling returns the widest visibility the author may currently use: private when the
// author carries visibility_ceiling_private (silence), public otherwise.
func (r AuthorRestrictions) VisibilityCeiling() Visibility {
	if r.VisibilityCeilingPrivate {
		return VisibilityPrivate
	}
	return VisibilityPublic
}

// EffectiveVisibility is the visibility a status is judged by on this server:
// min(original, ceiling) (ADR-0058 §11). The stored value is never rewritten, so lifting the
// ceiling restores the original audience. The same function caps the requested visibility when
// a status is posted.
func EffectiveVisibility(original Visibility, author AuthorRestrictions) Visibility {
	return Narrower(original, author.VisibilityCeiling())
}

// PostingVisibilities returns the visibilities that a new status of the author keeps as asked for,
// from the widest audience to the narrowest: those that EffectiveVisibility leaves unchanged.
// Clients offer these and say that any other one becomes the ceiling (ADR-0058 §11).
func PostingVisibilities(author AuthorRestrictions) []Visibility {
	var out []Visibility
	for _, v := range Visibilities() {
		if EffectiveVisibility(v, author) == v {
			out = append(out, v)
		}
	}
	return out
}
