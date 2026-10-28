// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// RegistrationMode says who may sign up (FR-001). It mirrors the instance setting
// registration_mode, which the instance module owns.
type RegistrationMode string

// Registration modes.
const (
	RegistrationOpen     RegistrationMode = "open"     // anyone may sign up
	RegistrationApproval RegistrationMode = "approval" // sign-ups wait for a moderator's approval
	RegistrationInvite   RegistrationMode = "invite"   // sign-up needs an invite
	RegistrationClosed   RegistrationMode = "closed"   // nobody may sign up
)

// RegistrationApplication is a sign-up from the registration page (data-model §4.1,
// registration_applications). Each user has at most one.
type RegistrationApplication struct {
	ID     string
	UserID string
	// Reason is why the applicant wants to join, asked in approval mode; "" when not given.
	Reason string
	// InviteID is the invite redeemed at sign-up, in any mode that takes invites; "" when none.
	InviteID    string
	SubmittedAt time.Time
}

// MaxRegistrationReasonLength bounds RegistrationApplication.Reason, in characters.
const MaxRegistrationReasonLength = 1000

// ErrInvalidReason reports a registration reason that ValidateRegistrationReason rejects.
var ErrInvalidReason = errors.New("invalid registration reason")

// ValidateRegistrationReason accepts valid UTF-8 without NUL (data-model §1.2) of at most
// MaxRegistrationReasonLength characters.
func ValidateRegistrationReason(reason string) error {
	if !validText(reason, MaxRegistrationReasonLength) {
		return fmt.Errorf("%w: must be at most %d characters of text", ErrInvalidReason, MaxRegistrationReasonLength)
	}
	return nil
}

// MaxDecisionNoteLength bounds RegistrationDecision.Note, in characters.
const MaxDecisionNoteLength = 1000

// ErrInvalidNote reports a decision note that ValidateDecisionNote rejects.
var ErrInvalidNote = errors.New("invalid registration decision note")

// ValidateDecisionNote accepts valid UTF-8 without NUL (data-model §1.2) of at most
// MaxDecisionNoteLength characters.
func ValidateDecisionNote(note string) error {
	if !validText(note, MaxDecisionNoteLength) {
		return fmt.Errorf("%w: must be at most %d characters of text", ErrInvalidNote, MaxDecisionNoteLength)
	}
	return nil
}

// validText reports whether s is valid UTF-8 without NUL (data-model §1.2) of at most maxLen
// characters: a moderator's or an applicant's free text that every database dialect stores as
// it is.
func validText(s string, maxLen int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) <= maxLen
}

// Decision is the outcome of a registration review.
type Decision string

// Decisions (data-model §4.1, registration_decisions.decision).
const (
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
)

// Decider says who decided on an application.
type Decider string

// Deciders (data-model §4.1, registration_decisions.decided_by).
const (
	// DecidedByModerator: a moderator, in the admin screens or with the CLI.
	DecidedByModerator Decider = "moderator"
	// DecidedBySystem: the server, which approves sign-ups at once in open and invite mode, and
	// those with a usable invite in approval mode (ADR-0108 §4).
	DecidedBySystem Decider = "system"
	// DecidedByServiceClosure: the server, which rejects the pending applications when the
	// closure of the service is announced (ADR-0088 §2).
	DecidedByServiceClosure Decider = "service_closure"
)

// RegistrationDecision records that an application was approved or rejected (data-model §4.1,
// registration_decisions).
type RegistrationDecision struct {
	ID            string
	ApplicationID string
	Decision      Decision
	DecidedBy     Decider
	// DecidedByUserID is the moderator who decided, or "" for the system and the CLI.
	DecidedByUserID string
	// Note is the moderator's note; "" when none.
	Note      string
	DecidedAt time.Time
}

// RegistrationState is where a user's registration stands.
type RegistrationState string

// Registration states (data-model §4.1, user_registration_states.state).
const (
	RegistrationPending  RegistrationState = "pending"
	RegistrationApproved RegistrationState = "approved"
	RegistrationRejected RegistrationState = "rejected"
)

// State returns the registration state that d leads to.
func (d Decision) State() RegistrationState {
	if d == DecisionApproved {
		return RegistrationApproved
	}
	return RegistrationRejected
}

// UserRegistrationState is the current registration state of a user (the
// user_registration_states projection). A user without one was created with the CLI and is
// approved.
type UserRegistrationState struct {
	UserID string
	State  RegistrationState
	// Since is when the user entered the state.
	Since time.Time
}

// Invite is an invite code that lets people sign up (data-model §4.1, invites): it is required
// in invite mode and optional in open and approval mode (ADR-0108 §4).
type Invite struct {
	ID   string
	Code string
	// CreatedByUserID is the user who created the invite, or "" for the CLI.
	CreatedByUserID string
	// CreatorKind is how the invite was created: by the staff or the CLI, or by a member under
	// the caps, whose eligibility every sign-up checks again (ADR-0108 §3).
	CreatorKind InviteCreatorKind
	// MaxUses is how many sign-ups the invite allows; 0 means no limit.
	MaxUses int
	// ExpiresAt is when the invite stops working; nil means never.
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// Expired reports whether the invite has expired at now.
func (i Invite) Expired(now time.Time) bool {
	return i.ExpiresAt != nil && !now.Before(*i.ExpiresAt)
}

// Exhausted reports whether an invite with uses redemptions allows no more.
func (i Invite) Exhausted(uses int) bool { return i.MaxUses > 0 && uses >= i.MaxUses }

// InviteRedemption is one use of an invite (invite_redemptions). Seq is 1 for the first use
// and one more for each later one, so that the primary key (InviteID, Seq) keeps concurrent
// sign-ups from exceeding MaxUses.
type InviteRedemption struct {
	InviteID   string
	Seq        int
	UserID     string
	RedeemedAt time.Time
}

// InviteRevocation records that an invite was revoked (invite_revocations).
type InviteRevocation struct {
	InviteID string
	// RevokedByUserID is the user who revoked it, or "" for the CLI.
	RevokedByUserID string
	RevokedAt       time.Time
}

// InviteCodeLength is the length of an invite code: 80 random bits in base32.
const InviteCodeLength = 16

// NormalizeInviteCode returns the form invite codes are stored and looked up in: upper case,
// without the spaces and hyphens people add when they copy or type a code.
func NormalizeInviteCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, strings.ToUpper(code))
}

// TokenPurpose is what a user token is for.
type TokenPurpose string

// Token purposes (data-model §4.1, user_tokens.purpose). The verification and reset tokens work
// once; an unsubscribe token works any number of times until it expires (ADR-0086 §4).
const (
	PurposeEmailVerification TokenPurpose = "email_verification"
	PurposePasswordReset     TokenPurpose = "password_reset"
	PurposeEmailUnsubscribe  TokenPurpose = "email_unsubscribe"
)

// UnsubscribeCategory is a category of email that its recipient can turn off from the email
// itself (ADR-0086 §4, user_tokens.unsubscribe_category): a group of the notification settings
// whose email the user chooses (ADR-0109 §4), such as "mention" or "admin_report".
type UnsubscribeCategory string

// UserToken is a token sent to a user, such as the link of a verification email (data-model
// §4.1, user_tokens). Only the SHA-256 digest of the token is stored (ADR-0019 §4).
type UserToken struct {
	ID      string
	UserID  string
	Purpose TokenPurpose
	// EmailAddressID is the address an email verification token confirms; "" for other purposes.
	EmailAddressID string
	// UnsubscribeCategory is the category of email that an unsubscribe token turns off; "" for
	// other purposes.
	UnsubscribeCategory UnsubscribeCategory
	TokenHash           []byte
	ExpiresAt           time.Time
	IssuedAt            time.Time
}

// Expired reports whether the token has expired at now.
func (t UserToken) Expired(now time.Time) bool { return !now.Before(t.ExpiresAt) }

// UserTokenRedemption records that a token was used (user_token_redemptions); a token is used
// at most once.
type UserTokenRedemption struct {
	TokenID    string
	RedeemedAt time.Time
}

// EmailVerification records that a user confirmed an email address (user_email_verifications).
type EmailVerification struct {
	EmailAddressID string
	VerifiedAt     time.Time
}
