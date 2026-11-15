// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// componentGroup is the group of emoji-test.txt whose emoji (the skin tone modifiers and hair components alone)
// the picker does not offer.
const componentGroup = "Component"

// isSkinTone reports whether r is one of the five skin tone modifiers (U+1F3FB..U+1F3FF).
func isSkinTone(r rune) bool { return r >= 0x1F3FB && r <= 0x1F3FF }

// baseEmoji is an emoji of the picker without a skin tone, with its skin tone variants.
type baseEmoji struct {
	entry
	skins []entry
}

// emojiGroup is a group of emoji-test.txt with its emoji in the file's order.
type emojiGroup struct {
	id    string // "smileys-emotion"
	emoji []*baseEmoji
}

// emojiSet is the web client's emoji: every fully-qualified emoji outside the Component group, as base emoji in
// their groups with their skin tone variants, and the five skin tone modifiers.
type emojiSet struct {
	groups []emojiGroup
	tones  []entry
}

// groupID turns the name of a group into the identifier the client keys its messages with:
// "Smileys & Emotion" becomes "smileys-emotion".
func groupID(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.ReplaceAll(name, " & ", " ")), " ", "-")
}

// baseNameCandidates lists the names that the base emoji of a skin tone variant may have, most specific first.
// The base of "kiss: woman, man, light skin tone" is "kiss: woman, man", but the base of
// "kiss: person, person, light skin tone, medium-light skin tone" is "kiss": removing the modifiers from a sequence
// does not always give its base, so the variants are matched by name.
func baseNameCandidates(name string) []string {
	head, parts, ok := strings.Cut(name, ": ")
	if !ok {
		return nil
	}
	var kept []string
	for p := range strings.SplitSeq(parts, ", ") {
		if !strings.HasSuffix(p, "skin tone") {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return []string{head}
	}
	return []string{head + ": " + strings.Join(kept, ", "), head}
}

// buildEmojiSet groups the fully-qualified emoji of emoji-test.txt and attaches each skin tone variant to its base.
func buildEmojiSet(entries []entry) (*emojiSet, error) {
	set := &emojiSet{}
	byName := map[string]*baseEmoji{}
	var variants []entry
	for _, e := range entries {
		if e.group == componentGroup {
			if e.status == "component" && len(e.points) == 1 && isSkinTone(e.points[0]) {
				set.tones = append(set.tones, e)
			}
			continue
		}
		if e.status != "fully-qualified" {
			continue
		}
		if slices.ContainsFunc(e.points, isSkinTone) {
			variants = append(variants, e)
			continue
		}
		id := groupID(e.group)
		if len(set.groups) == 0 || set.groups[len(set.groups)-1].id != id {
			set.groups = append(set.groups, emojiGroup{id: id})
		}
		g := &set.groups[len(set.groups)-1]
		b := &baseEmoji{entry: e}
		g.emoji = append(g.emoji, b)
		byName[e.name] = b
	}
	for _, v := range variants {
		var base *baseEmoji
		for _, name := range baseNameCandidates(v.name) {
			if base = byName[name]; base != nil {
				break
			}
		}
		if base == nil {
			return nil, fmt.Errorf("no base emoji for the skin tone variant %q (%s)", v.name, v.hex)
		}
		base.skins = append(base.skins, v)
	}
	if len(set.tones) != 5 {
		return nil, fmt.Errorf("emoji-test.txt has %d skin tone modifiers in the Component group, not 5", len(set.tones))
	}
	return set, nil
}

// cldrEntry is the annotation of one emoji in a cldr-json annotations file.
type cldrEntry struct {
	Default []string `json:"default"`
	TTS     []string `json:"tts"`
}

// annotations holds the CLDR annotations and derived annotations of a locale.
type annotations struct {
	plain, derived map[string]cldrEntry
}

func readCLDR(path, root string) (map[string]cldrEntry, error) {
	src, err := os.ReadFile(path) //nolint:gosec // G304: an input under the -cldr directory the task passes
	if err != nil {
		return nil, err
	}
	var doc map[string]struct {
		Annotations map[string]cldrEntry `json:"annotations"`
	}
	if err := json.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m := doc[root].Annotations
	if len(m) == 0 {
		return nil, fmt.Errorf("%s: no %s.annotations", path, root)
	}
	return m, nil
}

func loadAnnotations(dir, locale string) (*annotations, error) {
	plain, err := readCLDR(filepath.Join(dir, "annotations", locale+".json"), "annotations")
	if err != nil {
		return nil, err
	}
	derived, err := readCLDR(filepath.Join(dir, "annotationsDerived", locale+".json"), "annotationsDerived")
	if err != nil {
		return nil, err
	}
	return &annotations{plain: plain, derived: derived}, nil
}

// annotation is the CLDR short name and the keywords of an emoji.
type annotation struct {
	name     string
	keywords []string
}

// lookup finds the annotation of an emoji. CLDR keys its annotations by the sequence without VS16 (U+FE0F), so the
// fully-qualified sequence is looked up as it is and without VS16, in the annotations and then the derived ones
// (sequences such as flags, keycaps, and skin tone variants).
func (a *annotations) lookup(text string) (annotation, bool) {
	for _, key := range []string{text, strings.ReplaceAll(text, "️", "")} {
		for _, m := range []map[string]cldrEntry{a.plain, a.derived} {
			e, ok := m[key]
			if !ok || len(e.TTS) == 0 || e.TTS[0] == "" {
				continue
			}
			name := e.TTS[0]
			var keywords []string
			for _, k := range e.Default {
				if k != name {
					keywords = append(keywords, k)
				}
			}
			return annotation{name: name, keywords: keywords}, true
		}
	}
	return annotation{}, false
}

// localeNames is the data of a locale: the names and keywords of the base emoji, the names of the skin tone variants,
// and the names of the five skin tones, in emoji-test.txt's order.
type localeNames struct {
	tones []namedEmoji
	emoji []namedEmoji
	skins []namedEmoji
}

type namedEmoji struct {
	text string
	annotation
}

// buildNames names every emoji of set in a locale. An emoji without a CLDR name in the locale takes its en
// annotation, or else the English name of emoji-test.txt; fallbacks counts them.
func buildNames(set *emojiSet, ann, en *annotations) (names localeNames, fallbacks int) {
	name := func(e entry) namedEmoji {
		if a, ok := ann.lookup(e.text()); ok {
			return namedEmoji{e.text(), a}
		}
		fallbacks++
		if a, ok := en.lookup(e.text()); ok {
			return namedEmoji{e.text(), a}
		}
		return namedEmoji{e.text(), annotation{name: e.name}}
	}
	for _, t := range set.tones {
		names.tones = append(names.tones, name(t))
	}
	for _, g := range set.groups {
		for _, b := range g.emoji {
			names.emoji = append(names.emoji, name(b.entry))
			for _, s := range b.skins {
				names.skins = append(names.skins, name(s))
			}
		}
	}
	return names, fallbacks
}

// js encodes v as JSON without escaping <, >, and &.
func js(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // strings and slices of strings always encode
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// writeLines writes the JSON members or elements in lines, one per line, separated by commas.
func writeLines(b *bytes.Buffer, indent string, lines []string) {
	for i, l := range lines {
		b.WriteString(indent)
		b.WriteString(l)
		if i < len(lines)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
}

// baseJSON renders base.gen.json: the versions, then the groups in emoji-test.txt's order, each with its emoji in
// that order as [emoji, version] or [emoji, version, [[variant, version], ...]]. One emoji per line keeps the diffs
// of a version update readable.
func baseJSON(set *emojiSet, version, cldrVersion string) []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"emojiVersion\": %s,\n  \"cldrVersion\": %s,\n  \"groups\": [\n", js(version), js(cldrVersion))
	for i, g := range set.groups {
		fmt.Fprintf(&b, "    {\n      \"id\": %s,\n      \"emoji\": [\n", js(g.id))
		lines := make([]string, 0, len(g.emoji))
		for _, e := range g.emoji {
			item := []any{e.text(), e.version}
			if len(e.skins) > 0 {
				skins := make([][]string, 0, len(e.skins))
				for _, s := range e.skins {
					skins = append(skins, []string{s.text(), s.version})
				}
				item = append(item, skins)
			}
			lines = append(lines, js(item))
		}
		writeLines(&b, "        ", lines)
		b.WriteString("      ]\n    }")
		if i < len(set.groups)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n}\n")
	return b.Bytes()
}

// namesJSON renders <locale>.gen.json: "tones" maps each skin tone modifier to its name, "emoji" each base emoji to
// [name, [keywords...]], and "skins" each skin tone variant to its name.
func namesJSON(names localeNames) []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	section := func(key string, items []namedEmoji, value func(namedEmoji) any, last bool) {
		fmt.Fprintf(&b, "  %s: {\n", js(key))
		lines := make([]string, 0, len(items))
		for _, it := range items {
			lines = append(lines, js(it.text)+": "+js(value(it)))
		}
		writeLines(&b, "    ", lines)
		b.WriteString("  }")
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	nameOnly := func(n namedEmoji) any { return n.name }
	section("tones", names.tones, nameOnly, false)
	section("emoji", names.emoji, func(n namedEmoji) any {
		keywords := n.keywords
		if keywords == nil {
			keywords = []string{}
		}
		return []any{n.name, keywords}
	}, false)
	section("skins", names.skins, nameOnly, true)
	b.WriteString("}\n")
	return b.Bytes()
}
