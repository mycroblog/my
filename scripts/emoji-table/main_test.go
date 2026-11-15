// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const fixture = `# group: Smileys & Emotion
263A FE0F ; fully-qualified # ☺️ E0.6 smiling face
263A ; unqualified # ☺ E0.6 smiling face
# group: People & Body
1F44B ; fully-qualified # 👋 E0.6 waving hand
1F44B 1F3FB ; fully-qualified # 👋🏻 E1.0 waving hand: light skin tone
1F48F ; fully-qualified # 💏 E0.6 kiss
1F9D1 1F3FB 200D 2764 FE0F 200D 1F48B 200D 1F9D1 1F3FC ; fully-qualified # 🧑🏻‍❤️‍💋‍🧑🏼 E13.1 kiss: person, person, light skin tone, medium-light skin tone
1F469 200D 2764 FE0F 200D 1F48B 200D 1F468 ; fully-qualified # 👩‍❤️‍💋‍👨 E2.0 kiss: woman, man
1F469 1F3FB 200D 2764 FE0F 200D 1F48B 200D 1F468 1F3FB ; fully-qualified # 👩🏻‍❤️‍💋‍👨🏻 E13.1 kiss: woman, man, light skin tone
# group: Component
1F3FB ; component # 🏻 E1.0 light skin tone
1F3FC ; component # 🏼 E1.0 medium-light skin tone
1F3FD ; component # 🏽 E1.0 medium skin tone
1F3FE ; component # 🏾 E1.0 medium-dark skin tone
1F3FF ; component # 🏿 E1.0 dark skin tone
`

func TestBuildEmojiSet(t *testing.T) {
	entries, err := parseEmojiTest([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	set, err := buildEmojiSet(entries)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range set.groups {
		for _, e := range g.emoji {
			var skins []string
			for _, s := range e.skins {
				skins = append(skins, s.text()+"@"+s.version)
			}
			got = append(got, g.id+" "+e.text()+"@"+e.version+" "+strings.Join(skins, ","))
		}
	}
	want := []string{
		"smileys-emotion ☺️@0.6 ",
		"people-body 👋@0.6 👋🏻@1.0",
		// The base of the variant with two people of different tones is matched by the head of its name.
		"people-body 💏@0.6 🧑🏻‍❤️‍💋‍🧑🏼@13.1",
		"people-body 👩‍❤️‍💋‍👨@2.0 👩🏻‍❤️‍💋‍👨🏻@13.1",
	}
	if !slices.Equal(got, want) {
		t.Errorf("emoji set:\n got %q\nwant %q", got, want)
	}
	if len(set.tones) != 5 {
		t.Errorf("tones: got %d, want 5", len(set.tones))
	}
}

func TestBuildEmojiSetRejectsAnOrphanVariant(t *testing.T) {
	src := strings.Replace(fixture, "1F44B ; fully-qualified # 👋 E0.6 waving hand\n", "", 1)
	entries, err := parseEmojiTest([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildEmojiSet(entries); err == nil {
		t.Error("a skin tone variant without a base emoji must fail")
	}
}

func TestBuildNames(t *testing.T) {
	entries, err := parseEmojiTest([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	set, err := buildEmojiSet(entries)
	if err != nil {
		t.Fatal(err)
	}
	en := &annotations{
		plain: map[string]cldrEntry{
			// CLDR keys the sequence without VS16.
			"☺":  {Default: []string{"face", "smile"}, TTS: []string{"smiling face"}},
			"👋":  {Default: []string{"hand", "wave", "waving hand"}, TTS: []string{"waving hand"}},
			"💏":  {TTS: []string{"kiss"}},
			"🏻":  {TTS: []string{"light skin tone"}},
			"🏼":  {TTS: []string{"medium-light skin tone"}},
			"🏽":  {TTS: []string{"medium skin tone"}},
			"🏾":  {TTS: []string{"medium-dark skin tone"}},
			"🏿":  {TTS: []string{"dark skin tone"}},
			"👋🏻": {TTS: []string{"not used: the plain annotations come first"}},
		},
		derived: map[string]cldrEntry{
			"👋🏻":         {TTS: []string{"waving hand: light skin tone"}},
			"👩‍❤‍💋‍👨":    {TTS: []string{"kiss: woman, man"}},
			"🧑🏻‍❤‍💋‍🧑🏼":  {TTS: []string{"kiss: person, person, light skin tone, medium-light skin tone"}},
			"👩🏻‍❤️‍💋‍👨🏻": {TTS: []string{"kiss: woman, man, light skin tone"}},
		},
	}
	ja := &annotations{
		plain:   map[string]cldrEntry{"☺": {Default: []string{"笑顔"}, TTS: []string{"スマイル"}}},
		derived: map[string]cldrEntry{},
	}

	names, fallbacks := buildNames(set, en, en)
	if fallbacks != 0 {
		t.Errorf("en fallbacks: got %d, want 0", fallbacks)
	}
	got := string(namesJSON(names))
	for _, want := range []string{
		`"☺️": ["smiling face",["face","smile"]]`,
		// The name is not repeated in the keywords.
		`"👋": ["waving hand",["hand","wave"]]`,
		`"💏": ["kiss",[]]`,
		`"👋🏻": "not used: the plain annotations come first"`,
		`"🧑🏻‍❤️‍💋‍🧑🏼": "kiss: person, person, light skin tone, medium-light skin tone"`,
		`"🏿": "dark skin tone"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("en data has no %s:\n%s", want, got)
		}
	}

	names, fallbacks = buildNames(set, ja, en)
	if fallbacks != len(names.emoji)+len(names.skins)+len(names.tones)-1 {
		t.Errorf("ja fallbacks: got %d", fallbacks)
	}
	got = string(namesJSON(names))
	for _, want := range []string{`"☺️": ["スマイル",["笑顔"]]`, `"👋": ["waving hand",["hand","wave"]]`} {
		if !strings.Contains(got, want) {
			t.Errorf("ja data has no %s:\n%s", want, got)
		}
	}
	// Without an en annotation either, the name is emoji-test.txt's.
	names, _ = buildNames(set, ja, ja)
	if got := string(namesJSON(names)); !strings.Contains(got, `"💏": ["kiss",[]]`) {
		t.Errorf("ja data without en has no emoji-test.txt name:\n%s", got)
	}
}

// The data the web client commits.
type baseData struct {
	EmojiVersion string `json:"emojiVersion"`
	Groups       []struct {
		ID    string            `json:"id"`
		Emoji []json.RawMessage `json:"emoji"`
	} `json:"groups"`
}

type namesData struct {
	Tones map[string]string            `json:"tones"`
	Emoji map[string][]json.RawMessage `json:"emoji"`
	Skins map[string]string            `json:"skins"`
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	src, err := os.ReadFile(path) //nolint:gosec // G304: a file of the repository the test names
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	src := readFile(t, path)
	if err := json.Unmarshal(src, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// TestCommittedData checks the committed web client data against the committed server table, which one run of the
// generator writes from the same emoji-test.txt: the web data has every fully-qualified emoji, and en and ja name
// each of them.
func TestCommittedData(t *testing.T) {
	root := filepath.Join("..", "..")
	src := readFile(t, filepath.Join(root, "internal", "domain", "content", "emoji_table.gen.go"))
	_, table, _ := strings.Cut(string(src), "fullyQualifiedEmoji = `")
	table, _, _ = strings.Cut(table, "`")
	var want []string
	for line := range strings.SplitSeq(table, "\n") {
		var rs []rune
		for p := range strings.FieldsSeq(line) {
			r, err := parseCodePoint(p)
			if err != nil {
				t.Fatal(err)
			}
			rs = append(rs, r)
		}
		want = append(want, string(rs))
	}
	if len(want) < 3000 {
		t.Fatalf("the server table has only %d emoji", len(want))
	}

	dir := filepath.Join(root, "web", "app-features", "src", "emoji", "unicode")
	var base baseData
	readJSON(t, filepath.Join(dir, "base.gen.json"), &base)
	if len(base.Groups) != 9 {
		t.Errorf("groups: got %d, want 9", len(base.Groups))
	}
	var bases, skins []string
	for _, g := range base.Groups {
		for _, raw := range g.Emoji {
			var item []json.RawMessage
			if err := json.Unmarshal(raw, &item); err != nil {
				t.Fatal(err)
			}
			var text string
			if err := json.Unmarshal(item[0], &text); err != nil {
				t.Fatal(err)
			}
			bases = append(bases, text)
			if len(item) == 3 {
				var variants [][2]string
				if err := json.Unmarshal(item[2], &variants); err != nil {
					t.Fatal(err)
				}
				for _, v := range variants {
					skins = append(skins, v[0])
				}
			}
		}
	}
	got := slices.Concat(bases, skins)
	if !sameSet(got, want) {
		t.Errorf("base.gen.json has %d emoji, the server table %d fully-qualified emoji; they must be the same set", len(got), len(want))
	}

	for _, locale := range []string{"en", "ja"} {
		var names namesData
		readJSON(t, filepath.Join(dir, locale+".gen.json"), &names)
		if len(names.Tones) != 5 {
			t.Errorf("%s: tones: got %d, want 5", locale, len(names.Tones))
		}
		for _, e := range bases {
			item := names.Emoji[e]
			var name string
			if len(item) != 2 || json.Unmarshal(item[0], &name) != nil || name == "" {
				t.Errorf("%s: no name of %s", locale, e)
			}
		}
		for _, e := range skins {
			if names.Skins[e] == "" {
				t.Errorf("%s: no name of the skin tone variant %s", locale, e)
			}
		}
		if len(names.Emoji) != len(bases) || len(names.Skins) != len(skins) {
			t.Errorf("%s: %d names and %d skin tone names for %d emoji and %d variants", locale, len(names.Emoji), len(names.Skins), len(bases), len(skins))
		}
	}
}

func sameSet(a, b []string) bool {
	a, b = slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b))
	return slices.Equal(a, b)
}
