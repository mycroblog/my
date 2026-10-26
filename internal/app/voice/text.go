// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"github.com/rivo/uniseg"

	"github.com/mycroblog/my/internal/domain/voice"
)

// Graphemes counts the grapheme clusters of s under Unicode text segmentation (UAX #29), the
// counter that the settings of a room are validated with (voice.Settings.Validate).
func Graphemes(s string) int { return uniseg.GraphemeClusterCount(s) }

var _ voice.GraphemeCounter = Graphemes
