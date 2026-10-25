// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/domain/federation"
)

// TestAllows is the table of ADR-0076 (流れの分類), row by row: o passes, x does not, and r is
// refused with a Reject (which Allows reports as not passing).
func TestAllows(t *testing.T) {
	table := map[federation.Flow]string{
		//                     off inbound outbound both
		federation.FlowPublish:   "x x o o",
		federation.FlowIngest:    "x o x o",
		federation.FlowFollowOut: "x o x o",
		federation.FlowFollowIn:  "x r o o",
		federation.FlowRetract:   "x o o o",
		federation.FlowProtect:   "x o o o",
		federation.FlowIdentify:  "x o o o",
	}
	modes := federation.Modes()
	if len(table) != len(federation.Flows()) {
		t.Fatalf("the table has %d flows, want %d", len(table), len(federation.Flows()))
	}
	for _, flow := range federation.Flows() {
		row, ok := table[flow]
		if !ok {
			t.Fatalf("flow %s is not in the table", flow)
		}
		cells := []byte{row[0], row[2], row[4], row[6]}
		for i, m := range modes {
			if got, want := federation.Allows(m, flow), cells[i] == 'o'; got != want {
				t.Errorf("Allows(%s, %s) = %v, want %v", m, flow, got, want)
			}
			if got, want := federation.Refuses(m, flow), cells[i] == 'r'; got != want {
				t.Errorf("Refuses(%s, %s) = %v, want %v", m, flow, got, want)
			}
		}
	}
	for _, flow := range federation.Flows() {
		if federation.Allows("", flow) || federation.Allows("sideways", flow) {
			t.Errorf("an unknown mode lets %s through", flow)
		}
	}
	if federation.Allows(federation.ModeBoth, "unknown") {
		t.Error("both lets an unknown flow through")
	}
}

func TestModeValid(t *testing.T) {
	for _, m := range federation.Modes() {
		if !m.Valid() {
			t.Errorf("%s is not valid", m)
		}
	}
	if federation.Mode("").Valid() || federation.Mode("Both").Valid() {
		t.Error("an unknown mode is valid")
	}
	if !slices.Contains(federation.Modes(), federation.DefaultMode) || federation.DefaultMode != federation.ModeBoth {
		t.Errorf("DefaultMode = %s, want both", federation.DefaultMode)
	}
}

func TestSends(t *testing.T) {
	act := func(k federation.OutboundObjectKind, typ federation.ActivityType) federation.OutboundActivity {
		return federation.OutboundActivity{ObjectKind: k, Type: typ}
	}
	cases := []struct {
		name       string
		a          federation.OutboundActivity
		retracting bool
		// off inbound outbound both
		want string
	}{
		{"Create of a status", act(federation.OutboundStatus, federation.ActivityCreate), false, "x x o o"},
		{"Update of a status", act(federation.OutboundStatus, federation.ActivityUpdate), false, "x x o o"},
		{"Delete of a status", act(federation.OutboundStatus, federation.ActivityDelete), false, "x o o o"},
		{"Announce", act(federation.OutboundBoost, federation.ActivityAnnounce), false, "x x o o"},
		{"Undo Announce", act(federation.OutboundBoost, federation.ActivityUndo), false, "x o o o"},
		{"Like", act(federation.OutboundFavourite, federation.ActivityLike), false, "x x o o"},
		{"EmojiReact", act(federation.OutboundFavourite, federation.ActivityEmojiReact), false, "x x o o"},
		{"Undo Like", act(federation.OutboundFavourite, federation.ActivityUndo), false, "x o o o"},
		{"vote", act(federation.OutboundPollVote, federation.ActivityCreate), false, "x x o o"},
		{"Update of a profile", act(federation.OutboundAccount, federation.ActivityUpdate), false, "x x o o"},
		{"Update of a suspension", act(federation.OutboundAccount, federation.ActivityUpdate), true, "x o o o"},
		{"Delete of an actor", act(federation.OutboundAccount, federation.ActivityDelete), false, "x o o o"},
		{"Update with movedTo", act(federation.OutboundAccount, federation.ActivityUpdate), true, "x o o o"},
		{"Move", act(federation.OutboundMove, federation.ActivityMove), false, "x o o o"},
		{"Follow", act(federation.OutboundFollow, federation.ActivityFollow), false, "x o x o"},
		{"Undo Follow", act(federation.OutboundFollow, federation.ActivityUndo), false, "x o x o"},
		{"Accept of a follow", act(federation.OutboundFollow, federation.ActivityAccept), false, "x x o o"},
		{"Reject of a follow", act(federation.OutboundFollow, federation.ActivityReject), false, "x o o o"},
		{"Reject of a refused Follow", act(federation.OutboundRefusal, federation.ActivityReject), false, "x o o o"},
		{"Block", act(federation.OutboundBlock, federation.ActivityBlock), false, "x o o o"},
		{"Undo Block", act(federation.OutboundBlock, federation.ActivityUndo), false, "x o o o"},
		{"Flag", act(federation.OutboundReport, federation.ActivityFlag), false, "x o o o"},
		{"Follow of a relay", act(federation.OutboundRelay, federation.ActivityFollow), false, "x o o o"},
		{"Undo of a relay", act(federation.OutboundRelay, federation.ActivityUndo), false, "x o o o"},
		{"Accept of a relay's Follow", act(federation.OutboundRelay, federation.ActivityAccept), false, "x x o o"},
		{"Reject of a Follow of the instance actor", act(federation.OutboundRelay, federation.ActivityReject), false, "x o o o"},
		{"QuoteRequest", act(federation.OutboundQuote, federation.ActivityQuoteRequest), false, "x x o o"},
		{"Accept of a QuoteRequest", act(federation.OutboundQuote, federation.ActivityAccept), false, "x x o o"},
		{"Reject of a QuoteRequest", act(federation.OutboundQuote, federation.ActivityReject), false, "x x o o"},
		{"Delete of a QuoteAuthorization", act(federation.OutboundQuote, federation.ActivityDelete), false, "x o o o"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i, m := range federation.Modes() {
				if got, want := federation.Sends(m, tc.a, tc.retracting), tc.want[2*i] == 'o'; got != want {
					t.Errorf("Sends(%s) = %v, want %v", m, got, want)
				}
			}
		})
	}
}

func TestAddsRecipients(t *testing.T) {
	del := federation.OutboundActivity{ObjectKind: federation.OutboundAccount, Type: federation.ActivityDelete}
	upd := federation.OutboundActivity{ObjectKind: federation.OutboundAccount, Type: federation.ActivityUpdate}
	for _, m := range federation.Modes() {
		want := federation.Allows(m, federation.FlowPublish)
		if got := federation.AddsRecipients(m, del, false); got != want {
			t.Errorf("AddsRecipients(%s, Delete of an actor) = %v, want %v", m, got, want)
		}
		if got := federation.AddsRecipients(m, upd, true); got != want {
			t.Errorf("AddsRecipients(%s, Update of a suspension) = %v, want %v", m, got, want)
		}
		if !federation.AddsRecipients(m, upd, false) {
			t.Errorf("AddsRecipients(%s, Update of a profile) = false, want true", m)
		}
	}
}
