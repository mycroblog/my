// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mycroblog/my/internal/domain/voice"
)

func tag(name, normalized string) voice.Hashtag {
	return voice.Hashtag{Name: name, NameNormalized: normalized}
}

func TestHashtags(t *testing.T) {
	t.Parallel()
	valid := []struct {
		name  string
		typed []string
		want  []voice.Hashtag
	}{
		{"none", nil, nil},
		{"with and without the #", []string{"Go", "#Music"}, []voice.Hashtag{tag("Go", "go"), tag("Music", "music")}},
		{"NFKC and lowercase", []string{"ＧＯ"}, []voice.Hashtag{tag("ＧＯ", "go")}},
		{"letters, marks, digits, and underscores", []string{"日本語_2026", "café"}, []voice.Hashtag{tag("日本語_2026", "日本語_2026"), tag("café", "café")}},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Hashtags(tc.typed)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Hashtags(%q) = %+v, %v; want %+v", tc.typed, got, err, tc.want)
			}
		})
	}
	for _, typed := range []string{"", "#", "2026", "go-lang", "go lang", " go", "go!", "##go", "#go#music", "https://example.com"} {
		t.Run("rejects "+typed, func(t *testing.T) {
			t.Parallel()
			var se *voice.SettingsError
			if got, err := Hashtags([]string{"ok", typed}); !errors.As(err, &se) || se.Field != voice.FieldHashtags {
				t.Errorf("Hashtags(%q) = %+v, %v; want a hashtags error", typed, got, err)
			}
		})
	}
}

func TestGraphemes(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]int{"": 0, "abc": 3, "e\u0301": 1, "\U0001F468\u200D\U0001F469\u200D\U0001F467": 1} {
		if got := Graphemes(s); got != want {
			t.Errorf("Graphemes(%q) = %d, want %d", s, got, want)
		}
	}
}
