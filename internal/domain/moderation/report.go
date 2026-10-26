// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxReportComment is the number of characters of a report's comment that are kept; a received
// Flag's longer comment is cut (federation.md §11).
const MaxReportComment = 5000

// Bounds of a report filed on this server (FR-701): its comment, and how many statuses and
// rules it points at.
const (
	MaxLocalReportComment  = 1000
	MaxLocalReportStatuses = 20
	MaxLocalReportRules    = 20
)

// Report is a report of an account to the moderators (reports, a resource; FR-701).
type Report struct {
	ID string
	// URI is the id of the received Flag, or the URI synthesized for a Flag without one; "" for
	// a local report.
	URI               string
	ReporterAccountID string
	TargetAccountID   string
	Category          Category
	Comment           string
	// Forwarded reports that the report was forwarded to the target's server.
	Forwarded bool
	FiledAt   time.Time
	// StatusIDs are the statuses of the target that the report points at (report_statuses).
	StatusIDs []string
	// RuleIDs are the instance rules that the report cites (report_rule_citations); only a
	// report of CategoryViolation cites rules.
	RuleIDs []string
	// VoiceRoom is the voice chat room that a local report points at (report_voice_rooms,
	// ADR-0100 §11), nil for none; the target is the host of the room when it was reported.
	VoiceRoom *ReportVoiceRoom
}

// ReportVoiceRoom is the voice chat room of a report with the revision of its settings when it
// was reported. The room and its revisions are the voice module's.
type ReportVoiceRoom struct {
	RoomID     string
	RevisionID string
}

// ErrInvalidReport reports a report with a missing or invalid field.
var ErrInvalidReport = errors.New("moderation: invalid report")

// Validate checks the invariants of a new report.
func (r Report) Validate() error {
	switch {
	case r.ID == "" || r.ReporterAccountID == "" || r.TargetAccountID == "" || r.FiledAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidReport)
	case !r.Category.Valid():
		return fmt.Errorf("%w: category %q", ErrInvalidReport, r.Category)
	case len(r.RuleIDs) > 0 && r.Category != CategoryViolation:
		return fmt.Errorf("%w: a report of category %q cites rules", ErrInvalidReport, r.Category)
	case r.VoiceRoom != nil && (r.VoiceRoom.RoomID == "" || r.VoiceRoom.RevisionID == ""):
		return fmt.Errorf("%w: the voice chat room has an empty field", ErrInvalidReport)
	case r.VoiceRoom != nil && (r.URI != "" || r.Forwarded || len(r.StatusIDs) > 0):
		return fmt.Errorf("%w: a report of a voice chat room is local and points at no statuses", ErrInvalidReport)
	}
	if err := checkText("comment", r.Comment, MaxReportComment); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidReport, err)
	}
	return nil
}

// CountError reports a list of a record with more than Max items.
type CountError struct {
	// Field is the list, such as status_ids or rule_ids.
	Field string
	Max   int
}

func (e *CountError) Error() string {
	return fmt.Sprintf("the %s has more than %d items", e.Field, e.Max)
}

// ValidateLocal checks the invariants of a new report filed on this server: those of Validate,
// no URI, a comment of at most MaxLocalReportComment characters, and at most
// MaxLocalReportStatuses statuses and MaxLocalReportRules rules. A comment that fails fails with
// a *TextError, a list that is too long with a *CountError, both wrapped with ErrInvalidReport.
func (r Report) ValidateLocal() error {
	if err := r.Validate(); err != nil {
		return err
	}
	switch {
	case r.URI != "":
		return fmt.Errorf("%w: a local report has a URI", ErrInvalidReport)
	case len(r.StatusIDs) > MaxLocalReportStatuses:
		return fmt.Errorf("%w: %w", ErrInvalidReport, &CountError{Field: "status_ids", Max: MaxLocalReportStatuses})
	case len(r.RuleIDs) > MaxLocalReportRules:
		return fmt.Errorf("%w: %w", ErrInvalidReport, &CountError{Field: "rule_ids", Max: MaxLocalReportRules})
	}
	if err := checkText("comment", r.Comment, MaxLocalReportComment); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidReport, err)
	}
	return nil
}

// TruncateComment returns the first MaxReportComment characters of comment.
func TruncateComment(comment string) string {
	if utf8.RuneCountInString(comment) <= MaxReportComment {
		return comment
	}
	n := 0
	for i := range comment {
		if n == MaxReportComment {
			return comment[:i]
		}
		n++
	}
	return comment
}

// ReportState is whether a report awaits the moderators (report_states.state).
type ReportState string

// Report states.
const (
	ReportOpen     ReportState = "open"
	ReportResolved ReportState = "resolved"
)

// ReportStatus is the current state of a report (the report_states projection).
type ReportStatus struct {
	ReportID string
	State    ReportState
	// Outcome is that of the latest resolution while the report is resolved, "" while open.
	Outcome        ReportOutcome
	AssigneeUserID string // "" for none
	// Since is when the report entered its state: when it was filed, resolved, or reopened.
	Since time.Time
}

// ReportOutcome is how a moderator resolved a report (report_resolutions.outcome).
type ReportOutcome string

// Report outcomes.
const (
	// OutcomeActioned: the moderator took an action on the target.
	OutcomeActioned ReportOutcome = "actioned"
	// OutcomeDismissed: the moderator found nothing to act on.
	OutcomeDismissed ReportOutcome = "dismissed"
)

// Valid reports whether o is one of the report outcomes.
func (o ReportOutcome) Valid() bool { return o == OutcomeActioned || o == OutcomeDismissed }

// ReportAssignment is the event that a moderator took a report, or gave it up
// (report_assignments).
type ReportAssignment struct {
	ID       string
	ReportID string
	// AssigneeUserID is the moderator who handles the report; "" gives it up.
	AssigneeUserID   string
	AssignedByUserID string // "" for the CLI
	AssignedAt       time.Time
}

// ReportResolution is the event that a moderator resolved a report (report_resolutions).
type ReportResolution struct {
	ID               string
	ReportID         string
	Outcome          ReportOutcome
	ResolvedByUserID string // "" for the CLI
	ResolvedAt       time.Time
}

// ReportReopening is the event that a moderator reopened a resolved report
// (report_reopenings).
type ReportReopening struct {
	ID               string
	ReportID         string
	ReopenedByUserID string // "" for the CLI
	ReopenedAt       time.Time
}

// MaxReportNote is the number of characters of a note on a report.
const MaxReportNote = 2000

// ReportNote is a moderator's internal note on a report (report_notes, an event). Only the
// moderators see it: never the target, nor the reporter.
type ReportNote struct {
	ID           string
	ReportID     string
	AuthorUserID string // "" for the CLI
	Body         string
	WrittenAt    time.Time
}

// ErrInvalidReportNote reports a note with a missing or invalid field.
var ErrInvalidReportNote = errors.New("moderation: invalid report note")

// Validate checks the invariants of a new note: a body that is not blank, of at most
// MaxReportNote characters. A body that is too long or not valid text fails with a *TextError
// wrapped with ErrInvalidReportNote.
func (n ReportNote) Validate() error {
	switch {
	case n.ID == "" || n.ReportID == "" || n.WrittenAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidReportNote)
	case strings.TrimSpace(n.Body) == "":
		return fmt.Errorf("%w: the body is blank", ErrInvalidReportNote)
	}
	if err := checkText("body", n.Body, MaxReportNote); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidReportNote, err)
	}
	return nil
}

// TargetHistory counts what the moderators had about the target of a report: the target's
// other reports, and the actions on the target, lifted and expired ones included.
type TargetHistory struct {
	// Reports counts the target's reports other than the one shown.
	Reports int64
	// Actions counts the actions on the target.
	Actions int64
}

// ReportDetail is a report with its state, its notes oldest first, and its target's history,
// as a moderator opens it.
type ReportDetail struct {
	ReportRecord
	Notes   []ReportNote
	History TargetHistory
}

// ReportRecord is a report with its current state, as the moderators list it.
type ReportRecord struct {
	Report Report
	Status ReportStatus
}
