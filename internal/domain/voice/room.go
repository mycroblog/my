// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bounds of the settings of a room (ADR-0100 §3).
const (
	// MaxNameGraphemes is the longest name, in grapheme clusters.
	MaxNameGraphemes = 100
	// MaxNameLength bounds the name in code points as well, so that a few clusters of many
	// combining marks cannot make a name of any size (voice_room_revisions.name).
	MaxNameLength = 1000
	// MaxDescriptionLength is the longest description, in characters (code points).
	MaxDescriptionLength = 500
	// MaxHashtags is how many hashtags a room has at most.
	MaxHashtags = 5
	// MaxHashtagLength is the longest hashtag, in characters, without its #.
	MaxHashtagLength = 100
	// MaxCohosts is how many co-hosts a room has at most (ADR-0100 §5).
	MaxCohosts = 3
)

// Visibility is who sees a room and may join it (voice_room_revisions.visibility).
type Visibility string

// The visibilities of a room.
const (
	// VisibilityLocal: every local user.
	VisibilityLocal Visibility = "local"
	// VisibilityFollowers: the followers of the current host.
	VisibilityFollowers Visibility = "followers"
	// VisibilityMutuals: the users who follow the current host and whom the host follows.
	VisibilityMutuals Visibility = "mutuals"
	// VisibilityInvited: the invited users only.
	VisibilityInvited Visibility = "invited"
)

// Valid reports whether v is a known visibility.
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityLocal, VisibilityFollowers, VisibilityMutuals, VisibilityInvited:
		return true
	}
	return false
}

// Speakers is who may become a speaker without a grant (voice_room_revisions.speakers).
type Speakers string

// The speakers settings of a room.
const (
	// SpeakersEveryone: every participant.
	SpeakersEveryone Speakers = "everyone"
	// SpeakersFollowers: the participants who follow the current host.
	SpeakersFollowers Speakers = "followers"
	// SpeakersMutuals: the participants who follow the current host and whom the host follows.
	SpeakersMutuals Speakers = "mutuals"
	// SpeakersManual: nobody; the host and the co-hosts grant it.
	SpeakersManual Speakers = "manual"
)

// Valid reports whether s is a known speakers setting.
func (s Speakers) Valid() bool {
	switch s {
	case SpeakersEveryone, SpeakersFollowers, SpeakersMutuals, SpeakersManual:
		return true
	}
	return false
}

// Default settings of a new room (ADR-0100 §3).
const (
	DefaultVisibility = VisibilityLocal
	DefaultSpeakers   = SpeakersManual
)

// Role is whether a participant speaks or listens (voice_room_role_changes.role,
// voice_room_invitations.role). Host and co-host are not roles: a co-host speaks or listens.
type Role string

// The roles of a participant.
const (
	RoleSpeaker  Role = "speaker"
	RoleListener Role = "listener"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r == RoleSpeaker || r == RoleListener }

// Transport is the media transport a room opened with (voice_rooms.transport): the value of
// the voice setting at the time, never off.
type Transport string

// The transports of a room.
const (
	TransportP2P     Transport = "p2p"
	TransportLiveKit Transport = "livekit"
	TransportSora    Transport = "sora"
)

// Valid reports whether t is a known transport.
func (t Transport) Valid() bool {
	return t == TransportP2P || t == TransportLiveKit || t == TransportSora
}

// State is whether a room is open (voice_room_heads.state).
type State string

// The states of a room.
const (
	StateOpen  State = "open"
	StateEnded State = "ended"
)

// EndReason is why a room ended (voice_room_endings.reason).
type EndReason string

// The reasons a room ends.
const (
	// EndHost: the host ended it.
	EndHost EndReason = "host"
	// EndEmpty: nobody was left in it.
	EndEmpty EndReason = "empty"
	// EndHostDisconnected: the grace of the disconnected host ran out with no eligible co-host.
	EndHostDisconnected EndReason = "host_disconnected"
	// EndHostIneligible: the host lost the eligibility with no eligible co-host.
	EndHostIneligible EndReason = "host_ineligible"
	// EndModerator: a moderator ended it.
	EndModerator EndReason = "moderator"
	// EndTimeLimit: it reached the longest duration that the operator set.
	EndTimeLimit EndReason = "time_limit"
	// EndDisabled: the operator turned voice chat off.
	EndDisabled EndReason = "disabled"
)

// Valid reports whether r is a known end reason.
func (r EndReason) Valid() bool {
	switch r {
	case EndHost, EndEmpty, EndHostDisconnected, EndHostIneligible, EndModerator, EndTimeLimit, EndDisabled:
		return true
	}
	return false
}

// ParticipationEndReason is why a participation ended (voice_room_participation_ends.reason).
type ParticipationEndReason string

// The reasons a participation ends.
const (
	// LeftRoom: the participant left.
	LeftRoom ParticipationEndReason = "left"
	// MovedRoom: the participant joined another room (one room per account, ADR-0100 §5).
	MovedRoom ParticipationEndReason = "moved"
	// Disconnected: the heartbeat of the participant expired (ADR-0100 §6).
	Disconnected ParticipationEndReason = "disconnected"
	// RemovedFromRoom: the host or a co-host removed the participant, who is banned from the room.
	RemovedFromRoom ParticipationEndReason = "removed"
	// BlockedInRoom: a block came between the participant and the host or a co-host (ADR-0100 §11).
	BlockedInRoom ParticipationEndReason = "blocked"
	// SuspendedAccount: the account of the participant was suspended or deleted.
	SuspendedAccount ParticipationEndReason = "suspended"
	// RoomEnded: the room ended.
	RoomEnded ParticipationEndReason = "room_ended"
)

// Valid reports whether r is a known participation end reason.
func (r ParticipationEndReason) Valid() bool {
	switch r {
	case LeftRoom, MovedRoom, Disconnected, RemovedFromRoom, BlockedInRoom, SuspendedAccount, RoomEnded:
		return true
	}
	return false
}

// HostChangeReason is why a room has a new host (voice_room_host_changes.reason).
type HostChangeReason string

// The reasons of a change of host.
const (
	// HostOpened: the first host, who opened the room.
	HostOpened HostChangeReason = "opened"
	// HostHandedOver: the host handed the room over.
	HostHandedOver HostChangeReason = "handed_over"
	// HostDisconnected: the grace of the disconnected host ran out.
	HostDisconnected HostChangeReason = "host_disconnected"
	// HostIneligible: the host lost the eligibility.
	HostIneligible HostChangeReason = "host_ineligible"
	// HostLeft: the host left and chose the next host.
	HostLeft HostChangeReason = "host_left"
)

// Valid reports whether r is a known host change reason.
func (r HostChangeReason) Valid() bool {
	switch r {
	case HostOpened, HostHandedOver, HostDisconnected, HostIneligible, HostLeft:
		return true
	}
	return false
}

// CohostChange is a co-host assigned or removed (voice_room_cohost_changes.change).
type CohostChange string

// The changes of a co-host.
const (
	CohostAssigned CohostChange = "assigned"
	CohostRemoved  CohostChange = "removed"
)

// Valid reports whether c is a known co-host change.
func (c CohostChange) Valid() bool { return c == CohostAssigned || c == CohostRemoved }

// SpeakGrantChange is an explicit permission to speak granted or revoked
// (voice_room_speak_grants.change). An invitation as a speaker writes a grant too.
type SpeakGrantChange string

// The changes of a speak grant.
const (
	SpeakGranted SpeakGrantChange = "granted"
	SpeakRevoked SpeakGrantChange = "revoked"
)

// Valid reports whether c is a known speak grant change.
func (c SpeakGrantChange) Valid() bool { return c == SpeakGranted || c == SpeakRevoked }

// HandRaiseChange is a change of a raised hand (voice_room_hand_raises.change): the participant
// raises or lowers it, and the host or a co-host grants or dismisses it.
type HandRaiseChange string

// The changes of a raised hand.
const (
	HandRaised    HandRaiseChange = "raised"
	HandLowered   HandRaiseChange = "lowered"
	HandGranted   HandRaiseChange = "granted"
	HandDismissed HandRaiseChange = "dismissed"
)

// Valid reports whether c is a known hand raise change.
func (c HandRaiseChange) Valid() bool {
	switch c {
	case HandRaised, HandLowered, HandGranted, HandDismissed:
		return true
	}
	return false
}

// Raised reports whether the hand is up after c.
func (c HandRaiseChange) Raised() bool { return c == HandRaised }

// Hashtag is a hashtag of a room: Name as typed (without the #) and NameNormalized under the
// normalization of the hashtags of statuses, which the content module applies.
type Hashtag struct {
	Name           string
	NameNormalized string
}

// Settings are the settings of a room that a revision records (ADR-0100 §3).
type Settings struct {
	Name        string
	Description string
	// Hashtags are in the order the host gave them.
	Hashtags   []Hashtag
	Visibility Visibility
	Speakers   Speakers
	// Icon is the emoji shown before the name, the zero Icon for none.
	Icon Icon
}

// MaxIconLength bounds the bytes of the Unicode emoji of an icon, as icon_emoji_key does.
const MaxIconLength = 100

// Icon is the emoji icon of a room, in the shape of the emoji of a reaction (ADR-0052 §1): a
// Unicode emoji, fully qualified, or the ID of a custom emoji of this instance. At most one is
// set; the zero Icon is no icon. The application layer checks that the emoji is one a local
// account may use.
type Icon struct {
	Unicode       string
	CustomEmojiID string
}

// IsZero reports the absence of an icon.
func (i Icon) IsZero() bool { return i.Unicode == "" && i.CustomEmojiID == "" }

// Key returns what icon_emoji_key stores: the Unicode emoji, the ID of the custom emoji, or ""
// for no icon.
func (i Icon) Key() string {
	if i.CustomEmojiID != "" {
		return i.CustomEmojiID
	}
	return i.Unicode
}

// IconOf returns the icon that icon_emoji_key and icon_custom_emoji_id store.
func IconOf(key, customEmojiID string) Icon {
	if customEmojiID != "" {
		return Icon{CustomEmojiID: customEmojiID}
	}
	return Icon{Unicode: key}
}

func (i Icon) valid() bool {
	return (i.Unicode == "" || i.CustomEmojiID == "") && utf8.ValidString(i.Unicode) && len(i.Unicode) <= MaxIconLength
}

// SettingsField names the setting that Validate rejected.
type SettingsField string

// The fields of the settings.
const (
	FieldName        SettingsField = "name"
	FieldDescription SettingsField = "description"
	FieldHashtags    SettingsField = "hashtags"
	FieldVisibility  SettingsField = "visibility"
	FieldSpeakers    SettingsField = "speakers"
	FieldIcon        SettingsField = "icon"
)

// SettingsError reports the invalid setting.
type SettingsError struct{ Field SettingsField }

func (e *SettingsError) Error() string { return "voice: invalid room " + string(e.Field) }

// GraphemeCounter returns how many grapheme clusters s has. The application layer gives one
// that follows Unicode text segmentation; this package uses only the standard library.
type GraphemeCounter func(s string) int

// Validate checks s: a name of 1 to MaxNameGraphemes grapheme clusters (and at most
// MaxNameLength code points) on one line, without surrounding white space or control
// characters; a description of at most MaxDescriptionLength characters without control
// characters other than line breaks; at most MaxHashtags hashtags, each with a name and a
// normalized name of 1 to MaxHashtagLength characters without # or white space, unique by the
// normalized name; a known visibility and speakers setting; and at most one emoji of an icon.
// It returns a *SettingsError.
func (s Settings) Validate(graphemes GraphemeCounter) error {
	invalid := func(f SettingsField) error { return &SettingsError{Field: f} }
	switch {
	case !validName(s.Name, graphemes):
		return invalid(FieldName)
	case !utf8.ValidString(s.Description) || utf8.RuneCountInString(s.Description) > MaxDescriptionLength ||
		strings.ContainsFunc(s.Description, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }):
		return invalid(FieldDescription)
	case !validHashtags(s.Hashtags):
		return invalid(FieldHashtags)
	case !s.Visibility.Valid():
		return invalid(FieldVisibility)
	case !s.Speakers.Valid():
		return invalid(FieldSpeakers)
	case !s.Icon.valid():
		return invalid(FieldIcon)
	}
	return nil
}

func validName(name string, graphemes GraphemeCounter) bool {
	if name == "" || strings.TrimSpace(name) != name || !utf8.ValidString(name) ||
		strings.ContainsFunc(name, unicode.IsControl) || utf8.RuneCountInString(name) > MaxNameLength {
		return false
	}
	return graphemes(name) <= MaxNameGraphemes
}

func validHashtags(tags []Hashtag) bool {
	if len(tags) > MaxHashtags {
		return false
	}
	seen := make(map[string]bool, len(tags))
	for _, t := range tags {
		if !validTag(t.Name) || !validTag(t.NameNormalized) || seen[t.NameNormalized] {
			return false
		}
		seen[t.NameNormalized] = true
	}
	return true
}

func validTag(s string) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= MaxHashtagLength &&
		!strings.ContainsFunc(s, func(r rune) bool { return r == '#' || unicode.IsSpace(r) || unicode.IsControl(r) })
}
