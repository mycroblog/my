// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Bounds of the settings of an event (ADR-0161 §5, §6).
const (
	// MaxTitleGraphemes is the longest title, in grapheme clusters.
	MaxTitleGraphemes = 100
	// MaxTitleLength bounds the title in code points as well, so that a few clusters of many
	// combining marks cannot make a title of any size (gathering_revisions.title).
	MaxTitleLength = 1000
	// MaxDescriptionLength is the longest description, in characters (code points).
	MaxDescriptionLength = 3000
	// MaxPlaces is how many places an event has at most, one of each kind.
	MaxPlaces = 3
	// MaxPlaceNameLength is the longest name of a place, in characters.
	MaxPlaceNameLength = 100
	// MaxAddressLength is the longest address of a physical place, in characters.
	MaxAddressLength = 500
	// MaxURLLength is the longest URL of a place, in bytes.
	MaxURLLength = 2000
	// MaxCapacity is the largest capacity; 0 is no capacity.
	MaxCapacity = 10000
	// MaxCoorganizers is how many co-organizers an event has at most (ADR-0161 §4).
	MaxCoorganizers = 3
	// MaxDuration is the longest an event lasts.
	MaxDuration = 7 * 24 * time.Hour
	// MaxNoteLength is the longest reason of a cancellation, in characters.
	MaxNoteLength = 500
)

// Visibility is who sees an event (gathering_revisions.visibility). Followers, following, and
// mutuals are measured from the organizer.
type Visibility string

// The visibilities of an event.
const (
	// VisibilityPublic: every local user.
	VisibilityPublic Visibility = "public"
	// VisibilityFollowers: the followers of the organizer.
	VisibilityFollowers Visibility = "followers"
	// VisibilityFollowing: the users the organizer follows.
	VisibilityFollowing Visibility = "following"
	// VisibilityMutuals: the users who follow the organizer and whom the organizer follows.
	VisibilityMutuals Visibility = "mutuals"
	// VisibilityInvited: the invited users only.
	VisibilityInvited Visibility = "invited"
)

// Valid reports whether v is a known visibility.
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityPublic, VisibilityFollowers, VisibilityFollowing, VisibilityMutuals, VisibilityInvited:
		return true
	}
	return false
}

// PlaceKind is the kind of a place (gathering_revision_places.kind).
type PlaceKind string

// The kinds of places.
const (
	// PlaceVoice: a voice chat room of this server that opens when the event starts.
	PlaceVoice PlaceKind = "voice"
	// PlacePhysical: a place to meet in person, with a name, an address, and a map link.
	PlacePhysical PlaceKind = "physical"
	// PlaceURL: a link, such as a stream or a meeting of another service.
	PlaceURL PlaceKind = "url"
)

// Valid reports whether k is a known kind of place.
func (k PlaceKind) Valid() bool { return k == PlaceVoice || k == PlacePhysical || k == PlaceURL }

// PlaceDisclosure is who sees the physical and URL places of an event
// (gathering_revisions.place_disclosure).
type PlaceDisclosure string

// The disclosures of the places.
const (
	// DisclosureApproved: the participants the organizers approved.
	DisclosureApproved PlaceDisclosure = "approved"
	// DisclosureViewers: everyone who sees the event.
	DisclosureViewers PlaceDisclosure = "viewers"
)

// Valid reports whether d is a known disclosure.
func (d PlaceDisclosure) Valid() bool { return d == DisclosureApproved || d == DisclosureViewers }

// Speakers is who may speak without a grant in the voice chat room of an event, as the room's own
// setting (gathering_revision_places.speakers).
type Speakers string

// The speakers settings of a voice place.
const (
	SpeakersEveryone  Speakers = "everyone"
	SpeakersFollowers Speakers = "followers"
	SpeakersMutuals   Speakers = "mutuals"
	SpeakersManual    Speakers = "manual"
)

// Valid reports whether s is a known speakers setting.
func (s Speakers) Valid() bool {
	switch s {
	case SpeakersEveryone, SpeakersFollowers, SpeakersMutuals, SpeakersManual:
		return true
	}
	return false
}

// Defaults of a new event (ADR-0161 §5, §6).
const (
	DefaultVisibility      = VisibilityPublic
	DefaultPlaceDisclosure = DisclosureApproved
	DefaultSpeakers        = SpeakersManual
)

// Answer is an attendee's answer (gathering_rsvps.answer).
type Answer string

// The answers of an attendee.
const (
	AnswerGoing    Answer = "going"
	AnswerMaybe    Answer = "maybe"
	AnswerDeclined Answer = "declined"
)

// Valid reports whether a is a known answer.
func (a Answer) Valid() bool { return a == AnswerGoing || a == AnswerMaybe || a == AnswerDeclined }

// Attending reports an answer of going or maybe: the attendees whom the organizers may approve and
// whom the attendee list shows.
func (a Answer) Attending() bool { return a == AnswerGoing || a == AnswerMaybe }

// ApprovalChange is an attendee approved or the approval revoked (gathering_rsvp_approvals.change).
type ApprovalChange string

// The changes of an approval.
const (
	Approved        ApprovalChange = "approved"
	ApprovalRevoked ApprovalChange = "revoked"
)

// Valid reports whether c is a known approval change.
func (c ApprovalChange) Valid() bool { return c == Approved || c == ApprovalRevoked }

// CoorganizerChange is a co-organizer assigned or removed (gathering_coorganizer_changes.change).
type CoorganizerChange string

// The changes of a co-organizer.
const (
	CoorganizerAssigned CoorganizerChange = "assigned"
	CoorganizerRemoved  CoorganizerChange = "removed"
)

// Valid reports whether c is a known co-organizer change.
func (c CoorganizerChange) Valid() bool { return c == CoorganizerAssigned || c == CoorganizerRemoved }

// CancelReason is why an event was cancelled (gathering_cancellations.reason).
type CancelReason string

// The reasons of a cancellation.
const (
	// CancelOrganizer: the organizer cancelled it.
	CancelOrganizer CancelReason = "organizer"
	// CancelModerator: a moderator cancelled it.
	CancelModerator CancelReason = "moderator"
	// CancelOrganizerSuspended: the organizer was suspended before it started.
	CancelOrganizerSuspended CancelReason = "organizer_suspended"
	// CancelOrganizerDeleted: the organizer was deleted before it started.
	CancelOrganizerDeleted CancelReason = "organizer_deleted"
)

// Valid reports whether r is a known reason.
func (r CancelReason) Valid() bool {
	switch r {
	case CancelOrganizer, CancelModerator, CancelOrganizerSuspended, CancelOrganizerDeleted:
		return true
	}
	return false
}

// State is whether an event is unscheduled, scheduled, or cancelled (gathering_heads.state). An
// event that ended is still scheduled: its end is in the past.
type State string

// The states of an event.
const (
	// StateUnscheduled: the event polls its date and no candidate was confirmed yet.
	StateUnscheduled State = "unscheduled"
	StateScheduled   State = "scheduled"
	StateCancelled   State = "cancelled"
)

// Place is a place of an event (ADR-0161 §6). Name, Address, and URL are those of a physical
// place (URL is its optional map link) and of a URL place; Speakers is that of a voice place. The
// other fields of a kind are empty.
type Place struct {
	Kind     PlaceKind
	Name     string
	Address  string
	URL      string
	Speakers Speakers
}

// Settings are the settings of an event that a revision records (ADR-0161 §5).
type Settings struct {
	Title       string
	Description string
	Visibility  Visibility
	// TimeZone is the IANA name of the event's time zone, such as Asia/Tokyo.
	TimeZone        string
	PlaceDisclosure PlaceDisclosure
	// Capacity bounds the attendees who answer going, 0 for no capacity.
	Capacity int
	// Places are in the order the organizer gave them, at most one of each kind.
	Places []Place
}

// Place returns the place of kind k, false without one.
func (s Settings) Place(k PlaceKind) (Place, bool) {
	for _, p := range s.Places {
		if p.Kind == k {
			return p, true
		}
	}
	return Place{}, false
}

// Field names the setting that a validation rejected.
type Field string

// The fields of the settings and the schedule.
const (
	FieldTitle           Field = "title"
	FieldDescription     Field = "description"
	FieldVisibility      Field = "visibility"
	FieldTimeZone        Field = "time_zone"
	FieldPlaceDisclosure Field = "place_disclosure"
	FieldCapacity        Field = "capacity"
	FieldPlaces          Field = "places"
	FieldStartsAt        Field = "starts_at"
	FieldEndsAt          Field = "ends_at"
	FieldCandidates      Field = "candidates"
	FieldNote            Field = "note"
)

// SettingsError reports the invalid setting.
type SettingsError struct{ Field Field }

func (e *SettingsError) Error() string { return "gathering: invalid " + string(e.Field) }

// GraphemeCounter returns how many grapheme clusters s has. The application layer gives one
// that follows Unicode text segmentation; this package uses only the standard library.
type GraphemeCounter func(s string) int

// Validate checks s: a title of 1 to MaxTitleGraphemes grapheme clusters (and at most
// MaxTitleLength code points) on one line, without surrounding white space or control
// characters; a description of at most MaxDescriptionLength characters without control
// characters other than line breaks; a known visibility and disclosure; a time zone that
// ValidTimeZone accepts; a capacity of 0 to MaxCapacity; and at most MaxPlaces places, one of each
// kind, each valid for its kind (validPlace). It returns a *SettingsError.
func (s Settings) Validate(graphemes GraphemeCounter) error {
	invalid := func(f Field) error { return &SettingsError{Field: f} }
	switch {
	case !validLine(s.Title, MaxTitleLength, true) || graphemes(s.Title) > MaxTitleGraphemes:
		return invalid(FieldTitle)
	case !validText(s.Description, MaxDescriptionLength):
		return invalid(FieldDescription)
	case !s.Visibility.Valid():
		return invalid(FieldVisibility)
	case !ValidTimeZone(s.TimeZone):
		return invalid(FieldTimeZone)
	case !s.PlaceDisclosure.Valid():
		return invalid(FieldPlaceDisclosure)
	case s.Capacity < 0 || s.Capacity > MaxCapacity:
		return invalid(FieldCapacity)
	case !validPlaces(s.Places):
		return invalid(FieldPlaces)
	}
	return nil
}

// ValidNote reports whether note is a valid reason of a cancellation: at most MaxNoteLength
// characters without control characters other than line breaks.
func ValidNote(note string) bool { return validText(note, MaxNoteLength) }

// ValidTimeZone reports whether name is the IANA name of a time zone that time.LoadLocation
// knows; "Local" and "" are not names of a zone.
func ValidTimeZone(name string) bool {
	if name == "" || name == "Local" || len(name) > 100 {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// ValidURL reports whether raw is a URL that a place may carry: an absolute http or https URL
// with a host and no user information, of at most MaxURLLength bytes, without white space or
// control characters (ADR-0161 §6). The server never fetches it.
func ValidURL(raw string) bool {
	if raw == "" || len(raw) > MaxURLLength || !utf8.ValidString(raw) ||
		strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func validPlaces(places []Place) bool {
	if len(places) > MaxPlaces {
		return false
	}
	seen := map[PlaceKind]bool{}
	for _, p := range places {
		if seen[p.Kind] || !validPlace(p) {
			return false
		}
		seen[p.Kind] = true
	}
	return true
}

// validPlace checks a place for its kind: a voice place has a known speakers setting and nothing
// else; a physical place has a name, an address of at most MaxAddressLength characters, and an
// optional map URL; a URL place has a name and a URL; neither of the last two has speakers.
func validPlace(p Place) bool {
	switch p.Kind {
	case PlaceVoice:
		return p.Speakers.Valid() && p.Name == "" && p.Address == "" && p.URL == ""
	case PlacePhysical:
		return p.Speakers == "" && validLine(p.Name, MaxPlaceNameLength, true) && validText(p.Address, MaxAddressLength) &&
			(p.URL == "" || ValidURL(p.URL))
	case PlaceURL:
		return p.Speakers == "" && validLine(p.Name, MaxPlaceNameLength, true) && p.Address == "" && ValidURL(p.URL)
	}
	return false
}

// validLine reports a single line of at most max code points without surrounding white space or
// control characters, not empty when required.
func validLine(s string, maxLength int, required bool) bool {
	if s == "" {
		return !required
	}
	return strings.TrimSpace(s) == s && utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl) &&
		utf8.RuneCountInString(s) <= maxLength
}

// validText reports a text of at most max code points without control characters other than line
// breaks.
func validText(s string, maxLength int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= maxLength &&
		!strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' })
}

// Schedule is the fixed start and end of an event, in UTC (ADR-0161 §5).
type Schedule struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// Validate checks a schedule that is being set at now: it starts after now and at most a year
// ahead, and ends after it starts and at most MaxDuration later. It returns a *SettingsError of
// FieldStartsAt or FieldEndsAt.
func (s Schedule) Validate(now time.Time) error {
	switch {
	case s.StartsAt.IsZero() || !s.StartsAt.After(now) || s.StartsAt.After(now.AddDate(1, 0, 0)):
		return &SettingsError{Field: FieldStartsAt}
	case !s.EndsAt.After(s.StartsAt) || s.EndsAt.Sub(s.StartsAt) > MaxDuration:
		return &SettingsError{Field: FieldEndsAt}
	}
	return nil
}
