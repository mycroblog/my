// SPDX-License-Identifier: AGPL-3.0-only

package gathering_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/gathering"
)

// The roles of a viewer of an event's board (ADR-0161 §17), as the facts that the application
// layer gathers say them.
var (
	boardOrganizer   = gathering.Facts{}
	boardCoorganizer = gathering.Facts{Coorganizer: true}
	boardGoing       = gathering.Facts{Answered: true, Attending: true}
	boardInvitedGoes = gathering.Facts{Invited: true, Answered: true, Attending: true}
	boardDeclined    = gathering.Facts{Answered: true}
	boardInvited     = gathering.Facts{Invited: true}
	boardStranger    = gathering.Facts{}
	boardVoterOnly   = gathering.Facts{Voted: true}
)

func boardEvent(vis gathering.Visibility, w gathering.BoardWriters) gathering.Event {
	return gathering.Event{OrganizerAccountID: organizer, Visibility: vis, BoardWriters: w}
}

// TestCanReadBoard checks who reads a board: the organizer, the co-organizers, and the viewers who
// see the event and answer going or maybe; never one who declined, an invitee who has not
// answered, a stranger, a blocked viewer, or a viewer who is not a local, active account.
func TestCanReadBoard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		vis    gathering.Visibility
		viewer gathering.Viewer
		f      gathering.Facts
		want   bool
	}{
		{"the organizer", gathering.VisibilityPublic, local(organizer), boardOrganizer, true},
		{"the organizer of an invited-only event", gathering.VisibilityInvited, local(organizer), boardOrganizer, true},
		{"a co-organizer who did not answer", gathering.VisibilityInvited, local(viewerID), boardCoorganizer, true},
		{"going or maybe", gathering.VisibilityPublic, local(viewerID), boardGoing, true},
		{"an invitee who answered going or maybe", gathering.VisibilityInvited, local(viewerID), boardInvitedGoes, true},
		{"an answer keeps a narrowed event's board", gathering.VisibilityMutuals, local(viewerID), boardGoing, true},
		{"not going", gathering.VisibilityPublic, local(viewerID), boardDeclined, false},
		{"an invitee who has not answered", gathering.VisibilityInvited, local(viewerID), boardInvited, false},
		{"a stranger who sees a public event", gathering.VisibilityPublic, local(viewerID), boardStranger, false},
		{"a voter of the date poll who has not answered", gathering.VisibilityPublic, local(viewerID), boardVoterOnly, false},
		{"going, blocked", gathering.VisibilityPublic, local(viewerID), gathering.Facts{Answered: true, Attending: true, Blocked: true}, false},
		{"a co-organizer, blocked", gathering.VisibilityPublic, local(viewerID), gathering.Facts{Coorganizer: true, Blocked: true}, false},
		{"going, but not logged in", gathering.VisibilityPublic, gathering.Viewer{}, boardGoing, false},
		{"going, remote", gathering.VisibilityPublic, gathering.Viewer{AccountID: viewerID, Active: true}, boardGoing, false},
		{"going, suspended", gathering.VisibilityPublic, gathering.Viewer{AccountID: viewerID, Local: true}, boardGoing, false},
		{"the organizer, suspended", gathering.VisibilityPublic, gathering.Viewer{AccountID: organizer, Local: true}, boardOrganizer, false},
	} {
		for _, w := range []gathering.BoardWriters{gathering.BoardWritersOrganizer, gathering.BoardWritersOrganizers, gathering.BoardWritersReaders} {
			if got := gathering.CanReadBoard(boardEvent(tc.vis, w), tc.viewer, tc.f); got != tc.want {
				t.Errorf("%s (writers %s): CanReadBoard = %v, want %v", tc.name, w, got, tc.want)
			}
		}
	}
}

// TestCanWriteBoard checks who writes among the readers for each choice of board_writers, and that
// an unknown choice admits the organizer alone.
func TestCanWriteBoard(t *testing.T) {
	type roles struct{ organizer, coorganizer, going, declined, invited, stranger bool }
	for _, tc := range []struct {
		writers gathering.BoardWriters
		want    roles
	}{
		{gathering.BoardWritersOrganizer, roles{organizer: true}},
		{gathering.BoardWritersOrganizers, roles{organizer: true, coorganizer: true}},
		{gathering.BoardWritersReaders, roles{organizer: true, coorganizer: true, going: true}},
		{"", roles{organizer: true}},
		{"everyone", roles{organizer: true}},
	} {
		e := boardEvent(gathering.VisibilityPublic, tc.writers)
		for name, c := range map[string]struct {
			viewer gathering.Viewer
			f      gathering.Facts
			want   bool
		}{
			"organizer":   {local(organizer), boardOrganizer, tc.want.organizer},
			"coorganizer": {local(viewerID), boardCoorganizer, tc.want.coorganizer},
			"going":       {local(viewerID), boardGoing, tc.want.going},
			"declined":    {local(viewerID), boardDeclined, tc.want.declined},
			"invited":     {local(viewerID), boardInvited, tc.want.invited},
			"stranger":    {local(viewerID), boardStranger, tc.want.stranger},
		} {
			if got := gathering.CanWriteBoard(e, c.viewer, c.f); got != c.want {
				t.Errorf("writers %q, %s: CanWriteBoard = %v, want %v", tc.writers, name, got, c.want)
			}
		}
	}
	// Writing needs reading: a blocked co-organizer and a suspended organizer write nothing.
	e := boardEvent(gathering.VisibilityPublic, gathering.BoardWritersReaders)
	if gathering.CanWriteBoard(e, local(viewerID), gathering.Facts{Coorganizer: true, Blocked: true}) {
		t.Error("a blocked co-organizer writes on the board")
	}
	if gathering.CanWriteBoard(e, gathering.Viewer{AccountID: organizer, Local: true}, boardOrganizer) {
		t.Error("a suspended organizer writes on the board")
	}
}

// TestBoardOpen checks the term of a board: open from the creation, read-only at once when the
// event is canceled, and BoardWritableAfterEnd after a fixed end; an event that polls its date has
// no end yet.
func TestBoardOpen(t *testing.T) {
	if gathering.BoardWritableAfterEnd != 7*24*time.Hour {
		t.Errorf("BoardWritableAfterEnd = %v, want 7 days", gathering.BoardWritableAfterEnd)
	}
	end := time.Date(2026, 12, 1, 14, 0, 0, 0, time.UTC)
	// An event that polls its date ends with its last candidate date, as a scheduled one with
	// its date: both are a BoardTerm with that end.
	scheduled := gathering.BoardTerm{EndsAt: end}
	for _, tc := range []struct {
		name string
		term gathering.BoardTerm
		now  time.Time
		want bool
	}{
		{"before the start", scheduled, end.Add(-30 * 24 * time.Hour), true},
		{"at the end", scheduled, end, true},
		{"a second before 7 days after the end", scheduled, end.Add(gathering.BoardWritableAfterEnd - time.Second), true},
		{"a second after 7 days after the end", scheduled, end.Add(gathering.BoardWritableAfterEnd + time.Second), false},
		{"a month after the end", scheduled, end.Add(30 * 24 * time.Hour), false},
		{"canceled before the start", gathering.BoardTerm{EndsAt: end, Canceled: true, CanceledAt: end.Add(-48 * time.Hour)}, end.Add(-24 * time.Hour), false},
		{"canceled while polling", gathering.BoardTerm{EndsAt: end, Canceled: true, CanceledAt: end.Add(-48 * time.Hour)}, end.Add(-24 * time.Hour), false},
		{"polling, a year after its last candidate", scheduled, end.Add(365 * 24 * time.Hour), false},
		{"no end", gathering.BoardTerm{}, end, false},
	} {
		if got := gathering.BoardOpen(tc.term, tc.now); got != tc.want {
			t.Errorf("%s: BoardOpen = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestBoard checks that Board joins the decisions: it sees what CanSee sees, reads what
// CanReadBoard reads, is open as BoardOpen says, and writes only while open and admitted.
func TestBoard(t *testing.T) {
	end := time.Date(2026, 12, 1, 14, 0, 0, 0, time.UTC)
	open := gathering.BoardTerm{EndsAt: end}
	canceled := gathering.BoardTerm{EndsAt: end, Canceled: true, CanceledAt: end.Add(-48 * time.Hour)}
	now := end.Add(-time.Hour)
	organizers := boardEvent(gathering.VisibilityPublic, gathering.BoardWritersOrganizers)
	invited := boardEvent(gathering.VisibilityInvited, gathering.BoardWritersReaders)
	for _, tc := range []struct {
		name   string
		e      gathering.Event
		term   gathering.BoardTerm
		viewer gathering.Viewer
		f      gathering.Facts
		want   gathering.BoardAccess
	}{
		{"the organizer", organizers, open, local(organizer), boardOrganizer, gathering.BoardAccess{Sees: true, Reads: true, Open: true, Writes: true}},
		{"the organizer of a canceled event", organizers, canceled, local(organizer), boardOrganizer, gathering.BoardAccess{Sees: true, Reads: true}},
		{"going, among organizers' writers", organizers, open, local(viewerID), boardGoing, gathering.BoardAccess{Sees: true, Reads: true, Open: true}},
		{"going, among readers' writers", invited, open, local(viewerID), boardInvitedGoes, gathering.BoardAccess{Sees: true, Reads: true, Open: true, Writes: true}},
		{"going, after the term", invited, open, local(viewerID), boardInvitedGoes, gathering.BoardAccess{Sees: true, Reads: true}},
		{"not going", organizers, open, local(viewerID), boardDeclined, gathering.BoardAccess{Sees: true, Open: true}},
		{"an invitee who has not answered", invited, open, local(viewerID), boardInvited, gathering.BoardAccess{Sees: true, Open: true}},
		{"a viewer who does not see the event", invited, open, local(viewerID), boardStranger, gathering.BoardAccess{Open: true}},
		{"not logged in", organizers, open, gathering.Viewer{}, boardStranger, gathering.BoardAccess{Open: true}},
	} {
		at := now
		if tc.name == "going, after the term" {
			at = end.Add(gathering.BoardWritableAfterEnd + time.Hour)
		}
		got := gathering.Board(tc.e, tc.term, tc.viewer, tc.f, at)
		if !tc.want.Sees {
			// What the term is says nothing to a viewer who does not see the event.
			got.Open, tc.want.Open = false, false
		}
		if got != tc.want {
			t.Errorf("%s: Board = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// TestBoardWriters checks the choices of board_writers, their default, and that the settings of an
// event refuse an unknown one.
func TestBoardWriters(t *testing.T) {
	if gathering.DefaultBoardWriters != gathering.BoardWritersOrganizers {
		t.Errorf("DefaultBoardWriters = %q, want organizers", gathering.DefaultBoardWriters)
	}
	for w, want := range map[gathering.BoardWriters]bool{
		"organizer": true, "organizers": true, "readers": true, "": false, "everyone": false, "Readers": false,
	} {
		if got := w.Valid(); got != want {
			t.Errorf("BoardWriters(%q).Valid() = %v, want %v", w, got, want)
		}
	}
	for _, w := range []gathering.BoardWriters{"", "everyone"} {
		s := validSettings()
		s.BoardWriters = w
		var se *gathering.SettingsError
		if err := s.Validate(count); !errors.As(err, &se) || se.Field != gathering.FieldBoardWriters {
			t.Errorf("writers %q: Validate = %v, want the field %s", w, err, gathering.FieldBoardWriters)
		}
	}
}

// TestAnswerAttending checks that going and maybe attend, and that declining does not.
func TestAnswerAttending(t *testing.T) {
	for a, want := range map[gathering.Answer]bool{
		gathering.AnswerGoing: true, gathering.AnswerMaybe: true, gathering.AnswerDeclined: false, "": false,
	} {
		if got := a.Attending(); got != want {
			t.Errorf("Answer(%q).Attending() = %v, want %v", a, got, want)
		}
	}
}
