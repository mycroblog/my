// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Role badges (ADR-0070, FR-913). A role has no badge or one, made of a label and a color. Any
// role may have one, the built-in ones included, and none has one by default. The label is the
// staff's own text, the same in every locale, which clients render as user content; the color
// is one of the badge colors of the design tokens, whose text and background pairs keep their
// contrast in every theme. A user shows on their profile the badges of the roles they hold that
// they chose to show; the choice is off until they turn it on, and it goes away when they lose
// the role or the role loses its badge.

// BadgeColor is the color of a role badge, the name of a pair of design tokens
// (badge-<color> and badge-<color>-fg of @my/ui).
type BadgeColor string

// The badge colors (data-model §4.1, role_badges.color).
const (
	BadgeNeutral BadgeColor = "neutral"
	BadgeRed     BadgeColor = "red"
	BadgeOrange  BadgeColor = "orange"
	BadgeYellow  BadgeColor = "yellow"
	BadgeGreen   BadgeColor = "green"
	BadgeTeal    BadgeColor = "teal"
	BadgeBlue    BadgeColor = "blue"
	BadgePurple  BadgeColor = "purple"
	BadgePink    BadgeColor = "pink"
)

// AllBadgeColors lists every badge color, in the order the staff choose from.
func AllBadgeColors() []BadgeColor {
	return []BadgeColor{
		BadgeNeutral, BadgeRed, BadgeOrange, BadgeYellow, BadgeGreen, BadgeTeal, BadgeBlue, BadgePurple, BadgePink,
	}
}

// Valid reports whether c is a badge color.
func (c BadgeColor) Valid() bool {
	return slices.Contains(AllBadgeColors(), c)
}

// MaxBadgeLabelLength is the length limit of a badge's label, in characters.
const MaxBadgeLabelLength = 32

// ErrInvalidBadgeLabel reports a label that is empty, longer than MaxBadgeLabelLength
// characters, not valid UTF-8, or more than one line.
var ErrInvalidBadgeLabel = errors.New("identity: a badge label is 1 to 32 characters on one line")

// ErrInvalidBadgeColor reports a color that is not a badge color.
var ErrInvalidBadgeColor = errors.New("identity: unknown badge color")

// RoleBadge is the badge of a role (data-model §4.1, role_badges).
type RoleBadge struct {
	Label     string
	Color     BadgeColor
	UpdatedAt time.Time
}

// NormalizeRoleBadge checks a badge's label and color and returns the label without the white
// space around it.
func NormalizeRoleBadge(label string, color BadgeColor) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" || !utf8.ValidString(label) || utf8.RuneCountInString(label) > MaxBadgeLabelLength {
		return "", ErrInvalidBadgeLabel
	}
	for _, c := range label {
		if unicode.IsControl(c) || c == ' ' || c == ' ' {
			return "", ErrInvalidBadgeLabel
		}
	}
	if !color.Valid() {
		return "", ErrInvalidBadgeColor
	}
	return label, nil
}

// SameBadge reports whether a and b show the same badge, whenever they were set.
func SameBadge(a, b *RoleBadge) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Label == b.Label && a.Color == b.Color
}

// ProfileBadge is a badge that a user shows on their profile.
type ProfileBadge struct {
	RoleID string
	Label  string
	Color  BadgeColor
}

// BadgeChoice is a role that a user holds and that has a badge, with whether the user shows the
// badge on their profile.
type BadgeChoice struct {
	ProfileBadge
	Displayed bool
}

// KeepsBadgeDisplay reports whether a user who shows (displayed) the badge of a role that is
// replaced by another (a change of plan) shows the badge of the new role: the choice moves when
// the new role has a badge, and goes away when it has none.
func KeepsBadgeDisplay(displayed, newRoleHasBadge bool) bool {
	return displayed && newRoleHasBadge
}
