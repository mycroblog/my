// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// The retention of the moderation records (ADR-0038, data-model §4.3, §8). Each kind of record is
// erased after its period, counted from the time below, unless a hold on its account stops it:
//
//   - a report: from its last resolution while it is resolved; an open report, and one that an
//     action follows from, stay (the latter as long as the action);
//   - an action on an account with its appeal: from the end of the action, the later of its
//     lift or expiry and the decision on its appeal. An action that imposes nothing lasting
//     (warn, delete_statuses) ends when it is issued; one without an end that is not lifted
//     (a ban, an indefinite suspension) stays while it is in effect, and so does one whose
//     appeal is pending;
//   - an audit log entry: from when it was recorded;
//   - a sign-up block: from its end, the earlier of its lift and its expiry, for the fixed
//     SignupBlockRetention; one without an end that is not lifted stays while it is in effect,
//     and a hold on the account whose ban made the block keeps it.

// Default retention periods (the defaults of the instance settings reports_retention_days,
// moderation_actions_retention_days, and audit_log_retention_days).
const (
	DefaultReportRetention   = 365 * 24 * time.Hour
	DefaultActionRetention   = 365 * 24 * time.Hour
	DefaultAuditLogRetention = 730 * 24 * time.Hour
)

// SignupBlockRetention is how long a sign-up block is kept after it ends (ADR-0038). It is not
// an instance setting.
const SignupBlockRetention = 30 * 24 * time.Hour

// MaxRetentionHoldReason is the number of characters of the reason of a hold.
const MaxRetentionHoldReason = 1000

// ErrInvalidRetentionHold reports a hold with a missing or invalid field.
var ErrInvalidRetentionHold = errors.New("moderation: invalid retention hold")

// RetentionHold stops the erasure of an account's records after their retention: its moderation
// records, its sign-in and registration origins, and its statuses (retention_holds, an event).
// It lasts until it is released.
type RetentionHold struct {
	ID        string
	AccountID string
	// Reason is why the records are preserved, such as a court order.
	Reason          string
	CreatedByUserID string // "" for the CLI
	CreatedAt       time.Time
}

// Validate checks the invariants of a new hold: its fields and a reason of 1 to
// MaxRetentionHoldReason characters that is not only white space.
func (h RetentionHold) Validate() error {
	if h.ID == "" || h.AccountID == "" || h.CreatedAt.IsZero() {
		return fmt.Errorf("%w: a field is empty", ErrInvalidRetentionHold)
	}
	if err := checkText("reason", h.Reason, MaxRetentionHoldReason); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRetentionHold, err)
	}
	if strings.TrimSpace(h.Reason) == "" {
		return fmt.Errorf("%w: %w", ErrInvalidRetentionHold, &TextError{Field: "reason"})
	}
	return nil
}

// RetentionHoldRelease is the event that a hold was released (retention_hold_releases).
type RetentionHoldRelease struct {
	HoldID           string
	ReleasedByUserID string // "" for the CLI
	ReleasedAt       time.Time
}

// RetentionHoldRecord is a hold with its release.
type RetentionHoldRecord struct {
	Hold    RetentionHold
	Release *RetentionHoldRelease // nil while the hold is in effect
}

// Active reports whether the hold still stops the erasure.
func (r RetentionHoldRecord) Active() bool { return r.Release == nil }
