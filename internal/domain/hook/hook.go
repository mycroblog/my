// SPDX-License-Identifier: AGPL-3.0-only

// Package hook holds the rules of the decision hooks (ADR-0147): the fixed vocabulary of what a
// plugin may answer, what each hook accepts and does when the plugin fails, and which decision
// has an effect or stays past its retention. The domain knows nothing of the plugins.
package hook

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Name names a hook. The names are also the kinds of the plugin settings (ADR-0146 §2).
type Name string

// The hooks (ADR-0147 §2).
const (
	InboundStatus Name = "inbound.status"
	InboundFollow Name = "inbound.follow"
	SignupCheck   Name = "signup.check"
	MediaCheck    Name = "media.check"
	LocalStatus   Name = "local.status"
)

// Names lists the hooks in documentation order.
func Names() []Name {
	return []Name{InboundStatus, InboundFollow, SignupCheck, MediaCheck, LocalStatus}
}

// Mode says how far a hook acts. Only a decision of ModeEnforce has an effect (ADR-0147 §5).
type Mode string

// The modes of a hook.
const (
	// ModeOff calls no plugin.
	ModeOff Mode = "off"
	// ModeObserve records what the plugin answers and acts on nothing.
	ModeObserve Mode = "observe"
	// ModeEnforce acts on the verdict.
	ModeEnforce Mode = "enforce"
)

// Outcome is what a decision records: a verdict of the plugin, or the kind of its failure, which
// is never a verdict (ADR-0147 §4).
type Outcome string

// The verdicts (ADR-0147 §1).
const (
	Allow           Outcome = "allow"
	Drop            Outcome = "drop"
	Flag            Outcome = "flag"
	MarkSensitive   Outcome = "mark_sensitive"
	RequireApproval Outcome = "require_approval"
	Reject          Outcome = "reject"
)

// The kinds of failure.
const (
	// FailedTimeout is a call past its deadline.
	FailedTimeout Outcome = "timeout"
	// FailedError is a call that failed in any other way.
	FailedError Outcome = "error"
	// FailedInvalid is an answer outside the vocabulary of the hook, an empty one included.
	FailedInvalid Outcome = "invalid"
	// FailedBreakerOpen is a call that the circuit breaker skipped.
	FailedBreakerOpen Outcome = "breaker_open"
)

// Outcomes lists every outcome, the verdicts first.
func Outcomes() []Outcome {
	return []Outcome{
		Allow, Drop, Flag, MarkSensitive, RequireApproval, Reject,
		FailedTimeout, FailedError, FailedInvalid, FailedBreakerOpen,
	}
}

// Failed reports whether the outcome is a failure instead of a verdict.
func (o Outcome) Failed() bool {
	switch o {
	case FailedTimeout, FailedError, FailedInvalid, FailedBreakerOpen:
		return true
	}
	return false
}

// LeavesState reports whether acting on the verdict leaves something that lasts, which the audit
// log records with the system as the actor. Drop and Reject store nothing, so they stay in the
// decisions alone, whose retention is short (ADR-0147 §11).
func (o Outcome) LeavesState() bool {
	switch o {
	case MarkSensitive, Flag, RequireApproval:
		return true
	}
	return false
}

// Reason is the coded reason of a verdict. Free text goes in the note alone (ADR-0147 §1).
type Reason string

// The reasons. ReasonNone is a verdict that gives none.
const (
	ReasonNone       Reason = ""
	ReasonSpam       Reason = "spam"
	ReasonHarassment Reason = "harassment"
	ReasonSexual     Reason = "sexual"
	ReasonViolence   Reason = "violence"
	ReasonIllegal    Reason = "illegal"
	ReasonMalware    Reason = "malware"
	ReasonOther      Reason = "other"
)

// Valid reports whether r is one of the reasons.
func (r Reason) Valid() bool {
	switch r {
	case ReasonNone, ReasonSpam, ReasonHarassment, ReasonSexual, ReasonViolence, ReasonIllegal, ReasonMalware, ReasonOther:
		return true
	}
	return false
}

// OnFailure is what a hook does when the plugin fails and its mode is ModeEnforce.
type OnFailure string

// The failure behaviours (ADR-0147 §2, §4).
const (
	// PassOnFailure lets the subject through. It is the default of every hook.
	PassOnFailure OnFailure = "pass"
	// RetryOnFailure fails the job, which runs again and calls the plugin again, and records no
	// decision. The ordering key of an inbound hook holds back the actor's later jobs meanwhile.
	RetryOnFailure OnFailure = "retry"
	// ApproveOnFailure treats the subject as RequireApproval.
	ApproveOnFailure OnFailure = "require_approval"
)

// Spec is what a hook accepts and how it behaves on failure (ADR-0147 §2).
type Spec struct {
	Name Name
	// Verdicts are the verdicts the hook acts on; any other answer of a plugin is FailedInvalid.
	Verdicts []Outcome
	// OnFailure lists the failure behaviours an operator may choose, the default first.
	OnFailure []OnFailure
}

var specs = map[Name]Spec{
	InboundStatus: {
		Name: InboundStatus, Verdicts: []Outcome{Allow, Drop, Flag, MarkSensitive},
		OnFailure: []OnFailure{PassOnFailure, RetryOnFailure},
	},
	InboundFollow: {
		Name: InboundFollow, Verdicts: []Outcome{Allow, Drop, Flag},
		OnFailure: []OnFailure{PassOnFailure, RetryOnFailure},
	},
	SignupCheck: {
		Name: SignupCheck, Verdicts: []Outcome{Allow, RequireApproval, Reject},
		OnFailure: []OnFailure{PassOnFailure, ApproveOnFailure},
	},
	// A reject of media.check keeps nothing of the upload: its original is gone once it is
	// processed (ADR-0147 §2).
	MediaCheck: {
		Name: MediaCheck, Verdicts: []Outcome{Allow, MarkSensitive, Flag, Reject},
		OnFailure: []OnFailure{PassOnFailure, RetryOnFailure},
	},
	LocalStatus: {
		Name: LocalStatus, Verdicts: []Outcome{Allow, Flag, Reject},
		OnFailure: []OnFailure{PassOnFailure},
	},
}

// SpecOf returns the spec of a hook.
func SpecOf(n Name) (Spec, bool) {
	s, ok := specs[n]
	return s, ok
}

// Accepts reports whether the hook acts on the verdict.
func (s Spec) Accepts(o Outcome) bool { return slices.Contains(s.Verdicts, o) }

// AcceptsFailure reports whether an operator may choose the behaviour for the hook.
func (s Spec) AcceptsFailure(f OnFailure) bool { return slices.Contains(s.OnFailure, f) }

// MaxNote bounds the note of a decision, in characters (ADR-0147 §1).
const MaxNote = 200

// CleanNote returns the note of a plugin as the moderators read it: plain text on one line of at
// most MaxNote characters. The plugin is not trusted to keep to that.
func CleanNote(note string) string {
	note = strings.ToValidUTF8(note, "")
	note = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, note)
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > MaxNote {
		note = strings.TrimSpace(string([]rune(note)[:MaxNote]))
	}
	return note
}

// ErrInvalidDecision reports a decision with a missing or invalid field.
var ErrInvalidDecision = errors.New("hook: invalid decision")

// Decision is what a hook answered about one subject (hook_decisions, an event). It holds no text
// of a post, no email address, and no IP address.
type Decision struct {
	ID   string
	Hook Name
	// SubjectKey identifies the subject for the hook, such as the URI of a received activity: one
	// decision per hook and subject.
	SubjectKey string
	// AccountID is the account the subject belongs to, "" when there is none.
	AccountID string
	// Mode is ModeObserve or ModeEnforce, the mode when the decision was made.
	Mode      Mode
	Outcome   Outcome
	Reason    Reason
	Note      string
	Latency   time.Duration
	DecidedAt time.Time
}

// Validate checks the invariants of a new decision.
func (d Decision) Validate() error {
	if d.ID == "" || d.SubjectKey == "" || d.DecidedAt.IsZero() {
		return fmt.Errorf("%w: a field is empty", ErrInvalidDecision)
	}
	spec, known := SpecOf(d.Hook)
	switch {
	case !known:
		return fmt.Errorf("%w: hook %q", ErrInvalidDecision, d.Hook)
	case d.Mode != ModeObserve && d.Mode != ModeEnforce:
		return fmt.Errorf("%w: mode %q", ErrInvalidDecision, d.Mode)
	case !d.Outcome.Failed() && !spec.Accepts(d.Outcome):
		return fmt.Errorf("%w: %s does not take %q", ErrInvalidDecision, d.Hook, d.Outcome)
	case !d.Reason.Valid():
		return fmt.Errorf("%w: reason %q", ErrInvalidDecision, d.Reason)
	case utf8.RuneCountInString(d.Note) > MaxNote || !utf8.ValidString(d.Note) || strings.ContainsRune(d.Note, 0):
		return fmt.Errorf("%w: the note", ErrInvalidDecision)
	case d.Latency < 0:
		return fmt.Errorf("%w: negative latency", ErrInvalidDecision)
	}
	return nil
}

// Override is a moderator's cancellation of a decision (hook_decision_overrides, an event).
type Override struct {
	DecisionID string
	// ByUserID is the moderator, "" for the CLI.
	ByUserID string
	At       time.Time
}

// Record is a decision with its cancellation.
type Record struct {
	Decision Decision
	Override *Override // nil while the decision stands
}

// Effect returns what the owner of the subject acts on: always Allow in ModeObserve and for a
// cancelled decision, the verdict in ModeEnforce, and for a failure what onFailure says.
func (r Record) Effect(onFailure OnFailure) Outcome {
	d := r.Decision
	switch {
	case d.Mode != ModeEnforce || r.Override != nil:
		return Allow
	case !d.Outcome.Failed():
		return d.Outcome
	case onFailure == ApproveOnFailure:
		return RequireApproval
	}
	return Allow
}

// Overridable reports whether a moderator can cancel the decision from the list: a verdict of
// ModeEnforce that stopped or changed something and is not cancelled. A reject of local.status is
// cancelled through its appeal, which a person reviews with the text the author sends (ADR-0147 §9).
func (r Record) Overridable() bool {
	d := r.Decision
	if r.Override != nil || d.Mode != ModeEnforce || d.Outcome.Failed() || d.Outcome == Allow {
		return false
	}
	return !d.Hook.Appealable() || d.Outcome != Reject
}

// Expired is what the retention of the decisions reads of one past its retention period.
type Expired struct {
	ID         string
	Hook       Name
	AccountID  string
	Mode       Mode
	Outcome    Outcome
	DecidedAt  time.Time
	Overridden bool
	// AppealPending says that its author appealed it and no moderator has decided the appeal yet.
	AppealPending bool
	// StatusIDs are the statuses the decision had an effect on that still exist.
	StatusIDs []string
}

// RetentionPolicy says which decisions past their retention period stay (ADR-0147 §6).
type RetentionPolicy struct {
	// Held are the accounts that a retention hold preserves (ADR-0058 §8).
	Held map[string]bool
	// AppealsOpenSince is the time from which a reject of local.status can still be appealed.
	AppealsOpenSince time.Time
}

// Keeps reports whether the decision stays: the projection status_sensitivity_overrides points at
// it and could not be rebuilt without it, a hold preserves its account, or its author can still
// appeal it or waits for the review of an appeal.
func (p RetentionPolicy) Keeps(e Expired) bool {
	switch {
	case e.AccountID != "" && p.Held[e.AccountID]:
		return true
	case e.Mode != ModeEnforce || e.Overridden:
		return false
	case e.Outcome == MarkSensitive && len(e.StatusIDs) > 0:
		return true
	case e.Hook.Appealable() && e.Outcome == Reject && (e.AppealPending || !e.DecidedAt.Before(p.AppealsOpenSince)):
		return true
	}
	return false
}

// EditSubjectKey is the subject key of local.status for an edit of a local post: the post and the
// new revision, so that each edit is its own subject (ADR-0147 §2). A post's own key is its ID.
func EditSubjectKey(statusID, revisionID string) string { return statusID + "#" + revisionID }

// IsEditSubject reports whether a subject key of local.status is that of an edit
// (EditSubjectKey). A reject of an edit refuses that edit only and cannot be appealed.
func IsEditSubject(key string) bool { return strings.Contains(key, "#") }

// Appealable reports whether the author of a subject can appeal a reject of the hook: only
// local.status, whose author is a local account and whose reject no moderator sees otherwise
// (ADR-0147 §9). An overturned appeal exempts the account from the hook.
func (n Name) Appealable() bool { return n == LocalStatus }

// The bounds of an appeal, in characters. The text of the post follows the largest
// max_status_length; the comment, those of the appeals of moderation actions.
const (
	MaxAppealStatusText = 100000
	MaxAppealComment    = 2000
)

// The failures of an appeal.
var (
	// ErrNotAppealable reports a decision that its author cannot appeal: not a reject of a new
	// post by local.status in force (a reject of an edit is not appealed), or already cancelled.
	ErrNotAppealable = errors.New("hook: the decision cannot be appealed")
	// ErrAppealWindowClosed reports an appeal after the appeal window.
	ErrAppealWindowClosed = errors.New("hook: the appeal window is closed")
	// ErrInvalidAppeal reports an appeal whose text is blank, too long, or not valid.
	ErrInvalidAppeal = errors.New("hook: invalid appeal")
)

// AppealDeadline returns the last moment the author can appeal the decision: its time plus the
// appeal window (appeal_window_days) at the time of the appeal.
func AppealDeadline(d Decision, window time.Duration) time.Time { return d.DecidedAt.Add(window) }

// CheckAppealable returns why the record cannot be appealed at now, nil when it can.
func (r Record) CheckAppealable(now time.Time, window time.Duration) error {
	d := r.Decision
	switch {
	case !d.Hook.Appealable() || d.Mode != ModeEnforce || d.Outcome != Reject || r.Override != nil || IsEditSubject(d.SubjectKey):
		return ErrNotAppealable
	case now.After(AppealDeadline(d, window)):
		return ErrAppealWindowClosed
	}
	return nil
}

// Appeal is an author's appeal of a reject of local.status, at most one per decision. It holds the
// text the author wanted to post, since the decision holds none and a person reviews it.
type Appeal struct {
	ID         string
	DecisionID string
	// AccountID is the author, the account of the decision.
	AccountID string
	// StatusText is the text of the post the author tried to post.
	StatusText string
	// Comment is what the author adds for the moderators, "" for none.
	Comment     string
	SubmittedAt time.Time
}

// Validate checks the fields of a new appeal.
func (a Appeal) Validate() error {
	switch {
	case a.ID == "" || a.DecisionID == "" || a.AccountID == "" || a.SubmittedAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidAppeal)
	case strings.TrimSpace(a.StatusText) == "":
		return &AppealTextError{Field: "status_text"}
	}
	for _, f := range []struct {
		name, value string
		max         int
	}{{"status_text", a.StatusText, MaxAppealStatusText}, {"comment", a.Comment, MaxAppealComment}} {
		if !utf8.ValidString(f.value) || strings.ContainsRune(f.value, 0) {
			return &AppealTextError{Field: f.name}
		}
		if utf8.RuneCountInString(f.value) > f.max {
			return &AppealTextError{Field: f.name, Max: f.max}
		}
	}
	return nil
}

// AppealTextError is an ErrInvalidAppeal of one field: blank or not valid, or longer than Max.
type AppealTextError struct {
	Field string
	// Max is the limit that the text passes, 0 when it is blank or not valid.
	Max int
}

func (e *AppealTextError) Error() string {
	if e.Max > 0 {
		return fmt.Sprintf("%v: %s is longer than %d characters", ErrInvalidAppeal, e.Field, e.Max)
	}
	return fmt.Sprintf("%v: %s is blank or not valid", ErrInvalidAppeal, e.Field)
}

// Unwrap returns ErrInvalidAppeal.
func (e *AppealTextError) Unwrap() error { return ErrInvalidAppeal }

// AppealOutcome is what a moderator decided on an appeal.
type AppealOutcome string

// The outcomes of an appeal.
const (
	// AppealUpheld leaves the decision as it is.
	AppealUpheld AppealOutcome = "upheld"
	// AppealOverturned cancels the decision and exempts the account from the hook.
	AppealOverturned AppealOutcome = "overturned"
)

// Valid reports whether o is an outcome.
func (o AppealOutcome) Valid() bool { return o == AppealUpheld || o == AppealOverturned }

// AppealDecision is a moderator's decision on an appeal, at most one.
type AppealDecision struct {
	AppealID string
	Outcome  AppealOutcome
	// ByUserID is the moderator, "" for the CLI.
	ByUserID  string
	DecidedAt time.Time
}

// AppealRecord is an appeal with the decision it appeals and its review, nil while pending.
type AppealRecord struct {
	Appeal   Appeal
	Decision Record
	Review   *AppealDecision
}

// Exemption says that a hook no longer reviews an account, because a moderator overturned the
// account's appeal. The row is the current value; it does not follow the decision or the appeal,
// which retention removes.
type Exemption struct {
	Hook       Name
	AccountID  string
	ExemptedAt time.Time
}
