// SPDX-License-Identifier: AGPL-3.0-only

package gathering_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/domain/gathering"
)

const (
	organizer = "organizer"
	viewerID  = "viewer"
)

func local(id string) gathering.Viewer {
	return gathering.Viewer{AccountID: id, Local: true, Active: true}
}

func TestCanSeeByVisibility(t *testing.T) {
	follows := gathering.Facts{FollowsOrganizer: true}
	followed := gathering.Facts{FollowedByOrganizer: true}
	mutual := gathering.Facts{FollowsOrganizer: true, FollowedByOrganizer: true}
	none := gathering.Facts{}
	for _, tc := range []struct {
		vis  gathering.Visibility
		f    gathering.Facts
		want bool
	}{
		{gathering.VisibilityPublic, none, true},
		{gathering.VisibilityFollowers, none, false},
		{gathering.VisibilityFollowers, follows, true},
		{gathering.VisibilityFollowers, followed, false},
		{gathering.VisibilityFollowing, follows, false},
		{gathering.VisibilityFollowing, followed, true},
		{gathering.VisibilityMutuals, follows, false},
		{gathering.VisibilityMutuals, followed, false},
		{gathering.VisibilityMutuals, mutual, true},
		{gathering.VisibilityInvited, mutual, false},
		{gathering.VisibilityInvited, gathering.Facts{Invited: true}, true},
		// An answer keeps the event visible after the visibility narrowed.
		{gathering.VisibilityInvited, gathering.Facts{Answered: true}, true},
		{gathering.VisibilityInvited, gathering.Facts{Voted: true}, true},
		{gathering.VisibilityMutuals, gathering.Facts{Coorganizer: true}, true},
	} {
		e := gathering.Event{OrganizerAccountID: organizer, Visibility: tc.vis}
		if got := gathering.CanSee(e, local(viewerID), tc.f); got != tc.want {
			t.Errorf("CanSee(%s, %+v) = %v, want %v", tc.vis, tc.f, got, tc.want)
		}
	}
}

func TestCanSeeRefusesBlocksAndOutsiders(t *testing.T) {
	e := gathering.Event{OrganizerAccountID: organizer, Visibility: gathering.VisibilityPublic}
	// A block hides the event from the invited and from the co-organizers too.
	if gathering.CanSee(e, local(viewerID), gathering.Facts{Blocked: true, Invited: true, Answered: true, Coorganizer: true}) {
		t.Error("a blocked viewer sees the event")
	}
	for name, v := range map[string]gathering.Viewer{
		"anonymous": {},
		"remote":    {AccountID: viewerID, Active: true},
		"suspended": {AccountID: viewerID, Local: true},
	} {
		if gathering.CanSee(e, v, gathering.Facts{Invited: true}) {
			t.Errorf("%s sees the event", name)
		}
	}
	if !gathering.CanSee(e, local(organizer), gathering.Facts{}) {
		t.Error("the organizer does not see the event")
	}
	invited := gathering.Event{OrganizerAccountID: organizer, Visibility: gathering.VisibilityInvited}
	if !gathering.CanSee(invited, local(organizer), gathering.Facts{}) {
		t.Error("the organizer does not see an invited-only event")
	}
}

func TestCanSeePlace(t *testing.T) {
	e := gathering.Event{OrganizerAccountID: organizer, Visibility: gathering.VisibilityPublic, PlaceDisclosure: gathering.DisclosureApproved}
	v := local(viewerID)
	if gathering.CanSeePlace(e, gathering.PlacePhysical, v, gathering.Facts{Answered: true}) {
		t.Error("a viewer who is not approved sees the physical place")
	}
	if gathering.CanSeePlace(e, gathering.PlaceURL, v, gathering.Facts{}) {
		t.Error("a viewer who is not approved sees the URL place")
	}
	if !gathering.CanSeePlace(e, gathering.PlaceVoice, v, gathering.Facts{}) {
		t.Error("a viewer does not see the voice place")
	}
	if !gathering.CanSeePlace(e, gathering.PlacePhysical, v, gathering.Facts{Approved: true}) {
		t.Error("an approved viewer does not see the physical place")
	}
	if !gathering.CanSeePlace(e, gathering.PlacePhysical, local(organizer), gathering.Facts{}) {
		t.Error("the organizer does not see the physical place")
	}
	if !gathering.CanSeePlace(e, gathering.PlacePhysical, v, gathering.Facts{Coorganizer: true}) {
		t.Error("a co-organizer does not see the physical place")
	}
	all := e
	all.PlaceDisclosure = gathering.DisclosureViewers
	if !gathering.CanSeePlace(all, gathering.PlacePhysical, v, gathering.Facts{}) {
		t.Error("a viewer does not see the physical place shown to all")
	}
	if gathering.CanSeePlace(all, gathering.PlacePhysical, v, gathering.Facts{Blocked: true, Approved: true}) {
		t.Error("a blocked viewer sees a place")
	}
	hidden := gathering.Event{OrganizerAccountID: organizer, Visibility: gathering.VisibilityInvited, PlaceDisclosure: gathering.DisclosureViewers}
	if gathering.CanSeePlace(hidden, gathering.PlaceVoice, v, gathering.Facts{}) {
		t.Error("a viewer who does not see the event sees its voice place")
	}
}

func TestCanOrganize(t *testing.T) {
	ok := gathering.OrganizerFacts{Local: true, Active: true, EveryoneCreates: true}
	if !gathering.CanOrganize(ok) {
		t.Error("a local active user may not organize while everyone may")
	}
	permitted := gathering.OrganizerFacts{Local: true, Active: true, MayCreate: true}
	if !gathering.CanOrganize(permitted) {
		t.Error("a holder of create_events may not organize")
	}
	for name, f := range map[string]gathering.OrganizerFacts{
		"not permitted": {Local: true, Active: true},
		"limited":       {Local: true, Active: true, Limited: true, EveryoneCreates: true},
		"suspended":     {Local: true, EveryoneCreates: true},
		"remote":        {Active: true, EveryoneCreates: true},
	} {
		if gathering.CanOrganize(f) {
			t.Errorf("%s may organize", name)
		}
	}
}

func validSettings() gathering.Settings {
	return gathering.Settings{
		Title: "Picnic", Visibility: gathering.VisibilityPublic, TimeZone: "Asia/Tokyo",
		PlaceDisclosure: gathering.DisclosureApproved,
		Places: []gathering.Place{
			{Kind: gathering.PlacePhysical, Name: "Park", Address: "1-1 Chiyoda", URL: "https://maps.example/park"},
			{Kind: gathering.PlaceURL, Name: "Stream", URL: "http://stream.example/live"},
			{Kind: gathering.PlaceVoice, Speakers: gathering.SpeakersManual},
		},
	}
}

func count(s string) int { return utf8.RuneCountInString(s) }

func TestSettingsValidate(t *testing.T) {
	if err := validSettings().Validate(count); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	for _, tc := range []struct {
		name  string
		edit  func(*gathering.Settings)
		field gathering.Field
	}{
		{"empty title", func(s *gathering.Settings) { s.Title = "" }, gathering.FieldTitle},
		{"long title", func(s *gathering.Settings) { s.Title = strings.Repeat("a", 101) }, gathering.FieldTitle},
		{"padded title", func(s *gathering.Settings) { s.Title = " a" }, gathering.FieldTitle},
		{"long description", func(s *gathering.Settings) { s.Description = strings.Repeat("a", 3001) }, gathering.FieldDescription},
		{"visibility", func(s *gathering.Settings) { s.Visibility = "local" }, gathering.FieldVisibility},
		{"time zone", func(s *gathering.Settings) { s.TimeZone = "Mars/Olympus" }, gathering.FieldTimeZone},
		{"local time zone", func(s *gathering.Settings) { s.TimeZone = "Local" }, gathering.FieldTimeZone},
		{"disclosure", func(s *gathering.Settings) { s.PlaceDisclosure = "all" }, gathering.FieldPlaceDisclosure},
		{"capacity", func(s *gathering.Settings) { s.Capacity = 10001 }, gathering.FieldCapacity},
		{"negative capacity", func(s *gathering.Settings) { s.Capacity = -1 }, gathering.FieldCapacity},
		{"two physical places", func(s *gathering.Settings) {
			s.Places = append(s.Places[:1], gathering.Place{Kind: gathering.PlacePhysical, Name: "Hall"})
		}, gathering.FieldPlaces},
		{"javascript URL", func(s *gathering.Settings) { s.Places[1].URL = "javascript:alert(1)" }, gathering.FieldPlaces},
		{"ftp URL", func(s *gathering.Settings) { s.Places[1].URL = "ftp://files.example/x" }, gathering.FieldPlaces},
		{"data map URL", func(s *gathering.Settings) { s.Places[0].URL = "data:text/html,hi" }, gathering.FieldPlaces},
		{"URL without a URL", func(s *gathering.Settings) { s.Places[1].URL = "" }, gathering.FieldPlaces},
		{"voice speakers", func(s *gathering.Settings) { s.Places[2].Speakers = "" }, gathering.FieldPlaces},
		{"voice with a name", func(s *gathering.Settings) { s.Places[2].Name = "Room" }, gathering.FieldPlaces},
		{"long address", func(s *gathering.Settings) { s.Places[0].Address = strings.Repeat("a", 501) }, gathering.FieldPlaces},
		{"unknown kind", func(s *gathering.Settings) { s.Places[0].Kind = "boat" }, gathering.FieldPlaces},
	} {
		s := validSettings()
		s.Places = append([]gathering.Place{}, s.Places...)
		tc.edit(&s)
		var se *gathering.SettingsError
		if err := s.Validate(count); !errors.As(err, &se) || se.Field != tc.field {
			t.Errorf("%s: Validate = %v, want the field %s", tc.name, err, tc.field)
		}
	}
}

func TestValidURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://example.com/a?b=c":     true,
		"http://example.com":            true,
		"HTTPS://example.com":           true,
		"javascript:alert(1)":           false,
		"mailto:a@example.com":          false,
		"//example.com":                 false,
		"https://user:pw@example.com/":  false,
		"https://example.com/a b":       false,
		"https://":                      false,
		"file:///etc/passwd":            false,
		"https://example.com/" + "\x00": false,
	} {
		if got := gathering.ValidURL(raw); got != want {
			t.Errorf("ValidURL(%q) = %v, want %v", raw, got, want)
		}
	}
	if gathering.ValidURL("https://example.com/" + strings.Repeat("a", 2000)) {
		t.Error("ValidURL accepts a URL over 2,000 bytes")
	}
}

func TestScheduleValidate(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	ok := gathering.Schedule{StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour)}
	if err := ok.Validate(now); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	for name, tc := range map[string]struct {
		s     gathering.Schedule
		field gathering.Field
	}{
		"past":       {gathering.Schedule{StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour)}, gathering.FieldStartsAt},
		"now":        {gathering.Schedule{StartsAt: now, EndsAt: now.Add(time.Hour)}, gathering.FieldStartsAt},
		"next year":  {gathering.Schedule{StartsAt: now.AddDate(1, 0, 1), EndsAt: now.AddDate(1, 0, 2)}, gathering.FieldStartsAt},
		"backwards":  {gathering.Schedule{StartsAt: now.Add(2 * time.Hour), EndsAt: now.Add(time.Hour)}, gathering.FieldEndsAt},
		"empty":      {gathering.Schedule{StartsAt: now.Add(time.Hour), EndsAt: now.Add(time.Hour)}, gathering.FieldEndsAt},
		"eight days": {gathering.Schedule{StartsAt: now.Add(time.Hour), EndsAt: now.Add(time.Hour + 8*24*time.Hour)}, gathering.FieldEndsAt},
	} {
		var se *gathering.SettingsError
		if err := tc.s.Validate(now); !errors.As(err, &se) || se.Field != tc.field {
			t.Errorf("%s: Validate = %v, want the field %s", name, err, tc.field)
		}
	}
}
