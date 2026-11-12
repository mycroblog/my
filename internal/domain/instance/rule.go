// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits of the instance rules (FR-705).
const (
	// MaxRuleTextLength bounds the text of a rule, in characters.
	MaxRuleTextLength = 300
	// MaxRuleHintLength bounds the hint of a rule, in characters.
	MaxRuleHintLength = 1000
	// MaxRules bounds the rules in effect at once.
	MaxRules = 100
)

var (
	// ErrRuleRetired reports a change of a retired rule, which can no longer change.
	ErrRuleRetired = errors.New("instance: rule is retired")
	// ErrTooManyRules reports a new rule beyond MaxRules.
	ErrTooManyRules = errors.New("instance: too many rules")
)

// RuleTextError reports a rule text or hint that a revision cannot hold. It wraps
// ErrInvalidValue.
type RuleTextError struct {
	// Field is "text" or "hint".
	Field string
	// Max is the limit that the value exceeds, or 0 when it is malformed.
	Max int
	Why string
}

func (e *RuleTextError) Error() string {
	return fmt.Sprintf("rule %s: %s", e.Field, e.Why)
}

// Unwrap makes errors.Is(err, ErrInvalidValue) hold.
func (e *RuleTextError) Unwrap() error { return ErrInvalidValue }

// ValidateRuleText checks the text and the hint of one rule revision: a single-line,
// non-blank text and a hint ("" for none) within their limits, valid UTF-8 without U+0000.
// They hold one value, written in any language and shown as it is in every
// locale.
func ValidateRuleText(text, hint string) error {
	fail := func(field string, limit int, why string) error {
		return &RuleTextError{Field: field, Max: limit, Why: why}
	}
	if err := checkRuleString(text, MaxRuleTextLength, true); err != "" {
		return fail("text", maxIfTooLong(err, MaxRuleTextLength), err)
	}
	if strings.TrimSpace(text) == "" {
		return fail("text", 0, "is blank")
	}
	if err := checkRuleString(hint, MaxRuleHintLength, false); err != "" {
		return fail("hint", maxIfTooLong(err, MaxRuleHintLength), err)
	}
	return nil
}

const tooLong = "too long"

func maxIfTooLong(why string, limit int) int {
	if why == tooLong {
		return limit
	}
	return 0
}

func checkRuleString(s string, limit int, singleLine bool) string {
	switch {
	case !utf8.ValidString(s):
		return "not valid UTF-8"
	case strings.ContainsRune(s, 0):
		return "contains NUL"
	case utf8.RuneCountInString(s) > limit:
		return tooLong
	case singleLine && strings.ContainsAny(s, "\r\n"):
		return "contains a line break"
	}
	return ""
}

// RuleRevision is one version of a rule (instance_rule_revisions): its position among the
// rules, its text, and its hint. The revision with the greatest ID is the current one of its
// rule.
type RuleRevision struct {
	ID       string
	RuleID   string
	Position int
	Text     string
	// Hint is an optional explanation of the rule, "" when there is none.
	Hint string
	// RevisedByUserID is the user who made the change, or "" for a change from the CLI.
	RevisedByUserID string
	RevisedAt       time.Time
}

// RuleRetirement records that a rule no longer applies (instance_rule_retirements). Reports
// that cite it still refer to it.
type RuleRetirement struct {
	RuleID string
	// RetiredByUserID is the user who retired the rule, or "" for the CLI.
	RetiredByUserID string
	RetiredAt       time.Time
}

// RuleHead points at the current revision of a rule in effect (the instance_rule_heads
// projection).
type RuleHead struct {
	RuleID     string
	RevisionID string
	Position   int
}

// Supersedes reports whether r should replace the head h, which is the zero value when the
// rule has none yet.
func (r RuleRevision) Supersedes(h RuleHead) bool {
	return h.RevisionID == "" || r.ID > h.RevisionID
}

// Rule is a rule with its current revision, and its retirement if it is retired.
type Rule struct {
	ID        string
	CreatedAt time.Time
	Revision  RuleRevision
	// Retirement is nil for a rule in effect.
	Retirement *RuleRetirement
}

// Retired reports whether the rule no longer applies.
func (r Rule) Retired() bool { return r.Retirement != nil }
