// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPollClosed(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	cases := []struct {
		name     string
		closesAt *time.Time
		want     bool
	}{
		{"no end", nil, false},
		{"ended", &past, true},
		{"ends now", &now, true},
		{"open", &future, false},
	}
	for _, c := range cases {
		if got := (Poll{ClosesAt: c.closesAt}).Closed(now); got != c.want {
			t.Errorf("%s: Closed = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPollChoices(t *testing.T) {
	options := []PollOption{{Position: 0, Title: "a"}, {Position: 1, Title: "b"}, {Position: 2, Title: "c"}}
	single := Poll{Options: options}
	multiple := Poll{Multiple: true, Options: options}
	cases := []struct {
		name    string
		poll    Poll
		choices []int
		want    []int
	}{
		{"single", single, []int{1}, []int{1}},
		{"multiple sorted", multiple, []int{2, 0}, []int{0, 2}},
		{"none", single, nil, nil},
		{"two in single", single, []int{0, 1}, nil},
		{"unknown", single, []int{3}, nil},
		{"negative", multiple, []int{-1}, nil},
		{"twice", multiple, []int{1, 1}, nil},
	}
	for _, c := range cases {
		got, err := c.poll.Choices(c.choices)
		if c.want == nil {
			if !errors.Is(err, ErrInvalidPollChoices) {
				t.Errorf("%s: err = %v, want ErrInvalidPollChoices", c.name, err)
			}
			continue
		}
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: Choices = %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

func TestPollOption(t *testing.T) {
	p := Poll{Options: []PollOption{{Position: 0, Title: "a"}, {Position: 1, Title: "b"}}}
	if o, ok := p.Option(1); !ok || o.Title != "b" {
		t.Errorf("Option(1) = %v, %v", o, ok)
	}
	if _, ok := p.Option(2); ok {
		t.Error("Option(2) found a choice")
	}
}

func TestPollVoteURI(t *testing.T) {
	if got, want := PollVoteURI("example.org", testID(1)), "https://example.org/ap/objects/"+testID(1); got != want {
		t.Errorf("PollVoteURI = %q, want %q", got, want)
	}
}

func TestNewPoll(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 500_000_000, time.UTC)
	ok := PollInput{Options: []string{" yes ", "no"}, Duration: time.Hour, Multiple: true, HideTotals: true}
	p, err := NewPoll(testID(1), ok, now)
	if err != nil {
		t.Fatalf("NewPoll = %v", err)
	}
	want := now.Add(time.Hour).Truncate(time.Second)
	if p.ID != testID(1) || !p.Multiple || !p.HideTotals || p.ClosesAt == nil || !p.ClosesAt.Equal(want) ||
		!slices.Equal(p.Options, []PollOption{{Position: 0, Title: "yes"}, {Position: 1, Title: "no"}}) {
		t.Errorf("NewPoll = %+v", p)
	}
	long := strings.Repeat("あ", MaxPollOptionLength+1)
	cases := []struct {
		name     string
		in       PollInput
		option   int
		duration bool
	}{
		{"one choice", PollInput{Options: []string{"a"}, Duration: time.Hour}, -1, false},
		{"too many choices", PollInput{Options: []string{"a", "b", "c", "d", "e"}, Duration: time.Hour}, -1, false},
		{"blank choice", PollInput{Options: []string{"a", " "}, Duration: time.Hour}, 1, false},
		{"long choice", PollInput{Options: []string{"a", long}, Duration: time.Hour}, 1, false},
		{"NUL", PollInput{Options: []string{"a", "b\x00"}, Duration: time.Hour}, 1, false},
		{"twice", PollInput{Options: []string{"a", " a"}, Duration: time.Hour}, 1, false},
		{"short", PollInput{Options: []string{"a", "b"}, Duration: MinPollDuration - time.Second}, -1, true},
		{"long", PollInput{Options: []string{"a", "b"}, Duration: MaxPollDuration + time.Second}, -1, true},
	}
	for _, c := range cases {
		_, err := NewPoll(testID(1), c.in, now)
		var inputErr *PollInputError
		if !errors.Is(err, ErrInvalidPoll) || !errors.As(err, &inputErr) || inputErr.Option != c.option || inputErr.Duration != c.duration {
			t.Errorf("%s: NewPoll = %#v, want ErrInvalidPoll about option %d (duration %t)", c.name, err, c.option, c.duration)
		}
	}
	if _, err := NewPoll(testID(1), PollInput{Options: []string{"a", strings.Repeat("あ", MaxPollOptionLength)}, Duration: MaxPollDuration}, now); err != nil {
		t.Errorf("NewPoll at the limits = %v", err)
	}
}

func TestPollOptionByTitleAndTotalsHidden(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Minute), now.Add(-time.Minute)
	p := Poll{Options: []PollOption{{Position: 0, Title: "a"}, {Position: 1, Title: "b"}}, HideTotals: true, ClosesAt: &future}
	if pos, ok := p.OptionByTitle(" b "); !ok || pos != 1 {
		t.Errorf("OptionByTitle(b) = %d, %v", pos, ok)
	}
	if _, ok := p.OptionByTitle("c"); ok {
		t.Error("OptionByTitle(c) found a choice")
	}
	if !p.TotalsHidden(now) {
		t.Error("TotalsHidden of an open poll that hides them = false")
	}
	p.ClosesAt = &past
	if p.TotalsHidden(now) {
		t.Error("TotalsHidden of an ended poll = true")
	}
}
