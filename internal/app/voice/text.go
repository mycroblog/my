// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"strings"

	"github.com/rivo/uniseg"

	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/voice"
)

// Graphemes counts the grapheme clusters of s under Unicode text segmentation (UAX #29), the
// counter that the settings of a room are validated with (voice.Settings.Validate).
func Graphemes(s string) int { return uniseg.GraphemeClusterCount(s) }

var _ voice.GraphemeCounter = Graphemes

// Hashtags turns the hashtags of a room, as the host typed them with or without their leading
// #, into the room's hashtags in the same order (ADR-0100 §3). Each one must be a whole hashtag
// under the grammar of the hashtags of statuses (content.ParseText: letters, marks, digits, and
// underscores, with at least one letter or mark), and its normalized name is the one of the
// content module (content.NormalizeTag). It returns a *voice.SettingsError for the hashtags
// otherwise; the count, the lengths, and the duplicates are voice.Settings.Validate's.
func Hashtags(typed []string) ([]voice.Hashtag, error) {
	if len(typed) == 0 {
		return nil, nil
	}
	out := make([]voice.Hashtag, 0, len(typed))
	for _, t := range typed {
		tag := "#" + strings.TrimPrefix(t, "#")
		segs := content.ParseText(tag).Segments
		if len(segs) != 1 || segs[0].Kind != content.SegmentHashtag || segs[0].Text != tag {
			return nil, &voice.SettingsError{Field: voice.FieldHashtags}
		}
		h := segs[0].Hashtag
		out = append(out, voice.Hashtag{Name: h.Name, NameNormalized: h.NameNormalized})
	}
	return out, nil
}
