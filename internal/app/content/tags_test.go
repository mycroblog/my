// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
)

func TestParseHashtag(t *testing.T) {
	for _, tc := range []struct {
		in, name, normalized string
		ok                   bool
	}{
		{in: "Cats", name: "Cats", normalized: "cats", ok: true},
		{in: "#Cats", name: "Cats", normalized: "cats", ok: true},
		{in: "猫_2026", name: "猫_2026", normalized: "猫_2026", ok: true},
		{in: "ＣＡＴＳ", name: "ＣＡＴＳ", normalized: "cats", ok: true},
		{in: strings.Repeat("a", appcontent.MaxHashtagLength), name: strings.Repeat("a", appcontent.MaxHashtagLength), normalized: strings.Repeat("a", appcontent.MaxHashtagLength), ok: true},
		{in: strings.Repeat("a", appcontent.MaxHashtagLength+1)},
		{in: ""},
		{in: "#"},
		{in: "##cats"},
		{in: "2026"},
		{in: "cats dogs"},
		{in: "cats-dogs"},
		{in: "\xff"},
	} {
		h, ok := appcontent.ParseHashtag(tc.in)
		if ok != tc.ok || h.Name != tc.name || h.NameNormalized != tc.normalized {
			t.Errorf("ParseHashtag(%q) = %+v, %v; want %q, %q, %v", tc.in, h, ok, tc.name, tc.normalized, tc.ok)
		}
	}
}
