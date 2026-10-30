// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"strings"
	"time"
	"unicode/utf8"
)

// The requests of the holders of create_custom_emojis (ADR-0070 §2, §3, §7): a request to create
// a local emoji, or to replace the image of one the requester made, that a holder of
// manage_custom_emojis approves or rejects. A request is a resource (custom_emoji_requests); its
// end is an event (custom_emoji_request_outcomes); and a request without an outcome is pending,
// which the projection custom_emoji_pending_requests keeps, with the shortcode that a pending
// request to create reserves.

// CustomEmojiRequestKind is what a request asks for.
type CustomEmojiRequestKind string

// Kinds of request.
const (
	// RequestCreate asks for a new local emoji.
	RequestCreate CustomEmojiRequestKind = "create"
	// RequestImage asks for a new image of a local emoji that the requester made.
	RequestImage CustomEmojiRequestKind = "image"
)

// Valid reports whether k is a known kind.
func (k CustomEmojiRequestKind) Valid() bool { return k == RequestCreate || k == RequestImage }

// CustomEmojiRequestOutcomeKind is how a request ended.
type CustomEmojiRequestOutcomeKind string

// Outcomes of a request.
const (
	RequestApproved  CustomEmojiRequestOutcomeKind = "approved"
	RequestRejected  CustomEmojiRequestOutcomeKind = "rejected"
	RequestWithdrawn CustomEmojiRequestOutcomeKind = "withdrawn"
	// RequestLapsed closes a request to replace the image of an emoji that was deleted.
	RequestLapsed CustomEmojiRequestOutcomeKind = "lapsed"
)

// CustomEmojiRequestState is the state of a request: pending, or its outcome.
type CustomEmojiRequestState string

// RequestPending is the state of a request without an outcome; the other states are the
// outcomes.
const RequestPending CustomEmojiRequestState = "pending"

// CustomEmojiRequestStates lists every state, in a stable order.
var CustomEmojiRequestStates = []CustomEmojiRequestState{
	RequestPending, CustomEmojiRequestState(RequestApproved), CustomEmojiRequestState(RequestRejected),
	CustomEmojiRequestState(RequestWithdrawn), CustomEmojiRequestState(RequestLapsed),
}

// Valid reports whether s is a known state.
func (s CustomEmojiRequestState) Valid() bool {
	for _, k := range CustomEmojiRequestStates {
		if s == k {
			return true
		}
	}
	return false
}

// MaxCustomEmojiRejectReason bounds the reason of a rejection, in characters.
const MaxCustomEmojiRejectReason = 500

// CustomEmojiRequest is a request (custom_emoji_requests). EmojiID is the emoji whose image a
// RequestImage replaces, "" for a RequestCreate; Shortcode is the shortcode of the new emoji, or
// a copy of the target's. MediaID is an upload of the requester's local account.
type CustomEmojiRequest struct {
	ID              string
	Kind            CustomEmojiRequestKind
	RequesterUserID string
	EmojiID         string
	Shortcode       string
	MediaID         string
	CreatedAt       time.Time
}

// Label returns how the audit log names a request (ADR-0070 §8): :shortcode:.
func (r CustomEmojiRequest) Label() string { return ":" + r.Shortcode + ":" }

// CustomEmojiRequestOutcome is the end of a request (custom_emoji_request_outcomes), at most one
// per request. DecidedByUserID is "" for the CLI, a withdrawal's requester is recorded, and a
// lapse has none; Reason is the optional reason of a rejection; EmojiID is the emoji that an
// approval made or changed.
type CustomEmojiRequestOutcome struct {
	ID              string
	RequestID       string
	Outcome         CustomEmojiRequestOutcomeKind
	DecidedByUserID string
	Reason          string
	EmojiID         string
	DecidedAt       time.Time
}

// CustomEmojiRequestRecord is a request with its outcome, nil while it is pending.
type CustomEmojiRequestRecord struct {
	Request CustomEmojiRequest
	Outcome *CustomEmojiRequestOutcome
}

// State returns the state of the request.
func (r CustomEmojiRequestRecord) State() CustomEmojiRequestState {
	if r.Outcome == nil {
		return RequestPending
	}
	return CustomEmojiRequestState(r.Outcome.Outcome)
}

// NormalizeCustomEmojiRejectReason trims the reason of a rejection and checks it: valid text of
// at most MaxCustomEmojiRejectReason characters. "" is no reason.
func NormalizeCustomEmojiRejectReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if !ValidText(reason) || utf8.RuneCountInString(reason) > MaxCustomEmojiRejectReason {
		return "", emojiError(EmojiInvalidReason, reason)
	}
	return reason, nil
}

// CustomEmojiLimitPeriod returns the month that the limit of a creator counts at now
// (ADR-0124 §1): the UTC calendar month, from since, the first day at 00:00 UTC, until the first
// day of the next month at 00:00 UTC, when the count resets.
func CustomEmojiLimitPeriod(now time.Time) (since, until time.Time) {
	now = now.UTC()
	since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return since, since.AddDate(0, 1, 0)
}
