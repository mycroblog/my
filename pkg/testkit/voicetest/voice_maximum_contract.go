// SPDX-License-Identifier: AGPL-3.0-only

package voicetest

import (
	"testing"

	"github.com/mycroblog/my/pkg/testkit"
)

// voiceMaximum checks that a revision keeps the room's maximum (ADR-0100 §3), none as 0, and
// that a new revision replaces it: FindRoom and OpenRooms read the current one.
func voiceMaximum(t *testing.T, e *voiceEnv) {
	host, other := testkit.NamedID("voice-host"), testkit.NamedID("voice-other")
	s := voiceSettings("Capped")
	s.MaxParticipants = 3
	capped := e.openRoom(t, host, s, true)
	free := e.openRoom(t, other, voiceSettings("Free"), true)
	if got := e.find(t, capped.ID).Revision.Settings.MaxParticipants; got != 3 {
		t.Errorf("FindRoom: the maximum = %d, want 3", got)
	}
	if got := e.find(t, free.ID).Revision.Settings.MaxParticipants; got != 0 {
		t.Errorf("FindRoom: the maximum of a room without one = %d, want 0", got)
	}

	for _, n := range []int{500, 2, 0} {
		s.MaxParticipants = n
		e.revise(t, capped.ID, host, s)
		if got := e.find(t, capped.ID).Revision.Settings.MaxParticipants; got != n {
			t.Errorf("FindRoom after a revision to %d: the maximum = %d", n, got)
		}
	}

	s.MaxParticipants = 7
	e.revise(t, capped.ID, host, s)
	recs, err := e.h.Voice.OpenRooms(t.Context(), nil, "")
	if err != nil {
		t.Fatalf("OpenRooms: %v", err)
	}
	got := map[string]int{}
	for _, r := range recs {
		got[r.ID] = r.Revision.Settings.MaxParticipants
	}
	if len(got) != 2 || got[capped.ID] != 7 || got[free.ID] != 0 {
		t.Errorf("OpenRooms: the maximums = %v, want %s: 7 and %s: 0", got, capped.ID, free.ID)
	}
}
