// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"slices"
	"strings"
	"time"
)

// DeclaredMediaType is the kind of a remote attachment as its document declares it: from the
// AS2 type (Image, Video, Audio), or for a Document from the mediaType. The media proxy never
// trusts it; it detects the format from the content.
func DeclaredMediaType(as2Type, mediaType string) MediaType {
	switch as2Type {
	case "Image":
		return MediaTypeImage
	case "Video":
		return MediaTypeVideo
	case "Audio":
		return MediaTypeAudio
	}
	switch mt := strings.ToLower(mediaType); {
	case strings.HasPrefix(mt, "image/"):
		return MediaTypeImage
	case strings.HasPrefix(mt, "video/"):
		return MediaTypeVideo
	case strings.HasPrefix(mt, "audio/"):
		return MediaTypeAudio
	}
	return MediaTypeUnknown
}

// RemoteRevisionSource is the remote subtype of a revision (remote_status_revisions): the
// activity that carried it, "" for a revision made from a fetch, the version of the HTML
// policy that sanitized its bodies (ADR-0019 §2), and the number of media attachments of the
// received post after the first MaxAttachments, which are left out.
type RemoteRevisionSource struct {
	ActivityURI        string
	HTMLPolicyVersion  int
	OmittedAttachments int
}

// CustomEmoji is a custom emoji, unique by Shortcode on Domain ("" for this instance). URI is
// the id of a remote emoji's Emoji object, "" when it has none. CreatedByUserID is the user who
// made a local emoji (ADR-0070 §7): the requester of an approved request, or the administrator
// who registered or copied it; "" for one made with the CLI and for a remote one.
type CustomEmoji struct {
	ID              string
	Shortcode       string
	Domain          string
	URI             string
	CreatedByUserID string
	CreatedAt       time.Time
}

// CustomEmojiImage is one image of a custom emoji (custom_emoji_images); the newest is current.
type CustomEmojiImage struct {
	ID        string
	EmojiID   string
	MediaID   string
	RevisedAt time.Time
}

// CurrentCustomEmoji is a custom emoji with its current image and its settings (the defaults
// when it has none).
type CurrentCustomEmoji struct {
	Emoji    CustomEmoji
	Image    CustomEmojiImage
	Settings CustomEmojiSettings
}

// CustomEmojiRecord is a custom emoji as the administrators manage it: with its current image,
// its settings, and its aliases, sorted.
type CustomEmojiRecord struct {
	CurrentCustomEmoji
	Aliases []string
}

// Local reports whether the emoji is of this instance.
func (e CustomEmoji) Local() bool { return e.Domain == "" }

// Label returns how the audit log names the emoji (ADR-0051 §5): :shortcode:, followed by
// @domain for a remote emoji.
func (e CustomEmoji) Label() string {
	if e.Domain == "" {
		return ":" + e.Shortcode + ":"
	}
	return ":" + e.Shortcode + ":@" + e.Domain
}

// MaxPollOptions bounds the choices of a remote poll that are kept.
const MaxPollOptions = 20

// Poll is the poll of a revision (FR-112). Options are ordered by Position, from 0.
type Poll struct {
	ID       string
	Multiple bool
	// ClosesAt is nil for a poll without an end.
	ClosesAt *time.Time
	// HideTotals hides the counts until the poll ends (a local poll's option; a remote poll
	// shows what its server reports).
	HideTotals bool
	Options    []PollOption
}

// PollOption is one choice of a poll.
type PollOption struct {
	Position int
	Title    string
}

// PollTallyObservation is the counts of a remote poll as its server reported them.
type PollTallyObservation struct {
	ID     string
	PollID string
	// VotersCount is nil when the server did not report it.
	VotersCount *int64
	ObservedAt  time.Time
	// Votes are the votes of each choice, by position.
	Votes []int64
}

// TotalVotes is the sum of the votes of every choice.
func (o PollTallyObservation) TotalVotes() int64 {
	var n int64
	for _, v := range o.Votes {
		n += v
	}
	return n
}

// Supersedes reports whether o replaces prev, the newest observation (the zero value when there
// is none). Votes on a remote poll are never taken back, so an observation with fewer votes in
// total is an older one that arrived late, and one with the same counts changes nothing: whatever
// order the Updates of a poll arrive in, the tallies end at the highest counts.
func (o PollTallyObservation) Supersedes(prev PollTallyObservation) bool {
	if prev.ID == "" {
		return true
	}
	if o.TotalVotes() < prev.TotalVotes() {
		return false
	}
	return !slices.Equal(o.Votes, prev.Votes) || !equalCount(o.VotersCount, prev.VotersCount)
}

func equalCount(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// PollTally is the current votes of one choice of a poll (the poll_tallies projection).
type PollTally struct {
	PollID      string
	Position    int
	Votes       int64
	VotersCount *int64
}

// Tallies returns the tallies that o makes current.
func (o PollTallyObservation) Tallies() []PollTally {
	out := make([]PollTally, len(o.Votes))
	for i, v := range o.Votes {
		out[i] = PollTally{PollID: o.PollID, Position: i, Votes: v, VotersCount: o.VotersCount}
	}
	return out
}
