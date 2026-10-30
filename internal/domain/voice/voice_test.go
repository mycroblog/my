// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	host   = "01890a5d-ac96-7000-8000-000000000001"
	viewer = "01890a5d-ac96-7000-8000-000000000002"
)

func open(v Visibility, s Speakers) Room {
	return Room{HostAccountID: host, Visibility: v, Speakers: s}
}

var local = Viewer{AccountID: viewer, Local: true, Active: true}

func TestCanSee(t *testing.T) {
	t.Parallel()
	ended := open(VisibilityLocal, SpeakersManual)
	ended.Ended = true
	hostViewer := Viewer{AccountID: host, Local: true, Active: true}
	cases := []struct {
		name   string
		rule   int
		room   Room
		viewer Viewer
		facts  Facts
		want   bool
	}{
		{"an ended room is not seen", 1, ended, local, Facts{}, false},
		{"an ended room is not seen by its participant", 1, ended, local, Facts{Participant: true}, false},
		{"an ended room is not seen by its host", 1, ended, hostViewer, Facts{}, false},
		{"a viewer who is not logged in does not see", 2, open(VisibilityLocal, SpeakersManual), Viewer{}, Facts{}, false},
		{"a remote viewer does not see", 2, open(VisibilityLocal, SpeakersManual), Viewer{AccountID: viewer, Active: true}, Facts{}, false},
		{"a suspended or deleted viewer does not see", 2, open(VisibilityLocal, SpeakersManual), Viewer{AccountID: viewer, Local: true}, Facts{}, false},
		{"a suspended host does not see", 2, open(VisibilityLocal, SpeakersManual), Viewer{AccountID: host, Local: true}, Facts{}, false},
		{"a remote participant does not see", 2, open(VisibilityLocal, SpeakersManual), Viewer{AccountID: viewer, Active: true}, Facts{Participant: true}, false},
		{"a block with the host or a co-host hides", 3, open(VisibilityLocal, SpeakersManual), local, Facts{Blocked: true}, false},
		{"a block hides from an invited viewer", 3, open(VisibilityLocal, SpeakersManual), local, Facts{Blocked: true, Invited: true}, false},
		{"a block hides from a participant", 3, open(VisibilityLocal, SpeakersManual), local, Facts{Blocked: true, Participant: true}, false},
		{"a block with the host hides from a co-host", 3, open(VisibilityLocal, SpeakersManual), local, Facts{Blocked: true, Cohost: true}, false},
		{"a ban hides", 4, open(VisibilityLocal, SpeakersManual), local, Facts{Banned: true}, false},
		{"a ban hides from an invited viewer", 4, open(VisibilityLocal, SpeakersManual), local, Facts{Banned: true, Invited: true}, false},
		{"a ban hides from a follower", 4, open(VisibilityFollowers, SpeakersManual), local, Facts{Banned: true, FollowsHost: true}, false},
		{"the host sees an invited room", 5, open(VisibilityInvited, SpeakersManual), hostViewer, Facts{}, true},
		{"a co-host sees an invited room", 5, open(VisibilityInvited, SpeakersManual), local, Facts{Cohost: true}, true},
		{"a participant sees a narrowed room", 5, open(VisibilityMutuals, SpeakersManual), local, Facts{Participant: true}, true},
		{"an invited viewer sees an invited room", 6, open(VisibilityInvited, SpeakersManual), local, Facts{Invited: true}, true},
		{"an invited viewer sees a followers room without following", 6, open(VisibilityFollowers, SpeakersManual), local, Facts{Invited: true}, true},
		{"local is seen by every local viewer", 7, open(VisibilityLocal, SpeakersManual), local, Facts{}, true},
		{"followers is seen by a follower", 7, open(VisibilityFollowers, SpeakersManual), local, Facts{FollowsHost: true}, true},
		{"followers is not seen by someone the host follows", 7, open(VisibilityFollowers, SpeakersManual), local, Facts{FollowedByHost: true}, false},
		{"followers is not seen by others", 7, open(VisibilityFollowers, SpeakersManual), local, Facts{}, false},
		{"mutuals is seen by a mutual", 7, open(VisibilityMutuals, SpeakersManual), local, Facts{FollowsHost: true, FollowedByHost: true}, true},
		{"mutuals is not seen by a follower only", 7, open(VisibilityMutuals, SpeakersManual), local, Facts{FollowsHost: true}, false},
		{"mutuals is not seen by someone the host follows only", 7, open(VisibilityMutuals, SpeakersManual), local, Facts{FollowedByHost: true}, false},
		{"invited is not seen without an invitation", 7, open(VisibilityInvited, SpeakersManual), local, Facts{FollowsHost: true, FollowedByHost: true}, false},
		{"an unknown visibility is not seen", 7, open("public", SpeakersManual), local, Facts{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CanSee(tc.room, tc.viewer, tc.facts); got != tc.want {
				t.Errorf("rule %d: CanSee(%+v, %+v, %+v) = %v, want %v", tc.rule, tc.room, tc.viewer, tc.facts, got, tc.want)
			}
		})
	}
}

func TestCanSpeakFreely(t *testing.T) {
	t.Parallel()
	hostViewer := Viewer{AccountID: host, Local: true, Active: true}
	cases := []struct {
		name  string
		rule  int
		room  Room
		who   Viewer
		facts Facts
		want  bool
	}{
		{"the host may", 1, open(VisibilityLocal, SpeakersManual), hostViewer, Facts{}, true},
		{"a limited host may", 1, open(VisibilityLocal, SpeakersManual), hostViewer, Facts{Limited: true}, true},
		{"a co-host may", 1, open(VisibilityLocal, SpeakersManual), local, Facts{Cohost: true}, true},
		{"a limited co-host may", 1, open(VisibilityLocal, SpeakersManual), local, Facts{Cohost: true, Limited: true}, true},
		{"a viewer without an account is not the host", 1, Room{Speakers: SpeakersManual}, Viewer{}, Facts{}, false},
		{"a limited account may not in an everyone room", 2, open(VisibilityLocal, SpeakersEveryone), local, Facts{Limited: true}, false},
		{"a limited follower may not in a followers room", 2, open(VisibilityLocal, SpeakersFollowers), local, Facts{Limited: true, FollowsHost: true}, false},
		{"a limited account may with a grant", 2, open(VisibilityLocal, SpeakersManual), local, Facts{Limited: true, SpeakGranted: true}, true},
		{"a grant allows in a manual room", 3, open(VisibilityLocal, SpeakersManual), local, Facts{SpeakGranted: true}, true},
		{"a grant allows in a mutuals room", 3, open(VisibilityLocal, SpeakersMutuals), local, Facts{SpeakGranted: true}, true},
		{"everyone may", 4, open(VisibilityLocal, SpeakersEveryone), local, Facts{}, true},
		{"a follower may in a followers room", 4, open(VisibilityLocal, SpeakersFollowers), local, Facts{FollowsHost: true}, true},
		{"someone the host follows may not in a followers room", 4, open(VisibilityLocal, SpeakersFollowers), local, Facts{FollowedByHost: true}, false},
		{"a mutual may in a mutuals room", 4, open(VisibilityLocal, SpeakersMutuals), local, Facts{FollowsHost: true, FollowedByHost: true}, true},
		{"a follower only may not in a mutuals room", 4, open(VisibilityLocal, SpeakersMutuals), local, Facts{FollowsHost: true}, false},
		{"nobody may in a manual room", 4, open(VisibilityLocal, SpeakersManual), local, Facts{FollowsHost: true, FollowedByHost: true}, false},
		{"an unknown setting allows nobody", 4, open(VisibilityLocal, "anyone"), local, Facts{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CanSpeakFreely(tc.room, tc.who, tc.facts); got != tc.want {
				t.Errorf("rule %d: CanSpeakFreely(%+v, %+v, %+v) = %v, want %v", tc.rule, tc.room, tc.who, tc.facts, got, tc.want)
			}
		})
	}
}

func TestCanHost(t *testing.T) {
	t.Parallel()
	eligible := HostFacts{Local: true, Active: true, MayStart: true}
	with := func(change func(*HostFacts)) HostFacts {
		f := eligible
		change(&f)
		return f
	}
	cases := []struct {
		name  string
		facts HostFacts
		want  bool
	}{
		{"a local active account with the permission", eligible, true},
		{"any local active account when everyone starts", HostFacts{Local: true, Active: true, EveryoneStarts: true}, true},
		{"not without the permission when only permitted users start", with(func(f *HostFacts) { f.MayStart = false }), false},
		{"not a remote account", with(func(f *HostFacts) { f.Local = false; f.EveryoneStarts = true }), false},
		{"not a suspended or deleted account", with(func(f *HostFacts) { f.Active = false; f.EveryoneStarts = true }), false},
		{"not a limited account with the permission", with(func(f *HostFacts) { f.Limited = true }), false},
		{"not a limited account when everyone starts", with(func(f *HostFacts) { f.Limited = true; f.EveryoneStarts = true }), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CanHost(tc.facts); got != tc.want {
				t.Errorf("CanHost(%+v) = %v, want %v", tc.facts, got, tc.want)
			}
		})
	}
}

func TestNextHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cohosts []Candidate
		want    string
		ok      bool
	}{
		{"no co-hosts", nil, "", false},
		{"the first nominated when eligible", []Candidate{{"a", true}, {"b", true}}, "a", true},
		{"skips the ineligible", []Candidate{{"a", false}, {"b", true}, {"c", true}}, "b", true},
		{"none eligible", []Candidate{{"a", false}, {"b", false}}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := NextHost(tc.cohosts)
			if got != tc.want || ok != tc.ok {
				t.Errorf("NextHost(%+v) = %q, %v; want %q, %v", tc.cohosts, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// runes counts code points; the tests below use only single-code-point clusters except where a
// test gives its own counter.
func runes(s string) int { return utf8.RuneCountInString(s) }

func TestSettingsValidate(t *testing.T) {
	t.Parallel()
	valid := Settings{
		Name: "Friday night", Description: "Line one\nline two",
		Hashtags:   []Hashtag{{Name: "Go", NameNormalized: "go"}, {Name: "Music", NameNormalized: "music"}},
		Visibility: DefaultVisibility, Speakers: DefaultSpeakers,
	}
	with := func(change func(*Settings)) Settings {
		s := valid
		s.Hashtags = append([]Hashtag(nil), valid.Hashtags...)
		change(&s)
		return s
	}
	tags := func(n int) []Hashtag {
		out := make([]Hashtag, n)
		for i := range out {
			name := "tag" + strings.Repeat("x", i)
			out[i] = Hashtag{Name: name, NameNormalized: name}
		}
		return out
	}
	// family is one grapheme cluster of seven code points.
	const family = "\U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466"
	clusters := func(s string) int { return strings.Count(s, family) + runes(strings.ReplaceAll(s, family, "")) }
	cases := []struct {
		name      string
		settings  Settings
		graphemes GraphemeCounter
		field     SettingsField
	}{
		{"valid", valid, runes, ""},
		{"the default settings with only a name", Settings{Name: "a", Visibility: DefaultVisibility, Speakers: DefaultSpeakers}, runes, ""},
		{"a name of 100 graphemes", with(func(s *Settings) { s.Name = strings.Repeat("あ", MaxNameGraphemes) }), runes, ""},
		{"a name of 100 clusters of many code points", with(func(s *Settings) { s.Name = strings.Repeat(family, MaxNameGraphemes) }), clusters, ""},
		{"an empty name", with(func(s *Settings) { s.Name = "" }), runes, FieldName},
		{"a name of 101 graphemes", with(func(s *Settings) { s.Name = strings.Repeat("あ", MaxNameGraphemes+1) }), runes, FieldName},
		{"a name of too many code points", with(func(s *Settings) { s.Name = "a" + strings.Repeat("́", MaxNameLength) }), func(string) int { return 1 }, FieldName},
		{"a name with surrounding spaces", with(func(s *Settings) { s.Name = " room " }), runes, FieldName},
		{"a name of two lines", with(func(s *Settings) { s.Name = "one\ntwo" }), runes, FieldName},
		{"a name of invalid UTF-8", with(func(s *Settings) { s.Name = "\xff" }), runes, FieldName},
		{"a description of 500 characters", with(func(s *Settings) { s.Description = strings.Repeat("あ", MaxDescriptionLength) }), runes, ""},
		{"a description of 501 characters", with(func(s *Settings) { s.Description = strings.Repeat("あ", MaxDescriptionLength+1) }), runes, FieldDescription},
		{"a description with a control character", with(func(s *Settings) { s.Description = "a\x00b" }), runes, FieldDescription},
		{"five hashtags", with(func(s *Settings) { s.Hashtags = tags(MaxHashtags) }), runes, ""},
		{"six hashtags", with(func(s *Settings) { s.Hashtags = tags(MaxHashtags + 1) }), runes, FieldHashtags},
		{"a duplicate hashtag", with(func(s *Settings) { s.Hashtags = append(s.Hashtags, Hashtag{Name: "GO", NameNormalized: "go"}) }), runes, FieldHashtags},
		{"a hashtag without a normalized name", with(func(s *Settings) { s.Hashtags[0].NameNormalized = "" }), runes, FieldHashtags},
		{"a hashtag with its #", with(func(s *Settings) { s.Hashtags[0].Name = "#Go" }), runes, FieldHashtags},
		{"a hashtag with a space", with(func(s *Settings) { s.Hashtags[0].Name = "G o" }), runes, FieldHashtags},
		{"a hashtag too long", with(func(s *Settings) {
			s.Hashtags[0] = Hashtag{Name: strings.Repeat("a", MaxHashtagLength+1), NameNormalized: strings.Repeat("a", MaxHashtagLength+1)}
		}), runes, FieldHashtags},
		{"an unknown visibility", with(func(s *Settings) { s.Visibility = "public" }), runes, FieldVisibility},
		{"an unknown speakers setting", with(func(s *Settings) { s.Speakers = "" }), runes, FieldSpeakers},
		{"a Unicode icon", with(func(s *Settings) { s.Icon = Icon{Unicode: "\U0001F3B5"} }), runes, ""},
		{"a custom emoji icon", with(func(s *Settings) { s.Icon = Icon{CustomEmojiID: "0190a6f2-0000-7000-8000-000000000001"} }), runes, ""},
		{"an icon of both kinds", with(func(s *Settings) {
			s.Icon = Icon{Unicode: "\U0001F3B5", CustomEmojiID: "0190a6f2-0000-7000-8000-000000000001"}
		}), runes, FieldIcon},
		{"an icon too long", with(func(s *Settings) { s.Icon = Icon{Unicode: strings.Repeat("a", MaxIconLength+1)} }), runes, FieldIcon},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.settings.Validate(tc.graphemes)
			if tc.field == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
				return
			}
			var se *SettingsError
			if !errors.As(err, &se) || se.Field != tc.field {
				t.Errorf("Validate = %v, want the field %s", err, tc.field)
			}
		})
	}
}

func TestValues(t *testing.T) {
	t.Parallel()
	checks := []struct {
		name  string
		valid []func() bool
		bad   func() bool
	}{
		{"visibility", []func() bool{VisibilityLocal.Valid, VisibilityFollowers.Valid, VisibilityMutuals.Valid, VisibilityInvited.Valid}, Visibility("public").Valid},
		{"speakers", []func() bool{SpeakersEveryone.Valid, SpeakersFollowers.Valid, SpeakersMutuals.Valid, SpeakersManual.Valid}, Speakers("all").Valid},
		{"role", []func() bool{RoleSpeaker.Valid, RoleListener.Valid}, Role("host").Valid},
		{"transport", []func() bool{TransportP2P.Valid, TransportLiveKit.Valid, TransportSora.Valid}, Transport("off").Valid},
		{"end reason", []func() bool{
			EndHost.Valid, EndEmpty.Valid, EndHostDisconnected.Valid, EndHostIneligible.Valid, EndModerator.Valid, EndTimeLimit.Valid, EndDisabled.Valid,
		}, EndReason("left").Valid},
		{"participation end reason", []func() bool{
			LeftRoom.Valid, MovedRoom.Valid, Disconnected.Valid, RemovedFromRoom.Valid, BlockedInRoom.Valid, SuspendedAccount.Valid, RoomEnded.Valid,
		}, ParticipationEndReason("host").Valid},
		{"host change reason", []func() bool{
			HostOpened.Valid, HostHandedOver.Valid, HostDisconnected.Valid, HostIneligible.Valid, HostLeft.Valid,
		}, HostChangeReason("moderator").Valid},
		{"co-host change", []func() bool{CohostAssigned.Valid, CohostRemoved.Valid}, CohostChange("granted").Valid},
		{"speak grant change", []func() bool{SpeakGranted.Valid, SpeakRevoked.Valid}, SpeakGrantChange("raised").Valid},
		{"hand raise change", []func() bool{HandRaised.Valid, HandLowered.Valid, HandGranted.Valid, HandDismissed.Valid}, HandRaiseChange("revoked").Valid},
	}
	for _, c := range checks {
		for i, v := range c.valid {
			if !v() {
				t.Errorf("%s %d is not valid", c.name, i)
			}
		}
		if c.bad() {
			t.Errorf("an unknown %s is valid", c.name)
		}
	}
	for _, c := range []HandRaiseChange{HandLowered, HandGranted, HandDismissed} {
		if c.Raised() {
			t.Errorf("%s leaves the hand raised", c)
		}
	}
	if !HandRaised.Raised() {
		t.Error("raised does not raise the hand")
	}
}

func TestIcon(t *testing.T) {
	t.Parallel()
	const id = "0190a6f2-0000-7000-8000-000000000001"
	cases := []struct {
		name     string
		key, cid string
		want     Icon
	}{
		{"no icon", "", "", Icon{}},
		{"a Unicode emoji", "\U0001F3B5", "", Icon{Unicode: "\U0001F3B5"}},
		{"a custom emoji", id, id, Icon{CustomEmojiID: id}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IconOf(tc.key, tc.cid)
			if got != tc.want {
				t.Fatalf("IconOf = %+v, want %+v", got, tc.want)
			}
			if got.Key() != tc.key || got.IsZero() != (tc.key == "") {
				t.Errorf("Key = %q, IsZero = %v for %+v", got.Key(), got.IsZero(), got)
			}
		})
	}
}
