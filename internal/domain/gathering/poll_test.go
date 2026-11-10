// SPDX-License-Identifier: AGPL-3.0-only

package gathering_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/gathering"
)

func TestValidateCandidates(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	at := func(h int) gathering.Schedule {
		start := now.Add(time.Duration(h) * time.Hour)
		return gathering.Schedule{StartsAt: start, EndsAt: start.Add(time.Hour)}
	}
	many := make([]gathering.Schedule, gathering.MaxCandidates+1)
	for i := range many {
		many[i] = at(i + 1)
	}
	for _, tc := range []struct {
		name  string
		cands []gathering.Schedule
		ok    bool
	}{
		{"one", []gathering.Schedule{at(1)}, true},
		{"the most", many[:gathering.MaxCandidates], true},
		{"none", nil, false},
		{"too many", many, false},
		{"the same twice", []gathering.Schedule{at(1), at(2), at(1)}, false},
		{"in the past", []gathering.Schedule{at(-1)}, false},
		{"ends before it starts", []gathering.Schedule{{StartsAt: now.Add(time.Hour), EndsAt: now}}, false},
	} {
		err := gathering.ValidateCandidates(tc.cands, now)
		var se *gathering.SettingsError
		switch {
		case tc.ok && err != nil:
			t.Errorf("%s: %v, want nil", tc.name, err)
		case !tc.ok && (!errors.As(err, &se) || se.Field != gathering.FieldCandidates):
			t.Errorf("%s: %v, want a SettingsError of candidates", tc.name, err)
		}
	}
}

func TestVoteValid(t *testing.T) {
	t.Parallel()
	for _, v := range []gathering.Vote{gathering.VoteYes, gathering.VoteMaybe, gathering.VoteNo} {
		if !v.Valid() {
			t.Errorf("%q invalid", v)
		}
	}
	if gathering.Vote("going").Valid() {
		t.Error(`"going" valid`)
	}
}
