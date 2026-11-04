// SPDX-License-Identifier: AGPL-3.0-only

package adr

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// AliasFile holds the numbers of the ADRs merged into another, relative to the repository root. Issues, pull
// requests, and commits cannot be rewritten, so the old numbers they cite resolve through it for good.
const AliasFile = Dir + "/aliases.yaml"

// Alias says where a retired ADR went: the ADR that holds it now and how its sections moved. Sections is
// looked at first, by the first component of the section number; then Section (every section went to one
// place); then Offset. A section none of them covers resolves to the ADR as a whole.
type Alias struct {
	To       string            `yaml:"to"`
	Section  string            `yaml:"section,omitempty"`
	Offset   int               `yaml:"offset,omitempty"`
	Sections map[string]string `yaml:"sections,omitempty"`
	Anchors  map[string]string `yaml:"anchors,omitempty"`
}

// Aliases maps a retired id (ADR-0099) to its Alias. The zero value has no aliases.
type Aliases map[string]Alias

var (
	reID  = regexp.MustCompile(`^ADR-\d{4}$`)
	reRef = regexp.MustCompile(`^(ADR-\d{4})(?:\s*([§#])\s*(\S+))?$`)
)

// LoadAliases reads AliasFile under root; a repository without the file has no aliases. It refuses an entry
// that does not name an ADR, or that points at another retired number: every old number is one step from the
// ADR that holds it, which is what makes rewriting citations safe to repeat.
func LoadAliases(root string) (Aliases, error) {
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(AliasFile)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var doc struct {
		Aliases Aliases `yaml:"aliases"`
	}
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", AliasFile, err)
	}
	for id, a := range doc.Aliases {
		if !reID.MatchString(id) || !reID.MatchString(a.To) {
			return nil, fmt.Errorf("%s: %s -> %q: both must be ADR-NNNN", AliasFile, id, a.To)
		}
		if _, retired := doc.Aliases[a.To]; retired {
			return nil, fmt.Errorf("%s: %s -> %s: the target is retired too", AliasFile, id, a.To)
		}
	}
	return doc.Aliases, nil
}

// Current returns the id that holds id now: its alias's target, or id itself.
func (as Aliases) Current(id string) string {
	if a, ok := as[id]; ok {
		return a.To
	}
	return id
}

// CurrentAll maps ids to the ADRs that hold them, in order and without repeats.
func (as Aliases) CurrentAll(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		cur := as.Current(id)
		if !seen[cur] {
			seen[cur] = true
			out = append(out, cur)
		}
	}
	return out
}

// Section returns the ADR and the section number that hold section sec of id. The number is "" when a retired
// ADR's section has no place of its own, so that the caller points at the ADR as a whole.
func (as Aliases) Section(id, sec string) (to, newSec string) {
	a, ok := as[id]
	if !ok {
		return id, sec
	}
	top, rest, sub := strings.Cut(sec, ".")
	join := func(s string) string {
		if sub {
			return s + "." + rest
		}
		return s
	}
	if s, ok := a.Sections[top]; ok {
		return a.To, join(s)
	}
	if a.Section != "" {
		return a.To, a.Section
	}
	if n, err := strconv.Atoi(top); err == nil && a.Offset != 0 {
		return a.To, join(strconv.Itoa(n + a.Offset))
	}
	return a.To, ""
}

// Ref rewrites a reference that starts with a retired number (ADR-0099, ADR-0099§5, ADR-0099#anchor) to where it
// lives now, and returns anything else unchanged. Every ADR has a #decision, so that anchor needs no entry.
func (as Aliases) Ref(ref string) string {
	m := reRef.FindStringSubmatch(ref)
	if m == nil {
		return ref
	}
	a, ok := as[m[1]]
	if !ok {
		return ref
	}
	switch m[2] {
	case "§":
		if _, s := as.Section(m[1], m[3]); s != "" {
			return a.To + "§" + s
		}
	case "#":
		if s, ok := a.Anchors[m[3]]; ok {
			return a.To + "#" + s
		}
		if strings.HasPrefix(m[3], "decision") {
			return a.To + "#" + m[3]
		}
	}
	return a.To
}
