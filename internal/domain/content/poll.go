// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// This file holds the votes of accounts in polls (FR-112, data-model §4.4). A vote with several
// choices is one PollVote per choice, and votes are never taken back.

var (
	// ErrNoPoll reports a vote on a status whose current revision has no poll.
	ErrNoPoll = errors.New("content: the status has no poll")
	// ErrPollClosed reports a vote in a poll whose end has passed.
	ErrPollClosed = errors.New("content: the poll is closed")
	// ErrAlreadyVoted reports a vote of an account that voted in the poll already.
	ErrAlreadyVoted = errors.New("content: the account voted in the poll already")
	// ErrInvalidPollChoices reports choices that the poll does not take: none, an unknown
	// position, the same position twice, or several in a single-choice poll.
	ErrInvalidPollChoices = errors.New("content: invalid poll choices")
	// ErrPollChanged reports a vote in a poll that is no longer the status's: an edit replaced it.
	ErrPollChanged = errors.New("content: the poll of the status changed")
	// ErrInvalidPoll reports a new local poll that breaks the limits of FR-112 (NewPoll).
	ErrInvalidPoll = errors.New("content: invalid poll")
)

// Limits of a local poll (FR-112). They are Mastodon's defaults, which the servers that read
// the Question also accept.
const (
	// MinPollOptions and MaxLocalPollOptions bound the choices of a local poll.
	MinPollOptions      = 2
	MaxLocalPollOptions = 4
	// MaxPollOptionLength is the number of characters (Unicode code points) of a choice.
	MaxPollOptionLength = 50
	// MinPollDuration and MaxPollDuration bound how long a local poll takes votes.
	MinPollDuration = 5 * time.Minute
	MaxPollDuration = 30 * 24 * time.Hour
)

// PollInput is a new local poll as its author describes it.
type PollInput struct {
	// Options are the titles of the choices, in order: plain text.
	Options []string
	// Duration is how long the poll takes votes, from when the status is posted.
	Duration   time.Duration
	Multiple   bool
	HideTotals bool
}

// NewPoll checks in against the limits of a local poll and returns the poll id that ends
// in.Duration after now, to the second. It fails with ErrInvalidPoll on fewer than
// MinPollOptions or more than MaxLocalPollOptions choices, a blank choice, one longer than
// MaxPollOptionLength, not valid UTF-8, or with a NUL character, the same choice twice, or a
// duration outside [MinPollDuration, MaxPollDuration]. The titles are trimmed.
func NewPoll(id string, in PollInput, now time.Time) (Poll, error) {
	if n := len(in.Options); n < MinPollOptions || n > MaxLocalPollOptions {
		return Poll{}, fmt.Errorf("%w: %d choices, %d to %d", ErrInvalidPoll, n, MinPollOptions, MaxLocalPollOptions)
	}
	if in.Duration < MinPollDuration || in.Duration > MaxPollDuration {
		return Poll{}, fmt.Errorf("%w: a duration of %s, %s to %s", ErrInvalidPoll, in.Duration, MinPollDuration, MaxPollDuration)
	}
	p := Poll{ID: id, Multiple: in.Multiple, HideTotals: in.HideTotals}
	for i, raw := range in.Options {
		title := strings.TrimSpace(raw)
		switch {
		case title == "":
			return Poll{}, fmt.Errorf("%w: choice %d is blank", ErrInvalidPoll, i)
		case !utf8.ValidString(title) || strings.ContainsRune(title, 0):
			return Poll{}, fmt.Errorf("%w: choice %d is not valid text", ErrInvalidPoll, i)
		case utf8.RuneCountInString(title) > MaxPollOptionLength:
			return Poll{}, fmt.Errorf("%w: choice %d is longer than %d characters", ErrInvalidPoll, i, MaxPollOptionLength)
		}
		if slices.ContainsFunc(p.Options, func(o PollOption) bool { return o.Title == title }) {
			return Poll{}, fmt.Errorf("%w: choice %q twice", ErrInvalidPoll, title)
		}
		p.Options = append(p.Options, PollOption{Position: i, Title: title})
	}
	closes := now.Add(in.Duration).Truncate(time.Second)
	p.ClosesAt = &closes
	return p, nil
}

// OptionByTitle returns the position of the first choice of p titled title, false when p has
// none: a remote vote names its choice by its title.
func (p Poll) OptionByTitle(title string) (int, bool) {
	title = strings.TrimSpace(title)
	for _, o := range p.Options {
		if o.Title == title {
			return o.Position, true
		}
	}
	return 0, false
}

// TotalsHidden reports whether the counts of p are hidden at now: its author chose to hide them
// until it ends, and it has not.
func (p Poll) TotalsHidden(now time.Time) bool {
	return p.HideTotals && !p.Closed(now)
}

// PollVote is one choice an account voted for in a poll.
type PollVote struct {
	ID        string
	PollID    string
	AccountID string
	Position  int
	// URI is the id of the Note that carries the choice to the poll's server, for a local
	// account's vote in a remote poll (PollVoteURI), or that a remote voter sent; "" otherwise.
	URI     string
	VotedAt time.Time
}

// PollVoteURI returns the id of the Note that carries a local account's vote with the ID id to a
// remote poll's server (ADR-0018: an object of this instance, /ap/objects/{uuid}). Its Create is
// at PollVoteURI + "/activity".
func PollVoteURI(host, id string) string {
	return "https://" + host + "/ap/objects/" + id
}

// Closed reports whether the end of p is at or before now; a poll without an end never closes.
func (p Poll) Closed(now time.Time) bool {
	return p.ClosesAt != nil && !p.ClosesAt.After(now)
}

// Choices checks choices, the positions an account votes for, against p and returns them in
// order. It fails with ErrInvalidPollChoices on none, a position that p does not have, a
// position given twice, or more than one in a single-choice poll.
func (p Poll) Choices(choices []int) ([]int, error) {
	if len(choices) == 0 {
		return nil, fmt.Errorf("%w: no choice", ErrInvalidPollChoices)
	}
	if !p.Multiple && len(choices) > 1 {
		return nil, fmt.Errorf("%w: %d choices in a single-choice poll", ErrInvalidPollChoices, len(choices))
	}
	out := slices.Clone(choices)
	slices.Sort(out)
	for i, c := range out {
		if c < 0 || c >= len(p.Options) {
			return nil, fmt.Errorf("%w: no choice %d", ErrInvalidPollChoices, c)
		}
		if i > 0 && out[i-1] == c {
			return nil, fmt.Errorf("%w: choice %d twice", ErrInvalidPollChoices, c)
		}
	}
	return out, nil
}

// Option returns the choice of p at position, false when p has none there.
func (p Poll) Option(position int) (PollOption, bool) {
	for _, o := range p.Options {
		if o.Position == position {
			return o, true
		}
	}
	return PollOption{}, false
}
