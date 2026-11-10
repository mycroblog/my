// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

// This file holds drafts and scheduled posts (data-model §4.4, ADR-0156): a draft is a local post
// saved on the server and read and written by its author alone, never federated. A schedule
// publishes it at a chosen time.

// Draft is a draft of a local post (drafts, a resource).
type Draft struct {
	ID        string
	AccountID string
	CreatedAt time.Time
}

// DraftAttachment is a media file that a draft attaches, with its alt text (draft_attachments).
type DraftAttachment struct {
	Position int
	MediaID  string
	// Description is the alt text, "" for none.
	Description string
}

// DraftPoll is the poll of a draft (draft_polls, draft_poll_options). Its end is set when the
// draft is published, Duration after.
type DraftPoll struct {
	Options    []string
	Duration   time.Duration
	Multiple   bool
	HideTotals bool
}

// DraftContent is the content of a draft (draft_contents and its children), replaced by every
// save.
type DraftContent struct {
	DraftID     string
	Text        string
	SpoilerText string
	Sensitive   bool
	// Language is a BCP 47 tag, "" for the author's default post language.
	Language   string
	Visibility Visibility
	// InReplyToID and QuotedStatusID are the statuses replied to and quoted, "" for none.
	InReplyToID    string
	QuotedStatusID string
	// InteractionPolicy is who may reply to, boost, and quote the post, of the kinds the author
	// chose; the kinds left out take the author's defaults when the draft is published.
	InteractionPolicy map[InteractionKind]InteractionSetting
	Attachments       []DraftAttachment
	// Poll is the poll, nil for none.
	Poll      *DraftPoll
	UpdatedAt time.Time
}

// Bounds of what a draft keeps. A draft keeps text the post limits would refuse, since its
// author is still writing it; these bound the rows alone, and publication checks the rest.
const (
	// MaxDraftTextLength is the number of characters of the text and content warning of a draft.
	MaxDraftTextLength = 100000
	// MaxDraftAttachments is the number of media files of a draft.
	MaxDraftAttachments = 4
	// MaxDraftMediaDescription is the number of characters of an alt text.
	MaxDraftMediaDescription = 1500
)

// ErrInvalidDraft reports draft content outside the bounds of a draft.
var ErrInvalidDraft = errors.New("content: the draft is not valid")

// Validate reports ErrInvalidDraft for content that a draft cannot keep: text that is not
// valid UTF-8 or has a NUL character, text longer than MaxDraftTextLength, an invalid visibility
// or interaction policy, more than MaxDraftAttachments files or one file twice, an alt text
// longer than MaxDraftMediaDescription, a poll with media, or a poll with more than
// MaxLocalPollOptions choices, a choice longer than MaxPollOptionLength, or a duration outside
// [MinPollDuration, MaxPollDuration].
func (c DraftContent) Validate() error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrInvalidDraft}, args...)...)
	}
	for _, v := range []string{c.Text, c.SpoilerText} {
		if !ValidText(v) {
			return invalid("the text is not valid")
		}
	}
	if n := utf8.RuneCountInString(c.Text) + utf8.RuneCountInString(c.SpoilerText); n > MaxDraftTextLength {
		return invalid("%d characters, at most %d", n, MaxDraftTextLength)
	}
	if !ValidText(c.Language) || len(c.Language) > 64 {
		return invalid("the language is not valid")
	}
	if !c.Visibility.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidVisibility, c.Visibility)
	}
	for k, set := range c.InteractionPolicy {
		if !k.Valid() {
			return fmt.Errorf("%w: kind %q", ErrInvalidInteractionPolicy, k)
		}
		if err := set.Validate(); err != nil {
			return err
		}
	}
	if len(c.Attachments) > MaxDraftAttachments {
		return invalid("%d media files, at most %d", len(c.Attachments), MaxDraftAttachments)
	}
	for i, a := range c.Attachments {
		if a.MediaID == "" || slices.ContainsFunc(c.Attachments[:i], func(o DraftAttachment) bool { return o.MediaID == a.MediaID }) {
			return invalid("media %q is attached twice or not named", a.MediaID)
		}
		if !ValidText(a.Description) || utf8.RuneCountInString(a.Description) > MaxDraftMediaDescription {
			return invalid("the description of media %s is not valid", a.MediaID)
		}
	}
	if p := c.Poll; p != nil {
		if len(c.Attachments) > 0 {
			return invalid("a poll attaches no media")
		}
		if len(p.Options) > MaxLocalPollOptions {
			return invalid("%d poll choices, at most %d", len(p.Options), MaxLocalPollOptions)
		}
		for _, o := range p.Options {
			if !ValidText(o) || utf8.RuneCountInString(o) > MaxPollOptionLength {
				return invalid("a poll choice is not valid")
			}
		}
		if p.Duration < MinPollDuration || p.Duration > MaxPollDuration || p.Duration%time.Second != 0 {
			return invalid("the poll duration %s is outside [%s, %s]", p.Duration, MinPollDuration, MaxPollDuration)
		}
	}
	return nil
}

// DraftSchedule is one schedule of a draft (draft_schedules, a resource).
type DraftSchedule struct {
	ID        string
	DraftID   string
	PublishAt time.Time
	CreatedAt time.Time
}

// DraftOutcome is how a schedule ended (draft_schedule_outcomes.outcome).
type DraftOutcome string

// Outcomes of a schedule. A published draft leaves no rows (its schedules go with it), so the
// outcomes that stay are cancelled and failed.
const (
	DraftPublished DraftOutcome = "published"
	DraftFailed    DraftOutcome = "failed"
	DraftCancelled DraftOutcome = "cancelled"
)

// DraftFailureReason is why a scheduled draft was not published
// (draft_schedule_outcomes.failure_reason), shown to its author.
type DraftFailureReason string

// Failure reasons of a schedule.
const (
	// DraftFailureRestricted: the author may not post (a moderation action, a suspension, or an
	// account that is not active).
	DraftFailureRestricted DraftFailureReason = "restricted"
	// DraftFailureReplyUnavailable: the status replied to is gone, hidden, or does not take the
	// reply.
	DraftFailureReplyUnavailable DraftFailureReason = "reply_unavailable"
	// DraftFailureQuoteUnavailable: the status quoted is gone, hidden, or may not be quoted.
	DraftFailureQuoteUnavailable DraftFailureReason = "quote_unavailable"
	// DraftFailureMediaUnavailable: an attached file is gone or failed processing.
	DraftFailureMediaUnavailable DraftFailureReason = "media_unavailable"
	// DraftFailureInvalid: the content does not pass the checks of a post (such as its length).
	DraftFailureInvalid DraftFailureReason = "invalid"
	// DraftFailureRejected: the server's automated review refused the post.
	DraftFailureRejected DraftFailureReason = "rejected"
)

// DraftFailureReasons are the failure reasons in the order they are presented.
var DraftFailureReasons = []DraftFailureReason{
	DraftFailureRestricted, DraftFailureReplyUnavailable, DraftFailureQuoteUnavailable,
	DraftFailureMediaUnavailable, DraftFailureInvalid, DraftFailureRejected,
}

// Valid reports whether r is a known failure reason.
func (r DraftFailureReason) Valid() bool { return slices.Contains(DraftFailureReasons, r) }

// DraftScheduleOutcome is the end of a schedule (draft_schedule_outcomes, an event).
type DraftScheduleOutcome struct {
	ScheduleID string
	Outcome    DraftOutcome
	// FailureReason is set for DraftFailed alone.
	FailureReason DraftFailureReason
	OccurredAt    time.Time
}

// ScheduledDraft is the schedule in effect of a draft (scheduled_drafts, a projection).
type ScheduledDraft struct {
	DraftID    string
	ScheduleID string
	AccountID  string
	PublishAt  time.Time
}

// DraftIdempotencyKey is the Idempotency-Key of the post that publishing a draft creates, so that
// a second run of the publication finds the post of the first (local_statuses).
func DraftIdempotencyKey(draftID string) string { return "draft:" + draftID }

// DraftDay returns the UTC day of t, which max_scheduled_posts_per_day counts by: its start and
// the start of the next day.
func DraftDay(t time.Time) (time.Time, time.Time) {
	t = t.UTC()
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 1)
}
