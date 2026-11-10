// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import "time"

// The date poll of an event (ADR-0161 §7): an event created without a fixed date has candidates,
// which the people who see it answer yes, maybe, or no until an organizer confirms one of them.

// MaxCandidates is how many candidates, not withdrawn, an event polls at most.
const MaxCandidates = 20

// Vote is an answer to a candidate of a date poll (gathering_votes.vote).
type Vote string

// The answers to a candidate.
const (
	VoteYes   Vote = "yes"
	VoteMaybe Vote = "maybe"
	VoteNo    Vote = "no"
)

// Valid reports whether v is a known vote.
func (v Vote) Valid() bool { return v == VoteYes || v == VoteMaybe || v == VoteNo }

// ValidateCandidates checks the candidates of a new date poll at now: 1 to MaxCandidates of them,
// each a valid schedule (Schedule.Validate), and no two the same. It returns a *SettingsError of
// FieldCandidates.
func ValidateCandidates(candidates []Schedule, now time.Time) error {
	if len(candidates) == 0 || len(candidates) > MaxCandidates {
		return &SettingsError{Field: FieldCandidates}
	}
	for i, c := range candidates {
		if c.Validate(now) != nil || SameSchedule(candidates[:i], c) {
			return &SettingsError{Field: FieldCandidates}
		}
	}
	return nil
}

// SameSchedule reports whether one of schedules starts and ends at the same instants as s.
func SameSchedule(schedules []Schedule, s Schedule) bool {
	for _, o := range schedules {
		if o.StartsAt.Equal(s.StartsAt) && o.EndsAt.Equal(s.EndsAt) {
			return true
		}
	}
	return false
}
